package ggui

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// layoutGen counts the changes that can move something on screen: every
// Signal write and every RequestLayout. The runtime lays the tree out again
// only when it has advanced, or the window changed size, and paints every
// frame regardless.
var layoutGen atomic.Uint64

// RequestLayout asks the runtime to lay the tree out again next frame. A
// Signal write does this by itself; call it for state a widget keeps
// outside signals when that state changes its size or its children's.
func RequestLayout() { layoutGen.Add(1) }

// tracker holds the running computation. listener is the effect that reads
// subscribe to (nil inside Untrack); owner is the effect that newly created
// effects belong to, so that they are disposed when it re-runs or is
// disposed. This mirrors Svelte's automatic dependency tracking and Solid's
// ownership tree: nothing is declared, reads and creations are observed.
type tracker struct {
	mu       sync.Mutex
	listener *effect
	owner    *effect
}

var deps tracker

// source is anything an effect can subscribe to.
type source interface {
	unsubscribe(e *effect)
}

type effect struct {
	fn       func()
	dirty    bool
	disposed bool

	owner    *effect
	children []*effect
	cleanups []func()
	sources  []source
}

// reset undoes everything the last run set up: child effects, cleanups and
// subscriptions. It runs before each re-run and on dispose.
func (e *effect) reset() {
	for _, c := range e.children {
		c.dispose()
	}
	e.children = nil
	for i := len(e.cleanups) - 1; i >= 0; i-- {
		e.cleanups[i]()
	}
	e.cleanups = nil
	for _, s := range e.sources {
		s.unsubscribe(e)
	}
	e.sources = nil
}

func (e *effect) dispose() {
	if e.disposed {
		return
	}
	e.disposed = true
	e.reset()
	effects.remove(e)
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
	e := deps.listener
	deps.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if e != nil && !e.disposed {
		if _, ok := s.subs[e]; !ok {
			s.subs[e] = struct{}{}
			e.sources = append(e.sources, s)
		}
	}
	return s.val
}

// Peek returns the current value without subscribing the running Effect.
func (s *Signal[T]) Peek() T {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.val
}

func (s *Signal[T]) unsubscribe(e *effect) {
	s.mu.Lock()
	delete(s.subs, e)
	s.mu.Unlock()
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
	layoutGen.Add(1)
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

// Peek returns the memoized value without subscribing the running Effect.
func (m *Memo[T]) Peek() T { return m.sig.Peek() }

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
// Subscriptions are collected afresh on every run, so an effect follows only
// what it read last time. An Effect created while another effect runs belongs
// to it: it is disposed when the owner re-runs or is disposed, so effects and
// Derived values created inside a Builder do not pile up across rebuilds.
// Effect returns a dispose function; re-runs are driven by the frame loop.
func Effect(fn func()) (dispose func()) {
	e := &effect{fn: fn}
	deps.mu.Lock()
	e.owner = deps.owner
	deps.mu.Unlock()
	if e.owner != nil {
		e.owner.children = append(e.owner.children, e)
	}
	effects.add(e)
	// A panic in fn must not leave a half-built effect registered.
	ok := false
	defer func() {
		if !ok {
			e.dispose()
		}
	}()
	runEffect(e)
	ok = true
	return e.dispose
}

// Root runs fn untracked under a fresh owner that never re-runs, so effects
// and components fn creates live until the returned dispose is called or the
// enclosing owner is disposed. It is how a container keeps children alive
// across its own re-runs; For uses it per key.
func Root(fn func()) (dispose func()) {
	r := &effect{}
	deps.mu.Lock()
	r.owner = deps.owner
	prevListener, prevOwner := deps.listener, deps.owner
	deps.listener, deps.owner = nil, r
	deps.mu.Unlock()
	if r.owner != nil {
		r.owner.children = append(r.owner.children, r)
	}
	ok := false
	defer func() {
		deps.mu.Lock()
		deps.listener, deps.owner = prevListener, prevOwner
		deps.mu.Unlock()
		if !ok {
			r.dispose()
		}
	}()
	fn()
	ok = true
	return r.dispose
}

// withOwner runs fn with owner as the current owner and no listener.
func withOwner(owner *effect, fn func()) {
	deps.mu.Lock()
	prevListener, prevOwner := deps.listener, deps.owner
	deps.listener, deps.owner = nil, owner
	deps.mu.Unlock()
	defer func() {
		deps.mu.Lock()
		deps.listener, deps.owner = prevListener, prevOwner
		deps.mu.Unlock()
	}()
	fn()
}

func currentOwner() *effect {
	deps.mu.Lock()
	defer deps.mu.Unlock()
	return deps.owner
}

// OnCleanup registers fn to run before the enclosing Effect re-runs and when
// it is disposed. Call it from inside an Effect, a Builder or a Component
// setup, for timers, subscriptions and anything else that must be undone.
func OnCleanup(fn func()) {
	deps.mu.Lock()
	o := deps.owner
	deps.mu.Unlock()
	if o == nil {
		panic("ggui: OnCleanup called outside an Effect")
	}
	o.cleanups = append(o.cleanups, fn)
}

// Untrack runs fn without subscribing the running Effect to the signals fn
// reads. Effects created inside still belong to the running Effect.
func Untrack(fn func()) {
	deps.mu.Lock()
	prev := deps.listener
	deps.listener = nil
	deps.mu.Unlock()
	defer func() {
		deps.mu.Lock()
		deps.listener = prev
		deps.mu.Unlock()
	}()
	fn()
}

func runEffect(e *effect) {
	e.reset()

	deps.mu.Lock()
	prevListener, prevOwner := deps.listener, deps.owner
	deps.listener, deps.owner = e, e
	deps.mu.Unlock()
	defer func() {
		deps.mu.Lock()
		deps.listener, deps.owner = prevListener, prevOwner
		deps.mu.Unlock()
	}()

	e.dirty = false
	e.fn()
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
			if e.dirty && !e.disposed {
				runEffect(e)
				ran = true
			}
		}
		if !ran {
			return
		}
	}
}
