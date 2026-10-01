package ui

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// settledChart mounts c at size and runs its entrance to the end.
func settledChart(t *testing.T, c *ChartWidget, size ggui.Size) *ggui.Probe {
	t.Helper()
	p := ggui.NewProbe(c, size)
	t.Cleanup(p.Close)
	p.Frame()
	p.Advance(3 * time.Second)
	p.Frame()
	return p
}

// within reports whether p lies in r, allowing for rounding at the edges.
func within(r ggui.Rect, p ggui.Point) bool {
	const eps = 1e-6
	return p.X >= r.Origin.X-eps && p.X <= r.Origin.X+r.Size.W+eps && p.Y >= r.Origin.Y-eps && p.Y <= r.Origin.Y+r.Size.H+eps
}

func TestChartDotPainterSeesEveryValuePlacedInThePlot(t *testing.T) {
	t.Parallel()
	data := chartTestData()
	seen := map[[2]int]ChartPointContext{}
	c := LineChart(data, chartTestConfig).Animation(0).ActiveIndex(1).DotPainter(func(_ *ggui.Canvas, ctx ChartPointContext) {
		seen[[2]int{ctx.Index, ctx.SeriesIndex}] = ctx
	})
	settledChart(t, c, ggui.Sz(400, 240))
	if len(seen) != len(data)*len(chartTestConfig) {
		t.Fatalf("dot painter saw %d points, want %d", len(seen), len(data)*len(chartTestConfig))
	}
	for at, ctx := range seen {
		i, j := at[0], at[1]
		if want := data[i].Values[chartTestConfig[j].Key]; ctx.Value != want || ctx.Series.Key != chartTestConfig[j].Key || ctx.Datum.Label != data[i].Label {
			t.Errorf("point %v: value %v of series %q in %q, want %v of %q in %q", at, ctx.Value, ctx.Series.Key, ctx.Datum.Label, want, chartTestConfig[j].Key, data[i].Label)
		}
		if !within(ctx.Plot, ctx.Position) {
			t.Errorf("point %v at %v lies outside the plot %v", at, ctx.Position, ctx.Plot)
		}
		if ctx.Active != (i == 1) {
			t.Errorf("point %v: Active = %v, want it only on the ActiveIndex category", at, ctx.Active)
		}
	}
	// Higher values sit higher: y grows downwards on the canvas.
	if seen[[2]int{2, 0}].Position.Y >= seen[[2]int{0, 0}].Position.Y {
		t.Error("the larger value was not drawn above the smaller one")
	}
}

func TestChartLabelPainterOverridesLabelFormatter(t *testing.T) {
	t.Parallel()
	var formatted, painted int
	format := func(ctx ChartPointContext) string { formatted++; return fmt.Sprintf("%.0f!", ctx.Value) }
	c := BarChart(chartTestData(), chartTestConfig).Animation(0).LabelFormatter(format)
	settledChart(t, c, ggui.Sz(400, 240))
	formatted = 0
	c.Paint(nil, ggui.Rct(ggui.Point{}, ggui.Sz(400, 240)))
	if formatted != 6 {
		t.Fatalf("LabelFormatter ran %d times, want once per value (6)", formatted)
	}
	formatted = 0
	c.LabelPainter(func(*ggui.Canvas, ChartPointContext) { painted++ })
	c.Paint(nil, ggui.Rct(ggui.Point{}, ggui.Sz(400, 240)))
	if painted != 6 || formatted != 0 {
		t.Fatalf("with a LabelPainter: painter ran %d times and formatter %d, want 6 and 0", painted, formatted)
	}
}

func TestChartTickFormatterRewritesCategoryTicks(t *testing.T) {
	t.Parallel()
	var got []string
	c := AreaChart(chartTestData(), chartTestConfig).Animation(0).TickFormatter(func(s string) string {
		got = append(got, s)
		return s[:1]
	})
	settledChart(t, c, ggui.Sz(400, 240))
	got = nil
	c.Paint(nil, ggui.Rct(ggui.Point{}, ggui.Sz(400, 240)))
	if len(got) != 3 || got[0] != "Jan" || got[2] != "Mar" {
		t.Fatalf("TickFormatter saw %q, want every category label in order", got)
	}
	got = nil
	c.Axes(false, true)
	c.Paint(nil, ggui.Rct(ggui.Point{}, ggui.Sz(400, 240)))
	if len(got) != 0 {
		t.Fatalf("hidden x axis still formatted ticks %q", got)
	}
}

