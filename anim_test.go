package ggui

import (
	"testing"
	"time"
)

func TestTweenMovesOverItsDuration(t *testing.T) {
	anims.list = nil
	tw := Tween(0.0, time.Second).Easing(EaseLinear)
	tw.Set(100)
	t0 := time.Unix(0, 0)
	anims.step(t0)
	anims.step(t0.Add(250 * time.Millisecond))
	if got := Untrack(tw.Get); got != 25 {
		t.Fatalf("value at 250ms = %v, want 25", got)
	}
	anims.step(t0.Add(2 * time.Second))
	if got := Untrack(tw.Get); got != 100 || len(anims.list) != 0 {
		t.Fatalf("value at the end = %v with %d animations still registered", got, len(anims.list))
	}
}

func TestTweenRetargetsFromCurrentValue(t *testing.T) {
	anims.list = nil
	tw := Tween(0.0, time.Second).Easing(EaseLinear)
	tw.Set(100)
	t0 := time.Unix(0, 0)
	anims.step(t0)
	anims.step(t0.Add(500 * time.Millisecond))
	tw.Set(0)
	anims.step(t0.Add(500 * time.Millisecond))
	anims.step(t0.Add(1000 * time.Millisecond))
	if got := Untrack(tw.Get); got != 25 {
		t.Fatalf("value = %v, want 25 (halfway back from 50)", got)
	}
	tw.Jump(7)
	anims.step(t0.Add(5 * time.Second))
	if Untrack(tw.Get) != 7 {
		t.Fatalf("Jump did not stick: %v", Untrack(tw.Get))
	}
}

func TestTweenDrivesEffects(t *testing.T) {
	anims.list = nil
	tw := Tween(0, 100*time.Millisecond)
	runs := 0
	observe(func() { tw.Get(); runs++ })
	tw.Set(10)
	t0 := time.Unix(0, 0)
	anims.step(t0)
	anims.step(t0.Add(50 * time.Millisecond))
	effects.flush()
	if runs != 2 {
		t.Fatalf("effect ran %d times, want 2 after the value moved", runs)
	}
}

func TestSpringSettlesAtTarget(t *testing.T) {
	anims.list = nil
	sp := Spring(0.0)
	sp.Set(1)
	now := time.Unix(0, 0)
	overshot := false
	for i := 0; i < 600 && len(anims.list) > 0; i++ {
		now = now.Add(time.Second / 60)
		anims.step(now)
		if Untrack(sp.Get) > 1 {
			overshot = true
		}
	}
	if len(anims.list) != 0 || Untrack(sp.Get) != 1 {
		t.Fatalf("spring did not settle: value %v, %d running", Untrack(sp.Get), len(anims.list))
	}
	if !overshot {
		t.Fatal("default spring should overshoot a little")
	}
}

func TestEasingEndpoints(t *testing.T) {
	for _, e := range []Easing{EaseLinear, EaseIn, EaseOut, EaseInOut} {
		if e(0) != 0 || e(1) != 1 {
			t.Fatalf("easing does not map 0->0 and 1->1: %v %v", e(0), e(1))
		}
	}
	if EaseInOut(0.5) != 0.5 {
		t.Fatalf("EaseInOut(0.5) = %v", EaseInOut(0.5))
	}
}

func TestMotionRetargetsMidway(t *testing.T) {
	var m Motion
	t0 := time.Unix(0, 0)
	m.MoveTo(0, t0, 100*time.Millisecond) // first target: no animation
	if m.Value(t0) != 0 {
		t.Fatal("initial value")
	}
	m.MoveTo(1, t0, 100*time.Millisecond)
	mid := m.Value(t0.Add(50 * time.Millisecond))
	if mid <= 0 || mid >= 1 {
		t.Fatalf("mid = %v", mid)
	}
	m.MoveTo(0, t0.Add(50*time.Millisecond), 100*time.Millisecond)
	if got := m.Value(t0.Add(50 * time.Millisecond)); got != mid {
		t.Fatalf("retarget jumped from %v to %v", mid, got)
	}
	if m.Value(t0.Add(time.Second)) != 0 {
		t.Fatal("did not arrive")
	}
}

func TestAnimationCleanupOnlyStopsOwnedValues(t *testing.T) {
	outside := Tween(0.0, time.Second).Easing(EaseLinear)
	defer outside.Jump(0)
	var tw *Tweened[float64]
	var sp *Sprung[float64]
	p := NewProbe(Box(), Sz(10, 10)).Setup(func() {
		tw = Tween(0.0, time.Second).Easing(EaseLinear)
		sp = Spring(0.0)
	})
	defer p.Close()
	p.Frame()
	tw.Set(100)
	sp.Set(100)
	outside.Set(100)
	p.Advance(0)
	p.Advance(100 * time.Millisecond)
	tv, sv := Untrack(tw.Get), Untrack(sp.Get)
	p.Close()
	for _, a := range anims.list {
		if a == tw || a == sp {
			t.Fatal("closed probe retains its animation")
		}
	}
	// Stale handles cannot restart animations after the owner's cleanup.
	tw.Set(200)
	sp.Set(200)
	tw.Jump(300)
	sp.Jump(300)
	anims.step(outside.start.Add(500 * time.Millisecond))
	if Untrack(tw.Get) != tv || Untrack(sp.Get) != sv {
		t.Fatal("a disposed owner's animation changed value")
	}
	if Untrack(outside.Get) != 50 {
		t.Fatalf("unowned animation was stopped: %v", Untrack(outside.Get))
	}
}

func TestAnimationCreatedDuringStepIsNotDropped(t *testing.T) {
	second := Tween(0.0, time.Second).Easing(EaseLinear)
	defer second.Jump(0)
	first := Tween(0.0, time.Second).Easing(func(p float64) float64 {
		if !second.running {
			second.Set(10)
		}
		return p
	})
	defer first.Jump(0)
	t0 := time.Unix(100, 0)
	first.Set(10)
	anims.step(t0)
	anims.step(t0.Add(100 * time.Millisecond))
	anims.step(t0.Add(200 * time.Millisecond))
	if Untrack(second.Get) <= 0 {
		t.Fatal("animation registered by an easing callback was lost")
	}
}

func TestAnimationCanLoseOwnerDuringStep(t *testing.T) {
	var tw *Tweened[float64]
	var dispose func()
	dispose = Root(func() {
		tw = Tween(0.0, time.Second).Easing(func(p float64) float64 {
			if p > 0 {
				dispose()
			}
			return p
		})
	})
	defer dispose()
	tw.Set(100)
	t0 := time.Unix(100, 0)
	anims.step(t0)
	anims.step(t0.Add(500 * time.Millisecond))
	if Untrack(tw.Get) != 0 || tw.running {
		t.Fatal("step updated an animation after its owner was disposed")
	}
	for _, s := range anims.list {
		if s == tw {
			t.Fatal("step restored the disposed animation's registration")
		}
	}
}
