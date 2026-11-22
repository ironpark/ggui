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

See [docs/charts.md](../../docs/charts.md) and [docs/examples/charts](../../docs/examples/charts) for full usage and examples.
