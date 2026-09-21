// Command charts is a native catalog of the shadcn chart examples: a Select
// bound to the example name, a Reactive island that rebuilds only the card,
// a Replay button that restarts the entry animation and a theme switch
// through BindTheme. The inspector opens on F1.
//
// The UI lives in build so main_test.go can drive it headlessly with a
// Probe.
package main

import (
	"flag"
	"log"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/chartdemo"
	"github.com/ironpark/ggui/ui"
)

// model holds one signal per piece of state. Bumping Generation changes the
// card's key, which remounts the chart and restarts its animation.
type model struct {
	Selected   *ggui.StateValue[string]
	Dark       *ggui.StateValue[bool]
	Generation *ggui.StateValue[int]
}

func newModel(name string, dark bool) model {
	return model{Selected: ggui.State(name), Dark: ggui.State(dark), Generation: ggui.State(0)}
}

func (m model) replay() { ggui.Add(m.Generation, 1) }

// cardKey is comparable, as Key requires.
type cardKey struct {
	name string
	gen  int
}

// build is the root Builder. It reads nothing reactive, so it runs once;
// the theme comes from the Env and the card is the island that follows the
// selection.
func (m model) build() ggui.Widget {
	return ggui.Box(ggui.Column(
		ggui.Row(
			ggui.Title("Charts"),
			ggui.Spacer(),
			ui.Button("Replay", m.replay),
			ui.Switch(m.Dark, "Dark theme"),
		).Gap(12),
		ui.Select(m.Selected, chartdemo.Names()).Named("Chart example"),
		// Key recreates the card whenever the selection or the replay
		// generation changes; the rest of the tree stays as it is.
		ggui.Key(ggui.Combine(m.Selected, m.Generation, func(name string, gen int) cardKey { return cardKey{name, gen} }), func(k cardKey) ggui.Widget {
			return chartdemo.Card(chartdemo.Find(k.name))
		}),
		ggui.Caption("Arrow keys explore values · Enter selects · Replay restarts motion"),
	).Gap(20).Align(ggui.AlignStretch)).Pad(24)
}

func main() {
	name := flag.String("chart", "chart-area-default", "example name")
	dark := flag.Bool("dark", false, "dark theme")
	renderDir := flag.String("render-dir", "", "render all variants and animation samples to this directory, then exit")
	flag.Parse()
	if *renderDir != "" {
		if err := renderCharts(*renderDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	m := newModel(*name, *dark)
	app := ggui.New(ggui.Config{
		Title:         "ggui · charts",
		Width:         740,
		Height:        600,
		Resizable:     true,
		Inspector:     "f1",
		Accessibility: ggui.AccessibilityAlways,
	}, m.build)

	// Setup runs under the app's root owner, so the theme binding is
	// disposed with the app. The app fills the window with the theme's
	// background, so the tree needs no Fill of its own.
	preset := ggui.ThemePreset{Base: ggui.BaseNeutral, Accent: ggui.AccentBlue}
	app.Setup(func() { ggui.BindTheme(m.Dark, preset.Dark(), preset.Light()) })
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
