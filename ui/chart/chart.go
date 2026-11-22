// Package chart renders native area, bar, line, pie, radar, and radial charts
// directly through ggui's GPU canvas, sharing theme tokens, tooltips, legends,
// keyboard navigation, and reduced-motion support.
package chart

import (
	"fmt"
	"image/color"
	"maps"
	"math"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/internal/controlkit"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Kind selects a native chart renderer. All kinds share config, tooltips,
// legends, keyboard navigation, theme tokens and reduced-motion support.
type Kind uint8

const (
	KindArea Kind = iota
	KindBar
	KindLine
	KindPie
	KindRadar
	KindRadial
)

// polar reports whether the kind renders around a center rather than on x/y
// axes. Kinds ask through this rather than comparing against KindLine, so the
// iota order of the block above stays an implementation detail.
func (k Kind) polar() bool { return k == KindPie || k == KindRadar || k == KindRadial }

// Curve controls interpolation between observations.
type Curve uint8

const (
	CurveNatural Curve = iota
	CurveLinear
	CurveStep
	CurveMonotone
)

// Stack controls series accumulation. Positive and negative values have
// separate baselines; expanded stacks normalize each category to 100 percent.
type Stack uint8

const (
	StackUnstacked Stack = iota
	StackStacked
	StackExpanded
)

// Indicator selects the tooltip's series marker.
type Indicator uint8

const (
	IndicatorDot Indicator = iota
	IndicatorLine
	IndicatorDashed
)

// Series describes one numeric data key. Zero-valued colors use Theme.Chart.
// Icon is optional and is shared by tooltip and legend.
type Series struct {
	// ColorIndex is a 1-based Theme.Chart token; zero uses the series order.
	ColorIndex int
	ThemeColor func(*uitheme.Theme) color.Color
	// Optional per-series polar radii are fractions of the chart radius.
	InnerRadius, OuterRadius float64
	FillOpacity              *float64
	Key, Label               string
	Color                    color.Color
	Icon                     icons.Role
}

// Config is an ordered list of chart series.
type Config []Series

// Datum is a category and its keyed values. Missing/non-finite values are
// gaps, never zero. Color and Icon optionally override a category's appearance.
type Datum struct {
	ColorIndex int
	Label      string
	Values     map[string]float64
	Color      color.Color
	Icon       icons.Role
}

// TooltipOptions customize the built-in content without affecting hit tests.
// FormatValue and FormatLabel also format the accessible data description.
// Formatters and content callbacks must treat the supplied data as read-only.
type TooltipOptions struct {
	ShowTotal  bool
	TotalLabel string
	FormatName func(Series, Datum) string
	// Content replaces the popup body with any ggui widget.
	Content                            func(TooltipPayload) ggui.Widget
	HideLabel, HideIndicator, Disabled bool
	Indicator                          Indicator
	Label                              string
	FormatLabel                        func(string) string
	FormatValue                        func(float64, Series, Datum) string
}

// TooltipPayload provides typed data to custom tooltip content.
type TooltipPayload struct {
	Index  int
	Datum  Datum
	Config Config
}

// PolarGrid describes radar grid variants. Rings defaults to 5.
type PolarGrid struct {
	Circle, Fill, HideSpokes, HideRings bool
	Rings                               int
}

// PointContext is passed to optional custom point and label painters.
type PointContext struct {
	Plot               ggui.Rect
	Env                ggui.Env
	Index, SeriesIndex int
	Datum              Datum
	Series             Series
	Value              float64
	Position           ggui.Point
	Color              color.Color
	Active             bool
}

// Widget renders charts directly through ggui's GPU canvas. It owns a
// snapshot of data; Data replaces it and invalidates cached geometry. Configure
// a widget before mounting, or use a reactive boundary to replace its options.
type Widget struct {
	props       property.Owner
	words       *ui.Strings // the Strings of the last layout
	pointer     controlkit.PointerMotion
	tickPaint   func(*ggui.Canvas, PointContext)
	activeRing  bool
	radialTrack bool
	labelInside bool
	ggui.Interactive
	kind                                                           Kind
	data                                                           []Datum
	config                                                         Config
	height                                                         float64
	curve                                                          Curve
	stack                                                          Stack
	horizontal, grid, xAxis, yAxis, legend, dots, labels, gradient bool
	categoryColors, separators, tooltipCursor                      bool
	inner, outer, startAngle, endAngle, corner, stroke, opacity    float64
	domainMin, domainMax                                           float64
	domainSet                                                      bool
	centerValue, centerLabel                                       string
	polarGrid                                                      PolarGrid
	tooltip                                                        TooltipOptions
	tickFormat                                                     func(string) string
	labelFormat                                                    func(PointContext) string
	dotPaint                                                       func(*ggui.Canvas, PointContext)
	labelPaint                                                     func(*ggui.Canvas, PointContext)
	onSelect                                                       func(int, Datum)
	duration                                                       time.Duration
	delay                                                          time.Duration
	env                                                            ggui.Env
	theme                                                          *uitheme.Theme
	reduced                                                        bool
	gridLine                                                       color.Color // the cartesian grid, resolved each layout
	active, selected                                               int
	plot                                                           ggui.Rect
	geometry                                                       chartGeometry
	cacheSize                                                      ggui.Size
	revision, cacheRevision                                        uint64
	started                                                        time.Time
	textCache                                                      map[chartTextKey]*chartText
	textLayout                                                     uint64 // counts Layouts, so a cached label re-measures once under each
	curves                                                         [][]chartCurvePaths
	curveSize                                                      ggui.Size
}

// Container creates a responsive chart. The default height is 240 logical
// pixels; an unbounded width falls back to 400. Constructors below select a kind.
func Container(kind Kind, data []Datum, config Config) *Widget {
	c := &Widget{kind: kind, config: append(Config(nil), config...), height: 240,
		radialTrack: true, grid: true, xAxis: true, curve: CurveNatural, inner: 0, outer: .8, startAngle: 0, endAngle: 360,
		corner: 4, stroke: 2, opacity: .4, separators: true, tooltipCursor: true, duration: 1500 * time.Millisecond,
		active: -1, selected: -1, cacheRevision: ^uint64(0)}
	c.Role = ggui.RoleGroup
	c.AutoKey()
	switch kind {
	case KindBar:
		c.duration = 400 * time.Millisecond
	case KindPie:
		c.delay = 400 * time.Millisecond
	case KindArea:
		c.stroke = 1
	case KindRadar:
		c.opacity = .6
	}
	c.Data(data)
	return c
}

// Area creates an area chart.
func Area(data []Datum, config Config) *Widget {
	return Container(KindArea, data, config)
}

// Bar creates a bar chart.
func Bar(data []Datum, config Config) *Widget {
	return Container(KindBar, data, config)
}

// Line creates a line chart.
func Line(data []Datum, config Config) *Widget {
	return Container(KindLine, data, config)
}

// Pie creates a pie chart with category colors enabled.
func Pie(data []Datum, config Config) *Widget {
	return Container(KindPie, data, config).CategoryColors(true)
}

// Radar creates a radar chart.
func Radar(data []Datum, config Config) *Widget {
	return Container(KindRadar, data, config)
}

// Radial creates a radial bar chart with a 0.25 inner radius and category colors enabled.
func Radial(data []Datum, config Config) *Widget {
	return Container(KindRadial, data, config).InnerRadius(.25).CategoryColors(true)
}

// Key assigns this control an input identity. Configure it before mount.
func (c *Widget) Key(k any) *Widget { c.Interactive.SetKey(k); return c }

// Name sets the accessible name.
func (c *Widget) Name(s string) *Widget { c.SetName(s); return c }

// BindName follows a non-nil accessible-name reader.
func (c *Widget) BindName(r ggui.Readable[string]) *Widget {
	c.Interactive.BindName(r)
	return c
}

// Data copies input, preventing caller mutation from corrupting geometry caches.
func (c *Widget) Data(data []Datum) *Widget {
	defer property.Watch(&c.props, &c.active)()
	defer property.Watch(&c.props, &c.data)()
	defer property.Watch(&c.props, &c.selected)()
	defer property.Watch(&c.props, &c.started)()
	defer property.Watch(&c.props, &c.textCache)()
	c.data = make([]Datum, len(data))
	for i, d := range data {
		c.data[i] = d
		c.data[i].Values = make(map[string]float64, len(d.Values))
		maps.Copy(c.data[i].Values, d.Values)
	}
	c.revision++
	c.started = time.Time{}
	c.active = -1
	c.selected = -1
	c.textCache = nil
	return c
}
func (c *Widget) Height(v float64) *Widget {
	defer property.Watch(&c.props, &c.height)()
	c.height = max(0, v)
	return c
}
func (c *Widget) Curve(v Curve) *Widget {
	defer property.Watch(&c.props, &c.curve)()
	c.curve = v
	c.revision++
	return c
}
func (c *Widget) Stack(v Stack) *Widget {
	defer property.Watch(&c.props, &c.stack)()
	c.stack = v
	c.revision++
	return c
}
func (c *Widget) Horizontal(v bool) *Widget { c.horizontal = v; c.revision++; return c }
func (c *Widget) Grid(v bool) *Widget {
	defer property.Watch(&c.props, &c.grid)()
	c.grid = v
	return c
}
func (c *Widget) Axes(x, y bool) *Widget {
	defer property.Watch(&c.props, &c.xAxis)()
	defer property.Watch(&c.props, &c.yAxis)()
	c.xAxis = x
	c.yAxis = y
	c.revision++
	return c
}
func (c *Widget) Legend(v bool) *Widget {
	defer property.Watch(&c.props, &c.legend)()
	c.legend = v
	c.revision++
	return c
}
func (c *Widget) Dots(v bool) *Widget {
	defer property.Watch(&c.props, &c.dots)()
	c.dots = v
	return c
}
func (c *Widget) Labels(v bool) *Widget {
	defer property.Watch(&c.props, &c.labels)()
	c.labels = v
	return c
}
func (c *Widget) Gradient(v bool) *Widget {
	defer property.Watch(&c.props, &c.gradient)()
	c.gradient = v
	return c
}
func (c *Widget) CategoryColors(v bool) *Widget {
	defer property.Watch(&c.props, &c.categoryColors)()
	c.categoryColors = v
	return c
}
func (c *Widget) Separators(v bool) *Widget {
	defer property.Watch(&c.props, &c.separators)()
	c.separators = v
	return c
}
func (c *Widget) InnerRadius(fraction float64) *Widget {
	defer property.Watch(&c.props, &c.inner)()
	c.inner = clamp(fraction, 0, .95)
	return c
}
func (c *Widget) OuterRadius(fraction float64) *Widget {
	defer property.Watch(&c.props, &c.outer)()
	c.outer = clamp(fraction, .05, 1)
	return c
}

// Angles are degrees, counter-clockwise from three o'clock, like Recharts.
func (c *Widget) Angles(start, end float64) *Widget {
	defer property.Watch(&c.props, &c.endAngle)()
	defer property.Watch(&c.props, &c.startAngle)()
	if finite(start) && finite(end) {
		c.startAngle = start
		c.endAngle = start + clamp(end-start, -360, 360)
	}
	return c
}
func (c *Widget) CornerRadius(v float64) *Widget {
	defer property.Watch(&c.props, &c.corner)()
	c.corner = max(0, v)
	return c
}
func (c *Widget) StrokeWidth(v float64) *Widget {
	defer property.Watch(&c.props, &c.stroke)()
	c.stroke = max(0, v)
	return c
}
func (c *Widget) FillOpacity(v float64) *Widget {
	defer property.Watch(&c.props, &c.opacity)()
	c.opacity = clamp(v, 0, 1)
	return c
}
func (c *Widget) Domain(lo, hi float64) *Widget {
	defer property.Watch(&c.props, &c.domainMax)()
	defer property.Watch(&c.props, &c.domainMin)()
	defer property.Watch(&c.props, &c.domainSet)()
	c.domainSet = finite(lo) && finite(hi) && hi > lo
	c.domainMin = lo
	c.domainMax = hi
	c.revision++
	return c
}
func (c *Widget) CenterText(value, label string) *Widget {
	defer property.Watch(&c.props, &c.centerLabel)()
	defer property.Watch(&c.props, &c.centerValue)()
	c.centerValue = value
	c.centerLabel = label
	return c
}
func (c *Widget) PolarGrid(v PolarGrid) *Widget {
	defer property.Watch(&c.props, &c.polarGrid)()
	c.polarGrid = v
	return c
}
func (c *Widget) Tooltip(v TooltipOptions) *Widget {
	defer property.Watch(&c.props, &c.tooltip)()
	c.tooltip = v
	return c
}
func (c *Widget) TooltipCursor(v bool) *Widget {
	defer property.Watch(&c.props, &c.tooltipCursor)()
	c.tooltipCursor = v
	return c
}
func (c *Widget) TickFormatter(fn func(string) string) *Widget { c.tickFormat = fn; return c }
func (c *Widget) LabelFormatter(fn func(PointContext) string) *Widget {
	c.labelFormat = fn
	c.labels = true
	return c
}
func (c *Widget) DotPainter(fn func(*ggui.Canvas, PointContext)) *Widget {
	c.dotPaint = fn
	c.dots = true
	return c
}
func (c *Widget) LabelPainter(fn func(*ggui.Canvas, PointContext)) *Widget {
	c.labelPaint = fn
	c.labels = true
	return c
}
func (c *Widget) OnSelect(fn func(int, Datum)) *Widget { c.onSelect = fn; return c }

// ActiveIndex selects a persistent shape, independent of transient hover.
func (c *Widget) ActiveIndex(index int) *Widget {
	defer property.Watch(&c.props, &c.selected)()
	c.selected = index
	return c
}
func (c *Widget) Animation(d time.Duration) *Widget {
	defer property.Watch(&c.props, &c.duration)()
	c.duration = max(0, d)
	return c
}

// AnimationDelay overrides the entrance delay (400ms for pies, zero otherwise).
func (c *Widget) AnimationDelay(d time.Duration) *Widget {
	defer property.Watch(&c.props, &c.delay)()
	c.delay = max(0, d)
	return c
}

// Replay restarts the entrance without rebuilding or copying the data.
func (c *Widget) Replay() {
	c.started = time.Time{}
	ggui.Invalidate(c.env)
}
func (c *Widget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	c.env = env
	c.words = ui.StringsOf(env)
	c.theme = uitheme.Of(env)
	c.reduced = env.ReducedMotion()
	c.gridLine = fade(c.theme.Border, .5) // shadcn strokes the grid at border/50
	c.textLayout++                        // cached labels re-measure under env as they are next used
	return cs.Constrain(ggui.Sz(bounded(cs.MaxW, 400), c.height))
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (c *Widget) value(i, j int) (float64, bool) {
	if i < 0 || i >= len(c.data) || j < 0 || j >= len(c.config) {
		return 0, false
	}
	v, ok := c.data[i].Values[c.config[j].Key]
	return v, ok && finite(v)
}
func (c *Widget) seriesColor(j int) color.Color {
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
	return controlkit.ColorOr(s.Color, c.chartToken(index))
}

// chartToken picks the i'th palette token, wrapping, and falls back to the
// primary color when the theme leaves the slot unset.
func (c *Widget) chartToken(i int) color.Color {
	return controlkit.ColorOr(c.theme.Chart[i%len(c.theme.Chart)], c.theme.Primary)
}
func (c *Widget) pointColor(i, j int) color.Color {
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
func (c *Widget) seriesOpacity(j int) float64 {
	if c.config[j].FillOpacity != nil {
		return clamp(*c.config[j].FillOpacity, 0, 1)
	}
	return c.opacity
}

// TickPainter customizes polar category labels, including icons and multiple lines.
func (c *Widget) TickPainter(fn func(*ggui.Canvas, PointContext)) *Widget {
	c.tickPaint = fn
	return c
}

// ActiveRing draws an additional outer ring around a selected pie sector.
func (c *Widget) ActiveRing(v bool) *Widget {
	defer property.Watch(&c.props, &c.activeRing)()
	c.activeRing = v
	return c
}

// RadialTrack toggles the muted full-circle track behind radial bars.
func (c *Widget) RadialTrack(v bool) *Widget {
	defer property.Watch(&c.props, &c.radialTrack)()
	c.radialTrack = v
	return c
}

// InsideLabels places pie labels inside sectors; otherwise labels sit outside.
func (c *Widget) InsideLabels(v bool) *Widget {
	defer property.Watch(&c.props, &c.labelInside)()
	c.labelInside = v
	return c
}

// DefaultTooltipIndex sets initial tooltip state without selecting a shape.
func (c *Widget) DefaultTooltipIndex(i int) *Widget {
	defer property.Watch(&c.props, &c.active)()
	c.active = i
	return c
}
func (c *Widget) formatValue(i, j int, v float64) string {
	if c.tooltip.FormatValue != nil {
		return c.tooltip.FormatValue(v, c.config[j], c.data[i])
	}
	return fmt.Sprintf("%g", v)
}
func seriesLabel(s Series) string {
	if s.Label != "" {
		return s.Label
	}
	return s.Key
}
func (c *Widget) name() string {
	return pick(c.SemanticName() != "", c.SemanticName(), c.words.OrDefault().ChartName)
}

// Semantics implements ggui.Semantic, including the built-in fallback name.
func (c *Widget) Semantics() (ggui.Role, string) { return c.Role, c.name() }

func (c *Widget) Describe() ggui.Node {
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
	return ggui.Node{Role: ggui.RoleGroup, Name: c.name(), Value: b.String(), Description: c.words.OrDefault().ChartHelp, Min: 0, Max: float64(max(0, len(c.data)-1)), Now: float64(max(0, i)), Actions: ggui.ActionFocus | ggui.ActionIncrement | ggui.ActionDecrement | ggui.ActionSetValue}
}

// Act lets assistive technologies explore chart categories without a pointer.
func (c *Widget) Act(a ggui.Action) bool {
	if len(c.data) == 0 {
		return false
	}
	switch a.Kind {
	case ggui.ActionIncrement:
		c.active = controlkit.StepIndex(c.active, 1, len(c.data), nil)
	case ggui.ActionDecrement:
		c.active = controlkit.StepIndex(c.active, -1, len(c.data), nil)
	case ggui.ActionSetValue:
		if !finite(a.Num) {
			return false
		}
		c.active = int(clamp(a.Num, 0, float64(len(c.data)-1)))
	default:
		return false
	}
	return true
}
func (c *Widget) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd, ggui.KeyEscape, ggui.KeyEnter:
		return ev.Kind == ggui.KeyPress
	}
	return false
}
func (c *Widget) HandleKey(ev ggui.KeyEvent) {
	c.Keyboard(ev, nil)
	if ev.Kind != ggui.KeyPress || len(c.data) == 0 {
		return
	}
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowDown:
		c.active = controlkit.StepIndex(c.active, -1, len(c.data), nil)
	case ggui.KeyArrowRight, ggui.KeyArrowUp:
		c.active = controlkit.StepIndex(c.active, 1, len(c.data), nil)
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
func (c *Widget) selectActive() {
	if c.active >= 0 && c.active < len(c.data) {
		c.selected = c.active
		if c.onSelect != nil {
			c.onSelect(c.active, c.data[c.active])
		}
	}
}
func (c *Widget) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerScroll {
		return false
	}
	c.Pointer(ev, nil)
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove, ggui.PointerDown:
		if c.pointer.Moved(ev.Pos) || ev.Kind == ggui.PointerDown {
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
func (c *Widget) progress() float64 {
	if c.reduced || c.duration <= 0 {
		return 1
	}
	now := ggui.FrameTime()
	if c.started.IsZero() {
		c.started = now
	}
	elapsed := now.Sub(c.started)
	if elapsed < c.delay {
		ggui.WakeAt(c.started.Add(c.delay))
		return 0
	}
	t := clamp(float64(elapsed-c.delay)/float64(c.duration), 0, 1)
	if t < 1 {
		ggui.Now() // still entering, so ask for the next frame
	}
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
