package geom

import (
	"math"
	"testing"
)

func TestConstructorsConvertEveryNumericType(t *testing.T) {
	t.Parallel()
	want := Point{X: 3, Y: 4}
	for name, got := range map[string]Point{
		"int":     Pt(3, 4),
		"int8":    Pt[int8](3, 4),
		"uint16":  Pt[uint16](3, 4),
		"int64":   Pt[int64](3, 4),
		"uint":    Pt[uint](3, 4),
		"float32": Pt[float32](3, 4),
		"float64": Pt(3.0, 4.0),
	} {
		if got != want {
			t.Errorf("Pt with %s = %v, want %v", name, got, want)
		}
	}
	if got := Sz[uint8](200, 100); got != (Size{W: 200, H: 100}) {
		t.Errorf("Sz[uint8](200, 100) = %v, want {200 100}", got)
	}
	if got := Pt(-1.5, 0.25); got != (Point{-1.5, 0.25}) {
		t.Errorf("Pt kept fractions as %v, want {-1.5 0.25}", got)
	}
	if got := Rct(Pt(1, 2), Sz(3, 4)); got != (Rect{Origin: Point{1, 2}, Size: Size{3, 4}}) {
		t.Errorf("Rct = %v, want origin {1 2} size {3 4}", got)
	}
}

func TestPointAddSumsComponents(t *testing.T) {
	t.Parallel()
	if got := Pt(1, -2).Add(Pt(0.5, 5)); got != Pt(1.5, 3.0) {
		t.Fatalf("Add = %v, want {1.5 3}", got)
	}
	if got := Pt(7, 8).Add(Point{}); got != Pt(7, 8) {
		t.Fatalf("adding the zero point moved %v", got)
	}
}

