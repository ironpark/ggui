package ggui

import "testing"

func TestConstraintsConstrain(t *testing.T) {
	c := Constraints{MinW: 10, MinH: 10, MaxW: 100, MaxH: 100}
	got := c.Constrain(Size{W: 5, H: 200})
	if got != (Size{W: 10, H: 100}) {
		t.Fatalf("Constrain() = %+v, want {10 100}", got)
	}
}

func TestBoxAddsPaddingAroundChild(t *testing.T) {
	b := Box(Box().Size(20, 10)).Pad(5)
	got := b.Layout(Loose(Sz(200, 200)), Env{})
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
	got := col.Layout(Loose(Sz(200, 200)), Env{})
	if got != (Size{W: 30, H: 34}) {
		t.Fatalf("Layout() = %+v, want {30 34}", got)
	}
}

func TestCenterFillsAvailableSpace(t *testing.T) {
	c := Center(Box().Size(10, 10))
	got := c.Layout(Loose(Sz(100, 50)), Env{})
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
	if s := got[1].Layout(Loose(Sz(100, 100)), Env{}); s != (Size{W: 5, H: 20}) {
		t.Fatalf("second child = %+v, want {5 20}", s)
	}
}

func TestListLaysOutItemsLikeColumn(t *testing.T) {
	l := List([]float64{10, 20}, func(h float64) Widget { return Box().Size(30, h) }).Gap(4)
	got := l.Layout(Loose(Sz(200, 200)), Env{})
	if got != (Size{W: 30, H: 34}) {
		t.Fatalf("Layout() = %+v, want {30 34}", got)
	}
}

