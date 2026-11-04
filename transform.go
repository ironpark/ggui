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

// at returns the transform for the child painted in r.
func (t *TransformWidget) at(r Rect) Affine {
	return t.shown.About(Pt(r.Origin.X+r.Size.W*t.originX, r.Origin.Y+r.Size.H*t.originY))
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

// contains reports whether the window point p lands on r.
func (r *hitRegion) contains(p Point) bool {
	if !r.rect.Contains(p) {
		return false
	}
	if r.space == nil {
		return true
	}
	q, ok := r.space.reach(p)
	return ok && r.local.Contains(q)
}

// at returns the window point p in the coordinates r was painted in, which
// is where its handler expects an event.
func (r *hitRegion) at(p Point) Point { return r.space.local(p) }

// offset returns the window point p from the corner of the Rect r was
// registered for, in the coordinates it was painted in: where on a drag
// source the pointer took hold, or where in a drop zone it let go.
func (r *hitRegion) offset(p Point) Point {
	if r.space == nil {
		return p.Sub(r.full.Origin)
	}
	return r.at(p).Sub(r.origin)
}

// pointerAt is ev as r's handler sees it.
func (r *hitRegion) pointerAt(ev PointerEvent) PointerEvent {
	if r.space == nil {
		return ev
	}
	if ev.Scroll != (Point{}) {
		// A delta is a direction, so it turns and scales but does not move:
		// a scroll view turned on its side scrolls along its own axis, and
		// a pan under a zoom follows the finger.
		ev.Scroll = r.at(ev.Scroll).Sub(r.at(Point{}))
	}
	ev.Pos = r.at(ev.Pos)
	return ev
}
