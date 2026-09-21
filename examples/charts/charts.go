// This catalog contains the shadcn chart examples, separate from the public
// chart API. Data and example names follow shadcn/ui (MIT; see NOTICE).
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
	"github.com/ironpark/ggui/ui"
)

//go:embed fixtures.json
var fixtureJSON []byte

type chartExample struct {
	Stacked                                   bool
	Opacities                                 map[string]float64
	Name, Title, Description, Curve, Subtitle string
	Data                                      []map[string]any
	Keys                                      []string
	Colors                                    map[string]int
}

var chartExamples = load()

func load() []chartExample {
	var out []chartExample
	if err := json.Unmarshal(fixtureJSON, &out); err != nil {
		panic(err)
	}
	return out
}
func chartNames() []string {
	out := make([]string, len(chartExamples))
	for i, e := range chartExamples {
		out[i] = e.Name
	}
	return out
}
func findChart(name string) chartExample {
	for _, e := range chartExamples {
		if e.Name == name {
			return e
		}
	}
	return chartExamples[0]
}
func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
func short(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("Jan 2")
	}
	if len(s) > 3 {
		return title(s[:3])
	}
	return title(s)
}
func buildChart(e chartExample) *ui.ChartWidget {
	kind := ui.ChartArea
	switch strings.Split(e.Name, "-")[1] {
	case "bar", "tooltip":
		kind = ui.ChartBar
	case "line":
		kind = ui.ChartLine
	case "pie":
		kind = ui.ChartPie
	case "radar":
		kind = ui.ChartRadar
	case "radial":
		kind = ui.ChartRadial
	}
	data := nextData(e)
	config := make(ui.ChartConfig, len(e.Keys))
	for i, k := range e.Keys {
		config[i] = ui.ChartSeries{Key: k, Label: title(k), ColorIndex: e.Colors[k]}
		if opacity, ok := e.Opacities[k]; ok {
			config[i].FillOpacity = &opacity
		}
	}
	name := e.Name
	has := func(s string) bool { return strings.Contains(name, s) }
	if has("icons") {
		roles := []icons.Role{"chart-trending-down", "chart-trending-up"}
		if kind == ui.ChartRadar {
			roles = []icons.Role{"chart-arrow-down", "chart-arrow-up"}
		}
		if has("tooltip") {
			roles = []icons.Role{"chart-footprints", "chart-waves"}
		}
		for i := range config {
			config[i].Icon = roles[i%2]
		}
	}
	if has("pie-stacked") {
		config[0].OuterRadius = .6
		config[1].InnerRadius = .7
		config[1].OuterRadius = .9
	}
	if has("stacked-expand") {
		v := .1
		config[0].FillOpacity = &v
	}
	if has("bar-negative") {
		for i := range data {
			if data[i].Values[e.Keys[0]] < 0 {
				data[i].ColorIndex = 2
			} else {
				data[i].ColorIndex = 1
			}
		}
	}

	c := ui.ChartContainer(kind, data, config).Name(e.Title).Height(240).TickFormatter(short)
	tooltip := ui.ChartTooltipOptions{}
	if kind == ui.ChartArea {
		tooltip.Indicator = ui.ChartIndicatorLine
		c.TooltipCursor(false)
	}
	switch e.Curve {
	case "linear":
		c.Curve(ui.ChartLinear)
	case "step":
		c.Curve(ui.ChartStep)
	case "monotone":
		c.Curve(ui.ChartMonotone)
	}
	if e.Stacked || has("stacked") || has("tooltip") {
		c.Stack(ui.ChartStacked)
	}
	if has("expand") {
		c.Stack(ui.ChartExpanded)
	}
	if has("gradient") || has("area-interactive") {
		c.Gradient(true)
	}
	if has("legend") || has("icons") && (kind == ui.ChartArea || kind == ui.ChartRadar) || has("bar-stacked") || has("area-interactive") {
		c.Legend(true)
	}
	if has("axes") {
		c.Axes(true, true)
	}
	if has("horizontal") || has("bar-mixed") || has("bar-label-custom") {
		c.Horizontal(true).Grid(false)
	}
	if has("mixed") || has("bar-active") || has("dots-colors") {
		c.CategoryColors(true)
	}
	if has("active") {
		c.ActiveIndex(2)
	}
	if has("dots") || has("line-label") {
		c.Dots(true)
	}
	if has("label") && !has("tooltip") && !has("radar-label") {
		c.Labels(true)
	}
	if has("line-label-custom") || has("pie-label-list") {
		c.LabelFormatter(func(ctx ui.ChartPointContext) string { return title(ctx.Datum.Label) })
	}
	if has("dots-custom") {
		c.DotPainter(func(dst *ggui.Canvas, ctx ui.ChartPointContext) {
			chartIcons["chart-commit"].Draw(dst, ggui.Rct(ctx.Position.Add(ggui.Pt(-10., -10.)), ggui.Sz(20., 20.)), ctx.Color, 0)
		})
	}

	if kind == ui.ChartPie {
		c.CategoryColors(true).Height(250)
		tooltip.HideLabel = true
		if has("donut") || has("interactive") {
			c.InnerRadius(.6)
		}
		if has("separator-none") {
			c.Separators(false)
		}
		if has("donut-text") {
			total := 0.
			for _, d := range data {
				total += d.Values[e.Keys[0]]
			}
			c.CenterText(comma(total), "Visitors")
		}
		if has("interactive") {
			c.ActiveIndex(0).ActiveRing(true).CenterText(fmt.Sprintf("%g", data[0].Values[e.Keys[0]]), "Visitors")
		}
	}
	if kind == ui.ChartRadar {
		c.Height(250).StrokeWidth(0).TickFormatter(func(s string) string { return s })
		grid := ui.ChartPolarGrid{Circle: has("circle"), Fill: has("fill"), HideSpokes: has("no-lines") || has("grid-custom")}
		if has("grid-custom") {
			grid.Rings = 1
		}
		c.PolarGrid(grid)
		if has("grid-none") {
			c.Grid(false)
		}
		if has("lines-only") {
			c.FillOpacity(0).StrokeWidth(2)
		}
		if has("radius") {
			c.Axes(false, true)
		}
		if has("dots") || has("circle") || has("grid-none") {
			c.Dots(true)
		}
	}
	if has("bar-default") || has("bar-active") || has("bar-label") {
		c.CornerRadius(8)
	}
	if has("horizontal") || has("mixed") {
		c.CornerRadius(5)
	}
	if has("pie-label-list") {
		c.InsideLabels(true)
	}
	if has("bar-negative") {
		c.Grid(false).Axes(false, false).LabelFormatter(func(ctx ui.ChartPointContext) string { return short(ctx.Datum.Label) })
	}
	if has("bar-label-custom") {
		c.Axes(false, false).Grid(true).LabelPainter(func(dst *ggui.Canvas, ctx ui.ChartPointContext) {
			demoText(dst, ctx.Env, short(ctx.Datum.Label), ggui.Pt(ctx.Plot.Origin.X+8, ctx.Position.Y-7), ctx.Env.Theme().PrimaryFg, 0)
			demoText(dst, ctx.Env, fmt.Sprintf("%g", ctx.Value), ctx.Position.Add(ggui.Pt(8., -7.)), ctx.Env.Theme().Fg, 0)
		})
	}
	if has("radar-label-custom") {
		c.TickPainter(func(dst *ggui.Canvas, ctx ui.ChartPointContext) {
			env := ctx.Env
			w := ggui.Column(ggui.Text(fmt.Sprintf("%g / %g", ctx.Datum.Values["desktop"], ctx.Datum.Values["mobile"])).Size(12).Color(ctx.Color), ggui.Caption(ctx.Datum.Label).Size(11)).Align(ggui.AlignCenter)
			size := w.Layout(ggui.Loose(ggui.Sz(120, 50)), env)
			dst.Paint(w, ggui.Rct(ctx.Position.Add(ggui.Pt(-size.W/2, -size.H/2)), size))
		})
	}
	if kind == ui.ChartRadial {
		c.Height(250).OuterRadius(.88).CategoryColors(true).InnerRadius(.27).CornerRadius(0).Grid(false)
		tooltip.HideLabel = true
		if has("grid") {
			c.Grid(true).OuterRadius(.8)
		}
		if has("label") {
			c.Angles(-90, 380)
		}
		if has("text") || has("shape") {
			c.OuterRadius(.72).InnerRadius(.8889).Angles(0, 250).CornerRadius(10).CenterText("1,260", "Visitors")
		}
		if has("shape") {
			c.OuterRadius(.76).InnerRadius(.6842).Angles(0, 100).CornerRadius(0)
		}
		if has("stacked") {
			c.RadialTrack(false).InnerRadius(.7273).Angles(0, 180).CornerRadius(5).CategoryColors(false).CenterText("1,830", "Visitors")
		}
	}
	if has("tooltip") {
		c.DefaultTooltipIndex(1)
		c.Grid(false).TooltipCursor(false).TickFormatter(func(s string) string {
			d, err := time.Parse("2006-01-02", s)
			if err == nil {
				return d.Format("Mon")
			}
			return s
		})
		switch {
		case has("indicator-line"):
			tooltip.Indicator = ui.ChartIndicatorLine
		case has("indicator-none"):
			tooltip.HideIndicator = true
		case has("label-none"):
			tooltip.HideLabel = true
			tooltip.HideIndicator = true
		case has("label-custom"):
			tooltip.Label = "Activities"
			tooltip.Indicator = ui.ChartIndicatorLine
		case has("label-formatter"):
			tooltip.FormatLabel = func(s string) string {
				d, err := time.Parse("2006-01-02", s)
				if err == nil {
					return d.Format("January 2, 2006")
				}
				return s
			}
		case has("formatter") || has("advanced"):
			tooltip.HideLabel = true
			tooltip.FormatValue = func(v float64, _ ui.ChartSeries, _ ui.ChartDatum) string { return fmt.Sprintf("%g kcal", v) }
		}
	}
	if has("tooltip-icons") {
		tooltip.HideLabel = true
	}
	if has("advanced") {
		tooltip.ShowTotal = true
	}
	return c.Tooltip(tooltip)
}

