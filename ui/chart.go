package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
)

// ChartKind selects a native chart renderer. All kinds share config, tooltips,
// legends, keyboard navigation, theme tokens and reduced-motion support.
type ChartKind uint8

const (
	ChartArea ChartKind = iota
	ChartBar
	ChartLine
	ChartPie
	ChartRadar
	ChartRadial
)

// polar reports whether the kind renders around a center rather than on x/y
// axes. Kinds ask through this rather than comparing against ChartLine, so the
// iota order of the block above stays an implementation detail.
func (k ChartKind) polar() bool { return k == ChartPie || k == ChartRadar || k == ChartRadial }

// ChartCurve controls interpolation between observations.
type ChartCurve uint8

const (
	ChartNatural ChartCurve = iota
	ChartLinear
	ChartStep
	ChartMonotone
)

// ChartStack controls series accumulation. Positive and negative values have
// separate baselines; expanded stacks normalize each category to 100 percent.
type ChartStack uint8

const (
	ChartUnstacked ChartStack = iota
	ChartStacked
	ChartExpanded
)

// ChartIndicator selects the tooltip's series marker.
type ChartIndicator uint8

const (
	ChartIndicatorDot ChartIndicator = iota
	ChartIndicatorLine
	ChartIndicatorDashed
)

// ChartSeries describes one numeric data key. Zero-valued colors use Theme.Chart.
// Icon is optional and is shared by tooltip and legend.
type ChartSeries struct {
	// ColorIndex is a 1-based Theme.Chart token; zero uses the series order.
	ColorIndex int
	ThemeColor func(ggui.Theme) color.Color
	// Optional per-series polar radii are fractions of the chart radius.
	InnerRadius, OuterRadius float64
	FillOpacity              *float64
	Key, Label               string
	Color                    color.Color
	Icon                     icons.Role
}
type ChartConfig []ChartSeries

// ChartDatum is a category and its keyed values. Missing/non-finite values are
// gaps, never zero. Color and Icon optionally override a category's appearance.
type ChartDatum struct {
	ColorIndex int
	Label      string
	Values     map[string]float64
	Color      color.Color
	Icon       icons.Role
}

// ChartTooltipOptions customize the built-in content without affecting hit tests.
// FormatValue and FormatLabel also format the accessible data description.
// Formatters and content callbacks must treat the supplied data as read-only.
type ChartTooltipOptions struct {
	ShowTotal  bool
	TotalLabel string
	FormatName func(ChartSeries, ChartDatum) string
	// Content replaces the popup body with any ggui widget.
	Content                            func(ChartTooltipPayload) ggui.Widget
	HideLabel, HideIndicator, Disabled bool
	Indicator                          ChartIndicator
	Label                              string
	FormatLabel                        func(string) string
	FormatValue                        func(float64, ChartSeries, ChartDatum) string
}

// ChartTooltipPayload provides typed data to custom tooltip content.
type ChartTooltipPayload struct {
	Index  int
	Datum  ChartDatum
	Config ChartConfig
}

// ChartPolarGrid describes radar grid variants. Rings defaults to 5.
type ChartPolarGrid struct {
	Circle, Fill, HideSpokes, HideRings bool
	Rings                               int
}

// ChartPointContext is passed to optional custom point and label painters.
type ChartPointContext struct {
	Plot               ggui.Rect
	Env                ggui.Env
	Index, SeriesIndex int
	Datum              ChartDatum
	Series             ChartSeries
	Value              float64
	Position           ggui.Point
	Color              color.Color
	Active             bool
}

