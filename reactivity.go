package ggui

import (
	"reflect"

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

// Remember returns the value made for key the last time the widgets around
// it were built, or makes one with init the first time. It is how state a
// View or Reactive builds outlives that View's rebuilds, as a field's draft
// should when the page around it is rebuilt:
//
//	ggui.View(fields, func(fs []Field) ggui.Widget {
//		rows := []ggui.Widget{}
//		for _, f := range fs {
//			text := ggui.Remember(f.ID, func() *ggui.StateValue[string] { return ggui.State(f.Value) })
//			rows = append(rows, ui.TextField(text))
//		}
//		return ggui.Column(rows...)
//	})
//
// A value is kept while the builds around it keep asking for it: one a
// View's build stops asking for is let go when that build ends, and so is
// one asked for by a View its parent rebuilt without, and everything made
// in a Component, an If or Key branch or an Each row goes with it. init
// runs untracked under an owner of its own, so a Derived or an Effect it
// makes lives exactly as long as the value.
//
// Keys belong to the nearest Component, branch or row, together with the
// type of the value, so different kinds of state can share a key; two
// Views asking for one key and type share the value. Outside every
// builder, Remember just returns init().
func Remember[K comparable, T any](key K, init func() T) T {
	b, _ := reactive.Memory().(*boundary)
	if b == nil {
		return Untrack(init)
	}
	m := b.mem
	k := memoKey{key, reflect.TypeFor[T]()}
	e := m.entries[k]
	if e == nil {
		e = &memoEntry{}
		reactive.WithOwner(m.owner, func() {
			e.dispose = reactive.Root(func() { e.value = init() })
		})
		if m.entries == nil {
			m.entries = map[memoKey]*memoEntry{}
		}
		m.entries[k] = e
	}
	if e.by != b || e.seen != b.gen {
		e.by, e.seen = b, b.gen
		b.asked = append(b.asked, k)
	}
	return e.value.(T)
}

// memory is what one Component, branch, row or app keeps remembered values
// in; it goes when its owner does, and the values with it.
type memory struct {
	owner   *reactive.Computation
	entries map[memoKey]*memoEntry
	orphans []memoKey // asked for by Views since disposed; see boundary.build
}

type memoKey struct {
	key any
	typ reflect.Type
}

type memoEntry struct {
	value   any
	dispose func()
	by      *boundary // the builder that asked for it last
	seen    uint64    // by's build that did
}

// boundary is a builder Remember is asked in: a View or Reactive, which
// runs again, or the setup of a Component, branch, row or app, which runs
// once and owns the memory.
type boundary struct {
	mem      *memory
	own      memory    // the memory, when this builder owns it
	gen      uint64    // its builds so far
	asked    []memoKey // what its last build asked for
	disposed bool
}

// remembering runs fn, a builder that runs once, with a memory of its own
// that lasts as long as the owner running it.
func remembering[T any](fn func() T) T {
	b := &boundary{}
	b.own.owner = reactive.CurrentOwner()
	b.mem = &b.own
	prev := reactive.SwapMemory(b)
	defer reactive.SwapMemory(prev)
	return reactive.Build(fn)
}

// newBoundary is the boundary of a View or Reactive being made now, in the
// memory of the builder making it, or nil outside every builder.
func newBoundary() *boundary {
	parent, _ := reactive.Memory().(*boundary)
	if parent == nil {
		return nil
	}
	b := &boundary{mem: parent.mem}
	if reactive.CurrentOwner() != nil {
		OnCleanup(func() {
			b.disposed = true
			b.mem.orphans = append(b.mem.orphans, b.asked...)
		})
	}
	return b
}

// build runs one build of the boundary, then lets go of what it asked for
// last time and not this time, and of what a View this build replaced
// asked for and no new one did.
func (b *boundary) build(fn func() Widget) Widget {
	if b == nil {
		return fn()
	}
	b.gen++
	last := b.asked
	b.asked = nil
	prev := reactive.SwapMemory(b)
	defer reactive.SwapMemory(prev)
	w := fn()
	b.mem.drop(last, func(e *memoEntry) bool { return e.by == b && e.seen != b.gen })
	orphans := b.mem.orphans
	b.mem.orphans = nil
	b.mem.drop(orphans, func(e *memoEntry) bool { return e.by.disposed })
	return w
}

// drop disposes the entries of keys that gone reports gone.
func (m *memory) drop(keys []memoKey, gone func(*memoEntry) bool) {
	for _, k := range keys {
		if e := m.entries[k]; e != nil && gone(e) {
			e.dispose()
			delete(m.entries, k)
		}
	}
}

// DraftValue is an edit held back from the value it edits until it is
// committed. Build one with Draft.
type DraftValue[T any] struct {
	source Readable[T]
	commit func(T) error
	value  *StateValue[T]
	dirty  *StateValue[bool]
	err    *StateValue[string]
	eq     func(a, b T) bool
}

// Committer is a Binding that holds an edit back until it is committed, as
// a Draft does. A text input bound to one commits it when it loses focus
// or submits, and reverts it on Escape.
type Committer interface {
	// Commit hands the edit on and reports whether it was taken.
	Commit() bool
	// Revert drops the edit and any error, reporting whether there was
	// either.
	Revert() bool
}

// Draft is a Binding for a text field that edits source without writing
// it on every keystroke: the field shows source until it is edited, then
// the edit, until a commit hands the edit to commit. A commit that fails
// keeps the edit and shows commit's error through Error, so the user can
// repair it; Escape, or Revert, goes back to source.
//
//	name := ggui.Draft(ggui.Select(store, (*Project).Name), func(v string) error {
//		return project.Rename(v)
//	})
//	ui.Field("Name", ui.TextField(name)).BindError(name.Error())
//
// commit runs when the field loses focus or submits, as Commit does. It
// writes the value on to the model, and source follows from there; source
// may normalize what it was given. A Draft made in a View's build is lost
// with that build; keep one with Remember to have it, edit and error
// included, outlive the page rebuilding around it.
func Draft[T any](source Readable[T], commit func(T) error) *DraftValue[T] {
	return &DraftValue[T]{
		source: source,
		commit: commit,
		value:  State(*new(T)),
		dirty:  State(false),
		err:    State(""),
		eq:     reactive.ComparableEqual[T](),
	}
}

// Get returns the edit while there is one, and source otherwise.
func (d *DraftValue[T]) Get() T {
	if d.dirty.Get() {
		return d.value.Get()
	}
	return d.source.Get()
}

// GetAny implements the reader Textf and Sprintf unwrap.
func (d *DraftValue[T]) GetAny() any { return d.Get() }

// Set makes v the edit, or drops the edit when v is what source holds, and
// clears the error a failed commit left.
func (d *DraftValue[T]) Set(v T) {
	if d.eq != nil && d.eq(v, Untrack(d.source.Get)) {
		d.dirty.Set(false)
	} else {
		d.value.Set(v)
		d.dirty.Set(true)
	}
	d.err.Set("")
}

// Commit hands the edit to commit, if there is one, and reports whether it
// was taken. A failed commit keeps the edit and sets Error.
func (d *DraftValue[T]) Commit() bool {
	if !Untrack(d.dirty.Get) {
		return true
	}
	if err := d.commit(Untrack(d.value.Get)); err != nil {
		d.err.Set(err.Error())
		return false
	}
	d.dirty.Set(false)
	d.err.Set("")
	return true
}

// Revert drops the edit and the error, and reports whether there was
// either.
func (d *DraftValue[T]) Revert() bool {
	had := Untrack(d.dirty.Get) || Untrack(d.err.Get) != ""
	d.dirty.Set(false)
	d.err.Set("")
	return had
}

// Fail shows msg as the error, as a check made elsewhere does; Set and a
// commit clear it.
func (d *DraftValue[T]) Fail(msg string) { d.err.Set(msg) }

// Error is the error of the last failed commit, or Fail's, or "".
func (d *DraftValue[T]) Error() Readable[string] { return d.err }

// Dirty reports whether there is an edit not yet committed.
func (d *DraftValue[T]) Dirty() Readable[bool] { return d.dirty }

// Store makes a model the UI was not written for, such as a plain struct an
// app keeps its documents and undo history in, something the UI follows.
// The model stays plain Go: what the UI shows of it is selected with Select,
// a Derived that is computed again whenever the store says the model
// changed, and changes are made through Update or Action, which say so.
//
//	store := ggui.NewStore(&Editor{})
//	title := ggui.Select(store, func(e *Editor) string { return e.Title })
//	save := ui.Button("Save", store.Action((*Editor).Save))
//
// M is usually a pointer, so that Update changes the model in place. A
// change made around the store, such as one a worker posts, is published
// with Changed. Forgetting to is the mistake a Store exists to catch: a
// ggui_debug build compares what every Select gives after each frame's
// input with what it last gave, and reports a Select whose model changed
// with nothing published.
type Store[M any] struct {
	model   M
	version *StateValue[uint64]
	depth   int // how many Updates are running, so nested ones publish once
	//lint:ignore U1000 only a ggui_debug build checks a store
	storeChecks
}

// NewStore wraps model.
func NewStore[M any](model M) *Store[M] {
	s := &Store[M]{model: model, version: State[uint64](0)}
	watchStore(s)
	return s
}

// Model returns the model, to read in an event handler or a test. Reading
// it is not followed; select what the UI shows.
func (s *Store[M]) Model() M { return s.model }

// Update runs fn on the model and then publishes the change, once however
// many Updates fn itself runs.
func (s *Store[M]) Update(fn func(M)) {
	s.depth++
	defer func() {
		if s.depth--; s.depth == 0 {
			s.Changed()
		}
	}()
	fn(s.model)
}

// Action is Update as an event handler, for a button or a shortcut.
func (s *Store[M]) Action(fn func(M)) func() {
	return func() { s.Update(fn) }
}

// Changed publishes a change made to the model outside Update. Inside one
// it is left to the outermost Update.
func (s *Store[M]) Changed() {
	if s.depth == 0 {
		s.version.Update(func(n uint64) uint64 { return n + 1 })
	}
}

// Select is the part of the store's model fn picks, computed again when the
// store publishes a change and passed on to its readers only when it
// differs, by its Equal method or ==, or as WithEqual says.
func Select[M, T any](s *Store[M], fn func(M) T) *DerivedValue[T] {
	d := Derived(func() T {
		s.version.Get()
		return fn(s.model)
	})
	watchSelect(s, d, fn)
	return d
}

// SelectBind is a Binding over the part of the model get picks: a control
// shows it and hands an edit to set, which runs as an Update.
func SelectBind[M, T any](s *Store[M], get func(M) T, set func(M, T)) Binding[T] {
	return storeBinding[M, T]{Select(s, get), s, set}
}

type storeBinding[M, T any] struct {
	value *DerivedValue[T]
	store *Store[M]
	set   func(M, T)
}

func (b storeBinding[M, T]) Get() T  { return b.value.Get() }
func (b storeBinding[M, T]) Set(v T) { b.store.Update(func(m M) { b.set(m, v) }) }
