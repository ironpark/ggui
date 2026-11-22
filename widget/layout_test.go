package widget

import (
	"image/color"
	"slices"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/inspect"
	"golang.org/x/image/font/gofont/goregular"
)

func TestConstraintsConstrain(t *testing.T) {
	t.Parallel()
	c := ggui.Constraints{MinW: 10, MinH: 10, MaxW: 100, MaxH: 100}
	got := c.Constrain(ggui.Size{W: 5, H: 200})
	if got != (ggui.Size{W: 10, H: 100}) {
		t.Fatalf("Constrain() = %+v, want {10 100}", got)
	}
}

func TestBoxAddsPaddingAroundChild(t *testing.T) {
	t.Parallel()
	b := Box(Box().Size(20, 10)).Pad(5)
	got := b.Layout(ggui.Loose(ggui.Sz(200, 200)), ggui.Env{})
	if got != (ggui.Size{W: 30, H: 20}) {
		t.Fatalf("Layout() = %+v, want {30 20}", got)
	}
}

func TestBoxMaxWidthCapsTheBoxAndItsChild(t *testing.T) {
	t.Parallel()
	wide := func() ggui.Widget { return Row(Spacer()) } // as wide as allowed
	for _, tc := range []struct {
		name string
		box  *BoxWidget
		c    ggui.Constraints
		want ggui.Size
	}{
		{"loose", Box(wide()).MaxWidth(100).Pad(10), ggui.Loose(ggui.Sz(500, 50)), ggui.Sz(100, 20)},
		{"narrower parent", Box(wide()).MaxWidth(100), ggui.Loose(ggui.Sz(60, 50)), ggui.Sz(60, 0)},
		{"fixed width above the cap", Box().Width(300).MaxWidth(100), ggui.Loose(ggui.Sz(500, 50)), ggui.Sz(100, 0)},
		{"fixed width below the cap", Box().Width(80).MaxWidth(100), ggui.Loose(ggui.Sz(500, 50)), ggui.Sz(80, 0)},
		{"tight parent wins", Box(wide()).MaxWidth(100), ggui.Tight(ggui.Sz(500, 50)), ggui.Sz(500, 50)},
		{"max height", Box().Height(300).MaxHeight(40), ggui.Loose(ggui.Sz(500, 500)), ggui.Sz(0, 40)},
	} {
		if got := tc.box.Layout(tc.c, ggui.Env{}); got != tc.want {
			t.Errorf("%s: Layout() = %+v, want %+v", tc.name, got, tc.want)
		}
	}
	inner := Box(wide()).MaxWidth(100).Pad(10)
	Center(inner).Layout(ggui.Tight(ggui.Sz(500, 50)), ggui.Env{})
	if inner.childRect.Size.W != 80 {
		t.Errorf("centered: child is %v wide, want 80 inside the padding", inner.childRect.Size.W)
	}
}

func TestBoxRejectsSeveralChildren(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("Box(a, b) did not panic")
		}
	}()
	Box(Text("a"), Text("b"))
}

func TestColumnSumsHeightsAndGaps(t *testing.T) {
	t.Parallel()
	col := Column(
		Box().Size(10, 10),
		Box().Size(30, 20),
	).Gap(4)
	got := col.Layout(ggui.Loose(ggui.Sz(200, 200)), ggui.Env{})
	if got != (ggui.Size{W: 30, H: 34}) {
		t.Fatalf("Layout() = %+v, want {30 34}", got)
	}
}

func TestCenterFillsAvailableSpace(t *testing.T) {
	t.Parallel()
	c := Center(Box().Size(10, 10))
	got := c.Layout(ggui.Loose(ggui.Sz(100, 50)), ggui.Env{})
	if got != (ggui.Size{W: 100, H: 50}) {
		t.Fatalf("Layout() = %+v, want {100 50}", got)
	}
}

func TestSzAndPtAcceptAnyNumericType(t *testing.T) {
	t.Parallel()
	if got := ggui.Sz(3, 4); got != (ggui.Size{W: 3, H: 4}) {
		t.Fatalf("Sz() = %+v, want {3 4}", got)
	}
	if got := ggui.Pt(float32(1.5), float32(2.5)); got != (ggui.Point{X: 1.5, Y: 2.5}) {
		t.Fatalf("Pt() = %+v, want {1.5 2.5}", got)
	}
}

func TestChildrenBuildsOnePerItem(t *testing.T) {
	t.Parallel()
	got := ggui.Children([]float64{10, 20}, func(h float64) ggui.Widget {
		return Box().Size(5, h)
	})
	if len(got) != 2 {
		t.Fatalf("len(Children()) = %d, want 2", len(got))
	}
	if s := got[1].Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{}); s != (ggui.Size{W: 5, H: 20}) {
		t.Fatalf("second child = %+v, want {5 20}", s)
	}
}

func TestListLaysOutItemsLikeColumn(t *testing.T) {
	t.Parallel()
	l := List([]float64{10, 20}, func(h float64) ggui.Widget { return Box().Size(30, h) }).Gap(4)
	got := l.Layout(ggui.Loose(ggui.Sz(200, 200)), ggui.Env{})
	if got != (ggui.Size{W: 30, H: 34}) {
		t.Fatalf("Layout() = %+v, want {30 34}", got)
	}
}

