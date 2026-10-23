package ggui

import (
	"math"

	"github.com/ironpark/ggfx"
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
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, p := range [4]Point{
		r.Origin, {X: r.Origin.X + r.Size.W, Y: r.Origin.Y},
		{X: r.Origin.X, Y: r.Origin.Y + r.Size.H}, {X: r.Origin.X + r.Size.W, Y: r.Origin.Y + r.Size.H},
	} {
		q := m.Apply(p)
		x0, y0, x1, y1 = min(x0, q.X), min(y0, q.Y), max(x1, q.X), max(y1, q.Y)
	}
	return Rct(Pt(x0, y0), Sz(x1-x0, y1-y0))
}

// geoM returns m as a ggfx transform on Image pixels, at scale pixels to
// a logical one.
func (m Affine) geoM(scale float64) ggfx.GeoM {
	var g ggfx.GeoM
	g.SetElement(0, 0, m.a1+1)
	g.SetElement(0, 1, m.c)
	g.SetElement(0, 2, m.tx*scale)
	g.SetElement(1, 0, m.b)
	g.SetElement(1, 1, m.d1+1)
	g.SetElement(1, 2, m.ty*scale)
	return g
}
