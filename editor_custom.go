package ggui

import "image"

// TextInputState is a read-only snapshot for custom editor presentations.
// Offsets are UTF-8 byte boundaries in Text. Composing reports an IME preedit.
type TextInputState struct {
	Text               string
	Anchor, Caret      int
	Focused, Composing bool
}

// Filter normalizes edits before publishing them to the binding. It must be a
// pure, prefix-preserving function: removing characters and truncating a suffix
// are supported. External binding writes remain the application's responsibility.
// Filtering applies to typing, paste, IME commits, undo and accessibility edits.
func (t *TextInputWidget) Filter(fn func(string) string) *TextInputWidget {
	t.filter = fn
	return t
}

// EditingState returns the displayed text and selection, including IME preedit.
func (t *TextInputWidget) EditingState() TextInputState {
	s, caret := t.rendered()
	a := t.ed.anchor
	if t.composition != "" {
		a = caret
	}
	return TextInputState{s, a, caret, t.Focused(), t.composition != ""}
}

// Select sets the selection at UTF-8 byte offsets, clamped to rune boundaries.
func (t *TextInputWidget) Select(anchor, caret int) {
	if t.IsDisabled() {
		return
	}
	t.ime.Confirm()
	t.ed.moveTo(anchor, false)
	t.ed.moveTo(caret, true)
	t.blink = Now()
}

// PaintCustom replaces the standard text drawing, for segmented editors such
// as Input OTP. The callback returns the logical caret bounds used by the native
// IME. The caller owns hit regions and semantics and forwards keyboard events
// and HandleTick to this editor. Layout must have run first.
func (t *TextInputWidget) PaintCustom(dst *Canvas, r Rect, paint func(*Canvas, Rect, TextInputState) Rect) {
	t.rect, t.scale = r, dst.Scale()
	caret := paint(dst, r, t.EditingState())
	t.caretPx = image.Rect(int(dst.px(caret.Origin.X)), int(dst.px(caret.Origin.Y)), int(dst.px(caret.Origin.X+caret.Size.W)), int(dst.px(caret.Origin.Y+caret.Size.H)))
}
