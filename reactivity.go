package ggui

import "github.com/ironpark/ggui/internal/reactive"

// The reactive core lives in internal/reactive: signals, effects, memos and
// the ownership tree, which need nothing from widgets or painting. It is
// reached through the names it has always had, so that ggui.State and
// ggui.Effect keep meaning what they did.
//
// Generic types can be aliased and bring their methods with them. Generic
// functions cannot, so those are one-line forwards, as ggui/a11y does for
// the geometry constructors.

type (
	// Readable is anything a computation can read and subscribe to.
	Readable[T any] = reactive.Readable[T]
	// Writable is anything a caller can write.
	Writable[T any] = reactive.Writable[T]
	// Binding is both, as a widget takes for two-way state.
	Binding[T any] = reactive.Binding[T]
	// StateValue is a signal: the unit of reactive state.
	StateValue[T any] = reactive.StateValue[T]
	// DerivedValue is a memo: a value computed from other signals.
	DerivedValue[T any] = reactive.DerivedValue[T]
	// Lens is a two-way view onto part of a StateValue.
	Lens[U any] = reactive.Lens[U]
	// Cleanup is what an Effect returns to undo itself.
	Cleanup = reactive.Cleanup
)

// State returns a new signal holding v.
func State[T any](v T) *StateValue[T] { return reactive.State(v) }

// Derived returns a memo of fn, recomputed when what it reads changes.
func Derived[T any](fn func() T) *DerivedValue[T] { return reactive.Derived(fn) }

// Map returns a memo of fn applied to r.
func Map[T, U any](r Readable[T], fn func(T) U) *DerivedValue[U] { return reactive.Map(r, fn) }

// Combine returns a memo of fn applied to a and b.
func Combine[A, B, C any](a Readable[A], b Readable[B], fn func(A, B) C) *DerivedValue[C] {
	return reactive.Combine(a, b, fn)
}

// Watch runs fn whenever src changes, and returns a function to stop.
func Watch[T any](src Readable[T], fn func(T)) (dispose func()) { return reactive.Watch(src, fn) }

// Effect runs fn now and again whenever what it read changes.
func Effect(fn func() Cleanup) Cleanup { return reactive.Effect(fn) }

// Root runs fn under a fresh owner and returns a function to dispose it.
func Root(fn func()) (dispose func()) { return reactive.Root(fn) }

// OnCleanup registers fn to run when the current owner is disposed.
func OnCleanup(fn func()) { reactive.OnCleanup(fn) }

// Untrack runs fn without subscribing the running Effect to what it reads.
func Untrack[T any](fn func() T) T { return reactive.Untrack(fn) }

// Toggle inverts a boolean signal.
func Toggle(s Writable[bool]) { reactive.Toggle(s) }

// Add increments a numeric signal by d.
func Add[N Number](s Writable[N], d N) { reactive.Add(s, d) }

// Append appends items to a slice signal.
func Append[T any](s Writable[[]T], items ...T) { reactive.Append(s, items...) }

// Remove drops the elements of a slice signal for which drop returns true.
func Remove[T any](s Writable[[]T], drop func(T) bool) { reactive.Remove(s, drop) }

// Const returns a Readable that never changes.
func Const[T any](v T) Readable[T] { return reactive.Const(v) }

// Bind adapts a getter and setter pair into a Binding, for controls that
// edit state owned by a model method rather than a signal.
func Bind[T any](get func() T, set func(T)) Binding[T] { return &funcBinding[T]{get, set} }

type funcBinding[T any] struct {
	get func() T
	set func(T)
}

func (b *funcBinding[T]) Get() T  { return b.get() }
func (b *funcBinding[T]) Set(v T) { b.set(v) }
