package ggui

import (
	"image"
	"image/color"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/inspect"
	"github.com/ironpark/ggui/internal/framestats"
	"github.com/ironpark/ggui/internal/platform/a11ybridge"
	"github.com/ironpark/ggui/internal/platform/textinput"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/runtime"
	"github.com/ironpark/ggwd"
	"github.com/ironpark/ggwd/native"
)

// Config describes a window: the one New opens, or one App.OpenWindow
// adds. Only Title, Width and Height are commonly set; the zero value of
// every other field is an ordinary decorated window centered on the screen.
type Config struct {
	Title  string
	Width  int
	Height int

	// The size limits in logical pixels. Zero means no limit.
	MinWidth  int
	MinHeight int
	MaxWidth  int
	MaxHeight int

	// Position is where the window opens on the screen, in logical pixels;
	// nil centers it.
	Position *Point

	Resizable bool
	// Frameless opens the window without its title bar and border.
	Frameless bool
	// AlwaysOnTop keeps the window above ordinary windows.
	AlwaysOnTop bool
	// Transparent lets what is behind the window show through wherever
	// nothing is painted. The window's background is clear unless
	// Background says otherwise. DirectX cannot present such a window, so
	// on a Windows machine that chose it the window fails to open.
	Transparent bool
	// Hidden opens the window without showing it; Show shows it.
	Hidden bool
	// Maximized opens the window maximized.
	Maximized bool
	// Unfocused opens the window without taking the keyboard focus.
	Unfocused bool
	// Icon is the window's icon in several sizes, where the platform shows
	// one; the platform picks the size closest to what it needs.
	Icon []image.Image

	Background color.Color // nil follows the root environment background
	Inspector  string      // a chord that toggles the widget inspector, such as "f1"; empty for none. Import ggui/inspect/panel for the panel.

	// Accessibility says when the window talks to the platform's
	// accessibility API. The zero value waits for an assistive technology
	// to attach and costs nothing while none has; see AccessibilityMode.
	Accessibility AccessibilityMode
}

func (c Config) withDefaults() Config {
	if c.Title == "" {
		c.Title = "ggui"
	}
	if c.Width == 0 {
		c.Width = 800
	}
	if c.Height == 0 {
		c.Height = 600
	}
	return c
}

// options is the ggfx form of c.
func (c Config) options() *ggwd.WindowOptions {
	o := &ggwd.WindowOptions{
		Title:       c.Title,
		Width:       float64(c.Width),
		Height:      float64(c.Height),
		Resizable:   c.Resizable,
		Undecorated: c.Frameless,
		Floating:    c.AlwaysOnTop,
		Transparent: c.Transparent,
		Hidden:      c.Hidden,
		Maximized:   c.Maximized,
		Unfocused:   c.Unfocused,
		MinWidth:    float64(c.MinWidth),
		MinHeight:   float64(c.MinHeight),
		MaxWidth:    float64(c.MaxWidth),
		MaxHeight:   float64(c.MaxHeight),
	}
	if c.Position != nil {
		o.Position = &ggwd.Point{X: c.Position.X, Y: c.Position.Y}
	}
	return o
}

// WindowState is whether a window is shown normally, minimized, maximized
// or fullscreen.
type WindowState uint8

// The states a window can be in.
const (
	WindowNormal WindowState = iota
	WindowMinimized
	WindowMaximized
	WindowFullscreen
)

// Window is one native window and the widget tree it shows. Each frame it
// routes input to the regions painted last frame, settles reactive
// bindings and layout, runs user effects, then paints and collects the
// next frame's regions. Its root setup runs once, when it opens.
//
// Every window of an App shares the one UI thread and the process's
// reactive runtime, so a signal written in one window updates every window
// that reads it. New opens the first window; App.OpenWindow opens more.
type Window struct {
	frameErr error
	frameLoop
	app *App
	cfg Config

	canvas Canvas // also holds the frame's screen-pixels-per-logical-pixel scale
	input  inputState
	touch  touchInput
	cursor CursorShape

	window        atomic.Pointer[ggwd.Window] // the native window, once open; read by Post from any goroutine
	pending       frameInput                  // input gathered from events since the last frame
	pos           Point                       // the last known pointer position, in logical pixels
	wakeTimer     *time.Timer
	keyboard      ggwd.Keyboard          // keys held, for Mods
	touches       map[ggwd.TouchID]Point // touches in progress, in logical pixels
	ax            a11ybridge.Bridge
	semWanted     atomic.Bool  // a caller of Semantics reads the tree
	performs      atomic.Int32 // Perform calls not yet carried out, which need a tree
	dragOver      bool
	closeAsked    bool // a close the OnCloseRequest handlers have yet to answer
	dragAt        Point
	seenGen       uint64            // layoutGen as this window's last frame left it
	menuShortcuts []*ShortcutHandle // the app menus' chords, where the menu bar does not run them

	inspect      bool
	inspectChord Chord          // parsed from cfg.Inspector; Key is zero for none
	panel        InspectorPanel // the registered inspector, once wanted
	sinks        []func(*inspect.Frame)
	overlay      Overlay // drawn over every frame; the inspector while it is on

	// backbuf is what the window paints into while a BackdropBlur needs to
	// read the frame, copied to the screen at the end; backdropTTL counts
	// down the frames it is kept after the last blur.
	backbuf     *ggfx.Image
	backdropTTL int

	// A frame only time asks for is painted in part, over the last frame
	// where it is kept (see damage.go): motion is the regions it paints,
	// motionWhy whether the last frame allows it, and motionGen and
	// motionSize the state and the size it was painted at. dirty is set by
	// all else that asks for a frame, which then paints whole.
	motion    []Rect
	motionGen uint64
	// freeSeen and fontSeen are reactive.FreeGen and fontGen as the last
	// frame left them: a frame a scroll contains has neither move.
	freeSeen, fontSeen uint64
	motionSize         image.Point
	motionWhy          string // why the next frame may not be painted in part, or "ok"
	// motionLost says the last frame moved with time where no region
	// says, as a dialog that takes no room does: the next is painted
	// whole, never skipped.
	motionLost bool
	drawing    windowDraw        // the whole frame being drawn
	damagePx   []image.Rectangle // what a frame hands setDamage
	dirty      atomic.Uint32
	repaintMu  sync.Mutex
	repaints   []Rect // asked for by RepaintRect since the last frame

	focused  *StateValue[bool]
	viewport *StateValue[Size]
	state    *StateValue[WindowState]
}

