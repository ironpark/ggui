package reactive

import (
	"reflect"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ironpark/ggui/geom"
)

// A goroutine's layout generation counts the changes on it that can move
// something on screen: every StateValue write and every Invalidate. The
// runtime lays a tree out again only when it has advanced, or the window
// changed size, and paints every frame regardless. Its state generation
// counts the writes alone, so that settle can tell a write that happened
// during a layout and go round again.
//
// Both are kept per goroutine, in its scope: a runtime's signals are
// written on the goroutine that runs its frames, and a write a probe on
// another goroutine makes must neither lay this one out again nor make its
// settle go round. A change that concerns every tree, such as a new default
// font, goes to globalLayoutGen instead. Every generation is drawn from
// genSeq, so no two are equal.
var genSeq, globalLayoutGen atomic.Uint64

// RequestLayout asks the runtime to lay out the trees whose frames run on
// this goroutine again next frame. It belongs on the UI goroutine, as a
// signal write does.
func RequestLayout() { current().layoutGen = genSeq.Add(1) }

// RequestLayoutEverywhere asks for every tree on every goroutine to be laid
// out again, for a change that reaches them all, such as the default font.
func RequestLayoutEverywhere() { globalLayoutGen.Store(genSeq.Add(1)) }

// Measurement dependencies are separate from reactive subscriptions: even an
// Untrack read affects layout, but never subscribes the enclosing computation.
type LayoutSource interface{ LayoutVersion() uint64 }

// The running computation, the owner new computations join and the layout
// recorder a read reports to are the running goroutine's; see scope. This
// mirrors Svelte's automatic dependency tracking and Solid's ownership
// tree: nothing is declared, reads and creations are observed.

// source is anything an effect can subscribe to.
type source interface {
	unsubscribe(e *Computation)
	// producer is the memo Computation that computes this source, or nil for a
	// plain signal: how Refresh finds the upstream to settle first.
	producer() *Computation
}

// downstream is a memo's value seen from its Computation: the readers to carry
// staleness on to.
type downstream interface{ markSubsCheck() }

// An Computation is clean, or it may be stale. A StateValue write marks its direct
// subscribers dirty and everything downstream of a memo check: check means
// "an input of yours may have changed", and is resolved by refreshing the
// upstream memos, which turns it into dirty or back into clean. It is what
// keeps a reader from seeing one input updated and another not.
const (
	stateClean uint8 = iota
	stateCheck
	stateDirty
)

