// Package ggui is a cross-platform GUI framework for Go: Flutter's widget tree
// and layout model, driven by SvelteKit-style reactivity, rendered by Ebitengine.
package ggui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
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

// App drives one window: it flushes reactive effects, rebuilds the tree when
// state changed, then lays out and paints it.
type App struct {
	cfg   Config
	build Builder

	root  Widget
	frame []func()
	dirty bool
}

// New creates an App that renders the tree returned by build.
func New(cfg Config, build Builder) *App {
	return &App{cfg: cfg.withDefaults(), build: build, dirty: true}
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
	// marks the App dirty when it changes.
	Effect(func() {
		a.root = a.build()
		a.dirty = true
	})
	return ebiten.RunGame(a)
}

// OnFrame registers fn to run once per frame, before effects are flushed.
// It is the seam for input handling until dedicated event widgets exist.
func (a *App) OnFrame(fn func()) {
	a.frame = append(a.frame, fn)
}

// Update implements ebiten.Game.
func (a *App) Update() error {
	for _, fn := range a.frame {
		fn()
	}
	effects.flush()
	return nil
}

// Draw implements ebiten.Game.
func (a *App) Draw(screen *ebiten.Image) {
	screen.Fill(a.cfg.Background)
	if a.root == nil {
		return
	}
	b := screen.Bounds()
	size := a.root.Layout(Tight(Sz(b.Dx(), b.Dy())))
	a.root.Paint(screen, Rect{Size: size})
	a.dirty = false
}

// Layout implements ebiten.Game.
func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}
