package ggui

import (
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestTouchFlingSpeedHoldAndInterruption(t *testing.T) {
	run := func(speed float64, hold bool, interrupt bool) float64 {
		var in inputState
		now := time.Unix(100, 0)
		restore := SetClock(func() time.Time { return now })
		defer restore()
		s := Scroll(Box().Size(100, 10000))
		paintFrame(&in, s, Sz(100, 100))
		p := Pt(50, 80)
		in.dispatch(frameInput{touch: true, pos: p, down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
		for i := 0; i < 10; i++ {
			now = now.Add(16 * time.Millisecond)
			p.Y -= speed * .016
			in.dispatch(frameInput{touch: true, pos: p})
		}
		if hold {
			for i := 0; i < 30; i++ {
				now = now.Add(16 * time.Millisecond)
				in.dispatch(frameInput{touch: true, pos: p})
			}
		}
		now = now.Add(16 * time.Millisecond)
		in.dispatch(frameInput{touch: true, pos: p, up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
		before := s.position()
		if interrupt {
			now = now.Add(16 * time.Millisecond)
			in.dispatch(frameInput{touch: true, pos: Pt(50, 50), down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
		}
		for i := 0; i < 180; i++ {
			now = now.Add(16 * time.Millisecond)
			in.dispatch(frameInput{})
		}
		if in.touchMotion.target != nil {
			t.Fatal("fling never settled")
		}
		return s.position() - before
	}
	slow, fast := run(200, false, false), run(1000, false, false)
	t.Logf("released travel: slow=%.1fpx fast=%.1fpx", slow, fast)
	if slow <= 0 || fast < slow*4 || fast < 250 {
		t.Fatalf("fling distance does not follow speed: slow=%v fast=%v", slow, fast)
	}
	if d := run(1000, true, false); d != 0 {
		t.Fatalf("holding before release still flings: %v", d)
	}
	if d := run(1000, false, true); d != 0 {
		t.Fatalf("new touch did not stop inertia: %v", d)
	}
}

func TestTouchMomentumFrameRateAndBounds(t *testing.T) {
	run := func(step time.Duration, height float64) (float64, bool) {
		var in inputState
		s := Scroll(Box().Size(100, height))
		paintFrame(&in, s, Sz(100, 100))
		target := in.topmost(func(r *hitRegion) bool { return r.pointer == s })
		now := time.Unix(100, 0)
		in.touchMotion = touchMotion{target: keep(target), last: now, velocity: Pt(0, -1000)}
		for elapsed := time.Duration(0); elapsed < time.Second; elapsed += step {
			now = now.Add(step)
			in.stepTouchMomentum(now)
		}
		return s.position(), in.touchMotion.target != nil
	}
	a, _ := run(10*time.Millisecond, 10000)
	b, _ := run(20*time.Millisecond, 10000)
	if math.Abs(a-b) > 1e-8 {
		t.Fatalf("frame-dependent distance: %v vs %v", a, b)
	}
	if d, active := run(10*time.Millisecond, 150); d != 50 || active {
		t.Fatalf("did not stop at edge: %v active=%v", d, active)
	}
}

func TestTouchMomentumStopsOnPauseOrMissingTarget(t *testing.T) {
	for _, missing := range []bool{false, true} {
		var in inputState
		s := Scroll(Box().Size(100, 1000))
		paintFrame(&in, s, Sz(100, 100))
		target := in.topmost(func(r *hitRegion) bool { return r.pointer == s })
		now := time.Unix(100, 0)
		in.touchMotion = touchMotion{target: keep(target), last: now, velocity: Pt(0, -1000)}
		dt := time.Second
		if missing {
			in.regions = nil
			dt = 16 * time.Millisecond
		}
		in.stepTouchMomentum(now.Add(dt))
		if s.position() != 0 || in.touchMotion.target != nil {
			t.Fatal("stale fling continued")
		}
	}
}

func TestTouchVelocityReversesWithoutOldMomentum(t *testing.T) {
	now := time.Unix(100, 0)
	m := touchMotion{last: now, pos: Pt(0, 0), velocity: Pt(0, -1000)}
	m.sample(now.Add(16*time.Millisecond), Pt(0, 10))
	if m.velocity.Y <= 0 {
		t.Fatalf("velocity kept old direction: %v", m.velocity)
	}
}
