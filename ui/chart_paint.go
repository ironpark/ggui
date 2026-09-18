package ui

import (
	"fmt"
	"image/color"
	"math"
	"sort"

	"github.com/ironpark/ggui"
)

type chartTextKey struct {
	text string
	size float64
	rgba uint64
}
type chartText struct {
	widget *ggui.TextWidget
	size   ggui.Size
}

func (c *ChartWidget) text(dst *ggui.Canvas, text string, p ggui.Point, size float64, col color.Color, align float64) {
	if col == nil {
		return
	}
	t := c.measuredText(text, size, col)
	dst.Paint(t.widget, ggui.Rct(ggui.Pt(p.X-align*t.size.W, p.Y), t.size))
}
func (c *ChartWidget) measuredText(text string, size float64, col color.Color) *chartText {
	r, g, b, a := col.RGBA()
	key := chartTextKey{text, size, uint64(r)<<48 | uint64(g)<<32 | uint64(b)<<16 | uint64(a)}
	if c.textCache == nil {
		c.textCache = make(map[chartTextKey]*chartText)
	}
	t := c.textCache[key]
	if t == nil {
		w := ggui.Text(text).Size(size).Color(col)
		t = &chartText{w, w.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded)), c.env)}
		if len(c.textCache) > 512 {
			clear(c.textCache)
		}
		c.textCache[key] = t
	}
	return t
}

// chartPath fills and strokes a closed polyline.
func chartPath(dst *ggui.Canvas, points []ggui.Point, fill, stroke color.Color, width float64) {
	if dst == nil || dst.Image == nil || len(points) < 2 {
		return
	}
	var path ggui.Path
	path.MoveTo(points[0].X, points[0].Y)
	for _, p := range points[1:] {
		path.LineTo(p.X, p.Y)
	}
	path.Close()
	chartDrawPath(dst, &path, fill, stroke, width)
}
func chartDrawPath(dst *ggui.Canvas, path *ggui.Path, fill, stroke color.Color, width float64) {
	dst.FillPath(path, fill)
	dst.StrokePath(path, width, stroke)
}
func (c *ChartWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	c.Hit(dst, r, c, ggui.CursorShapeDefault)
	if r.Empty() {
		return
	}
	c.geometryFor(r.Size)
	c.plot = ggui.Rct(r.Origin.Add(ggui.Pt(12., 12.)), ggui.Sz(max(0, r.Size.W-24), max(0, r.Size.H-24)))
	if c.kind.polar() {
		c.plot = r
	}
	// Measuring the legend shapes every label, so do it once and reuse the
	// height for both the plot inset and the legend rect below.
	legendH := 0.
	if c.legend {
		legendH = c.legendHeight(r.Size.W)
		c.plot.Size.H = max(0, c.plot.Size.H-legendH)
	}
	if !c.kind.polar() {
		if c.xAxis {
			c.plot.Size.H = max(0, c.plot.Size.H-28)
		}
		if c.yAxis || c.horizontal && c.xAxis {
			c.plot.Origin.X += 40
			c.plot.Size.W = max(0, c.plot.Size.W-40)
		}
	}
	if c.plot.Empty() {
		return
	}
	progress := c.progress()
	if len(c.data) == 0 || len(c.config) == 0 {
		c.text(dst, "No data", r.Center().Add(ggui.Pt(0., -7.)), 12, c.theme.MutedFg, .5)
		return
	}
	if c.kind.polar() {
		c.paintPolar(dst, progress)
	} else {
		c.paintAxes(dst)
		c.paintCartesian(dst, progress)
	}
	if c.legend {
		c.paintLegend(dst, ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+r.Size.H-legendH), ggui.Sz(r.Size.W, legendH)))
	}
	if c.Focused && c.FocusVisible {
		dst.StrokeRoundRect(r, 4, 2, c.theme.Ring)
	}
	if c.active >= 0 && c.active < len(c.data) && !c.tooltip.Disabled {
		c.paintTooltip(dst, r)
	}
}

