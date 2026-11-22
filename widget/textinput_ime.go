package widget

import (
	"image"

	"github.com/ironpark/ggui"
)

// IME callbacks, the editor's ggui.IMEHandler, after examples/textinput in
// the Ebitengine repository. The IME is the platform's when an app runs;
// tests substitute a fake through ggui.SetIME that feeds compositions and
// commits by hand.

// noIME is the IME of an editor another rebuilt over it took its own
// from: it composes nothing.
type noIME struct{}

func (noIME) Update(image.Rectangle) (bool, error) { return false, nil }
func (noIME) Confirm()                             {}
func (noIME) Cancel()                              {}
func (noIME) Rebase(string, string)                {}
func (noIME) Typed(string)                         {}

// imeOwner is the editor an IME serves. It is the IME's handler's one way
// to the editor, so that an editor rebuilt over another takes the IME over
// by pointing it at itself.
type imeOwner struct{ t *TextInputWidget }

// imeSession tells a new session where the caret is and the text around
// it, which is the range a replacing commit replaces.
func (t *TextInputWidget) imeSession() (caret image.Rectangle, before, after string) {
	lo, hi := t.ed.Selection()
	t.imeStart, t.imeEnd = 0, len(t.ed.Text)
	return t.caretBounds, t.ed.Text[:lo], t.ed.Text[hi:]
}

// imeComposition shows preedit text at the caret; "" clears it.
func (t *TextInputWidget) imeComposition(text string, caret int) {
	changed := t.composition != text
	t.composition, t.compCaret = text, caret
	t.blink = ggui.Now()
	if changed && t.multiline {
		ggui.Invalidate(t.env)
	}
}

// imeCommit inserts committed text in place of the selection.
func (t *TextInputWidget) imeCommit(text string) {
	t.ed.Replace(text)
	t.commit()
}

// imeReplace applies a commit that rewrote the surrounding text the session
// was given: before + text + after replace that whole range.
func (t *TextInputWidget) imeReplace(before, text, after string) {
	start, end := t.ed.Snap(t.imeStart), t.ed.Snap(t.imeEnd)
	if start > end {
		start, end = end, start
	}
	t.ed.Anchor, t.ed.Caret = start, end
	t.ed.Replace(before + text + after)
	t.ed.MoveTo(start+len(before)+len(text), false)
	t.commit()
}

// HandleTick implements TickHandler: it runs the IME while focused.
func (t *TextInputWidget) HandleTick() bool {
	if !t.Focused() || t.IsDisabled() {
		return false
	}
	handled, err := t.ime.Update(t.caretBounds)
	if err != nil {
		t.imeErr = err
	}
	return handled
}