// newWindow makes a window for app that is not open yet.
func newWindow(app *App, rt *reactive.Runtime, cfg Config, build Builder) *Window {
	w := &Window{app: app, cfg: cfg.withDefaults()}
	w.host = w
	w.rt = rt
	w.wake = func() {
		// Posted work is painted by what it changes: a signal or the
		// layout keeps the next frame from being painted in part, and a
		// change outside them asks Repaint. Waking alone marks nothing.
		w.wakeFrame()
		// The window may draw no frame, hidden or covered as it may be;
		// the app runs the work anyway.
		if app != nil {
			app.wakeUp()
		}
	}
	w.build = build
	w.dialogs = w.nativeDialogs()
	w.clipboard = runtime.NativeClipboard()
	w.focused = State(!cfg.Unfocused && !cfg.Hidden)
	w.viewport = State(Sz(float64(w.cfg.Width), float64(w.cfg.Height)))
	w.state = State(WindowNormal)
	if cfg.Maximized {
		w.state.Set(WindowMaximized)
	}
	if cfg.Inspector != "" {
		// A chord rather than a KeyboardKey, whose zero value is KeyA and
		// would have made "Inspector: ggui.KeyA" mean none.
		w.inspectChord = MustChord(cfg.Inspector)
	}
	return w
}

// open builds the tree and creates the native window. It runs on the UI
// thread while the engine runs, from inside an event.
func (w *Window) open() error {
	prev := runningLoop()
	w.start()
	// Opening a window from another window's frame must leave that frame's
	// loop current.
	if prev != nil {
		setRunning(prev)
	}
	w.ax.Start(w.Perform, w.cfg.Accessibility)
	nw, err := ggwd.NewWindow(w.cfg.options())
	if err != nil {
		w.release()
		return err
	}
	w.window.Store(nw)
	nw.SetTextInputEnabled(false)
	if len(w.cfg.Icon) > 0 {
		nw.SetIcon(w.cfg.Icon)
	}
	w.ax.SetWindow(nw)
	// Hide, Maximize and Restore before the window opened changed the
	// Config it opened with, not the signals newWindow set; the platform
	// reports no change from the state it opened in, so set them here.
	// Posted, they run before what the window's events post.
	focused := !w.cfg.Unfocused && !w.cfg.Hidden
	state := WindowNormal
	if w.cfg.Maximized {
		state = WindowMaximized
	}
	w.post(func() {
		w.focused.Set(focused)
		w.state.Set(state)
	})
	return nil
}

// Setup registers fn to run under the window's root owner before the first
// build, so derived values and watchers have an owner and are disposed by
// Close. Call it before the window opens.
func (w *Window) Setup(fn func()) *Window {
	w.setup = append(w.setup, fn)
	return w
}

// Post queues fn to run on the UI thread before the window's next frame's
// input. It is the one way a goroutine may touch signals: do the work off
// the thread, then Post the Set. Work posted by a callback runs next frame.
// Work posted before the window opens, such as before App.Run, waits and
// runs once, after Setup and the first build.
func (w *Window) Post(fn func()) { w.post(fn) }

// Close closes the window and disposes its root owner, and with it every
// effect, memo and component the window created. It does not ask the
// OnCloseRequest handlers. Closing an App's last window ends App.Run.
// Call it from the UI thread; from a goroutine, Post it. Closing twice is
// harmless.
func (w *Window) Close() {
	if w.closed {
		return
	}
	w.release()
	if nw := w.window.Load(); nw != nil && w.app != nil && w.app.running {
		nw.Close()
	}
}

// release frees what the window holds without closing the native one.
func (w *Window) release() {
	if w.panel != nil {
		w.panel.Release()
	}
	if nw := w.window.Load(); nw != nil {
		textinput.CloseWindow(nw)
	}
	if w.wakeTimer != nil {
		w.wakeTimer.Stop()
	}
	w.ax.Close()
	w.close()
	w.canvas.freeLayers()
	w.freeBackbuf()
}