// ChartWidget renders charts directly through ggui's GPU canvas. It owns a
// snapshot of data; Data replaces it and invalidates cached geometry. Configure
// a widget before mounting, or use a reactive boundary to replace its options.
type ChartWidget struct {
	pointer     pointerMotion
	tickPaint   func(*ggui.Canvas, ChartPointContext)
	activeRing  bool
	radialTrack bool
	labelInside bool
	ggui.Interactive
	kind                                                           ChartKind
	data                                                           []ChartDatum
	config                                                         ChartConfig
	height                                                         float64
	curve                                                          ChartCurve
	stack                                                          ChartStack
	horizontal, grid, xAxis, yAxis, legend, dots, labels, gradient bool
	categoryColors, separators, tooltipCursor                      bool
	inner, outer, startAngle, endAngle, corner, stroke, opacity    float64
	domainMin, domainMax                                           float64
	domainSet                                                      bool
	centerValue, centerLabel                                       string
	polarGrid                                                      ChartPolarGrid
	tooltip                                                        ChartTooltipOptions
	tickFormat                                                     func(string) string
	labelFormat                                                    func(ChartPointContext) string
	dotPaint                                                       func(*ggui.Canvas, ChartPointContext)
	labelPaint                                                     func(*ggui.Canvas, ChartPointContext)
	onSelect                                                       func(int, ChartDatum)
	duration                                                       time.Duration
	delay                                                          time.Duration
	env                                                            ggui.Env
	theme                                                          ggui.Theme
	reduced                                                        bool
	active, selected                                               int
	plot                                                           ggui.Rect
	geometry                                                       chartGeometry
	cacheSize                                                      ggui.Size
	revision, cacheRevision                                        uint64
	started                                                        time.Time
	textCache                                                      map[chartTextKey]*chartText
	curves                                                         [][]chartCurvePaths
	curveRect                                                      ggui.Rect
}

// ChartContainer creates a responsive chart. The default height is 240 logical
// pixels; an unbounded width falls back to 400. Constructors below select a kind.
func ChartContainer(kind ChartKind, data []ChartDatum, config ChartConfig) *ChartWidget {
	c := &ChartWidget{kind: kind, config: append(ChartConfig(nil), config...), height: 240,
		radialTrack: true, grid: true, xAxis: true, curve: ChartNatural, inner: 0, outer: .8, startAngle: 0, endAngle: 360,
		corner: 4, stroke: 2, opacity: .4, separators: true, tooltipCursor: true, duration: 1500 * time.Millisecond,
		active: -1, selected: -1, cacheRevision: ^uint64(0)}
	c.Role = ggui.RoleGroup
	c.Name = "Chart"
	c.AutoKey()
	switch kind {
	case ChartBar:
		c.duration = 400 * time.Millisecond
	case ChartPie:
		c.delay = 400 * time.Millisecond
	case ChartArea:
		c.stroke = 1
	case ChartRadar:
		c.opacity = .6
	}
	c.Data(data)
	return c
}
func AreaChart(data []ChartDatum, config ChartConfig) *ChartWidget {
	return ChartContainer(ChartArea, data, config)
}
func BarChart(data []ChartDatum, config ChartConfig) *ChartWidget {
	return ChartContainer(ChartBar, data, config)
}
func LineChart(data []ChartDatum, config ChartConfig) *ChartWidget {
	return ChartContainer(ChartLine, data, config)
}
func PieChart(data []ChartDatum, config ChartConfig) *ChartWidget {
	return ChartContainer(ChartPie, data, config).CategoryColors(true)
}
func RadarChart(data []ChartDatum, config ChartConfig) *ChartWidget {
	return ChartContainer(ChartRadar, data, config)
}
func RadialChart(data []ChartDatum, config ChartConfig) *ChartWidget {
	return ChartContainer(ChartRadial, data, config).InnerRadius(.25).CategoryColors(true)
}
func (c *ChartWidget) Named(s string) *ChartWidget { c.Name = s; return c }

