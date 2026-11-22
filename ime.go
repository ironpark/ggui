package ggui

import (
	"image"

	"github.com/ironpark/ggui/internal/platform/textinput"
)

// IME is a text widget's session with the platform's input method, through
// which typing reaches it on every platform: composed, as Korean and
// Japanese are, with the composition shown in place, or a keystroke at a
// time. NewIME starts one. The widget calls Update from HandleTick while it
// has the focus, and Confirm before it edits its text itself: a session's
// next commit applies to the text it was last told about.
type IME interface {
	// Update runs the input method for a frame, with the caret where the
	// widget last drew it, in logical pixels, for the candidate window. It
	// reports whether the input method took the frame's keys, which
	// HandleTick returns.
	Update(caret image.Rectangle) (handled bool, err error)
	// Confirm ends the session, committing what was being composed.
	Confirm()
	// Cancel ends the session, dropping what was being composed.
	Cancel()
	// Rebase points the session at new text around the caret, written
	// outside the input method.
	Rebase(before, after string)
	// Typed is the text of a KeyText event. An input method delivers
	// typing through IMEHandler.Commit and ignores it; with none, as under
	// a Probe, Typed commits it.
	Typed(text string)
}

// IMEHandler is how an IME edits the widget it serves. Every function but
// End is required.
type IMEHandler struct {
	// Session starts a session: the caret, in logical pixels, and the text
	// before and after it, which the input method reads for prediction
	// and Replace replaces.
	Session func() (caret image.Rectangle, before, after string)
	// Composition shows text being composed at the caret, with its own
	// caret at a byte offset into it; "" clears it.
	Composition func(text string, caret int)
	// Commit puts text in place of the selection.
	Commit func(text string)
	// Replace puts before, text and after in place of the text the
	// session started with, with the caret after text: an input method
	// rewrote what it was told about, as an autocorrect does.
	Replace func(before, text, after string)
	// End is the user ending input from the platform's side, as by
	// dismissing a virtual keyboard: the widget should give up the focus,
	// since the next session would show the keyboard again.
	End func()
}

// NewIME starts the IME h serves: the one SetIME installed for the tree
// being built, else the platform's input method. That works only while an
// App runs; outside one, as under a Probe, there is no input method:
// Update does nothing and Typed commits what was typed.
func NewIME(h IMEHandler) IME {
	for w := range ownerWorld().chain() {
		if w.ime != nil {
			return w.ime(h)
		}
	}
	return newPlatformIME(h)
}

// SetIME makes fn start the IME of every text widget built from now on, in
// place of the platform's input method, and returns a function that
// restores the one before. Like SetClock, it sets that of the App or Probe
// whose frames run on this goroutine, or, before any has, the one every
// App and Probe made on the goroutine, and every widget built outside
// them, reads. It is for tests, whose fake composes and commits by hand
// through the handler it is given:
//
//	restore := ggui.SetIME(func(h ggui.IMEHandler) ggui.IME { return &fakeIME{h: h} })
//	defer restore()
func SetIME(fn func(IMEHandler) IME) (restore func()) {
	w := activeWorld()
	prev := w.ime
	w.ime = fn
	return func() { w.ime = prev }
}

// platformIME drives textinput.Composer, but only inside a running app:
// the package panics when used before ggfx has chosen its backend.
type platformIME struct {
	textinput.Composer
	h IMEHandler
}

// newPlatformIME builds the platform's IME driver for h, after
// examples/textinput in the Ebitengine repository.
func newPlatformIME(h IMEHandler) *platformIME {
	c := &platformIME{h: h}
	c.OnNewSession = func() *textinput.SessionOptions {
		caret, before, after := h.Session()
		return &textinput.SessionOptions{CaretBounds: caret, TextBeforeCaret: before, TextAfterCaret: after}
	}
	c.OnComposition = func(comp *textinput.Composition) {
		start, _ := comp.SelectionRangeInBytes()
		h.Composition(comp.Text(), start)
	}
	c.OnCommit = func(commit *textinput.Commit) {
		if before, after := commit.IsSurroundingTextReplaced(); before || after {
			nb, na := commit.SurroundingText()
			h.Replace(nb, commit.Text(), na)
			return
		}
		h.Commit(commit.Text())
	}
	c.OnEndByUser = h.End
	return c
}

func (c *platformIME) Rebase(before, after string) { textinput.Rebase(&c.Composer, before, after) }

func (c *platformIME) Update(caret image.Rectangle) (bool, error) {
	textinput.UpdateCaret(caret)
	if !appRunning.Load() {
		return false, nil
	}
	return c.Composer.Update()
}

// Typed commits text typed outside a running app, as under a Probe, where
// there is no platform IME and typed text arrives as KeyText instead, as
// it would without one. Inside one the IME delivers it.
func (c *platformIME) Typed(text string) {
	if !appRunning.Load() {
		c.h.Commit(text)
	}
}
