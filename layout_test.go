package ggui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestConstraintsConstrain(t *testing.T) {
	c := Constraints{MinW: 10, MinH: 10, MaxW: 100, MaxH: 100}
	got := c.Constrain(Size{W: 5, H: 200})
	if got != (Size{W: 10, H: 100}) {
		t.Fatalf("Constrain() = %+v, want {10 100}", got)
	}
}

func TestBoxAddsPaddingAroundChild(t *testing.T) {
	b := Box(Box().Size(20, 10)).Pad(5)
	got := b.Layout(Loose(Sz(200, 200)))
	if got != (Size{W: 30, H: 20}) {
		t.Fatalf("Layout() = %+v, want {30 20}", got)
	}
}

func TestBoxRejectsSeveralChildren(t *testing.T) {
	mustPanic(t, "Box(a, b)", func() { Box(Text("a"), Text("b")) })
}

func TestColumnSumsHeightsAndGaps(t *testing.T) {
	col := Column(
		Box().Size(10, 10),
		Box().Size(30, 20),
	).Gap(4)
	got := col.Layout(Loose(Sz(200, 200)))
	if got != (Size{W: 30, H: 34}) {
		t.Fatalf("Layout() = %+v, want {30 34}", got)
	}
}

func TestCenterFillsAvailableSpace(t *testing.T) {
	c := Center(Box().Size(10, 10))
	got := c.Layout(Loose(Sz(100, 50)))
	if got != (Size{W: 100, H: 50}) {
		t.Fatalf("Layout() = %+v, want {100 50}", got)
	}
}

func TestSzAndPtAcceptAnyNumericType(t *testing.T) {
	if got := Sz(3, 4); got != (Size{W: 3, H: 4}) {
		t.Fatalf("Sz() = %+v, want {3 4}", got)
	}
	if got := Pt(float32(1.5), float32(2.5)); got != (Point{X: 1.5, Y: 2.5}) {
		t.Fatalf("Pt() = %+v, want {1.5 2.5}", got)
	}
}

func TestChildrenBuildsOnePerItem(t *testing.T) {
	got := Children([]float64{10, 20}, func(h float64) Widget {
		return Box().Size(5, h)
	})
	if len(got) != 2 {
		t.Fatalf("len(Children()) = %d, want 2", len(got))
	}
	if s := got[1].Layout(Loose(Sz(100, 100))); s != (Size{W: 5, H: 20}) {
		t.Fatalf("second child = %+v, want {5 20}", s)
	}
}

func TestListLaysOutItemsLikeColumn(t *testing.T) {
	l := List([]float64{10, 20}, func(h float64) Widget { return Box().Size(30, h) }).Gap(4)
	got := l.Layout(Loose(Sz(200, 200)))
	if got != (Size{W: 30, H: 34}) {
		t.Fatalf("Layout() = %+v, want {30 34}", got)
	}
}

// probe is a fixed-size widget that records the Rect it was painted in.
func probe(w, h float64, got *Rect) Widget {
	return FromFuncs(
		func(c Constraints) Size { return c.Constrain(Sz(w, h)) },
		func(_ *ebiten.Image, r Rect) { *got = r },
	)
}

// mustPanic fails the test unless fn panics.
func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s did not panic", what)
		}
	}()
	fn()
}

func TestInsetsShorthand(t *testing.T) {
	cases := []struct {
		in   []float64
		want EdgeInsets
	}{
		{nil, EdgeInsets{}},
		{[]float64{8}, EdgeInsets{8, 8, 8, 8}},
		{[]float64{4, 12}, EdgeInsets{Top: 4, Right: 12, Bottom: 4, Left: 12}},
		{[]float64{1, 2, 3, 4}, EdgeInsets{Top: 1, Right: 2, Bottom: 3, Left: 4}},
	}
	for _, c := range cases {
		if got := Insets(c.in...); got != c.want {
			t.Fatalf("Insets(%v) = %+v, want %+v", c.in, got, c.want)
		}
	}
	mustPanic(t, "Insets(1, 2, 3)", func() { Insets(1, 2, 3) })
}