// probe is a fixed-size widget that records the Rect it was painted in.
func probe(w, h float64, got *ggui.Rect) ggui.Widget {
	return ggui.FromFuncs(
		func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(w, h)) },
		func(_ *ggui.Canvas, r ggui.Rect) { *got = r },
	)
}

func TestRowSumsWidthsAndGaps(t *testing.T) {
	t.Parallel()
	var second ggui.Rect
	row := Row(
		Box().Size(10, 10),
		probe(30, 20, &second),
	).Gap(4)
	got := row.Layout(ggui.Loose(ggui.Sz(200, 200)), ggui.Env{})
	if got != (ggui.Size{W: 44, H: 20}) {
		t.Fatalf("Layout() = %+v, want {44 20}", got)
	}
	row.Paint(nil, ggui.Rct(ggui.Pt(0, 0), got))
	if want := ggui.Rct(ggui.Pt(14, 0), ggui.Sz(30, 20)); second != want {
		t.Fatalf("second child painted at %+v, want %+v", second, want)
	}
}

func TestPaddingInsetsChild(t *testing.T) {
	t.Parallel()
	var child ggui.Rect
	p := Padding(probe(20, 10, &child), 1, 2, 3, 4)
	got := p.Layout(ggui.Loose(ggui.Sz(200, 200)), ggui.Env{})
	if got != (ggui.Size{W: 26, H: 14}) {
		t.Fatalf("Layout() = %+v, want {26 14}", got)
	}
	p.Paint(nil, ggui.Rct(ggui.Pt(100, 50), got))
	if want := ggui.Rct(ggui.Pt(104, 51), ggui.Sz(20, 10)); child != want {
		t.Fatalf("child painted at %+v, want %+v", child, want)
	}
}

func TestPaddingShrinksChildConstraints(t *testing.T) {
	t.Parallel()
	p := Padding(Box().Size(500, 500), 10)
	got := p.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})
	if got != (ggui.Size{W: 100, H: 100}) {
		t.Fatalf("Layout() = %+v, want the child clamped inside {100 100}", got)
	}
}

func TestStackHugsLargestChildAndLayersAtOrigin(t *testing.T) {
	t.Parallel()
	var a, b ggui.Rect
	st := Stack(probe(10, 30, &a), probe(20, 5, &b))
	got := st.Layout(ggui.Loose(ggui.Sz(200, 200)), ggui.Env{})
	if got != (ggui.Size{W: 20, H: 30}) {
		t.Fatalf("Layout() = %+v, want {20 30}", got)
	}
	st.Paint(nil, ggui.Rct(ggui.Pt(7, 9), got))
	if a.Origin != (ggui.Point{X: 7, Y: 9}) || b.Origin != (ggui.Point{X: 7, Y: 9}) {
		t.Fatalf("children painted at %+v and %+v, want both at {7 9}", a.Origin, b.Origin)
	}
}

func TestStackExpandFillsSpace(t *testing.T) {
	t.Parallel()
	st := Stack(Box().Size(10, 10)).Expand()
	if got := st.Layout(ggui.Loose(ggui.Sz(200, 100)), ggui.Env{}); got != (ggui.Size{W: 200, H: 100}) {
		t.Fatalf("Layout() = %+v, want {200 100}", got)
	}
}

func TestAlignPlacesChildByFraction(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		w    *AlignWidget
		want ggui.Point
	}{
		{"center", Center(nil), ggui.Point{X: 45, Y: 20}},
		{"bottom right", Align(nil).Bottom().Right(), ggui.Point{X: 90, Y: 40}},
		{"top left", Align(nil).Top().Left(), ggui.Point{X: 0, Y: 0}},
		{"quarter", Align(nil).At(0.25, 1), ggui.Point{X: 22.5, Y: 40}},
	}
	for _, c := range cases {
		var got ggui.Rect
		c.w.child = probe(10, 10, &got)
		size := c.w.Layout(ggui.Loose(ggui.Sz(100, 50)), ggui.Env{})
		if size != (ggui.Size{W: 100, H: 50}) {
			t.Fatalf("%s: Layout() = %+v, want to fill {100 50}", c.name, size)
		}
		c.w.Paint(nil, ggui.Rct(ggui.Pt(0, 0), size))
		if got.Origin != c.want {
			t.Fatalf("%s: child painted at %+v, want %+v", c.name, got.Origin, c.want)
		}
	}
}

func TestStateInfersTypeFromLiteral(t *testing.T) {
	t.Parallel()
	var _ *ggui.StateValue[int] = ggui.State(0)
	var _ *ggui.StateValue[string] = ggui.State("??")
	var _ *ggui.StateValue[float64] = ggui.State[float64](0)
	var _ *ggui.StateValue[ggui.Widget] = ggui.State[ggui.Widget](nil)
}

