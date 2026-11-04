package ggui

import (
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui/internal/kinetics"
	"github.com/ironpark/ggwd"
)

func TestTouchTapAndCapturedDrag(t *testing.T) {
	t.Parallel()
	var touch touchInput
	var in inputState
	var taps, ups, drags int
	var released Point
	w := Pointer(Box().Size(50, 50)).OnTap(func() { taps++ }).
		OnDrag(func(PointerEvent) { drags++ }).
		OnUp(func(ev PointerEvent) { ups++; released = ev.Pos })
	paintFrame(&in, w, Sz(50, 50))
	step := func(ids []ggwd.TouchID, pos Point) {
		f := frameInput{pos: Pt(999, 999)}
		touch.apply(&f, ids, func(ggwd.TouchID) Point { return pos })
		in.dispatch(f)
	}
	step([]ggwd.TouchID{0}, Pt(10, 10))
	step(nil, Point{})
	if taps != 1 || ups != 1 || released != Pt(10, 10) {
		t.Fatalf("tap/release = %d/%d at %v", taps, ups, released)
	}
	step([]ggwd.TouchID{1}, Pt(10, 10))
	step([]ggwd.TouchID{1}, Pt(80, 80))
	step(nil, Point{})
	if taps != 1 || ups != 2 || drags != 1 || released != Pt(80, 80) || in.pressed != nil {
		t.Fatalf("captured drag: taps=%d ups=%d drags=%d release=%v", taps, ups, drags, released)
	}
}

func TestTouchKeepsPrimaryAndWaitsForRemainingFingers(t *testing.T) {
	t.Parallel()
	var touch touchInput
	position := func(id ggwd.TouchID) Point { return Pt(float64(id), 10) }
	step := func(ids ...ggwd.TouchID) frameInput {
		f := frameInput{down: []MouseButton{MouseButtonRight}}
		touch.apply(&f, ids, position)
		return f
	}
	step(1)
	if f := step(2, 1); f.pos != position(1) || len(f.down) != 0 {
		t.Fatalf("second finger changed primary: %+v", f)
	}
	if f := step(2); len(f.up) != 1 || f.pos != position(1) {
		t.Fatalf("missing primary release: %+v", f)
	}
	if f := step(2); len(f.down) != 0 || len(f.up) != 0 {
		t.Fatalf("remaining finger started a new press: %+v", f)
	}
	step()
	if f := step(3); len(f.down) != 1 || f.down[0] != MouseButtonLeft || f.pos != position(3) {
		t.Fatalf("new gesture failed: %+v", f)
	}
}

func TestTouchPanScrollsAndCancelsTap(t *testing.T) {
	t.Parallel()
	var in inputState
	var touch touchInput
	taps, ups := 0, 0
	child := Pointer(Box().Size(100, 400)).OnTap(func() { taps++ }).OnUp(func(PointerEvent) { ups++ })
	s := Scroll(child).Speed(99)
	step := func(held bool, p Point) {
		paintFrame(&in, s, Sz(100, 100))
		var ids []ggwd.TouchID
		if held {
			ids = []ggwd.TouchID{1}
		}
		f := frameInput{}
		touch.apply(&f, ids, func(ggwd.TouchID) Point { return p })
		in.dispatch(f)
	}
	step(true, Pt(50, 80))
	step(true, Pt(50, 77))
	if s.position() != 0 {
		t.Fatal("small tap jitter scrolled")
	}
	step(false, Point{})
	if taps != 1 {
		t.Fatal("tap with jitter was lost")
	}
	step(true, Pt(50, 80))
	step(true, Pt(50, 60))
	if s.position() != 20 {
		t.Fatalf("offset = %v, want 20 logical pixels", s.position())
	}
	step(true, Pt(50, -10))
	if s.position() != 90 {
		t.Fatalf("captured offset = %v, want 90", s.position())
	}
	step(false, Point{})
	if taps != 1 || ups != 2 || in.pressed != nil || in.touchPanning {
		t.Fatalf("pan did not cancel press: taps=%d ups=%d", taps, ups)
	}
}

