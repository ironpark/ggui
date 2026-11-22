package widget

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/editlayout"
	"github.com/ironpark/ggui/internal/textedit"
)

// Semantics implements Semantic.
func (t *TextInputWidget) Semantics() (ggui.Role, string) {
	if t.SemanticName() != "" {
		return ggui.RoleTextField, t.SemanticName()
	}
	return ggui.RoleTextField, t.placeholder
}

// Describe implements Describer: the field, its contents and whether it
// takes input. A password's contents are the bullets drawn for it, which
// tell a screen reader its length and where the caret is, but not what it
// says. ui.TextField describes the padded box it draws around this
// editor with the same handler, which is why the two are one node and not
// two: the first description of a handler in a frame is the one kept.
func (t *TextInputWidget) Describe() ggui.Node {
	role, name := t.Semantics()
	lo, hi := t.ed.Selection()
	n := ggui.Node{
		Role:     role,
		Name:     name,
		Value:    t.display(t.ed.Text),
		Disabled: t.IsDisabled(),
		Actions:  ggui.ActionFocus | ggui.ActionSetValue | ggui.ActionSetSelection,
		SelStart: t.toDisplay(lo),
		SelEnd:   t.toDisplay(hi),
	}
	// A screen reader reads a field character by character and line by
	// line, and needs to know where each of them went. Working that out
	// costs a measurement per character, so it is only done while
	// something is attached that will ask; see a11y.WantsDetail. A
	// password is
	// left out of it entirely: its shape on screen is bullets, and its
	// contents are not for reading out.
	if a11y.WantsDetail() && !t.password {
		n.Runs = t.runs()
	}
	if t.onDescribe != nil {
		// The runs are kept for the next description, so the callback is
		// not handed them to change: copying them for it each frame would
		// cost what keeping them saves. Runs it sets are its own.
		runs := n.Runs
		n.Runs = nil
		t.onDescribe(&n)
		if n.Runs == nil {
			n.Runs = runs
		}
	}
	return n
}

// runs freezes the editor's layout into the tree: one entry per line, with
// the position of every character boundary on it.
//
// The geometry is where the editor last painted, not where it is about to:
// describing happens before the paint that would move it, and ui.TextField
// describes this widget from the box around it, a step earlier still. A
// field that moved or scrolled this frame therefore reports character
// positions one frame behind, which is a frame that has not been shown yet.
func (t *TextInputWidget) runs() []ggui.TextRun {
	d := &t.described
	key := runsKey{t.ed.Text, t.wrapStyle(), t.textStyle().LineHeight, t.measureScale()}
	if d.measured == nil || d.key != key {
		d.key, d.measured = key, t.measureRuns()
	}
	// The runs are measured from the editor's top left, unscrolled, and
	// moved to where it was painted here: a scroll moves them all, and
	// measures none of them again.
	d.out = append(d.out[:0], d.measured...)
	for i := range d.out {
		r := &d.out[i].Rect
		r.Origin = r.Origin.Add(t.rect.Origin)
		if t.multiline {
			r.Origin.Y -= t.scroll
		} else {
			r.Origin.X -= t.scroll
		}
	}
	return d.out
}

// runsKey is what the runs' measures depend on: the text, how it is
// wrapped and in what, how far apart its lines are, and the display scale.
type runsKey struct {
	text       string
	style      editlayout.Style
	lineHeight float64
	scale      float64
}

// measureRuns is runs, measured from the editor's top left, unscrolled.
func (t *TextInputWidget) measureRuns() []ggui.TextRun {
	r := t.wrap(t.ed.Text)
	out := make([]ggui.TextRun, 0, len(r.Spans))
	h := t.height()
	scale := t.measureScale()
	for i, sp := range r.Spans {
		y := 0.0
		if t.multiline {
			y = float64(i) * t.spacing()
		}
		var width func(b int) float64
		var w float64
		if bl := t.bidiAt(r, i); bl != nil {
			width, w = func(b int) float64 { return bl.X(b-sp.Start) / scale }, bl.Width()/scale
		} else {
			width = t.prefixWidths(sp)
			w = width(sp.End)
		}
		run := ggui.TextRun{Start: sp.Start, End: sp.End, Rect: ggui.Rct(ggui.Pt(0, y), ggui.Sz(w, h))}
		for b := sp.Start; ; b = textedit.NextRune(t.ed.Text, b) {
			run.Stops = append(run.Stops, ggui.TextStop{Byte: b, X: width(b)})
			if b >= sp.End {
				break
			}
		}
		out = append(out, run)
	}
	return out
}

// Act implements Actor: the platform's text API replaces the contents
// outright, which is what a dictation or a braille display does, and moves
// the caret, which is what a screen reader does as it reads along.
func (t *TextInputWidget) Act(a ggui.Action) bool {
	if t.IsDisabled() {
		return false
	}
	if t.onAction != nil && t.onAction(a) {
		return true
	}
	switch a.Kind {
	case ggui.ActionSetValue:
		t.ed.SetText(a.Text)
		t.commit()
		return true
	case ggui.ActionSetSelection:
		t.ed.MoveTo(t.fromDisplay(a.SelStart), false)
		t.ed.MoveTo(t.fromDisplay(a.SelEnd), true)
		return true
	}
	return false
}