func TestExpandedTakesLeftoverMainAxis(t *testing.T) {
	t.Parallel()
	var a, b, c ggui.Rect
	row := Row(
		probe(10, 10, &a),
		Expanded(probe(1, 10, &b)),
		Flex(probe(1, 10, &c), 3),
	).Gap(5)
	got := row.Layout(ggui.Loose(ggui.Sz(100, 50)), ggui.Env{})
	if got != (ggui.Size{W: 100, H: 10}) {
		t.Fatalf("Layout() = %+v, want to fill the width {100 10}", got)
	}
	row.Paint(nil, ggui.Rct(ggui.Pt(0, 0), got))
	// 100 - 10 - 2*5 = 80 free, split 1:3.
	if b.Size.W != 20 || c.Size.W != 60 {
		t.Fatalf("flex widths = %v, %v; want 20, 60", b.Size.W, c.Size.W)
	}
	if b.Origin.X != 15 || c.Origin.X != 40 {
		t.Fatalf("flex origins = %v, %v; want 15, 40", b.Origin.X, c.Origin.X)
	}
}

// A shrinking Scroll in a Flexible is as wide as its content while that
// fits, so what follows it follows its content, and at most its share.
func TestShrinkingScrollInAFlexible(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ content, after float64 }{
		{100, 100},
		{900, 390},
	} {
		var a, b ggui.Rect
		row := Row(Flexible(Scroll(probe(tc.content, 20, &a)).Horizontal().Shrink()), probe(10, 10, &b))
		row.Paint(nil, ggui.Rct(ggui.Pt(0, 0), row.Layout(ggui.Tight(ggui.Sz(400, 20)), ggui.Env{})))
		if b.Origin.X != tc.after {
			t.Errorf("content %v: the next child at %v, want %v", tc.content, b.Origin.X, tc.after)
		}
	}
}

// A Flexible child takes what it needs, up to its share: what follows it
// follows it, until it fills its share and the rest is pushed to the end.
func TestFlexibleTakesUpToItsShare(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ need, want, after float64 }{
		{30, 30, 30},  // fits: the next child follows at once
		{500, 90, 90}, // too wide: it takes the row but the next child's room
	} {
		var a, b ggui.Rect
		row := Row(Flexible(probe(tc.need, 10, &a)), probe(10, 10, &b))
		row.Paint(nil, ggui.Rct(ggui.Pt(0, 0), row.Layout(ggui.Tight(ggui.Sz(100, 10)), ggui.Env{})))
		if a.Size.W != tc.want || b.Origin.X != tc.after {
			t.Errorf("need %v: flexible %v, next at %v; want %v, %v", tc.need, a.Size.W, b.Origin.X, tc.want, tc.after)
		}
	}
}

func TestSpacerPushesNeighboursApart(t *testing.T) {
	t.Parallel()
	var last ggui.Rect
	row := Row(Box().Size(10, 10), Spacer(), probe(10, 10, &last))
	row.Paint(nil, ggui.Rct(ggui.Pt(0, 0), row.Layout(ggui.Loose(ggui.Sz(100, 10)), ggui.Env{})))
	if last.Origin.X != 90 {
		t.Fatalf("last child at x=%v, want 90", last.Origin.X)
	}
}

func TestJustifyDistributesSlack(t *testing.T) {
	t.Parallel()
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
		var got [3]ggui.Rect
		col := Column(probe(10, 10, &got[0]), probe(10, 10, &got[1]), probe(10, 10, &got[2])).Justify(c.j)
		size := col.Layout(ggui.Loose(ggui.Sz(50, 100)), ggui.Env{})
		if c.j != JustifyStart && size.H != 100 {
			t.Fatalf("%s: Layout() = %+v, want to fill the height", c.name, size)
		}
		col.Paint(nil, ggui.Rct(ggui.Pt(0, 0), size))
		for i, want := range c.want {
			if d := got[i].Origin.Y - want; d > 1e-9 || d < -1e-9 {
				t.Fatalf("%s: child %d at y=%v, want %v", c.name, i, got[i].Origin.Y, want)
			}
		}
	}
}

func TestCrossAlignPlacesAndStretches(t *testing.T) {
	t.Parallel()
	var got ggui.Rect
	row := Row(Box().Size(10, 40), probe(10, 10, &got)).Align(AlignCenter)
	row.Paint(nil, ggui.Rct(ggui.Pt(0, 0), row.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})))
	if got.Origin.Y != 15 {
		t.Fatalf("centered child at y=%v, want 15", got.Origin.Y)
	}

	row = Row(probe(10, 10, &got)).Align(AlignStretch)
	size := row.Layout(ggui.Loose(ggui.Sz(100, 60)), ggui.Env{})
	row.Paint(nil, ggui.Rct(ggui.Pt(0, 0), size))
	if size.H != 60 || got.Size.H != 60 {
		t.Fatalf("stretch: row %v, child %v; want both 60 high", size.H, got.Size.H)
	}
}

