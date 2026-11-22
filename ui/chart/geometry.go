package chart

import (
	"math"

	"github.com/ironpark/ggui"
)

type chartGeometry struct {
	lo, hi float64
	series []chartSeriesGeometry
}
type chartSeriesGeometry struct {
	top, base []ggui.Point
	valid     []bool
	indices   []int
}

// stackLevels is where each category's next stacked value starts, above zero
// for a positive value and below it for a negative one, and the magnitude an
// expanded stack divides each category by.
type stackLevels struct {
	pos, neg, sums []float64
}

// push stacks v on category i and returns the level it starts from.
func (l stackLevels) push(i int, v float64) float64 {
	if v >= 0 {
		base := l.pos[i]
		l.pos[i] += v
		return base
	}
	base := l.neg[i]
	l.neg[i] += v
	return base
}

// geometryFor prepares normalized points once per data/size change. Paint only
// transforms those points; input lookup never scans Cartesian data.
func (c *Widget) geometryFor(size ggui.Size) {
	if c.cacheSize == size && c.cacheRevision == c.revision {
		return
	}
	c.curves = nil
	c.cacheSize = size
	c.cacheRevision = c.revision
	n := len(c.data)
	g := chartGeometry{lo: 0, hi: 0, series: make([]chartSeriesGeometry, len(c.config))}
	levels := stackLevels{pos: make([]float64, n), neg: make([]float64, n), sums: make([]float64, n)}
	if c.stack == StackExpanded {
		c.sumMagnitudes(levels.sums)
	}
	for j := range c.config {
		g.series[j] = c.seriesGeometry(j, levels, &g)
	}
	g.lo, g.hi = c.valueDomain(g.lo, g.hi)
	g.normalize()
	pixels := size.W
	if c.horizontal {
		pixels = size.H
	}
	buckets := int(max(1, pixels))
	for j := range g.series {
		s := &g.series[j]
		s.indices = chartSamples(s.top, s.valid, buckets)
		if c.stack != StackUnstacked {
			s.indices = mergeChartSamples(s.indices, chartSamples(s.base, s.valid, buckets))
		}
	}
	c.geometry = g
}

// sumMagnitudes adds each category's total magnitude across the series to
// sums, which an expanded stack divides its values by.
func (c *Widget) sumMagnitudes(sums []float64) {
	for i := range c.data {
		for j := range c.config {
			if v, ok := c.value(i, j); ok {
				sums[i] += math.Abs(v)
			}
		}
	}
}

// seriesGeometry places series j's values on their categories, stacked on
// levels, and widens g's lo and hi to cover them.
func (c *Widget) seriesGeometry(j int, levels stackLevels, g *chartGeometry) chartSeriesGeometry {
	n := len(c.data)
	s := chartSeriesGeometry{top: make([]ggui.Point, n), base: make([]ggui.Point, n), valid: make([]bool, n)}
	for i := range c.data {
		v, ok := c.value(i, j)
		s.valid[i] = ok
		if !ok {
			continue
		}
		if c.stack == StackExpanded && levels.sums[i] > 0 {
			v /= levels.sums[i]
		}
		base := 0.
		if c.stack != StackUnstacked {
			base = levels.push(i, v)
		}
		g.lo = min(g.lo, base, base+v)
		g.hi = max(g.hi, base, base+v)
		x := c.categoryFraction(i)
		s.top[i] = ggui.Pt(x, base+v)
		s.base[i] = ggui.Pt(x, base)
	}
	return s
}

// valueDomain is the value range the plot spans, given the data's lo and hi:
// the domain set on the chart, the unit range of an expanded stack, or the
// data's range widened to round steps.
func (c *Widget) valueDomain(lo, hi float64) (float64, float64) {
	switch {
	case c.domainSet:
		lo, hi = c.domainMin, c.domainMax
	case c.stack == StackExpanded:
		if hi > 0 {
			hi = 1
		}
		if lo < 0 {
			lo = -1
		}
	case hi > lo:
		// Four equal, round-number intervals match the reference axes.
		step := niceChartStep(hi/4 - lo/4)
		roundLo := math.Floor(lo/step) * step
		roundHi := math.Ceil(hi/step) * step
		if finite(roundLo) && finite(roundHi) && roundHi > roundLo {
			lo, hi = roundLo, roundHi
		}
	}
	if hi <= lo {
		hi = lo + 1
	}
	return lo, hi
}