// Closed reports whether the window has closed. It may be called from any
// goroutine.
func (w *Window) Closed() bool {
	w.postMu.Lock()
	defer w.postMu.Unlock()
	return w.closed
}

// App returns the app the window belongs to.
func (w *Window) App() *App { return w.app }

// handle routes one of the window's events. Input is gathered until the
// next frame, which it requests; a FrameEvent runs the frame.
func (w *Window) handle(ev ggwd.Event) error {
	switch ev := ev.(type) {
	case ggwd.FrameEvent:
		return w.runFrameEvent(ev)
	case ggwd.KeyEvent:
		w.keyboard.SyncMods(ev.Modifiers)
		w.keyboard.Set(ev.Key, ev.Pressed)
		if ev.Pressed {
			// The modifiers of this press, from the held keys and from the
			// event itself, which sees a modifier the OS never reported as a
			// key. Presses within one frame share the union, so a chord is
			// not lost when its modifier was released before the frame ran.
			m := w.keyboard.Mods().Or(ev.Modifiers)
			w.pending.mods = w.pending.mods.Or(m)
			if w.cfg.Inspector != "" && (KeyEvent{Kind: KeyPress, Key: ev.Key, Mods: m}).Is(w.inspectChord) {
				// The inspector's chord is the inspector's: the focused
				// widget sees neither the press nor its repeats.
				if !ev.Repeat {
					w.Inspector(!w.inspect)
				}
			} else {
				w.pending.keys = append(w.pending.keys, ev.Key)
			}
		} else {
			w.pending.released = append(w.pending.released, ev.Key)
		}
		w.markDirty(dirtyEvent)
	case ggwd.CompositionEvent:
		textinput.HandleEvent(ev)
		w.markDirty(dirtyEvent)
	case ggwd.TextEvent:
		if !textinput.HandleEvent(ev) {
			w.pending.text += ev.Text
		}
		w.markDirty(dirtyEvent)
	case ggwd.MouseMoveEvent:
		w.pos = Pt(ev.X, ev.Y)
		if w.canvas.fs().pointerRead {
			w.markDirty(dirtyEvent)
		} else {
			// What the move reaches is painted again, as hovering it
			// enters or leaves a region; see deliver.
			w.wakeFrame()
		}
	case ggwd.MouseButtonEvent:
		w.pos = Pt(ev.X, ev.Y)
		// A modifier pressed while another window had the focus reached
		// no KeyEvent here; the click still sees it.
		w.keyboard.SyncMods(ev.Modifiers)
		w.pending.mods = w.pending.mods.Or(ev.Modifiers)
		if ev.Pressed {
			w.pending.down = append(w.pending.down, ev.Button)
		} else {
			w.pending.up = append(w.pending.up, ev.Button)
		}
		w.markDirty(dirtyEvent)
	case ggwd.ScrollEvent:
		w.keyboard.SyncMods(ev.Modifiers)
		w.pending.mods = w.pending.mods.Or(ev.Modifiers)
		if ev.Precise {
			// A trackpad's points, as the logical pixels they move.
			w.pending.wheelPx = w.pending.wheelPx.Add(Pt(ev.X*ggwd.PreciseLine, ev.Y*ggwd.PreciseLine))
		} else {
			w.pending.wheel = w.pending.wheel.Add(Pt(ev.X, ev.Y))
		}
		w.pending.phase = laterPhase(w.pending.phase, ev.Phase)
		// The wheel is a pointer event: what it reaches is painted again
		// (see deliver), and what it scrolls through a signal or a layout
		// marks the frame whole by itself.
		w.wakeFrame()
	case ggwd.TouchEvent:
		if w.touches == nil {
			w.touches = map[ggwd.TouchID]Point{}
		}
		if ev.Phase == ggwd.TouchPhaseEnded {
			delete(w.touches, ev.ID)
		} else {
			w.touches[ev.ID] = Pt(ev.X, ev.Y)
		}
		w.markDirty(dirtyEvent)
	case ggwd.DragEvent:
		switch ev.Phase {
		case ggwd.DragEntered, ggwd.DragMoved:
			w.dragOver, w.dragAt = true, Pt(ev.X, ev.Y)
		case ggwd.DragExited, ggwd.DragEnded:
			w.dragOver = false
		}
		w.markDirty(dirtyEvent)
	case ggwd.DropEvent:
		w.pending.drop = append(w.pending.drop, droppedFiles(ev.Files)...)
		w.markDirty(dirtyEvent)
	case ggwd.CloseEvent:
		if len(w.closeRequests) > 0 {
			// The handlers decide in a frame, where they may put up a
			// question; the window stays until they agree or Close is called.
			ev.KeepOpen()
			if w.closeAsked {
				// A second click before they ran asks them nothing more.
				break
			}
			w.closeAsked = true
			w.post(func() {
				w.closeAsked = false
				if w.closeAllowed() {
					w.Close()
				}
			})
			break
		}
		// The window closes after this event. Dispose the tree and detach
		// the native hooks first, while the window still exists.
		w.release()
	case ggwd.FocusEvent:
		if !ev.Focused {
			// Releases are not reported while another window has the focus.
			w.keyboard.Reset()
		}
		w.post(func() { w.focused.Set(ev.Focused) })
	case ggwd.WindowStateEvent:
		w.post(func() { w.state.Set(windowState(ev.State)) })
	}
	return nil
}

