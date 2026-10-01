package ggui

import (
	"image"
	"image/color"
	"slices"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/inspect"
	"github.com/ironpark/ggui/internal/a11ybridge"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textinput"
	"github.com/ironpark/ggui/runtime"
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
func (c Config) options() *ggfx.WindowOptions {
	o := &ggfx.WindowOptions{
		Title:       c.Title,
		Width:       c.Width,
		Height:      c.Height,
		Resizable:   c.Resizable,
		Undecorated: c.Frameless,
		Floating:    c.AlwaysOnTop,
		Transparent: c.Transparent,
		Hidden:      c.Hidden,
		Maximized:   c.Maximized,
		Unfocused:   c.Unfocused,
		MinWidth:    c.MinWidth,
		MinHeight:   c.MinHeight,
		MaxWidth:    c.MaxWidth,
		MaxHeight:   c.MaxHeight,
	}
	if c.Position != nil {
		o.Position = &image.Point{X: int(c.Position.X), Y: int(c.Position.Y)}
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
	spare  []hitRegion
	input  inputState
	touch  touchInput
	cursor CursorShape

	window        atomic.Pointer[ggfx.Window] // the native window, once open; read by Post from any goroutine
	pending       frameInput                  // input gathered from events since the last frame
	pos           Point                       // the last known pointer position, in logical pixels
	wakeTimer     *time.Timer
	keys          [KeyMax + 1]bool       // keys held, for Mods
	touches       map[ggfx.TouchID]Point // touches in progress, in logical pixels
	ax            a11ybridge.Bridge
	dragOver      bool
	dragAt        Point
	seenGen       uint64            // layoutGen as this window's last frame left it
	menuShortcuts []*ShortcutHandle // the app menus' chords, where the menu bar does not run them
	settling      time.Time         // until when a change to the native window is followed; see native

	inspect      bool
	inspectChord Chord          // parsed from cfg.Inspector; Key is zero for none
	panel        InspectorPanel // the registered inspector, once wanted
	sinks        []func(*inspect.Frame)
	overlay      Overlay // drawn over every frame; the inspector while it is on

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
		w.requestFrame()
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
	nw, err := ggfx.NewWindow(w.cfg.options())
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
func (w *Window) handle(ev ggfx.Event) error {
	switch ev := ev.(type) {
	case ggfx.FrameEvent:
		return w.runFrameEvent(ev)
	case ggfx.KeyEvent:
		if int(ev.Key) < len(w.keys) {
			w.keys[ev.Key] = ev.Pressed
		}
		if ev.Pressed {
			// The modifiers of this press, from the held keys and from the
			// event itself, which sees a modifier the OS never reported as a
			// key. Presses within one frame share the union, so a chord is
			// not lost when its modifier was released before the frame ran.
			m := w.mods().or(modsFrom(ev.Modifiers))
			w.pending.mods = w.pending.mods.or(m)
			w.pending.keys = append(w.pending.keys, ev.Key)
			if w.cfg.Inspector != "" && !ev.Repeat &&
				(KeyEvent{Kind: KeyPress, Key: ev.Key, Mods: m}).Is(w.inspectChord) {
				w.Inspector(!w.inspect)
			}
		}
		w.requestFrame()
	case ggfx.CompositionEvent:
		textinput.HandleEvent(ev)
		w.requestFrame()
	case ggfx.TextEvent:
		if !textinput.HandleEvent(ev) {
			w.pending.text += ev.Text
		}
		w.requestFrame()
	case ggfx.MouseMoveEvent:
		w.pos = Pt(ev.X, ev.Y)
		w.requestFrame()
	case ggfx.MouseButtonEvent:
		w.pos = Pt(ev.X, ev.Y)
		if ev.Pressed {
			w.pending.down = append(w.pending.down, ev.Button)
		} else {
			w.pending.up = append(w.pending.up, ev.Button)
		}
		w.requestFrame()
	case ggfx.ScrollEvent:
		w.pending.wheel = w.pending.wheel.Add(Pt(ev.X*wheelUnit, ev.Y*wheelUnit))
		w.requestFrame()
	case ggfx.TouchEvent:
		if w.touches == nil {
			w.touches = map[ggfx.TouchID]Point{}
		}
		if ev.Phase == ggfx.TouchPhaseEnded {
			delete(w.touches, ev.ID)
		} else {
			w.touches[ev.ID] = Pt(ev.X, ev.Y)
		}
		w.requestFrame()
	case ggfx.DragEvent:
		switch ev.Phase {
		case ggfx.DragEntered, ggfx.DragMoved:
			w.dragOver, w.dragAt = true, Pt(ev.X, ev.Y)
		case ggfx.DragExited, ggfx.DragEnded:
			w.dragOver = false
		}
		w.requestFrame()
	case ggfx.DropEvent:
		w.pending.drop = append(w.pending.drop, droppedFiles(ev.Files)...)
		w.requestFrame()
	case ggfx.CloseEvent:
		if len(w.closeRequests) > 0 {
			// The handlers decide in a frame, where they may put up a
			// question; the window stays until they agree or Close is called.
			ev.KeepOpen()
			w.post(func() {
				if w.closeAllowed() {
					w.Close()
				}
			})
			break
		}
		// The window closes after this event. Dispose the tree and detach
		// the native hooks first, while the window still exists.
		w.release()
	case ggfx.FocusEvent:
		if !ev.Focused {
			// Releases are not reported while another window has the focus.
			w.keys = [KeyMax + 1]bool{}
		}
		w.post(func() { w.focused.Set(ev.Focused); w.refreshState() })
	case ggfx.ResizeEvent:
		// A resize comes with a frame at the new size, whose screen the
		// viewport follows; the state may have changed with it.
		w.post(w.refreshState)
	}
	return nil
}

// requestFrame asks the window for a frame. It is safe from any goroutine
// and before the window exists, when the first frame comes on its own.
func (w *Window) requestFrame() {
	if nw := w.window.Load(); nw != nil {
		nw.RequestFrame()
	}
}

// mods reports the modifier keys currently held.
func (w *Window) mods() Mods {
	return Mods{
		Shift: w.keys[KeyShift] || w.keys[KeyShiftLeft] || w.keys[KeyShiftRight],
		Ctrl:  w.keys[KeyControl] || w.keys[KeyControlLeft] || w.keys[KeyControlRight],
		Alt:   w.keys[KeyAlt] || w.keys[KeyAltLeft] || w.keys[KeyAltRight],
		Meta:  w.keys[KeyMeta] || w.keys[KeyMetaLeft] || w.keys[KeyMetaRight],
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
func (w *Window) Semantics() *SemTree { return w.semantics() }

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
func (w *Window) runFrameEvent(ev ggfx.FrameEvent) error {
	if w.frameErr != nil {
		return w.frameErr
	}
	if w.closed {
		return nil
	}
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
	w.dispatchInput(f)
	cursor := w.input.cursorOver(w.overlay, f.pos)
	if cursor != w.cursor {
		w.cursor = cursor
		ev.Window.SetCursorShape(w.cursor)
	}
	if w.closed {
		return nil
	}
	wd := w.world()
	if err := w.tick(wd.frame.begin(wd.frame.raw())); err != nil {
		return err
	}
	w.draw(ev.Screen, ev.Scale)
	if w.frameErr != nil {
		return w.frameErr
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
		w.wakeTimer = time.AfterFunc(max(wake.Sub(FrameTime()), 0), w.requestFrame)
	}
	return nil
}

// catchUp settles the window's effects and layout at its current size
// without painting, for a change another window made.
func (w *Window) catchUp() error {
	if w.closed || w.root == nil {
		return nil
	}
	setRunning(&w.frameLoop)
	if err := w.settle(Untrack(w.viewport.Get)); err != nil {
		return err
	}
	w.seenGen = layoutGen()
	return nil
}

// wantsFrame reports whether the next frame must run without waiting for
// input: an animation or a momentum scroll is moving, a widget read the
// clock while painting (a Motion, a caret blink), work is posted or runs
// every frame, a touch is down, files are being dragged over, or the
// native window was just changed.
func (w *Window) wantsFrame(f frameInput) bool {
	wd := w.world()
	if wd.anims.active() || wd.frame.timeRead() || len(w.frame) > 0 || w.hasPosted() {
		return true
	}
	if time.Now().Before(w.settling) {
		w.refreshState()
		return true
	}
	return f.drag || w.touch.active || w.touch.waiting || w.input.touchMotion.target != nil
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
	f.mods = f.mods.or(w.mods())
	ids := make([]ggfx.TouchID, 0, len(w.touches))
	for id := range w.touches {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	w.touch.apply(&f, ids, func(id ggfx.TouchID) Point { return w.touches[id] })
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
func (w *Window) draw(screen *ggfx.Image, scale float64) {
	if scale > 0 {
		w.canvas.scale = scale
	}
	screen.Fill(w.background())
	if w.root == nil {
		return
	}
	defer w.canvas.trimLayers()
	// Last frame's regions stay readable while this frame paints, for
	// Adopter handoff, and input keeps routing to them until the paint is
	// done; the buffer freed two frames ago takes the new ones.
	w.canvas.Image = screen
	w.canvas.prev, w.canvas.hits = w.canvas.hits, w.spare[:0]
	b := screen.Bounds()
	logical := Sz(w.canvas.dp(float64(b.Dx())), w.canvas.dp(float64(b.Dy())))
	f := w.canvas.fs()
	clear(f.trace)
	clear(f.traceWidgets)
	f.tracing = w.inspect || len(w.sinks) > 0
	f.trace, f.traceWidgets = f.trace[:0], f.traceWidgets[:0]
	f.logical = logical
	if Untrack(w.viewport.Get) != logical {
		w.viewport.Set(logical)
	}
	f.focusBounds = Rect{}
	if w.input.focused != nil {
		f.focusBounds = w.input.focused.rect
	}
	w.canvas.nextFrame()
	w.canvas.resetSemantics()
	if err := w.settle(logical); err != nil {
		w.frameErr = err
		return
	}
	if w.closed || w.root == nil {
		return
	}
	if w.cfg.Background == nil && !w.cfg.Transparent {
		screen.Fill(rootEnv().Background())
	}
	w.paintTree(&w.canvas)
	w.input.regions = w.canvas.hits
	w.input.observers = w.canvas.inputObservers
	w.input.painted = w.canvas.shortcuts
	w.input.applyFocusRequest(&w.canvas)
	w.publishSemantics(&w.canvas, w.input.focused)
	w.ax.Publish(w.semantics(), w.takeAnnouncements())
	if f.tracing {
		w.publishInspect(&w.canvas, w.semantics())
	}
	if w.overlay != nil {
		w.overlay.Paint(&w.canvas)
	}
	w.spare = w.canvas.prev
}