// paintBars draws one series of bars. Bars and curves are unrelated algorithms,
// so paintCartesian dispatches here rather than inlining both in one loop.
func (c *ChartWidget) paintBars(dst, clipped *ggui.Canvas, r ggui.Rect, s chartSeriesGeometry, j, m int, progress float64) {
	n := len(c.data)
	band := r.Size.W / float64(n)
	if c.horizontal {
		band = r.Size.H / float64(n)
	}
	groups := m
	group := j
	if c.stack != ChartUnstacked {
		groups, group = 1, 0
	}
	width := max(.75, (band*.8)/float64(groups)-2)
	offset := -band*.4 + float64(group)*band*.8/float64(groups) + 1
	zero := -c.geometry.lo / (c.geometry.hi - c.geometry.lo)
	for _, i := range s.indices {
		if !s.valid[i] {
			continue
		}
		top, base := s.top[i], s.base[i]
		top.Y = zero + (top.Y-zero)*progress
		base.Y = zero + (base.Y-zero)*progress
		p, q := chartPosition(r, top), chartPosition(r, base)
		bar := ggui.Rct(ggui.Pt(p.X+offset, min(p.Y, q.Y)), ggui.Sz(width, math.Abs(q.Y-p.Y)))
		if c.horizontal {
			x0 := r.Origin.X + base.Y*r.Size.W
			x1 := r.Origin.X + top.Y*r.Size.W
			bar = ggui.Rct(ggui.Pt(min(x0, x1), r.Origin.Y+top.X*r.Size.H+offset), ggui.Sz(math.Abs(x1-x0), width))
			p = ggui.Pt(x1, bar.Origin.Y+width/2)
		} else {
			p.X = bar.Origin.X + width/2
		}
		col := c.pointColor(i, j)
		if c.selected >= 0 && c.selected != i {
			col = fade(col, .45)
		}
		c.paintBar(clipped, bar, i, j, col)
		if c.labels && progress == 1 {
			c.paintLabel(dst, c.context(i, j, p))
		}
	}
}