// A stretched Row in a vertical Scroll has no height to fill, so its
// children come out as tall as the tallest, like CSS cards in a row.
func TestStretchedRowInScrollMakesChildrenEqualHeight(t *testing.T) {
	t.Parallel()
	var short, tall ggui.Rect
	left := recordRect(Box(Box().Size(40, 20)).Pad(8), &short)
	right := recordRect(Box(Box().Size(40, 70)).Pad(8), &tall)
	row := Row(Expanded(left), Expanded(right)).Gap(10).Stretch()
	s := Scroll(row)
	s.Paint(nil, ggui.Rct(ggui.Pt(0, 0), s.Layout(ggui.Tight(ggui.Sz(200, 300)), ggui.Env{})))
	if short.Size.H != 86 || tall.Size.H != 86 {
		t.Fatalf("cards %v and %v high, want both 86", short.Size.H, tall.Size.H)
	}
}

// recordRect lays child out unchanged and records the Rect it is painted in.
func recordRect(child ggui.Widget, got *ggui.Rect) ggui.Widget {
	return ggui.FromFuncs(child.Layout, func(dst *ggui.Canvas, r ggui.Rect) { *got = r; child.Paint(dst, r) })
}

// A stretched Column with no width to fill makes its children as wide as
// the widest.
func TestStretchedColumnWithUnboundedWidthMatchesWidest(t *testing.T) {
	t.Parallel()
	var narrow ggui.Rect
	col := Column(probe(10, 10, &narrow), Box().Size(50, 10)).Stretch()
	size := col.Layout(ggui.Constraints{MaxW: ggui.Unbounded, MaxH: 100}, ggui.Env{})
	col.Paint(nil, ggui.Rct(ggui.Pt(0, 0), size))
	if size.W != 50 || narrow.Size.W != 50 {
		t.Fatalf("column %v wide, child %v; want both 50", size.W, narrow.Size.W)
	}
}

func TestFlexIsTransparentOutsideAFlow(t *testing.T) {
	t.Parallel()
	if got := Expanded(Box().Size(7, 7)).Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{}); got != (ggui.Size{W: 7, H: 7}) {
		t.Fatalf("Layout() = %+v, want the child's {7 7}", got)
	}
}

func TestFixedBoxGivesChildTightConstraints(t *testing.T) {
	t.Parallel()
	var got ggui.Constraints
	child := ggui.FromFuncs(
		func(c ggui.Constraints, _ ggui.Env) ggui.Size { got = c; return c.Constrain(ggui.Sz(1, 1)) },
		func(*ggui.Canvas, ggui.Rect) {},
	)
	Box(child).Width(100).Pad(10).Layout(ggui.Loose(ggui.Sz(500, 500)), ggui.Env{})
	if got.MinW != 80 || got.MaxW != 80 {
		t.Fatalf("child width constraints = [%v, %v], want tight 80", got.MinW, got.MaxW)
	}
	if got.MinH != 0 || got.MaxH != 480 {
		t.Fatalf("child height constraints = [%v, %v], want loose up to 480", got.MinH, got.MaxH)
	}
}

func TestContentCentersInsideFixedBox(t *testing.T) {
	t.Parallel()
	var text, row ggui.Rect
	b := Box(
		Column(
			probe(40, 10, &text),
			Row(probe(20, 10, &row)).Justify(JustifyCenter),
		).Align(AlignCenter),
	).Width(200).Pad(20)
	size := b.Layout(ggui.Loose(ggui.Sz(1000, 1000)), ggui.Env{})
	b.Paint(nil, ggui.Rct(ggui.Pt(0, 0), size))
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
	t.Parallel()
	w := Wrap(
		Box().Size(40, 10), Box().Size(40, 20), Box().Size(40, 10),
		Box().Size(40, 10),
	).Gap(5)
	got := w.Layout(ggui.Loose(ggui.Sz(100, 200)), ggui.Env{})
	// 40+5+40 = 85 fits, a third would be 130: two per line, two lines of
	// heights 20 and 10 with a 5 run gap.
	if got != (ggui.Size{W: 85, H: 35}) {
		t.Fatalf("Layout() = %+v, want {85 35}", got)
	}
	if w.rects[2].Origin != ggui.Pt(0, 25) || w.rects[3].Origin != ggui.Pt(45, 25) {
		t.Fatalf("second line at %v %v, want (0,25) (45,25)", w.rects[2].Origin, w.rects[3].Origin)
	}
	w.Align(AlignCenter)
	w.Layout(ggui.Loose(ggui.Sz(100, 200)), ggui.Env{})
	if w.rects[0].Origin.Y != 5 {
		t.Fatalf("centered first child at y=%v in a 20-tall line, want 5", w.rects[0].Origin.Y)
	}
	if got := Wrap(Box().Size(40, 10), Box().Size(40, 10)).Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, 10)), ggui.Env{}); got.W != 80 {
		t.Fatalf("unbounded width gave %v, want one line of 80", got)
	}
}