type Computation struct {
	fn         func()
	state      uint8
	user       bool
	persistent bool
	// rt is the Runtime whose flushes run this Computation: its owner's, or
	// the creating goroutine's Base for one created with no owner.
	rt *Runtime
	// loop is the frame loop this Computation belongs to, held only so that
	// flushUsers can tell one loop's effects from another's. It is never
	// dereferenced here, so the reactive core needs no frame loop type.
	loop     any
	running  bool
	disposed bool

	// cell is the memo's value this Computation computes, if it is a memo's:
	// what carries staleness on to the memo's own readers.
	cell downstream

	owner                    *Computation
	firstChild, lastChild    *Computation
	prevSibling, nextSibling *Computation
	cleanups                 []func()

	// Registration and sibling links preserve creation order while allowing
	// a disposed computation to leave both lists in constant time.
	prevEffect, nextEffect *Computation
	registered             bool
	sources                []source

	// Where this Computation was created, in the ggui_debug build only: what
	// lets ErrCycle name the effects a cycle is made of. Empty otherwise.
	origin string

	// Identity for what is constructed under this Computation: keyRoot is the
	// identity of the mounted component or block this Computation is the root of, path
	// is the Computation's place under the nearest such root, by construction
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

// AutoID returns the identity for a widget being constructed now: nil
// outside a keyed component.
func AutoID() any {
	o := CurrentOwner()
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
	k := autoKey{root: root, path: o.path + "/" + strconv.Itoa(o.seq)}
	o.seq++
	return k
}

// place gives e its path under owner: owner's path plus e's construction
// ordinal, or elem when the caller names it, as EachKeyed does with the item key.
func (e *Computation) place(owner *Computation, elem string) {
	if owner == nil {
		return
	}
	if elem == "" {
		elem = strconv.Itoa(owner.seq)
		owner.seq++
	}
	e.path = owner.path + "/" + elem
}

// attach appends a child to its owner's ordered list.
func (e *Computation) attach(owner *Computation) {
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
func (e *Computation) detach() {
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
func (e *Computation) reset() {
	var children []*Computation
	for child := e.firstChild; child != nil; child = child.nextSibling {
		if e.disposed || !child.persistent {
			children = append(children, child)
		}
	}
	for _, child := range children {
		child.dispose()
	}

	WithOwner(nil, func() {
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

func (e *Computation) dispose() {
	if e.disposed {
		return
	}
	e.disposed = true
	e.detach()
	e.reset()
	e.rt.effects.remove(e)
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

// AnyReader is what Sprintf looks for among its arguments: a reactive value
// read without its type.
type AnyReader interface{ GetAny() any }

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
	subs    map[*Computation]struct{}

	// memo is the Computation that computes this value, when the signal is a
	// DerivedValue's cell rather than state someone writes.
	memo *Computation
}

// State creates a StateValue holding v. T is inferred from the argument, so
// State(0) is a *StateValue[int] and State("") a *StateValue[string]; name it
// explicitly (State[float64](0), State[Widget](nil)) when the literal would
// infer the wrong type or none at all. Writing an equal value is a no-op
// when T has an Equal method or is comparable; see WithEqual to supply
// equality for other types, or to pass nil so that every write notifies.
func State[T any](v T) *StateValue[T] {
	return &StateValue[T]{val: v, eq: ComparableEqual[T](), subs: map[*Computation]struct{}{}}
}

// equaler is the equality a type declares for itself, as time.Time does.
// A value that has it is compared with it, so a struct holding a slice or a
// map still drops redundant writes.
type equaler[T any] interface{ Equal(T) bool }

// ComparableEqual returns the test Set uses to drop redundant writes: the
// type's own Equal method, else == for comparable T, else nil. Interfaces
// are excluded from ==: they compare fine until a dynamic value that does
// not, at which point == panics.
func ComparableEqual[T any]() func(a, b T) bool {
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
	CheckUIThread("StateValue.Get")
	sc := current()
	e := sc.listener

	s.mu.Lock()
	defer s.mu.Unlock()
	if e != nil && !e.disposed {
		if _, ok := s.subs[e]; !ok {
			s.subs[e] = struct{}{}
			e.sources = append(e.sources, s)
		}
	}
	if sc.measuring != nil {
		sc.measuring(s, s.version)
	}
	return s.val
}

// producer implements source: non-nil only for a memo's cell.
func (s *StateValue[T]) producer() *Computation { return s.memo }

// markSubsCheck implements downstream: every reader of this memo may now be
// stale. Collected under the lock and marked outside it, as Set does.
func (s *StateValue[T]) markSubsCheck() {
	s.mu.Lock()
	subs := make([]*Computation, 0, len(s.subs))
	for e := range s.subs {
		subs = append(subs, e)
	}
	s.mu.Unlock()
	for _, e := range subs {
		markCheck(e)
	}
}

func (s *StateValue[T]) unsubscribe(e *Computation) {
	s.mu.Lock()
	delete(s.subs, e)
	s.mu.Unlock()
}

// Set stores v and invalidates every subscriber. A write equal to the current
// value changes nothing and notifies no one.
func (s *StateValue[T]) Set(v T) {
	CheckUIThread("StateValue.Set")
	if current().derivedDepth > 0 {
		panic("ggui: state write inside Derived")
	}
	s.store(v)
}

func (s *StateValue[T]) store(v T) {
	CheckUIThread("StateValue.Set")
	s.mu.Lock()
	if s.eq != nil && s.eq(s.val, v) {
		s.mu.Unlock()
		return
	}
	s.val = v
	sc := current()
	sc.stateGen = genSeq.Add(1)
	s.version++
	sc.layoutGen = sc.stateGen
	subs := make([]*Computation, 0, len(s.subs))
	for e := range s.subs {
		subs = append(subs, e)
	}
	s.mu.Unlock()

	for _, e := range subs {
		markDirty(e)
		e.rt.effects.dirtyGen++
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
func Add[N geom.Number](s Writable[N], d N) { s.Update(func(n N) N { return n + d }) }

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
	eff     *Computation
	dispose func()
}

// Derived creates a lazy, read-only value. fn runs on the first Get and the
// first Get after its dependencies change. State writes inside fn panic.
// Computations created under an owner are disposed with it; otherwise call
// Dispose explicitly. WithEqual controls downstream change notification.
func Derived[T any](fn func() T) *DerivedValue[T] {
	m := &DerivedValue[T]{sig: State(*new(T))}
	e := newComputation(func() {
		// How deep the goroutine is inside Derived is what makes a state
		// write in there a panic rather than a silent cycle.
		value := func() T {
			sc := current()
			sc.derivedDepth++
			defer func() { sc.derivedDepth-- }()
			return fn()
		}()
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
	CheckUIThread("DerivedValue.Get")
	if m.eff.running {
		panic("ggui: cyclic Derived")
	}
	Refresh(m.eff)
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

// Observe runs fn now and again when what it read changes, without making
// it a user effect. It is Effect without the post-layout pass.
func Observe(fn func()) (dispose func()) {
	_, dispose = EffectWith(fn)
	return dispose
}

// Cleanup releases an effect or a mounted resource. It may be nil.
type Cleanup = func()

// Effect schedules a side effect after layout. It belongs to the current
// owner; its cleanup runs untracked before another execution and on disposal.
func Effect(fn func() Cleanup) Cleanup {
	CheckUIThread("Effect")
	if CurrentOwner() == nil {
		panic("ggui: Effect requires an owner; use Component or Root")
	}
	e := newComputation(func() {
		if cleanup := fn(); cleanup != nil {
			OnCleanup(cleanup)
		}
	}, true)
	e.state = stateDirty
	e.rt.effects.userPending = true
	e.rt.effects.dirtyGen++
	return e.dispose
}

func newComputation(fn func(), user bool) *Computation {
	e := &Computation{fn: fn, user: user, origin: effectOrigin()}
	if owner := CurrentOwner(); owner != nil {
		e.attach(owner)
		e.place(owner, "")
		e.loop, e.rt = owner.loop, owner.rt
	} else {
		e.rt = Base()
	}
	e.rt.effects.add(e)
	return e
}

// EffectWith runs an internal binding immediately. User effects use a
// separate post-layout phase and never drive widget construction.
func EffectWith(fn func()) (*Computation, func()) {
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
	return RootWith(nil, "", func() { CurrentOwner().persistent = true; fn() })
}

// RootWith is Root for a root that is the instance of a keyed component
// (identity) or has a name of its own under its owner (elem), for the
// identities AutoID derives.
func RootWith(identity any, elem string, fn func()) (dispose func()) {
	return rootIn(nil, identity, elem, fn)
}

// Root is the package Root for a root whose computations rt flushes,
// whatever the current owner's runtime: how a frame loop gives the tree it
// builds a graph of its own.
func (rt *Runtime) Root(fn func()) (dispose func()) {
	return rootIn(rt, nil, "", func() { CurrentOwner().persistent = true; fn() })
}

// rootIn is RootWith with the root's runtime chosen: rt, or when nil the
// owner's, or the goroutine's Base.
func rootIn(rt *Runtime, identity any, elem string, fn func()) (dispose func()) {
	r := &Computation{keyRoot: identity, origin: effectOrigin(), persistent: false}
	sc, prevListener, prevOwner := enter(nil, r)
	r.owner = prevOwner
	if r.owner != nil {
		r.attach(r.owner)
		r.loop, r.rt = r.owner.loop, r.owner.rt
		r.place(r.owner, elem)
	}
	switch {
	case rt != nil:
		r.rt = rt
	case r.rt == nil:
		r.rt = Base()
	}
	ok := false
	defer func() {
		sc.listener, sc.owner = prevListener, prevOwner
		if !ok {
			r.dispose()
		}
	}()
	fn()
	ok = true
	return r.dispose
}

// WithOwner runs fn with owner as the current owner and no listener.
func WithOwner(owner *Computation, fn func()) {
	sc, prevListener, prevOwner := enter(nil, owner)
	defer func() { sc.listener, sc.owner = prevListener, prevOwner }()
	fn()
}

// CurrentOwner returns the running goroutine's owner, or nil.
func CurrentOwner() *Computation { return current().owner }

// OnCleanup registers fn to run before the enclosing Effect re-runs and when
// it is disposed. Call it from inside an Effect, a Builder or a Component
// setup, for timers, subscriptions and anything else that must be undone.
func OnCleanup(fn func()) {
	o := CurrentOwner()
	if o == nil {
		panic("ggui: OnCleanup requires an owner")
	}
	if o.disposed {
		WithOwner(nil, fn)
		return
	}
	o.cleanups = append(o.cleanups, fn)
}

// Untrack runs fn without subscribing the running Effect to the signals fn
// reads. Effects created inside still belong to the running Effect.
func Untrack[T any](fn func() T) T {
	sc := current()
	if sc.listener == nil {
		return fn()
	}
	prev := sc.listener
	sc.listener = nil
	defer func() { sc.listener = prev }()
	return fn()
}

// markDirty records that a signal this Computation read has changed, and carries
// check on to the readers of the memo it computes.
func markDirty(e *Computation) {
	if e.user {
		e.rt.effects.userPending = true
	}
	if e.state == stateDirty {
		return
	}
	e.state = stateDirty
	e.rt.effects.dirtyGen++
	if e.cell != nil {
		e.cell.markSubsCheck()
	}
}

// markCheck records that an input of this Computation may have changed. It stops
// at an effect that is already dirty or already checked, so the walk is
// linear and a cycle terminates.
func markCheck(e *Computation) {
	if e.user {
		e.rt.effects.userPending = true
	}
	if e.state != stateClean {
		return
	}
	e.state = stateCheck
	// A reader in another runtime learns of the change only through this
	// mark, so its flush must not take itself for settled.
	e.rt.effects.dirtyGen++
	if e.cell != nil {
		e.cell.markSubsCheck()
	}
}

// Refresh settles e now: a checked Computation first settles the memos it read,
// which makes it dirty if one of them actually changed, and a dirty Computation
// re-runs. It reports whether the Computation ran, and leaves e clean either way.
func Refresh(e *Computation) (ran bool) {
	if e.disposed || e.running || e.state == stateClean {
		return false
	}
	if e.state == stateCheck {
		for _, src := range e.sources {
			if p := src.producer(); p != nil {
				Refresh(p)
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

func runEffect(e *Computation) {
	e.reset()
	e.seq = 0

	sc, prevListener, prevOwner := enter(e, e)
	defer func() { sc.listener, sc.owner = prevListener, prevOwner }()

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
	first, last *Computation
	count       int

	// Like Computation.state, these are confined to the UI thread. A signal
	// write advances dirtyGen; only a quiet flush records settledGen.
	// Internal bindings run immediately; new user effects explicitly mark work.
	dirtyGen, settledGen uint64
	userPending          bool

	// The effects the last pass ran, kept for ErrCycle. The slice is
	// reused, so a settled frame allocates nothing for it.
	lastPass []*Computation
}

// Runtime is one reactive graph's scheduler: the computations it owns and
// the bookkeeping that tells a flush whether any of them is stale. Signals
// belong to no runtime; a write marks each reader in the reader's own.
//
// A computation belongs to its owner's runtime, and one created with no
// owner to its goroutine's Base. A frame loop builds its tree under
// Runtime.Root, so what the tree creates is flushed by that loop alone, and
// a loop that is never closed leaves nothing behind in anyone else's frames.
//
// A runtime runs on one goroutine at a time. Two runtimes may run on two
// goroutines at once, as probes under t.Parallel do, as long as nothing one
// of them flushes is written from the other: a signal written by one
// goroutine and read by another's computations is a race, runtimes or not.
type Runtime struct {
	effects effectSet

	// base is the Base of the goroutine that made this runtime, which holds
	// what was built there before the runtime's tree: a widget a test built
	// before its probe. Nil for a Base itself.
	base *Runtime

	// Host is whatever the package driving this runtime keeps beside it.
	// The root package keeps its animations and frame clock here.
	Host any
}

// NewRuntime returns an empty Runtime that flushes the running goroutine's
// Base along with its own computations.
func NewRuntime() *Runtime { return &Runtime{base: Base()} }

// Related returns the runtimes a flush of rt covers: rt itself, the Base it
// was made beside, and the running goroutine's Base, which holds what a
// handler running in rt's frame built with no owner. The array is padded
// with nil, so that a frame asking allocates nothing.
func (rt *Runtime) Related() (out [3]*Runtime) {
	out[0] = rt
	n := 1
	for _, x := range [...]*Runtime{rt.base, Base()} {
		if x != nil && x != out[0] && x != out[1] {
			out[n] = x
			n++
		}
	}
	return out
}

// Parent is the Base rt was made beside, or nil for a Base.
func (rt *Runtime) Parent() *Runtime { return rt.base }

// sets fills buf with the effect sets a flush of rt runs, and returns them;
// see Related.
func (rt *Runtime) sets(buf *[3]*effectSet) []*effectSet {
	out := buf[:0]
	for _, x := range rt.Related() {
		if x != nil {
			out = append(out, &x.effects)
		}
	}
	return out
}

func (s *effectSet) add(e *Computation) {
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

func (s *effectSet) remove(e *Computation) {
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
// Computation in a cycle is clean the moment after it runs and dirty the moment
// after its partner does.
func (s *effectSet) unsettled() (stuck []*Computation, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPass, s.count
}

func (s *effectSet) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// MaxFlushPasses bounds how far a change propagates through derived values in
// one frame. Chains settle in a pass or two; the cap only stops a cycle.
const MaxFlushPasses = 16

// settled reports whether every write this set was told of has been
// flushed.
func (s *effectSet) settled() bool { return s.dirtyGen == s.settledGen }

// snapshot lists the registered computations, oldest first.
func (s *effectSet) snapshot() []*Computation {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]*Computation, 0, s.count)
	for e := s.first; e != nil; e = e.nextEffect {
		list = append(list, e)
	}
	return list
}

// flush re-runs every dirty Computation of sets, repeating until they are
// all quiet so that a DerivedValue feeding another Computation lands in the
// same frame. Called once per frame by the runtime. It reports false when
// the effects were still dirty after MaxFlushPasses, which only a cycle
// causes.
func flush(sets []*effectSet) (settled bool) {
	if allSettled(sets) {
		return true
	}
	for range MaxFlushPasses {
		ran := false
		for _, s := range sets {
			s.lastPass = s.lastPass[:0]
			for _, e := range s.snapshot() {
				if !e.user && e.cell == nil && Refresh(e) {
					ran = true
					s.lastPass = append(s.lastPass, e)
				}
			}
		}
		if !ran {
			// Read after the quiet pass: cleanup and nested flushes may
			// have written signals while earlier passes were running.
			for _, s := range sets {
				s.settledGen = s.dirtyGen
			}
			return true
		}
	}
	return false
}

func allSettled(sets []*effectSet) bool {
	for _, s := range sets {
		if !s.settled() {
			return false
		}
	}
	return true
}

// flushUsers executes one post-layout pass over sets. Layout is settled
// again before another pass, so effects never observe a partially mounted
// tree.
func flushUsers(sets []*effectSet, loop any) bool {
	ran := false
	for _, s := range sets {
		if !s.userPending {
			continue
		}
		s.userPending = false
		var pending []*Computation
		for e := s.first; e != nil; e = e.nextEffect {
			if e.user && e.state != stateClean {
				if e.loop == nil || e.loop == loop {
					pending = append(pending, e)
				} else {
					s.userPending = true
				}
			}
		}
		s.lastPass = s.lastPass[:0]
		for _, e := range pending {
			if Refresh(e) {
				ran = true
				s.lastPass = append(s.lastPass, e)
			}
		}
	}
	return ran
}

func (s *StateValue[T]) LayoutVersion() uint64 {
	if s.memo != nil {
		Refresh(s.memo)
	}
	return s.version
}

// The frame loop drives the reactive core from the root package. These are
// the entry points it needs; everything else here stays private.

// Disposed reports whether this computation's owner has been torn down.
func (e *Computation) Disposed() bool { return e.disposed }

// Flush runs the pending effects of rt and the runtimes it covers until
// quiet, reporting whether they settled.
func (rt *Runtime) Flush() bool {
	var buf [3]*effectSet
	return flush(rt.sets(&buf))
}

// FlushUsers runs one post-layout pass of the user effects of rt and the
// runtimes it covers that belong to loop, reporting whether any ran.
func (rt *Runtime) FlushUsers(loop any) bool {
	var buf [3]*effectSet
	return flushUsers(rt.sets(&buf), loop)
}

// Unsettled names the effects the last pass of rt's flush ran, for
// ErrCycle, with how many effects the runtimes it covers hold in all.
func (rt *Runtime) Unsettled() (stuck []*Computation, total int) {
	var buf [3]*effectSet
	for _, s := range rt.sets(&buf) {
		pass, n := s.unsettled()
		stuck, total = append(stuck, pass...), total+n
	}
	return stuck, total
}

// Settled reports whether every write reaching rt or a runtime it covers
// has been flushed.
func (rt *Runtime) Settled() bool {
	var buf [3]*effectSet
	return allSettled(rt.sets(&buf))
}

// Count is how many effects rt holds, for a test asserting that a
// construction or teardown left none behind.
func (rt *Runtime) Count() int { return rt.effects.len() }

// Flush is Base().Flush, for the running goroutine.
func Flush() bool { return Base().Flush() }

// FlushUsers is Base().FlushUsers, for the running goroutine.
func FlushUsers(loop any) bool { return Base().FlushUsers(loop) }

// Unsettled is Base().Unsettled, for the running goroutine.
func Unsettled() (stuck []*Computation, total int) { return Base().Unsettled() }

// Settled is Base().Settled, for the running goroutine.
func Settled() bool { return Base().Settled() }

// StateGen counts StateValue writes, so that a frame can tell whether one
// happened while it was laying out.
func StateGen() uint64 { return current().stateGen }

// LayoutGen changes whenever something that can move a tree laid out on
// this goroutine changes: a RequestLayout here or a RequestLayoutEverywhere.
// Both parts only grow, so their sum changes whenever either does.
func LayoutGen() uint64 { return current().layoutGen + globalLayoutGen.Load() }

// Origin is where this computation was created, in a ggui_debug build, for
// the ErrCycle message the frame loop builds.
func (e *Computation) Origin() string { return e.origin }

// Derived reports whether this computation computes a memo's value.
func (e *Computation) Derived() bool { return e.cell != nil }

// Loop and SetLoop carry the frame loop an effect belongs to. It is opaque
// here: the reactive core only ever compares one against another.
func (e *Computation) Loop() any     { return e.loop }
func (e *Computation) SetLoop(v any) { e.loop = v }

// Children lists this computation's live children, oldest first. The
// ownership tree is private; this is for a caller that only needs to see
// what is still attached.
func (e *Computation) Children() []*Computation {
	var out []*Computation
	for c := e.firstChild; c != nil; c = c.nextSibling {
		out = append(out, c)
	}
	return out
}

// Count is Base().Count, for the running goroutine.
func Count() int { return Base().Count() }