func (c *ChartWidget) paintAxes(dst *ggui.Canvas) {
	r := c.plot
	g := c.geometry
	for k := 0; k <= 4; k++ {
		f := float64(k) / 4
		y := r.Origin.Y + r.Size.H*(1-f)
		if c.grid {
			if c.horizontal {
				x := r.Origin.X + r.Size.W*f
				dst.StrokeLine(ggui.Pt(x, r.Origin.Y), ggui.Pt(x, r.Origin.Y+r.Size.H), 1, c.theme.Border)
			} else {
				dst.StrokeLine(ggui.Pt(r.Origin.X, y), ggui.Pt(r.Origin.X+r.Size.W, y), 1, c.theme.Border)
			}
		}
		if c.yAxis && !c.horizontal {
			label := fmt.Sprintf("%g", g.lo+(g.hi-g.lo)*f)
			if c.stack == ChartExpanded {
				label = fmt.Sprintf("%.0f%%", (g.lo+(g.hi-g.lo)*f)*100)
			}
			c.text(dst, label, ggui.Pt(r.Origin.X-10, y-6), 12, c.theme.MutedFg, 1)
		}
	}
	if !c.xAxis {
		return
	}
	span := r.Size.W
	if c.horizontal {
		span = r.Size.H
	}
	gap := 60.
	if c.horizontal {
		gap = 24
	}
	count := min(len(c.data), max(2, int(span/gap)))
	for tick := 0; tick < count; tick++ {
		i := 0
		if count > 1 {
			i = int(math.Round(float64(tick) * float64(len(c.data)-1) / float64(count-1)))
		}
		d := c.data[i]
		f := c.categoryFraction(i)
		label := d.Label
		if c.tickFormat != nil {
			label = c.tickFormat(label)
		}
		if c.horizontal {
			c.text(dst, label, ggui.Pt(r.Origin.X-10, r.Origin.Y+f*r.Size.H-6), 12, c.theme.MutedFg, 1)
		} else {
			c.text(dst, label, ggui.Pt(r.Origin.X+f*r.Size.W, r.Origin.Y+r.Size.H+10), 12, c.theme.MutedFg, .5)
		}
	}
}
func (c *ChartWidget) context(i, j int, p ggui.Point) ChartPointContext {
	v, _ := c.value(i, j)
	return ChartPointContext{c.plot, c.env, i, j, c.data[i], c.config[j], v, p, c.pointColor(i, j), c.active == i || c.selected == i}
}
func (c *ChartWidget) paintLabel(dst *ggui.Canvas, ctx ChartPointContext) {
	if c.labelPaint != nil {
		c.labelPaint(dst, ctx)
		return
	}
	label := fmt.Sprintf("%g", ctx.Value)
	if c.labelFormat != nil {
		label = c.labelFormat(ctx)
	}
	if c.kind == ChartBar && c.horizontal {
		c.text(dst, label, ctx.Position.Add(ggui.Pt(8., -7.)), 12, c.theme.Fg, 0)
	} else {
		c.text(dst, label, ctx.Position.Add(ggui.Pt(0., -22.)), 12, c.theme.Fg, .5)
	}
}
func (c *ChartWidget) paintDot(dst *ggui.Canvas, ctx ChartPointContext) {
	if c.dotPaint != nil {
		c.dotPaint(dst, ctx)
		return
	}
	dst.FillCircle(ctx.Position, 3, ctx.Color)
	if ctx.Active {
		dst.FillCircle(ctx.Position, 5, c.theme.Card)
		dst.FillCircle(ctx.Position, 3.5, ctx.Color)
	}
}
func (c *ChartWidget) paintCartesian(dst *ggui.Canvas, progress float64) {
	r := c.plot
	n := len(c.data)
	m := len(c.config)
	if c.active >= 0 && c.active < n && c.tooltipCursor && c.kind == ChartBar {
		band := r.Size.W / float64(n)
		h := ggui.Rct(ggui.Pt(r.Origin.X+float64(c.active)*band, r.Origin.Y), ggui.Sz(band, r.Size.H))
		if c.horizontal {
			band = r.Size.H / float64(n)
			h = ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+float64(c.active)*band), ggui.Sz(r.Size.W, band))
		}
		dst.FillRect(h, c.theme.Muted)
	}
	clipped := dst.Clip(r)
	for j, s := range c.geometry.series {
		if c.kind == ChartBar {
			c.paintBars(dst, clipped, r, s, j, m, progress)
			continue
		}
		reveal := clipped
		if progress < 1 && c.kind == ChartArea {
			reveal = dst.Clip(ggui.Rct(r.Origin, ggui.Sz(r.Size.W*progress, r.Size.H)))
		}
		c.prepareCurves()
		totalLength := 0.
		if c.kind == ChartLine && progress < 1 {
			for k := range c.curves[j] {
				totalLength += c.curves[j][k].length
			}
		}
		remaining := totalLength * progress
		col, opacity := c.seriesColor(j), c.seriesOpacity(j)
		for k := range c.curves[j] {
			paths := &c.curves[j][k]
			if c.kind == ChartLine && progress < 1 {
				if remaining <= 0 {
					break
				}
				if remaining < paths.length {
					paths.strokePrefix(reveal, remaining, c.stroke, col)
					break
				}
				remaining -= paths.length
			}
			c.paintCurvePaths(reveal, paths, col, opacity)
		}

		if progress == 1 && (c.dots || c.labels) {
			for _, i := range s.indices {
				if !s.valid[i] {
					continue
				}
				p := chartPosition(r, s.top[i])
				ctx := c.context(i, j, p)
				if c.dots {
					c.paintDot(dst, ctx)
				}
				if c.labels {
					c.paintLabel(dst, ctx)
				}
			}
		}
		if c.active >= 0 && c.active < n && s.valid[c.active] {
			p := chartPosition(r, s.top[c.active])
			if c.tooltipCursor {
				dst.StrokeLine(ggui.Pt(p.X, r.Origin.Y), ggui.Pt(p.X, r.Origin.Y+r.Size.H), 1, c.theme.Border)
			}
			c.paintDot(dst, c.context(c.active, j, p))
		}
	}
}