func TestGridSharesWidthAndSizesRows(t *testing.T) {
	t.Parallel()
	var cell ggui.Rect
	g := Grid(3,
		Box().Size(10, 10), Box().Size(10, 30), Box().Size(10, 10),
		probe(10, 10, &cell),
	).Gap(6)
	got := g.Layout(ggui.Loose(ggui.Sz(96, 200)), ggui.Env{})
	// (96 - 2*6)/3 = 28 per column; rows 30 and 10 with a 6 gap.
	if got != (ggui.Size{W: 96, H: 46}) {
		t.Fatalf("Layout() = %+v, want {96 46}", got)
	}
	g.Paint(nil, ggui.Rct(ggui.Pt(0, 0), got))
	if cell != ggui.Rct(ggui.Pt(0, 36), ggui.Sz(28, 10)) {
		t.Fatalf("fourth cell at %+v, want (0,36) 28x10 with the width given tight", cell)
	}
	if got := Grid(2, Box().Size(30, 10), Box().Size(10, 10)).Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, 10)), ggui.Env{}); got.W != 60 {
		t.Fatalf("unbounded width gave %v, want 2 columns of the widest child", got)
	}
}

// Borrowed rectangles must survive sibling layouts and cached children must
// still paint correctly when parent scratch buffers are reused on later frames.
func TestNestedLayoutRectsSurviveCacheHitsAndResize(t *testing.T) {
	t.Parallel()
	var painted [6]ggui.Rect
	layouts := 0
	a := ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size { layouts++; return c.Constrain(ggui.Sz(20, 10)) }, func(_ *ggui.Canvas, r ggui.Rect) { painted[0] = r })
	tree := Stack(
		Column(
			ggui.Cached(Row(a, probe(20, 20, &painted[1])).Gap(5).Align(AlignStart)),
			ggui.Cached(Grid(2, probe(7, 8, &painted[2]), probe(9, 12, &painted[3])).Gap(4)),
		).Gap(7),
		Wrap(probe(30, 5, &painted[4]), probe(30, 10, &painted[5])).Gap(3),
	)
	origin := ggui.Pt(7, 11)
	for _, width := range []float64{100, 100, 40, 40, 100} {
		size := tree.Layout(ggui.Loose(ggui.Sz(width, 100)), ggui.Env{})
		tree.Paint(nil, ggui.Rct(origin, size))
		cell := (width - 4) / 2
		want := [6]ggui.Rect{
			ggui.Rct(ggui.Pt(0, 0), ggui.Sz(20, 10)), ggui.Rct(ggui.Pt(25, 0), ggui.Sz(20, 20)),
			ggui.Rct(ggui.Pt(0, 27), ggui.Sz(cell, 8)), ggui.Rct(ggui.Pt(cell+4, 27), ggui.Sz(cell, 12)),
			ggui.Rct(ggui.Pt(0, 0), ggui.Sz(30, 5)), ggui.Rct(ggui.Pt(33, 0), ggui.Sz(30, 10)),
		}
		if width == 40 {
			want[1].Size.W = 15
			want[5].Origin = ggui.Pt(0, 8)
		}
		for i, rect := range want {
			rect.Origin = rect.Origin.Add(origin)
			if painted[i] != rect {
				t.Fatalf("width %v, child %d painted at %+v, want %+v", width, i, painted[i], rect)
			}
		}
	}
	if layouts != 3 {
		t.Fatalf("first child measured %d times, want 3 across cache hits and resizing", layouts)
	}
}

func TestSpaceUsesTheEnvironmentUnit(t *testing.T) {
	t.Parallel()
	env := ggui.Env{}.With(ggui.SpacingKey, 12.0)
	free := ggui.Loose(ggui.Sz(500, 500))
	box := func() ggui.Widget { return Box().Size(10, 10) }
	cases := []struct {
		name string
		w    ggui.Widget
		c    ggui.Constraints
		want ggui.Size
	}{
		{"row", Row(box(), box()).Space(2), free, ggui.Sz(10+24+10, 10)},
		{"wrap", Wrap(box(), box()).Space(2), free, ggui.Sz(10+24+10, 10)},
		{"wrap across lines", Wrap(box(), box()).Space(2), ggui.Loose(ggui.Sz(30, 500)), ggui.Sz(10, 10+24+10)},
		{"grid", Grid(1, box(), box()).Space(1), free, ggui.Sz(500, 10+12+10)},
	}
	for _, tc := range cases {
		if got := tc.w.Layout(tc.c, env); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRunGapAndRowGapSeparateLinesOnly(t *testing.T) {
	t.Parallel()
	var last ggui.Rect
	w := Wrap(Box().Size(40, 10), Box().Size(40, 10), probe(40, 10, &last)).Gap(5).RunGap(20)
	size := w.Layout(ggui.Loose(ggui.Sz(90, 200)), ggui.Env{})
	if size != ggui.Sz(85, 40) {
		t.Fatalf("wrap = %v, want two per line 5 apart and lines 20 apart: {85 40}", size)
	}
	w.Paint(nil, ggui.Rct(ggui.Pt(100, 100), size))
	if last != ggui.Rct(ggui.Pt(100, 130), ggui.Sz(40, 10)) {
		t.Fatalf("third child painted at %+v, want the second line's start at (100,130)", last)
	}
	g := Grid(2, Box().Size(10, 10), Box().Size(10, 10), Box().Size(10, 10)).Gap(4).RowGap(16)
	if got := g.Layout(ggui.Loose(ggui.Sz(24, 200)), ggui.Env{}); got != ggui.Sz(24, 36) {
		t.Fatalf("grid = %v, want columns 4 apart and rows 16 apart: {24 36}", got)
	}
}

func TestLineHeightSpacesTextLines(t *testing.T) {
	t.Parallel()
	free := ggui.Loose(ggui.Sz(500, 500))
	one := Text("a\nb").LineHeight(1).Layout(free, ggui.Env{})
	three := Text("a\nb").LineHeight(3).Layout(free, ggui.Env{})
	// A line with a line height set is a box of it, so each of the two
	// lines grows by twice the size.
	if !closeTo(three.H-one.H, 4*ggui.DefaultTextSize) {
		t.Fatalf("two lines at line height 3 are %v taller than at 1, want %v", three.H-one.H, 4*ggui.DefaultTextSize)
	}
	if !closeTo(one.H, 2*ggui.DefaultTextSize) {
		t.Fatalf("two lines at line height 1 are %v tall, want twice the size, %v", one.H, 2*ggui.DefaultTextSize)
	}
	inherited := Styled(Text("a\nb")).LineHeight(3).Layout(free, ggui.Env{})
	if inherited != three {
		t.Fatalf("an inherited line height of 3 gives %v, want %v", inherited, three)
	}
}

func TestStyledSetsFontAndMergesStyle(t *testing.T) {
	t.Parallel()
	font := ggui.MustFont(goregular.TTF)
	w := Text("x")
	Styled(w).Font(font).Style(ggui.TextStyle{Size: 20}).Style(ggui.TextStyle{LineHeight: 2}).Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})
	if w.resolved.Font != font || w.resolved.Size != 20 || w.resolved.LineHeight != 2 {
		t.Fatalf("resolved %+v, want the Styled font, size 20 and line height 2 merged", w.resolved)
	}
}

