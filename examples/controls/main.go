// Command controls is a native Carousel and Input OTP comparison gallery.
package main

import (
	"flag"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/controldemo"
	"github.com/ironpark/ggui/ui"
	"log"
)

func main() {
	name := flag.String("example", "carousel-demo", "example name")
	darkFlag := flag.Bool("dark", false, "dark theme")
	renderDir := flag.String("render-dir", "", "render examples and states to PNG, then exit")
	flag.Parse()
	if *renderDir != "" {
		if err := renderControls(*renderDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	selected, dark := ggui.State(*name), ggui.State(*darkFlag)
	app := ggui.New(ggui.Config{Title: "ggui Carousel & Input OTP", Width: 680, Height: 700, Accessibility: ggui.AccessibilityAlways}, func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			preset := ggui.ThemePreset{Base: ggui.BaseNeutral, Accent: ggui.AccentBlue}
			t := preset.Light()
			if dark.Get() {
				t = preset.Dark()
			}
			return ggui.Themed(t, ggui.Box(ggui.Column(ggui.Row(ggui.Title("Carousel & Input OTP"), ggui.Spacer(), ui.Switch(dark, "Dark theme")).Gap(12), ui.Select(selected, controldemo.Names).Named("Component example"), ggui.Reactive(func() ggui.Widget { return controldemo.Build(selected.Get()).Widget }), ggui.Caption("Drag slides or use arrow keys · Paste a code or edit individual slots")).Gap(20).Align(ggui.AlignStretch)).Pad(24).Fill(t.Bg))
		})
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
