package ui

import (
	"fmt"
	"image/color"
	"math"

	"github.com/ironpark/ggui"
)

func chartSector(dst *ggui.Canvas, center ggui.Point, inner, outer, start, end float64, col color.Color, separator color.Color, rounded float64) {
	if dst == nil || dst.Image == nil || outer <= inner || end == start {
		return
	}
	sweep := end - start
	sign := math.Copysign(1, sweep)
	corner := min(max(0, rounded), (outer-inner)/2, math.Abs(sweep)*math.Pi/180*max(inner, 1)/2)
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
func (c *ChartWidget) paintPolar(dst *ggui.Canvas, progress float64) {
	center, radius := c.polarBounds()
	if radius <= 0 {
		return
	}
	switch c.kind {
	case ChartRadar:
		c.paintRadar(dst, center, radius, progress)
	case ChartPie:
		c.paintPie(dst, center, radius, progress)
	case ChartRadial:
		c.paintRadial(dst, center, radius, progress)
	}
	if c.centerValue != "" {
		c.text(dst, c.centerValue, center.Add(ggui.Pt(0., -22.)), 28, c.theme.Fg, .5)
		c.text(dst, c.centerLabel, center.Add(ggui.Pt(0., 12.)), 12, c.theme.MutedFg, .5)
	}
}
func (c *ChartWidget) paintPie(dst *ggui.Canvas, center ggui.Point, radius, progress float64) {
	rings := len(c.config)
	for j := range c.config {
		total := 0.
		for i := range c.data {
			if v, ok := c.value(i, j); ok && v > 0 {
				total += v
			}
		}
		if total <= 0 {
			continue
		}
		outer := radius - float64(j)*(radius*(1-c.inner))/float64(rings)
		inner := radius * c.inner
		if rings > 1 {
			inner = outer - radius*(1-c.inner)/float64(rings) + 3
		}
		series := c.config[j]
		if series.OuterRadius > 0 {
			outer = radius * clamp(series.OuterRadius, 0, 1)
			inner = radius * clamp(series.InnerRadius, 0, series.OuterRadius)
		}
		angle := c.startAngle
		for i := range c.data {
			v, ok := c.value(i, j)
			if !ok || v <= 0 {
				continue
			}
			sweep := (c.endAngle - c.startAngle) * v / total * progress
			end := angle + sweep
			r := outer
			if i == c.selected {
				r += 10
			}
			var sep color.Color
			if c.separators {
				sep = c.theme.Card
			}
			chartSector(dst, center, inner, r, angle, end, c.pointColor(i, j), sep, 0)
			if c.activeRing && i == c.selected {
				chartSector(dst, center, outer+12, outer+25, angle, end, c.pointColor(i, j), nil, 0)
			}
			if c.labels && progress == 1 {
				p := polarPoint(center, (inner+r)/2, (angle+end)/2)
				if !c.labelInside {
					p = polarPoint(center, r+18, (angle+end)/2)
					p.Y += 16
				}
				ctx := c.context(i, j, p)
				if c.labelInside {
					ctx.Position.Y += 16
				}
				c.paintLabel(dst, ctx)
			}
			angle = end
		}
	}
}
func (c *ChartWidget) paintRadial(dst *ggui.Canvas, center ggui.Point, radius, progress float64) {
	inner := radius * c.inner
	n := len(c.data)
	if c.grid {
		for k := 1; k <= 5; k++ {
			points := make([]ggui.Point, 97)
			for j := range points {
				points[j] = polarPoint(center, radius*float64(k)/5, float64(j)*360/96)
			}
			chartPath(dst, points, true, nil, c.theme.Border, 1)
		}
	}

	if c.stack != ChartUnstacked {
		band := (radius - inner) / float64(n)
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
			in := inner + float64(i)*band
			out := in + band*.8
			if c.radialTrack {
				chartSector(dst, center, in, out, 0, 360, c.theme.Muted, nil, 0)
			}
			for j := range c.config {
				v, ok := c.value(i, j)
				if !ok || v <= 0 {
					continue
				}
				end := angle + (c.endAngle-c.startAngle)*v/total*progress
				chartSector(dst, center, in, out, angle, end, c.seriesColor(j), nil, c.corner)
				angle = end
			}
		}
		return
	}
	maximum := max(0, c.geometry.hi)
	if n == 1 {
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
	if c.domainSet {
		maximum = c.domainMax
	}
	if maximum <= 0 {
		return
	}
	band := (radius - inner) / float64(n)
	for i := range c.data {
		v, ok := c.value(i, 0)
		if !ok || v <= 0 {
			continue
		}
		in := inner + float64(i)*band + band*.1
		out := in + band*.8
		if c.radialTrack {
			chartSector(dst, center, in, out, 0, 360, c.theme.Muted, nil, 0)
		}
		end := c.startAngle + (c.endAngle-c.startAngle)*clamp(v/maximum, 0, 1)*progress
		chartSector(dst, center, in, out, c.startAngle, end, c.pointColor(i, 0), nil, c.corner)
		if c.labels && progress == 1 {
			label := c.data[i].Label
			ctx := c.context(i, 0, polarPoint(center, (in+out)/2, c.startAngle))
			if c.labelFormat != nil {
				label = c.labelFormat(ctx)
			}
			c.arcLabel(dst, label, center, (in+out)/2, c.startAngle, c.theme.Fg)
		}
	}
}
func (c *ChartWidget) paintRadar(dst *ggui.Canvas, center ggui.Point, radius, progress float64) {
	n := len(c.data)
	if n < 3 {
		return
	}
	grid := c.polarGrid
	rings := grid.Rings
	if rings <= 0 {
		rings = 5
	}
	if c.grid {
		if !grid.HideRings {
			for k := rings; k >= 1; k-- {
				r := radius * float64(k) / float64(rings)
				var fill color.Color
				if grid.Fill && k == rings {
					fill = fade(c.seriesColor(0), .2)
				}
				if grid.Circle {
					points := make([]ggui.Point, 97)
					for i := range points {
						points[i] = polarPoint(center, r, float64(i)*360/96)
					}
					chartPath(dst, points, true, fill, c.theme.Border, 1)
				} else {
					points := make([]ggui.Point, n)
					for i := range points {
						points[i] = polarPoint(center, r, 90-float64(i)*360/float64(n))
					}
					chartPath(dst, points, true, fill, c.theme.Border, 1)
				}
			}
		}
		if !grid.HideSpokes {
			for i := range c.data {
				dst.StrokeLine(center, polarPoint(center, radius, 90-float64(i)*360/float64(n)), 1, c.theme.Border)
			}
		}
	}
	for i, d := range c.data {
		angle := 90 - float64(i)*360/float64(n)
		p := polarPoint(center, radius+16, angle)
		label := d.Label
		if c.tickFormat != nil {
			label = c.tickFormat(label)
		}
		if c.tickPaint != nil {
			c.tickPaint(dst, c.context(i, 0, p))
		} else if d.Icon != "" {
			paintIcon(dst, c.env, d.Icon, ggui.Rct(p.Add(ggui.Pt(-8., -8.)), ggui.Sz(16., 16.)), c.theme.MutedFg, 0)
		} else {
			c.text(dst, label, p.Add(ggui.Pt(0., -6.)), 12, c.theme.MutedFg, .5)
		}
	}
	if c.yAxis {
		for k := 1; k <= rings; k++ {
			f := float64(k) / float64(rings)
			p := polarPoint(center, radius*f, 60)
			c.text(dst, fmt.Sprintf("%g", c.geometry.lo+(c.geometry.hi-c.geometry.lo)*f), p, 11, c.theme.Fg, .5)
		}
	}

	for j := range c.config {
		points := make([]ggui.Point, n)
		for i := range c.data {
			v, ok := c.value(i, j)
			if !ok {
				v = 0
			}
			f := clamp((v-c.geometry.lo)/(c.geometry.hi-c.geometry.lo), 0, 1)
			points[i] = polarPoint(center, radius*f*progress, 90-float64(i)*360/float64(n))
		}
		col := c.seriesColor(j)
		chartPath(dst, points, true, fade(col, c.seriesOpacity(j)), col, c.stroke)
		if progress == 1 {
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
	}
}

func (c *ChartWidget) arcLabel(dst *ggui.Canvas, label string, center ggui.Point, radius, start float64, col color.Color) {
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
