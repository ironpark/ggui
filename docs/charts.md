# Charts

[Documentation](README.md) · [Project README](../README.md)

Create native charts, configure their appearance, and support interactive data exploration.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

`ui` provides native area, bar, line, pie/donut, radar and radial charts. They
render through ggui's Canvas, inherit `Theme.Chart`, support pointer and keyboard
exploration, and respect `ReducedMotionKey`. No WebView, JavaScript runtime or
additional dependency is required.

The design and example datasets follow the [shadcn chart component](https://ui.shadcn.com/docs/components/base/chart)
and [chart gallery](https://ui.shadcn.com/charts/area#charts). The Go API expresses
the same chart options directly; it is not a Recharts API compatibility layer.
Typography and control styling use the application's ggui theme.

## On this page

- [A first chart](#a-first-chart)
- [Constructors and options](#constructors-and-options)
- [Tooltip and legend components](#tooltip-and-legend-components)
- [Interaction, updates and motion](#interaction-updates-and-motion)
- [Efficiency](#efficiency)
- [Complete reference catalog and visual validation](#complete-reference-catalog-and-visual-validation)

## A first chart

```go
config := ui.ChartConfig{
    {Key: "desktop", Label: "Desktop", ColorIndex: 1},
    {Key: "mobile", Label: "Mobile", ColorIndex: 2},
}
data := []ui.ChartDatum{
    {Label: "January", Values: map[string]float64{"desktop": 186, "mobile": 80}},
    {Label: "February", Values: map[string]float64{"desktop": 305, "mobile": 200}},
    {Label: "March", Values: map[string]float64{"desktop": 237, "mobile": 120}},
}
chart := ui.BarChart(data, config).
    Named("Monthly visitors").
    Height(240).
    Legend(true).
    TickFormatter(func(s string) string { return s[:3] }).
    Tooltip(ui.ChartTooltipOptions{Indicator: ui.ChartIndicatorLine})

panel := ui.Card(ggui.Column(
    ggui.Title("Visitors"),
    ggui.Caption("January – March"),
    chart,
).Gap(12).Align(ggui.AlignStretch))
```

`ChartConfig` is ordered: the order determines series, stacking and legend order.
Keys map to numeric values; labels, colors and icons are independent of the data.
Missing, NaN and infinite values create gaps in Cartesian charts. An explicit zero
remains a real observation. Pie/radial sectors omit non-positive values. Empty
charts show “No data”. Radar charts require at least three categories.

## Constructors and options

| Constructor | Options |
| --- | --- |
| `AreaChart` | Natural, linear, midpoint-step and monotone curves; multiple series; stacking; normalized stacks; gradients; axes; legends |
| `BarChart` | Vertical or horizontal; grouped or stacked; signed values; rounded ends; category colors; labels; persistent selection |
| `LineChart` | All four curves; multiple series; dots; category-colored dots; custom point painters and labels |
| `PieChart` | Pie, donut, concentric series, separators, inside/outside labels, center text, active sectors and active rings |
| `RadarChart` | Multiple series; polygon/circular grids; filled grids; spokes/rings toggles; dots; radius labels; custom category labels |
| `RadialChart` | Concentric bars, stacked sectors, start/end angles, background tracks, rounded corners, arc labels and center text |

`ChartContainer(kind, data, config)` is the common constructor. Charts fill a
bounded width and default to 240 pixels tall (400 pixels wide when unbounded).
`Height` sets a logical height. Use `ggui.Box` and the normal layout widgets to
constrain size or create an aspect ratio.

### Configure the plot

- `Curve(ChartNatural | ChartLinear | ChartStep | ChartMonotone)` selects interpolation.
- `Stack(ChartUnstacked | ChartStacked | ChartExpanded)` controls accumulation.
  Positive and negative stacks have separate baselines. Expanded stacks divide
  by the sum of absolute values in each category.
- `Grid`, `Axes`, `Domain`, `TickFormatter`, `Legend` configure chart furniture.
  `Horizontal(true)` is for bars. `Axes`' first flag controls category labels;
  the second enables numeric labels (including the radar radius axis).
- `Dots`, `Labels`, `LabelFormatter`, `DotPainter`, `LabelPainter` customize data
  marks. Painter contexts include the datum, series, value, position, plot rect,
  theme environment, resolved color and active state.
- `Gradient(bool)` enables or disables area gradients; `StrokeWidth(px)` sets
  the line width and `FillOpacity(fraction)` sets overall fill opacity.
- `InnerRadius` and `OuterRadius` are fractions. `InnerRadius` is relative to the
  chart's outer radius; `OuterRadius` is relative to half the smaller chart
  dimension. Per-series radii support differently sized concentric pies.
- `Angles` uses degrees counter-clockwise from three o'clock, with a maximum
  full revolution in either direction. `CenterText`, `Separators`, `InsideLabels`,
  `ActiveRing`, `RadialTrack` and `CornerRadius` customize polar charts.
- `PolarGrid(ChartPolarGrid{...})` selects circular/polygonal grids, fill, ring
  count and spoke visibility. `TickPainter` customizes radar category labels.

### Choose series and category colors

`ChartSeries.Color` overrides its palette token. `ColorIndex` selects one of the
five theme tokens (1-based); zero uses series order. `ThemeColor` can resolve a
custom color from the current theme. A datum can also override its color or token.
`CategoryColors(true)` uses successive tokens for categories. Optional series
`FillOpacity` overrides the chart's opacity. `Icon` uses ggui's normal icon-set
lookup in both tooltips and legends.

## Tooltip and legend components

The built-in tooltip is configured with `Tooltip(ChartTooltipOptions{...})`:

- Dot, line or dashed indicators; `HideLabel`, `HideIndicator`, `Disabled`.
- `Label`, `FormatLabel`, `FormatName`, `FormatValue` for typed label/name/value
  mapping instead of JavaScript's `labelKey`/`nameKey` lookups.
- `ShowTotal` and `TotalLabel` add a separated summary row.
- `Content` returns any ggui widget for a completely custom popup body.

Callbacks must treat supplied data as read-only. Tooltip placement stays within
the viewport, and overlays paint above surrounding content. `TooltipCursor(false)`
hides the category band/crosshair. `DefaultTooltipIndex` supplies initial tooltip
state without selecting a shape.

`ChartTooltip(chart, index)` / `ChartTooltipContent(chart, index)` render the built-in
content as a standalone widget. `ChartLegend(chart)` / `ChartLegendContent(chart)`
render a standalone legend. They have independent layout/theme contexts, so they
can be placed outside the chart. Built-in legends wrap at narrow widths.

## Interaction, updates and motion

### Explore and select data

Charts are focusable. Left/right and up/down explore categories, Home/End jump
to the ends, Enter selects, and Escape dismisses the transient tooltip. The
selected category's values are exposed to assistive technology, including native
increment/decrement actions. `OnSelect` handles pointer clicks and Enter.
`ActiveIndex` controls persistent selection separately from hover.

### Replace data

`Data(newData)` copies the new data, resets transient/selected state, invalidates
geometry and restarts the entrance. Do not mutate the original maps to update a
chart. Call `Data` on the UI thread, or create a chart inside `ggui.View` for a
reactive data source. Configure layout options before mounting or rebuild through
a reactive boundary when those options change.

### Animate changes

Entrances use the reference's ease curve. Duration is 400ms for bars and 1500ms
for the other families; pies additionally wait 400ms before starting. Areas reveal
horizontally, lines reveal by stroke length, bars grow, pies/radials sweep, and
radars expand from the center. `Animation` and `AnimationDelay` override timing.
`Animation(0)` disables entrance motion; `Replay`
restarts it. Reduced motion immediately renders the final state. Data replacement
replays entrance motion; arbitrary point-to-point data morphing is not implemented.

## Efficiency

Geometry is rebuilt only after a data/configuration change or resize. Curve paths
are cached; large series retain ordered per-pixel extrema and gap boundaries,
while tooltips still read the original observations. Cartesian hit lookup is
constant-time. Dense curves are partitioned to avoid the renderer's stencil
winding limits. Text measurements and gradient coverage masks are reused.
Gradient masks are cropped to each path's bounds and recomposited on the GPU;
there is no per-frame CPU bitmap rasterization.

On an Apple M1 Ultra, the included 100,000-observation benchmarks measured roughly
2.4ms to prepare geometry, zero allocations on a geometry-cache hit, and about
1µs / one small allocation for cached **headless** paint with axes disabled.
The headless number excludes GPU submission and text drawing; it is not an FPS
claim. Run the benchmarks on the target device:

```sh
go test ./ui -run '^$' -bench BenchmarkChart -benchmem
```

## Complete reference catalog and visual validation

Run from the repository root:

```sh
go run ./examples/charts
go run ./examples/charts -dark -chart chart-pie-donut-text
go run ./examples/charts -render-dir /tmp/ggui-chart-renders
```

The catalog includes all 70 examples from the inspected shadcn source, using its
original datasets. The picker changes examples; Replay restarts the entrance;
Dark theme checks token inheritance. Interactive examples include date-range,
series and persistent pie selection controls. The existing component gallery also
includes a chart preview under Data.

The render command exercises the actual GPU renderer and writes 560 PNGs: all
70 variants, light/dark, at 0/150/750/2000ms. Tooltip fixtures also render their
standalone content. This is an explicit developer tool; it does not run during
ordinary unit tests or capture the user's desktop.

Reference review used the live shadcn gallery and the source of every example.
Native render review covered the complete catalog and animation samples. The
comparison/fix cycle corrected translucent gradient seams, lost sections in dense
curves, category tick collisions, stacked-bar joins, polar sizing and labels,
series-color order, custom tooltip totals and keyboard/hover interference.
This is a native visual adaptation, not a pixel-identical browser rendering:
ggui fonts and controls remain native.

Automated tests cover every catalog entry at 320/640px widths, signed and expanded
stacks, invalid/missing data, empty and narrow layouts, selection, keyboard and
assistive actions, reduced motion, cache invalidation, path device scaling,
legend wrapping, independent content theming and preservation of sampled spikes.
