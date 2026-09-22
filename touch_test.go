package ggui

import (
	"testing"

	"github.com/ironpark/ggfx"
)

func TestTouchTapAndCapturedDrag(t *testing.T) {
	var touch touchInput
	var in inputState
	var taps, ups, drags int
	var released Point
	w := Pointer(Box().Size(50, 50)).OnTap(func() { taps++ }).
		OnDrag(func(PointerEvent) { drags++ }).
		OnUp(func(ev PointerEvent) { ups++; released = ev.Pos })
	paintFrame(&in, w, Sz(50, 50))
	step := func(ids []ggfx.TouchID, pos Point) {
		f := frameInput{pos: Pt(999, 999)}
		touch.apply(&f, ids, func(ggfx.TouchID) Point { return pos })
		in.dispatch(f)
	}
	step([]ggfx.TouchID{0}, Pt(10, 10))
	step(nil, Point{})
	if taps != 1 || ups != 1 || released != Pt(10, 10) {
		t.Fatalf("tap/release = %d/%d at %v", taps, ups, released)
	}
	step([]ggfx.TouchID{1}, Pt(10, 10))
	step([]ggfx.TouchID{1}, Pt(80, 80))
	step(nil, Point{})
	if taps != 1 || ups != 2 || drags != 1 || released != Pt(80, 80) || in.pressed != nil {
		t.Fatalf("captured drag: taps=%d ups=%d drags=%d release=%v", taps, ups, drags, released)
	}
}

func TestTouchKeepsPrimaryAndWaitsForRemainingFingers(t *testing.T) {
	var touch touchInput
	position := func(id ggfx.TouchID) Point { return Pt(float64(id), 10) }
	step := func(ids ...ggfx.TouchID) frameInput {
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
	var in inputState
	var touch touchInput
	taps, ups := 0, 0
	child := Pointer(Box().Size(100, 400)).OnTap(func() { taps++ }).OnUp(func(PointerEvent) { ups++ })
	s := Scroll(child).Speed(99)
	step := func(held bool, p Point) {
		paintFrame(&in, s, Sz(100, 100))
		var ids []ggfx.TouchID
		if held {
			ids = []ggfx.TouchID{1}
		}
		f := frameInput{}
		touch.apply(&f, ids, func(ggfx.TouchID) Point { return p })
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