// markDirty notes that something asked for the window to be painted whole,
// by why (see damage.go).
func (w *Window) markDirty(reason dirtyReason) {
	w.dirty.Or(uint32(reason))
	w.wakeFrame()
}

// requestFrame asks the window for a frame. It is safe from any goroutine
// and before the window exists, when the first frame comes on its own.
func (w *Window) requestFrame() {
	w.markDirty(dirtyOther)
}

// Repaint asks for the whole window to be painted in the next frame, for
// a change that no signal and no layout records, such as fields that work
// posted with Post wrote: posted work alone repaints only what its signals
// and layout changes reach. Safe to call from any goroutine.
func (w *Window) Repaint() {
	noteCaller(dirtyRepaint)
	w.markDirty(dirtyRepaint)
}

// wakeFrame asks for a frame for the time a widget asked to be painted
// again at, which nothing else changes: it may be painted in part.
func (w *Window) wakeFrame() {
	if nw := w.window.Load(); nw != nil {
		nw.RequestFrame()
	}
}

// OnFrame registers fn to run once per frame, before input is dispatched
// and effects are flushed, for per-frame work that no widget owns. OnKey
// covers global shortcuts; Pointer and Focus cover input aimed at a widget.
func (w *Window) OnFrame(fn func()) {
	w.frame = append(w.frame, fn)
}

// OnKey registers a window-wide key handler: fn sees every key press, with
// its modifiers, before the focused widget does, and a press it returns
// true for goes no further. A held key repeats into it as it would into a
// widget. Shortcut is the simpler form for one chord; OnKey is for what a
// chord cannot say. Keep the keys it takes away from a text field in
// mind: returning true for Space while one is focused would eat the space
// bar.
func (w *Window) OnKey(fn func(KeyEvent) bool) {
	w.input.shortcuts = append(w.input.shortcuts, fn)
}

// Shortcut runs fn on the chord, such as "cmd+s" or "escape"; see
// ParseChord for the names. A chord with a modifier runs before the
// focused widget and takes the key from it. A bare key reaches the focused
// widget first, and the shortcut runs only when the widget did not consume
// it (a text field consumes every key but Escape), unless the handle is
// made Exclusive. It panics on a chord ParseChord rejects.
//
//	app.Shortcut("cmd+s", save)
//	app.Shortcut("space", func() { ggui.Add(count, 1) })
func (w *Window) Shortcut(chord string, fn func()) *ShortcutHandle {
	return w.input.addShortcut(chord, fn)
}

// Semantics returns the accessibility tree as the last frame left it. The
// tree is finished and frozen, so it may be read from any goroutine and
// held for as long as it is useful; a changed frame publishes another rather
// than changing this one. Unchanged descriptions reuse the snapshot. A platform
// accessibility bridge answers queries from it, since the live frame belongs
// to the UI goroutine.
//
// A window builds the tree only while something reads it: an assistive
// technology, the inspector, a caller of Semantics, or a Perform waiting to
// be carried out. The first call to Semantics turns it on for good and asks
// for a frame; until that frame, the tree returned is the last one built,
// which is empty before any was.
func (w *Window) Semantics() *SemTree {
	w.wantSemantics()
	return w.semantics()
}

// wantSemantics makes every frame from the next on build the tree.
func (w *Window) wantSemantics() {
	if !w.semWanted.Swap(true) && w.wake != nil {
		w.wake()
	}
}

// OnDrop registers fn to receive files dropped onto the window that no
// drop zone under the cursor took; see PointerWidget.OnDrop for zones.
func (w *Window) OnDrop(fn func(DropEvent)) { w.input.drops = append(w.input.drops, fn) }

// SetDialogs replaces the file and message dialogs, for a host that draws
// its own. A nil p restores the platform's.
func (w *Window) SetDialogs(p runtime.Dialogs) *Window {
	if p == nil {
		p = w.nativeDialogs()
	}
	w.dialogs = p
	return w
}

// SetClipboard replaces the clipboard text fields cut, copy and paste
// with. A nil c restores the platform's.
func (w *Window) SetClipboard(c runtime.Clipboard) *Window {
	if c == nil {
		c = runtime.NativeClipboard()
	}
	w.clipboard = c
	return w
}

// Inspector turns the widget inspector on or off: an overlay that outlines
// the selected widget painted through Canvas.Paint and names the one under the
// cursor with its size and position. Config.Inspector binds it to a key.
// On, it takes the window's one overlay slot, replacing whatever SetOverlay
// installed; off, it leaves the slot empty.
//
// The panel is the one registered with RegisterInspector, which importing
// ggui/inspect/panel does, and that package ships only in builds tagged
// ggui_inspector. Without the import or the tag this call is a no-op, so a
// release binary carries none of it.
func (w *Window) Inspector(on bool) {
	panel := w.inspectorPanel()
	w.inspect = on && panel != nil
	if w.inspect {
		w.overlay = panel
		return
	}
	if panel != nil {
		panel.Reset()
		if w.overlay == panel {
			w.overlay = nil
		}
	}
}