// curvePath uses natural cubic splines (the reference's default), monotone
// Hermite tangents, straight lines, or midpoint steps. Coordinates are logical.
func curvePath(path *ggui.Path, points []ggui.Point, curve ChartCurve, move bool, trace func(ggui.Point)) {
	n := len(points)
	if n == 0 {
		return
	}
	if move {
		path.MoveTo(points[0].X, points[0].Y)
	} else {
		path.LineTo(points[0].X, points[0].Y)
	}
	if trace != nil {
		trace(points[0])
	}
	previous := points[0]
	lineTo := func(x, y float64) {
		path.LineTo(x, y)
		previous = ggui.Pt(x, y)
		if trace != nil {
			trace(previous)
		}
	}
	cubicTo := func(x1, y1, x2, y2, x3, y3 float64) {
		path.CubicTo(x1, y1, x2, y2, x3, y3)
		if trace != nil {
			for k := 1; k <= 12; k++ {
				t := float64(k) / 12
				u := 1 - t
				trace(ggui.Pt(u*u*u*previous.X+3*u*u*t*x1+3*u*t*t*x2+t*t*t*x3, u*u*u*previous.Y+3*u*u*t*y1+3*u*t*t*y2+t*t*t*y3))
			}
		}
		previous = ggui.Pt(x3, y3)
	}

	if n == 1 {
		return
	}
	if curve == ChartNatural && n > 2 {
		control := naturalControls(points)
		for i := 0; i < n-1; i++ {
			q := ggui.Pt((points[n-1].X+control[n-2].X)/2, (points[n-1].Y+control[n-2].Y)/2)
			if i < n-2 {
				q = ggui.Pt(2*points[i+1].X-control[i+1].X, 2*points[i+1].Y-control[i+1].Y)
			}
			p := points[i+1]
			cubicTo(control[i].X, control[i].Y, q.X, q.Y, p.X, p.Y)
		}
		return
	}
	slopes := make([]float64, n)
	if curve == ChartMonotone {
		slopes = monotoneSlopes(points)
	}
	for i := 1; i < n; i++ {
		p, q := points[i-1], points[i]
		switch curve {
		case ChartStep:
			mid := (p.X + q.X) / 2
			lineTo(mid, p.Y)
			lineTo(mid, q.Y)
			lineTo(q.X, q.Y)
		case ChartMonotone:
			dx := (q.X - p.X) / 3
			cubicTo(p.X+dx, p.Y+slopes[i-1]*dx, q.X-dx, q.Y-slopes[i]*dx, q.X, q.Y)
		default:
			lineTo(q.X, q.Y)
		}
	}
}

// naturalControls returns the first Bezier control point of each segment of
// the natural cubic spline through points, by solving the tridiagonal system
// the spline's continuity conditions give. It expects at least three points.
func naturalControls(points []ggui.Point) []ggui.Point {
	n := len(points)
	control := make([]ggui.Point, n-1)
	rhs := make([]ggui.Point, n-1)
	diag := make([]float64, n-1)
	rhs[0] = ggui.Pt(points[0].X+2*points[1].X, points[0].Y+2*points[1].Y)
	diag[0] = 2
	// Forward sweep: eliminate the sub-diagonal, carrying the elimination
	// into the right-hand side. The last row has the free-end coefficients.
	for i := 1; i < n-1; i++ {
		a, b := 1., 4.
		rhs[i] = ggui.Pt(4*points[i].X+2*points[i+1].X, 4*points[i].Y+2*points[i+1].Y)
		if i == n-2 {
			a = 2
			b = 7
			rhs[i] = ggui.Pt(8*points[i].X+points[i+1].X, 8*points[i].Y+points[i+1].Y)
		}
		factor := a / diag[i-1]
		diag[i] = b - factor
		rhs[i].X -= factor * rhs[i-1].X
		rhs[i].Y -= factor * rhs[i-1].Y
	}
	// Back substitution.
	control[n-2] = ggui.Pt(rhs[n-2].X/diag[n-2], rhs[n-2].Y/diag[n-2])
	for i := n - 3; i >= 0; i-- {
		control[i] = ggui.Pt((rhs[i].X-control[i+1].X)/diag[i], (rhs[i].Y-control[i+1].Y)/diag[i])
	}
	return control
}

// monotoneSlopes returns the tangent at each point for a Fritsch-Carlson
// monotone cubic: the harmonic-mean slope where the neighbouring segments
// agree in direction, and zero at a turning point, so the curve never
// overshoots a value the data does not have.
func monotoneSlopes(points []ggui.Point) []float64 {
	n := len(points)
	slopes := make([]float64, n)
	for i := range n {
		switch {
		case i == 0:
			slopes[i] = (points[1].Y - points[0].Y) / (points[1].X - points[0].X)
		case i == n-1:
			slopes[i] = (points[i].Y - points[i-1].Y) / (points[i].X - points[i-1].X)
		default:
			a := (points[i].Y - points[i-1].Y) / (points[i].X - points[i-1].X)
			b := (points[i+1].Y - points[i].Y) / (points[i+1].X - points[i].X)
			if a*b > 0 {
				slopes[i] = math.Copysign(min(math.Abs(a), math.Abs(b), .5*math.Abs(a+b)), a)
			}
		}
	}
	return slopes
}

type chartCurvePaths struct {
	line, fill, partial ggui.Path
	samples             []ggui.Point
	lengths             []float64
	length              float64
}

