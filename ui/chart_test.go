package ui

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func chartTestData() []ChartDatum {
	return []ChartDatum{{Label: "Jan", Values: map[string]float64{"a": 10, "b": 5}}, {Label: "Feb", Values: map[string]float64{"a": -4, "b": -6}}, {Label: "Mar", Values: map[string]float64{"a": 20, "b": 8}}}
}

var chartTestConfig = ChartConfig{{Key: "a", Label: "Desktop"}, {Key: "b", Label: "Mobile"}}

func TestChartStackDomainAndSnapshot(t *testing.T) {
	t.Parallel()
	data := chartTestData()
	c := BarChart(data, chartTestConfig).Stack(ChartStacked)
	data[0].Values["a"] = 900
	c.geometryFor(ggui.Sz(400, 240))
	if c.geometry.lo != -10 || c.geometry.hi != 30 {
		t.Fatalf("domain %v..%v", c.geometry.lo, c.geometry.hi)
	}
	s := c.geometry.series[1]
	if got := s.top[0].Y; math.Abs(got-.625) > 1e-9 {
		t.Fatalf("stack top %v", got)
	}
	c.Stack(ChartExpanded)
	c.geometryFor(ggui.Sz(400, 240))
	if c.geometry.lo != -1 || c.geometry.hi != 1 {
		t.Fatal("expanded domain")
	}
	if c.geometry.series[1].top[0].Y != 1 || c.geometry.series[1].top[1].Y != 0 {
		t.Fatal("expanded totals")
	}
}
func TestChartEmptyMissingInvalidAndNarrow(t *testing.T) {
	t.Parallel()
	for kind := ChartArea; kind <= ChartRadial; kind++ {
		for _, data := range [][]ChartDatum{nil, {{Label: "only", Values: map[string]float64{"a": 0}}}, {{Values: map[string]float64{"a": math.NaN()}}, {Values: map[string]float64{"b": math.Inf(1)}}}} {
			c := ChartContainer(kind, data, chartTestConfig)
			p := ggui.NewProbe(c, ggui.Sz(1, 1))
			p.Frame()
			p.Resize(ggui.Sz(400, 240))
			p.Frame()
			p.Advance(2 * time.Second)
			p.Frame()
			p.Close()
			for _, s := range c.geometry.series {
				for _, point := range s.top {
					if !finite(point.X) || !finite(point.Y) {
						t.Fatalf("kind %v: nonfinite geometry", kind)
					}
				}
			}
		}
	}
}
func TestChartKeyboardPointerSelection(t *testing.T) {
	t.Parallel()
	selected := -1
	c := BarChart(chartTestData(), chartTestConfig).Animation(0).OnSelect(func(i int, _ ChartDatum) { selected = i })
	p := ggui.NewProbe(c, ggui.Sz(400, 240))
	defer p.Close()
	p.Frame()
	p.Click(c.plot.Origin.Add(ggui.Pt(20., 20.)))
	if c.active != 0 || selected != 0 {
		t.Fatalf("pointer: active=%d selected=%d", c.active, selected)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowRight)
	if c.active != 1 {
		t.Fatalf("keyboard active=%d", c.active)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if selected != 1 {
		t.Fatal("enter selection")
	}
	if c.Describe().Value != "Feb, Desktop: -4, Mobile: -6" {
		t.Fatal(c.Describe().Value)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	if c.active != 2 {
		t.Fatal("end")
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if c.active != -1 {
		t.Fatal("escape")
	}
}
func TestChartMotionAndReducedMotion(t *testing.T) {
	t.Parallel()
	c := AreaChart(chartTestData(), chartTestConfig)
	now := time.Unix(100, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	defer restore()
	if c.progress() != 0 {
		t.Fatal("entrance starts at zero")
	}
	now = now.Add(750 * time.Millisecond)
	if p := c.progress(); p < .75 || p > .85 {
		t.Fatalf("ease midpoint %v", p)
	}
	now = now.Add(time.Second)
	if c.progress() != 1 {
		t.Fatal("entrance completes")
	}
	c.Replay()
	if c.progress() != 0 {
		t.Fatal("replay")
	}
	c.reduced = true
	if c.progress() != 1 {
		t.Fatal("reduced motion")
	}
}
func TestChartDownsamplePreservesSpikesAndGaps(t *testing.T) {
	t.Parallel()
	points := make([]ggui.Point, 10000)
	valid := make([]bool, len(points))
	for i := range points {
		points[i] = ggui.Pt(float64(i)/9999, .5)
		valid[i] = true
	}
	points[2345].Y = 1
	points[2346].Y = 0
	valid[5999] = false
	samples := chartSamples(points, valid, 300)
	if len(samples) > 1205 {
		t.Fatalf("too many samples: %d", len(samples))
	}
	seen := map[int]bool{}
	last := -1
	for _, i := range samples {
		if i <= last {
			t.Fatal("not ordered")
		}
		seen[i] = true
		last = i
	}
	for _, i := range []int{0, 2345, 2346, 5999, 9999} {
		if !seen[i] {
			t.Fatalf("lost boundary/extremum %d", i)
		}
	}
}
func TestChartGeometryAndCurveCaches(t *testing.T) {
	t.Parallel()
	c := AreaChart(chartTestData(), chartTestConfig)
	p := ggui.NewProbe(c, ggui.Sz(400, 240))
	defer p.Close()
	p.Frame()
	p.Advance(2 * time.Second)
	p.Frame()
	old := &c.geometry.series[0].top[0]
	curve := &c.curves[0][0]
	p.Frame()
	if old != &c.geometry.series[0].top[0] || curve != &c.curves[0][0] {
		t.Fatal("unchanged frame rebuilt geometry")
	}
	c.Data([]ChartDatum{{Values: map[string]float64{"a": 2}}, {Values: map[string]float64{"a": 3}}})
	p.Frame()
	if c.geometry.hi < 3 || old == &c.geometry.series[0].top[0] {
		t.Fatal("data invalidation")
	}
	p.Resize(ggui.Sz(600, 240))
	p.Frame()
	if c.cacheSize.W != 600 {
		t.Fatal("resize invalidation")
	}
}
func TestChartPolarHitTesting(t *testing.T) {
	t.Parallel()
	c := PieChart([]ChartDatum{{Values: map[string]float64{"a": 1}}, {Values: map[string]float64{"a": 1}}}, chartTestConfig[:1]).InnerRadius(.5).Animation(0)
	p := ggui.NewProbe(c, ggui.Sz(300, 300))
	defer p.Close()
	p.Frame()
	center, radius := c.polarBounds()
	if c.hitIndex(center) != -1 {
		t.Fatal("donut hole hit")
	}
	if c.hitIndex(polarPoint(center, radius*.75, 90)) != 0 || c.hitIndex(polarPoint(center, radius*.75, 270)) != 1 {
		t.Fatal("sectors")
	}
	c.Angles(180, 0)
	if c.hitIndex(polarPoint(center, radius*.75, 270)) != -1 {
		t.Fatal("outside semicircle")
	}
}
func BenchmarkChartCachedGeometry100K(b *testing.B) {
	data := make([]ChartDatum, 100000)
	for i := range data {
		data[i] = ChartDatum{Values: map[string]float64{"a": math.Sin(float64(i))}}
	}
	c := LineChart(data, chartTestConfig[:1])
	size := ggui.Sz(800, 240)
	c.geometryFor(size)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.geometryFor(size)
	}
}
func BenchmarkChartCachedPaint100K(b *testing.B) {
	data := make([]ChartDatum, 100000)
	for i := range data {
		data[i] = ChartDatum{Values: map[string]float64{"a": math.Sin(float64(i))}}
	}
	c := LineChart(data, chartTestConfig[:1]).Animation(0).Axes(false, false)
	p := ggui.NewProbe(c, ggui.Sz(800, 240))
	defer p.Close()
	p.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.Paint(nil, ggui.Rct(ggui.Point{}, ggui.Sz(800, 240)))
	}
}

func TestChartTooltipFormatsTotalAndLegendWrap(t *testing.T) {
	t.Parallel()
	c := BarChart(chartTestData(), chartTestConfig).Tooltip(ChartTooltipOptions{ShowTotal: true, Label: "Total visits in the selected month", FormatValue: func(v float64, _ ChartSeries, _ ChartDatum) string { return fmt.Sprintf("%.0f visitors", v) }})
	c.Layout(ggui.Loose(ggui.Sz(400, 240)), uitheme.Default().Apply(ggui.Env{}))
	rows, size := c.tooltipRows(0)
	if len(rows) != 2 || rows[0].value != "10 visitors" || size.H != 112 {
		t.Fatalf("tooltip rows/size %v %v", rows, size)
	}
	if c.legendHeight(70) <= c.legendHeight(400) {
		t.Fatal("legend did not wrap")
	}
	standalone := ChartTooltipContent(c, 0)
	dark := uitheme.Dark()
	standalone.Layout(ggui.Loose(ggui.Sz(400, 240)), dark.Apply(ggui.Env{}))
	if c.theme.Bg == dark.Bg {
		t.Fatal("standalone content changed chart's theme")
	}
}
func TestChartMotionThroughProbe(t *testing.T) {
	t.Parallel()
	c := LineChart(chartTestData(), chartTestConfig)
	p := ggui.NewProbe(c, ggui.Sz(400, 240))
	defer p.Close()
	p.Frame()
	p.Advance(150 * time.Millisecond)
	early := c.progress()
	p.Advance(600 * time.Millisecond)
	middle := c.progress()
	p.Advance(850 * time.Millisecond)
	if !(early > 0 && early < middle && middle < 1 && c.progress() == 1) {
		t.Fatalf("motion %v %v %v", early, middle, c.progress())
	}
	reduced := AreaChart(chartTestData(), chartTestConfig)
	r := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, reduced), ggui.Sz(400, 240))
	defer r.Close()
	r.Frame()
	if reduced.progress() != 1 {
		t.Fatal("reduced motion env ignored")
	}
}
func TestChartTooltipAndLegendEmpty(t *testing.T) {
	t.Parallel()
	c := PieChart(nil, nil)
	p := ggui.NewProbe(ggui.Column(ChartLegend(c), ChartTooltip(c, 99)), ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
}
func BenchmarkChartPrepare100K(b *testing.B) {
	data := make([]ChartDatum, 100000)
	for i := range data {
		data[i] = ChartDatum{Values: map[string]float64{"a": math.Sin(float64(i))}}
	}
	c := LineChart(data, chartTestConfig[:1])
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.revision++
		c.geometryFor(ggui.Sz(800, 240))
	}
}

