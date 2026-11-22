// Package kinetics is the arithmetic of motion in ggui: easing curves, the
// resistance a scroll meets past its ends, where a snapping scroll comes to
// rest, a touch's pan and velocity, and the fling that carries on from it. It
// holds no widget, frame or clock: callers pass the time and positions in.
package kinetics

import (
	"math"

	"github.com/ironpark/ggui/gem"
)

// RubberReach is how far past an end, as a share of the viewport's
// extent, content can be pulled at the most: it nears it, and never gets
// there, however far the input goes.
const RubberReach = 0.25

// rubberGive is how much of the input's movement past an end the content
// follows at the end itself, as iOS's 0.55.
const rubberGive = 0.55

// Rubber moves raw, the offset a scroll's content shows at, by d, with
// movement past lo or hi resisted the further it goes, as iOS resists it:
// input carried t past an end puts the content
//
//	reach·give·t / (give·t + reach)
//
// past it, a little over half the movement at the end and ever less after,
// nearing reach, RubberReach of extent, and never getting there. Momentum
// gives less again than a finger. Movement back toward the range is never
// resisted.
func Rubber(raw, d, lo, hi, extent float64, momentum bool) float64 {
	next := raw + d
	if next >= lo && next <= hi || raw < lo && d > 0 && next <= hi || raw > hi && d < 0 && next >= lo {
		return next
	}
	edge := gem.Pick(next < lo, lo, hi)
	from := edge
	if next < lo && raw < lo || next > hi && raw > hi {
		from = raw
	}
	reach := max(extent, 1) * RubberReach
	give := rubberGive
	if momentum {
		give *= 0.4
	}
	// How far the input has gone past the end to put the content where it
	// is, and then where it puts it, gone d further.
	over := min(math.Abs(from-edge), reach*(1-1e-9))
	t := reach*over/(give*(reach-over)) + math.Abs(next-from)
	over = reach * give * t / (give*t + reach)
	return edge + math.Copysign(over, next-edge)
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
			t = gem.Pick(dir > 0, math.Ceil(n), math.Floor(n))
		}
	}
	v := gem.Clamp(t*step, 0, end)
	if !paging && math.Abs(end-pos) < math.Abs(v-pos) {
		v = end
	}
	return v
}
