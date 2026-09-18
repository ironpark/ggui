package ui

import (
	"fmt"
	"github.com/ironpark/ggui"
	"math"
	"testing"
	"time"
)

func chartTestData() []ChartDatum {
	return []ChartDatum{{Label: "Jan", Values: map[string]float64{"a": 10, "b": 5}}, {Label: "Feb", Values: map[string]float64{"a": -4, "b": -6}}, {Label: "Mar", Values: map[string]float64{"a": 20, "b": 8}}}
}

var chartTestConfig = ChartConfig{{Key: "a", Label: "Desktop"}, {Key: "b", Label: "Mobile"}}

func TestChartStackDomainAndSnapshot(t *testing.T) {
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
	c := BarChart(chartTestData(), chartTestConfig).Tooltip(ChartTooltipOptions{ShowTotal: true, Label: "Total visits in the selected month", FormatValue: func(v float64, _ ChartSeries, _ ChartDatum) string { return fmt.Sprintf("%.0f visitors", v) }})
	c.Layout(ggui.Loose(ggui.Sz(400, 240)), ggui.Env{}.WithTheme(ggui.DefaultTheme()))
	rows, size := c.tooltipRows(0)
	if len(rows) != 2 || rows[0].value != "10 visitors" || size.H != 112 {
		t.Fatalf("tooltip rows/size %v %v", rows, size)
	}
	if c.legendHeight(70) <= c.legendHeight(400) {
		t.Fatal("legend did not wrap")
	}
	standalone := ChartTooltipContent(c, 0)
	dark := ggui.DarkTheme()
	standalone.Layout(ggui.Loose(ggui.Sz(400, 240)), ggui.Env{}.WithTheme(dark))
	if c.theme.Bg == dark.Bg {
		t.Fatal("standalone content changed chart's theme")
	}
}
func TestChartMotionThroughProbe(t *testing.T) {
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
