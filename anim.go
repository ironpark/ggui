package ggui

import (
	"math"
	"slices"
	"sync"
	"time"
)

// Animation is a reactive value that moves toward its target over time
// instead of jumping, after Svelte's tweened and spring stores. Tween moves
// along an easing curve for a fixed duration; Spring moves with physics and
// keeps its momentum when retargeted mid-flight. Both are Readers, so a
// Reactive island that reads one rebuilds every frame the value moves:
//
//	width := ggui.Tween(0.0, 200*time.Millisecond)
//	...
//	ggui.Reactive(func() ggui.Widget { return ggui.Box().Size(width.Get(), 4) })
//	...
//	width.Set(120) // slides there over 200ms
//
// The runtime steps every running animation once per frame, before effects
// are flushed. Tween and Spring created under an owner stop and release their
// registration when that owner is disposed; their value then stays frozen.
// Create component-local animations in setup so they survive builder reruns.
// Animations created without an owner live until they settle or Jump stops them.

// Easing maps normalized time in [0, 1] to normalized progress.
type Easing func(t float64) float64

// EaseLinear moves at constant speed.
func EaseLinear(t float64) float64 { return t }

// EaseOut starts fast and decelerates (cubic). It is the default: most UI
// motion should arrive gently.
func EaseOut(t float64) float64 { u := 1 - t; return 1 - u*u*u }

// EaseIn starts slowly and accelerates (cubic).
func EaseIn(t float64) float64 { return t * t * t }

// EaseInOut accelerates, then decelerates (cubic).
func EaseInOut(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := -2*t + 2
	return 1 - u*u*u/2
}

// stepper is an animation the runtime advances each frame.
type stepper interface {
	step(now time.Time) (running bool)
	active() bool
}

type animator struct {
	mu   sync.Mutex
	list []stepper
}

var anims animator

func (a *animator) add(s stepper) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if slices.Contains(a.list, s) {
		return
	}
	a.list = append(a.list, s)
}

// remove drops a stopped animation immediately, even if no more frames run.
func (a *animator) remove(s stepper) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if i := slices.Index(a.list, s); i >= 0 {
		a.list = slices.Delete(a.list, i, i+1)
	}
}

// step advances every running animation and drops the ones that finished.
func (a *animator) step(now time.Time) {
	a.mu.Lock()
	list := append([]stepper(nil), a.list...)
	a.mu.Unlock()

	for _, s := range list {
		s.step(now)
	}
	// Filter the live list once: callbacks may have added or stopped other
	// animations during step. Preserve new registrations, and avoid a
	// quadratic series of removals when many animations finish together.
	a.mu.Lock()
	a.list = slices.DeleteFunc(a.list, func(s stepper) bool { return !s.active() })
	a.mu.Unlock()
}

// Tweened is a value that eases from where it is to its target over a fixed
// duration. Build one with Tween.
type Tweened[T Number] struct {
	sig      *StateValue[T]
	duration time.Duration
	ease     Easing

	from, to T
	start    time.Time
	running  bool
	disposed bool
}

// Tween creates a Tweened at v that takes d to reach each new target, with
// EaseOut. T is inferred from v, so write Tween(0.0, d) for a float64.
func Tween[T Number](v T, d time.Duration) *Tweened[T] {
	t := &Tweened[T]{sig: State(v), duration: d, ease: EaseOut, from: v, to: v}
	if currentOwner() != nil {
		OnCleanup(func() {
			t.disposed, t.running = true, false
			anims.remove(t)
		})
	}
	return t
}

// TweenOf creates a Tweened that follows src: it starts at src's value and
// eases to each new one over d. It watches src, so like Watch it needs an
// owner: create it in App.Setup or component setup.
//
//	bar := ui.Progress(ggui.TweenOf(fraction, 300*time.Millisecond))
func TweenOf[T Number](src Readable[T], d time.Duration) *Tweened[T] {
	t := Tween(Untrack(src.Get), d)
	Watch(src, t.Set)
	return t
}

// Easing sets the curve; see EaseLinear, EaseIn, EaseOut, EaseInOut.
func (t *Tweened[T]) Easing(e Easing) *Tweened[T] { t.ease = e; return t }

// Duration sets how long each move takes.
func (t *Tweened[T]) Duration(d time.Duration) *Tweened[T] { t.duration = d; return t }

// Get returns the current value and subscribes the running Effect, so a
// Builder that reads it rebuilds as the value moves.
func (t *Tweened[T]) Get() T { return t.sig.Get() }

// GetAny returns the value as any and subscribes, for Sprintf.
func (t *Tweened[T]) GetAny() any { return t.Get() }

// Target returns where the value is heading.
func (t *Tweened[T]) Target() T { return t.to }

// Set starts moving from the current value to target.
func (t *Tweened[T]) Set(target T) {
	if t.disposed {
		return
	}
	t.from, t.to = Untrack(t.sig.Get), target
	if t.duration <= 0 || t.from == t.to {
		t.sig.Set(target)
		t.running = false
		anims.remove(t)
		return
	}
	t.start = time.Time{} // taken from the first step
	t.running = true
	anims.add(t)
}

// Jump moves to v at once, with no animation.
func (t *Tweened[T]) Jump(v T) {
	if t.disposed {
		return
	}
	t.from, t.to, t.running = v, v, false
	anims.remove(t)
	t.sig.Set(v)
}

func (t *Tweened[T]) active() bool { return t.running }

