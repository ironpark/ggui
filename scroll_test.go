package ggui

import (
	"image/color"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	var painted [3]Rect
	pos := State(0.0)
	s := Scroll(tall(nil, &painted)).BindOffset(pos)
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
	t.Parallel()
	var got Point
	s := Pointer(Scroll(Box().Size(10, 10))).OnScroll(func(d Point) { got = d })
	var in inputState
	paintFrame(&in, s, Sz(100, 100))
	in.dispatch(frameInput{pos: Pt(5, 5), wheel: Pt(0, -1)})
	if got != (Point{X: 0, Y: -1}) {
		t.Fatalf("wheel did not fall through a scroll with no overflow: got %v", got)
	}
}

func TestClipNestsAndTrimsRegions(t *testing.T) {
	t.Parallel()
	var c Canvas
	inner := c.Clip(Rct(Pt(0, 0), Sz(50, 50))).Clip(Rct(Pt(25, 25), Sz(100, 100)))
	inner.HitPointer(Rct(Pt(0, 0), Sz(200, 200)), Pointer(Box()))
	inner.HitPointer(Rct(Pt(60, 60), Sz(10, 10)), Pointer(Box()))
	if len(c.hits) != 1 || c.hits[0].rect != Rct(Pt(25, 25), Sz(25, 25)) {
		t.Fatalf("hits = %+v, want one region trimmed to {25 25 25 25}", c.hits)
	}
}

