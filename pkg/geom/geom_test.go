package geom

import (
	"math"
	"testing"

	"github.com/ironpark/ggui/pkg/layout"
)

func nearPt(a, b Point) bool { return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9 }

func TestAffineComposesAndInverts(t *testing.T) {
	t.Parallel()
	if !(Affine{}).IsIdentity() || !nearPt(Affine{}.Apply(layout.Pt(3, 4)), layout.Pt(3, 4)) {
		t.Fatal("zero Affine is not the identity")
	}
	turn := Affine{}.Rotate(math.Pi / 2)
	if got := turn.Apply(layout.Pt(1, 0)); !nearPt(got, layout.Pt(0, 1)) {
		t.Fatalf("a quarter turn takes (1, 0) to %v; want (0, 1), clockwise on screen", got)
	}
	m := Affine{}.Scale(2, 3).Translate(10, 0)
	if got := m.Apply(layout.Pt(1, 1)); !nearPt(got, layout.Pt(12, 3)) {
		t.Fatalf("scale then move gives %v; want (12, 3)", got)
	}
	inv, ok := m.Rotate(0.3).Skew(0.2, 0).Invert()
	if !ok {
		t.Fatal("an invertible transform has no inverse")
	}
	p := layout.Pt(5, -7)
	if got := inv.Apply(m.Rotate(0.3).Skew(0.2, 0).Apply(p)); math.Abs(got.X-p.X) > 1e-9 || math.Abs(got.Y-p.Y) > 1e-9 {
		t.Fatalf("inverse round trip gives %v; want %v", got, p)
	}
	if _, ok := (Affine{}).Scale(0, 1).Invert(); ok {
		t.Fatal("a zero scale has an inverse")
	}
	about := Affine{}.Scale(2, 2).About(layout.Pt(10, 10))
	if got := about.Apply(layout.Pt(10, 10)); !nearPt(got, layout.Pt(10, 10)) {
		t.Fatalf("About moved its own centre to %v", got)
	}
	b := turn.Bounds(layout.Rct(layout.Pt(0, 0), layout.Sz(10, 4)))
	if !nearPt(b.Origin, layout.Pt(-4, 0)) || math.Abs(b.Size.W-4) > 1e-9 || math.Abs(b.Size.H-10) > 1e-9 {
		t.Fatalf("turned bounds %v; want (-4,0) 4x10", b)
	}
}

func TestOutlineContainsByTheNonzeroRule(t *testing.T) {
	t.Parallel()
	var o Outline
	// A square with a square hole wound the other way.
	for _, sub := range [][]Point{
		{layout.Pt(0, 0), layout.Pt(10, 0), layout.Pt(10, 10), layout.Pt(0, 10)},
		{layout.Pt(3, 3), layout.Pt(3, 7), layout.Pt(7, 7), layout.Pt(7, 3)},
	} {
		o.MoveTo(sub[0])
		for _, p := range sub[1:] {
			o.LineTo(p)
		}
		o.Close()
	}
	if !o.Contains(layout.Pt(1, 1)) || o.Contains(layout.Pt(5, 5)) || o.Contains(layout.Pt(11, 5)) {
		t.Fatal("want the ring filled and the hole empty")
	}
	if b := o.Bounds(); b != layout.Rct(layout.Pt(0, 0), layout.Sz(10, 10)) {
		t.Fatalf("bounds %v", b)
	}
	var curve Outline
	curve.MoveTo(layout.Pt(0, 0))
	curve.CubicTo(layout.Pt(0, 20), layout.Pt(20, 20), layout.Pt(20, 0))
	if !curve.Contains(layout.Pt(10, 10)) || curve.Contains(layout.Pt(10, 16)) {
		t.Fatal("a cubic curve is not followed")
	}
	if got := len(curve.Polygons()[0]); got != CurveSteps+1 {
		t.Fatalf("a curve flattened into %d points, want %d", got, CurveSteps+1)
	}
	var empty Outline
	if empty.Contains(layout.Pt(0, 0)) || empty.Bounds() != (Rect{}) {
		t.Fatal("an empty outline holds something")
	}
}

// benchOutline is a rounded rectangle of four lines and four curves.
func benchOutline() *Outline {
	var o Outline
	o.MoveTo(layout.Pt(10, 0))
	o.LineTo(layout.Pt(190, 0))
	o.QuadTo(layout.Pt(200, 0), layout.Pt(200, 10))
	o.LineTo(layout.Pt(200, 90))
	o.QuadTo(layout.Pt(200, 100), layout.Pt(190, 100))
	o.LineTo(layout.Pt(10, 100))
	o.QuadTo(layout.Pt(0, 100), layout.Pt(0, 90))
	o.LineTo(layout.Pt(0, 10))
	o.QuadTo(layout.Pt(0, 0), layout.Pt(10, 0))
	o.Close()
	return &o
}

var sinkBool bool

// BenchmarkOutlineContains hit tests a flattened outline, as a pointer
// moving over a clipped or shaped widget does each frame.
func BenchmarkOutlineContains(b *testing.B) {
	o := benchOutline()
	o.Polygons()
	i := 0
	for b.Loop() {
		sinkBool = o.Contains(layout.Pt(float64(i%220)-10, float64(i%120)-10))
		i += 7
	}
}

// BenchmarkOutlinePolygons flattens an outline again after a change.
func BenchmarkOutlinePolygons(b *testing.B) {
	o := benchOutline()
	b.ReportAllocs()
	for b.Loop() {
		o.stale = true
		o.Polygons()
	}
}

func BenchmarkAffineThenApply(b *testing.B) {
	m := Affine{}.Scale(2, 2).Rotate(0.3)
	p := layout.Pt(3, 4)
	for b.Loop() {
		p = m.Then(Affine{}.Translate(1, 0)).Apply(p)
		if p.X > 1e6 {
			p = layout.Pt(3, 4)
		}
	}
}
