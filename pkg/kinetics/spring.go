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

// springSubstep is the longest step Step integrates in one go: semi-implicit
// Euler in steps this small keeps the springs widgets use stable.
const springSubstep = 1.0 / 240

// MaxSpringStep is the most time one Step advances, so that a frame that
// came late, or a window that was hidden, does not fling the spring.
const MaxSpringStep = 0.1

// Step advances the spring dt seconds, at most MaxSpringStep, and reports
// whether it has come to rest: within Precision of To and moving slower
// than Precision, at which it lands on To and stops.
func (s *Spring) Step(dt float64) (rest bool) {
	// A stiffer or more heavily damped spring takes shorter steps: past
	// about one over its damping plus twice its natural frequency, each
	// step overshoots the last and the spring flies apart.
	h := springSubstep
	if r := s.Damping + 2*math.Sqrt(max(s.Stiffness, 0)); r*h > 1 {
		h = 1 / r
	}
	for dt = min(dt, MaxSpringStep); dt > 0; dt -= h {
		d := min(dt, h)
		s.Vel += (-s.Stiffness*(s.Pos-s.To) - s.Damping*s.Vel) * d
		s.Pos += s.Vel * d
	}
	if math.Abs(s.Pos-s.To) >= s.Precision || math.Abs(s.Vel) >= s.Precision {
		return false
	}
	s.Pos, s.Vel = s.To, 0
	return true
}
