package ggui

import (
	"image"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/property"
)

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
	c.layer(area, func(layer *Canvas) { paint(layer.enter(m, inverse, nil)) }, func(img *ggfx.Image) {
		op := &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear}
		op.GeoM.Translate(float64(img.Bounds().Min.X), float64(img.Bounds().Min.Y))
		op.GeoM.Concat(m.geoM(c.Scale()))
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
	area := c.physical(grow(inverse.Bounds(dest), c.dp(2)))
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
	defer t.props.Layout()()
	s := t.scale.Get()
	off := t.offset.Get()
	t.shown = Affine{}.Scale(s, s).Rotate(t.rotate.Get()).Then(t.matrix).Translate(off.X, off.Y)
	return t.child.Layout(c, env)
}

// at returns the transform for the child painted in r.
func (t *TransformWidget) at(r Rect) Affine {
	return t.shown.About(Pt(r.Origin.X+r.Size.W*t.originX, r.Origin.Y+r.Size.H*t.originY))
}

// Paint implements Widget.
func (t *TransformWidget) Paint(dst *Canvas, r Rect) {
	dst.Transform(t.at(r), func(dst *Canvas) { dst.Paint(t.child, r) })
}
