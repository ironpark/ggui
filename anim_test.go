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
	if got := tw.Peek(); got != 25 {
		t.Fatalf("value at 250ms = %v, want 25", got)
	}
	anims.step(t0.Add(2 * time.Second))
	if got := tw.Peek(); got != 100 || len(anims.list) != 0 {
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
	if got := tw.Peek(); got != 25 {
		t.Fatalf("value = %v, want 25 (halfway back from 50)", got)
	}
	tw.Jump(7)
	anims.step(t0.Add(5 * time.Second))
	if tw.Peek() != 7 {
		t.Fatalf("Jump did not stick: %v", tw.Peek())
	}
}

func TestTweenDrivesEffects(t *testing.T) {
	anims.list = nil
	tw := Tween(0, 100*time.Millisecond)
	runs := 0
	Effect(func() { tw.Get(); runs++ })
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
		if sp.Peek() > 1 {
			overshot = true
		}
	}
	if len(anims.list) != 0 || sp.Peek() != 1 {
		t.Fatalf("spring did not settle: value %v, %d running", sp.Peek(), len(anims.list))
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
