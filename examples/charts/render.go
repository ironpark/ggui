package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ironpark/ggui/ui"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/chartdemo"
)

// renderGame exercises the real GPU renderer, independently of desktop capture.
// Each example is saved at entrance, 150ms, 750ms and completion in both themes.
type renderGame struct {
	directory string
	index     int
	err       error
	done      bool
}

func (g *renderGame) Update() error {
	if g.err != nil {
		return g.err
	}
	if g.done {
		return ebiten.Termination
	}
	return nil
}
func (g *renderGame) Layout(int, int) (int, int) { return 480, 420 }
func (g *renderGame) Draw(screen *ebiten.Image) {
	if g.done || g.err != nil {
		return
	}
	e := chartdemo.Examples[g.index%len(chartdemo.Examples)]
	dark := g.index >= len(chartdemo.Examples)
	preset := ggui.ThemePreset{Base: ggui.BaseNeutral, Accent: ggui.AccentBlue}
	theme := preset.Light()
	mode := "light"
	if dark {
		theme = preset.Dark()
		mode = "dark"
	}
	now := time.Unix(100, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	defer restore()
	// Probe mounts reactive examples under an owner and cleans up subscriptions.
	var w ggui.Widget
	p := ggui.ProbeBuilder(func() ggui.Widget { w = chartdemo.Card(e); return ggui.Themed(theme, w) }, ggui.Sz(480, 420))
	p.Frame()
	defer p.Close()
	env := ggui.Env{}.WithTheme(theme).WithText(theme.Text)
	size := w.Layout(ggui.Loose(ggui.Sz(480, 600)), env)
	img := ebiten.NewImage(480, int(size.H))
	defer img.Deallocate()
	for _, ms := range []int{0, 150, 750, 2000} {
		now = time.Unix(100, 0).Add(time.Duration(ms) * time.Millisecond)
		ggui.SetClock(func() time.Time { return now })
		img.Fill(theme.Bg)
		dst := &ggui.Canvas{Image: img}
		w.Paint(dst, ggui.Rct(ggui.Point{}, size))
		if strings.Contains(e.Name, "tooltip") {
			content := chartdemo.Decorate(ui.ChartTooltipContent(chartdemo.Build(e), 1))
			tipSize := content.Layout(ggui.Loose(ggui.Sz(400, 240)), env)
			content.Paint(dst, ggui.Rct(ggui.Pt(180., 180.), tipSize))
		}
		name := filepath.Join(g.directory, fmt.Sprintf("%s-%s-%04d.png", e.Name, mode, ms))
		if err := savePNG(name, img); err != nil {
			g.err = err
			return
		}
	}
	screen.Fill(theme.Bg)
	screen.DrawImage(img, nil)
	g.index++
	if g.index == 2*len(chartdemo.Examples) {
		g.done = true
	}
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

func renderCharts(directory string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	ebiten.SetWindowTitle("ggui chart render validation")
	ebiten.SetWindowSize(480, 420)
	return ebiten.RunGame(&renderGame{directory: directory})
}
