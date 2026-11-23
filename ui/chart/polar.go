package chart

import (
	"fmt"
	"image/color"
	"math"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// sectorSpec is an annular wedge: the ring between inner and outer about
// center, swept from start to end degrees, its corners rounded by up to
// rounded.
type sectorSpec struct {
	center                            ggui.Point
	inner, outer, start, end, rounded float64
}

func chartSector(dst *ggui.Canvas, s sectorSpec, col color.Color, separator color.Color) {
	center, inner, outer, start, end := s.center, s.inner, s.outer, s.start, s.end
	if dst == nil || dst.Image == nil || outer <= inner || end == start {
		return
	}
	sweep := end - start
	sign := math.Copysign(1, sweep)
	corner := min(max(0, s.rounded), (outer-inner)/2, math.Abs(sweep)*math.Pi/180*max(inner, 1)/2)
	if math.Abs(sweep) >= 359.9 {
		corner = 0
	}
	outsideOffset := corner / outer * 180 / math.Pi * sign
	insideOffset := corner / max(inner, 1) * 180 / math.Pi * sign
	var path ggui.Path
	first := polarPoint(center, outer, start+outsideOffset)
	path.MoveTo(first.X, first.Y)
	arc := func(radius, a, b float64) {
		steps := max(2, int(math.Ceil(math.Abs(b-a)/3)))
		for i := 1; i <= steps; i++ {
			p := polarPoint(center, radius, a+(b-a)*float64(i)/float64(steps))
			path.LineTo(p.X, p.Y)
		}
	}
	quad := func(control, end ggui.Point) { path.QuadTo(control.X, control.Y, end.X, end.Y) }
	line := func(p ggui.Point) { path.LineTo(p.X, p.Y) }
	arc(outer, start+outsideOffset, end-outsideOffset)
	quad(polarPoint(center, outer, end), polarPoint(center, outer-corner, end))
	line(polarPoint(center, inner+corner, end))
	quad(polarPoint(center, inner, end), polarPoint(center, inner, end-insideOffset))
	arc(inner, end-insideOffset, start+insideOffset)
	quad(polarPoint(center, inner, start), polarPoint(center, inner+corner, start))
	line(polarPoint(center, outer-corner, start))
	quad(polarPoint(center, outer, start), first)
	path.Close()
	chartDrawPath(dst, &path, col, separator, 2)
}
func (c *Widget) paintPolar(dst *ggui.Canvas, progress float64) {
	frame := c.polarFrame()
	if frame.radius <= 0 {
		return
	}
	switch c.kind {
	case KindRadar:
		c.paintRadar(dst, frame, progress)
	case KindPie:
		c.paintPie(dst, frame, progress)
	case KindRadial:
		c.paintRadial(dst, frame, progress)
	}
	if c.centerValue != "" {
		c.text(dst, c.centerValue, frame.center.Add(ggui.Pt(0., -22.)), 28, c.theme.Fg, .5)
		c.text(dst, c.centerLabel, frame.center.Add(ggui.Pt(0., 12.)), 12, c.theme.MutedFg, .5)
	}
}

// pieRing is one series' ring of a pie: its radii, and the positive total
// its slices share.
type pieRing struct {
	series              int
	inner, outer, total float64
}

// pieRingFor places series j's ring: the series split the disc's band
// evenly, the first outermost, unless a series sets its own radii.
func (c *Widget) pieRingFor(j int, frame polarFrame) pieRing {
	radius := frame.radius
	rings := len(c.config)
	outer := radius - float64(j)*(radius*(1-c.inner))/float64(rings)
	inner := frame.inner
	if rings > 1 {
		inner = outer - radius*(1-c.inner)/float64(rings) + 3
	}
	series := c.config[j]
	if series.OuterRadius > 0 {
		outer = radius * clamp(series.OuterRadius, 0, 1)
		inner = radius * clamp(series.InnerRadius, 0, series.OuterRadius)
	}
	return pieRing{series: j, inner: inner, outer: outer, total: c.positiveTotal(j)}
}
func (c *Widget) paintPie(dst *ggui.Canvas, frame polarFrame, progress float64) {
	for j := range c.config {
		if ring := c.pieRingFor(j, frame); ring.total > 0 {
			c.paintPieRing(dst, frame, ring, progress)
		}
	}
}

// paintPieRing draws a ring's slices, the selected one pulled out and, with
// an active ring, circled, and their labels once the entrance is over.
func (c *Widget) paintPieRing(dst *ggui.Canvas, frame polarFrame, ring pieRing, progress float64) {
	j := ring.series
	var sep color.Color
	if c.separators {
		sep = c.theme.Card
	}
	angle := c.startAngle
	for i := range c.data {
		v, ok := c.value(i, j)
		if !ok || v <= 0 {
			continue
		}
		sweep := (c.endAngle - c.startAngle) * v / ring.total * progress
		end := angle + sweep
		slice := sectorSpec{center: frame.center, inner: ring.inner, outer: ring.outer, start: angle, end: end}
		if i == c.selected {
			slice.outer += 10
		}
		chartSector(dst, slice, c.pointColor(i, j), sep)
		if c.activeRing && i == c.selected {
			chartSector(dst, sectorSpec{center: frame.center, inner: ring.outer + 12, outer: ring.outer + 25, start: angle, end: end}, c.pointColor(i, j), nil)
		}
		if c.labels && progress == 1 {
			mid := (angle + end) / 2
			p := polarPoint(frame.center, (slice.inner+slice.outer)/2, mid)
			if !c.labelInside {
				p = polarPoint(frame.center, slice.outer+18, mid)
			}
			p.Y += 16
			c.paintLabel(dst, c.context(i, j, p))
		}
		angle = end
	}
}
func (c *Widget) paintRadial(dst *ggui.Canvas, frame polarFrame, progress float64) {
	if c.grid {
		for k := 1; k <= 5; k++ {
			chartPath(dst, circlePoints(frame.center, frame.radius*float64(k)/5), nil, c.theme.Border, 1)
		}
	}
	if c.stack != StackUnstacked {
		c.paintRadialStacks(dst, frame, progress)
		return
	}
	c.paintRadialBands(dst, frame, progress)
}

// paintRadialStacks gives each category a band of its own, its sweep split
// among the series in proportion to their positive values.
func (c *Widget) paintRadialStacks(dst *ggui.Canvas, frame polarFrame, progress float64) {
	band := (frame.radius - frame.inner) / float64(len(c.data))
	for i := range c.data {
		total := 0.
		for j := range c.config {
			if v, ok := c.value(i, j); ok && v > 0 {
				total += v
			}
		}
		if total == 0 {
			continue
		}
		angle := c.startAngle
		in := frame.inner + float64(i)*band
		out := in + band*.8
		if c.radialTrack {
			chartSector(dst, sectorSpec{center: frame.center, inner: in, outer: out, start: 0, end: 360}, c.theme.Muted, nil)
		}
		for j := range c.config {
			v, ok := c.value(i, j)
			if !ok || v <= 0 {
				continue
			}
			end := angle + (c.endAngle-c.startAngle)*v/total*progress
			chartSector(dst, sectorSpec{center: frame.center, inner: in, outer: out, start: angle, end: end, rounded: c.corner}, c.seriesColor(j), nil)
			angle = end
		}
	}
}

// radialMaximum is the value a full sweep of an unstacked radial chart
// stands for: the domain's maximum when set, else the largest value shown.
func (c *Widget) radialMaximum() float64 {
	if c.domainSet {
		return c.domainMax
	}
	maximum := max(0, c.geometry.hi)
	if len(c.data) == 1 {
		maximum = 0
		for j := range c.config {
			if v, ok := c.value(0, j); ok {
				maximum = max(maximum, v)
			}
		}
	}
	for i := range c.data {
		if v, ok := c.value(i, 0); ok {
			maximum = max(maximum, v)
		}
	}
	return maximum
}

// paintRadialBands gives each category's first-series value an arc on a
// band of its own, over a track, and labels the bands along their arcs.
func (c *Widget) paintRadialBands(dst *ggui.Canvas, frame polarFrame, progress float64) {
	maximum := c.radialMaximum()
	if maximum <= 0 {
		return
	}
	band := (frame.radius - frame.inner) / float64(len(c.data))
	for i := range c.data {
		v, ok := c.value(i, 0)
		if !ok || v <= 0 {
			continue
		}
		in := frame.inner + float64(i)*band + band*.1
		out := in + band*.8
		if c.radialTrack {
			chartSector(dst, sectorSpec{center: frame.center, inner: in, outer: out, start: 0, end: 360}, c.theme.Muted, nil)
		}
		end := c.startAngle + (c.endAngle-c.startAngle)*clamp(v/maximum, 0, 1)*progress
		chartSector(dst, sectorSpec{center: frame.center, inner: in, outer: out, start: c.startAngle, end: end, rounded: c.corner}, c.pointColor(i, 0), nil)
		if c.labels && progress == 1 {
			label := c.data[i].Label
			ctx := c.context(i, 0, polarPoint(frame.center, (in+out)/2, c.startAngle))
			if c.labelFormat != nil {
				label = c.labelFormat(ctx)
			}
			c.arcLabel(dst, label, frame.center, (in+out)/2, c.startAngle, c.theme.Fg)
		}
	}
}
func (c *Widget) paintRadar(dst *ggui.Canvas, frame polarFrame, progress float64) {
	if len(c.data) < 3 {
		return
	}
	rings := c.polarGrid.Rings
	if rings <= 0 {
		rings = 5
	}
	if c.grid {
		c.paintRadarGrid(dst, frame, rings)
	}
	c.paintRadarCategories(dst, frame)
	if c.yAxis {
		for k := 1; k <= rings; k++ {
			f := float64(k) / float64(rings)
			p := polarPoint(frame.center, frame.radius*f, 60)
			c.text(dst, fmt.Sprintf("%g", c.geometry.lo+(c.geometry.hi-c.geometry.lo)*f), p, 11, c.theme.Fg, .5)
		}
	}
	for j := range c.config {
		c.paintRadarSeries(dst, frame, j, progress)
	}
}

// paintRadarGrid draws the radar's rings, the outermost first, as circles or
// as polygons through the spokes, and then the spokes, as PolarGrid asks.
func (c *Widget) paintRadarGrid(dst *ggui.Canvas, frame polarFrame, rings int) {
	grid := c.polarGrid
	n := len(c.data)
	if !grid.HideRings {
		for k := rings; k >= 1; k-- {
			r := frame.radius * float64(k) / float64(rings)
			var fill color.Color
			if grid.Fill && k == rings {
				fill = fade(c.seriesColor(0), .2)
			}
			points := circlePoints(frame.center, r)
			if !grid.Circle {
				points = make([]ggui.Point, n)
				for i := range points {
					points[i] = polarPoint(frame.center, r, radarAngle(i, n))
				}
			}
			chartPath(dst, points, fill, c.theme.Border, 1)
		}
	}
	if !grid.HideSpokes {
		for i := range c.data {
			dst.StrokeLine(frame.center, polarPoint(frame.center, frame.radius, radarAngle(i, n)), 1, c.theme.Border)
		}
	}
}

// paintRadarCategories names each spoke past the rim: through the tick
// painter when set, else with the category's icon, else with its label.
func (c *Widget) paintRadarCategories(dst *ggui.Canvas, frame polarFrame) {
	n := len(c.data)
	for i, d := range c.data {
		angle := radarAngle(i, n)
		p := polarPoint(frame.center, frame.radius+16, angle)
		label := d.Label
		if c.tickFormat != nil {
			label = c.tickFormat(label)
		}
		if c.tickPaint != nil {
			c.tickPaint(dst, c.context(i, 0, p))
		} else if d.Icon != "" {
			ui.PaintIcon(dst, c.env, d.Icon, ggui.Rct(p.Add(ggui.Pt(-8., -8.)), ggui.Sz(16., 16.)), c.theme.MutedFg, 0)
		} else {
			c.text(dst, label, p.Add(ggui.Pt(0., -6.)), 12, c.theme.MutedFg, .5)
		}
	}
}

// paintRadarSeries draws series j's polygon, grown out from the center by
// progress, and once it is whole, its dots and labels.
func (c *Widget) paintRadarSeries(dst *ggui.Canvas, frame polarFrame, j int, progress float64) {
	n := len(c.data)
	points := make([]ggui.Point, n)
	for i := range c.data {
		v, ok := c.value(i, j)
		if !ok {
			v = 0
		}
		f := clamp((v-c.geometry.lo)/(c.geometry.hi-c.geometry.lo), 0, 1)
		points[i] = polarPoint(frame.center, frame.radius*f*progress, radarAngle(i, n))
	}
	col := c.seriesColor(j)
	chartPath(dst, points, fade(col, c.seriesOpacity(j)), col, c.stroke)
	if progress != 1 {
		return
	}
	for i, p := range points {
		if _, ok := c.value(i, j); !ok {
			continue
		}
		ctx := c.context(i, j, p)
		if c.dots || c.active == i {
			c.paintDot(dst, ctx)
		}
		if c.labels {
			c.paintLabel(dst, ctx)
		}
	}
}

func (c *Widget) arcLabel(dst *ggui.Canvas, label string, center ggui.Point, radius, start float64, col color.Color) {
	angle := start + 4
	for _, r := range label {
		text := c.measuredText(string(r), 10, col)
		advance := text.size.W / max(1, radius) * 180 / math.Pi
		a := angle + advance/2
		p := polarPoint(center, radius, a)
		rect := ggui.Rct(p.Add(ggui.Pt(-text.size.W/2, -text.size.H/2)), text.size)
		text.widget.PaintRotated(dst, rect, (-a-90)*math.Pi/180)
		angle += advance
	}
}