func TestFillingWidgetsFallBackToContentWhenUnbounded(t *testing.T) {
	t.Parallel()
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

func TestScrollBarFollowsEnvironmentAndPreservesOverrides(t *testing.T) {
	t.Parallel()
	s := Scroll(Box().Size(100, 300))
	for _, style := range []ScrollStyle{{Color: color.Black}, {Color: color.White}} {
		s.Layout(Tight(Sz(100, 100)), Env{}.With(ScrollStyleKey, style))
		if s.bar != style.Color {
			t.Fatal("default scrollbar did not follow environment")
		}
	}
	s.Layout(Tight(Sz(100, 100)), Env{})
	r, g, b, a := s.bar.RGBA()
	// Premultiplied channels plus white behind alpha must not become white.
	if r+65535-a == 65535 && g+65535-a == 65535 && b+65535-a == 65535 {
		t.Fatal("scrollbar disappears on white")
	}
	s.Bar(red)
	s.Layout(Tight(Sz(100, 100)), Env{}.With(ScrollStyleKey, ScrollStyle{Color: color.White}))
	if s.bar != red {
		t.Fatal("environment replaced explicit scrollbar color")
	}
	s.Bar(nil)
	s.Layout(Tight(Sz(100, 100)), Env{})
	if s.bar != nil {
		t.Fatal("environment made a hidden scrollbar visible")
	}
}

func TestScrollThumbDrag(t *testing.T) {
	t.Parallel()
	for _, horizontal := range []bool{false, true} {
		offset := State(0.0)
		taps := 0
		content := Sz(100, 500)
		if horizontal {
			content = Sz(500, 100)
		}
		s := Scroll(Tap(Box().Size(content.W, content.H), func() { taps++ })).BindOffset(offset)
		if horizontal {
			s.Horizontal()
		}
		var in inputState
		paint := func() { paintFrame(&in, s, Sz(100, 100)) }
		paint()
		grab := s.thumbHit.Origin.Add(Pt(s.thumbHit.Size.W/2, s.thumbHit.Size.H/2))
		in.dispatch(frameInput{pos: grab, down: []MouseButton{MouseButtonLeft}})
		if !s.dragging || !s.CaptureTouchDrag() {
			t.Fatal("thumb did not capture press")
		}
		delta := Pt(0, 40)
		if horizontal {
			delta = Pt(40, 0)
		}
		paint()
		in.dispatch(frameInput{pos: grab.Add(delta)})
		if got := Untrack(offset.Get); got != 200 {
			t.Fatalf("horizontal=%v offset=%v, want 200", horizontal, got)
		}
		paint()
		in.dispatch(frameInput{pos: Pt(1000, 1000)})
		if got := Untrack(offset.Get); got != 400 {
			t.Fatalf("drag outside should clamp to end, got %v", got)
		}
		paint()
		in.dispatch(frameInput{pos: Pt(1000, 1000), up: []MouseButton{MouseButtonLeft}})
		if s.dragging || taps != 0 {
			t.Fatalf("dragging=%v content taps=%d", s.dragging, taps)
		}
		paint()
		in.dispatch(frameInput{pos: Pt(0, 0)})
		if Untrack(offset.Get) != 400 {
			t.Fatal("scroll moved after release")
		}
	}
}

func TestScrollHiddenAndTinyThumb(t *testing.T) {
	t.Parallel()
	s := Scroll(Box().Size(100, 500)).Bar(nil)
	var in inputState
	paintFrame(&in, s, Sz(100, 100))
	if s.HandlePointer(PointerEvent{Kind: PointerDown, Button: MouseButtonLeft, Pos: Pt(98, 5)}) {
		t.Fatal("hidden scrollbar intercepted press")
	}
	s.Bar((Env{}).ScrollStyle().Color)
	paintFrame(&in, s, Sz(100, 8))
	if s.thumbLength() > 8 {
		t.Fatal("thumb extends beyond tiny viewport")
	}
}

func TestScrollThumbHoverAndDragFeedback(t *testing.T) {
	t.Parallel()
	s := Scroll(Box().Size(100, 500))
	var in inputState
	paintFrame(&in, s, Sz(100, 100))
	at := s.thumbHit.Origin.Add(Pt(5, 5))
	in.dispatch(frameInput{pos: at})
	if !s.thumbHovered {
		t.Fatal("thumb hover not activated")
	}
	in.dispatch(frameInput{pos: at, down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(200, 200)})
	if s.thumbHovered || !s.dragging {
		t.Fatal("leaving thumb should retain drag feedback only")
	}
	in.dispatch(frameInput{pos: Pt(200, 200), up: []MouseButton{MouseButtonLeft}})
	if s.thumbHovered || s.dragging {
		t.Fatal("feedback remains after release outside")
	}
}

// FollowEnd keeps a growing log at its end until the user scrolls back,
// and follows again once they scroll to the end, across rebuilds and with
// a bound offset alike.
func TestScrollFollowEnd(t *testing.T) {
	t.Parallel()
	for _, bound := range []bool{false, true} {
		lines, rebuild := State(10), State(0)
		pos := State(0.0)
		p := ProbeBuilder(func() Widget {
			rebuild.Get()
			s := Scroll(Reactive(func() Widget { return Box().Size(50, float64(lines.Get()*20)) })).FollowEnd()
			if bound {
				s.BindOffset(pos)
			}
			return s
		}, Sz(50, 100))
		offset := func() float64 { return Untrack(p.root.(*ScrollWidget).position) }
		step := func(what string, want float64) {
			t.Helper()
			p.Frame()
			if got := offset(); got != want {
				t.Fatalf("bound=%v, %s: offset %v, want %v", bound, what, got, want)
			}
		}
		step("opened", 100)
		lines.Set(15)
		step("grew at the end", 200)
		p.Scroll(Pt(10, 10), Pt(0, 2)) // back 40
		step("scrolled back", 160)
		lines.Set(20)
		step("grew while scrolled back", 160)
		rebuild.Set(1)
		step("rebuilt while scrolled back", 160)
		p.Scroll(Pt(10, 10), Pt(0, -100))
		step("scrolled to the end", 300)
		lines.Set(25)
		step("grew after returning", 400)
		rebuild.Set(2)
		lines.Set(30)
		step("rebuilt and grew at the end", 500)
		p.Close()
	}
}

// A scroll rebuilt with its content, as a View showing a growing log
// rebuilds it every line, keeps following or not as the old one did: the
// new one is laid out before it adopts the old one's state.
func TestScrollFollowEndAcrossContentRebuilds(t *testing.T) {
	t.Parallel()
	for _, bound := range []bool{false, true} {
		lines := State(10)
		pos := State(0.0)
		var current *ScrollWidget
		p := ProbeBuilder(func() Widget {
			return Column(Text("header"), View(lines, func(n int) Widget {
				current = Scroll(Box().Size(50, float64(n*20))).FollowEnd()
				if bound {
					current.BindOffset(pos)
				}
				return Box(current).Height(100)
			}))
		}, Sz(50, 200))
		offset := func() float64 { return Untrack(current.position) }
		step := func(what string, want float64) {
			t.Helper()
			p.Frame()
			p.Frame()
			if got := offset(); got != want {
				t.Fatalf("bound=%v, %s: offset %v, want %v", bound, what, got, want)
			}
		}
		step("opened", 100)
		lines.Set(15)
		step("grew at the end", 200)
		p.Scroll(Pt(10, 50), Pt(0, 2))
		step("scrolled back", 160)
		lines.Set(20)
		step("grew while scrolled back", 160)
		p.Scroll(Pt(10, 50), Pt(0, -100))
		step("scrolled to the end", 300)
		lines.Set(25)
		step("grew after returning", 400)
		p.Close()
	}
}