func TestRowSumsWidthsAndGaps(t *testing.T) {
	var second Rect
	row := Row(
		Box().Size(10, 10),
		probe(30, 20, &second),
	).Gap(4)
	got := row.Layout(Loose(Sz(200, 200)))
	if got != (Size{W: 44, H: 20}) {
		t.Fatalf("Layout() = %+v, want {44 20}", got)
	}
	row.Paint(nil, Rct(Pt(0, 0), got))
	if want := Rct(Pt(14, 0), Sz(30, 20)); second != want {
		t.Fatalf("second child painted at %+v, want %+v", second, want)
	}
}

func TestPaddingInsetsChild(t *testing.T) {
	var child Rect
	p := Padding(probe(20, 10, &child)).Padding(EdgeInsets{Top: 1, Right: 2, Bottom: 3, Left: 4})
	got := p.Layout(Loose(Sz(200, 200)))
	if got != (Size{W: 26, H: 14}) {
		t.Fatalf("Layout() = %+v, want {26 14}", got)
	}
	p.Paint(nil, Rct(Pt(100, 50), got))
	if want := Rct(Pt(104, 51), Sz(20, 10)); child != want {
		t.Fatalf("child painted at %+v, want %+v", child, want)
	}
}

func TestPaddingShrinksChildConstraints(t *testing.T) {
	p := Padding(Box().Size(500, 500), 10)
	got := p.Layout(Loose(Sz(100, 100)))
	if got != (Size{W: 100, H: 100}) {
		t.Fatalf("Layout() = %+v, want the child clamped inside {100 100}", got)
	}
}

func TestStackHugsLargestChildAndLayersAtOrigin(t *testing.T) {
	var a, b Rect
	st := Stack(probe(10, 30, &a), probe(20, 5, &b))
	got := st.Layout(Loose(Sz(200, 200)))
	if got != (Size{W: 20, H: 30}) {
		t.Fatalf("Layout() = %+v, want {20 30}", got)
	}
	st.Paint(nil, Rct(Pt(7, 9), got))
	if a.Origin != (Point{7, 9}) || b.Origin != (Point{7, 9}) {
		t.Fatalf("children painted at %+v and %+v, want both at {7 9}", a.Origin, b.Origin)
	}
}

func TestStackExpandFillsSpace(t *testing.T) {
	st := Stack(Box().Size(10, 10)).Expand()
	if got := st.Layout(Loose(Sz(200, 100))); got != (Size{W: 200, H: 100}) {
		t.Fatalf("Layout() = %+v, want {200 100}", got)
	}
}

func TestAlignPlacesChildByFraction(t *testing.T) {
	cases := []struct {
		name string
		w    *AlignWidget
		want Point
	}{
		{"center", Center(nil), Point{45, 20}},
		{"bottom right", Align(nil).Bottom().Right(), Point{90, 40}},
		{"top left", Align(nil).Top().Left(), Point{0, 0}},
		{"quarter", Align(nil).At(0.25, 1), Point{22.5, 40}},
	}
	for _, c := range cases {
		var got Rect
		c.w.child = probe(10, 10, &got)
		size := c.w.Layout(Loose(Sz(100, 50)))
		if size != (Size{W: 100, H: 50}) {
			t.Fatalf("%s: Layout() = %+v, want to fill {100 50}", c.name, size)
		}
		c.w.Paint(nil, Rct(Pt(0, 0), size))
		if got.Origin != c.want {
			t.Fatalf("%s: child painted at %+v, want %+v", c.name, got.Origin, c.want)
		}
	}
}

func TestStateInfersTypeFromLiteral(t *testing.T) {
	var _ *Signal[int] = State(0)
	var _ *Signal[string] = State("??")
	var _ *Signal[float64] = State[float64](0)
	var _ *Signal[Widget] = State[Widget](nil)
}
