package ggui

import (
	"math"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/render"
)

// Blur paints a subtree blurred by a Gaussian of sigma logical pixels, as
// Flutter's ImageFiltered does: paint draws into a Layer covering r and as
// far beyond it as the blur spreads, which is blurred and composited onto
// c. Hit regions and semantics are where paint put them. A zero sigma is
// paint with no layer.
//
// The blur runs in two passes, one along each axis. A wide one is worked
// on a copy made smaller first and scaled back up, so its cost does not
// grow with sigma.
func (c *Canvas) Blur(r Rect, sigma float64, paint func(dst *Canvas)) {
	if c == nil || c.Image == nil || !(sigma > 0) || math.IsInf(sigma, 0) {
		paint(c)
		return
	}
	reach := 3 * sigma
	area := c.physical(grow(r, reach)).Intersect(c.root().Image.Bounds())
	if area.Empty() {
		paint(c)
		return
	}
	c.layer(area, paint, func(img *ggfx.Image) {
		c.blurImage(img, c.px(sigma), false)
		op := &ggfx.DrawImageOptions{}
		op.GeoM.Translate(float64(img.Bounds().Min.X), float64(img.Bounds().Min.Y))
		c.Image.DrawImage(img, op)
	})
}

// BackdropBlur blurs what has been painted under r so far by a Gaussian of
// sigma logical pixels, with corners rounded by radius: frosted glass behind
// a panel painted next. It reads the image c draws into, so inside a Layer
// it blurs only what that layer holds. Pixels beyond the edge of what can be
// read repeat its edge, so the backdrop does not darken there. A window's
// screen cannot be read, so the first frame a window paints one in goes
// without it, and the window paints into a backbuffer while they last.
func (c *Canvas) BackdropBlur(r Rect, radius, sigma float64) {
	if c == nil || c.Image == nil || !(sigma > 0) || math.IsInf(sigma, 0) {
		return
	}
	if c.window {
		// A window's screen cannot be read, so the window paints into a
		// backbuffer from the next frame on and this one goes without.
		c.fs().backdrop = true
	}
	if !c.readable() {
		return
	}
	bounds := c.Image.Bounds()
	target := c.physical(r).Intersect(bounds)
	area := c.physical(grow(r, 3*sigma)).Intersect(bounds)
	if target.Empty() || area.Empty() {
		return
	}
	c.scratch(area, func(backdrop *ggfx.Image) {
		op := &ggfx.DrawImageOptions{Blend: ggfx.BlendCopy}
		op.GeoM.Translate(float64(area.Min.X), float64(area.Min.Y))
		backdrop.DrawImage(c.Image.SubImage(area).(*ggfx.Image), op)
		c.blurImage(backdrop, c.px(sigma), true)
		part := backdrop.SubImage(target).(*ggfx.Image)
		if radius > 0 {
			c.maskRoundRect(part, r, radius)
			return
		}
		op = &ggfx.DrawImageOptions{}
		op.GeoM.Translate(float64(target.Min.X), float64(target.Min.Y))
		c.Image.DrawImage(part, op)
	})
}

// blurImage blurs src in place by sigma Image pixels, its passes taking
// turns with one scratch image. clampEdge repeats src's edge beyond it
// rather than reading transparency there.
func (c *Canvas) blurImage(src *ggfx.Image, sigma float64, clampEdge bool) {
	area := src.Bounds()
	k := render.BlurFactor(sigma)
	if k == 1 {
		c.scratch(area, func(across *ggfx.Image) {
			render.BlurPass(across, src, sigma, true, clampEdge)
			render.BlurPass(src, across, sigma, false, clampEdge)
		})
		return
	}
	// src is read once, to shrink it, so its corner holds the first pass.
	small := render.Shrunk(area, k)
	across := src.SubImage(small).(*ggfx.Image)
	c.scratch(small, func(shrunk *ggfx.Image) {
		op := &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear, Blend: ggfx.BlendCopy}
		op.GeoM.Scale(1/float64(k), 1/float64(k))
		op.GeoM.Translate(float64(area.Min.X), float64(area.Min.Y))
		shrunk.DrawImage(src, op)
		render.BlurPass(across, shrunk, sigma/float64(k), true, clampEdge)
		render.BlurPass(shrunk, across, sigma/float64(k), false, clampEdge)
		op = &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear, Blend: ggfx.BlendCopy}
		op.GeoM.Scale(float64(k), float64(k))
		op.GeoM.Translate(float64(area.Min.X), float64(area.Min.Y))
		src.DrawImage(shrunk, op)
	})
}