func TestChartRadarTickPainterRunsOncePerCategoryAroundTheRim(t *testing.T) {
	t.Parallel()
	data := append(chartTestData(), ChartDatum{Label: "Apr", Values: map[string]float64{"a": 3}})
	var ticks []ChartPointContext
	c := RadarChart(data, chartTestConfig).Animation(0).PolarGrid(ChartPolarGrid{Circle: true, Fill: true, Rings: 3}).
		TickPainter(func(_ *ggui.Canvas, ctx ChartPointContext) { ticks = append(ticks, ctx) })
	settledChart(t, c, ggui.Sz(300, 300))
	ticks = nil
	c.Paint(nil, ggui.Rct(ggui.Point{}, ggui.Sz(300, 300)))
	if len(ticks) != len(data) {
		t.Fatalf("TickPainter ran %d times, want once per category (%d)", len(ticks), len(data))
	}
	center, radius := c.polarBounds()
	for i, ctx := range ticks {
		if ctx.Index != i {
			t.Errorf("tick %d painted category %d", i, ctx.Index)
		}
		if d := math.Hypot(ctx.Position.X-center.X, ctx.Position.Y-center.Y); math.Abs(d-(radius+16)) > 1e-6 {
			t.Errorf("tick %d sits %v from the center, want just outside the rim (%v)", i, d, radius+16)
		}
		if got := c.hitIndex(polarPoint(center, radius/2, radarAngle(i, len(data)))); got != i {
			t.Errorf("the spoke of category %d hit-tests as %d", i, got)
		}
	}
}

func TestChartHorizontalBarsHitTestAlongTheVerticalAxis(t *testing.T) {
	t.Parallel()
	selected := -1
	c := BarChart(chartTestData(), chartTestConfig).Animation(0).Horizontal(true).OnSelect(func(i int, _ ChartDatum) { selected = i })
	p := settledChart(t, c, ggui.Sz(400, 300))
	band := c.plot.Size.H / 3
	for i := range 3 {
		at := ggui.Pt(c.plot.Origin.X+c.plot.Size.W/2, c.plot.Origin.Y+band*(float64(i)+.5))
		if got := c.hitIndex(at); got != i {
			t.Errorf("band %d hit-tests as %d", i, got)
		}
	}
	p.Click(ggui.Pt(c.plot.Origin.X+10, c.plot.Origin.Y+band*2.5))
	if selected != 2 {
		t.Fatalf("clicking the third band selected %d", selected)
	}
}

func TestChartDomainOverridesTheDataRangeOnlyWhenValid(t *testing.T) {
	t.Parallel()
	c := LineChart(chartTestData(), chartTestConfig).Domain(0, 100)
	c.geometryFor(ggui.Sz(400, 240))
	if c.geometry.lo != 0 || c.geometry.hi != 100 {
		t.Fatalf("Domain(0, 100) gave %v..%v", c.geometry.lo, c.geometry.hi)
	}
	// Mar's 20 sits a fifth of the way up a 0..100 domain.
	if y := c.geometry.series[0].top[2].Y; math.Abs(y-.2) > 1e-9 {
		t.Fatalf("20 of 0..100 normalized to %v, want 0.2", y)
	}
	for _, bad := range [][2]float64{{5, 1}, {3, 3}, {math.NaN(), 1}, {0, math.Inf(1)}} {
		c.Domain(bad[0], bad[1])
		c.geometryFor(ggui.Sz(400, 240))
		if c.geometry.lo > -6 || c.geometry.hi < 20 {
			t.Fatalf("invalid Domain%v was used: %v..%v does not cover the data", bad, c.geometry.lo, c.geometry.hi)
		}
	}
}

func TestChartSettersClampOutOfRangeValues(t *testing.T) {
	t.Parallel()
	c := PieChart(chartTestData(), chartTestConfig).OuterRadius(3).FillOpacity(2).CornerRadius(-4).StrokeWidth(-1).Height(-10)
	settledChart(t, c, ggui.Sz(300, 300))
	if _, radius := c.polarBounds(); radius != 150 {
		t.Fatalf("OuterRadius(3) gave radius %v, want the full 150", radius)
	}
	c.OuterRadius(0)
	if _, radius := c.polarBounds(); radius != 150*.05 {
		t.Fatalf("OuterRadius(0) gave radius %v, want the 5%% minimum", radius)
	}
	if c.seriesOpacity(0) != 1 || c.corner != 0 || c.stroke != 0 {
		t.Fatalf("opacity %v, corner %v, stroke %v; want 1, 0, 0", c.seriesOpacity(0), c.corner, c.stroke)
	}
	if size := c.Layout(ggui.Loose(ggui.Sz(300, 300)), ggui.Env{}); size.H != 0 {
		t.Fatalf("Height(-10) laid out %v tall, want 0", size.H)
	}
	c.Height(120)
	if size := c.Layout(ggui.Loose(ggui.Sz(300, 300)), ggui.Env{}); size.H != 120 {
		t.Fatalf("Height(120) laid out %v tall", size.H)
	}
}

func TestChartLegendTakesRoomFromThePlot(t *testing.T) {
	t.Parallel()
	plain := BarChart(chartTestData(), chartTestConfig).Animation(0)
	legend := BarChart(chartTestData(), chartTestConfig).Animation(0).Legend(true)
	settledChart(t, plain, ggui.Sz(400, 240))
	settledChart(t, legend, ggui.Sz(400, 240))
	if legend.plot.Size.H >= plain.plot.Size.H {
		t.Fatalf("plot is %v tall with a legend and %v without", legend.plot.Size.H, plain.plot.Size.H)
	}
}

