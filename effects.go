package ggui

import (
	"image"
	"image/color"
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
	// What paint draws is smeared as far as reach past it.
	c.fs().drew(r, reach)
	area := c.physical(r.Grow(reach)).Intersect(c.root().Image.Bounds())
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
// read repeat its edge, so the backdrop does not darken there. A window
// reads its screen where the graphics driver can (Metal); elsewhere the
// first frame a window paints one in goes without it, and the window paints
// into a backbuffer while they last.
func (c *Canvas) BackdropBlur(r Rect, radius, sigma float64) {
	if c == nil || c.Image == nil || !(sigma > 0) || math.IsInf(sigma, 0) {
		return
	}
	if c.window {
		// A screen that cannot be read has the window paint into a
		// backbuffer from the next frame on, and this one goes without;
		// one that can needs none (see presentBackdrop).
		c.fs().backdrop = true
	}
	c.fs().drew(r, 0)
	if !c.readable() {
		return
	}
	bounds := c.Image.Bounds()
	target := c.physical(r).Intersect(bounds)
	area := c.physical(r.Grow(3 * sigma)).Intersect(bounds)
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

// OpacityWidget paints its child as a whole at an opacity: overlapping parts
// of the child do not show through each other, as they would if each were
// faded on its own. Build one with Opacity.
type OpacityWidget struct {
	props   property.Owner
	child   Widget
	opacity property.Value[float64]
	shown   float64
}

// Opacity paints child at opacity, from 0, invisible, to 1, as it is. An
// invisible child is still laid out and still takes input, as Flutter's
// Opacity does; wrap it in Inert as well to stop that.
func Opacity(child Widget, opacity float64) *OpacityWidget {
	o := &OpacityWidget{child: child}
	o.opacity.Set(opacity)
	return o
}

// Opacity sets the opacity, clamped to [0, 1].
func (o *OpacityWidget) Opacity(v float64) *OpacityWidget {
	property.Set(&o.props, &o.opacity, v)
	return o
}

// BindOpacity follows a non-nil opacity reader, a Tween say.
func (o *OpacityWidget) BindOpacity(r Readable[float64]) *OpacityWidget {
	property.Bind(&o.props, &o.opacity, r, "BindOpacity")
	return o
}

// Baseline implements Baseliner: the child's.
func (o *OpacityWidget) Baseline() (float64, bool) { return baselineOf(o.child) }

// Layout implements Widget.
func (o *OpacityWidget) Layout(c Constraints, env Env) Size {
	defer o.props.Layout().EndLayout()
	o.shown = clamp(o.opacity.Get(), 0, 1)
	return o.child.Layout(c, env)
}

// Paint implements Widget. A fully opaque child paints directly, with no
// layer, and an invisible one with no image, so that it still registers its
// input and semantics.
func (o *OpacityWidget) Paint(dst *Canvas, r Rect) {
	switch {
	case o.shown >= 1 || dst == nil:
		dst.Paint(o.child, r)
		return
	case o.shown <= 0:
		dst.into(nil).Paint(o.child, r)
		return
	}
	dst.Layer(LayerOptions{Fade: 1 - o.shown}, func(layer *Canvas) { layer.Paint(o.child, r) })
}

// ShadowStyle describes an outer, rounded-rectangle shadow in logical pixels.
// Blur is the feather distance on either side of the edge, not Gaussian sigma.
// Positive Spread grows the silhouette; negative Spread shrinks it. Nil Color
// disables the shadow. Shadows do not change layout or pointer hit regions.
type ShadowStyle struct {
	Offset       Point
	Blur, Spread float64
	Color        color.Color
}

func (c *Canvas) shadowGeometry(r Rect, radius float64, s ShadowStyle) (render.Shadow, bool) {
	for _, v := range [...]float64{r.Origin.X, r.Origin.Y, r.Size.W, r.Size.H, radius, s.Offset.X, s.Offset.Y, s.Blur, s.Spread, c.Scale()} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return render.Shadow{}, false
		}
	}
	if r.Size.W <= 0 || r.Size.H <= 0 {
		return render.Shadow{}, false
	}
	w, h := r.Size.W+2*s.Spread, r.Size.H+2*s.Spread
	if w <= 0 || h <= 0 {
		return render.Shadow{}, false
	}
	scale := c.Scale()
	feather := max(max(s.Blur, 0)*scale, .5)
	center := r.Origin.Add(s.Offset).Add(Pt(r.Size.W/2, r.Size.H/2))
	cx, cy := c.px(center.X), c.px(center.Y)
	hw, hh := w*scale/2, h*scale/2
	// Clip before converting to integers: large offscreen shadows need no large
	// intermediate texture or unbounded draw quad.
	bounds, ok := render.Bounds(c.Image.Bounds(), cx, cy, hw, hh, feather)
	if !ok {
		return render.Shadow{}, false
	}
	// The corner never exceeds half the rect, grows with the spread and, once
	// scaled, never exceeds half the silhouette.
	corner := max(min(max(radius, 0), r.Size.W/2, r.Size.H/2)+s.Spread, 0)
	return render.Shadow{
		Bounds:  bounds,
		Centre:  Pt(cx, cy),
		Half:    Sz(hw, hh),
		Radius:  float32(min(corner*scale, hw, hh)),
		Feather: float32(feather),
	}, true
}

