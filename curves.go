package ggui

import "math"

// CubicBezier returns the easing CSS writes cubic-bezier(x1, y1, x2, y2): the
// curve from (0, 0) to (1, 1) with control points (x1, y1) and (x2, y2),
// read as progress y at time x. x1 and x2 are held to [0, 1] so time runs
// forward; y1 and y2 may leave it to overshoot.
//
//	ease := ggui.CubicBezier(0.4, 0, 0.2, 1)
func CubicBezier(x1, y1, x2, y2 float64) Easing {
	x1, x2 = clamp(x1, 0, 1), clamp(x2, 0, 1)
	// Polynomial coefficients of x(s) and y(s) for the curve parameter s.
	cx := 3 * x1
	bx := 3*(x2-x1) - cx
	ax := 1 - cx - bx
	cy := 3 * y1
	by := 3*(y2-y1) - cy
	ay := 1 - cy - by
	x := func(s float64) float64 { return ((ax*s+bx)*s + cx) * s }
	dx := func(s float64) float64 { return (3*ax*s+2*bx)*s + cx }
	return func(t float64) float64 {
		if t <= 0 {
			return 0
		}
		if t >= 1 {
			return 1
		}
		// Newton's method converges in a few steps on most curves; fall back
		// to bisection where the slope is too flat for it.
		s := t
		for range 8 {
			e := x(s) - t
			if math.Abs(e) < 1e-7 {
				return ((ay*s+by)*s + cy) * s
			}
			d := dx(s)
			if math.Abs(d) < 1e-6 {
				break
			}
			s -= e / d
		}
		lo, hi := 0.0, 1.0
		s = t
		for range 40 {
			if v := x(s); math.Abs(v-t) < 1e-7 {
				break
			} else if v < t {
				lo = s
			} else {
				hi = s
			}
			s = (lo + hi) / 2
		}
		return ((ay*s+by)*s + cy) * s
	}
}

// Interval runs e over the stretch of time from begin to end, both in
// [0, 1]: zero before it, one after. Give the parts of one animation
// overlapping or consecutive intervals to sequence them. A nil e is linear.
//
//	fade := ggui.Interval(0, 0.5, ggui.EaseOut)   // first half
//	grow := ggui.Interval(0.25, 1, ggui.EaseInOut) // overlapping the rest
func Interval(begin, end float64, e Easing) Easing {
	if e == nil {
		e = EaseLinear
	}
	return func(t float64) float64 {
		switch {
		case t >= end:
			return 1
		case t <= begin:
			return 0
		}
		return e((t - begin) / (end - begin))
	}
}

// Reverse returns e flipped end for end: t maps to 1 - e(1-t), so an easing
// in becomes the matching easing out. Use it for the way back, so a panel
// that slid in with EaseOutBack leaves with the same curve mirrored.
func Reverse(e Easing) Easing { return func(t float64) float64 { return 1 - e(1-t) } }

// EaseInOutCubic is EaseInOut, by its full name.
func EaseInOutCubic(t float64) float64 { return EaseInOut(t) }

const backOvershoot = 1.70158

// EaseInBack pulls back a little before it moves forward.
func EaseInBack(t float64) float64 {
	return (backOvershoot+1)*t*t*t - backOvershoot*t*t
}

// EaseOutBack overshoots its target a little and settles back.
func EaseOutBack(t float64) float64 { return 1 - EaseInBack(1-t) }

// EaseInOutBack pulls back at the start and overshoots at the end.
func EaseInOutBack(t float64) float64 {
	const c = backOvershoot * 1.525
	if t < 0.5 {
		return (2 * t) * (2 * t) * ((c+1)*2*t - c) / 2
	}
	u := 2*t - 2
	return (u*u*((c+1)*u+c) + 2) / 2
}

// EaseOutElastic overshoots and rings around its target like a plucked
// band before settling.
func EaseOutElastic(t float64) float64 {
	if t <= 0 || t >= 1 {
		return clamp(t, 0, 1)
	}
	return math.Pow(2, -10*t)*math.Sin((10*t-0.75)*(2*math.Pi/3)) + 1
}

// EaseInElastic is EaseOutElastic reversed: it winds up before it leaves.
func EaseInElastic(t float64) float64 { return 1 - EaseOutElastic(1-t) }

// EaseOutBounce drops onto its target and bounces to rest.
func EaseOutBounce(t float64) float64 {
	const n, d = 7.5625, 2.75
	switch {
	case t < 1/d:
		return n * t * t
	case t < 2/d:
		t -= 1.5 / d
		return n*t*t + 0.75
	case t < 2.5/d:
		t -= 2.25 / d
		return n*t*t + 0.9375
	default:
		t -= 2.625 / d
		return n*t*t + 0.984375
	}
}

// EaseInBounce is EaseOutBounce reversed: it bounces off its start.
func EaseInBounce(t float64) float64 { return 1 - EaseOutBounce(1-t) }

// The Material 3 motion curves. Standard suits small utility motion,
// Emphasized the expressive moves of larger elements; Decelerate is for
// what enters the screen, Accelerate for what leaves it.
var (
	standard             = CubicBezier(0.2, 0, 0, 1)
	standardAccelerate   = CubicBezier(0.3, 0, 1, 1)
	standardDecelerate   = CubicBezier(0, 0, 0, 1)
	emphasizedAccelerate = CubicBezier(0.3, 0, 0.8, 0.15)
	emphasizedDecelerate = CubicBezier(0.05, 0.7, 0.1, 1)
)

// EaseStandard is Material's standard curve, cubic-bezier(0.2, 0, 0, 1).
func EaseStandard(t float64) float64 { return standard(t) }

// EaseStandardAccelerate is Material's standard curve for an exit.
func EaseStandardAccelerate(t float64) float64 { return standardAccelerate(t) }

// EaseStandardDecelerate is Material's standard curve for an entrance.
func EaseStandardDecelerate(t float64) float64 { return standardDecelerate(t) }

// EaseEmphasized is Material's emphasized curve: a slow start, a quick
// middle and a long, gentle arrival. It is the two halves below joined at
// (1/6, 0.4), as Material specifies it.
func EaseEmphasized(t float64) float64 {
	const x, y = 1.0 / 6, 0.4
	if t < x {
		return y * emphasizedAccelerate(t/x)
	}
	return y + (1-y)*emphasizedDecelerate((t-x)/(1-x))
}

// EaseEmphasizedAccelerate is Material's emphasized curve for an exit.
func EaseEmphasizedAccelerate(t float64) float64 { return emphasizedAccelerate(t) }

// EaseEmphasizedDecelerate is Material's emphasized curve for an entrance.
func EaseEmphasizedDecelerate(t float64) float64 { return emphasizedDecelerate(t) }
