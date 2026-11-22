# Charts

`ui/chart` provides native GPU-rendered charts for ggui: area, bar, line, pie,
radar, and radial. All chart kinds share data/series configuration, theme
tokens (`Theme.Chart`), tooltips, legends, keyboard navigation, accessibility
actions, and reduced-motion support.

```go
import (
	"github.com/ironpark/ggui/ui/chart"
)

config := chart.Config{
	{Key: "desktop", Label: "Desktop", ColorIndex: 1},
	{Key: "mobile", Label: "Mobile", ColorIndex: 2},
}
data := []chart.Datum{
	{Label: "January", Values: map[string]float64{"desktop": 186, "mobile": 80}},
	{Label: "February", Values: map[string]float64{"desktop": 305, "mobile": 200}},
}

bars := chart.Bar(data, config).
	Height(240).
	Stack(chart.StackStacked).
	Legend(true).
	Tooltip(chart.TooltipOptions{Indicator: chart.IndicatorLine})
```

## Chart kinds

`chart.Container(kind, data, config)` creates any kind; convenience constructors
set kind-appropriate defaults:

| Constructor | Kind | Defaults |
| --- | --- | --- |
| `chart.Area(data, config)` | `KindArea` | 1px stroke, natural curve, 0.4 fill opacity |
| `chart.Bar(data, config)` | `KindBar` | 400ms entrance, 4px corner radius |
| `chart.Line(data, config)` | `KindLine` | 2px stroke, natural curve |
| `chart.Pie(data, config)` | `KindPie` | category colors, 400ms entrance delay |
| `chart.Radar(data, config)` | `KindRadar` | 0.6 fill opacity, 5-ring polygon grid |
| `chart.Radial(data, config)` | `KindRadial` | 0.25 inner radius, category colors, background track |

Standalone legend and tooltip widgets reuse the chart's labels, colors, icons,
and formatters without sharing interactive or animation state:

- `chart.Legend(c)` / `chart.LegendContent(c)`
- `chart.Tooltip(c, index)` / `chart.TooltipContent(c, index)`

See [docs/charts.md](../../docs/charts.md) and [examples/charts](../../examples/charts) for full usage and examples.

## Migration

Charts moved from `github.com/ironpark/ggui/ui` to `github.com/ironpark/ggui/ui/chart`
with non-stuttering names; old symbols in `ui` are not aliases.

| Before (`ui`) | Now (`ui/chart`) |
| --- | --- |
| `ui.ChartWidget` | `chart.Widget` |
| `ui.ChartContainer` | `chart.Container` |
| `ui.AreaChart`, `ui.BarChart`, `ui.LineChart` | `chart.Area`, `chart.Bar`, `chart.Line` |
| `ui.PieChart`, `ui.RadarChart`, `ui.RadialChart` | `chart.Pie`, `chart.Radar`, `chart.Radial` |
| `ui.ChartLegend`, `ui.ChartLegendContent` | `chart.Legend`, `chart.LegendContent` |
| `ui.ChartTooltip`, `ui.ChartTooltipContent` | `chart.Tooltip`, `chart.TooltipContent` |
| `ui.ChartKind` (`ui.ChartArea` ... `ui.ChartRadial`) | `chart.Kind` (`chart.KindArea` ... `chart.KindRadial`) |
| `ui.ChartCurve` (`ui.ChartNatural` ... `ui.ChartMonotone`) | `chart.Curve` (`chart.CurveNatural` ... `chart.CurveMonotone`) |
| `ui.ChartStack` (`ui.ChartUnstacked`, `ui.ChartStacked`, `ui.ChartExpanded`) | `chart.Stack` (`chart.StackUnstacked`, `chart.StackStacked`, `chart.StackExpanded`) |
| `ui.ChartIndicator` (`ui.ChartIndicatorDot`, `ui.ChartIndicatorLine`, `ui.ChartIndicatorDashed`) | `chart.Indicator` (`chart.IndicatorDot`, `chart.IndicatorLine`, `chart.IndicatorDashed`) |
| `ui.ChartSeries`, `ui.ChartConfig`, `ui.ChartDatum` | `chart.Series`, `chart.Config`, `chart.Datum` |
| `ui.ChartTooltipOptions`, `ui.ChartTooltipPayload` | `chart.TooltipOptions`, `chart.TooltipPayload` |
| `ui.ChartPolarGrid`, `ui.ChartPointContext` | `chart.PolarGrid`, `chart.PointContext` |
