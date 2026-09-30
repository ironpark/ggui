package ggui

import (
	"image/color"
	"math"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/render"
)

// The shapes below are drawn by the render package from signed distance
// fields; these methods turn logical coordinates into the Image's pixels.

// shadeRoundRect draws the rounded rectangle shape in col. A zero halfStroke
// fills it; otherwise shape is the centreline of a border of twice that
// width. Both are logical, as radius is.
func (c *Canvas) shadeRoundRect(shape Rect, radius, halfStroke float64, col color.Color) {
	scale := c.Scale()
	if math.IsNaN(scale) || math.IsInf(scale, 0) {
		return
	}
	render.Shade(c.Image, render.Field{
		Centre: Pt(c.px(shape.Origin.X+shape.Size.W/2), c.px(shape.Origin.Y+shape.Size.H/2)),
		Axis:   Pt(1, 0),
		Half:   Sz(shape.Size.W*scale/2, shape.Size.H*scale/2),
		Radius: radius * scale,
		Stroke: halfStroke * scale,
	}, col)
}

// shadeSegment draws the line from a to b, w wide, in col. The ends are cut
// square, as the vector renderer's default cap is.
func (c *Canvas) shadeSegment(a, b Point, w float64, col color.Color) {
	scale := c.Scale()
	if math.IsNaN(scale) || math.IsInf(scale, 0) {
		return
	}
	dx, dy := c.px(b.X-a.X), c.px(b.Y-a.Y)
	length := math.Hypot(dx, dy)
	if length == 0 || math.IsNaN(length) || math.IsInf(length, 0) {
		return
	}
	render.Shade(c.Image, render.Field{
		Centre: Pt(c.px((a.X+b.X)/2), c.px((a.Y+b.Y)/2)),
		Axis:   Pt(dx/length, dy/length),
		Half:   Sz(length/2, w*scale/2),
	}, col)
}

// maskRoundRect draws img, a part of a layer, onto c where it lies within
// the logical Rect r with corners rounded by radius.
func (c *Canvas) maskRoundRect(img *ggfx.Image, r Rect, radius float64) {
	scale := c.Scale()
	centre := r.Center()
	render.MaskRoundRect(c.Image, img, Pt(c.px(centre.X), c.px(centre.Y)), Sz(r.Size.W*scale/2, r.Size.H*scale/2), radius*scale)
}

// shadeImage draws img placed at the logical Rect at, cut to r with its
// corners rounded by o.Radius, in one draw: a quad over r and a pixel of
// edge, whose source positions follow at.
func (c *Canvas) shadeImage(img *ggfx.Image, r, at Rect, o ImageOptions) {
	scale := c.Scale()
	reach := 1 / scale
	if o.Rotation != 0 {
		reach += math.Hypot(r.Size.W, r.Size.H)/2 - min(r.Size.W, r.Size.H)/2
	}
	if at.Size.W <= 0 || at.Size.H <= 0 || !c.visiblePaintBounds(r, reach) {
		return
	}
	b := img.Bounds()
	kx, ky := float64(b.Dx())/at.Size.W, float64(b.Dy())/at.Size.H
	centre := r.Center()
	sin, cos := math.Sincos(o.Rotation)
	red, green, blue, alpha := float32(1), float32(1), float32(1), float32(1)
	if o.Tint != nil {
		t := render.Premul(o.Tint)
		red, green, blue, alpha = t[0], t[1], t[2], t[3]
	}
	if o.Fade != 0 {
		k := float32(1 - o.Fade)
		red, green, blue, alpha = red*k, green*k, blue*k, alpha*k
	}
	e := 1 / scale // one device pixel of edge to antialias into
	var vs [4]ggfx.Vertex
	for i, p := range [4]Point{
		{X: r.Origin.X - e, Y: r.Origin.Y - e}, {X: r.Origin.X + r.Size.W + e, Y: r.Origin.Y - e},
		{X: r.Origin.X - e, Y: r.Origin.Y + r.Size.H + e}, {X: r.Origin.X + r.Size.W + e, Y: r.Origin.Y + r.Size.H + e},
	} {
		d := Pt(p.X-centre.X, p.Y-centre.Y)
		turned := centre.Add(Pt(d.X*cos-d.Y*sin, d.X*sin+d.Y*cos))
		vs[i] = ggfx.Vertex{
			DstX: float32(c.px(turned.X)), DstY: float32(c.px(turned.Y)),
			SrcX: float32(float64(b.Min.X) + (p.X-at.Origin.X)*kx), SrcY: float32(float64(b.Min.Y) + (p.Y-at.Origin.Y)*ky),
			ColorR: red, ColorG: green, ColorB: blue, ColorA: alpha,
			Custom0: float32(d.X * scale), Custom1: float32(d.Y * scale),
		}
	}
	render.RoundRectImage(c.Image, img, vs, Sz(r.Size.W*scale/2, r.Size.H*scale/2), o.Radius*scale, o.Pixelated)
}
