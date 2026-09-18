package ggui

import (
	"fmt"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
)

// layoutGen counts the changes that can move something on screen: every
// Signal write and every Invalidate. The runtime lays the tree out again
// only when it has advanced, or the window changed size, and paints every
// frame regardless.
var layoutGen atomic.Uint64

// requestLayout asks the runtime to lay the tree out again next frame.
func requestLayout() { layoutGen.Add(1) }

// Invalidate tells the runtime that the widget laid out under env changed
// size, or its children did: the tree is laid out again next frame and the
// nearest Cached above the widget measures its subtree afresh. A Signal
// write does the first half by itself; a widget that keeps size-affecting
// state outside signals calls Invalidate when that state changes.
func Invalidate(env Env) {
	requestLayout()
	if c, ok := env.Get(cacheOwner); ok {
		c.invalidate()
	}
}

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

	owner                    *effect
	firstChild, lastChild    *effect
	prevSibling, nextSibling *effect
	cleanups                 []func()

	// Registration and sibling links preserve creation order while allowing
	// a disposed computation to leave both lists in constant time.
	prevEffect, nextEffect *effect
	registered             bool
	sources                []source

	// Keyed components mounted during this effect's runs. They outlive a
	// re-run and go when a run no longer claims them, or with the effect.
	keyed   map[any]*mounted
	claimed map[any]bool

	// Identity for what is constructed under this effect: keyRoot is the
	// identity of the Keyed or Mount instance this effect is the root of, path
	// is the effect's place under the nearest such root, by construction
	// order, and seq counts what the current run constructed.
	keyRoot any
	path    string
	seq     int
}

// autoKey is the identity a widget gets from the keyed component it was
// constructed in: the component's mount instance and the widget's place, by
// construction order, under it. It is the same across the component's
// rebuilds, so the widget's hit region, retained state and adoption
// follow it without a Key of its own.
type autoKey struct {
	root any
	path string
}

// autoID returns the identity for a widget being constructed now: nil
// outside a keyed component.
func autoID() any {
	o := currentOwner()
	if o == nil {
		return nil
	}
	var root any
	for r := o; r != nil; r = r.owner {
		if r.keyRoot != nil {
			root = r.keyRoot
			break
		}
	}
	if root == nil {
		return nil
	}
	k := autoKey{root: root, path: o.path + "/" + itoa(o.seq)}
	o.seq++
	return k
}

// place gives e its path under owner: owner's path plus e's construction
// ordinal, or elem when the caller names it, as For does with the item key.
func (e *effect) place(owner *effect, elem string) {
	if owner == nil {
		return
	}
	if elem == "" {
		elem = itoa(owner.seq)
		owner.seq++
	}
	e.path = owner.path + "/" + elem
}

// claim returns the mounted component under key, marking it as still in
// use by this run, or nil.
func (e *effect) claim(key any) *mounted {
	if e.claimed == nil {
		e.claimed = map[any]bool{}
	}
	if e.claimed[key] {
		panic(fmt.Sprintf("ggui: key %v used twice in one build", key))
	}
	e.claimed[key] = true
	return e.keyed[key]
}

func (e *effect) keep(key any, m *mounted) {
	if e.keyed == nil {
		e.keyed = map[any]*mounted{}
	}
	e.keyed[key] = m
}

// sweep disposes the keyed components the last run did not claim.
func (e *effect) sweep() {
	for k, m := range e.keyed {
		if !e.claimed[k] {
			m.dispose()
			delete(e.keyed, k)
		}
	}
	clear(e.claimed)
}

// attach appends a child to its owner's ordered list.
func (e *effect) attach(owner *effect) {
	e.owner = owner
	e.prevSibling = owner.lastChild
	if owner.lastChild != nil {
		owner.lastChild.nextSibling = e
	} else {
		owner.firstChild = e
	}
	owner.lastChild = e
}

// detach releases the parent's reference before running cleanup. reset can
// then consume its first child repeatedly, even if cleanup disposes siblings.
func (e *effect) detach() {
	if e.owner == nil {
		return
	}
	if e.prevSibling != nil {
		e.prevSibling.nextSibling = e.nextSibling
	} else {
		e.owner.firstChild = e.nextSibling
	}
	if e.nextSibling != nil {
		e.nextSibling.prevSibling = e.prevSibling
	} else {
		e.owner.lastChild = e.prevSibling
	}
	e.owner, e.prevSibling, e.nextSibling = nil, nil, nil
}