func TestChartAssistiveActions(t *testing.T) {
	t.Parallel()
	c := LineChart(chartTestData(), chartTestConfig)
	if !c.Act(ggui.Action{Kind: ggui.ActionIncrement}) || c.active != 0 {
		t.Fatal("increment")
	}
	if !c.Act(ggui.Action{Kind: ggui.ActionSetValue, Num: 2}) || c.active != 2 {
		t.Fatal("set value")
	}
	if !c.Act(ggui.Action{Kind: ggui.ActionDecrement}) || c.active != 1 {
		t.Fatal("decrement")
	}
	if c.Act(ggui.Action{Kind: ggui.ActionSetValue, Num: math.NaN()}) {
		t.Fatal("accepted nonfinite index")
	}
}

func TestChartStackSamplingPreservesBaselineSpikes(t *testing.T) {
	t.Parallel()
	data := make([]ChartDatum, 10000)
	for i := range data {
		data[i] = ChartDatum{Values: map[string]float64{"a": 1, "b": 9}}
	}
	data[4567].Values["a"] = 9
	data[4567].Values["b"] = 1
	c := AreaChart(data, chartTestConfig).Stack(ChartStacked)
	c.geometryFor(ggui.Sz(300, 240))
	found := false
	for _, i := range c.geometry.series[1].indices {
		if i == 4567 {
			found = true
		}
	}
	if !found {
		t.Fatal("flat total erased the stacked baseline spike")
	}
}
func TestChartReferenceAnimationTimings(t *testing.T) {
	t.Parallel()
	bar := BarChart(chartTestData(), chartTestConfig)
	pie := PieChart(chartTestData(), chartTestConfig[:1])
	if bar.duration != 400*time.Millisecond || pie.delay != 400*time.Millisecond {
		t.Fatal("reference timing defaults")
	}
	now := time.Unix(100, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	defer restore()
	pie.progress()
	now = now.Add(200 * time.Millisecond)
	if pie.progress() != 0 {
		t.Fatal("pie started before delay")
	}
	now = now.Add(2 * time.Second)
	if pie.progress() != 1 {
		t.Fatal("pie did not complete")
	}
}

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

// fuzzChartValues decodes raw into up to 64 values of one magnitude: each
// little-endian int16 scaled by 10^exp, with -32768 standing for a gap.
func fuzzChartValues(raw []byte, exp int8) []float64 {
	scale := math.Pow(10, float64(int(exp)%10))
	var out []float64
	for len(raw) >= 2 && len(out) < 64 {
		v := int16(binary.LittleEndian.Uint16(raw))
		raw = raw[2:]
		if v == math.MinInt16 {
			out = append(out, math.NaN())
			continue
		}
		out = append(out, float64(v)*scale)
	}
	return out
}

// FuzzChartDomainCoversData checks the automatic value axis over random data:
// the domain is finite, contains zero and every stacked extent, is at most
// twice as wide as the data, and every normalized point lies inside it.
func FuzzChartDomainCoversData(f *testing.F) {
	f.Add([]byte{10, 0, 5, 0, 0xfc, 0xff}, uint8(0), int8(0))
	f.Add([]byte{1, 0, 1, 0, 1, 0, 1, 0}, uint8(1), int8(-3))
	f.Add([]byte{0xff, 0x7f, 0x00, 0x80, 0x01, 0x80}, uint8(2), int8(9))
	f.Add([]byte{0, 0, 0, 0}, uint8(0), int8(4))
	f.Add([]byte{3, 0}, uint8(1), int8(-9))
	f.Fuzz(func(t *testing.T, raw []byte, stack uint8, exp int8) {
		values := fuzzChartValues(raw, exp)
		data := make([]ChartDatum, (len(values)+1)/2)
		for i, v := range values {
			if data[i/2].Values == nil {
				data[i/2].Values = map[string]float64{}
			}
			data[i/2].Values[chartTestConfig[i%2].Key] = v
		}
		c := LineChart(data, chartTestConfig).Stack(ChartStack(stack % 3))
		c.geometryFor(ggui.Sz(300, 200))
		g := c.geometry
		if !finite(g.lo) || !finite(g.hi) || g.lo >= g.hi {
			t.Fatalf("domain %v..%v is not a finite, increasing range", g.lo, g.hi)
		}
		if g.lo > 0 || g.hi < 0 {
			t.Fatalf("domain %v..%v leaves out the zero baseline", g.lo, g.hi)
		}
		// The data's own extent, with the stacking the chart applies.
		lo, hi := 0., 0.
		for i := range data {
			pos, neg, sum := 0., 0., 0.
			for j := range chartTestConfig {
				if v, ok := c.value(i, j); ok {
					sum += math.Abs(v)
				}
			}
			for j := range chartTestConfig {
				v, ok := c.value(i, j)
				if !ok {
					continue
				}
				if c.stack == ChartExpanded && sum > 0 {
					v /= sum
				}
				switch {
				case c.stack == ChartUnstacked:
					lo, hi = min(lo, v), max(hi, v)
				case v >= 0:
					pos += v
					hi = max(hi, pos)
				default:
					neg += v
					lo = min(lo, neg)
				}
			}
		}
		if c.stack != ChartExpanded && hi > lo && g.hi-g.lo > 2*(hi-lo)*(1+1e-9) {
			t.Fatalf("domain %v..%v is more than twice the data's %v..%v", g.lo, g.hi, lo, hi)
		}
		for j, s := range g.series {
			for i := range s.top {
				if !s.valid[i] {
					continue
				}
				for _, y := range []float64{s.top[i].Y, s.base[i].Y} {
					if !(y >= -1e-9 && y <= 1+1e-9) {
						t.Fatalf("series %d point %d normalized to %v, outside the domain", j, i, y)
					}
				}
			}
		}
	})
}

// FuzzNiceChartStepRoundsUpToAFriendlyNumber checks the axis step is the
// smallest of 1, 2, 2.5, 5 or 10 times a power of ten that is not below v.
func FuzzNiceChartStepRoundsUpToAFriendlyNumber(f *testing.F) {
	for _, v := range []float64{1, 1.25, 2, 2.4, 3, 7.5, 0.003, 12345, 1e-9, 9.99e12} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v float64) {
		v = math.Abs(v)
		if !finite(v) || v < 1e-200 || v > 1e200 {
			t.Skip()
		}
		step := niceChartStep(v)
		if step < v*(1-1e-12) || step > 2*v*(1+1e-12) {
			t.Fatalf("niceChartStep(%v) = %v, want within [v, 2v]", v, step)
		}
		mantissa := step / math.Pow(10, math.Floor(math.Log10(step)+1e-12))
		ok := false
		for _, m := range []float64{1, 2, 2.5, 5, 10} {
			ok = ok || math.Abs(mantissa-m) < 1e-9*m
		}
		if !ok {
			t.Fatalf("niceChartStep(%v) = %v, whose mantissa %v is not 1, 2, 2.5 or 5", v, step, mantissa)
		}
	})
}

