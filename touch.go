package ggui

import (
	"time"

	"github.com/ironpark/ggui/internal/kinetics"
	"slices"

	"github.com/ironpark/ggwd"
)

// touchInput maps a primary finger to the single pointer used by widgets.
// Once it lifts, wait for all fingers to lift before starting another press.
type touchInput struct {
	id              ggwd.TouchID
	active, waiting bool
	pos             Point
}

func (t *touchInput) apply(f *frameInput, ids []ggwd.TouchID, position func(ggwd.TouchID) Point) {
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
	now := activeWorld().frame.raw()
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
		in.touchMotion = touchMotion{Tracker: kinetics.Tracker{Last: now, Pos: f.pos}}
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
	in.touchMotion.Sample(now, f.pos, maxFrameStep)
	delta := f.pos.Sub(in.touchLast)
	if !in.touchPanning {
		delta = f.pos.Sub(in.touchStart)
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
			in.releasePress(f.pos, f.mods)
		}
	}
	in.touchLast = f.pos
	if in.touchPanning {
		// The original target has already been released. Never generate a tap.
		ended := len(f.up) != 0
		f.down, f.up = nil, nil
		if ended {
			in.touchMotion.Boost()
			in.touchMotion.target = in.touchScroll
			in.touchMotion.origin = in.touchStart
			in.touchMotion.Last = now
			in.touchPanning, in.touchScroll = false, nil
		}
	}
}

// touchMotion is a touch's velocity, tracked while the finger is down,
// and the fling that carries on from it after release: at target, from
// origin. Finger-down movement stays one-to-one; only released motion is
// decelerated.
type touchMotion struct {
	kinetics.Tracker
	origin Point
	target *hitRegion
}

func (m *touchMotion) stop() { m.target, m.Velocity = nil, Point{} }

// stepTouchMomentum advances a fling to now, scrolling its target by what
// it travels since the last step.
func (in *inputState) stepTouchMomentum(now time.Time) {
	m := &in.touchMotion
	if m.target == nil {
		return
	}
	dt := now.Sub(m.Last).Seconds()
	if dt <= 0 {
		return
	}
	// Background tabs should not resume an old fling or jump forward.
	if dt > maxFrameStep.Seconds() || !kinetics.Flinging(m.Velocity) {
		m.stop()
		return
	}
	cur := in.findPointer(m.target)
	if cur == nil {
		m.stop()
		return
	}
	if scope := in.activeScope(); scope != nil && (cur.scope == nil || !sameAny(scope.owner, cur.scope.owner)) {
		m.stop()
		return
	}
	travel, next, finished := kinetics.Fling(m.Velocity, dt)
	ev := PointerEvent{Kind: PointerScroll, Pos: m.origin, Scroll: travel, ScrollPixels: true, ScrollMomentum: true}
	m.Last, m.Velocity = now, next
	if moved := cur.pointer.HandlePointer(ev); !moved || finished {
		m.stop()
	}
}