// Shadow draws a soft rounded-rectangle silhouette behind a surface. Paint the
// surface afterward. It uses one shared shader, one DrawTrianglesShader call,
// and no intermediate images or CPU rasterization. This distance-field feather
// approximates UI shadows; it is not an image blur. Parent clipping is respected.
func (c *Canvas) Shadow(r Rect, radius float64, s ShadowStyle) {
	if c == nil || c.Image == nil || s.Color == nil {
		return
	}
	tint := render.Premul(s.Color)
	if tint[3] == 0 {
		return
	}
	// Seen or not, the shadow is where the widget painting draws, as far
	// past r as it spreads and blurs.
	c.fs().drew(Rct(r.Origin.Add(s.Offset), r.Size), max(s.Spread, 0)+max(s.Blur, 0)+1)
	g, ok := c.shadowGeometry(r, radius, s)
	if !ok {
		return
	}
	render.DrawShadow(c.Image, g, tint)
}

// FadeEdges paints paint into a layer over r and draws it with its ends
// along an axis faded out: the first lead logical pixels from clear to
// opaque, and the last trail from opaque to clear, across x when
// horizontal and across y otherwise. What is under r shows through the
// faded ends, as it would through the content's own transparency.
func (c *Canvas) FadeEdges(r Rect, horizontal bool, lead, trail float64, paint func(dst *Canvas)) {
	if lead <= 0 && trail <= 0 || c == nil || c.Image == nil || c.Image.Bounds().Empty() {
		paint(c)
		return
	}
	c.layer(c.Image.Bounds(), paint, func(img *ggfx.Image) {
		ramp := fadeRamp()
		// A ramp of fadeRampLength pixels, clear to opaque, stretched over
		// an end and turned to face into r.
		end := func(from, length float64, flip bool) {
			if length <= 0 {
				return
			}
			op := &ggfx.DrawImageOptions{Blend: ggfx.BlendDestinationIn, Filter: ggfx.FilterLinear}
			along, across := c.px(length)/fadeRampLength, c.px(pick(horizontal, r.Size.H, r.Size.W))
			if flip {
				along = -along
			}
			op.GeoM.Scale(along, across)
			if horizontal {
				op.GeoM.Translate(c.px(from), c.px(r.Origin.Y))
			} else {
				// The ramp runs across x; turned a quarter clockwise, it
				// runs down y, from r's right edge back over its width.
				op.GeoM.Rotate(math.Pi / 2)
				op.GeoM.Translate(c.px(r.Origin.X+r.Size.W), c.px(from))
			}
			img.DrawImage(ramp, op)
		}
		start, stop := pick(horizontal, r.Origin.X, r.Origin.Y), pick(horizontal, r.Origin.X+r.Size.W, r.Origin.Y+r.Size.H)
		end(start, min(lead, stop-start), false)
		end(stop, min(trail, stop-start), true)
		op := &ggfx.DrawImageOptions{}
		op.GeoM.Translate(float64(img.Bounds().Min.X), float64(img.Bounds().Min.Y))
		c.Image.DrawImage(img, op)
	})
}

