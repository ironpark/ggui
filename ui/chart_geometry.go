package ui

import (
	"github.com/ironpark/ggui"
	"math"
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

// geometryFor prepares normalized points once per data/size change. Paint only
// transforms those points; input lookup never scans Cartesian data.
func (c *ChartWidget) geometryFor(size ggui.Size) {
	if c.cacheSize == size && c.cacheRevision == c.revision {
		return
	}
	c.curves = nil
	c.cacheSize = size
	c.cacheRevision = c.revision
	n := len(c.data)
	g := chartGeometry{lo: 0, hi: 0, series: make([]chartSeriesGeometry, len(c.config))}
	pos, neg := make([]float64, n), make([]float64, n)
	sums := make([]float64, n)
	if c.stack == ChartExpanded {
		for i := range c.data {
			for j := range c.config {
				if v, ok := c.value(i, j); ok {
					sums[i] += math.Abs(v)
				}
			}
		}
	}
	for j := range c.config {
		s := chartSeriesGeometry{top: make([]ggui.Point, n), base: make([]ggui.Point, n), valid: make([]bool, n)}
		for i := range c.data {
			v, ok := c.value(i, j)
			s.valid[i] = ok
			if !ok {
				continue
			}
			if c.stack == ChartExpanded && sums[i] > 0 {
				v /= sums[i]
			}
			base := 0.
			if c.stack != ChartUnstacked {
				if v >= 0 {
					base = pos[i]
					pos[i] += v
				} else {
					base = neg[i]
					neg[i] += v
				}
			}
			g.lo = min(g.lo, base, base+v)
			g.hi = max(g.hi, base, base+v)
			x := .5
			if n > 1 {
				x = float64(i) / float64(n-1)
			}
			if c.kind == ChartBar {
				x = (float64(i) + .5) / float64(n)
			}
			s.top[i] = ggui.Pt(x, base+v)
			s.base[i] = ggui.Pt(x, base)
		}
		g.series[j] = s
	}
	if c.domainSet {
		g.lo, g.hi = c.domainMin, c.domainMax
	} else if c.stack == ChartExpanded {
		if g.hi > 0 {
			g.hi = 1
		}
		if g.lo < 0 {
			g.lo = -1
		}
	} else if g.hi > g.lo {
		// Four equal, round-number intervals match the reference axes.
		step := niceChartStep((g.hi - g.lo) / 4)
		g.lo = math.Floor(g.lo/step) * step
		g.hi = math.Ceil(g.hi/step) * step
	}
	if g.hi <= g.lo {
		g.hi = g.lo + 1
	}
	for j := range g.series {
		s := &g.series[j]
		for i := range s.top {
			s.top[i].Y = (s.top[i].Y - g.lo) / (g.hi - g.lo)
			s.base[i].Y = (s.base[i].Y - g.lo) / (g.hi - g.lo)
		}
		s.indices = chartSamples(s.top, s.valid, int(max(1, size.W)))
		if c.stack != ChartUnstacked {
			s.indices = mergeChartSamples(s.indices, chartSamples(s.base, s.valid, int(max(1, size.W))))
		}
	}
	c.geometry = g
}
func niceChartStep(v float64) float64 {
	p := math.Pow(10, math.Floor(math.Log10(v)))
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
		return 10 * p
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
		low, high := start, start
		for i := start + 1; i < end; i++ {
			if points[i].Y < points[low].Y {
				low = i
			}
			if points[i].Y > points[high].Y {
				high = i
			}
		}
		if low > high {
			low, high = high, low
		}
		last := -1
		for _, i := range []int{start, low, high, end - 1} {
			if i != last {
				out = append(out, i)
				last = i
			}
		}
		start = end
	}
	return out
}
func chartPosition(r ggui.Rect, p ggui.Point) ggui.Point {
	return ggui.Pt(r.Origin.X+p.X*r.Size.W, r.Origin.Y+(1-p.Y)*r.Size.H)
}
func polarPoint(center ggui.Point, radius, angle float64) ggui.Point {
	a := angle * math.Pi / 180
	return center.Add(ggui.Pt(radius*math.Cos(a), -radius*math.Sin(a)))
}
func (c *ChartWidget) polarBounds() (ggui.Point, float64) {
	return c.plot.Origin.Add(ggui.Pt(c.plot.Size.W/2, c.plot.Size.H/2)), max(0, min(c.plot.Size.W, c.plot.Size.H)/2*c.outer)
}
func (c *ChartWidget) hitIndex(p ggui.Point) int {
	if !c.plot.Contains(p) || len(c.data) == 0 || len(c.config) == 0 {
		return -1
	}
	n := len(c.data)
	if c.kind == ChartPie || c.kind == ChartRadial || c.kind == ChartRadar {
		center, radius := c.polarBounds()
		dx, dy := p.X-center.X, center.Y-p.Y
		distance := math.Hypot(dx, dy)
		if distance > radius+10 {
			return -1
		}
		angle := math.Atan2(dy, dx) * 180 / math.Pi
		if c.kind == ChartRadar {
			return int(math.Round(math.Mod(90-angle+360, 360)/360*float64(n))) % n
		}
		if distance < radius*c.inner {
			return -1
		}
		sweep := c.endAngle - c.startAngle
		if sweep == 0 {
			return -1
		}
		offset := math.Mod((angle-c.startAngle)*math.Copysign(1, sweep)+720, 360)
		if offset > math.Abs(sweep) {
			return -1
		}
		if c.kind == ChartRadial && c.stack == ChartUnstacked {
			return min(n-1, max(0, int((distance-radius*c.inner)/(radius*(1-c.inner))*float64(n))))
		}
		total := 0.
		for i := range c.data {
			v, ok := c.value(i, 0)
			if ok && v > 0 {
				total += v
			}
		}
		target := offset / math.Abs(sweep) * total
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
	f := (p.X - c.plot.Origin.X) / c.plot.Size.W
	if c.horizontal {
		f = (p.Y - c.plot.Origin.Y) / c.plot.Size.H
	}
	i := int(math.Round(f * float64(n-1)))
	if c.kind == ChartBar {
		i = int(f * float64(n))
	}
	return min(n-1, max(0, i))
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
