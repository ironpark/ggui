package ggui

import (
	"testing"
	"time"
)

func TestTransitionSlidesInOnceAndNotOnRebuild(t *testing.T) {
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
