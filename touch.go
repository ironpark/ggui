package ggui

import (
	"slices"

	"github.com/ironpark/ggfx"
)

// touchInput maps a primary finger to the single pointer used by widgets.
// Once it lifts, wait for all fingers to lift before starting another press.
type touchInput struct {
	id              ggfx.TouchID
	active, waiting bool
	pos             Point
}

func (t *touchInput) apply(f *frameInput, ids []ggfx.TouchID, position func(ggfx.TouchID) Point) {
	if !t.active && !t.waiting && len(ids) == 0 {
		return
	}
	// Ignore mouse edges while handling touch, including the release frame.
	f.down, f.up = nil, nil
	if t.waiting {
		f.pos = t.pos
		t.waiting = len(ids) != 0
		return
	}
	f.touch = true
	if !t.active {
		t.id, t.active = ids[0], true
		f.down = []MouseButton{MouseButtonLeft}
	}
	if slices.Contains(ids, t.id) {
		t.pos = position(t.id)
	} else {
		// ggfx no longer exposes the position of a released touch.
		// Keep its last position instead of releasing at the mouse cursor.
		f.up = []MouseButton{MouseButtonLeft}
		t.active, t.waiting = false, len(ids) != 0
	}
	f.pos = t.pos
}

// panTouch lets a scroll region take over a touch after a small movement.
// Hit testing stays at the gesture origin; a captured scroller continues
// receiving movement even when the finger leaves its viewport.
func (in *inputState) panTouch(f *frameInput) {
	now := clock()
	if len(f.down) != 0 || f.wheel != (Point{}) || len(f.keys) != 0 {
		in.touchMotion.stop()
	}
	in.stepTouchMomentum(now)
	if !f.touch {
		return
	}
	if len(f.down) != 0 {
		in.touchStart, in.touchLast = f.pos, f.pos
		in.touchScroll, in.touchPanning = nil, false
		in.touchMotion = touchMotion{last: now, pos: f.pos}
		return
	}
	// Assert before re-resolving: the scan only pays off for the rare control
	// that captures drags, and it runs on every frame of a held finger.
	if in.pressed != nil {
		if _, ok := in.pressed.pointer.(TouchDragCapturer); ok {
			if cur := in.findPointer(in.pressed); cur != nil {
				if capturer, ok := cur.pointer.(TouchDragCapturer); ok && capturer.CaptureTouchDrag() {
					return
				}
			}
		}
	}
	in.touchMotion.sample(now, f.pos)
	delta := Pt(f.pos.X-in.touchLast.X, f.pos.Y-in.touchLast.Y)
	if !in.touchPanning {
		delta = Pt(f.pos.X-in.touchStart.X, f.pos.Y-in.touchStart.Y)
		if delta.X*delta.X+delta.Y*delta.Y < 64 {
			return
		}
	}
	// Lock to the dominant axis so horizontal controls in vertical lists
	// can still be dragged without moving the list.
	if !in.touchPanning {
		if delta.X*delta.X > delta.Y*delta.Y {
			delta.Y = 0
		} else {
			delta.X = 0
		}
	}
	ev := PointerEvent{Kind: PointerScroll, Pos: in.touchStart, Scroll: delta, ScrollPixels: true}
	if in.touchPanning {
		if cur := in.findPointer(in.touchScroll); cur != nil && delta != (Point{}) {
			cur.pointer.HandlePointer(ev)
		}
	} else if delta != (Point{}) {
		if target := in.send(ev); target != nil {
			in.touchScroll, in.touchPanning = keep(target), true
			if in.pressed != nil {
				if cur := in.findPointer(in.pressed); cur != nil {
					cur.pointer.HandlePointer(PointerEvent{Kind: PointerUp, Pos: f.pos, Button: in.pressedBtn})
				}
				in.pressed = nil
			}
		}
	}
	in.touchLast = f.pos
	if in.touchPanning {
		// The original target has already been released. Never generate a tap.
		ended := len(f.up) != 0
		f.down, f.up = nil, nil
		if ended {
			in.touchMotion.boostRelease()
			in.touchMotion.target = in.touchScroll
			in.touchMotion.origin = in.touchStart
			in.touchMotion.last = now
			in.touchPanning, in.touchScroll = false, nil
		}
	}
}