const fadeRampLength = 64

// rampImage is the ramp FadeEdges masks with: a row of white from clear to
// opaque, made once, alpha-only.
var rampImage *ggfx.Image

func fadeRamp() *ggfx.Image {
	if rampImage == nil {
		src := image.NewAlpha(image.Rect(0, 0, fadeRampLength, 1))
		for x := range fadeRampLength {
			src.Pix[x] = uint8((float64(x) + .5) / fadeRampLength * 255)
		}
		rampImage = ggfx.NewImageFromImageWithOptions(src, &ggfx.NewImageFromImageOptions{Format: ggfx.PixelFormatAlpha})
	}
	return rampImage
}

// Transform paints through m: what paint draws is moved by m, and so is the
// input its hit regions receive. A pointer is matched through the inverse
// of m, and the handler is told where it landed in the coordinates it
// painted in, so a slider turned on its side still reads its own x. The
// semantics nodes paint registers are reported at the window Rect holding
// them.
//
// paint draws into a Layer the size of the window, untransformed, which is
// then composited through m; what lies outside the window before m moves
// it is not drawn. A transform with no inverse, such as a zero scale,
// draws nothing and takes no input.
func (c *Canvas) Transform(m Affine, paint func(dst *Canvas)) {
	if c == nil {
		paint(nil)
		return
	}
	if m.IsIdentity() {
		paint(c)
		return
	}
	inverse, ok := m.Invert()
	if !ok {
		// Nothing to see and nowhere to point, but the subtree still
		// paints, so what it retains survives a scale passing zero. The
		// space it paints in takes no point, and an overlay scheduled in
		// it goes through m too, and so shows nothing either.
		c.layer(image.Rectangle{}, func(layer *Canvas) {
			paint(layer.Inert().enter(m, Affine{}, func(Point) bool { return false }))
		}, func(*ggfx.Image) {})
		return
	}
	area, visible := c.transformArea(inverse)
	if !visible {
		// Nothing of the layer would land on c, but the subtree still
		// registers its input and semantics, which the clip then trims.
		paint(c.into(nil).enter(m, inverse, nil))
		return
	}
	// The transformed subtree lands within area, which is where the
	// widget painting draws: what it drew inside is in other coordinates.
	c.fs().drew(c.logicalRect(area), 0)
	c.layer(area, func(layer *Canvas) { paint(layer.enter(m, inverse, nil)) }, func(img *ggfx.Image) {
		op := &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear}
		op.GeoM.Translate(float64(img.Bounds().Min.X), float64(img.Bounds().Min.Y))
		op.GeoM.Concat(affineGeoM(m, c.Scale()))
		c.Image.DrawImage(img, op)
	})
}

// transformArea returns the part of a layer that m, the inverse of which is
// given, takes onto what c can show, with a pixel to spare for filtering,
// and false when none of it shows. Only that part is cleared, painted and
// composited.
func (c *Canvas) transformArea(inverse Affine) (image.Rectangle, bool) {
	dest := Rct(Point{}, c.Size())
	if c.clipped {
		dest = c.clip
	}
	if c.Image != nil {
		b := c.Image.Bounds()
		dest = dest.Intersect(Rct(Pt(c.dp(float64(b.Min.X)), c.dp(float64(b.Min.Y))), Sz(c.dp(float64(b.Dx())), c.dp(float64(b.Dy())))))
	}
	if dest.Empty() {
		return image.Rectangle{}, false
	}
	area := c.physical(inverse.Bounds(dest).Grow(c.dp(2)))
	if window := c.root().Image; window != nil {
		area = area.Intersect(window.Bounds())
	}
	return area, !area.Empty()
}

