package ggui

// Baseliner is a widget that knows where its first line of text sits: the
// distance from the top of the Rect it was laid out for to that line's
// baseline. Row.Align(AlignBaseline) lines children up on it, so a small
// caption beside a large title shares its baseline instead of its centre.
//
// The answer is valid after Layout, for the size Layout returned. Text and
// TextInput report their face's ascent; Box, Column, Align and the other
// containers report their first child's baseline moved by where that child
// went. A widget with no text in it reports false, and a Row then rests it
// on the baseline by its bottom edge, as CSS does with an image in a line
// of text.
type Baseliner interface {
	Baseline() (float64, bool)
}

// baselineOf asks w for its baseline, or reports false when w cannot say.
func baselineOf(w Widget) (float64, bool) {
	if b, ok := w.(Baseliner); ok {
		return b.Baseline()
	}
	return 0, false
}

// baselineAt is baselineOf for a child painted dy below its parent's top.
func baselineAt(w Widget, dy float64) (float64, bool) {
	b, ok := baselineOf(w)
	return b + dy, ok
}
