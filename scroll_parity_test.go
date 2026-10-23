package ggui

import (
	"math"
	"testing"
	"time"
)

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