// TransformWidget paints its child scaled, turned and moved, about an origin
// within it, and routes input through the same transform. Layout is not
// affected: the child takes the space it would without it, as Flutter's
// Transform does. Build one with Transform.
type TransformWidget struct {
	props   property.Owner
	child   Widget
	rotate  property.Value[float64]
	scale   property.Value[float64]
	offset  property.Value[Point]
	originX float64
	originY float64
	matrix  Affine
	shown   Affine // rotate, scale and offset as of the last layout
}

// Transform wraps child in a transform, the identity until Rotate, Scale,
// Translate or Matrix say otherwise. The child is scaled, then turned, then
// moved, about its center unless Origin says otherwise:
//
//	ggui.Transform(card).Rotate(math.Pi / 12).Scale(1.1)
func Transform(child Widget) *TransformWidget {
	t := &TransformWidget{child: child, originX: 0.5, originY: 0.5}
	t.scale.Set(1)
	return t
}

// Rotate turns the child by rad radians, clockwise.
func (t *TransformWidget) Rotate(rad float64) *TransformWidget {
	property.Set(&t.props, &t.rotate, rad)
	return t
}

// BindRotate follows a non-nil angle reader, an animation say.
func (t *TransformWidget) BindRotate(r Readable[float64]) *TransformWidget {
	property.Bind(&t.props, &t.rotate, r, "BindRotate")
	return t
}

// Scale sizes the child by s; 1 leaves it as it is.
func (t *TransformWidget) Scale(s float64) *TransformWidget {
	property.Set(&t.props, &t.scale, s)
	return t
}

// BindScale follows a non-nil scale reader.
func (t *TransformWidget) BindScale(r Readable[float64]) *TransformWidget {
	property.Bind(&t.props, &t.scale, r, "BindScale")
	return t
}

// Translate moves the child by dx, dy logical pixels. It is Offset(Pt(dx, dy)).
func (t *TransformWidget) Translate(dx, dy float64) *TransformWidget { return t.Offset(Pt(dx, dy)) }

// Offset moves the child by p.
func (t *TransformWidget) Offset(p Point) *TransformWidget {
	property.Set(&t.props, &t.offset, p)
	return t
}

// BindOffset follows a non-nil offset reader.
func (t *TransformWidget) BindOffset(r Readable[Point]) *TransformWidget {
	property.Bind(&t.props, &t.offset, r, "BindOffset")
	return t
}

// Origin sets the point scaling and turning happen about, as fractions of
// the child's size: (0, 0) is its top-left corner, (0.5, 0.5) its center,
// the default.
func (t *TransformWidget) Origin(x, y float64) *TransformWidget {
	defer property.Watch(&t.props, &t.originX)()
	defer property.Watch(&t.props, &t.originY)()
	t.originX, t.originY = x, y
	return t
}

// Matrix applies m after scaling and turning and before the move, about the
// origin: a skew, or a transform built elsewhere.
func (t *TransformWidget) Matrix(m Affine) *TransformWidget {
	defer property.Watch(&t.props, &t.matrix)()
	t.matrix = m
	return t
}

// Baseline implements Baseliner: the child's, where layout put it. A
// transform moves what is painted, not where its neighbours align.
func (t *TransformWidget) Baseline() (float64, bool) { return baselineOf(t.child) }

// Layout implements Widget.
func (t *TransformWidget) Layout(c Constraints, env Env) Size {
	defer t.props.Layout().EndLayout()
	s := t.scale.Get()
	off := t.offset.Get()
	t.shown = Affine{}.Scale(s, s).Rotate(t.rotate.Get()).Then(t.matrix).Translate(off.X, off.Y)
	return t.child.Layout(c, env)
}

// Paint implements Widget.
func (t *TransformWidget) Paint(dst *Canvas, r Rect) {
	dst.Transform(t.at(r), func(dst *Canvas) { dst.Paint(t.child, r) })
}

