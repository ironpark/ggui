package kinetics

import (
	"math"
	"time"

	"github.com/ironpark/ggui/pkg/layout"
)

const (
	// FlingFriction is the exponential drag on a fling, per second.
	FlingFriction = 3.0
	// FlingMinSpeed is the speed, in logical pixels per second, below which
	// a fling stops.
	FlingMinSpeed = 8.0
	// FlingMaxSpeed caps a sampled and a boosted speed.
	FlingMaxSpeed = 8000.0
)

// Tracker estimates a touch's velocity from its positions independently of
// the rate they arrive at. A finger's own movement is followed one to one
// by its caller; only what carries on after release decelerates.
type Tracker struct {
	Last     time.Time    // when Pos was sampled
	Pos      layout.Point // the last position sampled
	Velocity layout.Point // in logical pixels per second
}

// Sample adds pos, sampled at now. A gap longer than maxGap, a finger that
// stopped, drops the velocity. A short low-pass filter rejects noisy
// samples, while a reversal of direction drops the old momentum at once.
func (t *Tracker) Sample(now time.Time, pos layout.Point, maxGap time.Duration) {
	dt := now.Sub(t.Last).Seconds()
	if dt <= 0 {
		return
	}
	if dt > maxGap.Seconds() {
		t.Velocity = layout.Point{}
	} else {
		blend := 1 - math.Exp(-dt/0.035)
		filter := func(old, distance float64) float64 {
			v := max(-FlingMaxSpeed, min(FlingMaxSpeed, distance/dt))
			if old*v < 0 {
				old = 0
			}
			return old + blend*(v-old)
		}
		t.Velocity = layout.Pt(filter(t.Velocity.X, pos.X-t.Pos.X), filter(t.Velocity.Y, pos.Y-t.Pos.Y))
	}
	t.Last, t.Pos = now, pos
}

// Boost gives brisk flicks more reach without amplifying slow, precise
// gestures. Apply it once, on release, never while a finger is tracked.
func (t *Tracker) Boost() {
	speed := math.Hypot(t.Velocity.X, t.Velocity.Y)
	if speed <= 150 {
		return
	}
	gain := 1 + 0.8*min(1, (speed-150)/700)
	gain = min(gain, FlingMaxSpeed/speed)
	t.Velocity = layout.Pt(t.Velocity.X*gain, t.Velocity.Y*gain)
}

// Flinging reports whether a fling at velocity v is still moving.
func Flinging(v layout.Point) bool { return math.Hypot(v.X, v.Y) >= FlingMinSpeed }

// Fling advances a fling at velocity v by dt seconds: how far it travels,
// its velocity after, and whether it has come to rest. It integrates only
// up to the exact time the fling slows to FlingMinSpeed, so the distance a
// fling covers is the same whatever steps it is advanced in.
func Fling(v layout.Point, dt float64) (travel, next layout.Point, done bool) {
	stopAfter := math.Log(math.Hypot(v.X, v.Y)/FlingMinSpeed) / FlingFriction
	done = dt >= stopAfter
	dt = min(dt, stopAfter)
	decay := math.Exp(-FlingFriction * dt)
	distance := (1 - decay) / FlingFriction
	return layout.Pt(v.X*distance, v.Y*distance), layout.Pt(v.X*decay, v.Y*decay), done
}

// PanSlop is how far, in logical pixels, a finger moves from where it went
// down before it pans rather than presses.
const PanSlop = 8.0

// Pan tells a finger panning from one pressing. Until the finger has moved
// PanSlop from Start it is a press; past it, its first movement is locked
// to the dominant axis, so a horizontal control in a vertical list can
// still be dragged without moving the list. Once something takes the pan,
// Panning is set and the finger is followed one to one, on both axes.
type Pan struct {
	Start   layout.Point // where the finger went down
	Panning bool         // something took the pan
	last    layout.Point
}

// Down starts a pan for a finger gone down at p.
func (p *Pan) Down(at layout.Point) { *p = Pan{Start: at, last: at} }

// Move returns how far to scroll for the finger now at pos, and false
// while it is still a press: since the last move once Panning, and before,
// from Start along the dominant axis.
func (p *Pan) Move(pos layout.Point) (delta layout.Point, ok bool) {
	if p.Panning {
		delta = pos.Sub(p.last)
	} else {
		delta = pos.Sub(p.Start)
		if delta.X*delta.X+delta.Y*delta.Y < PanSlop*PanSlop {
			return layout.Point{}, false
		}
		if delta.X*delta.X > delta.Y*delta.Y {
			delta.Y = 0
		} else {
			delta.X = 0
		}
	}
	p.last = pos
	return delta, true
}
