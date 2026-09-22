package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

type controlsRender struct {
	directory string
	index     int
	done      bool
	err       error
}

func (g *controlsRender) Update() error {
	if g.err != nil {
		return g.err
	}
	if g.done {
		return ebiten.Termination
	}
	return nil
}
func (g *controlsRender) Layout(int, int) (int, int) { return 560, 520 }
func (g *controlsRender) Draw(screen *ebiten.Image) {
	if g.done || g.err != nil {
		return
	}
	n := len(controlNames)
	name := controlNames[g.index%n]
	dark := g.index/n%2 == 1
	width := 560
	if g.index/(2*n) == 1 {
		width = 320
	}
	preset := uitheme.Preset{Base: uitheme.BaseNeutral, Accent: uitheme.AccentBlue}
	theme, mode := preset.Light(), "light"
	if dark {
		theme, mode = preset.Dark(), "dark"
	}
	now := time.Unix(100, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	defer restore()
	var demo controlDemo
	p := ggui.ProbeBuilder(func() ggui.Widget {
		demo = buildControl(name)
		return uitheme.With(theme, demo.Widget)
	}, ggui.Sz(float64(width), 520))
	defer p.Close()
	p.Frame()
	env := theme.Apply(ggui.Env{}).WithText(theme.Text)
	size := demo.Widget.Layout(ggui.Loose(ggui.Sz(float64(width), 800)), env)
	img := ebiten.NewImage(width, int(size.H))
	defer img.Deallocate()
	for i, state := range []string{"initial", "active", "complete", "end"} {
		if i == 1 {
			if demo.Carousel != nil {
				demo.Carousel.Next()
			}
			if demo.OTP != nil {
				demo.OTP.Act(ggui.Action{Kind: ggui.ActionSetValue, Text: "12"})
				demo.OTP.HandleKey(ggui.KeyEvent{Kind: ggui.KeyFocus})
				demo.OTP.Input().Select(2, 2)
			}
		}
		if i == 2 && demo.OTP != nil {
			demo.OTP.Act(ggui.Action{Kind: ggui.ActionSetValue, Text: "123456"})
		}
		if i == 3 {
			if demo.Carousel != nil {
				demo.Carousel.ScrollTo(99)
			}
			if demo.OTP != nil {
				demo.OTP.Invalid(true)
				demo.OTP.HandleKey(ggui.KeyEvent{Kind: ggui.KeyBlur})
			}
		}
		delay := []time.Duration{0, 120 * time.Millisecond, 1800 * time.Millisecond, 3600 * time.Millisecond}[i]
		now = time.Unix(100, 0).Add(delay)
		ggui.SetClock(func() time.Time { return now })
		p.Frame()
		img.Fill(theme.Bg)
		demo.Widget.Paint(&ggui.Canvas{Image: img}, ggui.Rct(ggui.Point{}, size))
		name := filepath.Join(g.directory, fmt.Sprintf("%s-%s-%d-%s.png", name, mode, width, state))
		if err := savePNG(name, img); err != nil {
			g.err = err
			return
		}
	}
	screen.Fill(theme.Bg)
	screen.DrawImage(img, nil)
	g.index++
	g.done = g.index == n*4
}

// savePNG writes img to name, reporting either an encode or a close failure.
func savePNG(name string, img *ebiten.Image) (err error) {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	return png.Encode(f, img)
}

func renderControls(directory string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	ebiten.SetWindowTitle("ggui controls render validation")
	ebiten.SetWindowSize(560, 520)
	return ebiten.RunGame(&controlsRender{directory: directory})
}