// affineGeoM returns m as a ggfx transform on Image pixels, at scale
// pixels to a logical one.
func affineGeoM(m Affine, scale float64) ggfx.GeoM {
	a, b, c, d, tx, ty := m.Matrix()
	var g ggfx.GeoM
	g.SetElement(0, 0, a)
	g.SetElement(0, 1, c)
	g.SetElement(0, 2, tx*scale)
	g.SetElement(1, 0, b)
	g.SetElement(1, 1, d)
	g.SetElement(1, 2, ty*scale)
	return g
}

// paintSpace is the coordinate space a Canvas paints in when it is not the
// window's: inside a Transform, whose child paints untransformed, or inside
// a shape clip, which hit-tests against a shape rather than a Rect. A hit
// region keeps the space it was registered in, so input finds it through the
// transform and is told where it landed in the child's own coordinates.
// A nil space is the window's.
type paintSpace struct {
	parent *paintSpace
	// toParent takes this space's points to the parent's, and toLocal back.
	toParent, toLocal Affine
	// outer is the parent's clip where the space began, in the parent's
	// coordinates, when clipped is set. A transformed space starts
	// unclipped, so the clip it was painted under is kept here.
	outer   Rect
	clipped bool
	// inside reports whether a point in this space is within the shape a
	// ClipPath or ClipOval clips to; nil takes every point.
	inside func(Point) bool
}

// local returns where the window point p is in s, ignoring clips.
func (s *paintSpace) local(p Point) Point {
	if s == nil {
		return p
	}
	return s.toLocal.Apply(s.parent.local(p))
}

// reach returns where the window point p is in s, and false when a clip or
// a shape between the window and s hides it.
func (s *paintSpace) reach(p Point) (Point, bool) {
	if s == nil {
		return p, true
	}
	q, ok := s.parent.reach(p)
	if !ok || s.clipped && !s.outer.Contains(q) {
		return q, false
	}
	q = s.toLocal.Apply(q)
	if s.inside != nil && !s.inside(q) {
		return q, false
	}
	return q, true
}

// affine returns the transform from s's coordinates to the window's,
// leaving out the clips and shapes on the way.
func (s *paintSpace) affine() Affine {
	var m Affine
	for ; s != nil; s = s.parent {
		m = m.Then(s.toParent)
	}
	return m
}

// window returns the window Rect that holds r in s, cut to the clips
// between; full skips the cuts.
func (s *paintSpace) window(r Rect, full bool) Rect {
	for ; s != nil; s = s.parent {
		r = s.toParent.Bounds(r)
		if s.clipped && !full {
			r = r.Intersect(s.outer)
		}
	}
	return r
}

// toWindow returns the window Rect that holds the Rect r in c's space.
func (c *Canvas) toWindow(r Rect) Rect {
	if c == nil || c.space == nil {
		return r
	}
	return c.space.window(r, true)
}

// enter returns a Canvas like c that paints in a space reached from c's by
// m, in which a point counts only when inside accepts it. A space that moves
// nothing keeps c's clip, which is already in its coordinates; one that
// does starts unclipped and keeps c's clip as its outer bound.
func (c *Canvas) enter(m, inverse Affine, inside func(Point) bool) *Canvas {
	child := c.derive()
	s := &paintSpace{parent: c.space, toParent: m, toLocal: inverse, inside: inside}
	if !m.IsIdentity() {
		s.outer, s.clipped = c.clip, c.clipped
		child.clip, child.clipped = Rect{}, false
	}
	child.space = s
	return child
}

// placeRegion finishes a region registered in s: rect becomes the window
// Rect that holds it and local keeps it as it was painted. It reports false
// for a region left with no area and no key handler.
func (s *paintSpace) placeRegion(h *hitRegion) bool {
	h.space, h.local, h.origin = s, h.rect, h.full.Origin
	h.full = s.window(h.full, true)
	h.rect = s.window(h.rect, false)
	if h.local.Empty() || h.rect.Empty() {
		if h.key == nil {
			return false
		}
		h.rect = Rect{Origin: h.full.Origin}
	}
	return true
}

// at returns the window point p in the coordinates r was painted in, which
// is where its handler expects an event.
func (r *hitRegion) at(p Point) Point { return r.space.local(p) }
