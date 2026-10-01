package ui

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/ironpark/ggui"
)

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
