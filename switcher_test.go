package ggui

import (
	"testing"
	"time"
)

func TestSwitcherCrossFadesAndDisposesTheOldSubtree(t *testing.T) {
	t.Parallel()
	key := State(0)
	var rects [3]Rect
	taps := [3]int{}
	disposed := [3]bool{}
	sw := Switcher(key, func(k int) Widget {
		OnCleanup(func() { disposed[k] = true })
		return Tap(probe(50, 20, &rects[k]), func() { taps[k]++ })
	}).Slide(0, 10).Easing(EaseLinear).Duration(100 * time.Millisecond)
	p := NewProbe(Column(sw), Sz(100, 100))
	defer p.Close()
	p.Click(Pt(10, 10))
	if rects[0].Origin.Y != 0 || taps[0] != 1 {
		t.Fatalf("first subtree at %v with %d taps: it must appear in place at once", rects[0], taps[0])
	}

	key.Set(1)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	rects = [3]Rect{}
	p.Click(Pt(10, 10))
	if rects[0] == (Rect{}) || rects[1] == (Rect{}) {
		t.Fatalf("both subtrees must paint while switching: %v", rects)
	}
	if rects[1].Origin.Y != 5 || rects[0].Origin.Y != 5 {
		t.Fatalf("half way: new at y=%v, old at y=%v; want both 5", rects[1].Origin.Y, rects[0].Origin.Y)
	}
	if taps[0] != 1 || taps[1] != 1 {
		t.Fatalf("taps %v: the leaving subtree must be inert and the new one live", taps)
	}
	if disposed[0] {
		t.Fatal("the old subtree was disposed before it left")
	}

	p.Advance(200 * time.Millisecond)
	p.Advance(0)
	rects = [3]Rect{}
	p.Frame()
	if !disposed[0] || rects[0] != (Rect{}) || rects[1].Origin.Y != 0 {
		t.Fatalf("after the switch: old disposed %v at %v, new at %v", disposed[0], rects[0], rects[1])
	}
}

func TestSwitcherUnderReducedMotionSwitchesAtOnce(t *testing.T) {
	t.Parallel()
	key := State(0)
	var rects [2]Rect
	disposed := [2]bool{}
	sw := Switcher(key, func(k int) Widget {
		OnCleanup(func() { disposed[k] = true })
		return probe(50, 20, &rects[k])
	})
	p := NewProbe(Provide(ReducedMotionKey, true, Column(sw)), Sz(100, 100))
	defer p.Close()
	p.Frame()
	key.Set(1)
	p.Frame()
	rects = [2]Rect{}
	p.Frame()
	if !disposed[0] || rects[0] != (Rect{}) || rects[1] == (Rect{}) {
		t.Fatalf("reduced motion: old disposed %v painted %v, new painted %v", disposed[0], rects[0], rects[1])
	}
}

// Options are value setters: changed after mount, they apply from the
// next switch, and a switch under way keeps the duration it started with.
func TestSwitcherOptionsApplyFromTheNextSwitch(t *testing.T) {
	t.Parallel()
	key := State(0)
	var rects [3]Rect
	sw := Switcher(key, func(k int) Widget { return probe(50, 20, &rects[k]) }).
		Slide(0, 10).Easing(EaseLinear).Duration(100 * time.Millisecond)
	p := NewProbe(Column(sw), Sz(100, 100))
	defer p.Close()
	p.Frame()
	sw.Duration(200*time.Millisecond).Slide(0, 20)
	key.Set(1)
	p.Advance(0)
	p.Advance(0)
	p.Advance(100 * time.Millisecond)
	if rects[1].Origin.Y != 10 {
		t.Fatalf("half way through a 200ms slide of 20 the new subtree is at y=%v, want 10", rects[1].Origin.Y)
	}
	sw.Duration(time.Second)
	p.Advance(50 * time.Millisecond)
	if rects[1].Origin.Y != 5 {
		t.Fatalf("a duration changed mid-switch moved it to y=%v, want 5", rects[1].Origin.Y)
	}
}