func TestWrappersPassTheBaselineThrough(t *testing.T) {
	t.Parallel()
	free := ggui.Loose(ggui.Sz(200, 100))
	text := func() *TextWidget { return Text("x") }
	ascent := func(w ggui.Widget) float64 {
		w.Layout(free, ggui.Env{})
		b, ok := w.(ggui.Baseliner).Baseline()
		if !ok {
			t.Fatalf("%T has no baseline", w)
		}
		return b
	}
	want := ascent(text())
	for name, w := range map[string]ggui.Widget{
		"Styled":  Styled(text()),
		"Provide": ggui.Provide(ggui.SpacingKey, 3.0, text()),
		"WithEnv": ggui.WithEnv(func(e ggui.Env) ggui.Env { return e }, text()),
		"Flex":    Row(Flex(text(), 1)).Align(AlignStart),
	} {
		if got := ascent(w); !closeTo(got, want) {
			t.Errorf("%s: baseline %v, want the text's %v", name, got, want)
		}
	}
	// An Align moves the baseline with the child.
	al := Align(text()).Bottom()
	al.Layout(ggui.Tight(ggui.Sz(200, 100)), ggui.Env{})
	h := text().Layout(free, ggui.Env{}).H
	if got, _ := al.Baseline(); !closeTo(got, 100-h+want) {
		t.Fatalf("bottom-aligned baseline %v, want %v", got, 100-h+want)
	}
}

func TestScrollKeyKeepsItsOffsetWhenMoved(t *testing.T) {
	t.Parallel()
	moved := ggui.State(false)
	var s *ScrollWidget
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			s = Scroll(Box().Size(100, 1000)).Key("list")
			if moved.Get() {
				return Column(Box().Size(10, 20), Box(s).Size(100, 100))
			}
			return Column(Box(s).Size(100, 100))
		})
	}, ggui.Sz(100, 200))
	defer p.Close()
	p.Scroll(ggui.Pt(50, 50), ggui.Pt(0, -3))
	before := s.position()
	if before <= 0 {
		t.Fatalf("the wheel did not scroll: offset %v", before)
	}
	moved.Set(true)
	p.Frame()
	if got := s.position(); got != before {
		t.Fatalf("the rebuilt scroll at another place is at %v, want the %v it was scrolled to", got, before)
	}
	if s.CaptureTouchDrag() {
		t.Fatal("a scroll nobody is dragging captures touch drags")
	}
}

// Under AlignBaseline a large title and a small caption share a baseline,
// and the Row is as tall as the tallest ascent plus the tallest descent.
func TestRowAlignBaseline(t *testing.T) {
	t.Parallel()
	title, caption := Text("Title").Size(28), Text("caption").Size(12)
	row := Row(title, caption).Align(AlignBaseline)
	size := row.Layout(ggui.Loose(ggui.Sz(400, 400)), ggui.Env{})

	tb, ok1 := title.Baseline()
	cb, ok2 := caption.Baseline()
	if !ok1 || !ok2 || tb <= cb {
		t.Fatalf("baselines: title %v %v, caption %v %v", tb, ok1, cb, ok2)
	}
	if !closeTo(row.rects[0].Origin.Y+tb, row.rects[1].Origin.Y+cb) {
		t.Fatalf("baselines differ: %v vs %v", row.rects[0].Origin.Y+tb, row.rects[1].Origin.Y+cb)
	}
	rb, ok := row.Baseline()
	if !ok || !closeTo(rb, tb) {
		t.Fatalf("row baseline = %v %v, want %v", rb, ok, tb)
	}
	below := max(row.rects[0].Size.H-tb, row.rects[1].Size.H-cb)
	if !closeTo(size.H, tb+below) {
		t.Fatalf("row height = %v, want %v", size.H, tb+below)
	}
}

