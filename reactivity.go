package ggui

import (
	"github.com/ironpark/ggui/internal/reactive"
)

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

// Peek returns r's current value without subscribing anything to it, as
// Untrack(r.Get) does: for an action reading state it does not show, or a
// builder taking a snapshot on purpose.
func Peek[T any](r Readable[T]) T { return reactive.Untrack(r.Get) }

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
// edit state owned by a model method rather than a signal. The getter
// should read a signal, through the model's StateValue, Lens or Field:
// one that reads plain storage, or a copy captured when the control was
// built, leaves the control showing a stale value, and a ggui_debug build
// says so the first time it runs. State.Field and State.Lens bind part of
// a struct directly.
func Bind[T any](get func() T, set func(T)) Binding[T] {
	return &funcBinding[T]{get: get, set: set, origin: reactive.Origin()}
}

type funcBinding[T any] struct {
	get     func() T
	set     func(T)
	origin  string // where Bind was called, in a ggui_debug build
	checked bool
}

func (b *funcBinding[T]) Get() T {
	if !reactive.Debug || b.checked {
		return b.get()
	}
	b.checked = true
	return countReads(b.origin, b.get)
}

func (b *funcBinding[T]) Set(v T) { b.set(v) }

// Controlled binds a control to value, a copy taken when the view was
// built, and asks set for a change instead of making it. It is for a
// control whose change the model may refuse, such as a mode switch that a
// form with an invalid entry blocks: a change set takes rebuilds the view
// with the new value, and one it refuses leaves the control showing value.
// Unlike Bind, it reads no signal on purpose, so the view around it must
// rebuild when the model changes.
//
//	ggui.View(mode, func(m int) ggui.Widget {
//		return ui.ToggleGroup(ggui.Controlled(m, model.SwitchMode)).Options(modes)
//	})
func Controlled[T any](value T, set func(T)) Binding[T] {
	return controlled[T]{value, set}
}

type controlled[T any] struct {
	value T
	set   func(T)
}

func (c controlled[T]) Get() T  { return c.value }
func (c controlled[T]) Set(v T) { c.set(v) }

// countReads runs get, telling a layout recording in progress about what it
// reads as usual, and reports a getter that read no signal.
func countReads[T any](origin string, get func() T) T {
	var v T
	reads := 0
	outer := reactive.Recorder()
	reactive.Measure(func(src reactive.LayoutSource, version uint64) {
		reads++
		if outer != nil {
			outer(src, version)
		}
	}, func() { v = get() })
	if reads == 0 {
		reactive.ReportOnce("bind "+origin, "ggui: the Bind getter at %s read no signal, so its control will not see changes made elsewhere; read a StateValue, or bind with State.Field or State.Lens", origin)
	}
	return v
}

// Not is true while r is false, as a control's BindDisabled wants a "can"
// turned around.
func Not(r Readable[bool]) *DerivedValue[bool] {
	return Map(r, func(v bool) bool { return !v })
}

// Or is true while any one of rs is, as a control disabled while busy or
// while nothing is selected is.
func Or(rs ...Readable[bool]) *DerivedValue[bool] {
	return Derived(func() bool {
		for _, r := range rs {
			if r.Get() {
				return true
			}
		}
		return false
	})
}
