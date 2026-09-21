// Package ggui is a cross-platform GUI framework for Go: Flutter's widget tree
// and layout model, driven by Svelte-style reactivity, rendered by Ebitengine.
package ggui

import (
	"image/color"
	"math"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/runtime"
)

// Config describes the window an App opens.
type Config struct {
	Title      string
	Width      int
	Height     int
	Resizable  bool
	Background color.Color // nil follows the theme's Bg
	Inspector  string      // a chord that toggles the widget inspector, such as "f1"; empty for none

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
	ax     a11y.Bridge

	inspect      bool
	inspectChord Chord // parsed from cfg.Inspector; Key is zero for none
	insp         inspector
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
// build, so the derived values, Watch and BindTheme calls a main function
// makes have an owner and are disposed by Close. Call it before Run.
//
//	app.Setup(func() { ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme()) })
func (a *App) Setup(fn func()) *App {
	a.setup = append(a.setup, fn)
	return a
}

// Post queues fn to run on the UI thread before the next frame's input.
// It is the one way a goroutine may touch signals: do the work off the
// thread, then Post the Set. Work posted by a callback runs next frame.
func (a *App) Post(fn func()) { a.post(fn) }

// Close disposes the root owner, and with it every effect, memo and
// component the app created, and ends Run at the next frame. Call it from
// the UI thread; from a goroutine, Post it.
func (a *App) Close() {
	a.insp.release()
	a.close()
	unmarkUIThread()
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
	ebiten.SetWindowTitle(a.cfg.Title)
	ebiten.SetWindowSize(a.cfg.Width, a.cfg.Height)
	if a.cfg.Resizable {
		ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	}
	a.start()
	a.ax.Start(a.Perform, a.cfg.Accessibility)
	appRunning.Store(true)
	// Holding a key on macOS pops up the accent menu, as it does in every
	// text field on the platform; text editing relies on it.
	return ebiten.RunGameWithOptions(a, &ebiten.RunGameOptions{ApplePressAndHoldEnabled: true})
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
//
// The inspector ships only in builds tagged ggui_inspector. Without the tag
// this call is a no-op, so a release binary carries none of it.
func (a *App) Inspector(on bool) {
	a.inspect = on && inspectorEnabled
	a.insp.closed = false
	if !on {
		a.insp.reset()
	}
}

// SetInspector docks the inspector's panel and chooses whether it outlines
// every widget; the panel's own toolbar changes the same settings.
func (a *App) SetInspector(o InspectorOptions) { a.insp.apply(o) }

// Update implements ebiten.Game.
func (a *App) Update() error {
	if a.frameErr != nil {
		return a.frameErr
	}
	if a.closed {
		return ebiten.Termination
	}
	// Frames run here, whichever goroutine Ebitengine calls this on. A Probe
	// does not arm the check: it runs on its test's goroutine, and several
	// live in one process, so there is no single UI goroutine to compare
	// with and no frame racing the write either.
	markUIThread()
	a.runFrame()
	a.runPosted()
	f := a.readInput()
	if a.cfg.Inspector != "" && inpututil.IsKeyJustPressed(a.inspectChord.Key) &&
		(KeyEvent{Kind: KeyPress, Key: a.inspectChord.Key, Mods: f.mods}).Is(a.inspectChord) {
		a.Inspector(!a.inspect)
	}
	cf := a.canvas.fs()
	cf.pointer, cf.hasPointer = f.pos, true
	a.dispatchInput(f)
	cursor := a.input.cursor
	if a.inspect && a.input.pressed == nil {
		if shape, ok := a.insp.cursor(f.pos); ok {
			cursor = shape
		}
	}
	if cursor != a.cursor {
		a.cursor = cursor
		ebiten.SetCursorShape(a.cursor)
	}
	if a.closed {
		return ebiten.Termination
	}
	return a.tick(frame.begin(clock()))
}

// dispatchInput lets an existing app drag finish before the inspector can
// take the pointer. Otherwise a release over its panel would leave a slider
// or text selection captured indefinitely.
func (a *App) dispatchInput(f frameInput) {
	if !a.inspect || a.input.pressed != nil || !a.insp.input(f) {
		a.input.dispatch(f)
	}
	if a.insp.closed {
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

// Key repeat, in frames: a held key delivers KeyPress again after
// repeatDelay and then every repeatInterval frames.
const (
	repeatDelay    = 30
	repeatInterval = 2
)

// readInput gathers this frame's input from the platform, with positions
// converted from screen pixels to logical pixels.
func (a *App) readInput() frameInput {
	var f frameInput
	x, y := ebiten.CursorPosition()
	f.pos = Pt(a.canvas.dp(float64(x)), a.canvas.dp(float64(y)))
	for _, b := range mouseButtons {
		if inpututil.IsMouseButtonJustPressed(b) {
			f.down = append(f.down, b)
		}
		if inpututil.IsMouseButtonJustReleased(b) {
			f.up = append(f.up, b)
		}
	}
	ids := ebiten.AppendTouchIDs(nil)
	a.touch.apply(&f, ids, func(id ebiten.TouchID) Point {
		x, y := ebiten.TouchPosition(id)
		return Pt(a.canvas.dp(float64(x)), a.canvas.dp(float64(y)))
	})
	wx, wy := ebiten.Wheel()
	f.wheel = Pt(wx*wheelUnit, wy*wheelUnit)
	for _, k := range inpututil.AppendPressedKeys(nil) {
		if d := inpututil.KeyPressDuration(k); d == 1 || d > repeatDelay && (d-repeatDelay)%repeatInterval == 0 {
			f.keys = append(f.keys, k)
		}
	}
	f.text = string(ebiten.AppendInputChars(nil))
	if fsys := ebiten.DroppedFiles(); fsys != nil {
		f.drop = droppedFiles(fsys)
	}
	f.mods = Mods{
		Shift: ebiten.IsKeyPressed(KeyShift),
		Ctrl:  ebiten.IsKeyPressed(KeyControl),
		Alt:   ebiten.IsKeyPressed(KeyAlt),
		Meta:  ebiten.IsKeyPressed(KeyMeta),
	}
	return f
}

// Draw implements ebiten.Game.
func (a *App) Draw(screen *ebiten.Image) {
	// Draw runs at the display's rate while Update runs at a fixed TPS, so
	// the clock moves on again here: motion eased inside Paint is then as
	// smooth as the screen allows, and frozen for the paint.
	frame.begin(clock())
	bg := a.cfg.Background
	if bg == nil {
		bg = Untrack(theme.Get).Bg
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
	f.tracing, f.trace = a.inspect, f.trace[:0]
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
		screen.Fill(Untrack(theme.Get).Bg)
	}
	a.canvas.Paint(a.root, Rect{Size: a.rootSize})
	a.canvas.paintOverlays()
	a.input.regions = a.canvas.hits
	a.input.observers = a.canvas.inputObservers
	a.input.applyFocusRequest(&a.canvas)
	a.publishSemantics(&a.canvas, a.input.focused)
	a.ax.Publish(a.semantics(), a.takeAnnouncements())
	if a.inspect {
		a.insp.paint(&a.canvas)
	} else {
		a.insp.hide() // nothing to intercept while it is off
	}
	a.spare = a.canvas.prev
}

// LayoutF implements ebiten.LayoutFer: the screen is sized in physical
// pixels so that a HiDPI monitor gets a sharp image, while widgets keep
// working in logical pixels.
func (a *App) LayoutF(outsideWidth, outsideHeight float64) (float64, float64) {
	if s := ebiten.Monitor().DeviceScaleFactor(); s > 0 {
		a.canvas.scale = s
	}
	return a.canvas.px(outsideWidth), a.canvas.px(outsideHeight)
}

// Layout implements ebiten.Game. Ebitengine calls LayoutF instead.
func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	w, h := a.LayoutF(float64(outsideWidth), float64(outsideHeight))
	return int(math.Ceil(w)), int(math.Ceil(h))
}