// FuzzChartSamplesKeepOrderAndExtrema checks downsampling: indices are
// strictly increasing and in range, keep both ends, every gap boundary, and
// each pixel column's lowest and highest point.
func FuzzChartSamplesKeepOrderAndExtrema(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18}, uint8(1))
	f.Add([]byte{0, 255, 0, 255, 0, 255, 0, 255, 0, 255, 0, 255, 0, 255, 0, 255, 0, 255}, uint8(2))
	f.Add([]byte{9}, uint8(0))
	f.Add([]byte{}, uint8(3))
	f.Fuzz(func(t *testing.T, raw []byte, w uint8) {
		width := int(w%8) + 1
		n := len(raw)
		points := make([]ggui.Point, n)
		valid := make([]bool, n)
		for i, b := range raw {
			x := 0.
			if n > 1 {
				x = float64(i) / float64(n-1)
			}
			points[i] = ggui.Pt(x, float64(b&0x7f)/127)
			valid[i] = b&0x80 == 0
		}
		samples := chartSamples(points, valid, width)
		kept := make([]bool, n)
		for k, i := range samples {
			if i < 0 || i >= n || k > 0 && i <= samples[k-1] {
				t.Fatalf("samples %v are not strictly increasing indices below %d", samples, n)
			}
			kept[i] = true
		}
		if n == 0 {
			return
		}
		if !kept[0] || !kept[n-1] {
			t.Fatalf("samples %v dropped an end of %d points", samples, n)
		}
		column := func(i int) int { return int(points[i].X * float64(width)) }
		for start := 0; start < n; {
			end := start + 1
			for end < n && column(end) == column(start) && valid[end] == valid[start] {
				end++
			}
			if !kept[start] || !kept[end-1] {
				t.Fatalf("run %d..%d lost a boundary: samples %v", start, end-1, samples)
			}
			lowest, highest := points[start].Y, points[start].Y
			for i := start; i < end; i++ {
				lowest, highest = min(lowest, points[i].Y), max(highest, points[i].Y)
			}
			keptLow, keptHigh := false, false
			for i := start; i < end; i++ {
				keptLow = keptLow || kept[i] && points[i].Y == lowest
				keptHigh = keptHigh || kept[i] && points[i].Y == highest
			}
			if !keptLow || !keptHigh {
				t.Fatalf("run %d..%d lost its extremes %v..%v: samples %v", start, end-1, lowest, highest, samples)
			}
			start = end
		}
	})
}

