// Command charts is a focused, native catalog of the shadcn chart examples.
package main

import (
	"flag"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/chartdemo"
	"github.com/ironpark/ggui/ui"
	"log"
)

func main() {
	name := flag.String("chart", "chart-area-default", "example name")
	darkFlag := flag.Bool("dark", false, "dark theme")
	renderDir := flag.String("render-dir", "", "render all variants and animation samples to this directory, then exit")
	flag.Parse()
	if *renderDir != "" {
		if err := renderCharts(*renderDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	selected := ggui.State(*name)
	dark := ggui.State(*darkFlag)
	generation := ggui.State(0)
	app := ggui.New(ggui.Config{Title: "ggui Charts", Width: 740, Height: 600, Accessibility: ggui.AccessibilityAlways}, func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			theme := ggui.ThemePreset{Base: ggui.BaseNeutral, Accent: ggui.AccentBlue}.Light()
			if dark.Get() {
				theme = ggui.ThemePreset{Base: ggui.BaseNeutral, Accent: ggui.AccentBlue}.Dark()
			}
			return ggui.Themed(theme, ggui.Box(ggui.Column(
				ggui.Row(ggui.Title("Charts"), ggui.Spacer(), ui.Button("Replay", func() { ggui.Add(generation, 1) }), ui.Switch(dark, "Dark theme")).Gap(12),
				ui.Select(selected, chartdemo.Names()).Named("Chart example"),
				ggui.Reactive(func() ggui.Widget { generation.Get(); return chartdemo.Card(chartdemo.Find(selected.Get())) }),
				ggui.Caption("Arrow keys explore values · Enter selects · Replay restarts motion"),
			).Gap(20).Align(ggui.AlignStretch)).Pad(24).Fill(theme.Bg))
		})
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
