package ggui

import (
	"math"
	"time"
)

const (
	touchFriction = 3.0 // exponential drag per second
	touchMinSpeed = 8.0 // logical pixels per second
	touchMaxSpeed = 8000.0
)

// touchMotion estimates release velocity independently of the update rate.
// Finger-down movement stays one-to-one; only released motion is decelerated.
type touchMotion struct {
	last                  time.Time
	pos, velocity, origin Point
	target                *hitRegion
}

func (m *touchMotion) stop() { m.target, m.velocity = nil, Point{} }

// boostRelease gives brisk flicks more reach without amplifying slow,
// precise gestures. Apply once on release, never during finger tracking.
func (m *touchMotion) boostRelease() {
	speed := math.Hypot(m.velocity.X, m.velocity.Y)
	if speed <= 150 {
		return
	}
	gain := 1 + 0.8*min(1, (speed-150)/700)
	gain = min(gain, touchMaxSpeed/speed)
	m.velocity = Pt(m.velocity.X*gain, m.velocity.Y*gain)
}

func (m *touchMotion) sample(now time.Time, pos Point) {
	dt := now.Sub(m.last).Seconds()
	if dt <= 0 {
		return
	}
	if dt > maxFrameStep.Seconds() {
		m.velocity = Point{}
	} else {
		// A short low-pass filter rejects noisy samples, while a direction
		// reversal discards old momentum immediately.
		blend := 1 - math.Exp(-dt/0.035)
		filter := func(old, distance float64) float64 {
			v := max(-touchMaxSpeed, min(touchMaxSpeed, distance/dt))
			if old*v < 0 {
				old = 0
			}
			return old + blend*(v-old)
		}
		m.velocity = Pt(filter(m.velocity.X, pos.X-m.pos.X), filter(m.velocity.Y, pos.Y-m.pos.Y))
	}
	m.last, m.pos = now, pos
}

func (in *inputState) stepTouchMomentum(now time.Time) {
	m := &in.touchMotion
	if m.target == nil {
		return
	}
	dt := now.Sub(m.last).Seconds()
	if dt <= 0 {
		return
	}
	// Background tabs should not resume an old fling or jump forward.
	if dt > maxFrameStep.Seconds() || math.Hypot(m.velocity.X, m.velocity.Y) < touchMinSpeed {
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
	// Integrate only up to the exact stopping speed, so the final travel
	// distance also agrees across update rates.
	stopAfter := math.Log(math.Hypot(m.velocity.X, m.velocity.Y)/touchMinSpeed) / touchFriction
	finished := dt >= stopAfter
	dt = min(dt, stopAfter)
	decay := math.Exp(-touchFriction * dt)
	distance := (1 - decay) / touchFriction
	ev := PointerEvent{Kind: PointerScroll, Pos: m.origin,
		Scroll:       Pt(m.velocity.X*distance, m.velocity.Y*distance),
		ScrollPixels: true, ScrollMomentum: true}
	m.last = now
	m.velocity = Pt(m.velocity.X*decay, m.velocity.Y*decay)
	if moved := cur.pointer.HandlePointer(ev); !moved || finished {
		m.stop()
	}
}