func (c *ChartWidget) prepareCurves() {
	if c.curves != nil && c.curveRect == c.plot {
		return
	}
	c.curveRect = c.plot
	c.curves = make([][]chartCurvePaths, len(c.geometry.series))
	for j, s := range c.geometry.series {
		for start := 0; start < len(s.indices); {
			for start < len(s.indices) && !s.valid[s.indices[start]] {
				start++
			}
			end := start
			for end < len(s.indices) && s.valid[s.indices[end]] {
				end++
			}
			if end-start > 1 {
				// Bound stencil winding complexity for dense, oscillating curves.
				// Adjacent chunks share their endpoint so the chart stays continuous.
				for first := start; first < end-1; {
					last := min(first+25, end)
					ids := s.indices[first:last]
					top := make([]ggui.Point, len(ids))
					base := make([]ggui.Point, len(ids))
					for k, i := range ids {
						top[k] = chartPosition(c.plot, s.top[i])
						base[len(ids)-1-k] = chartPosition(c.plot, s.base[i])
					}
					paths := chartCurvePaths{}
					curvePath(&paths.line, top, c.curve, true, func(p ggui.Point) {
						if len(paths.samples) > 0 {
							q := paths.samples[len(paths.samples)-1]
							paths.length += math.Hypot(p.X-q.X, p.Y-q.Y)
						}
						paths.samples = append(paths.samples, p)
						paths.lengths = append(paths.lengths, paths.length)
					})
					if c.kind == ChartArea {
						curvePath(&paths.fill, top, c.curve, true, nil)
						curvePath(&paths.fill, base, c.curve, false, nil)
						paths.fill.Close()
					}
					c.curves[j] = append(c.curves[j], paths)
					first = last - 1
				}
			}
			start = end + 1
		}
	}
}
func (c *ChartWidget) paintCurvePaths(dst *ggui.Canvas, paths *chartCurvePaths, col color.Color, opacity float64) {
	if c.kind == ChartArea {
		if c.gradient {
			dst.FillPathGradient(&paths.fill, c.plot, fade(col, .8), fade(col, .05))
		} else {
			dst.FillPath(&paths.fill, fade(col, opacity))
		}
	}
	dst.StrokePath(&paths.line, c.stroke, col)
}

func (c *ChartWidget) paintBar(dst *ggui.Canvas, bar ggui.Rect, i, j int, col color.Color) {
	radius := min(c.corner, bar.Size.W/2, bar.Size.H/2)
	dst.FillRoundRect(bar, radius, col)
	if c.stack == ChartUnstacked || radius <= 0 {
		return
	}
	v, _ := c.value(i, j)
	before, after := false, false
	for k := range c.config {
		other, ok := c.value(i, k)
		if !ok || other == 0 || v*other < 0 {
			continue
		}
		if k < j {
			before = true
		}
		if k > j {
			after = true
		}
	}
	// Square only internal joins. The two exposed ends retain their rounding.
	first, last := before, after
	if (!c.horizontal && v >= 0) || (c.horizontal && v < 0) {
		first, last = after, before
	}
	if c.horizontal {
		if first {
			dst.FillRect(ggui.Rct(bar.Origin, ggui.Sz(radius, bar.Size.H)), col)
		}
		if last {
			dst.FillRect(ggui.Rct(bar.Origin.Add(ggui.Pt(bar.Size.W-radius, 0.)), ggui.Sz(radius, bar.Size.H)), col)
		}
	} else {
		if first {
			dst.FillRect(ggui.Rct(bar.Origin, ggui.Sz(bar.Size.W, radius)), col)
		}
		if last {
			dst.FillRect(ggui.Rct(bar.Origin.Add(ggui.Pt(0., bar.Size.H-radius)), ggui.Sz(bar.Size.W, radius)), col)
		}
	}
}

func (p *chartCurvePaths) strokePrefix(dst *ggui.Canvas, length, width float64, col color.Color) {
	if len(p.samples) < 2 {
		return
	}
	end := sort.SearchFloat64s(p.lengths, length)
	if end >= len(p.samples) {
		dst.StrokePath(&p.line, width, col)
		return
	}
	p.partial.Reset()
	p.partial.MoveTo(p.samples[0].X, p.samples[0].Y)
	for _, q := range p.samples[1:end] {
		p.partial.LineTo(q.X, q.Y)
	}
	if end > 0 {
		a, b := p.samples[end-1], p.samples[end]
		span := p.lengths[end] - p.lengths[end-1]
		f := 0.
		if span > 0 {
			f = (length - p.lengths[end-1]) / span
		}
		p.partial.LineTo(a.X+(b.X-a.X)*f, a.Y+(b.Y-a.Y)*f)
	}
	dst.StrokePath(&p.partial, width, col)
}
