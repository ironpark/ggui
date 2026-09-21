package ggui

import (
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
)

// layoutGen counts the changes that can move something on screen: every
// StateValue write and every Invalidate. The runtime lays the tree out again
// only when it has advanced, or the window changed size, and paints every
// frame regardless.
//
// It is atomic and stateGen is not, which is the difference between the two:
// requestLayout is reachable from anything that changes a size, and a stray
// call from another goroutine should cost one extra layout rather than tear
// the counter. stateGen is written only by store and read only by settle,
// both on the UI goroutine under checkUIThread, and marks a state write that
// happened during a layout so settle knows to go round again.
var layoutGen atomic.Uint64
var stateGen uint64

// requestLayout asks the runtime to lay the tree out again next frame.
func requestLayout() { layoutGen.Add(1) }

// Measurement dependencies are separate from reactive subscriptions: even an
// Untrack read affects layout, but never subscribes the enclosing computation.
type layoutSource interface{ layoutVersion() uint64 }

// measuring is the recorder a running Layout installs so that a signal
// read during measurement is remembered as an input of that layout. It is
// a func rather than the cache itself, so that the reactive core does not
// name the layout cache.
var measuring func(src layoutSource, version uint64)

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

// deps, effects and derivedDepth are the reactive system's one running
// state: there is a single UI goroutine, and every entry point that reaches
// them -- Get, Set, Derived, Effect -- runs under checkUIThread. deps takes
// a mutex anyway because a read may cross into a Derived's own computation.
var deps tracker

// source is anything an effect can subscribe to.
type source interface {
	unsubscribe(e *effect)
	// producer is the memo effect that computes this source, or nil for a
	// plain signal: how refresh finds the upstream to settle first.
	producer() *effect
}

// downstream is a memo's value seen from its effect: the readers to carry
// staleness on to.
type downstream interface{ markSubsCheck() }

// An effect is clean, or it may be stale. A StateValue write marks its direct
// subscribers dirty and everything downstream of a memo check: check means
// "an input of yours may have changed", and is resolved by refreshing the
// upstream memos, which turns it into dirty or back into clean. It is what
// keeps a reader from seeing one input updated and another not.
const (
	stateClean uint8 = iota
	stateCheck
	stateDirty
)

type effect struct {
	fn         func()
	state      uint8
	user       bool
	persistent bool
	// loop is the frame loop this effect belongs to, held only so that
	// flushUsers can tell one loop's effects from another's. It is never
	// dereferenced here, so the reactive core needs no frame loop type.
	loop     any
	running  bool
	disposed bool

	// cell is the memo's value this effect computes, if it is a memo's:
	// what carries staleness on to the memo's own readers.
	cell downstream

	owner                    *effect
	firstChild, lastChild    *effect
	prevSibling, nextSibling *effect
	cleanups                 []func()

	// Registration and sibling links preserve creation order while allowing
	// a disposed computation to leave both lists in constant time.
	prevEffect, nextEffect *effect
	registered             bool
	sources                []source

	// Where this effect was created, in the ggui_debug build only: what
	// lets ErrCycle name the effects a cycle is made of. Empty otherwise.
	origin string

	// Identity for what is constructed under this effect: keyRoot is the
	// identity of the mounted component or block this effect is the root of, path
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

// itoa formats a small non-negative int. The reactive core keeps its own
// rather than reaching into the frame loop's file for a digit loop.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
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
// ordinal, or elem when the caller names it, as EachKeyed does with the item key.
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
	var children []*effect
	for child := e.firstChild; child != nil; child = child.nextSibling {
		if e.disposed || !child.persistent {
			children = append(children, child)
		}
	}
	for _, child := range children {
		child.dispose()
	}

	withOwner(nil, func() {
		for _, v := range slices.Backward(e.cleanups) {
			v()
		}
	})
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
	effects.remove(e)
}

// Readable is the read side of a reactive value. *StateValue and *DerivedValue both
// satisfy it, so helpers such as Watch and Combine accept either.
type Readable[T any] interface {
	Get() T
}

// Binding is a reactive value that can be written as well as read: what a
// control binds to. *StateValue, *Lens, *Tweened and *Sprung satisfy it.
type Binding[T any] interface {
	Readable[T]
	Set(T)
}

// Writable is a binding with immediate read-modify-write semantics. Signals and
// lenses implement it; animated bindings do not. Updates belong on the UI thread.
type Writable[T any] interface {
	Binding[T]
	Update(func(T) T)
}

