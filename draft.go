package ggui

import "github.com/ironpark/ggui/internal/reactive"

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