// normalize maps every point's value onto 0..1 across lo..hi, halving both
// first where the span itself overflows.
func (g *chartGeometry) normalize() {
	span := g.hi - g.lo
	halfSpan := g.hi/2 - g.lo/2
	for j := range g.series {
		s := &g.series[j]
		for i := range s.top {
			if math.IsInf(span, 0) {
				s.top[i].Y = (s.top[i].Y/2 - g.lo/2) / halfSpan
				s.base[i].Y = (s.base[i].Y/2 - g.lo/2) / halfSpan
			} else {
				s.top[i].Y = (s.top[i].Y - g.lo) / span
				s.base[i].Y = (s.base[i].Y - g.lo) / span
			}
		}
	}
}
func niceChartStep(v float64) float64 {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 1
	}
	p := math.Pow(10, math.Floor(math.Log10(v)))
	if p <= 0 || math.IsNaN(p) || math.IsInf(p, 0) {
		return v
	}
	f := v / p
	switch {
	case f <= 1:
		return p
	case f <= 2:
		return 2 * p
	case f <= 2.5:
		return 2.5 * p
	case f <= 5:
		return 5 * p
	default:
		if step := 10 * p; !math.IsInf(step, 0) {
			return step
		}
		return v
	}
}

// chartSamples keeps extrema in each pixel column, in source order, including
// gap boundaries. Large series are bounded by display resolution without losing
// isolated spikes. The original observations remain available to tooltips.
func chartSamples(points []ggui.Point, valid []bool, width int) []int {
	n := len(points)
	if n <= width*4 {
		r := make([]int, n)
		for i := range r {
			r[i] = i
		}
		return r
	}
	out := make([]int, 0, width*4+2)
	for start := 0; start < n; {
		end := start + 1
		bucket := int(points[start].X * float64(width))
		for end < n && int(points[end].X*float64(width)) == bucket && valid[end] == valid[start] {
			end++
		}
		early, late := columnExtrema(points[start:end])
		last := -1
		for _, i := range []int{start, start + early, start + late, end - 1} {
			if i != last {
				out = append(out, i)
				last = i
			}
		}
		start = end
	}
	return out
}

// columnExtrema returns the indices of the lowest and the highest of points,
// the earlier one first.
func columnExtrema(points []ggui.Point) (int, int) {
	low, high := 0, 0
	for i := 1; i < len(points); i++ {
		if points[i].Y < points[low].Y {
			low = i
		}
		if points[i].Y > points[high].Y {
			high = i
		}
	}
	return min(low, high), max(low, high)
}
func chartPosition(r ggui.Rect, p ggui.Point) ggui.Point {
	return ggui.Pt(r.Origin.X+p.X*r.Size.W, r.Origin.Y+(1-p.Y)*r.Size.H)
}
func polarPoint(center ggui.Point, radius, angle float64) ggui.Point {
	a := angle * math.Pi / 180
	return center.Add(ggui.Pt(radius*math.Cos(a), -radius*math.Sin(a)))
}

// circlePoints samples a full circle as a closed polyline. The radial grid and
// the radar rings share it so both draw at one resolution.
func circlePoints(center ggui.Point, radius float64) []ggui.Point {
	const steps = 96
	points := make([]ggui.Point, steps+1)
	for i := range points {
		points[i] = polarPoint(center, radius, float64(i)*360/steps)
	}
	return points
}

// radarAngle is the angle of the i'th of n radar spokes: the first points
// straight up and the rest run clockwise. hitIndex inverts it.
func radarAngle(i, n int) float64 { return 90 - float64(i)*360/float64(n) }

