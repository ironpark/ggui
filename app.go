// Package ggui is a cross-platform GUI framework for Go: Flutter's widget tree
// and layout model, driven by Svelte-style reactivity, rendered by Ebitengine.
package ggui

import (
	"image/color"
	"slices"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/inspect"
	"github.com/ironpark/ggui/internal/platform/drag"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/runtime"
)

// Config describes the window an App opens.
type Config struct {
	Title      string
	Width      int
	Height     int
	Resizable  bool
	Background color.Color // nil follows the root environment background
	Inspector  string      // a chord that toggles the widget inspector, such as "f1"; empty for none. Import ggui/inspect/panel for the panel.

	// Accessibility says when the app talks to the platform's
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

// App drives one window. Each frame it routes input to the regions painted
// last frame, settles reactive bindings and layout, runs user effects, then
// paints and collects the next frame's regions. Root setup runs once.
type App struct {
	frameErr error
	frameLoop
	cfg Config

	canvas Canvas // also holds the frame's screen-pixels-per-logical-pixel scale
	spare  []hitRegion
	input  inputState
	touch  touchInput
	cursor CursorShape

	window    atomic.Pointer[ggfx.Window] // the app's window, from StartEvent on; read by Post from any goroutine
	pending   frameInput                  // input gathered from events since the last frame
	pos       Point                       // the last known pointer position, in logical pixels
	wakeTimer *time.Timer
	keys      [KeyMax + 1]bool       // keys held, for Mods
	touches   map[ggfx.TouchID]Point // touches in progress, in logical pixels
	ax        a11y.Bridge
	drags     bool // the platform's drag observer is installed

	inspect      bool
	inspectChord Chord          // parsed from cfg.Inspector; Key is zero for none
	panel        InspectorPanel // the registered inspector, once wanted
	sinks        []func(*inspect.Frame)
	overlay      Overlay // drawn over every frame; the inspector while it is on
}

// New creates an App that renders the tree returned by build.
func New(cfg Config, build Builder) *App {
	a := &App{cfg: cfg.withDefaults()}
	a.build = build
	a.dialogs = runtime.NativeFilePicker()
	if cfg.Inspector != "" {
		// A chord rather than a KeyboardKey, whose zero value is KeyA and
		// would have made "Inspector: ggui.KeyA" mean none.
		a.inspectChord = MustChord(cfg.Inspector)
	}
	return a
}

// Setup registers fn to run under the app's root owner before the first
// build, so derived values and watchers have an owner and are disposed by
// Close. Call it before Run.
//
//	app.Setup(func() {
//	    ggui.Watch(background, func(c color.Color) {
//	        ggui.SetEnv(ggui.Untrack(ggui.UseEnv).With(ggui.BackgroundKey, c))
//	    })
//	})
func (a *App) Setup(fn func()) *App {
	a.setup = append(a.setup, fn)
	return a
}

// Post queues fn to run on the UI thread before the next frame's input.
// It is the one way a goroutine may touch signals: do the work off the
// thread, then Post the Set. Work posted by a callback runs next frame.
func (a *App) Post(fn func()) {
	a.post(fn)
	a.requestFrame()
}

// Close disposes the root owner, and with it every effect, memo and
// component the app created, and ends Run at the next frame. Call it from
// the UI thread; from a goroutine, Post it.
func (a *App) Close() {
	if a.panel != nil {
		a.panel.Release()
	}
	a.close()
	reactive.UnmarkUIThread()
	// The loop notices at its next frame.
	a.requestFrame()
}

// Run opens the window and blocks until it closes, disposing owned resources
// on return, including when the engine returns an error.
func Run(cfg Config, build Builder) error {
	return New(cfg, build).Run()
}

// Run opens the window and blocks until it closes, disposing owned resources
// on return, including when the engine returns an error.
func (a *App) Run() error {
	defer func() {
		a.Close()
		appRunning.Store(false)
	}()
	a.start()
	a.ax.Start(a.Perform, a.cfg.Accessibility)
	appRunning.Store(true)
	// Holding a key on macOS pops up the accent menu, as it does in every
	// text field on the platform; text editing relies on it.
	return ggfx.Run(a, &ggfx.RunOptions{ApplePressAndHoldEnabled: true})
}

// HandleEvent implements ggfx.Handler. Input events are gathered until the
// next frame, which they request; a FrameEvent runs the frame.
func (a *App) HandleEvent(ev ggfx.Event) error {
	switch ev := ev.(type) {
	case ggfx.StartEvent:
		w, err := ggfx.NewWindow(&ggfx.WindowOptions{
			Title:     a.cfg.Title,
			Width:     a.cfg.Width,
			Height:    a.cfg.Height,
			Resizable: a.cfg.Resizable,
		})
		if err != nil {
			return err
		}
		a.window.Store(w)
	case ggfx.FrameEvent:
		return a.runFrameEvent(ev)
	case ggfx.KeyEvent:
		if int(ev.Key) < len(a.keys) {
			a.keys[ev.Key] = ev.Pressed
		}
		if ev.Pressed {
			a.pending.keys = append(a.pending.keys, ev.Key)
			if a.cfg.Inspector != "" && !ev.Repeat &&
				(KeyEvent{Kind: KeyPress, Key: ev.Key, Mods: a.mods()}).Is(a.inspectChord) {
				a.Inspector(!a.inspect)
			}
		}
		a.requestFrame()
	case ggfx.TextEvent:
		a.pending.text += ev.Text
		a.requestFrame()
	case ggfx.MouseMoveEvent:
		a.pos = Pt(ev.X, ev.Y)
		a.requestFrame()
	case ggfx.MouseButtonEvent:
		a.pos = Pt(ev.X, ev.Y)
		if ev.Pressed {
			a.pending.down = append(a.pending.down, ev.Button)
		} else {
			a.pending.up = append(a.pending.up, ev.Button)
		}
		a.requestFrame()
	case ggfx.ScrollEvent:
		a.pending.wheel = a.pending.wheel.Add(Pt(ev.X*wheelUnit, ev.Y*wheelUnit))
		a.requestFrame()
	case ggfx.TouchEvent:
		if a.touches == nil {
			a.touches = map[ggfx.TouchID]Point{}
		}
		if ev.Phase == ggfx.TouchPhaseEnded {
			delete(a.touches, ev.ID)
		} else {
			a.touches[ev.ID] = Pt(ev.X, ev.Y)
		}
		a.requestFrame()
	case ggfx.DropEvent:
		a.pending.drop = append(a.pending.drop, droppedFiles(ev.Files)...)
		a.requestFrame()
	case ggfx.FocusEvent:
		if !ev.Focused {
			// Releases are not reported while another window has the focus.
			a.keys = [KeyMax + 1]bool{}
		}
		a.requestFrame()
	case ggfx.ResizeEvent:
		a.requestFrame()
	}
	return nil
}

// requestFrame asks the window for a frame. It is safe from any goroutine
// and before the window exists, when the first frame comes on its own.
func (a *App) requestFrame() {
	if w := a.window.Load(); w != nil {
		w.RequestFrame()
	}
}

// mods reports the modifier keys currently held.
func (a *App) mods() Mods {
	return Mods{
		Shift: a.keys[KeyShift],
		Ctrl:  a.keys[KeyControl],
		Alt:   a.keys[KeyAlt],
		Meta:  a.keys[KeyMeta],
	}
}

// OnFrame registers fn to run once per frame, before input is dispatched
// and effects are flushed, for per-frame work that no widget owns. OnKey
// covers global shortcuts; Pointer and Focus cover input aimed at a widget.
func (a *App) OnFrame(fn func()) {
	a.frame = append(a.frame, fn)
}

// OnKey registers a global key handler: fn sees every key press, with its
// modifiers, before the focused widget does, and a press it returns true
// for goes no further. A held key repeats into it as it would into a
// widget. Shortcut is the simpler form for one chord; OnKey is for what a
// chord cannot say. Keep the keys it takes away from a text field in
// mind: returning true for Space while one is focused would eat the space
// bar.
func (a *App) OnKey(fn func(KeyEvent) bool) {
	a.input.shortcuts = append(a.input.shortcuts, fn)
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
func (a *App) Shortcut(chord string, fn func()) *ShortcutHandle {
	return a.input.addShortcut(chord, fn)
}

// Semantics returns the accessibility tree as the last frame left it. The
// tree is finished and frozen, so it may be read from any goroutine and
// held for as long as it is useful; a changed frame publishes another rather
// than changing this one. Unchanged descriptions reuse the snapshot. A platform
// accessibility bridge answers queries from it, since the live frame belongs
// to the UI goroutine.
func (a *App) Semantics() *SemTree { return a.semantics() }

// OnDrop registers fn to receive files dropped onto the window that no
// drop zone under the cursor took; see PointerWidget.OnDrop for zones.
func (a *App) OnDrop(fn func(DropEvent)) { a.input.drops = append(a.input.drops, fn) }

// SetDialogs replaces the file dialogs, for a host that draws its own. A
// nil p restores the platform's.
func (a *App) SetDialogs(p runtime.FilePicker) *App {
	if p == nil {
		p = runtime.NativeFilePicker()
	}
	a.dialogs = p
	return a
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
func (a *App) Inspector(on bool) {
	panel := a.inspectorPanel()
	a.inspect = on && panel != nil
	if a.inspect {
		a.overlay = panel
		return
	}
	if panel != nil {
		panel.Reset()
		if a.overlay == panel {
			a.overlay = nil
		}
	}
}

// SetInspector docks the inspector's panel and chooses whether it outlines
// every widget; the panel's own toolbar changes the same settings.
func (a *App) SetInspector(o InspectorOptions) {
	if panel := a.inspectorPanel(); panel != nil {
		panel.Apply(o)
	}
}

// runFrameEvent runs one frame: the OnFrame handlers and posted work, then
// the input gathered since the last frame, then the reactive settle, layout
// and paint. It asks for the next frame while something is still moving.
func (a *App) runFrameEvent(ev ggfx.FrameEvent) error {
	if a.frameErr != nil {
		return a.frameErr
	}
	if a.closed {
		return ggfx.Termination
	}
	// Frames run here, whichever goroutine the engine calls this on. A Probe
	// does not arm the check: it runs on its test's goroutine, and several
	// live in one process, so there is no single UI goroutine to compare
	// with and no frame racing the write either.
	reactive.MarkUIThread()
	a.runFrame()
	a.runPosted()
	f := a.takeInput()
	cf := a.canvas.fs()
	cf.pointer, cf.hasPointer = f.pos, true
	a.dispatchInput(f)
	cursor := a.input.cursorOver(a.overlay, f.pos)
	if cursor != a.cursor {
		a.cursor = cursor
		ev.Window.SetCursorShape(a.cursor)
	}
	if a.closed {
		return ggfx.Termination
	}
	if err := a.tick(frame.begin(clock())); err != nil {
		return err
	}
	a.draw(ev.Screen, ev.Scale)
	if a.frameErr != nil {
		return a.frameErr
	}
	if a.closed {
		return ggfx.Termination
	}
	if a.wantsFrame(f) {
		ev.Window.RequestFrame()
	} else if wake := frame.takeWake(); !wake.IsZero() {
		// A caret or a delayed tooltip changes at a known time; sleep
		// until then rather than painting every frame.
		if a.wakeTimer != nil {
			a.wakeTimer.Stop()
		}
		a.wakeTimer = time.AfterFunc(max(wake.Sub(FrameTime()), 0), a.requestFrame)
	}
	return nil
}

// wantsFrame reports whether the next frame must run without waiting for
// input: an animation or a momentum scroll is moving, a widget read the
// clock while painting (a Motion, a caret blink), work is posted or runs
// every frame, a touch is down, or files are being dragged over.
func (a *App) wantsFrame(f frameInput) bool {
	if anims.active() || frame.timeRead() || len(a.frame) > 0 || a.hasPosted() {
		return true
	}
	return f.drag || a.touch.active || a.touch.waiting || a.input.touchMotion.target != nil
}

// dispatchInput lets an existing app drag finish before the overlay can
// take the pointer. Otherwise a release over the inspector's panel would
// leave a slider or text selection captured indefinitely.
func (a *App) dispatchInput(f frameInput) {
	a.input.dispatchOver(a.overlay, f)
	if a.inspect && a.panel.Closed() {
		a.Inspector(false)
	}
}

// AccessibilityMode says when an App talks to the platform's accessibility
// API. It is defined by the bridge in the a11y package; these are the same
// values under the names ggui has always used for them.
type AccessibilityMode = a11y.Mode

// When the bridge is active.
const (
	// AccessibilityAuto turns the bridge on while an assistive technology
	// is attached, and off again when the last one leaves.
	AccessibilityAuto = a11y.Auto

	// AccessibilityAlways keeps the bridge active, which is what a tool
	// such as Accessibility Inspector or a screenshot run needs.
	AccessibilityAlways = a11y.Always

	// AccessibilityOff disables the native bridge entirely.
	AccessibilityOff = a11y.Off
)

// appRunning reports whether RunGame has started, which is when platform
// services such as the IME may be used.
var appRunning atomic.Bool

var mouseButtons = []MouseButton{MouseButtonLeft, MouseButtonRight, MouseButtonMiddle}

// takeInput turns the events gathered since the last frame into this frame's
// input. Positions arrive in logical pixels already.
func (a *App) takeInput() frameInput {
	f := a.pending
	a.pending = frameInput{}
	f.pos = a.pos
	f.mods = a.mods()
	ids := make([]ggfx.TouchID, 0, len(a.touches))
	for id := range a.touches {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	a.touch.apply(&f, ids, func(id ggfx.TouchID) Point { return a.touches[id] })
	if !a.drags {
		// The window's view exists once frames run, so the first frame
		// installs the observer; the hop to the main thread happens once.
		ggfx.RunOnMainThread(func() { a.drags = drag.Install() })
	}
	// The drag is in the view's points, which are logical pixels already.
	if x, y, over := drag.Position(); over {
		f.drag, f.dragAt = true, Pt(x, y)
	}
	return f
}

// draw paints the tree to screen, which is in physical pixels at scale
// pixels per logical pixel, so that a HiDPI monitor gets a sharp image while
// widgets keep working in logical pixels.
func (a *App) draw(screen *ggfx.Image, scale float64) {
	if scale > 0 {
		a.canvas.scale = scale
	}
	bg := a.cfg.Background
	if bg == nil {
		bg = rootEnv().Background()
	}
	screen.Fill(bg)
	if a.root == nil {
		return
	}
	// Last frame's regions stay readable while this frame paints, for
	// Adopter handoff, and input keeps routing to them until the paint is
	// done; the buffer freed two frames ago takes the new ones.
	a.canvas.Image = screen
	a.canvas.prev, a.canvas.hits = a.canvas.hits, a.spare[:0]
	b := screen.Bounds()
	logical := Sz(a.canvas.dp(float64(b.Dx())), a.canvas.dp(float64(b.Dy())))
	f := a.canvas.fs()
	clear(f.trace)
	clear(f.traceWidgets)
	f.tracing = a.inspect || len(a.sinks) > 0
	f.trace, f.traceWidgets = f.trace[:0], f.traceWidgets[:0]
	f.logical = logical
	f.focusBounds = Rect{}
	if a.input.focused != nil {
		f.focusBounds = a.input.focused.rect
	}
	a.canvas.nextFrame()
	a.canvas.resetSemantics()
	if err := a.settle(logical); err != nil {
		a.frameErr = err
		return
	}
	if a.closed || a.root == nil {
		return
	}
	if a.cfg.Background == nil {
		screen.Fill(rootEnv().Background())
	}
	a.canvas.Paint(a.root, Rect{Size: a.rootSize})
	a.canvas.paintOverlays()
	a.input.regions = a.canvas.hits
	a.input.observers = a.canvas.inputObservers
	a.input.applyFocusRequest(&a.canvas)
	a.publishSemantics(&a.canvas, a.input.focused)
	a.ax.Publish(a.semantics(), a.takeAnnouncements())
	if f.tracing {
		a.publishInspect(&a.canvas, a.semantics())
	}
	if a.overlay != nil {
		a.overlay.Paint(&a.canvas)
	}
	a.spare = a.canvas.prev
}
