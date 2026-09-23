package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/examples/internal/offscreen"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// renderGame exercises the real GPU renderer, independently of desktop capture.
// Each example is saved at entrance, 150ms, 750ms and completion in both themes.
type renderGame struct {
	directory string
	index     int
}

// step renders one example into g.directory.
func (g *renderGame) step() (done bool, err error) {
	e := chartExamples[g.index%len(chartExamples)]
	dark := g.index >= len(chartExamples)
	preset := uitheme.Preset{Base: uitheme.BaseNeutral, Accent: uitheme.AccentBlue}
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
	p := ggui.ProbeBuilder(func() ggui.Widget {
		w = chartCard(e)
		return uitheme.With(theme, w)
	}, ggui.Sz(480, 420))
	p.Frame()
	defer p.Close()
	env := theme.Apply(ggui.Env{}).WithText(theme.Text)
	size := w.Layout(ggui.Loose(ggui.Sz(480, 600)), env)
	img := ggfx.NewImage(480, int(size.H))
	defer img.Deallocate()
	for _, ms := range []int{0, 150, 750, 2000} {
		now = time.Unix(100, 0).Add(time.Duration(ms) * time.Millisecond)
		ggui.SetClock(func() time.Time { return now })
		img.Fill(theme.Bg)
		dst := &ggui.Canvas{Image: img}
		w.Paint(dst, ggui.Rct(ggui.Point{}, size))
		if strings.Contains(e.Name, "tooltip") {
			content := decorateChart(ui.ChartTooltipContent(buildChart(e), 1))
			tipSize := content.Layout(ggui.Loose(ggui.Sz(400, 240)), env)
			content.Paint(dst, ggui.Rct(ggui.Pt(180., 180.), tipSize))
		}
		name := filepath.Join(g.directory, fmt.Sprintf("%s-%s-%04d.png", e.Name, mode, ms))
		if err := offscreen.SavePNG(name, img); err != nil {
			return false, err
		}
	}
	g.index++
	return g.index == 2*len(chartExamples), nil
}

func renderCharts(directory string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	g := &renderGame{directory: directory}
	return offscreen.Run(g.step)
}