// fuzzCurvePoints decodes raw into points with strictly increasing x.
func fuzzCurvePoints(raw []byte) []ggui.Point {
	var points []ggui.Point
	x := 0.
	for len(raw) >= 2 && len(points) < 40 {
		x += 1 + float64(raw[0]%16)
		points = append(points, ggui.Pt(x, float64(int8(raw[1]))))
		raw = raw[2:]
	}
	return points
}

// FuzzChartNaturalSplineIsSmooth checks naturalControls against the natural
// cubic spline's defining conditions: at every interior point the joining
// Bezier segments agree in first and second derivative, and the second
// derivative vanishes at both ends.
func FuzzChartNaturalSplineIsSmooth(f *testing.F) {
	f.Add([]byte{0, 0, 3, 10, 1, 250, 7, 4})
	f.Add([]byte{1, 1, 1, 1, 1, 1})
	f.Add([]byte{15, 127, 0, 128, 15, 127, 0, 128, 15, 127})
	f.Fuzz(func(t *testing.T, raw []byte) {
		points := fuzzCurvePoints(raw)
		n := len(points)
		if n < 3 {
			t.Skip()
		}
		control := naturalControls(points)
		// Segment i runs points[i], a[i], b[i], points[i+1].
		a := control
		b := make([]ggui.Point, n-1)
		for i := range n - 2 {
			b[i] = ggui.Pt(2*points[i+1].X-control[i+1].X, 2*points[i+1].Y-control[i+1].Y)
		}
		b[n-2] = ggui.Pt((points[n-1].X+control[n-2].X)/2, (points[n-1].Y+control[n-2].Y)/2)
		const tol = 1e-6
		near := func(p, q ggui.Point, what string, i int) {
			scale := 1 + math.Abs(p.X) + math.Abs(p.Y)
			if math.Abs(p.X-q.X) > tol*scale || math.Abs(p.Y-q.Y) > tol*scale {
				t.Fatalf("%s at point %d: %v != %v (points %v)", what, i, p, q, points)
			}
		}
		for _, p := range control {
			if !finite(p.X) || !finite(p.Y) {
				t.Fatalf("non-finite control point for %v", points)
			}
		}
		for i := 1; i < n-1; i++ {
			// C1: b[i-1], points[i], a[i] are collinear and evenly spaced.
			near(ggui.Pt(points[i].X-b[i-1].X, points[i].Y-b[i-1].Y), ggui.Pt(a[i].X-points[i].X, a[i].Y-points[i].Y), "first derivative", i)
			// C2: a[i-1] - 2b[i-1] + p[i] == p[i] - 2a[i] + b[i].
			left := ggui.Pt(a[i-1].X-2*b[i-1].X+points[i].X, a[i-1].Y-2*b[i-1].Y+points[i].Y)
			right := ggui.Pt(points[i].X-2*a[i].X+b[i].X, points[i].Y-2*a[i].Y+b[i].Y)
			near(left, right, "second derivative", i)
		}
		near(ggui.Pt(points[0].X-2*a[0].X+b[0].X, points[0].Y-2*a[0].Y+b[0].Y), ggui.Point{}, "free start", 0)
		near(ggui.Pt(a[n-2].X-2*b[n-2].X+points[n-1].X, a[n-2].Y-2*b[n-2].Y+points[n-1].Y), ggui.Point{}, "free end", n-1)
	})
}