// positiveTotal sums the positive values of one series. Pie and radial share it
// with the hit test, so painting and hovering agree on what counts.
func (c *Widget) positiveTotal(j int) float64 {
	total := 0.
	for i := range c.data {
		if v, ok := c.value(i, j); ok && v > 0 {
			total += v
		}
	}
	return total
}

// categoryFraction maps a category index to its 0..1 position across the plot.
// Bars sit at the center of their band; every other kind sits on the gridline.
// categoryIndex inverts it, so geometry, axis ticks and hit-testing cannot drift.
func (c *Widget) categoryFraction(i int) float64 {
	n := len(c.data)
	if c.kind == KindBar {
		return (float64(i) + .5) / float64(n)
	}
	if n > 1 {
		return float64(i) / float64(n-1)
	}
	return .5
}

func (c *Widget) categoryIndex(f float64) int {
	n := len(c.data)
	i := int(math.Round(f * float64(n-1)))
	if c.kind == KindBar {
		i = int(f * float64(n))
	}
	return min(n-1, max(0, i))
}

// polarFrame is the disc a polar chart paints in and hit-tests against: its
// center and outer radius, and the radius of the hole at its middle.
type polarFrame struct {
	center        ggui.Point
	radius, inner float64
}

func (c *Widget) polarBounds() (ggui.Point, float64) {
	return c.plot.Center(), max(0, min(c.plot.Size.W, c.plot.Size.H)/2*c.outer)
}
func (c *Widget) polarFrame() polarFrame {
	center, radius := c.polarBounds()
	return polarFrame{center: center, radius: radius, inner: radius * c.inner}
}
func (c *Widget) hitIndex(p ggui.Point) int {
	if !c.plot.Contains(p) || len(c.data) == 0 || len(c.config) == 0 {
		return -1
	}
	if c.kind.polar() {
		return c.polarHitIndex(p)
	}
	f := (p.X - c.plot.Origin.X) / c.plot.Size.W
	if c.horizontal {
		f = (p.Y - c.plot.Origin.Y) / c.plot.Size.H
	}
	return c.categoryIndex(f)
}

// polarHitIndex is the category under p on a polar chart: a radar's nearest
// spoke, or the slice or band p falls in, or -1 outside them.
func (c *Widget) polarHitIndex(p ggui.Point) int {
	n := len(c.data)
	frame := c.polarFrame()
	dx, dy := p.X-frame.center.X, frame.center.Y-p.Y
	distance := math.Hypot(dx, dy)
	if distance > frame.radius+10 {
		return -1
	}
	angle := math.Atan2(dy, dx) * 180 / math.Pi
	if c.kind == KindRadar {
		return int(math.Round(math.Mod(90-angle+360, 360)/360*float64(n))) % n
	}
	if distance < frame.inner {
		return -1
	}
	sweep := c.endAngle - c.startAngle
	if sweep == 0 {
		return -1
	}
	offset := math.Mod((angle-c.startAngle)*math.Copysign(1, sweep), 360)
	if offset < 0 {
		offset += 360
	}
	if offset > math.Abs(sweep) {
		return -1
	}
	if c.kind == KindRadial && c.stack == StackUnstacked {
		return min(n-1, max(0, int((distance-frame.inner)/(frame.radius*(1-c.inner))*float64(n))))
	}
	return c.sliceAt(offset / math.Abs(sweep))
}

// sliceAt is the category whose slice of the first series covers the share f
// of the series' positive total, or -1.
func (c *Widget) sliceAt(f float64) int {
	target := f * c.positiveTotal(0)
	sum := 0.
	for i := range c.data {
		v, ok := c.value(i, 0)
		if !ok || v <= 0 {
			continue
		}
		sum += v
		if target < sum {
			return i
		}
	}
	return -1
}

func mergeChartSamples(a, b []int) []int {
	out := make([]int, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		v := 0
		if j == len(b) || (i < len(a) && a[i] < b[j]) {
			v = a[i]
			i++
		} else {
			v = b[j]
			j++
		}
		if len(out) == 0 || out[len(out)-1] != v {
			out = append(out, v)
		}
	}
	return out
}