// SetInspector docks the inspector's panel and chooses whether it outlines
// every widget; the panel's own toolbar changes the same settings.
func (w *Window) SetInspector(o InspectorOptions) {
	if panel := w.inspectorPanel(); panel != nil {
		panel.Apply(o)
	}
}

// runFrameEvent runs one frame: the OnFrame handlers and posted work, then
// the input gathered since the last frame, then the reactive settle, layout
// and paint. It asks for the next frame while something is still moving.
func (w *Window) runFrameEvent(ev ggwd.FrameEvent) error {
	if w.frameErr != nil {
		return w.frameErr
	}
	if w.closed {
		return nil
	}
	start := time.Now()
	defer func() { framestats.Frame(time.Since(start)) }()
	// The glyph and shaping caches expire after so many frames unused.
	text.AdvanceTick()
	// Frames run here, whichever goroutine the engine calls this on. A Probe
	// does not arm the check: it runs on its test's goroutine, and several
	// live in one process, so there is no single UI goroutine to compare
	// with and no frame racing the write either.
	reactive.MarkUIThread()
	// Handlers dispatched below reach this window through UseHost.
	setRunning(&w.frameLoop)
	restoreInput := textinput.WithWindow(ev.Window)
	defer restoreInput()
	w.runFrame()
	w.runPosted()
	f := w.takeInput()
	cf := w.canvas.fs()
	cf.pointer, cf.hasPointer = f.pos, true
	before := delivered.Load()
	w.dispatchInput(f)
	touched, placed := w.input.takeTouched()
	switch {
	case delivered.Load() != before:
		w.dirty.Or(uint32(dirtyInput))
	case !placed:
		w.dirty.Or(uint32(dirtyEvent))
	default:
		// What a pointer event reached is painted again, and what lies
		// beside it left as it was, unless a signal was written too.
		w.addRepaints(touched)
	}
	if w.closed {
		return nil
	}
	wd := w.world()
	if err := w.tick(wd.frame.begin(wd.frame.raw())); err != nil {
		return err
	}
	w.draw(ev.Screen, ev.Scale, ev.Kept, ev.SetDamage)
	if w.frameErr != nil {
		return w.frameErr
	}
	// The regions just painted decide the cursor, so one a widget changed
	// over this frame's input shows now rather than at the next event.
	if cursor := w.input.cursorOver(w.overlay, f.pos); cursor != w.cursor {
		w.cursor = cursor
		ev.Window.SetCursorShape(w.cursor)
	}
	if w.closed {
		return nil
	}
	w.seenGen = layoutGen()
	if w.wantsFrame(f) {
		ev.Window.RequestFrame()
	} else if wake := wd.frame.takeWake(); !wake.IsZero() {
		// A caret or a delayed tooltip changes at a known time; sleep
		// until then rather than painting every frame.
		if w.wakeTimer != nil {
			w.wakeTimer.Stop()
		}
		w.wakeTimer = time.AfterFunc(max(wake.Sub(FrameTime()), 0), w.wakeFrame)
	}
	return nil
}

// paintTarget returns the image this frame paints into, and whether it
// holds the last frame: screen, as its driver keeps it, or the backbuffer
// while a BackdropBlur has wanted one recently, unless it was made for
// this frame.
func (w *Window) paintTarget(screen *ggfx.Image, kept bool) (*ggfx.Image, bool) {
	if w.backdropTTL == 0 {
		w.freeBackbuf()
		return screen, kept
	}
	bounds := screen.Bounds()
	if w.backbuf == nil || w.backbuf.Bounds().Size() != bounds.Size() {
		w.freeBackbuf()
		// Unmanaged for the reason a layer is: see newLayerImage.
		w.backbuf = ggfx.NewImageWithOptions(image.Rectangle{Max: bounds.Size()}, &ggfx.NewImageOptions{Unmanaged: true})
		framestats.Kept(framestats.Backbuffer, w.backbuf.Bounds())
		return w.backbuf, false
	}
	return w.backbuf, true
}

// backdropFrames is how many frames the backbuffer outlives the last
// BackdropBlur, so one that blinks in and out does not reallocate it.
const backdropFrames = 120

// presentBackdrop copies a backbuffer frame to the screen and decides
// whether the next frame needs one, which a screen its driver can sample
// never does: a BackdropBlur reads it in place.
func (w *Window) presentBackdrop(screen, target *ggfx.Image) {
	if target != screen {
		screen.DrawImage(target, &ggfx.DrawImageOptions{Blend: ggfx.BlendCopy})
	}
	if w.canvas.fs().backdrop && !screen.CanBeSource() {
		w.backdropTTL = backdropFrames
		if target == screen {
			// This frame skipped its blurs; draw the next one with them.
			w.requestFrame()
		}
	} else if w.backdropTTL > 0 {
		w.backdropTTL--
	}
	// Kept for no blur, it goes, after this copy.
	if w.backdropTTL == 0 {
		w.freeBackbuf()
	}
}

// freeBackbuf frees the backbuffer, if there is one.
func (w *Window) freeBackbuf() {
	if w.backbuf != nil {
		framestats.Freed(framestats.Backbuffer, w.backbuf.Bounds())
		w.backbuf.Deallocate()
		w.backbuf = nil
	}
}

