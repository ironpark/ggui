package kinetics

import "math"

// Spring pulls a number toward a target like a mass on a spring: it
// overshoots a little, keeps its momentum when the target moves, and
// settles. A value of several numbers, a point or a color, is a Spring for
// each, at rest when all of them are.
type Spring struct {
	Pos, Vel, To float64

	Stiffness float64 // the spring constant: higher snaps faster
	Damping   float64 // the friction: lower overshoots more, higher creeps
	Precision float64 // how near To, and how slow, counts as at rest
}

// NewSpring returns a Spring at rest at at, with the stiffness and damping
// that settle in a few hundred milliseconds with a slight overshoot.
func NewSpring(at float64) Spring {
	return Spring{Pos: at, To: at, Stiffness: 170, Damping: 18, Precision: 0.01}
}

// MaxSpringStep is the most time one Step advances, so that a frame that
// came late, or a window that was hidden, does not jump the spring ahead.
const MaxSpringStep = 0.1

// Step advances the spring dt seconds, at most MaxSpringStep, and reports
// whether it has come to rest: within Precision of To and moving slower
// than Precision, at which it lands on To and stops.
//
// The motion is the damped oscillator's own solution, not an integration
// of it, so it is exact for any step and stable for any stiffness and
// damping.
func (s *Spring) Step(dt float64) (rest bool) {
	if dt = min(dt, MaxSpringStep); dt > 0 {
		x, v := oscillate(s.Pos-s.To, s.Vel, max(s.Stiffness, 0), max(s.Damping, 0), dt)
		s.Pos, s.Vel = s.To+x, v
	}
	if math.Abs(s.Pos-s.To) >= s.Precision || math.Abs(s.Vel) >= s.Precision {
		return false
	}
	s.Pos, s.Vel = s.To, 0
	return true
}

// oscillate returns where a unit mass at x moving at v is t seconds later,
// and how fast, pulled to zero by a spring of stiffness k and slowed by
// damping c: x” = -k x - c x'.
func oscillate(x, v, k, c, t float64) (float64, float64) {
	if k == 0 { // no spring: friction alone
		if c == 0 {
			return x + v*t, v
		}
		e := math.Exp(-c * t)
		return x + v*(1-e)/c, v * e
	}
	w := math.Sqrt(k) // the natural frequency
	z := c / (2 * w)  // the damping ratio
	switch {
	case math.Abs(z-1) < 1e-6: // critically damped
		e := math.Exp(-w * t)
		b := v + w*x
		return e * (x + b*t), e * (b - w*(x+b*t))
	case z < 1: // under: it overshoots and rings down
		a, wd := z*w, w*math.Sqrt(1-z*z)
		e := math.Exp(-a * t)
		sin, cos := math.Sincos(wd * t)
		b := (v + a*x) / wd
		return e * (x*cos + b*sin), e * (v*cos - (a*b+wd*x)*sin)
	default: // over: it creeps in without crossing
		q := w * math.Sqrt(z*z-1)
		r1, r2 := -z*w+q, -z*w-q
		c1 := (v - r2*x) / (r1 - r2)
		c2 := x - c1
		e1, e2 := math.Exp(r1*t), math.Exp(r2*t)
		return c1*e1 + c2*e2, c1*r1*e1 + c2*r2*e2
	}
}