// Lens is a two-way view of part of a StateValue's value. Build one with
// StateValue.Lens.
type Lens[U any] struct {
	get func() U
	set func(U)
}

// Lens returns a Binding onto the part of s's value that get selects: Get
// subscribes through s, and Set reads s, applies set to store the new part
// and writes the whole back. It is how a control binds to one field of a
// struct held in a single signal.
//
//	name := form.Lens(func(f Form) string { return f.Name }, func(f Form, v string) Form { f.Name = v; return f })
//	ui.TextField(name)
func (s *StateValue[T]) Lens[U any](get func(T) U, set func(T, U) T) *Lens[U] {
	return &Lens[U]{
		get: func() U { return get(s.Get()) },
		set: func(u U) { s.Set(set(Untrack(s.Get), u)) },
	}
}

// Field is Lens for a field that can be addressed: sel receives a copy of
// the whole and returns a pointer to the part, which is both how the part is
// read and where a write goes. It is the common case Lens covers with two
// closures.
//
//	name := form.Field(func(f *Form) *string { return &f.Name })
//	ui.TextField(name)
func (s *StateValue[T]) Field[U any](sel func(*T) *U) *Lens[U] {
	return &Lens[U]{
		get: func() U { v := s.Get(); return *sel(&v) },
		set: func(u U) {
			v := Untrack(s.Get)
			*sel(&v) = u
			s.Set(v)
		},
	}
}

// Get returns the part and subscribes the running Effect.
func (l *Lens[U]) Get() U { return l.get() }

// Set stores the part into the whole.
func (l *Lens[U]) Set(v U) { l.set(v) }

// Update applies fn to the current part and writes it through to the whole.
// Like StateValue.Update it belongs on the UI thread.
func (l *Lens[U]) Update(fn func(U) U) { l.Set(fn(Untrack(l.Get))) }

// GetAny returns the value as any and subscribes, for Sprintf.
func (l *Lens[U]) GetAny() any { return l.Get() }

// anyReader is what Sprintf looks for among its arguments: a reactive value
// read without its type.
type anyReader interface{ GetAny() any }

// GetAny returns the value as any and subscribes, for Sprintf.
func (s *StateValue[T]) GetAny() any { return s.Get() }

// GetAny returns the value as any and subscribes, for Sprintf.
func (m *DerivedValue[T]) GetAny() any { return m.Get() }

// StateValue is a reactive value. Reads inside an Effect subscribe to it; writes
// mark every subscriber dirty so the next frame recomputes them.
type StateValue[T any] struct {
	mu      sync.Mutex
	val     T
	version uint64
	eq      func(a, b T) bool
	subs    map[*effect]struct{}

	// memo is the effect that computes this value, when the signal is a
	// DerivedValue's cell rather than state someone writes.
	memo *effect
}

// State creates a StateValue holding v. T is inferred from the argument, so
// State(0) is a *StateValue[int] and State("") a *StateValue[string]; name it
// explicitly (State[float64](0), State[Widget](nil)) when the literal would
// infer the wrong type or none at all. Writing an equal value is a no-op
// when T has an Equal method or is comparable; see WithEqual to supply
// equality for other types, or to pass nil so that every write notifies.
func State[T any](v T) *StateValue[T] {
	return &StateValue[T]{val: v, eq: comparableEqual[T](), subs: map[*effect]struct{}{}}
}

// equaler is the equality a type declares for itself, as time.Time does.
// A value that has it is compared with it, so a struct holding a slice or a
// map still drops redundant writes.
type equaler[T any] interface{ Equal(T) bool }

// comparableEqual returns the test Set uses to drop redundant writes: the
// type's own Equal method, else == for comparable T, else nil. Interfaces
// are excluded from ==: they compare fine until a dynamic value that does
// not, at which point == panics.
func comparableEqual[T any]() func(a, b T) bool {
	t := reflect.TypeFor[T]()
	if t.Implements(reflect.TypeFor[equaler[T]]()) {
		return func(a, b T) bool {
			// A nil pointer or interface has no receiver to ask.
			av := reflect.ValueOf(&a).Elem()
			if av.Kind() == reflect.Pointer || av.Kind() == reflect.Interface {
				if av.IsNil() {
					return reflect.ValueOf(&b).Elem().IsNil()
				}
			}
			return any(a).(equaler[T]).Equal(b)
		}
	}
	if t.Kind() == reflect.Interface || !t.Comparable() {
		return nil
	}
	return func(a, b T) bool { return any(a) == any(b) }
}