// catchUp settles the window's effects and layout at its current size
// without painting, for a change another window made.
func (w *Window) catchUp() error {
	if w.closed || w.root == nil {
		return nil
	}
	setRunning(&w.frameLoop)
	// A layout shapes text as a frame does, and the shaping caches expire
	// by ticks: a window covered for hours, its rows laid out again as
	// their text changes, would otherwise keep every string it shaped.
	text.AdvanceTick()
	if err := w.settle(Untrack(w.viewport.Get)); err != nil {
		return err
	}
	w.seenGen = layoutGen()
	return nil
}

// wantsFrame reports whether the next frame must run without waiting for
// input: an animation or a momentum scroll is moving, a widget read the
// clock while painting (a Motion, a caret blink), work is posted or runs
// every frame, a touch is down or files are being dragged over.
func (w *Window) wantsFrame(f frameInput) bool {
	wd := w.world()
	// A layout asked for since the tree was last laid out, by a widget as
	// it painted (a disclosure easing open calls Invalidate there) or by
	// the layout itself, is run in a frame of its own. Nothing else
	// records it: seenGen takes the request as seen, and the frame would be
	// skipped as one nothing changed for, leaving the widget where it stood
	// until the next input. The frame is painted whole unless every such
	// request was held within a scroll's viewport, which is then noted as
	// contained; see Invalidate.
	relayout := w.root != nil && layoutGen() != w.laidGen
	if relayout && (freeGen() != w.laidFree || len(w.frameLoop.contained) == 0) {
		w.dirty.Or(uint32(dirtyLayout))
	}
	switch {
	case relayout:
		framestats.For(framestats.Layout)
	case wd.anims.active():
		framestats.For(framestats.Animation)
		w.dirty.Or(uint32(dirtyAnim))
		wd.anims.noteDirtiers()
	case wd.frame.timeRead():
		// Only what read the time changes: the frame may be painted in
		// part. What else changes by then marks it dirty itself.
		framestats.For(framestats.Time)
	case len(w.frame) > 0:
		framestats.For(framestats.Callback)
		w.dirty.Or(uint32(dirtyCallback))
	case w.hasPosted():
		framestats.For(framestats.Posted)
	case f.drag || w.touch.active || w.touch.waiting || w.input.touchMotion.target != nil:
		framestats.For(framestats.Input)
		w.dirty.Or(uint32(dirtyInput))
	default:
		return false
	}
	return true
}

// dispatchInput lets an existing app drag finish before the overlay can
// take the pointer. Otherwise a release over the inspector's panel would
// leave a slider or text selection captured indefinitely.
func (w *Window) dispatchInput(f frameInput) {
	w.input.dispatchOver(w.overlay, f)
	if w.inspect && w.panel.Closed() {
		w.Inspector(false)
	}
}