// FuzzChartMonotoneCurveNeverOvershoots traces the monotone curve and checks
// that between two observations it never leaves the band they span.
func FuzzChartMonotoneCurveNeverOvershoots(f *testing.F) {
	f.Add([]byte{0, 0, 3, 10, 1, 250, 7, 4})
	f.Add([]byte{2, 1, 2, 2, 2, 100, 2, 101, 2, 0})
	f.Add([]byte{0, 5, 0, 5})
	f.Fuzz(func(t *testing.T, raw []byte) {
		points := fuzzCurvePoints(raw)
		if len(points) < 2 {
			t.Skip()
		}
		var path ggui.Path
		var traced []ggui.Point
		curvePath(&path, points, ChartMonotone, true, func(p ggui.Point) { traced = append(traced, p) })
		segment := 0
		for _, p := range traced {
			if !finite(p.X) || !finite(p.Y) {
				t.Fatalf("non-finite trace point %v for %v", p, points)
			}
			for segment < len(points)-2 && p.X > points[segment+1].X {
				segment++
			}
			q, r := points[segment], points[segment+1]
			if p.Y < min(q.Y, r.Y)-1e-9 || p.Y > max(q.Y, r.Y)+1e-9 {
				t.Fatalf("curve reached %v between %v and %v", p, q, r)
			}
		}
	})
}