// WithEqual sets the test Set uses to drop redundant writes, and returns s so
// it can be chained onto State. Passing nil makes every write notify.
func (s *StateValue[T]) WithEqual(eq func(a, b T) bool) *StateValue[T] {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eq = eq
	return s
}

// Get returns the current value and subscribes the running Effect, if any.
func (s *StateValue[T]) Get() T {
	checkUIThread("StateValue.Get")
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
	if measuring != nil {
		measuring(s, s.version)
	}
	return s.val
}

// producer implements source: non-nil only for a memo's cell.
func (s *StateValue[T]) producer() *effect { return s.memo }

// markSubsCheck implements downstream: every reader of this memo may now be
// stale. Collected under the lock and marked outside it, as Set does.
func (s *StateValue[T]) markSubsCheck() {
	s.mu.Lock()
	subs := make([]*effect, 0, len(s.subs))
	for e := range s.subs {
		subs = append(subs, e)
	}
	s.mu.Unlock()
	for _, e := range subs {
		markCheck(e)
	}
}

func (s *StateValue[T]) unsubscribe(e *effect) {
	s.mu.Lock()
	delete(s.subs, e)
	s.mu.Unlock()
}

// Set stores v and invalidates every subscriber. A write equal to the current
// value changes nothing and notifies no one.
func (s *StateValue[T]) Set(v T) {
	checkUIThread("StateValue.Set")
	if derivedDepth > 0 {
		panic("ggui: state write inside Derived")
	}
	s.store(v)
}

func (s *StateValue[T]) store(v T) {
	checkUIThread("StateValue.Set")
	s.mu.Lock()
	if s.eq != nil && s.eq(s.val, v) {
		s.mu.Unlock()
		return
	}
	s.val = v
	stateGen++
	s.version++
	layoutGen.Add(1)
	subs := make([]*effect, 0, len(s.subs))
	for e := range s.subs {
		subs = append(subs, e)
	}
	s.mu.Unlock()

	for _, e := range subs {
		markDirty(e)
	}
	if len(subs) > 0 {
		effects.dirtyGen++
	}
}

// Update applies fn to the current value and stores the result. Like Set
// it belongs on the UI thread; it is not an atomic read-modify-write for
// goroutines, which hand their result back with App.Post.
func (s *StateValue[T]) Update(fn func(T) T) {
	s.mu.Lock()
	cur := s.val
	s.mu.Unlock()
	s.Set(fn(cur))
}

