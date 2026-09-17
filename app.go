// Package ggui is a cross-platform GUI framework for Go: Flutter's widget tree
// and layout model, driven by SvelteKit-style reactivity, rendered by Ebitengine.
package ggui

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Config describes the window an App opens.
type Config struct {
	Title      string
	Width      int
	Height     int
	Resizable  bool
	Background color.Color
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
	if c.Background == nil {
		c.Background = color.White
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
	return ebiten.RunGame(a)
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
	effects.flush()
	return nil
}

var mouseButtons = []ebiten.MouseButton{ebiten.MouseButtonLeft, ebiten.MouseButtonRight, ebiten.MouseButtonMiddle}

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
	f.keys = inpututil.AppendJustPressedKeys(nil)
	f.text = string(ebiten.AppendInputChars(nil))
	return f
}

// Draw implements ebiten.Game.
func (a *App) Draw(screen *ebiten.Image) {
	screen.Fill(a.cfg.Background)
	if a.root == nil {
		return
	}
	a.canvas.Image, a.canvas.hits = screen, a.canvas.hits[:0]
	b := screen.Bounds()
	logical := Sz(a.canvas.dp(float64(b.Dx())), a.canvas.dp(float64(b.Dy())))
	size := a.root.Layout(Tight(logical))
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