// A child with no baseline of its own rests on the shared baseline by its
// bottom edge.
func TestRowAlignBaselineWithoutBaseline(t *testing.T) {
	t.Parallel()
	title, box := Text("Title").Size(28), Box().Size(10, 10)
	row := Row(title, box).Align(AlignBaseline)
	row.Layout(ggui.Loose(ggui.Sz(400, 400)), ggui.Env{})
	tb, _ := title.Baseline()
	if !closeTo(row.rects[1].Origin.Y+10, row.rects[0].Origin.Y+tb) {
		t.Fatalf("box bottom %v, baseline %v", row.rects[1].Origin.Y+10, row.rects[0].Origin.Y+tb)
	}
}

// Containers pass their first child's baseline up, moved by where the
// child went.
func TestBaselinePropagates(t *testing.T) {
	t.Parallel()
	col := Column(Box().Size(10, 5), Padding(Text("x"), 3)).Gap(2)
	col.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})
	cb, ok := col.Baseline()
	inner, _ := col.children[1].(ggui.Baseliner).Baseline()
	if !ok || !closeTo(cb, 5+2+inner) {
		t.Fatalf("column baseline = %v %v, want %v", cb, ok, 5+2+inner)
	}
	if _, ok := Box().Size(4, 4).Baseline(); ok {
		t.Fatal("an empty box has a baseline")
	}
}

// An If showing nothing is left out of its flow: no gap beside it in a
// Column, Row or Wrap, and justification counts the children that are there.
func TestIfShowingNothingTakesNoGap(t *testing.T) {
	t.Parallel()
	off := ggui.State(false)
	col := Column(Box().Size(10, 10), ggui.If(off, func() ggui.Widget {
		return Box().Size(10, 10)
	}), Box().Size(10, 10)).Gap(4)
	p := ggui.NewProbe(Align(col).At(0, 0), ggui.Sz(100, 100))
	p.Frame()
	if h := col.rects[0].Size.H + col.rects[1].Size.H + col.rects[2].Size.H; h != 20 || col.rects[2].Origin.Y != 14 {
		t.Fatalf("third child at y=%v with %v of content, want 14 after one box and one gap", col.rects[2].Origin.Y, h)
	}
	off.Set(true)
	p.Frame()
	if got := col.rects[2].Origin.Y; got != 28 {
		t.Fatalf("third child at y=%v once shown, want 28 after two boxes and two gaps", got)
	}

	off.Set(false)
	p.Frame()
	row := Row(Box().Size(10, 10), ggui.If(off, func() ggui.Widget {
		return Box().Size(10, 10)
	}), Box().Size(10, 10)).Justify(SpaceBetween)
	row.Layout(ggui.Tight(ggui.Sz(100, 10)), ggui.Untrack(ggui.UseEnv))
	if got := row.rects[2].Origin.X; got != 90 {
		t.Fatalf("space-between placed the last box at x=%v, want 90 with the absent child ignored", got)
	}

	wrap := Wrap(Box().Size(10, 10), ggui.If(off, func() ggui.Widget {
		return Box().Size(10, 10)
	}), Box().Size(10, 10)).Gap(4)
	if w := wrap.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Untrack(ggui.UseEnv)).W; w != 24 {
		t.Fatalf("wrap width = %v, want two boxes and one gap", w)
	}
}

func TestSpacingAndNamedTextStyles(t *testing.T) {
	t.Parallel()
	env := ggui.Env{}.With(ggui.SpacingKey, 12.0)
	key := ggui.NewEnvKey[ggui.TextStyle]("heading")
	env = env.With(key, ggui.TextStyle{Size: 24})
	col := Column(Box().Size(10, 10), Box().Size(10, 10)).Space(2)
	if h := col.Layout(ggui.Loose(ggui.Sz(100, 100)), env).H; h != 20+2*env.Spacing() {
		t.Fatalf("height = %v", h)
	}
	title := Text("x").StyleKey(key).Role(ggui.RoleHeading)
	title.Layout(ggui.Loose(ggui.Sz(100, 100)), env)
	if title.resolved.Size != 24 {
		t.Fatalf("title size = %v", title.resolved.Size)
	}
}

// A Contain laid out and painted at rest, inside another, allocates
// nothing: the Env it hands down is the one it handed down last time.
func TestAContainAtRestAllocatesNothing(t *testing.T) {
	c := Contain(Box(Contain(Box().Size(10, 10))).Size(50, 50))
	run := func() {
		size := c.Layout(ggui.Tight(ggui.Sz(50, 50)), ggui.Env{})
		c.Paint(nil, ggui.Rct(ggui.Point{}, size))
	}
	run()
	if n := testing.AllocsPerRun(100, run); n != 0 {
		t.Fatalf("a Contain at rest allocates %v times a frame", n)
	}
}

