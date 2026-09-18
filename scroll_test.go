package ggui

import (
	"testing"
)

// tall is a 100x300 probe: three 100-high boxes with the middle one tappable.
func tall(taps *int, painted *[3]Rect) Widget {
	return Column(
		probe(100, 100, &painted[0]),
		Tap(probe(100, 100, &painted[1]), func() { *taps++ }),
		probe(100, 100, &painted[2]),
	)
}

func TestScrollGivesChildUnboundedHeightAndFillsViewport(t *testing.T) {
	var painted [3]Rect
	s := Scroll(tall(nil, &painted))
	got := s.Layout(Loose(Sz(100, 120)), Env{})
	if got != (Size{W: 100, H: 120}) {
		t.Fatalf("Layout() = %+v, want the viewport {100 120}", got)
	}
	if s.childSize.H != 300 {
		t.Fatalf("child laid out %v high, want 300 unclipped", s.childSize.H)
	}
}

func TestScrollWheelMovesAndClamps(t *testing.T) {
	var painted [3]Rect
	s := Scroll(tall(nil, &painted)).Speed(10)
	size := s.Layout(Loose(Sz(100, 120)), Env{})
	var in inputState
	paint := func() {
		var c Canvas
		s.Paint(&c, Rct(Pt(0, 0), size))
		in.regions = c.hits
	}
	paint()
	in.dispatch(frameInput{pos: Pt(50, 50), wheel: Pt(0, -5)}) // wheel down: content moves up
	s.Layout(Loose(Sz(100, 120)), Env{})
	paint()
	if painted[0].Origin.Y != -50 {
		t.Fatalf("first child at y=%v after scrolling 50, want -50", painted[0].Origin.Y)
	}
	in.dispatch(frameInput{pos: Pt(50, 50), wheel: Pt(0, -100)})
	s.Layout(Loose(Sz(100, 120)), Env{})
	paint()
	if painted[0].Origin.Y != -180 {
		t.Fatalf("first child at y=%v, want clamped to -(300-120) = -180", painted[0].Origin.Y)
	}
	in.dispatch(frameInput{pos: Pt(50, 50), wheel: Pt(0, 100)})
	s.Layout(Loose(Sz(100, 120)), Env{})
	if s.position() != 0 {
		t.Fatalf("offset = %v after scrolling back past the top, want 0", s.position())
	}
}

func TestScrollClipsHitRegions(t *testing.T) {
	taps := 0
	var painted [3]Rect
	s := Scroll(tall(&taps, &painted)).Speed(10)
	size := s.Layout(Loose(Sz(100, 120)), Env{})
	var in inputState
	paint := func() {
		var c Canvas
		s.Paint(&c, Rct(Pt(0, 0), size))
		in.regions = c.hits
	}
	click := func(p Point) {
		in.dispatch(frameInput{pos: p, down: []MouseButton{MouseButtonLeft}})
		in.dispatch(frameInput{pos: p, up: []MouseButton{MouseButtonLeft}})
	}
	paint()
	click(Pt(50, 150)) // the tappable box starts at y=100 but the viewport ends at 120
	if taps != 0 {
		t.Fatalf("taps = %d on a point outside the viewport, want 0", taps)
	}
	click(Pt(50, 110)) // the visible sliver of it
	if taps != 1 {
		t.Fatalf("taps = %d on the visible part, want 1", taps)
	}
	in.dispatch(frameInput{pos: Pt(50, 50), wheel: Pt(0, -10)}) // scroll by 100
	s.Layout(Loose(Sz(100, 120)), Env{})
	paint()
	click(Pt(50, 50)) // now the tappable box fills the viewport top
	if taps != 2 {
		t.Fatalf("taps = %d after scrolling the box into view, want 2", taps)
	}
}

func TestScrollOffsetBinding(t *testing.T) {
	var painted [3]Rect
	pos := State(0.0)
	s := Scroll(tall(nil, &painted)).Offset(pos)
	pos.Set(1000)
	size := s.Layout(Loose(Sz(100, 120)), Env{})
	if Untrack(pos.Get) != 180 {
		t.Fatalf("bound offset = %v after layout, want clamped 180", Untrack(pos.Get))
	}
	s.Paint(nil, Rct(Pt(0, 0), size))
	if painted[2].Origin.Y != 20 {
		t.Fatalf("last child at y=%v, want 20", painted[2].Origin.Y)
	}
}

func TestScrollDoesNotConsumeWheelWithNothingToScroll(t *testing.T) {
	var got Point
	s := Pointer(Scroll(Box().Size(10, 10))).OnScroll(func(d Point) { got = d })
	var in inputState
	paintFrame(&in, s, Sz(100, 100))
	in.dispatch(frameInput{pos: Pt(5, 5), wheel: Pt(0, -1)})
	if got != (Point{0, -1}) {
		t.Fatalf("wheel did not fall through a scroll with no overflow: got %v", got)
	}
}

func TestClipNestsAndTrimsRegions(t *testing.T) {
	var c Canvas
	inner := c.Clip(Rct(Pt(0, 0), Sz(50, 50))).Clip(Rct(Pt(25, 25), Sz(100, 100)))
	inner.HitPointer(Rct(Pt(0, 0), Sz(200, 200)), Pointer(Box()))
	inner.HitPointer(Rct(Pt(60, 60), Sz(10, 10)), Pointer(Box()))
	if len(c.hits) != 1 || c.hits[0].rect != Rct(Pt(25, 25), Sz(25, 25)) {
		t.Fatalf("hits = %+v, want one region trimmed to {25 25 25 25}", c.hits)
	}
}

func TestFillingWidgetsFallBackToContentWhenUnbounded(t *testing.T) {
	unb := Constraints{MaxW: 100, MaxH: Unbounded}
	cases := map[string]Widget{
		"align":   Align(Box().Size(10, 10)),
		"stack":   Stack(Box().Size(10, 10)).Expand(),
		"justify": Column(Box().Size(10, 10)).Justify(JustifyCenter),
		"stretch": Row(Box().Size(10, 10)).Align(AlignStretch),
		"flex":    Column(Expanded(Box().Size(10, 10))),
	}
	for name, w := range cases {
		got := w.Layout(unb, Env{})
		if got.H != 10 {
			t.Fatalf("%s: height %v under an unbounded axis, want the content's 10", name, got.H)
		}
	}
}

func TestScrollBarFollowsThemeAndPreservesOverrides(t *testing.T) {
	s := Scroll(Box().Size(100, 300))
	for _, theme := range []Theme{DefaultTheme(), DarkTheme()} {
		s.Layout(Tight(Sz(100, 100)), Env{}.WithTheme(theme))
		if s.bar != theme.MutedFg {
			t.Fatal("default scrollbar did not follow theme")
		}
	}
	s.Layout(Tight(Sz(100, 100)), Env{}.WithTheme(DefaultTheme()))
	r, g, b, a := s.bar.RGBA()
	// Premultiplied channels plus white behind alpha must not become white.
	if r+65535-a == 65535 && g+65535-a == 65535 && b+65535-a == 65535 {
		t.Fatal("scrollbar disappears on white")
	}
	s.Bar(red)
	s.Layout(Tight(Sz(100, 100)), Env{}.WithTheme(DarkTheme()))
	if s.bar != red {
		t.Fatal("theme replaced explicit scrollbar color")
	}
	s.Bar(nil)
	s.Layout(Tight(Sz(100, 100)), Env{}.WithTheme(DefaultTheme()))
	if s.bar != nil {
		t.Fatal("theme made a hidden scrollbar visible")
	}
}