func TestHorizontalTouchDragDoesNotScrollVerticalList(t *testing.T) {
	t.Parallel()
	var in inputState
	drags := 0
	s := Scroll(Pointer(Box().Size(100, 400)).OnDrag(func(PointerEvent) { drags++ }))
	paintFrame(&in, s, Sz(100, 100))
	in.dispatch(frameInput{touch: true, pos: Pt(10, 50), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{touch: true, pos: Pt(40, 51)})
	if s.position() != 0 || drags != 1 || in.touchPanning {
		t.Fatalf("horizontal drag stolen: offset=%v drags=%d", s.position(), drags)
	}
}

type touchDragControl struct{ *PointerWidget }

func (*touchDragControl) CaptureTouchDrag() bool { return true }
func (w *touchDragControl) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, w)
	dst.Paint(w.child, r)
}

func TestTouchCapturedControlDoesNotPanParent(t *testing.T) {
	t.Parallel()
	var in inputState
	drags, ups := 0, 0
	control := &touchDragControl{Pointer(Box().Size(100, 400)).
		OnDrag(func(PointerEvent) { drags++ }).
		OnUp(func(PointerEvent) { ups++ })}
	s := Scroll(control)
	paintFrame(&in, s, Sz(100, 100))
	in.dispatch(frameInput{touch: true, pos: Pt(50, 80), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{touch: true, pos: Pt(50, 40)})
	in.dispatch(frameInput{touch: true, pos: Pt(50, -20)})
	in.dispatch(frameInput{touch: true, pos: Pt(50, -20), up: []MouseButton{MouseButtonLeft}})
	if drags != 2 || ups != 1 || in.pressed != nil {
		t.Fatalf("lost captured drag: drags=%d ups=%d pressed=%v", drags, ups, in.pressed)
	}
	if s.position() != 0 || in.touchPanning || in.touchMotion.target != nil {
		t.Fatalf("captured control started scrolling: offset=%v", s.position())
	}
}

func TestTouchFlingSpeedHoldAndInterruption(t *testing.T) {
	t.Parallel()
	run := func(speed float64, hold bool, interrupt bool) float64 {
		var in inputState
		now := time.Unix(100, 0)
		restore := SetClock(func() time.Time { return now })
		defer restore()
		s := Scroll(Box().Size(100, 10000))
		paintFrame(&in, s, Sz(100, 100))
		p := Pt(50, 80)
		in.dispatch(frameInput{touch: true, pos: p, down: []MouseButton{MouseButtonLeft}})
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
		in.dispatch(frameInput{touch: true, pos: p, up: []MouseButton{MouseButtonLeft}})
		before := s.position()
		if interrupt {
			now = now.Add(16 * time.Millisecond)
			in.dispatch(frameInput{touch: true, pos: Pt(50, 50), down: []MouseButton{MouseButtonLeft}})
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
	t.Parallel()
	run := func(step time.Duration, height float64) (float64, bool) {
		var in inputState
		s := Scroll(Box().Size(100, height))
		paintFrame(&in, s, Sz(100, 100))
		target := in.topmost(func(r *hitRegion) bool { return r.pointer == s })
		now := time.Unix(100, 0)
		in.touchMotion = touchMotion{target: keep(target), Tracker: kinetics.Tracker{Last: now, Velocity: Pt(0, -1000)}}
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
	t.Parallel()
	for _, missing := range []bool{false, true} {
		var in inputState
		s := Scroll(Box().Size(100, 1000))
		paintFrame(&in, s, Sz(100, 100))
		target := in.topmost(func(r *hitRegion) bool { return r.pointer == s })
		now := time.Unix(100, 0)
		in.touchMotion = touchMotion{target: keep(target), Tracker: kinetics.Tracker{Last: now, Velocity: Pt(0, -1000)}}
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
