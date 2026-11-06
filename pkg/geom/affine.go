// Package geom is 2D geometry beyond boxes: affine transforms of points
// and rectangles, and outlines of lines and Bézier curves that can say
// whether they hold a point and what they cover. It builds on
// pkg/layout's Point and Rect, which ggui's own are.
package geom

import (
	"iter"
	"math"

	"github.com/ironpark/ggui/pkg/layout"
)

type (
	// Point is layout's Point, which ggui's is.
	Point = layout.Point
	// Rect is layout's Rect, which ggui's is.
	Rect = layout.Rect
)

// Affine is a 2D affine transform of logical points. The zero value is the
// identity, and each method returns the transform followed by one more
// step, as ggfx.GeoM's do: Affine{}.Scale(2, 2).Translate(10, 0) scales a
// point and then moves it.
type Affine struct {
	// The matrix [a c tx; b d ty] is stored with a and d less one, so the
	// zero value is the identity.
	a1, b, c, d1, tx, ty float64
}

// Translate returns m followed by a move of dx, dy.
func (m Affine) Translate(dx, dy float64) Affine {
	m.tx += dx
	m.ty += dy
	return m
}

// Scale returns m followed by a scale of sx, sy about the origin.
func (m Affine) Scale(sx, sy float64) Affine {
	return m.Then(Affine{a1: sx - 1, d1: sy - 1})
}

// Rotate returns m followed by a turn of rad radians about the origin,
// clockwise on screen, where y grows downwards.
func (m Affine) Rotate(rad float64) Affine {
	sin, cos := math.Sincos(rad)
	return m.Then(Affine{a1: cos - 1, b: sin, c: -sin, d1: cos - 1})
}

// Skew returns m followed by a shear of sx radians along x and sy along y.
func (m Affine) Skew(sx, sy float64) Affine {
	return m.Then(Affine{b: math.Tan(sy), c: math.Tan(sx)})
}

// About returns m applied about p rather than the origin: moved so that p
// is at the origin, transformed, and moved back.
func (m Affine) About(p Point) Affine {
	return Affine{}.Translate(-p.X, -p.Y).Then(m).Translate(p.X, p.Y)
}

// Then returns m followed by n.
func (m Affine) Then(n Affine) Affine {
	ma, md := m.a1+1, m.d1+1
	na, nd := n.a1+1, n.d1+1
	return Affine{
		a1: na*ma + n.c*m.b - 1,
		b:  n.b*ma + nd*m.b,
		c:  na*m.c + n.c*md,
		d1: n.b*m.c + nd*md - 1,
		tx: na*m.tx + n.c*m.ty + n.tx,
		ty: n.b*m.tx + nd*m.ty + n.ty,
	}
}

// Apply returns where m takes p.
func (m Affine) Apply(p Point) Point {
	return Point{X: (m.a1+1)*p.X + m.c*p.Y + m.tx, Y: m.b*p.X + (m.d1+1)*p.Y + m.ty}
}

// IsIdentity reports whether m leaves every point where it is.
func (m Affine) IsIdentity() bool { return m == Affine{} }

// Invert returns the transform that undoes m, and false when m collapses
// the plane onto a line or a point and so has none.
func (m Affine) Invert() (Affine, bool) {
	a, d := m.a1+1, m.d1+1
	det := a*d - m.b*m.c
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return Affine{}, false
	}
	ia, ib, ic, id := d/det, -m.b/det, -m.c/det, a/det
	return Affine{
		a1: ia - 1, b: ib, c: ic, d1: id - 1,
		tx: -(ia*m.tx + ic*m.ty),
		ty: -(ib*m.tx + id*m.ty),
	}, true
}

// Bounds returns the smallest Rect holding r once m has moved it.
func (m Affine) Bounds(r Rect) Rect {
	if m.IsIdentity() {
		return r
	}
	x0, x1 := r.Origin.X, r.Origin.X+r.Size.W
	y0, y1 := r.Origin.Y, r.Origin.Y+r.Size.H
	return boundsOf4(m.Apply(Point{X: x0, Y: y0}), m.Apply(Point{X: x1, Y: y0}),
		m.Apply(Point{X: x0, Y: y1}), m.Apply(Point{X: x1, Y: y1}))
}

// boundsOf4 is PointBounds over four points, without an iterator, so a
// transformed rect's bounds cost no allocation.
func boundsOf4(a, b, c, d Point) Rect {
	x0, x1 := min(a.X, b.X, c.X, d.X), max(a.X, b.X, c.X, d.X)
	y0, y1 := min(a.Y, b.Y, c.Y, d.Y), max(a.Y, b.Y, c.Y, d.Y)
	return layout.Rct(layout.Pt(x0, y0), layout.Sz(x1-x0, y1-y0))
}

// PointBounds returns the smallest Rect holding every point of pts, or the
// zero Rect when there are none.
func PointBounds(pts iter.Seq[Point]) Rect {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for q := range pts {
		x0, y0, x1, y1 = min(x0, q.X), min(y0, q.Y), max(x1, q.X), max(y1, q.Y)
	}
	if x0 > x1 {
		return Rect{}
	}
	return layout.Rct(layout.Pt(x0, y0), layout.Sz(x1-x0, y1-y0))
}

// Matrix returns m as the matrix [a c tx; b d ty], which takes (x, y) to
// (a·x + c·y + tx, b·x + d·y + ty).
func (m Affine) Matrix() (a, b, c, d, tx, ty float64) {
	return m.a1 + 1, m.b, m.c, m.d1 + 1, m.tx, m.ty
}