func TestIntersectReturnsTheOverlapOrNothing(t *testing.T) {
	t.Parallel()
	a := Rct(Pt(0, 0), Sz(10, 10))
	for _, tc := range []struct {
		name string
		o    Rect
		want Rect
	}{
		{"overlapping corner", Rct(Pt(5, 5), Sz(10, 10)), Rct(Pt(5, 5), Sz(5, 5))},
		{"inside", Rct(Pt(2, 3), Sz(4, 5)), Rct(Pt(2, 3), Sz(4, 5))},
		{"covering", Rct(Pt(-5, -5), Sz(30, 30)), a},
		{"touching edge", Rct(Pt(10, 0), Sz(5, 10)), Rect{}},
		{"touching corner", Rct(Pt(10, 10), Sz(5, 5)), Rect{}},
		{"apart", Rct(Pt(20, 20), Sz(5, 5)), Rect{}},
		{"zero width", Rct(Pt(5, 0), Sz(0, 10)), Rect{}},
		{"negative size", Rct(Pt(5, 5), Sz(-3, 4)), Rect{}},
	} {
		if got := a.Intersect(tc.o); got != tc.want {
			t.Errorf("%s: Intersect = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestUnionCoversBoth(t *testing.T) {
	t.Parallel()
	got := Rct(Pt(0, 0), Sz(2, 2)).Union(Rct(Pt(5, -1), Sz(1, 1)))
	if want := Rct(Pt(0, -1), Sz(6, 3)); got != want {
		t.Fatalf("Union = %v, want %v", got, want)
	}
	r := Rct(Pt(1, 1), Sz(4, 4))
	if got := r.Union(Rct(Pt(2, 2), Sz(1, 1))); got != r {
		t.Fatalf("a union with a rect inside changed %v to %v", r, got)
	}
}

func TestEmptyMeansNoArea(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		s    Size
		want bool
	}{
		{Sz(1, 1), false},
		{Sz(0.001, 5), false},
		{Sz(0, 5), true},
		{Sz(5, 0), true},
		{Sz(-1, 5), true},
		{Sz(5, -1), true},
		{Size{}, true},
	} {
		if got := Rct(Pt(3, 3), tc.s).Empty(); got != tc.want {
			t.Errorf("size %v: Empty = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestContainsIncludesTopLeftEdgesOnly(t *testing.T) {
	t.Parallel()
	r := Rct(Pt(10, 20), Sz(5, 5))
	for _, tc := range []struct {
		p    Point
		want bool
	}{
		{Pt(10, 20), true},
		{Pt(12, 22), true},
		{Pt(14.999, 24.999), true},
		{Pt(15, 22), false},
		{Pt(12, 25), false},
		{Pt(15, 25), false},
		{Pt(9.999, 22), false},
		{Pt(12, 19.999), false},
	} {
		if got := r.Contains(tc.p); got != tc.want {
			t.Errorf("%v contains %v = %v, want %v", r, tc.p, got, tc.want)
		}
	}
	if Rct(Pt(0, 0), Sz(0, 10)).Contains(Pt(0, 5)) {
		t.Error("a rect with no width contains its own origin edge")
	}
	if Rct(Pt(0, 0), Sz(10, 10)).Contains(Pt(math.NaN(), 5)) {
		t.Error("a NaN point is contained")
	}
}

func TestCenterIsTheMidpoint(t *testing.T) {
	t.Parallel()
	if got := Rct(Pt(10, 20), Sz(4, 6)).Center(); got != Pt(12, 23) {
		t.Fatalf("Center = %v, want {12 23}", got)
	}
	if got := (Rect{}).Center(); got != (Point{}) {
		t.Fatalf("the zero rect's center is %v, want the origin", got)
	}
}

// fuzzRect builds a Rect from integer components, so that every sum the
// geometry takes is exact and edges compare without rounding.
func fuzzRect(x, y, w, h int16) Rect { return Rct(Pt(x, y), Sz(w, h)) }

// FuzzRectAlgebra checks the set identities Intersect, Union and Contains
// must keep for integer-valued rects: Intersect and Union commute, the
// overlap is exactly the points in both, the union holds every point of
// either, and a non-empty rect contains its center.
func FuzzRectAlgebra(f *testing.F) {
	f.Add(int16(0), int16(0), int16(10), int16(10), int16(5), int16(5), int16(10), int16(10), int16(7), int16(7), int16(-3), int16(-3), int16(4), int16(4))
	f.Add(int16(0), int16(0), int16(10), int16(10), int16(10), int16(0), int16(5), int16(5), int16(10), int16(2), int16(0), int16(0), int16(1), int16(1))
	f.Add(int16(-5), int16(-5), int16(0), int16(3), int16(-5), int16(-5), int16(3), int16(3), int16(-5), int16(-5), int16(-5), int16(-5), int16(20), int16(20))
	f.Add(int16(1), int16(1), int16(-4), int16(-4), int16(0), int16(0), int16(2), int16(2), int16(1), int16(1), int16(0), int16(0), int16(1), int16(1))
	f.Fuzz(func(t *testing.T, ax, ay, aw, ah, bx, by, bw, bh, px, py, cx, cy, cw, ch int16) {
		a, b, c := fuzzRect(ax, ay, aw, ah), fuzzRect(bx, by, bw, bh), fuzzRect(cx, cy, cw, ch)
		p := Pt(px, py)
		ab := a.Intersect(b)
		if ab != b.Intersect(a) {
			t.Fatalf("Intersect is not commutative: %v vs %v", ab, b.Intersect(a))
		}
		if ab.Empty() && ab != (Rect{}) {
			t.Fatalf("an empty overlap is %v, want the zero Rect", ab)
		}
		if inBoth := a.Contains(p) && b.Contains(p); inBoth != ab.Contains(p) {
			t.Fatalf("%v in both %v and %v = %v, but in their overlap %v = %v", p, a, b, inBoth, ab, !inBoth)
		}
		if !ab.Empty() && (a.Intersect(ab) != ab || b.Intersect(ab) != ab) {
			t.Fatalf("the overlap %v is not contained in %v and %v", ab, a, b)
		}
		if l, r := ab.Intersect(c), a.Intersect(b.Intersect(c)); l != r {
			t.Fatalf("Intersect is not associative: %v vs %v", l, r)
		}
		if !a.Empty() && a.Intersect(a) != a {
			t.Fatalf("%v intersected with itself is %v", a, a.Intersect(a))
		}

		u := a.Union(b)
		if u != b.Union(a) {
			t.Fatalf("Union is not commutative: %v vs %v", u, b.Union(a))
		}
		if (a.Contains(p) || b.Contains(p)) && !u.Contains(p) {
			t.Fatalf("%v is in %v or %v but not in their union %v", p, a, b, u)
		}
		if !a.Empty() && !b.Empty() && (u.Intersect(a) != a || u.Intersect(b) != b) {
			t.Fatalf("the union %v does not cover %v and %v", u, a, b)
		}
		if !a.Empty() && !a.Contains(a.Center()) {
			t.Fatalf("%v does not contain its center %v", a, a.Center())
		}
		if a.Empty() && a.Contains(p) {
			t.Fatalf("the empty %v contains %v", a, p)
		}
	})
}