// Data copies input, preventing caller mutation from corrupting geometry caches.
func (c *ChartWidget) Data(data []ChartDatum) *ChartWidget {
	c.data = make([]ChartDatum, len(data))
	for i, d := range data {
		c.data[i] = d
		c.data[i].Values = make(map[string]float64, len(d.Values))
		for k, v := range d.Values {
			c.data[i].Values[k] = v
		}
	}
	c.revision++
	c.started = time.Time{}
	c.active = -1
	c.selected = -1
	c.textCache = nil
	return c
}
func (c *ChartWidget) Height(v float64) *ChartWidget      { c.height = max(0, v); return c }
func (c *ChartWidget) Curve(v ChartCurve) *ChartWidget    { c.curve = v; c.revision++; return c }
func (c *ChartWidget) Stack(v ChartStack) *ChartWidget    { c.stack = v; c.revision++; return c }
func (c *ChartWidget) Horizontal(v bool) *ChartWidget     { c.horizontal = v; c.revision++; return c }
func (c *ChartWidget) Grid(v bool) *ChartWidget           { c.grid = v; return c }
func (c *ChartWidget) Axes(x, y bool) *ChartWidget        { c.xAxis = x; c.yAxis = y; c.revision++; return c }
func (c *ChartWidget) Legend(v bool) *ChartWidget         { c.legend = v; c.revision++; return c }
func (c *ChartWidget) Dots(v bool) *ChartWidget           { c.dots = v; return c }
func (c *ChartWidget) Labels(v bool) *ChartWidget         { c.labels = v; return c }
func (c *ChartWidget) Gradient(v bool) *ChartWidget       { c.gradient = v; return c }
func (c *ChartWidget) CategoryColors(v bool) *ChartWidget { c.categoryColors = v; return c }
func (c *ChartWidget) Separators(v bool) *ChartWidget     { c.separators = v; return c }
func (c *ChartWidget) InnerRadius(fraction float64) *ChartWidget {
	c.inner = clamp(fraction, 0, .95)
	return c
}
func (c *ChartWidget) OuterRadius(fraction float64) *ChartWidget {
	c.outer = clamp(fraction, .05, 1)
	return c
}

// Angles are degrees, counter-clockwise from three o'clock, like Recharts.
func (c *ChartWidget) Angles(start, end float64) *ChartWidget {
	if finite(start) && finite(end) {
		c.startAngle = start
		c.endAngle = start + clamp(end-start, -360, 360)
	}
	return c
}
func (c *ChartWidget) CornerRadius(v float64) *ChartWidget { c.corner = max(0, v); return c }
func (c *ChartWidget) StrokeWidth(v float64) *ChartWidget  { c.stroke = max(0, v); return c }
func (c *ChartWidget) FillOpacity(v float64) *ChartWidget  { c.opacity = clamp(v, 0, 1); return c }
func (c *ChartWidget) Domain(lo, hi float64) *ChartWidget {
	c.domainSet = finite(lo) && finite(hi) && hi > lo
	c.domainMin = lo
	c.domainMax = hi
	c.revision++
	return c
}
func (c *ChartWidget) CenterText(value, label string) *ChartWidget {
	c.centerValue = value
	c.centerLabel = label
	return c
}
func (c *ChartWidget) PolarGrid(v ChartPolarGrid) *ChartWidget           { c.polarGrid = v; return c }
func (c *ChartWidget) Tooltip(v ChartTooltipOptions) *ChartWidget        { c.tooltip = v; return c }
func (c *ChartWidget) TooltipCursor(v bool) *ChartWidget                 { c.tooltipCursor = v; return c }
func (c *ChartWidget) TickFormatter(fn func(string) string) *ChartWidget { c.tickFormat = fn; return c }
func (c *ChartWidget) LabelFormatter(fn func(ChartPointContext) string) *ChartWidget {
	c.labelFormat = fn
	c.labels = true
	return c
}
func (c *ChartWidget) DotPainter(fn func(*ggui.Canvas, ChartPointContext)) *ChartWidget {
	c.dotPaint = fn
	c.dots = true
	return c
}
func (c *ChartWidget) LabelPainter(fn func(*ggui.Canvas, ChartPointContext)) *ChartWidget {
	c.labelPaint = fn
	c.labels = true
	return c
}
func (c *ChartWidget) OnSelect(fn func(int, ChartDatum)) *ChartWidget { c.onSelect = fn; return c }

