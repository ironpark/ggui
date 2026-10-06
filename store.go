package ggui

import "github.com/ironpark/ggui/internal/reactive"

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
	origin  string
	checks  []func() (origin string, stale bool)
}

// NewStore wraps model.
func NewStore[M any](model M) *Store[M] {
	s := &Store[M]{model: model, version: State[uint64](0), origin: reactive.Origin()}
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
	if reactive.Debug {
		origin := reactive.Origin()
		s.checks = append(s.checks, func() (string, bool) {
			if reactive.Disposed(d) {
				return origin, false
			}
			return origin, !reactive.Same(d, Untrack(d.Get), fn(s.model))
		})
	}
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