// FuzzChartPolarHitMatchesPaint lays out random pie and radar charts and
// checks that the middle of every painted sector or spoke hit-tests as the
// category painted there.
func FuzzChartPolarHitMatchesPaint(f *testing.F) {
	f.Add([]byte{1, 2, 3}, int16(0), int16(360), uint8(0))
	f.Add([]byte{5, 0, 5, 200, 7}, int16(90), int16(-270), uint8(40))
	f.Add([]byte{1, 1, 1, 1, 1, 1, 1}, int16(-180), int16(180), uint8(90))
	f.Add([]byte{3}, int16(45), int16(45), uint8(10))
	f.Fuzz(func(t *testing.T, raw []byte, start, end int16, inner uint8) {
		if len(raw) == 0 || len(raw) > 48 {
			t.Skip()
		}
		data := make([]ChartDatum, len(raw))
		for i, b := range raw {
			v := float64(b)
			if b >= 200 {
				v = -v // a non-positive value takes no sector
			}
			data[i] = ChartDatum{Values: map[string]float64{"a": v}}
		}
		config := chartTestConfig[:1]
		pie := PieChart(data, config).Angles(float64(start), float64(end)).InnerRadius(float64(inner%95) / 100)
		pie.plot = ggui.Rct(ggui.Point{}, ggui.Sz(300, 300))
		center, radius := pie.polarBounds()
		mid := radius * (1 + pie.inner) / 2
		total := pie.positiveTotal(0)
		angle := pie.startAngle
		for i := range data {
			v, ok := pie.value(i, 0)
			if !ok || v <= 0 {
				continue
			}
			sweep := (pie.endAngle - pie.startAngle) * v / total
			if math.Abs(sweep) > 1e-6 {
				if got := pie.hitIndex(polarPoint(center, mid, angle+sweep/2)); got != i {
					t.Fatalf("middle of sector %d (%v..%v degrees) hit-tests as %d", i, angle, angle+sweep, got)
				}
			}
			angle += sweep
		}
		radar := RadarChart(data, config)
		radar.plot = pie.plot
		for i := range data {
			if got := radar.hitIndex(polarPoint(center, radius/2, radarAngle(i, len(data)))); got != i {
				t.Fatalf("spoke %d of %d hit-tests as %d", i, len(data), got)
			}
		}
	})
}

func TestChartPieHitTestAcceptsAnyStartAngle(t *testing.T) {
	t.Parallel()
	data := []ChartDatum{{Values: map[string]float64{"a": 1}}, {Values: map[string]float64{"a": 1}}}
	c := PieChart(data, chartTestConfig[:1]).Angles(900, 1260)
	c.plot = ggui.Rct(ggui.Point{}, ggui.Sz(300, 300))
	center, radius := c.polarBounds()
	// The second sector spans 1080..1260 degrees; its middle, 1170, points straight up.
	if got := c.hitIndex(polarPoint(center, radius/2, 1170)); got != 1 {
		t.Fatalf("middle of the second sector hit-tests as %d, want 1", got)
	}
}