// Stretch is Align(AlignStretch).
func TestStretchIsAlignStretch(t *testing.T) {
	if Column().Stretch().align != AlignStretch || Row().Stretch().align != AlignStretch {
		t.Fatal("Stretch did not set AlignStretch")
	}
}

// A Box describes its box model and its border to the inspector, which the
// Layout and Computed panes show, as the box is when asked.
func TestBoxDescribesItsBoxModelToTheInspector(t *testing.T) {
	t.Parallel()
	box := Box(Box().Size(80, 30)).Pad(7, 11, 13, 17).Border(2, red)
	p := ggui.NewProbe(Align(box).At(0, 0), ggui.Sz(400, 400))
	defer p.Close()
	var fr *inspect.Frame
	p.OnInspect(func(f *inspect.Frame) { fr = f })
	p.Frame()
	i := slices.IndexFunc(fr.Nodes, func(n inspect.Node) bool { return n.Name == "Box" })
	if i < 0 {
		t.Fatalf("no Box among %+v", fr.Nodes)
	}
	info := fr.BoxOf(i)
	if !info.Valid || info.Padding != (inspect.Insets{Top: 7, Right: 11, Bottom: 13, Left: 17}) || info.Border != 2 || info.Content != ggui.Sz(80, 30) {
		t.Fatalf("incorrect box model: %+v", info)
	}
	if !slices.ContainsFunc(fr.Details(inspect.Computed, i), func(f inspect.Field) bool { return f.Key == "border" && f.Value == "2 px #ff0000" }) {
		t.Fatal("computed properties missed the actual border")
	}
	box.padding.Top = 9
	if fr.BoxOf(i).Padding.Top != 9 {
		t.Fatal("box model was frozen at trace time")
	}
}

func TestBoxClipKeepsTheChildInsideTheBorder(t *testing.T) {
	t.Parallel()
	var clip ggui.Rect
	var clipped bool
	inside := ggui.Rct(ggui.Pt(2, 2), ggui.Sz(76, 46))
	box := Box(clipProbe(ggui.Sz(80, 50), &clip, &clipped)).Radius(12).Border(2, color.Black)
	p := ggui.NewProbe(topLeft(box), ggui.Sz(200, 200))
	defer p.Close()
	img := ggfx.NewImage(200, 200)
	defer img.Deallocate()
	p.Draw(img)
	if clipped && clip == inside {
		t.Fatal("a box without Clip clipped its child")
	}
	box.Clip()
	p.Draw(img)
	if !clipped || clip != inside {
		t.Fatalf("child clipped to %v (%v), want the inside of the border", clip, clipped)
	}
	// Without a radius the clip is a plain rectangle and paints no layer.
	box.Radius(0)
	p.Draw(img)
	if !clipped || clip != inside || p.Layers() != 0 {
		t.Fatalf("square clip %v (%v) with %d layers", clip, clipped, p.Layers())
	}
}

func TestGapClearsSpaceAndWrapGridLayoutDoesNotMutateGap(t *testing.T) {
	t.Parallel()
	col := Column(Box().Size(10, 10), Box().Size(10, 10)).Space(4).Gap(3)
	if col.space != 0 || col.gap != 3 {
		t.Fatalf("Column.Space.Gap: space=%v gap=%v", col.space, col.gap)
	}
	row := Row(Box().Size(10, 10), Box().Size(10, 10)).Space(4).Gap(3).Align(AlignBaseline)
	if row.space != 0 || row.gap != 3 {
		t.Fatalf("Row.Space.Gap: space=%v gap=%v", row.space, row.gap)
	}
	if !slices.ContainsFunc(row.InspectFields(), func(f inspect.Field) bool { return f.Key == "align" && f.Value == "baseline" }) {
		t.Fatalf("Row AlignBaseline InspectFields = %+v, want baseline", row.InspectFields())
	}
	wrap := Wrap(Box().Size(10, 10), Box().Size(10, 10)).Gap(2).Space(3)
	wrap.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})
	if wrap.gap != 2 || wrap.runGap != 2 {
		t.Fatalf("Wrap.Layout mutated gap/runGap to %v/%v", wrap.gap, wrap.runGap)
	}
	wrap.Gap(5)
	if wrap.space != 0 {
		t.Fatalf("Wrap.Gap did not clear space=%v", wrap.space)
	}
	grid := Grid(2, Box().Size(10, 10), Box().Size(10, 10)).Gap(2).Space(3)
	grid.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{})
	if grid.gap != 2 || grid.rowGap != 2 {
		t.Fatalf("Grid.Layout mutated gap/rowGap to %v/%v", grid.gap, grid.rowGap)
	}
	grid.Gap(5)
	if grid.space != 0 {
		t.Fatalf("Grid.Gap did not clear space=%v", grid.space)
	}
}
