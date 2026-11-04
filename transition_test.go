package ggui

import (
	"image/color"
	"math"
	"testing"
	"time"
)

func TestTransitionSlidesInOnceAndNotOnRebuild(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	defer SetClock(func() time.Time { return now })()

	var got Rect
	tick := State(0)
	tree := Reactive(func() Widget {
		tick.Get() // rebuilt on every tick
		return Column(Transition(probe(50, 20, &got)).Slide(0, 30).Easing(EaseLinear).Duration(100 * time.Millisecond))
	})
	p := NewProbe(tree, Sz(100, 100))
	p.Frame()
	if got.Origin.Y != 30 {
		t.Fatalf("first frame at y=%v, want 30 (fully slid out)", got.Origin.Y)
	}
	now = base.Add(50 * time.Millisecond)
	tick.Set(1)
	p.Frame()
	if got.Origin.Y != 15 {
		t.Fatalf("halfway after a rebuild at y=%v, want 15: the start time must be retained", got.Origin.Y)
	}
	now = base.Add(time.Second)
	p.Frame()
	if got.Origin.Y != 0 {
		t.Fatalf("finished at y=%v, want 0", got.Origin.Y)
	}
}

func TestPresenceKeepsChildInertWhileLeaving(t *testing.T) {
	t.Parallel()
	show := State(true)
	taps := 0
	var got Rect
	child := Tap(probe(50, 20, &got), func() { taps++ })
	tree := Column(Presence(show, Transition(child).Slide(0, 40).Easing(EaseLinear).Duration(100*time.Millisecond)))
	p := NewProbe(tree, Sz(100, 100))
	defer p.Close()
	p.Click(Pt(10, 10))
	if got.Origin.Y != 0 || taps != 1 {
		t.Fatalf("visible child at y=%v taps=%d; want in place and tappable", got.Origin.Y, taps)
	}
	show.Set(false)
	p.Advance(0) // the effect starts the tween
	p.Advance(0) // the first step takes its start time
	p.Advance(50 * time.Millisecond)
	got = Rect{}
	p.Click(Pt(10, 10))
	if got == (Rect{}) || got.Origin.Y < 15 || got.Origin.Y > 25 {
		t.Fatalf("leaving child painted at %+v, want halfway out", got)
	}
	if taps != 1 {
		t.Fatal("a leaving child took a tap")
	}
	p.Advance(time.Second)
	got = Rect{}
	p.Frame()
	if got != (Rect{}) {
		t.Fatalf("child still painted after leaving: %+v", got)
	}
	show.Set(true)
	p.Advance(time.Second)
	got = Rect{}
	p.Frame()
	if got == (Rect{}) || got.Origin.Y != 40 {
		t.Fatalf("re-entering child at %+v, want at the start of its slide", got)
	}
}

// Presence drives its Transition with linear progress, so the Transition's
// own easing is the only curve applied.
func TestPresenceAppliesTheEasingOnce(t *testing.T) {
	t.Parallel()
	show := State(true)
	var got Rect
	tree := Column(Presence(show, Transition(probe(50, 20, &got)).Slide(0, 40).Easing(EaseIn).Duration(100*time.Millisecond)))
	p := NewProbe(tree, Sz(100, 100))
	defer p.Close()
	p.Frame()
	show.Set(false)
	p.Advance(0)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	// Leaving, half way back: progress 0.5, offset (1 - EaseIn(0.5)) * 40.
	if want := (1 - EaseIn(0.5)) * 40; math.Abs(got.Origin.Y-want) > 1e-6 {
		t.Fatalf("half way out at y=%v, want %v", got.Origin.Y, want)
	}
}

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