// ActiveIndex selects a persistent shape, independent of transient hover.
func (c *ChartWidget) ActiveIndex(index int) *ChartWidget     { c.selected = index; return c }
func (c *ChartWidget) Animation(d time.Duration) *ChartWidget { c.duration = max(0, d); return c }

// AnimationDelay overrides the entrance delay (400ms for pies, zero otherwise).
func (c *ChartWidget) AnimationDelay(d time.Duration) *ChartWidget { c.delay = max(0, d); return c }

// Replay restarts the entrance without rebuilding or copying the data.
func (c *ChartWidget) Replay() { c.started = time.Time{} }
func (c *ChartWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c.env = env
	c.theme = env.Theme()
	c.reduced = env.ReducedMotion()
	c.textCache = nil
	return cs.Constrain(ggui.Sz(bounded(cs.MaxW, 400), c.height))
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (c *ChartWidget) value(i, j int) (float64, bool) {
	if i < 0 || i >= len(c.data) || j < 0 || j >= len(c.config) {
		return 0, false
	}
	v, ok := c.data[i].Values[c.config[j].Key]
	return v, ok && finite(v)
}
func (c *ChartWidget) seriesColor(j int) color.Color {
	s := c.config[j]
	if s.ThemeColor != nil {
		if col := s.ThemeColor(c.theme); col != nil {
			return col
		}
	}
	index := j
	if s.ColorIndex > 0 {
		index = s.ColorIndex - 1
	}
	return colorOr(s.Color, c.chartToken(index))
}

// chartToken picks the i'th palette token, wrapping, and falls back to the
// primary color when the theme leaves the slot unset.
func (c *ChartWidget) chartToken(i int) color.Color {
	return colorOr(c.theme.Chart[i%len(c.theme.Chart)], c.theme.Primary)
}
func (c *ChartWidget) pointColor(i, j int) color.Color {
	d := c.data[i]
	if d.Color != nil {
		return d.Color
	}
	if d.ColorIndex > 0 {
		return c.chartToken(d.ColorIndex - 1)
	}
	if c.categoryColors {
		return c.chartToken(i)
	}
	return c.seriesColor(j)
}
func (c *ChartWidget) seriesOpacity(j int) float64 {
	if c.config[j].FillOpacity != nil {
		return clamp(*c.config[j].FillOpacity, 0, 1)
	}
	return c.opacity
}

// TickPainter customizes polar category labels, including icons and multiple lines.
func (c *ChartWidget) TickPainter(fn func(*ggui.Canvas, ChartPointContext)) *ChartWidget {
	c.tickPaint = fn
	return c
}

// ActiveRing draws an additional outer ring around a selected pie sector.
func (c *ChartWidget) ActiveRing(v bool) *ChartWidget { c.activeRing = v; return c }

// RadialTrack toggles the muted full-circle track behind radial bars.
func (c *ChartWidget) RadialTrack(v bool) *ChartWidget { c.radialTrack = v; return c }

// InsideLabels places pie labels inside sectors; otherwise labels sit outside.
func (c *ChartWidget) InsideLabels(v bool) *ChartWidget { c.labelInside = v; return c }

// DefaultTooltipIndex sets initial tooltip state without selecting a shape.
func (c *ChartWidget) DefaultTooltipIndex(i int) *ChartWidget { c.active = i; return c }
func (c *ChartWidget) formatValue(i, j int, v float64) string {
	if c.tooltip.FormatValue != nil {
		return c.tooltip.FormatValue(v, c.config[j], c.data[i])
	}
	return fmt.Sprintf("%g", v)
}
func seriesLabel(s ChartSeries) string {
	if s.Label != "" {
		return s.Label
	}
	return s.Key
}
func (c *ChartWidget) Describe() ggui.Node {
	var b strings.Builder
	i := c.active
	if i < 0 {
		i = c.selected
	}
	if i >= 0 && i < len(c.data) {
		b.WriteString(c.tooltipLabel(i))
		for j, s := range c.config {
			if v, ok := c.value(i, j); ok {
				fmt.Fprintf(&b, ", %s: %s", seriesLabel(s), c.formatValue(i, j, v))
			}
		}
	}
	return ggui.Node{Role: ggui.RoleGroup, Name: c.Name, Value: b.String(), Description: "Chart. Use arrow keys to explore values; Enter selects a category.", Min: 0, Max: float64(max(0, len(c.data)-1)), Now: float64(max(0, i)), Actions: ggui.ActionFocus | ggui.ActionIncrement | ggui.ActionDecrement | ggui.ActionSetValue}
}

// Act lets assistive technologies explore chart categories without a pointer.
func (c *ChartWidget) Act(a ggui.Action) bool {
	if len(c.data) == 0 {
		return false
	}
	switch a.Kind {
	case ggui.ActionIncrement:
		c.active = stepIndex(c.active, 1, len(c.data), nil)
	case ggui.ActionDecrement:
		c.active = stepIndex(c.active, -1, len(c.data), nil)
	case ggui.ActionSetValue:
		if !finite(a.Num) {
			return false
		}
		// An empty chart has no index to set: clamp would otherwise
		// report 0, which is not a category either.
		if len(c.data) == 0 {
			return false
		}
		c.active = int(clamp(a.Num, 0, float64(len(c.data)-1)))
	default:
		return false
	}
	return true
}
func (c *ChartWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd, ggui.KeyEscape, ggui.KeyEnter:
		return ev.Kind == ggui.KeyPress
	}
	return false
}
func (c *ChartWidget) HandleKey(ev ggui.KeyEvent) {
	c.Keyboard(ev, nil)
	if ev.Kind != ggui.KeyPress || len(c.data) == 0 {
		return
	}
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowDown:
		c.active = stepIndex(c.active, -1, len(c.data), nil)
	case ggui.KeyArrowRight, ggui.KeyArrowUp:
		c.active = stepIndex(c.active, 1, len(c.data), nil)
	case ggui.KeyHome:
		c.active = 0
	case ggui.KeyEnd:
		c.active = len(c.data) - 1
	case ggui.KeyEscape:
		c.active = -1
	case ggui.KeyEnter:
		c.selectActive()
	}
}
func (c *ChartWidget) selectActive() {
	if c.active >= 0 && c.active < len(c.data) {
		c.selected = c.active
		if c.onSelect != nil {
			c.onSelect(c.active, c.data[c.active])
		}
	}
}
func (c *ChartWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerScroll {
		return false
	}
	c.Pointer(ev, nil)
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove, ggui.PointerDown:
		if c.pointer.moved(ev.Pos) || ev.Kind == ggui.PointerDown {
			index := c.hitIndex(ev.Pos)
			if index >= 0 || !c.Focused {
				c.active = index
			}
		}
	case ggui.PointerExit:
		if !c.Focused {
			c.active = -1
		}
	case ggui.PointerTap:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		c.active = c.hitIndex(ev.Pos)
		c.selectActive()
	}
	return true
}
func (c *ChartWidget) progress() float64 {
	if c.reduced || c.duration <= 0 {
		return 1
	}
	now := ggui.Now()
	if c.started.IsZero() {
		c.started = now
	}
	t := clamp(float64(now.Sub(c.started)-c.delay)/float64(c.duration), 0, 1)
	if t == 0 || t == 1 {
		return t
	}
	// Recharts' ease timing: cubic-bezier(.25,.1,.25,1).
	lo, hi := 0., 1.
	for range 12 {
		u := (lo + hi) / 2
		x := 3*(1-u)*(1-u)*u*.25 + 3*(1-u)*u*u*.25 + u*u*u
		if x < t {
			lo = u
		} else {
			hi = u
		}
	}
	u := (lo + hi) / 2
	return 3*(1-u)*(1-u)*u*.1 + 3*(1-u)*u*u + u*u*u
}
