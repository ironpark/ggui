package ggui

import (
	"slices"
	"time"

	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/kinetics"
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

// panTouch lets a scroll region take over a touch once kinetics.Pan says
// it pans.
// Hit testing stays at the gesture origin; a captured scroller continues
// receiving movement even when the finger leaves its viewport.
func (in *inputState) panTouch(f *frameInput) {
	now := activeWorld().frame.raw()
	if len(f.down) != 0 || f.scrolled() || len(f.keys) != 0 {
		in.touchMotion.stop()
	}
	in.stepTouchMomentum(now)
	if !f.touch {
		return
	}
	if len(f.down) != 0 {
		in.touchPan.Down(f.pos)
		in.touchScroll = nil
		in.touchMotion = touchMotion{Last: now, Pos: f.pos}
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
	delta, ok := in.touchPan.Move(f.pos)
	if !ok {
		return
	}
	ev := PointerEvent{Kind: PointerScroll, Pos: in.touchPan.Start, Scroll: delta, ScrollPixels: true}
	if in.touchPan.Panning {
		if cur := in.findPointer(in.touchScroll); cur != nil && delta != (Point{}) {
			in.deliver(cur, ev)
		}
	} else if delta != (Point{}) {
		if target := in.send(ev); target != nil {
			in.touchScroll, in.touchPan.Panning = keep(target), true
			in.releasePress(f.pos, f.mods)
		}
	}
	if in.touchPan.Panning {
		// The original target has already been released. Never generate a tap.
		ended := len(f.up) != 0
		f.down, f.up = nil, nil
		if ended {
			in.touchMotion.Boost()
			in.touchMotion.target = in.touchScroll
			in.touchMotion.origin = in.touchPan.Start
			in.touchMotion.Last = now
			in.touchPan.Panning, in.touchScroll = false, nil
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
	if scope := in.activeScope(); scope != nil && (cur.scope == nil || !gem.Same(scope.owner, cur.scope.owner)) {
		m.stop()
		return
	}
	travel, next, finished := kinetics.Fling(m.Velocity, dt)
	ev := PointerEvent{Kind: PointerScroll, Pos: m.origin, Scroll: travel, ScrollPixels: true, ScrollMomentum: true}
	m.Last, m.Velocity = now, next
	if moved := in.deliver(cur, ev); !moved || finished {
		m.stop()
	}
}