// reset undoes everything the last run set up: child effects, cleanups and
// subscriptions. It runs before each re-run and on dispose.
func (e *effect) reset() {
	for e.firstChild != nil {
		e.firstChild.dispose()
	}
	for _, v := range slices.Backward(e.cleanups) {
		v()
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
	e.detach()
	e.reset()
	for _, m := range e.keyed {
		m.dispose()
	}
	e.keyed = nil
	effects.remove(e)
}

// Reader is the read side of a reactive value. *Signal and *Memo both
// satisfy it, so helpers such as Watch and Combine accept either.
type Reader[T any] interface {
	Get() T
}

// Binding is a reactive value that can be written as well as read: what a
// control binds to. *Signal, *Lens, *Tweened and *Sprung satisfy it.
type Binding[T any] interface {
	Reader[T]
	Peek() T
	Set(T)
}

// Writable is a binding with immediate read-modify-write semantics. Signals and
// lenses implement it; animated bindings do not. Updates belong on the UI thread.
type Writable[T any] interface {
	Binding[T]
	Update(func(T) T)
}

// Lens is a two-way view of part of a Signal's value. Build one with
// Signal.Lens.
type Lens[U any] struct {
	get  func() U
	peek func() U
	set  func(U)
}

// Lens returns a Binding onto the part of s's value that get selects: Get
// subscribes through s, and Set reads s, applies set to store the new part
// and writes the whole back. It is how a control binds to one field of a
// struct held in a single signal.
//
//	name := form.Lens(func(f Form) string { return f.Name }, func(f Form, v string) Form { f.Name = v; return f })
//	ui.TextField(name)
func (s *Signal[T]) Lens[U any](get func(T) U, set func(T, U) T) *Lens[U] {
	return &Lens[U]{
		get:  func() U { return get(s.Get()) },
		peek: func() U { return get(s.Peek()) },
		set:  func(u U) { s.Set(set(s.Peek(), u)) },
	}
}

// Get returns the part and subscribes the running Effect.
func (l *Lens[U]) Get() U { return l.get() }

// Peek returns the part without subscribing.
func (l *Lens[U]) Peek() U { return l.peek() }

// Set stores the part into the whole.
func (l *Lens[U]) Set(v U) { l.set(v) }

// Update applies fn to the current part and writes it through to the whole.
// Like Signal.Update it belongs on the UI thread.
func (l *Lens[U]) Update(fn func(U) U) { l.Set(fn(l.Peek())) }

// GetAny returns the value as any and subscribes, for Sprintf.
func (l *Lens[U]) GetAny() any { return l.Get() }

// anyReader is what Sprintf looks for among its arguments: a reactive value
// read without its type.
type anyReader interface{ GetAny() any }

// GetAny returns the value as any and subscribes, for Sprintf.
func (s *Signal[T]) GetAny() any { return s.Get() }

// GetAny returns the value as any and subscribes, for Sprintf.
func (m *Memo[T]) GetAny() any { return m.Get() }

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

// Update applies fn to the current value and stores the result. Like Set
// it belongs on the UI thread; it is not an atomic read-modify-write for
// goroutines, which hand their result back with App.Post.
func (s *Signal[T]) Update(fn func(T) T) {
	s.mu.Lock()
	cur := s.val
	s.mu.Unlock()
	s.Set(fn(cur))
}

// Map returns a Memo holding fn applied to s's value, recomputed whenever s
// changes. Its effect belongs to the current owner: a Builder disposes it
// before rebuilding, so retaining that Memo outside the build leaves a frozen
// value. Without an owner, call Dispose explicitly; App.Close does not own it.
func (s *Signal[T]) Map[U any](fn func(T) U) *Memo[U] {
	return Derived(func() U { return fn(s.Get()) })
}

// Toggle flips a writable boolean.
func Toggle(s Writable[bool]) { s.Update(func(b bool) bool { return !b }) }

// Add adds d to a writable number.
func Add[N Number](s Writable[N], d N) { s.Update(func(n N) N { return n + d }) }

// Append adds items to a writable slice, in a new slice so the
// change is noticed.
func Append[T any](s Writable[[]T], items ...T) {
	s.Update(func(ts []T) []T { return append(ts[:len(ts):len(ts)], items...) })
}

// Remove drops every item of a writable slice that drop accepts, into a new
// slice. Nothing is set, so nothing notifies, when no item matched.
func Remove[T any](s Writable[[]T], drop func(T) bool) {
	ts := s.Peek()
	out := ts[:0:0]
	for _, t := range ts {
		if !drop(t) {
			out = append(out, t)
		}
	}
	if len(out) != len(ts) {
		s.Set(out)
	}
}

// Memo is a derived value: it recomputes when one of the signals its function
// read changes, and notifies its own readers only when the result differs.
type Memo[T any] struct {
	sig     *Signal[T]
	dispose func()
}

// Derived creates a Memo computed by fn. fn runs once immediately, and again on
// the frame after any signal it read changes. The current owner disposes it
// before re-running or closing. A Memo retained outside that owner then keeps
// its last value. Without an owner, call Dispose explicitly or create it in Root;
// App.Close does not dispose computations created outside the app's owner.
func Derived[T any](fn func() T) *Memo[T] {
	var zero T
	m := &Memo[T]{sig: State(zero)}
	m.dispose = Effect(func() { m.sig.Set(fn()) })
	return m
}

// Map derives a value from any reactive source; Signal.Map and Memo.Map are
// the same for a source whose type is known.
func Map[T, U any](r Reader[T], fn func(T) U) *Memo[U] {
	return Derived(func() U { return fn(r.Get()) })
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
		e.attach(e.owner)
		e.place(e.owner, "")
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
func Root(fn func()) (dispose func()) { return rootWith(nil, "", fn) }

// rootWith is Root for a root that is the instance of a keyed component
// (identity) or has a name of its own under its owner (elem), for the
// identities autoID derives.
func rootWith(identity any, elem string, fn func()) (dispose func()) {
	r := &effect{keyRoot: identity}
	deps.mu.Lock()
	r.owner = deps.owner
	prevListener, prevOwner := deps.listener, deps.owner
	deps.listener, deps.owner = nil, r
	deps.mu.Unlock()
	if r.owner != nil {
		r.attach(r.owner)
		r.place(r.owner, elem)
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
	e.seq = 0

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
	e.sweep()
}

type effectSet struct {
	mu          sync.Mutex
	first, last *effect
	count       int
}

var effects effectSet

func (s *effectSet) add(e *effect) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.prevEffect, e.registered = s.last, true
	if s.last != nil {
		s.last.nextEffect = e
	} else {
		s.first = e
	}
	s.last = e
	s.count++
}

func (s *effectSet) remove(e *effect) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !e.registered {
		return
	}
	if e.prevEffect != nil {
		e.prevEffect.nextEffect = e.nextEffect
	} else {
		s.first = e.nextEffect
	}
	if e.nextEffect != nil {
		e.nextEffect.prevEffect = e.prevEffect
	} else {
		s.last = e.prevEffect
	}
	e.prevEffect, e.nextEffect, e.registered = nil, nil, false
	s.count--
}

// maxFlushPasses bounds how far a change propagates through derived values in
// one frame. Chains settle in a pass or two; the cap only stops a cycle.
const maxFlushPasses = 16

// flush re-runs every dirty effect, repeating until the tree is quiet so that a
// Memo feeding another effect lands in the same frame. Called once per frame by
// the runtime. It reports false when the effects were still dirty after
// maxFlushPasses, which only a cycle causes.
func (s *effectSet) flush() (settled bool) {
	for range maxFlushPasses {
		s.mu.Lock()
		list := make([]*effect, 0, s.count)
		for e := s.first; e != nil; e = e.nextEffect {
			list = append(list, e)
		}
		s.mu.Unlock()

		ran := false
		for _, e := range list {
			if e.dirty && !e.disposed {
				runEffect(e)
				ran = true
			}
		}
		if !ran {
			return true
		}
	}
	return false
}