// probe is a fixed-size widget that records the Rect it was painted in.
func probe(w, h float64, got *Rect) Widget {
	return FromFuncs(
		func(c Constraints, _ Env) Size { return c.Constrain(Sz(w, h)) },
		func(_ *Canvas, r Rect) { *got = r },
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
	got := row.Layout(Loose(Sz(200, 200)), Env{})
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
	p := Padding(probe(20, 10, &child), 1, 2, 3, 4)
	got := p.Layout(Loose(Sz(200, 200)), Env{})
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
	got := p.Layout(Loose(Sz(100, 100)), Env{})
	if got != (Size{W: 100, H: 100}) {
		t.Fatalf("Layout() = %+v, want the child clamped inside {100 100}", got)
	}
}

func TestStackHugsLargestChildAndLayersAtOrigin(t *testing.T) {
	var a, b Rect
	st := Stack(probe(10, 30, &a), probe(20, 5, &b))
	got := st.Layout(Loose(Sz(200, 200)), Env{})
	if got != (Size{W: 20, H: 30}) {
		t.Fatalf("Layout() = %+v, want {20 30}", got)
	}
	st.Paint(nil, Rct(Pt(7, 9), got))
	if a.Origin != (Point{X: 7, Y: 9}) || b.Origin != (Point{X: 7, Y: 9}) {
		t.Fatalf("children painted at %+v and %+v, want both at {7 9}", a.Origin, b.Origin)
	}
}

func TestStackExpandFillsSpace(t *testing.T) {
	st := Stack(Box().Size(10, 10)).Expand()
	if got := st.Layout(Loose(Sz(200, 100)), Env{}); got != (Size{W: 200, H: 100}) {
		t.Fatalf("Layout() = %+v, want {200 100}", got)
	}
}

func TestAlignPlacesChildByFraction(t *testing.T) {
	cases := []struct {
		name string
		w    *AlignWidget
		want Point
	}{
		{"center", Center(nil), Point{X: 45, Y: 20}},
		{"bottom right", Align(nil).Bottom().Right(), Point{X: 90, Y: 40}},
		{"top left", Align(nil).Top().Left(), Point{X: 0, Y: 0}},
		{"quarter", Align(nil).At(0.25, 1), Point{X: 22.5, Y: 40}},
	}
	for _, c := range cases {
		var got Rect
		c.w.child = probe(10, 10, &got)
		size := c.w.Layout(Loose(Sz(100, 50)), Env{})
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
	var _ *StateValue[int] = State(0)
	var _ *StateValue[string] = State("??")
	var _ *StateValue[float64] = State[float64](0)
	var _ *StateValue[Widget] = State[Widget](nil)
}

func TestExpandedTakesLeftoverMainAxis(t *testing.T) {
	var a, b, c Rect
	row := Row(
		probe(10, 10, &a),
		Expanded(probe(1, 10, &b)),
		Flex(probe(1, 10, &c), 3),
	).Gap(5)
	got := row.Layout(Loose(Sz(100, 50)), Env{})
	if got != (Size{W: 100, H: 10}) {
		t.Fatalf("Layout() = %+v, want to fill the width {100 10}", got)
	}
	row.Paint(nil, Rct(Pt(0, 0), got))
	// 100 - 10 - 2*5 = 80 free, split 1:3.
	if b.Size.W != 20 || c.Size.W != 60 {
		t.Fatalf("flex widths = %v, %v; want 20, 60", b.Size.W, c.Size.W)
	}
	if b.Origin.X != 15 || c.Origin.X != 40 {
		t.Fatalf("flex origins = %v, %v; want 15, 40", b.Origin.X, c.Origin.X)
	}
}

func TestSpacerPushesNeighboursApart(t *testing.T) {
	var last Rect
	row := Row(Box().Size(10, 10), Spacer(), probe(10, 10, &last))
	row.Paint(nil, Rct(Pt(0, 0), row.Layout(Loose(Sz(100, 10)), Env{})))
	if last.Origin.X != 90 {
		t.Fatalf("last child at x=%v, want 90", last.Origin.X)
	}
}

func TestJustifyDistributesSlack(t *testing.T) {
	cases := []struct {
		name string
		j    Justify
		want []float64 // y of each of three 10-high children in 100
	}{
		{"start", JustifyStart, []float64{0, 10, 20}},
		{"center", JustifyCenter, []float64{35, 45, 55}},
		{"end", JustifyEnd, []float64{70, 80, 90}},
		{"between", SpaceBetween, []float64{0, 45, 90}},
		{"around", SpaceAround, []float64{70.0 / 6, 70.0/6 + 10 + 70.0/3, 70.0/6 + 20 + 2*70.0/3}},
		{"evenly", SpaceEvenly, []float64{17.5, 45, 72.5}},
	}
	for _, c := range cases {
		var got [3]Rect
		col := Column(probe(10, 10, &got[0]), probe(10, 10, &got[1]), probe(10, 10, &got[2])).Justify(c.j)
		size := col.Layout(Loose(Sz(50, 100)), Env{})
		if c.j != JustifyStart && size.H != 100 {
			t.Fatalf("%s: Layout() = %+v, want to fill the height", c.name, size)
		}
		col.Paint(nil, Rct(Pt(0, 0), size))
		for i, want := range c.want {
			if d := got[i].Origin.Y - want; d > 1e-9 || d < -1e-9 {
				t.Fatalf("%s: child %d at y=%v, want %v", c.name, i, got[i].Origin.Y, want)
			}
		}
	}
}

func TestCrossAlignPlacesAndStretches(t *testing.T) {
	var got Rect
	row := Row(Box().Size(10, 40), probe(10, 10, &got)).Align(AlignCenter)
	row.Paint(nil, Rct(Pt(0, 0), row.Layout(Loose(Sz(100, 100)), Env{})))
	if got.Origin.Y != 15 {
		t.Fatalf("centered child at y=%v, want 15", got.Origin.Y)
	}

	row = Row(probe(10, 10, &got)).Align(AlignStretch)
	size := row.Layout(Loose(Sz(100, 60)), Env{})
	row.Paint(nil, Rct(Pt(0, 0), size))
	if size.H != 60 || got.Size.H != 60 {
		t.Fatalf("stretch: row %v, child %v; want both 60 high", size.H, got.Size.H)
	}
}

func TestFlexIsTransparentOutsideAFlow(t *testing.T) {
	if got := Expanded(Box().Size(7, 7)).Layout(Loose(Sz(100, 100)), Env{}); got != (Size{W: 7, H: 7}) {
		t.Fatalf("Layout() = %+v, want the child's {7 7}", got)
	}
}

func TestFixedBoxGivesChildTightConstraints(t *testing.T) {
	var got Constraints
	child := FromFuncs(
		func(c Constraints, _ Env) Size { got = c; return c.Constrain(Sz(1, 1)) },
		func(*Canvas, Rect) {},
	)
	Box(child).Width(100).Pad(10).Layout(Loose(Sz(500, 500)), Env{})
	if got.MinW != 80 || got.MaxW != 80 {
		t.Fatalf("child width constraints = [%v, %v], want tight 80", got.MinW, got.MaxW)
	}
	if got.MinH != 0 || got.MaxH != 480 {
		t.Fatalf("child height constraints = [%v, %v], want loose up to 480", got.MinH, got.MaxH)
	}
}

func TestContentCentersInsideFixedBox(t *testing.T) {
	var text, row Rect
	b := Box(
		Column(
			probe(40, 10, &text),
			Row(probe(20, 10, &row)).Justify(JustifyCenter),
		).Align(AlignCenter),
	).Width(200).Pad(20)
	size := b.Layout(Loose(Sz(1000, 1000)), Env{})
	b.Paint(nil, Rct(Pt(0, 0), size))
	if size.W != 200 {
		t.Fatalf("box is %v wide, want 200", size.W)
	}
	// Inner width is 160: the text centers at 20 + (160-40)/2, the row's
	// child at 20 + (160-20)/2.
	if text.Origin.X != 80 || row.Origin.X != 90 {
		t.Fatalf("text at x=%v, row child at x=%v; want 80 and 90", text.Origin.X, row.Origin.X)
	}
}

func TestWrapBreaksLinesAtWidth(t *testing.T) {
	w := Wrap(
		Box().Size(40, 10), Box().Size(40, 20), Box().Size(40, 10),
		Box().Size(40, 10),
	).Gap(5)
	got := w.Layout(Loose(Sz(100, 200)), Env{})
	// 40+5+40 = 85 fits, a third would be 130: two per line, two lines of
	// heights 20 and 10 with a 5 run gap.
	if got != (Size{W: 85, H: 35}) {
		t.Fatalf("Layout() = %+v, want {85 35}", got)
	}
	if w.offsets[2] != Pt(0, 25) || w.offsets[3] != Pt(45, 25) {
		t.Fatalf("second line at %v %v, want (0,25) (45,25)", w.offsets[2], w.offsets[3])
	}
	w.Align(AlignCenter)
	w.Layout(Loose(Sz(100, 200)), Env{})
	if w.offsets[0].Y != 5 {
		t.Fatalf("centered first child at y=%v in a 20-tall line, want 5", w.offsets[0].Y)
	}
	if got := Wrap(Box().Size(40, 10), Box().Size(40, 10)).Layout(Loose(Sz(Unbounded, 10)), Env{}); got.W != 80 {
		t.Fatalf("unbounded width gave %v, want one line of 80", got)
	}
}

func TestGridSharesWidthAndSizesRows(t *testing.T) {
	var cell Rect
	g := Grid(3,
		Box().Size(10, 10), Box().Size(10, 30), Box().Size(10, 10),
		probe(10, 10, &cell),
	).Gap(6)
	got := g.Layout(Loose(Sz(96, 200)), Env{})
	// (96 - 2*6)/3 = 28 per column; rows 30 and 10 with a 6 gap.
	if got != (Size{W: 96, H: 46}) {
		t.Fatalf("Layout() = %+v, want {96 46}", got)
	}
	g.Paint(nil, Rct(Pt(0, 0), got))
	if cell != Rct(Pt(0, 36), Sz(28, 10)) {
		t.Fatalf("fourth cell at %+v, want (0,36) 28x10 with the width given tight", cell)
	}
	if got := Grid(2, Box().Size(30, 10), Box().Size(10, 10)).Layout(Loose(Sz(Unbounded, 10)), Env{}); got.W != 60 {
		t.Fatalf("unbounded width gave %v, want 2 columns of the widest child", got)
	}
}

func TestAppLaysOutOnlyWhenSomethingChanged(t *testing.T) {
	a := New(Config{}, func() Widget { return Box() })
	a.root = a.build()
	size := Sz(100, 100)
	if !a.needsLayout(size) {
		t.Fatal("first frame must lay out")
	}
	if a.needsLayout(size) {
		t.Fatal("a still frame must not lay out")
	}
	if !a.needsLayout(Sz(200, 100)) || a.needsLayout(Sz(200, 100)) {
		t.Fatal("a resize must lay out once")
	}
	State(0).Set(1)
	if !a.needsLayout(Sz(200, 100)) {
		t.Fatal("a signal write must lay out")
	}
}
