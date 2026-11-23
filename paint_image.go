package ggui

import (
	"image/color"
	"math"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/render"
)

// ImageFit says how an image is placed in a box of another shape.
type ImageFit int

const (
	FitContain ImageFit = iota // scaled to fit inside, keeping its aspect ratio
	FitCover                   // scaled to cover the box, keeping its aspect ratio, and clipped
	FitFill                    // stretched to the box
	FitNone                    // drawn at its natural size, centered, and clipped
)

// Place returns the Rect an image of the natural size n is drawn in to fit
// the box r this way. An image with no size fills r.
func (f ImageFit) Place(n Size, r Rect) Rect {
	if n.W == 0 || n.H == 0 {
		return r
	}
	var s Size
	switch f {
	case FitFill:
		return r
	case FitNone:
		s = n
	case FitCover:
		k := max(r.Size.W/n.W, r.Size.H/n.H)
		s = Sz(n.W*k, n.H*k)
	default:
		k := min(r.Size.W/n.W, r.Size.H/n.H)
		s = Sz(n.W*k, n.H*k)
	}
	return Rct(Pt(r.Origin.X+(r.Size.W-s.W)/2, r.Origin.Y+(r.Size.H-s.H)/2), s)
}

// ImageOptions says how DrawImage draws an image. The zero value fits it
// inside the Rect, opaque, upright and smoothed.
type ImageOptions struct {
	// Fit places the image in the Rect; what falls outside is clipped.
	Fit ImageFit
	// Radius rounds the Rect's corners, antialiased, and the image is cut
	// to them: at half the side of a square, a round photo.
	Radius float64
	// Fade is how far the image fades out: 0 draws it opaque, 1 not at all.
	Fade float64
	// Tint multiplies every pixel, which is how a white icon takes a
	// colour; nil leaves them.
	Tint color.Color
	// Rotation turns the image clockwise about the centre of where it is
	// drawn, in radians.
	Rotation float64
	// Pixelated scales by the nearest pixel instead of smoothing.
	Pixelated bool
}

// DrawImage draws img in the logical Rect r, as o says.
func (c *Canvas) DrawImage(img *ggfx.Image, r Rect, o ImageOptions) {
	if c == nil || c.Image == nil || img == nil || o.Fade >= 1 {
		return
	}
	b := img.Bounds()
	if b.Empty() {
		return
	}
	at := o.Fit.Place(Sz(float64(b.Dx()), float64(b.Dy())), r)
	if o.Radius > 0 {
		c.shadeImage(img, r, at, o)
		return
	}
	if at != r {
		c = c.Clip(r)
	}
	c.drawImage(img, at, o)
}

// drawImage draws img stretched over the logical Rect r.
func (c *Canvas) drawImage(img *ggfx.Image, r Rect, o ImageOptions) {
	b := img.Bounds()
	// A turned image reaches as far as its corners do at this angle: no
	// further than r when hardly turned, so that a spinner turning from
	// upright stays within what the frame before it drew.
	reach := 0.0
	if o.Rotation != 0 {
		sin, cos := math.Abs(math.Sin(o.Rotation)), math.Abs(math.Cos(o.Rotation))
		w, h := r.Size.W, r.Size.H
		reach = max((w*cos+h*sin-w)/2, (w*sin+h*cos-h)/2, 0)
	}
	if !c.visiblePaintBounds(r, reach) {
		return
	}
	op := &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear}
	if o.Pixelated {
		op.Filter = ggfx.FilterNearest
	}
	w, h := float64(b.Dx()), float64(b.Dy())
	if o.Rotation != 0 {
		op.GeoM.Translate(-w/2, -h/2)
		op.GeoM.Scale(r.Size.W/w, r.Size.H/h)
		op.GeoM.Rotate(o.Rotation)
		op.GeoM.Concat(c.Geo(r.Center()))
	} else {
		op.GeoM.Scale(r.Size.W/w, r.Size.H/h)
		op.GeoM.Concat(c.Geo(r.Origin))
	}
	if o.Tint != nil {
		op.ColorScale.ScaleWithColor(o.Tint)
	}
	if o.Fade != 0 {
		op.ColorScale.ScaleAlpha(float32(1 - o.Fade))
	}
	c.Image.DrawImage(img, op)
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
	vis := r.Intersect(at)
	if vis.Empty() {
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
	// Pad by e only on edges where the placed image reaches r's boundary;
	// letterbox edges strictly inside r stay exact so edge texels do not smear
	// across the empty letterbox region.
	x0, y0 := vis.Origin.X, vis.Origin.Y
	x1, y1 := vis.Origin.X+vis.Size.W, vis.Origin.Y+vis.Size.H
	if at.Origin.X <= r.Origin.X {
		x0 -= e
	}
	if at.Origin.Y <= r.Origin.Y {
		y0 -= e
	}
	if at.Origin.X+at.Size.W >= r.Origin.X+r.Size.W {
		x1 += e
	}
	if at.Origin.Y+at.Size.H >= r.Origin.Y+r.Size.H {
		y1 += e
	}
	var vs [4]ggfx.Vertex
	for i, p := range [4]Point{
		{X: x0, Y: y0}, {X: x1, Y: y0},
		{X: x0, Y: y1}, {X: x1, Y: y1},
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
