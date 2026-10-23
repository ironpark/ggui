package ggui

import (
	"math"
	"testing"
	"time"
)

// stepAt steps the goroutine's animations at t0 plus each offset.
func stepAt(t0 time.Time, offsets ...time.Duration) {
	for _, d := range offsets {
		activeWorld().anims.step(t0.Add(d))
	}
}

func TestTimelinePlaysAndReverses(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	tl := Timeline(time.Second)
	done := 0
	tl.OnDone(func() { done++ })
	tl.Play()
	t0 := time.Unix(0, 0)
	stepAt(t0, 0, 250*time.Millisecond)
	if got := Untrack(tl.Get); got != 0.25 || !tl.Running() {
		t.Fatalf("progress at 250ms = %v", got)
	}
	stepAt(t0, 2*time.Second)
	if Untrack(tl.Get) != 1 || tl.Running() || done != 1 {
		t.Fatalf("after the end: %v running %v done %d", Untrack(tl.Get), tl.Running(), done)
	}
	tl.Reverse()
	t1 := t0.Add(5 * time.Second)
	stepAt(t1, 0, 500*time.Millisecond)
	if Untrack(tl.Get) != 0.5 {
		t.Fatalf("reversing halfway = %v", Untrack(tl.Get))
	}
	tl.Stop()
	stepAt(t1, time.Second)
	if Untrack(tl.Get) != 0.5 || done != 1 {
		t.Fatal("Stop did not freeze the timeline, or called OnDone")
	}
	tl.Seek(2)
	if Untrack(tl.Get) != 1 {
		t.Fatal("Seek did not clamp")
	}
	tl.Play() // from the end: starts over
	t2 := t1.Add(5 * time.Second)
	stepAt(t2, 0, 100*time.Millisecond)
	if math.Abs(Untrack(tl.Get)-0.1) > 1e-9 {
		t.Fatalf("replay from the end = %v", Untrack(tl.Get))
	}
	tl.Stop()
}

func TestTimelineRepeatsAndAlternates(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	done := 0
	tl := Timeline(100 * time.Millisecond).Repeat(1).Alternate().OnDone(func() { done++ })
	tl.Play()
	t0 := time.Unix(0, 0)
	stepAt(t0, 0, 150*time.Millisecond)
	if math.Abs(Untrack(tl.Get)-0.5) > 1e-9 || done != 1 {
		t.Fatalf("half way back = %v, done %d", Untrack(tl.Get), done)
	}
	stepAt(t0, 300*time.Millisecond)
	if Untrack(tl.Get) != 0 || tl.Running() || done != 2 {
		t.Fatalf("after one repeat: %v running %v done %d", Untrack(tl.Get), tl.Running(), done)
	}

	loop := Timeline(100 * time.Millisecond).Repeat(-1)
	loop.Play()
	stepAt(t0, 0, 1030*time.Millisecond)
	if math.Abs(Untrack(loop.Get)-0.3) > 1e-9 || !loop.Running() {
		t.Fatalf("looping timeline at 1030ms = %v", Untrack(loop.Get))
	}
	loop.Stop()
}

func TestTimelineDerivedValues(t *testing.T) {
	t.Parallel()
	tl := Timeline(time.Second)
	tl.Seek(0.5)
	if got := Untrack(tl.Interval(0.5, 1, nil).Get); got != 0 {
		t.Fatalf("Interval before its start = %v", got)
	}
	if got := Untrack(tl.Interval(0, 1, EaseIn).Get); got != 0.125 {
		t.Fatalf("eased progress = %v", got)
	}
	// Three items each taking half the timeline start at 0, 0.25 and 0.5.
	want := []float64{1, 0.5, 0}
	for i := range 3 {
		if got := Untrack(tl.Stagger(i, 3, 0.5, nil).Get); math.Abs(got-want[i]) > 1e-9 {
			t.Fatalf("Stagger item %d = %v, want %v", i, got, want[i])
		}
	}
	if got := Untrack(Interpolate(tl, Pt(0, 0), Pt(10, 20), LerpPoint).Get); got != Pt(5, 10) {
		t.Fatalf("Interpolate = %v", got)
	}
}

func TestTimelineDrivesABoundBoxAndHonorsReducedMotion(t *testing.T) {
	t.Parallel()
	var tl *TimelineValue
	var got Rect
	box := Box(probe(0, 0, &got))
	p := NewProbe(Column(box), Sz(300, 100)).Setup(func() {
		tl = Timeline(100 * time.Millisecond)
		box.BindWidth(Interpolate(tl, 100.0, 200.0, LerpFloat))
	})
	defer p.Close()
	p.Frame()
	if got.Size.W != 100 {
		t.Fatalf("width before playing = %v", got.Size.W)
	}
	tl.Play()
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	if got.Size.W != 150 {
		t.Fatalf("width half way = %v", got.Size.W)
	}
	p.Advance(time.Second)
	if got.Size.W != 200 {
		t.Fatalf("width at the end = %v", got.Size.W)
	}
	done := false
	tl.OnDone(func() { done = true })
	SetEnv(UseEnv().With(ReducedMotionKey, true))
	tl.Reverse()
	p.Frame()
	if Untrack(tl.Get) != 0 || !done || got.Size.W != 100 {
		t.Fatalf("reduced motion: progress %v done %v width %v", Untrack(tl.Get), done, got.Size.W)
	}
}

func TestTimelineStopsWithItsOwner(t *testing.T) {
	t.Parallel()
	var tl *TimelineValue
	p := NewProbe(Box(), Sz(10, 10)).Setup(func() { tl = Timeline(time.Second) })
	p.Frame()
	tl.Play()
	p.Advance(0)
	p.Advance(100 * time.Millisecond)
	v := Untrack(tl.Get)
	p.Close()
	tl.Play()
	tl.Seek(1)
	if Untrack(tl.Get) != v || tl.Running() {
		t.Fatal("a disposed timeline moved")
	}
}

// A done callback that plays a repeating timeline again starts a fresh run
// from the next step instead of looping over the time before it.
func TestTimelineReplayedFromOnDoneStartsFresh(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	tl := Timeline(100 * time.Millisecond).Repeat(-1)
	replayed := false
	tl.OnDone(func() {
		if !replayed {
			replayed = true
			tl.Reverse()
		}
	})
	tl.Play()
	t0 := time.Unix(0, 0)
	stepAt(t0, 0, 150*time.Millisecond)
	if !replayed || !tl.Running() {
		t.Fatalf("replayed %v running %v", replayed, tl.Running())
	}
	stepAt(t0, 160*time.Millisecond, 185*time.Millisecond)
	if got := Untrack(tl.Get); math.Abs(got-0.75) > 1e-9 {
		t.Fatalf("25ms into the reversed run the progress is %v, want 0.75", got)
	}
	tl.Stop()
}

// A duration cut to zero mid-run lands the timeline on its end; an endless
// repeat must not spin forever on cycles that take no time.
func TestTimelineDurationCutToZeroMidRunLands(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	done := 0
	tl := Timeline(100 * time.Millisecond).Repeat(-1).OnDone(func() { done++ })
	tl.Play()
	t0 := time.Unix(0, 0)
	stepAt(t0, 0, 10*time.Millisecond)
	tl.Duration(0)
	stepAt(t0, 20*time.Millisecond)
	if Untrack(tl.Get) != 1 || tl.Running() || done != 1 {
		t.Fatalf("after Duration(0): progress %v running %v done %d", Untrack(tl.Get), tl.Running(), done)
	}
}