func (t *Tweened[T]) step(now time.Time) bool {
	if !t.running {
		return false
	}
	if t.start.IsZero() {
		t.start = now
	}
	p := float64(now.Sub(t.start)) / float64(t.duration)
	if p >= 1 {
		t.sig.Set(t.to)
		t.running = false
		return false
	}
	f := t.ease(clamp(p, 0, 1))
	if !t.running {
		return false
	}
	t.sig.Set(T(float64(t.from) + (float64(t.to)-float64(t.from))*f))
	return true
}

// Sprung is a value that moves toward its target like a mass on a spring:
// it overshoots a little, keeps its momentum when the target changes, and
// settles. Build one with Spring.
type Sprung[T Number] struct {
	sig       *StateValue[T]
	stiffness float64
	damping   float64
	precision float64

	pos, vel, to float64
	last         time.Time
	running      bool
	disposed     bool
}

// Spring creates a Sprung at v. Stiffness and Damping tune the motion; the
// defaults settle in a few hundred milliseconds with a slight overshoot.
func Spring[T Number](v T) *Sprung[T] {
	s := &Sprung[T]{sig: State(v), stiffness: 170, damping: 18, precision: 0.01, pos: float64(v), to: float64(v)}
	if currentOwner() != nil {
		OnCleanup(func() {
			s.disposed, s.running = true, false
			anims.remove(s)
		})
	}
	return s
}

// SpringOf creates a Sprung that follows src, as TweenOf does for a tween.
// It needs an owner, like Watch.
func SpringOf[T Number](src Readable[T]) *Sprung[T] {
	s := Spring(Untrack(src.Get))
	Watch(src, s.Set)
	return s
}

// Stiffness sets the spring constant: higher snaps faster.
func (s *Sprung[T]) Stiffness(k float64) *Sprung[T] { s.stiffness = k; return s }

// Damping sets the friction: lower overshoots more, higher creeps.
func (s *Sprung[T]) Damping(d float64) *Sprung[T] { s.damping = d; return s }

// Get returns the current value and subscribes the running Effect.
func (s *Sprung[T]) Get() T { return s.sig.Get() }

// GetAny returns the value as any and subscribes, for Sprintf.
func (s *Sprung[T]) GetAny() any { return s.Get() }

// Target returns where the value is heading.
func (s *Sprung[T]) Target() T { return T(s.to) }

// Set retargets the spring; motion already under way carries over.
func (s *Sprung[T]) Set(target T) {
	if s.disposed {
		return
	}
	s.to = float64(target)
	if !s.running {
		s.last = time.Time{}
		s.running = true
		anims.add(s)
	}
}

// Jump moves to v at once and stops.
func (s *Sprung[T]) Jump(v T) {
	if s.disposed {
		return
	}
	s.pos, s.vel, s.to, s.running = float64(v), 0, float64(v), false
	anims.remove(s)
	s.sig.Set(v)
}

func (s *Sprung[T]) active() bool { return s.running }

func (s *Sprung[T]) step(now time.Time) bool {
	if !s.running {
		return false
	}
	if s.last.IsZero() {
		s.last = now
		return true
	}
	dt := min(now.Sub(s.last).Seconds(), 0.1)
	s.last = now
	// Semi-implicit Euler in small substeps keeps a stiff spring stable.
	const h = 1.0 / 240
	for ; dt > 0; dt -= h {
		d := min(dt, h)
		acc := -s.stiffness*(s.pos-s.to) - s.damping*s.vel
		s.vel += acc * d
		s.pos += s.vel * d
	}
	if math.Abs(s.pos-s.to) < s.precision && math.Abs(s.vel) < s.precision {
		s.pos, s.vel, s.running = s.to, 0, false
		s.sig.Set(T(s.to))
		return false
	}
	s.sig.Set(T(s.pos))
	return true
}

// Motion is a tween a widget runs inside Paint, for looks that move (a
// switch knob sliding over) without a signal or a rebuild. The zero value is
// ready: the first MoveTo sets the position without animating, later ones
// ease to the target with EaseOut, starting from wherever the previous move
// had got to, and Value reports where it is at a given time.
//
//	func (s *knob) Paint(dst *ggui.Canvas, r ggui.Rect) {
//		now := ggui.Now()
//		s.pos.MoveTo(target, now, 150*time.Millisecond)
//		x := r.Origin.X + s.pos.Value(now)*r.Size.W
//		...
//	}
type Motion struct {
	from, to float64
	start    time.Time
	duration time.Duration
	init     bool
}

// MoveTo retargets the motion, starting from its position at now and
// arriving after d.
func (m *Motion) MoveTo(target float64, now time.Time, d time.Duration) {
	if !m.init || d <= 0 {
		m.duration = 0
		m.from, m.to, m.init = target, target, true
		return
	}
	if target == m.to {
		return
	}
	m.from, m.to, m.start, m.duration = m.Value(now), target, now, d
}

// Toggle moves toward one while on and zero while off, over d, and
// returns the position at now: the progress of something revealing
// itself. A widget that shows or hides a panel calls it once per frame.
func (m *Motion) Toggle(on bool, now time.Time, d time.Duration) float64 {
	m.MoveTo(pick(on, 1.0, 0.0), now, d)
	return m.Value(now)
}

// Value returns the position at now.
func (m *Motion) Value(now time.Time) float64 {
	if m.duration <= 0 || m.from == m.to {
		return m.to
	}
	p := float64(now.Sub(m.start)) / float64(m.duration)
	if p >= 1 {
		m.from = m.to
		return m.to
	}
	return m.from + (m.to-m.from)*EaseOut(clamp(p, 0, 1))
}