// chartCard builds a complete example with interactive controls where the reference
// exposes a date range, series selector or persistent active category.
func chartCard(e chartExample) ggui.Widget {
	chart := buildChart(e)
	description := "January - June 2024"
	footer := "Showing total visitors for the last 6 months"
	if strings.Contains(e.Name, "area") || strings.Contains(e.Name, "radar") {
		description = "Showing total visitors for the last 6 months"
		footer = "January - June 2024"
	}
	if strings.Contains(e.Name, "tooltip") {
		description = e.Description
	}
	if e.Subtitle != "" {
		description = e.Subtitle
	}
	widgets := []ggui.Widget{ggui.Title(e.Title).Size(16), ggui.Caption(description)}
	if strings.Contains(e.Name, "interactive") {
		if strings.Contains(e.Name, "area") {
			days := ggui.State("Last 3 months")
			widgets = append(widgets, ui.Select(days).Options([]string{"Last 3 months", "Last 30 days", "Last 7 days"}).OnChange(func(v string) {
				copy := e
				if v == "Last 30 days" {
					copy.Data = e.Data[max(0, len(e.Data)-30):]
				} else if v == "Last 7 days" {
					copy.Data = e.Data[max(0, len(e.Data)-7):]
				}
				chart.Data(nextData(copy))
			}))
		} else if strings.Contains(e.Name, "pie") {
			names := make([]string, len(e.Data))
			for i, d := range nextData(e) {
				names[i] = d.Label
			}
			selected := ggui.State(names[0])
			widgets = append(widgets, ui.Select(selected).Options(names).OnChange(func(s string) {
				for i, name := range names {
					if s == name {
						chart.ActiveIndex(i)
						chart.CenterText(fmt.Sprintf("%g", nextData(e)[i].Values[e.Keys[0]]), "Visitors")
					}
				}
			}))
		} else {
			desktop := e
			desktop.Keys = []string{"desktop"}
			mobile := e
			mobile.Keys = []string{"mobile"}
			selection := ggui.State("desktop")
			widgets = append(widgets, ggui.Row(ui.Button("Desktop", func() { selection.Set("desktop") }), ui.Button("Mobile", func() { selection.Set("mobile") })).Gap(8))
			widgets = append(widgets, ggui.View(selection, func(s string) *ui.ChartWidget {
				if s == "mobile" {
					return buildChart(mobile)
				}
				return buildChart(desktop)
			}))
			chart = nil
		}
	}
	if chart != nil {
		widgets = append(widgets, chart)
	}
	if !strings.Contains(e.Name, "tooltip") {
		widgets = append(widgets, ggui.Text("Trending up by 5.2% this month ↗").Size(14), ggui.Caption(footer))
	}
	if strings.Contains(e.Name, "pie") || strings.Contains(e.Name, "radial") || strings.Contains(e.Name, "radar") {
		for i, w := range widgets {
			if text, ok := w.(*ggui.TextWidget); ok {
				widgets[i] = text.Align(.5)
			}
		}
	}
	column := ggui.Column(widgets...).Gap(12).Align(ggui.AlignStretch)
	return decorateChart(ui.Card(column).Pad(24))
}
func nextData(e chartExample) []ui.ChartDatum {
	data := make([]ui.ChartDatum, len(e.Data))
	for i, row := range e.Data {
		data[i] = ui.ChartDatum{Values: map[string]float64{}}
		for k, v := range row {
			switch v := v.(type) {
			case float64:
				data[i].Values[k] = v
			case string:
				if k != "fill" {
					data[i].Label = title(v)
				} else {
					key := strings.TrimSuffix(strings.TrimPrefix(v, "var(--color-"), ")")
					data[i].ColorIndex = e.Colors[key]
				}
			}
		}
	}
	return data
}

func comma(v float64) string {
	s := fmt.Sprintf("%.0f", v)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func demoText(dst *ggui.Canvas, env ggui.Env, s string, p ggui.Point, col color.Color, align float64) {
	w := ggui.Text(s).Size(12).Color(col)
	size := w.Layout(ggui.Loose(ggui.Sz(200, 30)), env)
	p.X -= size.W * align
	dst.Paint(w, ggui.Rct(p, size))
}

// decorateChart supplies the exact Lucide icons used by the reference examples.
func decorateChart(w ggui.Widget) ggui.Widget {
	return ggui.Provide(icons.SetKey, icons.Set(chartIcons), w)
}