// takeInput turns the events gathered since the last frame into this frame's
// input. Positions arrive in logical pixels already.
func (w *Window) takeInput() frameInput {
	f := w.pending
	w.pending = frameInput{}
	f.pos = w.pos
	// The union gathered from the frame's key presses, or the held keys
	// alone when there were none.
	f.mods = f.mods.Or(w.keyboard.Mods())
	ids := make([]ggwd.TouchID, 0, len(w.touches))
	for id := range w.touches {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	w.touch.apply(&f, ids, func(id ggwd.TouchID) Point { return w.touches[id] })
	f.drag, f.dragAt = w.dragOver, w.dragAt
	return f
}

// background is what the window clears to before painting.
func (w *Window) background() color.Color {
	if w.cfg.Background != nil {
		return w.cfg.Background
	}
	if w.cfg.Transparent {
		return color.Transparent
	}
	return rootEnv().Background()
}

// draw paints the tree to screen, which is in physical pixels at scale
// pixels per logical pixel, so that a HiDPI monitor gets a sharp image while
// widgets keep working in logical pixels.
func (w *Window) draw(screen *ggfx.Image, scale float64, kept bool, setDamage func([]image.Rectangle)) {
	dirtyBits := dirtyReason(w.dirty.Swap(0))
	if scale > 0 {
		if scale != w.canvas.scale {
			dirtyBits |= dirtyScale
			w.canvas.scale = scale
		}
	}
	asked := w.takeRepaints()
	if kept && dirtyBits == 0 && len(asked) == 0 && len(w.motion) == 0 && !w.motionLost && len(w.frameLoop.contained) == 0 &&
		reactive.StateGen() == w.motionGen && layoutGen() == w.seenGen {
		// Nothing changed: work was posted that wrote nothing, say. The
		// screen keeps the last frame, and untouched, nothing is presented.
		framestats.CountSkipped()
		return
	}
	target, kept := w.paintTarget(screen, kept)
	contained := w.takeContained()
	defer w.frameLoop.endFrameFree(len(contained) > 0, false)
	// within is where a frame whose changes a scroll contains paints, in
	// pixels; empty, the frame paints all of it.
	var within image.Rectangle
	if wanted := len(w.motion) > 0 || len(asked) > 0; dirtyBits != 0 {
		if wanted || len(contained) > 0 {
			refusePartialDirty(dirtyBits)
		}
	} else if len(contained) > 0 {
		within = w.containedDamage(target, kept, contained, asked)
	} else if wanted && w.drawPartial(screen, target, kept, asked, setDamage) {
		return
	}
	d := &w.drawing
	*d = windowDraw{w: w, screen: screen, target: target, dst: target, setDamage: setDamage}
	defer d.release()
	if !within.Empty() {
		d.dst = target.SubImage(within).(*ggfx.Image)
	}
	if within.Empty() {
		// All of it changes: the screen need not start from a copy of the last frame.
		d.damage(screen.Bounds())
	} else {
		d.damage(within)
	}
	if w.root == nil {
		d.dst.Fill(w.background())
		return
	}
	defer w.canvas.trimLayers()
	w.aim(d.dst)
	defer w.presentBackdrop(screen, target)
	logical := w.logicalSize(screen)
	if Untrack(w.viewport.Get) != logical {
		w.viewport.Set(logical)
	}
	d.tracing = w.inspect || len(w.sinks) > 0
	_, err := w.paint(paintPass{
		canvas: &w.canvas, in: &w.input, overlay: w.overlay, size: logical,
		tracing: d.tracing,
		semOff:  !w.semWanted.Load() && w.performs.Load() == 0 && !d.tracing && !w.ax.Listening(),
		hooks:   d,
	})
	if !d.filled {
		// The loop closed or lost its root as it settled: what is
		// presented is the background, not what the screen kept.
		d.dst.Fill(w.background())
	}
	if err != nil {
		w.frameErr = err
		return
	}
	w.noteMotion(target)
}

// aim points the window's Canvas at dst, which is the screen or the
// backbuffer, or a part of it.
func (w *Window) aim(dst *ggfx.Image) {
	w.canvas.Image, w.canvas.window = dst, true
}

// The methods here change the native window. Called before the window
// opens, they change the Config it opens with instead, where there is a
// field for it; the rest wait for nothing and do nothing. Call them from
// the UI thread.

// Title returns the window's title.
func (w *Window) Title() string { return w.cfg.Title }

// SetTitle changes the window's title.
func (w *Window) SetTitle(title string) {
	w.cfg.Title = title
	w.native(func(nw *ggwd.Window) { nw.SetTitle(title) })
}

// Size returns the size of the window's content, in logical pixels.
func (w *Window) Size() Size {
	if nw := w.window.Load(); nw != nil {
		return Sz(nw.Size())
	}
	return Sz(float64(w.cfg.Width), float64(w.cfg.Height))
}

// SetSize resizes the window's content to size, in logical pixels.
func (w *Window) SetSize(size Size) {
	w.cfg.Width, w.cfg.Height = int(size.W), int(size.H)
	w.native(func(nw *ggwd.Window) { nw.SetSize(float64(w.cfg.Width), float64(w.cfg.Height)) })
}

// Position returns where the window is on the screen, in logical pixels.
func (w *Window) Position() Point {
	if nw := w.window.Load(); nw != nil {
		return Pt(nw.Position())
	}
	if w.cfg.Position != nil {
		return *w.cfg.Position
	}
	return Point{}
}

// SetPosition moves the window to p on the screen, in logical pixels.
func (w *Window) SetPosition(p Point) {
	w.cfg.Position = &p
	w.native(func(nw *ggwd.Window) { nw.SetPosition(p.X, p.Y) })
}

// SetSizeLimits bounds the size the user can resize the window's content
// to, in logical pixels. A zero width or height means no limit.
func (w *Window) SetSizeLimits(minSize, maxSize Size) {
	w.cfg.MinWidth, w.cfg.MinHeight = int(minSize.W), int(minSize.H)
	w.cfg.MaxWidth, w.cfg.MaxHeight = int(maxSize.W), int(maxSize.H)
	w.native(func(nw *ggwd.Window) {
		nw.SetSizeLimits(float64(w.cfg.MinWidth), float64(w.cfg.MinHeight), float64(w.cfg.MaxWidth), float64(w.cfg.MaxHeight))
	})
}

// SetResizable sets whether the user can resize the window.
func (w *Window) SetResizable(resizable bool) {
	w.cfg.Resizable = resizable
	w.native(func(nw *ggwd.Window) { nw.SetResizable(resizable) })
}

// SetFrameless shows or hides the window's title bar and border.
func (w *Window) SetFrameless(frameless bool) {
	w.cfg.Frameless = frameless
	w.native(func(nw *ggwd.Window) { nw.SetDecorated(!frameless) })
}

// SetAlwaysOnTop sets whether the window stays above ordinary windows.
func (w *Window) SetAlwaysOnTop(on bool) {
	w.cfg.AlwaysOnTop = on
	w.native(func(nw *ggwd.Window) { nw.SetFloating(on) })
}

// SetIcon sets the window's icon, in several sizes; see Config.Icon.
func (w *Window) SetIcon(icon []image.Image) {
	w.cfg.Icon = icon
	w.native(func(nw *ggwd.Window) { nw.SetIcon(icon) })
}

// Minimize minimizes the window.
func (w *Window) Minimize() { w.native((*ggwd.Window).Minimize) }

// Maximize maximizes the window.
func (w *Window) Maximize() {
	w.cfg.Maximized = true
	w.native((*ggwd.Window).Maximize)
}

// Restore brings the window back from being minimized or maximized.
func (w *Window) Restore() {
	w.cfg.Maximized = false
	w.native((*ggwd.Window).Restore)
}

// SetFullscreen puts the window into fullscreen or takes it out.
func (w *Window) SetFullscreen(on bool) {
	w.native(func(nw *ggwd.Window) { nw.SetFullscreen(on) })
}

// Show shows a hidden window.
func (w *Window) Show() {
	w.cfg.Hidden = false
	w.native((*ggwd.Window).Show)
}

// Hide hides the window without closing it.
func (w *Window) Hide() {
	w.cfg.Hidden = true
	w.native((*ggwd.Window).Hide)
}

// Visible reports whether the window is shown.
func (w *Window) Visible() bool {
	if nw := w.window.Load(); nw != nil {
		return nw.IsVisible()
	}
	return !w.cfg.Hidden
}

// Focus brings the window to the front and gives it the keyboard focus.
func (w *Window) Focus() { w.native((*ggwd.Window).Focus) }

// RequestAttention asks for the user's attention, such as by bouncing the
// dock icon or flashing the taskbar button, without taking the focus.
func (w *Window) RequestAttention() { w.native((*ggwd.Window).RequestAttention) }

// Focused follows whether the window has the keyboard focus.
func (w *Window) Focused() Readable[bool] { return w.focused }

// Viewport follows the size of the window's content, in logical pixels.
func (w *Window) Viewport() Readable[Size] { return w.viewport }

// State follows whether the window is minimized, maximized or fullscreen.
func (w *Window) State() Readable[WindowState] { return w.state }

// native calls fn with the native window, when there is one. What fn
// changes comes back as events: a ResizeEvent for the size and a
// WindowStateEvent for the state, once the platform has animated it.
func (w *Window) native(fn func(*ggwd.Window)) {
	if nw := w.window.Load(); nw != nil {
		fn(nw)
	}
}

// windowState converts the engine's window state.
func windowState(s ggwd.WindowState) WindowState {
	switch s {
	case ggwd.WindowStateMinimized:
		return WindowMinimized
	case ggwd.WindowStateMaximized:
		return WindowMaximized
	case ggwd.WindowStateFullscreen:
		return WindowFullscreen
	}
	return WindowNormal
}

// Screen describes a display.
type Screen struct {
	Name string
	// Size is the display's size in logical pixels.
	Size Size
	// Scale is the display's physical pixels per logical pixel.
	Scale float64
}

func screenOf(m *ggwd.Monitor) Screen {
	width, height := m.Size()
	return Screen{Name: m.Name(), Size: Sz(float64(width), float64(height)), Scale: m.DeviceScaleFactor()}
}

// Screen returns the display the window is on, or the zero Screen before
// it opens.
func (w *Window) Screen() Screen {
	if nw := w.window.Load(); nw != nil {
		if m := nw.Monitor(); m != nil {
			return screenOf(m)
		}
	}
	return Screen{}
}

// Screens returns the displays attached to the machine, the primary one
// first. It needs a running App: before Run it returns none.
func Screens() []Screen {
	if !appRunning.Load() {
		return nil
	}
	var out []Screen
	for _, m := range ggwd.AppendMonitors(nil) {
		out = append(out, screenOf(m))
	}
	return out
}

func (w *Window) nativeDialogs() runtime.Dialogs {
	return runtime.NativeDialogsForWindow(func() uintptr {
		if nw := w.window.Load(); nw != nil {
			return native.Handle(nw)
		}
		return 0
	})
}

// windowDraw is what a Window's whole frame keeps while it paints, the
// paint's hooks among it: kept on the Window, a frame allocates none of it.
type windowDraw struct {
	w                   *Window
	screen, target, dst *ggfx.Image // dst is where the frame paints: target, or the part a scroll contains
	setDamage           func([]image.Rectangle)
	tracing             bool
	// filled is whether the background was filled: once the settle has
	// run, which may change the theme it comes from, or turn a frame a
	// scroll contains into a whole one.
	filled bool
}

// damage tells the host, when it asked, that r is what the frame changes.
func (d *windowDraw) damage(r image.Rectangle) {
	if d.setDamage != nil {
		d.w.damagePx = append(d.w.damagePx[:0], r)
		d.setDamage(d.w.damagePx)
	}
}

// release lets go of what the frame held. The damage last given stays in
// the Window's damagePx, which the host may read until the next frame.
func (d *windowDraw) release() { *d = windowDraw{} }

// settled implements paintHooks.
func (d *windowDraw) settled() {
	w := d.w
	if d.dst != d.target && (reactive.FreeGen() != w.freeSeen || fontGen() != w.fontSeen) {
		// The layout changed more than the scroll: paint all of it.
		framestats.Refuse("state changed")
		w.frameLoop.endFrameFree(true, true)
		d.dst = d.target
		w.aim(d.dst)
		d.damage(d.screen.Bounds())
	}
	d.dst.Fill(w.background())
	d.filled = true
}

// painted implements paintHooks.
func (d *windowDraw) painted() {
	w := d.w
	// Counted once the settle had its chance to paint it whole.
	if d.dst != d.target {
		framestats.CountContained()
	}
	w.ax.Publish(w.semantics(), w.takeAnnouncements())
	if d.tracing {
		w.publishInspect(&w.canvas, w.semantics())
	}
}
