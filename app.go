// Package ggui is a cross-platform GUI framework for Go: Flutter's widget tree
// and layout model, driven by SvelteKit-style reactivity, rendered by Ebitengine.
package ggui

import (
	"image/color"
	"math"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Config describes the window an App opens.
type Config struct {
	Title      string
	Width      int
	Height     int
	Resizable  bool
	Background color.Color // nil follows the theme's Bg
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
// last frame, flushes reactive effects (which rebuilds the tree when state
// changed), then lays out and paints, collecting the next frame's regions.
type App struct {
	cfg   Config
	build Builder

	root  Widget
	frame []func()

	canvas Canvas // also holds the frame's screen-pixels-per-logical-pixel scale
	input  inputState
	cursor ebiten.CursorShapeType
}

// New creates an App that renders the tree returned by build.
func New(cfg Config, build Builder) *App {
	return &App{cfg: cfg.withDefaults(), build: build}
}

// Run opens the window and blocks until it closes.
func Run(cfg Config, build Builder) error {
	return New(cfg, build).Run()
}

// Run opens the window and blocks until it closes.
func (a *App) Run() error {
	ebiten.SetWindowTitle(a.cfg.Title)
	ebiten.SetWindowSize(a.cfg.Width, a.cfg.Height)
	if a.cfg.Resizable {
		ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	}
	// The tree is rebuilt through an Effect, so every signal read during build
	// rebuilds the tree when it changes.
	Effect(func() { a.root = a.build() })
	appRunning.Store(true)
	// Holding a key on macOS pops up the accent menu, as it does in every
	// text field on the platform; text editing relies on it.
	return ebiten.RunGameWithOptions(a, &ebiten.RunGameOptions{ApplePressAndHoldEnabled: true})
}

// OnFrame registers fn to run once per frame, before input is dispatched
// and effects are flushed. Use it for global shortcuts and per-frame work
// that no widget owns; Pointer and Focus cover input aimed at a widget.
func (a *App) OnFrame(fn func()) {
	a.frame = append(a.frame, fn)
}

// Update implements ebiten.Game.
func (a *App) Update() error {
	for _, fn := range a.frame {
		fn()
	}
	a.input.dispatch(a.readInput())
	if a.input.cursor != a.cursor {
		a.cursor = a.input.cursor
		ebiten.SetCursorShape(a.cursor)
	}
	anims.step(time.Now())
	effects.flush()
	return nil
}

// appRunning reports whether RunGame has started, which is when platform
// services such as the IME may be used.
var appRunning atomic.Bool

var mouseButtons = []ebiten.MouseButton{ebiten.MouseButtonLeft, ebiten.MouseButtonRight, ebiten.MouseButtonMiddle}

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
	f.wheel = Pt(ebiten.Wheel())
	for _, k := range inpututil.AppendPressedKeys(nil) {
		if d := inpututil.KeyPressDuration(k); d == 1 || d > repeatDelay && (d-repeatDelay)%repeatInterval == 0 {
			f.keys = append(f.keys, k)
		}
	}
	f.text = string(ebiten.AppendInputChars(nil))
	f.mods = Mods{
		Shift: ebiten.IsKeyPressed(ebiten.KeyShift),
		Ctrl:  ebiten.IsKeyPressed(ebiten.KeyControl),
		Alt:   ebiten.IsKeyPressed(ebiten.KeyAlt),
		Meta:  ebiten.IsKeyPressed(ebiten.KeyMeta),
	}
	return f
}

// Draw implements ebiten.Game.
func (a *App) Draw(screen *ebiten.Image) {
	bg := a.cfg.Background
	if bg == nil {
		bg = theme.Peek().Bg
	}
	screen.Fill(bg)
	if a.root == nil {
		return
	}
	a.canvas.Image, a.canvas.hits = screen, a.canvas.hits[:0]
	b := screen.Bounds()
	logical := Sz(a.canvas.dp(float64(b.Dx())), a.canvas.dp(float64(b.Dy())))
	size := a.root.Layout(Tight(logical), rootEnv())
	a.root.Paint(&a.canvas, Rect{Size: size})
	a.input.regions = a.canvas.hits
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