// grow returns r grown by d on every side.
func grow(r Rect, d float64) Rect {
	return Rct(Pt(r.Origin.X-d, r.Origin.Y-d), Sz(r.Size.W+2*d, r.Size.H+2*d))
}

// BlurWidget paints its child blurred. Build one with Blur.
type BlurWidget struct {
	props property.Owner
	child Widget
	sigma property.Value[float64]
	shown float64
}

// Blur paints child blurred by a Gaussian of sigma logical pixels; zero
// paints it sharp.
//
//	ggui.Blur(content, 0).BindSigma(blur) // a Tween that eases a blur in
func Blur(child Widget, sigma float64) *BlurWidget {
	b := &BlurWidget{child: child}
	b.sigma.Set(sigma)
	return b
}

// Sigma sets the blur's standard deviation in logical pixels.
func (b *BlurWidget) Sigma(v float64) *BlurWidget {
	property.Set(&b.props, &b.sigma, v)
	return b
}

// BindSigma follows a non-nil sigma reader.
func (b *BlurWidget) BindSigma(r Readable[float64]) *BlurWidget {
	property.Bind(&b.props, &b.sigma, r, "BindSigma")
	return b
}

// Baseline implements Baseliner: the child's.
func (b *BlurWidget) Baseline() (float64, bool) { return baselineOf(b.child) }

// Layout implements Widget.
func (b *BlurWidget) Layout(c Constraints, env Env) Size {
	defer b.props.Layout().EndLayout()
	b.shown = b.sigma.Get()
	return b.child.Layout(c, env)
}

// Paint implements Widget.
func (b *BlurWidget) Paint(dst *Canvas, r Rect) {
	dst.Blur(r, b.shown, func(dst *Canvas) { dst.Paint(b.child, r) })
}

// BackdropBlurWidget blurs what lies behind it and paints its child on top:
// a frosted panel. Build one with BackdropBlur.
type BackdropBlurWidget struct {
	props  property.Owner
	child  Widget
	sigma  property.Value[float64]
	shown  float64
	radius float64
}

// BackdropBlur blurs what has been painted under child by a Gaussian of
// sigma logical pixels before painting child, as Flutter's BackdropFilter
// does. Give child a translucent fill to tint the glass. A nil child blurs
// all the space it is given.
//
//	ggui.BackdropBlur(ggui.Box(menu).Fill(glass).Radius(12), 16).Radius(12)
func BackdropBlur(child Widget, sigma float64) *BackdropBlurWidget {
	b := &BackdropBlurWidget{child: child}
	b.sigma.Set(sigma)
	return b
}

// Sigma sets the blur's standard deviation in logical pixels.
func (b *BackdropBlurWidget) Sigma(v float64) *BackdropBlurWidget {
	property.Set(&b.props, &b.sigma, v)
	return b
}

// BindSigma follows a non-nil sigma reader.
func (b *BackdropBlurWidget) BindSigma(r Readable[float64]) *BackdropBlurWidget {
	property.Bind(&b.props, &b.sigma, r, "BindSigma")
	return b
}

// Radius rounds the corners of the blurred area, to match the child's.
func (b *BackdropBlurWidget) Radius(r float64) *BackdropBlurWidget {
	b.radius = r
	return b
}

// Baseline implements Baseliner: the child's.
func (b *BackdropBlurWidget) Baseline() (float64, bool) {
	if b.child == nil {
		return 0, false
	}
	return baselineOf(b.child)
}

// Layout implements Widget.
func (b *BackdropBlurWidget) Layout(c Constraints, env Env) Size {
	defer b.props.Layout().EndLayout()
	b.shown = b.sigma.Get()
	if b.child == nil {
		return c.Constrain(Sz(bounded(c.MaxW, 0), bounded(c.MaxH, 0)))
	}
	return b.child.Layout(c, env)
}

// Paint implements Widget.
func (b *BackdropBlurWidget) Paint(dst *Canvas, r Rect) {
	dst.BackdropBlur(r, b.radius, b.shown)
	if b.child != nil {
		dst.Paint(b.child, r)
	}
}
