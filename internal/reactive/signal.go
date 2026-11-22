package reactive

import (
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/ironpark/ggui/pkg/layout"
)

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

// AnyReader is what Sprintf looks for among its arguments: a reactive value
// read without its type.
type AnyReader interface{ GetAny() any }

// Format formats like fmt.Sprintf, with each argument that is an AnyReader
// replaced by the value it holds now. Read inside a computation, those
// arguments are tracked like any other read.
func Format(format string, args []any) string {
	vals := make([]any, len(args))
	for i, a := range args {
		if r, ok := a.(AnyReader); ok {
			vals[i] = r.GetAny()
		} else {
			vals[i] = a
		}
	}
	return fmt.Sprintf(format, vals...)
}

// GetAny returns the value as any and subscribes, for Sprintf.
func (s *StateValue[T]) GetAny() any { return s.Get() }

// StateValue is a reactive value. Reads inside an Effect subscribe to it; writes
// mark every subscriber dirty so the next frame recomputes them.
type StateValue[T any] struct {
	// The runtime the value was made in, in a ggui_debug build, for the
	// UI-goroutine check; empty, and first so as not to be padded,
	// otherwise. See thread_debug.go.
	valueThread
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
	return &StateValue[T]{valueThread: madeHere(), val: v, eq: ComparableEqual[T](), subs: map[*Computation]struct{}{}}
}

// equaler is the equality a type declares for itself, as time.Time does.
// A value that has it is compared with it, so a struct holding a slice or a
// map still drops redundant writes.
type equaler[T any] interface{ Equal(T) bool }

// ComparableEqual returns the test Set uses to drop redundant writes: the
// type's own Equal method, else == for comparable T containing no interface
// fields, else nil. Interfaces are excluded from ==: they compare fine until
// holding a dynamic value that does not, at which point == panics.
func ComparableEqual[T any]() func(a, b T) bool {
	t := reflect.TypeFor[T]()
	if t.Implements(reflect.TypeFor[equaler[T]]()) {
		if k := t.Kind(); k == reflect.Pointer || k == reflect.Interface {
			return func(a, b T) bool {
				// A nil pointer or interface has no receiver to ask.
				av := reflect.ValueOf(&a).Elem()
				if av.IsNil() {
					return reflect.ValueOf(&b).Elem().IsNil()
				}
				return any(a).(equaler[T]).Equal(b)
			}
		}
		return func(a, b T) bool {
			return any(a).(equaler[T]).Equal(b)
		}
	}
	if !safeComparable(t) {
		return nil
	}
	return func(a, b T) bool { return any(a) == any(b) }
}

// safeComparable reports whether == on two values of t can never panic at
// runtime: t is comparable and contains no interface type, directly or inside
// structs or arrays.
func safeComparable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128,
		reflect.String, reflect.Pointer, reflect.UnsafePointer, reflect.Chan:
		return true
	case reflect.Array:
		return safeComparable(t.Elem())
	case reflect.Struct:
		for field := range t.Fields() {
			if !safeComparable(field.Type) {
				return false
			}
		}
		return true
	default:
		return false
	}
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
	s.checkThread("StateValue.Get")
	sc := current()
	e := sc.listener
	if Debug && e == nil && sc.building {
		reportSnapshot()
	}

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
	// This recurses down a chain of memos, so each level copies into an
	// array on its own stack: a buffer shared through the scope would be
	// taken by the outermost level and leave every inner one allocating.
	var small [8]*Computation
	s.mu.Lock()
	subs := s.copySubs(small[:0])
	s.mu.Unlock()
	for _, e := range subs {
		markCheck(e)
	}
}

// copySubs appends the subscribers to buf, under the lock the caller
// holds. Marking happens after the lock is released, as marking another
// value's readers takes that value's lock.
func (s *StateValue[T]) copySubs(buf []*Computation) []*Computation {
	for e := range s.subs {
		buf = append(buf, e)
	}
	return buf
}

func (s *StateValue[T]) unsubscribe(e *Computation) {
	s.mu.Lock()
	delete(s.subs, e)
	s.mu.Unlock()
}

// Set stores v and invalidates every subscriber. A write equal to the current
// value changes nothing and notifies no one.
func (s *StateValue[T]) Set(v T) {
	sc := current()
	if sc.derivedDepth > 0 {
		panic("ggui: state write inside Derived")
	}
	s.store(sc, v)
}

// swapLocked replaces s.val under s.mu and snapshots subscribers, using
// defer Unlock so a panicking s.eq never leaves s.mu locked.
func (s *StateValue[T]) swapLocked(sc *scope, v T) (subs []*Computation, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eq != nil && s.eq(s.val, v) {
		return nil, false
	}
	s.val = v
	s.version++
	// The copy goes into the scope's scratch buffer, taken for the length
	// of the marking, so a write reaching a thousand readers allocates
	// nothing. Marking runs no user code, but a write it somehow led to
	// would find the buffer gone and make its own.
	subs = s.copySubs(sc.subsScratch[:0])
	sc.subsScratch = nil
	return subs, true
}

// store is Set without the Derived check, for a memo storing its value; sc
// is the running goroutine's scope.
func (s *StateValue[T]) store(sc *scope, v T) {
	s.checkThread("StateValue.Set")
	subs, changed := s.swapLocked(sc, v)
	if !changed {
		return
	}
	var onFree func()
	// Lazy Derived recomputation derives from upstream StateValues that
	// already bumped stateGen/layoutGen/freeGen and triggered onFree/noteWrite.
	if s.memo == nil {
		sc.stateGen = genSeq.Add(1)
		if sc.writes != nil {
			noteWrite(sc.writes)
		}
		sc.layoutGen = sc.stateGen
		if sc.contained == 0 {
			sc.freeGen = sc.stateGen
			onFree = sc.onFree
		}
	}

	for _, e := range subs {
		markDirty(e)
	}
	clear(subs)
	sc.subsScratch = subs[:0]
	if onFree != nil {
		onFree()
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

func (s *StateValue[T]) LayoutVersion() uint64 {
	if s.memo != nil {
		Refresh(s.memo)
	}
	return s.version
}

// Toggle flips a writable boolean.
func Toggle(s Writable[bool]) { s.Update(func(b bool) bool { return !b }) }

// Add adds d to a writable number.
func Add[N layout.Number](s Writable[N], d N) { s.Update(func(n N) N { return n + d }) }

// Append adds items to a writable slice, in a new slice so the
// change is noticed.
func Append[T any](s Writable[[]T], items ...T) {
	s.Update(func(ts []T) []T { return append(slices.Clip(ts), items...) })
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

// Untrack runs fn without subscribing the running Effect to the signals fn
// reads. Effects created inside still belong to the running Effect.
func Untrack[T any](fn func() T) T {
	sc := current()
	if sc.listener == nil && !sc.building {
		return fn()
	}
	return untracked(sc, false, fn)
}

// Build runs fn, which builds widgets, untracked. A signal read directly in
// it is a snapshot the widgets it builds never see change, which is almost
// never what was meant; a ggui_debug build reports each place that does,
// once. A read inside Untrack is a snapshot asked for, and a computation fn
// creates, such as a Derived or an Observe, tracks its own reads as usual.
func Build[T any](fn func() T) T { return untracked(current(), true, fn) }

// untracked runs fn with no listener, building or not.
func untracked[T any](sc *scope, building bool, fn func() T) T {
	prev, wasBuilding := sc.listener, sc.building
	sc.listener, sc.building = nil, building
	defer func() { sc.listener, sc.building = prev, wasBuilding }()
	return fn()
}
