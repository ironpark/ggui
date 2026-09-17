package ggui

import (
	"reflect"
	"sync"
)

// tracker collects the signals read while a reactive computation runs, so that
// the computation can be re-run when any of them changes. This mirrors Svelte's
// automatic dependency tracking: nothing is declared, reads are observed.
type tracker struct {
	mu      sync.Mutex
	current *effect
}

var deps tracker

type effect struct {
	fn    func()
	dirty bool
}

// Reader is the read side of a reactive value. *Signal and *Memo both
// satisfy it, so helpers such as Watch and Combine accept either.
type Reader[T any] interface {
	Get() T
}

// Signal is a reactive value. Reads inside an Effect subscribe to it; writes
// mark every subscriber dirty so the next frame recomputes them.
type Signal[T any] struct {
	mu   sync.Mutex
	val  T
	eq   func(a, b T) bool
	subs map[*effect]struct{}
}

// State creates a Signal holding v. T is inferred from the argument, so
// State(0) is a *Signal[int] and State("") a *Signal[string]; name it
// explicitly (State[float64](0), State[Widget](nil)) when the literal would
// infer the wrong type or none at all. When T is comparable, writing an equal
// value is a no-op; see WithEqual to supply equality for other types.
func State[T any](v T) *Signal[T] {
	return &Signal[T]{val: v, eq: comparableEqual[T](), subs: map[*effect]struct{}{}}
}

// comparableEqual returns == for comparable T and nil for everything else.
// Interfaces are excluded: they compare fine until a dynamic value that does
// not, at which point == panics.
func comparableEqual[T any]() func(a, b T) bool {
	t := reflect.TypeFor[T]()
	if t.Kind() == reflect.Interface || !t.Comparable() {
		return nil
	}
	return func(a, b T) bool { return any(a) == any(b) }
}

// WithEqual sets the test Set uses to drop redundant writes, and returns s so
// it can be chained onto State. Passing nil makes every write notify.
func (s *Signal[T]) WithEqual(eq func(a, b T) bool) *Signal[T] {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eq = eq
	return s
}

// Get returns the current value and subscribes the running Effect, if any.
func (s *Signal[T]) Get() T {
	deps.mu.Lock()
	e := deps.current
	deps.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if e != nil {
		s.subs[e] = struct{}{}
	}
	return s.val
}

// Set stores v and invalidates every subscriber. A write equal to the current
// value changes nothing and notifies no one.
func (s *Signal[T]) Set(v T) {
	s.mu.Lock()
	if s.eq != nil && s.eq(s.val, v) {
		s.mu.Unlock()
		return
	}
	s.val = v
	subs := make([]*effect, 0, len(s.subs))
	for e := range s.subs {
		subs = append(subs, e)
	}
	s.mu.Unlock()

	for _, e := range subs {
		e.dirty = true
	}
}

// Update applies fn to the current value and stores the result.
func (s *Signal[T]) Update(fn func(T) T) {
	s.mu.Lock()
	cur := s.val
	s.mu.Unlock()
	s.Set(fn(cur))
}

// Map returns a Memo holding fn applied to s's value, recomputed whenever s
// changes. Create it once, next to the Signal: every call registers an effect,
// so calling it inside a Builder would add one per rebuild.
func (s *Signal[T]) Map[U any](fn func(T) U) *Memo[U] {
	return Derived(func() U { return fn(s.Get()) })
}

// Toggle flips a boolean signal.
func Toggle(s *Signal[bool]) { s.Update(func(b bool) bool { return !b }) }

// Add adds d to a numeric signal.
func Add[N Number](s *Signal[N], d N) { s.Update(func(n N) N { return n + d }) }

// Memo is a derived value: it recomputes when one of the signals its function
// read changes, and notifies its own readers only when the result differs.
type Memo[T any] struct {
	sig     *Signal[T]
	dispose func()
}

// Derived creates a Memo computed by fn. fn runs once immediately, and again on
// the frame after any signal it read changes. Like Signal.Map, create it once
// rather than inside a Builder.
func Derived[T any](fn func() T) *Memo[T] {
	var zero T
	m := &Memo[T]{sig: State(zero)}
	m.dispose = Effect(func() { m.sig.Set(fn()) })
	return m
}

// Combine derives a value from two reactive sources.
func Combine[A, B, C any](a Reader[A], b Reader[B], fn func(A, B) C) *Memo[C] {
	return Derived(func() C { return fn(a.Get(), b.Get()) })
}

// Get returns the memoized value and subscribes the running Effect, if any.
func (m *Memo[T]) Get() T { return m.sig.Get() }

// Dispose stops recomputation. Readers keep seeing the last computed value.
func (m *Memo[T]) Dispose() { m.dispose() }

// Map chains another derivation onto m.
func (m *Memo[T]) Map[U any](fn func(T) U) *Memo[U] {
	return Derived(func() U { return fn(m.Get()) })
}

// Watch runs fn with src's value now, and again whenever it changes. It returns
// a dispose function, like Effect.
func Watch[T any](src Reader[T], fn func(T)) (dispose func()) {
	return Effect(func() { fn(src.Get()) })
}

// Effect runs fn immediately and again whenever a Signal it read has changed.
// It returns a dispose function; re-runs are driven by the frame loop.
func Effect(fn func()) (dispose func()) {
	e := &effect{fn: fn}
	runEffect(e)
	effects.add(e)
	return func() { effects.remove(e) }
}

func runEffect(e *effect) {
	deps.mu.Lock()
	prev := deps.current
	deps.current = e
	deps.mu.Unlock()

	e.dirty = false
	e.fn()

	deps.mu.Lock()
	deps.current = prev
	deps.mu.Unlock()
}

type effectSet struct {
	mu   sync.Mutex
	list []*effect
}

var effects effectSet

func (s *effectSet) add(e *effect) {
	s.mu.Lock()
	s.list = append(s.list, e)
	s.mu.Unlock()
}

func (s *effectSet) remove(e *effect) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, x := range s.list {
		if x == e {
			s.list = append(s.list[:i], s.list[i+1:]...)
			return
		}
	}
}

// maxFlushPasses bounds how far a change propagates through derived values in
// one frame. Chains settle in a pass or two; the cap only stops a cycle.
const maxFlushPasses = 16

// flush re-runs every dirty effect, repeating until the tree is quiet so that a
// Memo feeding another effect lands in the same frame. Called once per frame by
// the runtime.
func (s *effectSet) flush() {
	for range maxFlushPasses {
		s.mu.Lock()
		list := append([]*effect(nil), s.list...)
		s.mu.Unlock()

		ran := false
		for _, e := range list {
			if e.dirty {
				runEffect(e)
				ran = true
			}
		}
		if !ran {
			return
		}
	}
}