// Map returns a DerivedValue holding fn applied to s's value, recomputed whenever s
// changes. It belongs to the current owner: a Reactive or View callback
// disposes its previous computations on replacement, so retaining it leaves a frozen
// value. Without an owner, call Dispose explicitly; App.Close does not own it.
func (s *StateValue[T]) Map[U any](fn func(T) U) *DerivedValue[U] {
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
	ts := Untrack(s.Get)
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

// DerivedValue is a derived value: it recomputes when one of the signals its function
// read changes, and notifies its own readers only when the result differs.
type DerivedValue[T any] struct {
	sig     *StateValue[T]
	eff     *effect
	dispose func()
}

// derivedDepth is how deep the running computation is inside Derived, which
// is what makes a state write in there a panic rather than a silent cycle.
var derivedDepth int

// Derived creates a lazy, read-only value. fn runs on the first Get and the
// first Get after its dependencies change. State writes inside fn panic.
// Computations created under an owner are disposed with it; otherwise call
// Dispose explicitly. WithEqual controls downstream change notification.
func Derived[T any](fn func() T) *DerivedValue[T] {
	m := &DerivedValue[T]{sig: State(*new(T))}
	e := newComputation(func() {
		value := func() T { derivedDepth++; defer func() { derivedDepth-- }(); return fn() }()
		m.sig.store(value)
	}, false)
	m.eff, m.dispose = e, e.dispose
	m.sig.memo, e.cell = e, m.sig
	e.state = stateDirty
	return m
}

// WithEqual controls whether a newly computed value notifies readers.
func (m *DerivedValue[T]) WithEqual(eq func(T, T) bool) *DerivedValue[T] {
	m.sig.WithEqual(eq)
	return m
}

// Map derives a value from any reactive source; StateValue.Map and DerivedValue.Map are
// the same for a source whose type is known.
func Map[T, U any](r Readable[T], fn func(T) U) *DerivedValue[U] {
	return Derived(func() U { return fn(r.Get()) })
}

// Combine derives a value from two reactive sources.
func Combine[A, B, C any](a Readable[A], b Readable[B], fn func(A, B) C) *DerivedValue[C] {
	return Derived(func() C { return fn(a.Get(), b.Get()) })
}

// Get returns the memoized value and subscribes the running Effect, if any.
// A memo whose inputs changed earlier in this frame is recomputed here, so a
// reader never sees one input updated and another not. Outside an effect
// this means fn may run at the call site rather than at the next flush.
func (m *DerivedValue[T]) Get() T {
	checkUIThread("DerivedValue.Get")
	if m.eff.running {
		panic("ggui: cyclic Derived")
	}
	refresh(m.eff)
	return m.sig.Get()
}

// Dispose stops recomputation. Readers keep seeing the last computed value.
func (m *DerivedValue[T]) Dispose() { m.dispose() }

// Map chains another derivation onto m.
func (m *DerivedValue[T]) Map[U any](fn func(T) U) *DerivedValue[U] {
	return Derived(func() U { return fn(m.Get()) })
}

// Watch runs fn with src's value after layout, and after changes. It returns
// a dispose function, like Effect.
func Watch[T any](src Readable[T], fn func(T)) (dispose func()) {
	return Effect(func() Cleanup { fn(src.Get()); return nil })
}

// observe is an immediate internal binding computation, not a user effect.
func observe(fn func()) (dispose func()) {
	_, dispose = effectWith(fn)
	return dispose
}

// Cleanup releases an effect or a mounted resource. It may be nil.
type Cleanup = func()

// Effect schedules a side effect after layout. It belongs to the current
// owner; its cleanup runs untracked before another execution and on disposal.
func Effect(fn func() Cleanup) Cleanup {
	checkUIThread("Effect")
	if currentOwner() == nil {
		panic("ggui: Effect requires an owner; use Component or Root")
	}
	e := newComputation(func() {
		if cleanup := fn(); cleanup != nil {
			OnCleanup(cleanup)
		}
	}, true)
	e.state = stateDirty
	effects.userPending = true
	effects.dirtyGen++
	return e.dispose
}

func newComputation(fn func(), user bool) *effect {
	e := &effect{fn: fn, user: user, origin: effectOrigin()}
	if owner := currentOwner(); owner != nil {
		e.attach(owner)
		e.place(owner, "")
		e.loop = owner.loop
	}
	effects.add(e)
	return e
}

// effectWith runs an internal binding immediately. User effects use a
// separate post-layout phase and never drive widget construction.
func effectWith(fn func()) (*effect, func()) {
	e := newComputation(fn, false)
	ok := false
	defer func() {
		if !ok {
			e.dispose()
		}
	}()
	runEffect(e)
	ok = true
	return e, e.dispose
}

// Root runs fn untracked under a fresh owner that never re-runs, so effects
// and components fn creates live until the returned dispose is called or the
// enclosing owner is disposed. It is how a container keeps children alive
// across its own re-runs; EachKeyed uses it per key.
func Root(fn func()) (dispose func()) {
	return rootWith(nil, "", func() { currentOwner().persistent = true; fn() })
}

// rootWith is Root for a root that is the instance of a keyed component
// (identity) or has a name of its own under its owner (elem), for the
// identities autoID derives.
func rootWith(identity any, elem string, fn func()) (dispose func()) {
	r := &effect{keyRoot: identity, origin: effectOrigin(), persistent: false}
	deps.mu.Lock()
	r.owner = deps.owner
	prevListener, prevOwner := deps.listener, deps.owner
	deps.listener, deps.owner = nil, r
	deps.mu.Unlock()
	if r.owner != nil {
		r.attach(r.owner)
		r.loop = r.owner.loop
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
		panic("ggui: OnCleanup requires an owner")
	}
	if o.disposed {
		withOwner(nil, fn)
		return
	}
	o.cleanups = append(o.cleanups, fn)
}

// Untrack runs fn without subscribing the running Effect to the signals fn
// reads. Effects created inside still belong to the running Effect.
func Untrack[T any](fn func() T) T {
	deps.mu.Lock()
	prev := deps.listener
	deps.listener = nil
	deps.mu.Unlock()
	defer func() {
		deps.mu.Lock()
		deps.listener = prev
		deps.mu.Unlock()
	}()
	return fn()
}

// markDirty records that a signal this effect read has changed, and carries
// check on to the readers of the memo it computes.
func markDirty(e *effect) {
	if e.user {
		effects.userPending = true
	}
	if e.state == stateDirty {
		return
	}
	e.state = stateDirty
	if e.cell != nil {
		e.cell.markSubsCheck()
	}
}

// markCheck records that an input of this effect may have changed. It stops
// at an effect that is already dirty or already checked, so the walk is
// linear and a cycle terminates.
func markCheck(e *effect) {
	if e.user {
		effects.userPending = true
	}
	if e.state != stateClean {
		return
	}
	e.state = stateCheck
	if e.cell != nil {
		e.cell.markSubsCheck()
	}
}

// refresh settles e now: a checked effect first settles the memos it read,
// which makes it dirty if one of them actually changed, and a dirty effect
// re-runs. It reports whether the effect ran, and leaves e clean either way.
func refresh(e *effect) (ran bool) {
	if e.disposed || e.running || e.state == stateClean {
		return false
	}
	if e.state == stateCheck {
		for _, src := range e.sources {
			if p := src.producer(); p != nil {
				refresh(p)
				if e.state == stateDirty {
					break
				}
			}
		}
	}
	if e.state == stateDirty {
		runEffect(e)
		return true
	}
	e.state = stateClean
	return false
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

	e.state, e.running = stateClean, true
	completed := false
	defer func() {
		e.running = false
		if !completed {
			e.state = stateDirty
		}
	}()
	e.fn()
	completed = true
}

type effectSet struct {
	mu          sync.Mutex
	first, last *effect
	count       int

	// Like effect.state, these are confined to the UI thread. A signal
	// write advances dirtyGen; only a quiet flush records settledGen.
	// Internal bindings run immediately; new user effects explicitly mark work.
	dirtyGen, settledGen uint64
	userPending          bool

	// The effects the last pass ran, kept for ErrCycle. The slice is
	// reused, so a settled frame allocates nothing for it.
	lastPass []*effect
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

// unsettled returns the effects the last flush pass ran, which when the
// passes ran out are the ones the cycle turns, with how many effects there
// were in all. Reading the states afterwards would miss half of them: an
// effect in a cycle is clean the moment after it runs and dirty the moment
// after its partner does.
func (s *effectSet) unsettled() (stuck []*effect, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPass, s.count
}

// maxFlushPasses bounds how far a change propagates through derived values in
// one frame. Chains settle in a pass or two; the cap only stops a cycle.
const maxFlushPasses = 16

// flush re-runs every dirty effect, repeating until the tree is quiet so that a
// DerivedValue feeding another effect lands in the same frame. Called once per frame by
// the runtime. It reports false when the effects were still dirty after
// maxFlushPasses, which only a cycle causes.
func (s *effectSet) flush() (settled bool) {
	if s.dirtyGen == s.settledGen {
		return true
	}
	for range maxFlushPasses {
		s.mu.Lock()
		list := make([]*effect, 0, s.count)
		for e := s.first; e != nil; e = e.nextEffect {
			list = append(list, e)
		}
		s.mu.Unlock()

		ran := false
		s.lastPass = s.lastPass[:0]
		for _, e := range list {
			if !e.user && e.cell == nil && refresh(e) {
				ran = true
				s.lastPass = append(s.lastPass, e)
			}
		}
		if !ran {
			// Read after the quiet pass: cleanup and nested flushes may
			// have written signals while earlier passes were running.
			s.settledGen = s.dirtyGen
			return true
		}
	}
	return false
}

// flushUsers executes one post-layout pass. Layout is settled again before
// another pass, so effects never observe a partially mounted tree.
func (s *effectSet) flushUsers(loop any) bool {
	if !s.userPending {
		return false
	}
	s.userPending = false
	var pending []*effect
	for e := s.first; e != nil; e = e.nextEffect {
		if e.user && e.state != stateClean {
			if e.loop == nil || e.loop == loop {
				pending = append(pending, e)
			} else {
				s.userPending = true
			}
		}
	}
	ran := false
	s.lastPass = s.lastPass[:0]
	for _, e := range pending {
		if refresh(e) {
			ran = true
			s.lastPass = append(s.lastPass, e)
		}
	}
	return ran
}

func (s *StateValue[T]) layoutVersion() uint64 {
	if s.memo != nil {
		refresh(s.memo)
	}
	return s.version
}
