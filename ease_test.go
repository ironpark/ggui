package ggui

import (
	"math"
	"testing"
	"time"
)

// Ease moves along EaseOut; EaseWith along the curve it is given.
func TestCanvasEaseWithMovesAlongTheCurveGiven(t *testing.T) {
	slot := NewSlot[*Motion]("ease with test")
	for _, c := range []struct {
		name string
		ease Easing
		want float64
	}{
		{"default", nil, EaseOut(0.5)},
		{"linear", EaseLinear, 0.5},
		{"bezier", CubicBezier(0.4, 0, 0.2, 1), CubicBezier(0.4, 0, 0.2, 1)(0.5)},
	} {
		target, got := 0.0, 0.0
		w := FromFuncs(
			func(cs Constraints, _ Env) Size { return cs.Constrain(Sz(10, 10)) },
			func(dst *Canvas, r Rect) {
				if c.ease == nil {
					got = dst.Ease(Anchor{Rect: r}, slot, target, 100*time.Millisecond)
					return
				}
				got = dst.EaseWith(Anchor{Rect: r}, slot, target, 100*time.Millisecond, c.ease)
			})
		p := NewProbe(w, Sz(100, 100))
		p.Advance(0)
		target = 1
		p.Advance(0)
		p.Advance(50 * time.Millisecond)
		if math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: halfway in time is %v of the way there, want %v", c.name, got, c.want)
		}
		p.Close()
	}
}
