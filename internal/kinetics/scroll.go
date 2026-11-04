// Package kinetics is the arithmetic of motion in ggui: easing curves, the
// resistance a scroll meets past its ends, where a snapping scroll comes to
// rest, and a touch's velocity and the fling that carries on from it. It
// holds no widget, frame or clock: callers pass the time and positions in.
package kinetics

import (
	"math"

	"github.com/ironpark/ggui/internal/fn"
)

// Rubber moves raw, the offset a scroll's content shows at, by d, with
// movement past lo or hi resisted the further it goes: half the movement
// at an end, none a whole extent past it, and less again for momentum
// than for a finger. Movement back toward the range is never resisted.
func Rubber(raw, d, lo, hi, extent float64, momentum bool) float64 {
	next := raw + d
	if next >= lo && next <= hi || raw < lo && d > 0 || raw > hi && d < 0 {
		return next
	}
	edge := fn.Pick(next < lo, lo, hi)
	from := raw
	if raw >= lo && raw <= hi {
		from = edge
	}
	extent = max(extent, 1)
	k := 0.5 * (1 - min(math.Abs(from-edge)/extent, 1))
	if momentum {
		k *= 0.4
	}
	return from + (next-from)*k
}

// Stretch is how much content pulled over past an end is stretched along
// the axis in a viewport extent long: one and a half times at most, a whole
// extent past it.
func Stretch(over, extent float64) float64 {
	return 1 + 0.5*min(math.Abs(over)/extent, 1)
}

// SnapTarget returns where a scroll at pos settles, snapping to multiples
// of step between 0 and end. A free snap goes to the nearest multiple, or
// to end when that is nearer, so the last rows can be reached. A paging
// one goes back to the page the gesture began on, at from, unless it moved
// more than a sixth of a page, when it goes on to the next page in dir, the
// sign of its last movement.
func SnapTarget(pos, end, step, from, dir float64, paging bool) float64 {
	n := pos / step
	t := math.Round(n)
	if paging {
		t = math.Round(from / step)
		if math.Abs(pos-from) > step/6 {
			t = fn.Pick(dir > 0, math.Ceil(n), math.Floor(n))
		}
	}
	v := fn.Clamp(t*step, 0, end)
	if !paging && math.Abs(end-pos) < math.Abs(v-pos) {
		v = end
	}
	return v
}