func TestAnimatedBoxEasesBoundValues(t *testing.T) {
	t.Parallel()
	width := State(100.0)
	pad := State(Insets(0))
	fill := State[color.Color](color.Black)
	var got Rect
	box := AnimatedBox(probe(0, 0, &got)).BindWidth(width).Height(20).BindPadding(pad).
		BindFill(fill).Radius(4).Easing(EaseLinear).Duration(100 * time.Millisecond)
	p := NewProbe(Column(box), Sz(300, 100))
	defer p.Close()
	p.Frame()
	if got.Size.W != 100 {
		t.Fatalf("starting width %v", got.Size.W)
	}
	width.Set(200)
	pad.Set(Insets(10))
	fill.Set(color.White)
	p.Advance(0)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	// Half way: 150 wide less 5 padding on each side.
	if got.Size.W != 140 || got.Origin.X != 5 {
		t.Fatalf("half way the child is at %v, want x=5 and 140 wide", got)
	}
	if g := color.GrayModel.Convert(box.box.fill.shown).(color.Gray); g.Y < 120 || g.Y > 135 {
		t.Fatalf("half way fill gray %v", g.Y)
	}
	p.Advance(time.Second)
	p.Frame()
	if got.Size.W != 180 || got.Origin.X != 10 || box.box.fill.shown != color.Color(color.White) {
		t.Fatalf("at the end the child is at %v with fill %v", got, box.box.fill.shown)
	}

	box.Width(50) // a literal after mount animates too
	p.Advance(0)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	if got.Size.W != 125-20 {
		t.Fatalf("a literal width half way = %v, want 105", got.Size.W)
	}
}

func TestAnimatedBoxUnderReducedMotionJumps(t *testing.T) {
	t.Parallel()
	width := State(100.0)
	var got Rect
	box := AnimatedBox(probe(0, 0, &got)).BindWidth(width).Height(10)
	p := NewProbe(Provide(ReducedMotionKey, true, Column(box)), Sz(300, 100))
	defer p.Close()
	p.Frame()
	width.Set(200)
	p.Frame()
	if got.Size.W != 200 {
		t.Fatalf("reduced motion width %v, want 200 at once", got.Size.W)
	}
}

// Zero sizes to the child, so no number between leads to it: a box growing
// from zero starts at the child's size, and one shrinking to zero lands.
func TestAnimatedBoxEasesFromTheChildSize(t *testing.T) {
	t.Parallel()
	width := State(0.0)
	var got Rect
	box := AnimatedBox(probe(60, 10, &got)).BindWidth(width).Easing(EaseLinear).Duration(100 * time.Millisecond)
	p := NewProbe(Column(box), Sz(300, 100))
	defer p.Close()
	p.Frame()
	if got.Size.W != 60 {
		t.Fatalf("sized to the child: %v, want 60", got.Size.W)
	}
	width.Set(160)
	p.Advance(0)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	if got.Size.W != 110 {
		t.Fatalf("half way from the child's 60 to 160: %v, want 110", got.Size.W)
	}
	p.Advance(time.Second)
	p.Frame()
	width.Set(0)
	p.Frame()
	if got.Size.W != 60 {
		t.Fatalf("back to the child's size: %v, want 60 at once", got.Size.W)
	}
}

// A collapsible box's zero is zero: it eases closed and open again.
func TestAnimatedBoxCollapsibleEasesToZero(t *testing.T) {
	t.Parallel()
	width := State(100.0)
	var got Rect
	box := AnimatedBox(probe(60, 10, &got)).BindWidth(width).Collapsible().Easing(EaseLinear).Duration(100 * time.Millisecond)
	var shown Rect
	p := NewProbe(Column(Row(box, probe(10, 10, &shown))), Sz(300, 100))
	defer p.Close()
	p.Frame()
	if shown.Origin.X != 100 {
		t.Fatalf("at 100 wide the next child is at %v", shown.Origin.X)
	}
	width.Set(0)
	p.Advance(0)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	if shown.Origin.X != 50 {
		t.Fatalf("half way to zero the next child is at %v, want 50", shown.Origin.X)
	}
	p.Advance(time.Second)
	p.Frame()
	if shown.Origin.X > 1e-6 {
		t.Fatalf("closed, the next child is at %v, want 0", shown.Origin.X)
	}
}
