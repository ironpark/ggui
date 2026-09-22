// Command controls is a native catalog of the shadcn Carousel and Input OTP
// examples: a Select bound to the example name, a keyed subtree that
// rebuilds only the demo, and a theme switch through BindTheme. The
// inspector opens on F1.
//
// The UI lives in build so main_test.go can drive it headlessly with a
// Probe.
package main

import (
	"flag"
	"log"

	"github.com/ironpark/ggui"
	_ "github.com/ironpark/ggui/inspect/panel"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// model holds one signal per piece of state.
type model struct {
	Selected *ggui.StateValue[string]
	Dark     *ggui.StateValue[bool]
}

func newModel(name string, dark bool) model {
	return model{Selected: ggui.State(name), Dark: ggui.State(dark)}
}

// build is the root Builder. It reads nothing reactive, so it runs once;
// the theme comes from the Env and the demo is the island that follows the
// selection.
func (m model) build() ggui.Widget {
	return ggui.Box(ggui.Column(
		ggui.Row(
			ui.Title("Carousel & Input OTP"),
			ggui.Spacer(),
			ui.Switch(m.Dark, "Dark theme"),
		).Gap(12),
		ui.Select(m.Selected).Options(controlNames).Name("Component example"),
		// Key recreates the demo when the selection changes; the Select and
		// the switch above stay as they are.
		ggui.Key(m.Selected, func(name string) ggui.Widget { return buildControl(name).Widget }),
		ui.Caption("Drag slides or use arrow keys · Paste a code or edit individual slots"),
	).Gap(20).Align(ggui.AlignStretch)).Pad(24)
}

func main() {
	name := flag.String("example", "carousel-demo", "example name")
	dark := flag.Bool("dark", false, "dark theme")
	renderDir := flag.String("render-dir", "", "render examples and states to PNG, then exit")
	flag.Parse()
	if *renderDir != "" {
		if err := renderControls(*renderDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	m := newModel(*name, *dark)
	app := ggui.New(ggui.Config{
		Title:         "ggui · carousel & input OTP",
		Width:         680,
		Height:        700,
		Resizable:     true,
		Inspector:     "f1",
		Accessibility: ggui.AccessibilityAlways,
	}, m.build)

	// Setup runs under the app's root owner, so the theme binding is
	// disposed with the app. The app fills the window with the theme's
	// background, so the tree needs no Fill of its own.
	preset := uitheme.Preset{Base: uitheme.BaseNeutral, Accent: uitheme.AccentBlue}
	app.Setup(func() { uitheme.Bind(m.Dark, preset.Dark(), preset.Light()) })
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
