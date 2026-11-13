package ggui

import (
	"image/color"
	"math"
	"testing"
	"time"
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

// rows is a column of n probes h high whose painted rects land in got.
func rows(n int, h float64, got []Rect) Widget {
	children := make([]Widget, n)
	for i := range children {
		children[i] = probe(100, h, &got[i])
	}
	return Column(children...)
}

func TestStickyPinsItsHeaderUntilTheNextSectionPushesItOut(t *testing.T) {
	t.Parallel()
	var headA, bodyA, headB, bodyB Rect
	a := Sticky(probe(100, 20, &headA), probe(100, 100, &bodyA))
	b := Sticky(probe(100, 20, &headB), probe(100, 100, &bodyB))
	offset := State(0.0)
	p := NewProbe(Scroll(Column(a, b)).BindOffset(offset), Sz(100, 60))
	defer p.Close()
	p.Frame()
	if headA.Origin.Y != 0 || bodyA.Origin.Y != 20 || headA.Size.W != 100 || a.Pinned() {
		t.Fatalf("at the top: header %v body %v pinned %v; want the header in its place", headA, bodyA, a.Pinned())
	}
	offset.Set(50)
	p.Frame()
	if headA.Origin.Y != 0 || bodyA.Origin.Y != -30 || !a.Pinned() {
		t.Fatalf("scrolled 50: header %v body %v pinned %v; want the header pinned at the top", headA, bodyA, a.Pinned())
	}
	offset.Set(110)
	p.Frame()
	// Section A spans 0..120, so its header can go no lower than 100.
	if headA.Origin.Y != -10 || headB.Origin.Y != 10 || b.Pinned() {
		t.Fatalf("scrolled 110: header A %v, B %v; want A pushed up to -10 by B at 10", headA, headB)
	}
	offset.Set(150)
	p.Frame()
	if headB.Origin.Y != 0 || !b.Pinned() {
		t.Fatalf("scrolled 150: header B %v; want it pinned", headB)
	}
}

func TestStickyOutsideAScrollIsAHeaderAboveItsBody(t *testing.T) {
	t.Parallel()
	var head, body Rect
	p := NewProbe(Sticky(probe(50, 20, &head), probe(80, 30, &body)), Sz(100, 100))
	defer p.Close()
	p.Frame()
	if head.Origin.Y != 0 || body.Origin.Y != 20 || head.Size.W != 100 {
		t.Fatalf("header %v, body %v; want the header stretched above the body", head, body)
	}
}

func TestEachStickyHeadersPinInPlainAndVirtualLists(t *testing.T) {
	t.Parallel()
	for _, virtual := range []bool{false, true} {
		n := 100
		list := make([]int, n)
		for i := range list {
			list[i] = i
		}
		items := State(list)
		painted := map[int]Rect{}
		each := EachKeyed(items, func(v int) int { return v }, func(it EachItem[int]) Widget {
			v := it.Item
			return FromFuncs(
				func(c Constraints, _ Env) Size { return c.Constrain(Sz(100, 20)) },
				func(_ *Canvas, r Rect) { painted[v] = r },
			)
		}).StickyHeaders(func(v int) bool { return v%10 == 0 })
		if virtual {
			each.ItemExtent(20)
		}
		offset := State(250.0)
		p := NewProbe(Scroll(each).BindOffset(offset), Sz(100, 60))
		p.Frame()
		if r := painted[10]; r.Origin.Y != 0 {
			t.Fatalf("virtual %v at 250: header 10 at %v, want pinned at 0", virtual, r)
		}
		clear(painted)
		offset.Set(390)
		p.Frame()
		if painted[10].Origin.Y != -10 || painted[20].Origin.Y != 10 {
			t.Fatalf("virtual %v at 390: header 10 at %v, 20 at %v; want 10 pushed to -10", virtual, painted[10], painted[20])
		}
		p.Close()
	}
}

func TestScrollControllerAnimatesJumpsAndAddsUp(t *testing.T) {
	t.Parallel()
	got := make([]Rect, 20)
	var sc ScrollController
	p := NewProbe(Scroll(rows(20, 50, got)).Controller(&sc), Sz(100, 100))
	defer p.Close()
	sc.JumpTo(200) // before the first layout: kept until then
	p.Frame()
	if sc.Offset() != 200 || got[4].Origin.Y != 0 || sc.MaxOffset() != 900 {
		t.Fatalf("JumpTo before layout: offset %v, row 4 at %v, max %v", sc.Offset(), got[4], sc.MaxOffset())
	}
	sc.ScrollTo(600)
	p.Frame()
	p.Advance(100 * time.Millisecond)
	if mid := sc.Offset(); mid <= 200 || mid >= 600 || !sc.Animating() {
		t.Fatalf("ScrollTo midway: offset %v animating %v; want between 200 and 600", mid, sc.Animating())
	}
	p.Advance(time.Second)
	p.Frame()
	if sc.Offset() != 600 || sc.Animating() || got[12].Origin.Y != 0 {
		t.Fatalf("ScrollTo done: offset %v, row 12 at %v", sc.Offset(), got[12])
	}
	sc.ScrollBy(100)
	sc.ScrollBy(100)
	p.Advance(time.Second)
	p.Frame()
	if sc.Offset() != 800 {
		t.Fatalf("two ScrollBy(100) from 600 reached %v, want 800", sc.Offset())
	}
	sc.ScrollTo(math.Inf(1))
	p.Advance(time.Second)
	p.Frame()
	if sc.Offset() != 900 {
		t.Fatalf("ScrollTo(+Inf) reached %v, want the end, 900", sc.Offset())
	}
	sc.ScrollTo(0)
	p.Frame()
	p.Scroll(Pt(50, 50), Pt(0, -1)) // the wheel takes over
	if sc.Animating() {
		t.Fatal("wheel input left the controller's animation running")
	}
}

func TestScrollControllerUnderReducedMotionJumps(t *testing.T) {
	t.Parallel()
	got := make([]Rect, 20)
	var sc ScrollController
	p := NewProbe(Provide(ReducedMotionKey, true, Scroll(rows(20, 50, got)).Controller(&sc)), Sz(100, 100))
	defer p.Close()
	p.Frame()
	sc.ScrollTo(300)
	p.Frame()
	if sc.Offset() != 300 || sc.Animating() {
		t.Fatalf("reduced motion: offset %v animating %v; want a jump to 300", sc.Offset(), sc.Animating())
	}
}

func TestScrollControllerScrollsToAKeyedRow(t *testing.T) {
	t.Parallel()
	for _, virtual := range []bool{false, true} {
		list := make([]int, 1000)
		for i := range list {
			list[i] = i * 7
		}
		each := EachKeyed(State(list), func(v int) int { return v }, func(EachItem[int]) Widget { return Box().Size(100, 20) })
		if virtual {
			each.ItemExtent(20)
		}
		var head Rect
		var sc ScrollController
		p := NewProbe(Scroll(Column(probe(100, 50, &head), each)).Controller(&sc), Sz(100, 100))
		p.Frame()
		sc.ScrollToKey(70) // the eleventh row, at 50 + 10*20
		p.Frame()
		p.Advance(time.Second)
		p.Frame()
		if sc.Offset() != 250 {
			t.Fatalf("virtual %v: ScrollToKey(70) reached %v, want 250", virtual, sc.Offset())
		}
		sc.JumpToKey(7 * 999)
		p.Frame()
		p.Frame()
		if sc.Offset() != sc.MaxOffset() || sc.MaxOffset() != 50+20000-100 {
			t.Fatalf("virtual %v: JumpToKey(last) reached %v, max %v", virtual, sc.Offset(), sc.MaxOffset())
		}
		sc.ScrollToKey("not a key")
		p.Frame()
		if sc.Offset() != sc.MaxOffset() {
			t.Fatal("a key of another type moved the scroll")
		}
		p.Close()
	}
}

func TestScrollBounceFollowsPastTheEndAndSpringsBack(t *testing.T) {
	t.Parallel()
	var painted [3]Rect
	s := Scroll(tall(nil, &painted)).Speed(1).Overscroll(OverscrollBounce)
	p := NewProbe(s, Sz(100, 120))
	defer p.Close()
	p.Frame()
	p.Scroll(Pt(50, 50), Pt(0, 40)) // pull down at the top
	p.Frame()
	if s.position() != 0 || painted[0].Origin.Y <= 0 || painted[0].Origin.Y >= 40 {
		t.Fatalf("pulled 40 at the top: offset %v, first row at %v; want it to follow, resisted", s.position(), painted[0])
	}
	p.Advance(200 * time.Millisecond)
	p.Advance(time.Second)
	p.Frame()
	if painted[0].Origin.Y != 0 || s.phys.over != 0 {
		t.Fatalf("after the pause: first row at %v, over %v; want it sprung back", painted[0], s.phys.over)
	}
	// A fling into the end stops once it has bounced far enough.
	stopped := false
	for range 50 {
		if !s.HandlePointer(PointerEvent{Kind: PointerScroll, Scroll: Pt(0, -60), ScrollPixels: true, ScrollMomentum: true}) {
			stopped = true
			break
		}
	}
	if !stopped || s.position() != s.maxOffset() || s.phys.over >= 0 {
		t.Fatalf("fling past the end: stopped %v, offset %v, over %v", stopped, s.position(), s.phys.over)
	}
}

func TestScrollClampsByDefaultAndStretchKeepsContentInPlace(t *testing.T) {
	t.Parallel()
	var painted [3]Rect
	s := Scroll(tall(nil, &painted)).Speed(1)
	p := NewProbe(s, Sz(100, 120))
	p.Scroll(Pt(50, 50), Pt(0, 40))
	p.Frame()
	if s.phys.over != 0 || painted[0].Origin.Y != 0 {
		t.Fatalf("default scroll overscrolled: over %v, row at %v", s.phys.over, painted[0])
	}
	p.Close()

	s = Scroll(tall(nil, &painted)).Speed(1).Overscroll(OverscrollStretch)
	p = NewProbe(s, Sz(100, 120))
	defer p.Close()
	p.Frame()
	p.Scroll(Pt(50, 50), Pt(0, 40))
	p.Frame()
	if s.phys.over <= 0 || painted[0].Origin.Y != 0 {
		t.Fatalf("stretch: over %v, row at %v; want the pull recorded and the content in place", s.phys.over, painted[0])
	}
}

func TestScrollSnapSettlesOnTheNearestStop(t *testing.T) {
	t.Parallel()
	got := make([]Rect, 10)
	s := Scroll(rows(10, 50, got)).Speed(1).Snap(50)
	p := NewProbe(s, Sz(100, 120))
	defer p.Close()
	p.Frame()
	p.Scroll(Pt(50, 50), Pt(0, -70))
	if s.position() != 70 {
		t.Fatalf("offset %v before settling, want 70", s.position())
	}
	p.Advance(50 * time.Millisecond)
	if s.position() != 70 {
		t.Fatalf("snapped to %v before the input paused", s.position())
	}
	p.Advance(100 * time.Millisecond)
	p.Advance(time.Second)
	p.Frame()
	if s.position() != 50 {
		t.Fatalf("settled at %v, want 50", s.position())
	}
	p.Scroll(Pt(50, 50), Pt(0, -1000))
	p.Advance(time.Second)
	p.Advance(time.Second)
	if s.position() != 380 {
		t.Fatalf("at the end settled at %v, want the end, 380", s.position())
	}
}

func TestScrollPagingTurnsAPageInTheGesturesDirection(t *testing.T) {
	t.Parallel()
	got := make([]Rect, 10)
	s := Scroll(rows(10, 50, got)).Speed(1).Paging()
	p := NewProbe(s, Sz(100, 100))
	defer p.Close()
	p.Frame()
	settle := func() {
		p.Advance(200 * time.Millisecond)
		p.Advance(time.Second)
		p.Frame()
	}
	p.Scroll(Pt(50, 50), Pt(0, -10))
	settle()
	if s.position() != 0 {
		t.Fatalf("a short gesture settled at %v, want back at 0", s.position())
	}
	p.Scroll(Pt(50, 50), Pt(0, -30))
	settle()
	if s.position() != 100 {
		t.Fatalf("a longer gesture settled at %v, want the next page, 100", s.position())
	}
	p.Scroll(Pt(50, 50), Pt(0, 25))
	settle()
	if s.position() != 0 {
		t.Fatalf("back by a quarter page settled at %v, want the previous page, 0", s.position())
	}
}

func TestScrollControllerRequestSurvivesARebuiltScroll(t *testing.T) {
	t.Parallel()
	got := make([]Rect, 20)
	var sc ScrollController
	tick := State(0)
	p := NewProbe(Reactive(func() Widget {
		_ = tick.Get()
		return Scroll(rows(20, 50, got)).Controller(&sc)
	}), Sz(100, 100))
	defer p.Close()
	p.Frame()
	sc.JumpTo(500)
	p.Frame()
	// The rebuilt scroll is laid out before it adopts the one it replaces;
	// a request made in between must not be overwritten by the adoption.
	tick.Set(1)
	p.Flush()
	sc.JumpTo(100)
	p.Frame()
	p.Frame()
	if sc.Offset() != 100 || got[2].Origin.Y != 0 {
		t.Fatalf("JumpTo across a rebuild: offset %v, row 2 at %v; want 100", sc.Offset(), got[2])
	}
}

func TestRevealTakesOverFromAnEasedScroll(t *testing.T) {
	t.Parallel()
	reveal := -1
	children := make([]Widget, 20)
	for i := range children {
		children[i] = &revealer{i: i, want: &reveal}
	}
	var sc ScrollController
	p := NewProbe(Scroll(Column(children...)).Controller(&sc), Sz(50, 100))
	defer p.Close()
	p.Frame()
	sc.ScrollTo(900)
	p.Advance(50 * time.Millisecond)
	reveal = 0
	p.Frame()
	reveal = -1
	p.Advance(time.Second)
	p.Frame()
	if sc.Animating() || sc.Offset() != 0 {
		t.Fatalf("after revealing row 0 mid-scroll: offset %v animating %v; want the reveal to win", sc.Offset(), sc.Animating())
	}
}

// wheeled is a window of a scroll, the left half of it, over content 2000
// tall, with config to configure the scroll, and its controller.
func wheeled(config func(*ScrollWidget)) (*WindowProbe, *ScrollController) {
	sc := &ScrollController{}
	p := NewWindowProbe(func() Widget {
		s := Scroll(Box().Size(100, 2000)).Controller(sc)
		if config != nil {
			config(s)
		}
		return Row(Box(s).Size(100, 100), Box().Size(100, 100))
	}, Sz(200, 100))
	p.Run(frameTime, 10)
	return p, sc
}

// offsets runs p's frames until the window rests and returns the scroll's
// offset after each.
func offsets(t *testing.T, p *WindowProbe, sc *ScrollController) (at []float64) {
	t.Helper()
	for range 60 {
		fr := p.Step(frameTime)
		at = append(at, sc.Offset())
		if !fr.Next {
			return at
		}
	}
	t.Fatalf("the window did not come to rest; offsets %v", at)
	return nil
}

// A wheel's notch moves a view as far as the platform scrolls for it, not
// a distance of the view's own, and the view eases over it: Windows and a
// browser scroll a hundred pixels a notch, and a jump that long loses the
// reader's place.
func TestAWheelsNotchScrollsThePlatformsDistanceEased(t *testing.T) {
	p, sc := wheeled(nil)
	defer p.Close()
	p.Wheel(Pt(50, 50), Pt(0, -1), 100)
	at := offsets(t, p, sc)
	if got := at[len(at)-1]; got != 100 {
		t.Fatalf("a notch of 100 came to rest at %v; offsets %v", got, at)
	}
	between := 0
	for i, v := range at {
		if i > 0 && v < at[i-1] {
			t.Fatalf("the notch went back: %v", at)
		}
		if v > 0 && v < 100 {
			between++
		}
	}
	if between < 3 {
		t.Fatalf("the notch jumped: offsets %v", at)
	}
	if n := len(at); n > 15 {
		t.Fatalf("the notch took %d frames; a wheel must not feel slow: %v", n, at)
	}
	// Another platform's notch is another distance.
	p.Wheel(Pt(50, 50), Pt(0, -1), 53)
	if at := offsets(t, p, sc); at[len(at)-1] != 153 {
		t.Fatalf("a notch of 53 more came to rest at %v", at[len(at)-1])
	}
}

// Notches that come while the last is still easing add to where it was
// heading: three quick notches scroll three notches.
func TestWheelNotchesAddUpWhileEasing(t *testing.T) {
	p, sc := wheeled(nil)
	defer p.Close()
	for range 3 {
		p.Wheel(Pt(50, 50), Pt(0, -1), 100)
		p.Step(frameTime)
		p.Step(frameTime)
	}
	if at := offsets(t, p, sc); at[len(at)-1] != 300 {
		t.Fatalf("three notches of 100 came to rest at %v", at[len(at)-1])
	}
	// And one back takes one off, from wherever the view has got to.
	p.Wheel(Pt(50, 50), Pt(0, -1), 100)
	p.Step(frameTime)
	p.Wheel(Pt(50, 50), Pt(0, 1), 100)
	if at := offsets(t, p, sc); at[len(at)-1] != 300 {
		t.Fatalf("a notch on and a notch back came to rest at %v, not where they began", at[len(at)-1])
	}
}

// A trackpad's scroll is the view's movement itself, momentum and all, and
// is followed as it comes: easing it would trail the fingers.
func TestATrackpadsScrollIsFollowedAtOnce(t *testing.T) {
	p, sc := wheeled(nil)
	defer p.Close()
	p.Trackpad(Pt(50, 50), Pt(0, -37.5))
	p.Step(frameTime)
	if got := sc.Offset(); got != 37.5 {
		t.Fatalf("a trackpad's 37.5 pixels moved the view %v in its frame", got)
	}
}

// A view eases nothing under reduced motion, and Speed still sets a
// distance of the view's own for a wheel's unit.
func TestAWheelsNotchJumpsUnderReducedMotionAndSpeedSetsItsDistance(t *testing.T) {
	sc := &ScrollController{}
	p := NewWindowProbe(func() Widget {
		return Provide(ReducedMotionKey, true, Box(Scroll(Box().Size(100, 2000)).Controller(sc)).Size(100, 100))
	}, Sz(200, 100))
	p.Run(frameTime, 10)
	p.Wheel(Pt(50, 50), Pt(0, -1), 100)
	p.Step(frameTime)
	if got := sc.Offset(); got != 100 {
		t.Fatalf("under reduced motion a notch moved the view %v in its frame", got)
	}
	p.Close()

	p, sc = wheeled(func(s *ScrollWidget) { s.Speed(30) })
	defer p.Close()
	p.Wheel(Pt(50, 50), Pt(0, -2), 100)
	if at := offsets(t, p, sc); at[len(at)-1] != 60 {
		t.Fatalf("two notches at Speed(30) came to rest at %v", at[len(at)-1])
	}
}

// A notch at the end of the content is the scroll around's: the inner one
// does not take it, and the outer eases as it would for its own.
func TestAWheelsNotchAtTheEndGoesToTheScrollAround(t *testing.T) {
	inner, outer := &ScrollController{}, &ScrollController{}
	p := NewWindowProbe(func() Widget {
		in := Box(Scroll(Box().Size(100, 150)).Controller(inner)).Size(100, 100)
		return Scroll(Column(in, Box().Size(100, 2000))).Controller(outer)
	}, Sz(200, 100))
	defer p.Close()
	p.Run(frameTime, 10)
	p.Wheel(Pt(50, 50), Pt(0, -1), 100)
	for range 30 {
		p.Step(frameTime)
	}
	if inner.Offset() != 50 || outer.Offset() != 0 {
		t.Fatalf("a notch moved the inner scroll to %v of 50 and the outer to %v", inner.Offset(), outer.Offset())
	}
	p.Wheel(Pt(50, 50), Pt(0, -1), 100)
	for range 30 {
		p.Step(frameTime)
	}
	if inner.Offset() != 50 || outer.Offset() != 100 {
		t.Fatalf("a notch at the inner scroll's end left it at %v and moved the outer to %v, not 100", inner.Offset(), outer.Offset())
	}
}

// A snapping scroll settles from where a notch led, not from part of the
// way there.
func TestAWheelsNotchOnASnappingScrollSettlesFromWhereItLed(t *testing.T) {
	p, sc := wheeled(func(s *ScrollWidget) { s.Snap(40) })
	defer p.Close()
	p.Wheel(Pt(50, 50), Pt(0, -1), 100)
	var at []float64
	for range 80 {
		fr := p.Step(frameTime)
		at = append(at, sc.Offset())
		if !fr.Next && fr.Wake.IsZero() {
			break
		}
	}
	if got := at[len(at)-1]; got != 80 && got != 120 {
		t.Fatalf("a notch of 100 on a scroll snapping by 40 came to rest at %v; offsets %v", got, at)
	}
	reached := false
	for _, v := range at {
		reached = reached || v == 100
	}
	if !reached {
		t.Fatalf("the snap cut the notch short: offsets %v", at)
	}
}

// The frames a notch eases over are painted within the viewport.
func TestAWheelsNotchEasingPaintsWithinTheViewport(t *testing.T) {
	p, _ := wheeled(nil)
	defer p.Close()
	p.Wheel(Pt(50, 50), Pt(0, -1), 100)
	frames := p.Run(frameTime, 60)
	for i, fr := range frames[1:] {
		if fr.Painted != FrameContained {
			t.Fatalf("frame %d of the notch was painted %v: %v", i+1, fr.Painted, frames)
		}
	}
}

// What a window's scroll events say of distance reaches a handler in
// ScrollBy, and an event that says nothing counts WheelLine a unit.
func TestScrollByIsThePlatformsDistance(t *testing.T) {
	if got := (PointerEvent{Kind: PointerScroll, Scroll: Pt(0, -2)}).ScrollBy(); got != Pt(0, -2*WheelLine) {
		t.Fatalf("an event that says no distance scrolls by %v", got)
	}
	if got := (PointerEvent{Kind: PointerScroll, Scroll: Pt(3, -7), ScrollPixels: true}).ScrollBy(); got != Pt(3, -7) {
		t.Fatalf("a scroll in pixels scrolls by %v", got)
	}
	var got []PointerEvent
	p := NewWindowProbe(func() Widget { return &scrollCatcher{got: &got} }, Sz(100, 100))
	defer p.Close()
	p.Run(frameTime, 3)
	p.Wheel(Pt(50, 50), Pt(0, -1), 100)
	p.Wheel(Pt(50, 50), Pt(0, -2), 100)
	p.Step(frameTime)
	if len(got) != 1 || got[0].Scroll != Pt(0, -3) || got[0].ScrollBy() != Pt(0, -300) || !got[0].ScrollStep || got[0].ScrollPixels {
		t.Fatalf("three notches of 100 reached the handler as %+v", got)
	}
	got = nil
	p.Trackpad(Pt(50, 50), Pt(0, -12))
	p.Step(frameTime)
	if len(got) != 1 || got[0].ScrollBy() != Pt(0, -12) || got[0].ScrollStep || !got[0].ScrollPixels {
		t.Fatalf("a trackpad's 12 pixels reached the handler as %+v", got)
	}
}

// scrollCatcher fills the window and keeps the scrolls that reach it.
type scrollCatcher struct{ got *[]PointerEvent }

func (c *scrollCatcher) Layout(cs Constraints, _ Env) Size { return cs.Max() }
func (c *scrollCatcher) Paint(dst *Canvas, r Rect)         { dst.HitPointer(r, c) }
func (c *scrollCatcher) HandlePointer(ev PointerEvent) bool {
	if ev.Kind == PointerScroll {
		*c.got = append(*c.got, ev)
	}
	return true
}

// A scroll draws its thumb at the thickness and margin of the ScrollStyle
// in its Env, and at 3 logical pixels, 2 from the edge, without them.
func TestScrollbarThumbTakesItsThicknessAndMarginFromTheStyle(t *testing.T) {
	t.Parallel()
	def := Env{}.ScrollStyle()
	if def.Thickness != 3 || def.HoverThickness != 4.5 || def.Margin != 2 {
		t.Fatalf("default bar is %v thick, %v under the pointer, %v from the edge; want 3, 4.5, 2", def.Thickness, def.HoverThickness, def.Margin)
	}
	style := ScrollStyle{Color: color.Black, Thickness: 7, HoverThickness: 7, Margin: 1}
	if got := (Env{}).With(ScrollStyleKey, style).ScrollStyle(); got.Thickness != 7 || got.HoverThickness != 7 || got.Margin != 1 {
		t.Fatalf("a style's sizes were replaced: %+v", got)
	}
	r := Rct(Pt(0, 0), Sz(100, 100))
	visual, hit := def.thumbRects(r, false, 10, 30, 0)
	if visual != Rct(Pt(95, 10), Sz(3, 30)) || hit != Rct(Pt(90, 10), Sz(10, 30)) {
		t.Fatalf("the default thumb is drawn at %v and grabbed at %v", visual, hit)
	}
	if visual, _ = def.thumbRects(r, false, 10, 30, 1); visual.Size.W != 4.5 {
		t.Fatalf("the default thumb is %v wide under the pointer, want 4.5", visual.Size.W)
	}
	themed := (Env{}).With(ScrollStyleKey, style).ScrollStyle()
	for _, amount := range []float64{0, 1} {
		if visual, _ = themed.thumbRects(r, false, 10, 30, amount); visual != Rct(Pt(92, 10), Sz(7, 30)) {
			t.Fatalf("a 7px thumb 1px from the edge is drawn at %v, hovered %v", visual, amount)
		}
	}
	if visual, hit = themed.thumbRects(r, true, 10, 30, 0); visual != Rct(Pt(10, 92), Sz(30, 7)) || hit != Rct(Pt(10, 90), Sz(30, 10)) {
		t.Fatalf("a horizontal 7px thumb is drawn at %v and grabbed at %v", visual, hit)
	}
}

// A scroll and the scroll inside it, along the other axis, are clipped to
// one viewport and so hold one Rect. Each is the widget it was last frame:
// the outer must not adopt the inner's offset as a rebuilt scroll would.
func TestNestedScrollsOfOneViewportKeepTheirOwnOffsets(t *testing.T) {
	var x, y ScrollController
	inner := Scroll(Box().Size(600, 900)).Horizontal().Controller(&x)
	outer := Scroll(inner).Controller(&y)
	p := NewWindowProbe(func() Widget { return outer }, Sz(300, 300))
	defer p.Close()
	p.Run(time.Second, 10)
	for range 5 {
		p.Trackpad(Pt(150, 150), Pt(0, -10))
		p.Step(16 * time.Millisecond)
	}
	if x.Offset() != 0 || y.Offset() != 50 {
		t.Fatalf("five swipes down scrolled to x %v, y %v, want 0 and 50", x.Offset(), y.Offset())
	}
}

// Content a rounding error taller than its viewport, as a sum of
// fractional heights given back less one of them comes out, has nothing to
// scroll: no thumb as long as the track, and the wheel is left to what is
// around. Content a pixel taller scrolls as ever.
func TestScrollTakesARoundingErrorOfOverflowForNone(t *testing.T) {
	t.Parallel()
	header, body := 31.516000000000002, 62.266
	total := header + body
	if total-header >= body {
		t.Fatal("the sum no longer loses a rounding error; pick another")
	}
	var got Point
	s := Scroll(Box().Size(100, body))
	w := Pointer(s).OnScroll(func(d Point) { got = d })
	var in inputState
	paintFrame(&in, w, Sz(100, total-header))
	if s.maxOffset() != 0 || s.thumbHit != (Rect{}) {
		t.Fatalf("content %v over its viewport scrolls %v with a thumb at %v", s.childSize.H-s.viewport.H, s.maxOffset(), s.thumbHit)
	}
	in.dispatch(frameInput{pos: Pt(5, 5), wheel: Pt(0, -1)})
	if got != (Point{X: 0, Y: -1}) {
		t.Fatalf("the wheel did not fall through: got %v", got)
	}
	tall := Scroll(Box().Size(100, 101))
	paintFrame(&in, tall, Sz(100, 100))
	if tall.maxOffset() != 1 || tall.thumbHit == (Rect{}) {
		t.Fatalf("content a pixel over scrolls %v with a thumb at %v", tall.maxOffset(), tall.thumbHit)
	}
}
