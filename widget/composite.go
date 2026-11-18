package widget

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/pkg/gem"
	"github.com/ironpark/ggui/property"
)

// Widgets that paint a subtree as a whole, through the Canvas's effects:
// blurred, over a blurred backdrop, faded, transformed or cut to a shape.
// None changes layout.

// BlurWidget paints its child blurred. Build one with Blur.
type BlurWidget struct {
	props property.Owner
	child ggui.Widget
	sigma property.Value[float64]
	shown float64
}

// Blur paints child blurred by a Gaussian of sigma logical pixels; zero
// paints it sharp.
//
//	widget.Blur(content, 0).BindSigma(blur) // a Tween that eases a blur in
func Blur(child ggui.Widget, sigma float64) *BlurWidget {
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
func (b *BlurWidget) BindSigma(r ggui.Readable[float64]) *BlurWidget {
	property.Bind(&b.props, &b.sigma, r, "BindSigma")
	return b
}

// Baseline implements Baseliner: the child's.
func (b *BlurWidget) Baseline() (float64, bool) { return ggui.BaselineOf(b.child) }

// Layout implements Widget.
func (b *BlurWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout().EndLayout()
	b.shown = b.sigma.Get()
	return b.child.Layout(c, env)
}

// Paint implements Widget.
func (b *BlurWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Blur(r, b.shown, func(dst *ggui.Canvas) { dst.Paint(b.child, r) })
}

// BackdropBlurWidget blurs what lies behind it and paints its child on top:
// a frosted panel. Build one with BackdropBlur.
type BackdropBlurWidget struct {
	props  property.Owner
	child  ggui.Widget
	sigma  property.Value[float64]
	shown  float64
	radius float64
}

// BackdropBlur blurs what has been painted under child by a Gaussian of
// sigma logical pixels before painting child, as Flutter's BackdropFilter
// does. Give child a translucent fill to tint the glass. A nil child blurs
// all the space it is given.
//
//	widget.BackdropBlur(widget.Box(menu).Fill(glass).Radius(12), 16).Radius(12)
func BackdropBlur(child ggui.Widget, sigma float64) *BackdropBlurWidget {
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
func (b *BackdropBlurWidget) BindSigma(r ggui.Readable[float64]) *BackdropBlurWidget {
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
	return ggui.BaselineOf(b.child)
}

// Layout implements Widget.
func (b *BackdropBlurWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout().EndLayout()
	b.shown = b.sigma.Get()
	if b.child == nil {
		return c.Constrain(ggui.Sz(gem.Bounded(c.MaxW, 0), gem.Bounded(c.MaxH, 0)))
	}
	return b.child.Layout(c, env)
}

// Paint implements Widget.
func (b *BackdropBlurWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
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
	child   ggui.Widget
	opacity property.Value[float64]
	shown   float64
}

// Opacity paints child at opacity, from 0, invisible, to 1, as it is. An
// invisible child is still laid out and still takes input, as Flutter's
// Opacity does; wrap it in Inert as well to stop that.
func Opacity(child ggui.Widget, opacity float64) *OpacityWidget {
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
func (o *OpacityWidget) BindOpacity(r ggui.Readable[float64]) *OpacityWidget {
	property.Bind(&o.props, &o.opacity, r, "BindOpacity")
	return o
}

// Baseline implements Baseliner: the child's.
func (o *OpacityWidget) Baseline() (float64, bool) { return ggui.BaselineOf(o.child) }

// Layout implements Widget.
func (o *OpacityWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer o.props.Layout().EndLayout()
	o.shown = gem.Clamp(o.opacity.Get(), 0, 1)
	return o.child.Layout(c, env)
}

// Paint implements Widget. A fully opaque child paints directly, with no
// layer, and an invisible one with no image, so that it still registers its
// input and semantics.
func (o *OpacityWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	switch {
	case o.shown >= 1 || dst == nil:
		dst.Paint(o.child, r)
		return
	case o.shown <= 0:
		dst.Invisible().Paint(o.child, r)
		return
	}
	dst.Layer(ggui.LayerOptions{Fade: 1 - o.shown}, func(layer *ggui.Canvas) { layer.Paint(o.child, r) })
}

// TransformWidget paints its child scaled, turned and moved, about an origin
// within it, and routes input through the same transform. Layout is not
// affected: the child takes the space it would without it, as Flutter's
// Transform does. Build one with Transform.
type TransformWidget struct {
	props   property.Owner
	child   ggui.Widget
	rotate  property.Value[float64]
	scale   property.Value[float64]
	offset  property.Value[ggui.Point]
	originX float64
	originY float64
	matrix  ggui.Affine
	shown   ggui.Affine // rotate, scale and offset as of the last layout
}

// Transform wraps child in a transform, the identity until Rotate, Scale,
// Translate or Matrix say otherwise. The child is scaled, then turned, then
// moved, about its center unless Origin says otherwise:
//
//	widget.Transform(card).Rotate(math.Pi / 12).Scale(1.1)
func Transform(child ggui.Widget) *TransformWidget {
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
func (t *TransformWidget) BindRotate(r ggui.Readable[float64]) *TransformWidget {
	property.Bind(&t.props, &t.rotate, r, "BindRotate")
	return t
}

// Scale sizes the child by s; 1 leaves it as it is.
func (t *TransformWidget) Scale(s float64) *TransformWidget {
	property.Set(&t.props, &t.scale, s)
	return t
}

// BindScale follows a non-nil scale reader.
func (t *TransformWidget) BindScale(r ggui.Readable[float64]) *TransformWidget {
	property.Bind(&t.props, &t.scale, r, "BindScale")
	return t
}

// Translate moves the child by dx, dy logical pixels. It is Offset(Pt(dx, dy)).
func (t *TransformWidget) Translate(dx, dy float64) *TransformWidget {
	return t.Offset(ggui.Pt(dx, dy))
}

// Offset moves the child by p.
func (t *TransformWidget) Offset(p ggui.Point) *TransformWidget {
	property.Set(&t.props, &t.offset, p)
	return t
}

// BindOffset follows a non-nil offset reader.
func (t *TransformWidget) BindOffset(r ggui.Readable[ggui.Point]) *TransformWidget {
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
func (t *TransformWidget) Matrix(m ggui.Affine) *TransformWidget {
	defer property.Watch(&t.props, &t.matrix)()
	t.matrix = m
	return t
}

// Baseline implements Baseliner: the child's, where layout put it. A
// transform moves what is painted, not where its neighbours align.
func (t *TransformWidget) Baseline() (float64, bool) { return ggui.BaselineOf(t.child) }

// Layout implements Widget.
func (t *TransformWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout().EndLayout()
	s := t.scale.Get()
	off := t.offset.Get()
	t.shown = ggui.Affine{}.Scale(s, s).Rotate(t.rotate.Get()).Then(t.matrix).Translate(off.X, off.Y)
	return t.child.Layout(c, env)
}

// Paint implements Widget.
func (t *TransformWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Transform(t.at(r), func(dst *ggui.Canvas) { dst.Paint(t.child, r) })
}

// at returns the transform for the child painted in r.
func (t *TransformWidget) at(r ggui.Rect) ggui.Affine {
	return t.shown.About(ggui.Pt(r.Origin.X+r.Size.W*t.originX, r.Origin.Y+r.Size.H*t.originY))
}

// ClipPathWidget clips its child to a path built for the Rect it paints in,
// input included. Build one with ClipPath, ClipOval or ClipRoundRect.
type ClipPathWidget struct {
	child  ggui.Widget
	shape  func(r ggui.Rect) *ggui.Path
	oval   bool
	round  bool
	radius float64
}

// ClipPath clips child to the path shape returns for the Rect the child
// paints in, in the same coordinates:
//
//	widget.ClipPath(photo, func(r ggui.Rect) *ggui.Path { return star(r) })
func ClipPath(child ggui.Widget, shape func(r ggui.Rect) *ggui.Path) *ClipPathWidget {
	return &ClipPathWidget{child: child, shape: shape}
}

// ClipOval clips child to the ellipse inscribed in its Rect, a circle when
// the child is square.
func ClipOval(child ggui.Widget) *ClipPathWidget { return &ClipPathWidget{child: child, oval: true} }

// ClipRoundRect clips child to its Rect with the corners rounded by radius,
// clamped to half the shorter side: content that runs to the edge of a
// rounded surface, a photo or a table, kept inside its corners. Input is
// clipped to the Rect, square corners and all; drawing it costs one mask
// rather than a path's.
func ClipRoundRect(child ggui.Widget, radius float64) *ClipPathWidget {
	return &ClipPathWidget{child: child, round: true, radius: radius}
}

// Baseline implements Baseliner: the child's.
func (w *ClipPathWidget) Baseline() (float64, bool) { return ggui.BaselineOf(w.child) }

// Layout implements Widget.
func (w *ClipPathWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return w.child.Layout(c, env)
}

// Paint implements Widget.
func (w *ClipPathWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	paint := func(dst *ggui.Canvas) { dst.Paint(w.child, r) }
	if w.oval {
		dst.ClipOval(r, paint)
		return
	}
	if w.round {
		dst.ClipRoundRect(r, min(w.radius, r.Size.W/2, r.Size.H/2), paint)
		return
	}
	dst.ClipPath(w.shape(r), paint)
}