func TestChartAnimationDelayHoldsTheEntrance(t *testing.T) {
	t.Parallel()
	c := LineChart(chartTestData(), chartTestConfig).Animation(time.Second).AnimationDelay(500 * time.Millisecond)
	now := time.Unix(100, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	defer restore()
	c.progress()
	now = now.Add(400 * time.Millisecond)
	if got := c.progress(); got != 0 {
		t.Fatalf("progress %v before the delay ran out, want 0", got)
	}
	now = now.Add(600 * time.Millisecond)
	if got := c.progress(); got <= 0 || got >= 1 {
		t.Fatalf("progress %v midway, want strictly between 0 and 1", got)
	}
	now = now.Add(time.Second)
	if got := c.progress(); got != 1 {
		t.Fatalf("progress %v after delay and duration, want 1", got)
	}
	c.AnimationDelay(-time.Second).Animation(-time.Second)
	c.Replay()
	if got := c.progress(); got != 1 {
		t.Fatalf("negative durations were not treated as none: progress %v", got)
	}
}

func TestChartDefaultTooltipIndexDescribesWithoutSelecting(t *testing.T) {
	t.Parallel()
	selected := -1
	c := BarChart(chartTestData(), chartTestConfig).Animation(0).DefaultTooltipIndex(2).OnSelect(func(i int, _ ChartDatum) { selected = i })
	settledChart(t, c, ggui.Sz(400, 240))
	d := c.Describe()
	if d.Now != 2 || d.Value != "Mar, Desktop: 20, Mobile: 8" {
		t.Fatalf("described %v %q, want category 2", d.Now, d.Value)
	}
	if selected != -1 || c.selected != -1 {
		t.Fatal("DefaultTooltipIndex selected a category")
	}
}

func TestChartEmptyChartIgnoresAssistiveActions(t *testing.T) {
	t.Parallel()
	c := LineChart(nil, chartTestConfig)
	for _, kind := range []ggui.ActionSet{ggui.ActionIncrement, ggui.ActionDecrement, ggui.ActionSetValue} {
		if c.Act(ggui.Action{Kind: kind, Num: 0}) {
			t.Errorf("empty chart accepted %v", kind)
		}
	}
	if d := c.Describe(); d.Value != "" || d.Max != 0 {
		t.Fatalf("empty chart described %q with max %v", d.Value, d.Max)
	}
}

func TestChartNameReachesSemantics(t *testing.T) {
	t.Parallel()
	c := AreaChart(chartTestData(), chartTestConfig).Name("Visitors")
	p := settledChart(t, c, ggui.Sz(400, 240))
	if _, ok := p.Semantics().Find(ggui.RoleGroup, "Visitors"); !ok {
		t.Fatalf("no group named Visitors:\n%s", p.Semantics())
	}
}

// TestChartEveryOptionPaintsFiniteGeometry paints each kind with every
// presentation option switched away from its default, including a selected
// and an active category, and checks the geometry stays finite.
func TestChartEveryOptionPaintsFiniteGeometry(t *testing.T) {
	t.Parallel()
	data := append(chartTestData(), ChartDatum{Label: "Apr", Values: map[string]float64{"a": 7, "b": math.NaN()}}, ChartDatum{Label: "May", Icon: "star", Values: map[string]float64{"a": 12, "b": 2}})
	for kind := ChartArea; kind <= ChartRadial; kind++ {
		for _, curve := range []ChartCurve{ChartNatural, ChartLinear, ChartStep, ChartMonotone} {
			for _, stack := range []ChartStack{ChartUnstacked, ChartStacked, ChartExpanded} {
				c := ChartContainer(kind, data, chartTestConfig).Curve(curve).Stack(stack).
					Grid(false).Axes(true, true).Legend(true).Dots(true).Labels(true).Gradient(true).
					Separators(false).TooltipCursor(false).CenterText("42", "total").ActiveRing(true).
					RadialTrack(false).InsideLabels(true).ActiveIndex(1).DefaultTooltipIndex(3).
					PolarGrid(ChartPolarGrid{HideSpokes: true, HideRings: true})
				settledChart(t, c, ggui.Sz(420, 300))
				c.Grid(true).Horizontal(true).Separators(true).RadialTrack(true).InsideLabels(false)
				c.Paint(nil, ggui.Rct(ggui.Point{}, ggui.Sz(420, 300)))
				for _, s := range c.geometry.series {
					for i := range s.top {
						if !finite(s.top[i].Y) || !finite(s.base[i].Y) {
							t.Fatalf("kind %v curve %v stack %v: point %d is not finite", kind, curve, stack, i)
						}
					}
				}
			}
		}
	}
}
