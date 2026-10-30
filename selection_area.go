package ggui

import (
	"slices"
	"strings"
)

// SelectionAreaWidget lets a drag select across the Selectable texts inside
// it, as a page in a browser does. Build one with SelectionArea.
type SelectionAreaWidget struct {
	child Widget

	// The Selectable texts inside, in the order they were painted, which
	// is the order a selection runs through them; painting gathers the
	// next frame's.
	texts, painting []*RichTextWidget

	// The selection runs from the anchor, where it began, to the focus,
	// where it ends, each a text and an offset in it.
	anchor, focus     *RichTextWidget
	anchorAt, focusAt int
	spans             bool // the selection spans more than one text
}

var selectionAreaKey = NewEnvKey[*SelectionAreaWidget]("selection area")

// SelectionArea lets the reader select across the Selectable texts inside
// child at once: a drag from one runs through those painted after it, ⌘A
// or Ctrl+A in one selects them all, and ⌘C or Ctrl+C copies what is
// selected of each, a line apart. A conversation of many paragraphs, each
// a text of its own, then copies as it reads.
//
//	ggui.SelectionArea(ggui.Column(
//		ggui.Text(question).Selectable(),
//		ggui.Text(answer).Selectable(),
//	))
func SelectionArea(child Widget) *SelectionAreaWidget { return &SelectionAreaWidget{child: child} }

// Layout implements Widget.
func (a *SelectionAreaWidget) Layout(c Constraints, env Env) Size {
	return a.child.Layout(c, env.With(selectionAreaKey, a))
}

// Paint implements Widget. A press on the area between its texts clears
// the selection.
func (a *SelectionAreaWidget) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, a)
	a.painting = a.painting[:0]
	dst.Paint(a.child, r)
	a.texts, a.painting = a.painting, a.texts
	clear(a.painting)
}

// HandlePointer implements PointerHandler: a press that reaches the area,
// past every text, clears the selection and lets the press go on.
func (a *SelectionAreaWidget) HandlePointer(ev PointerEvent) bool {
	if ev.Kind == PointerDown && ev.Button == MouseButtonLeft && !ev.Mods.Shift {
		a.clear(nil)
	}
	return false
}

// SelectedText returns what is selected in the area, each text's part a
// line apart, in the order the texts were painted.
func (a *SelectionAreaWidget) SelectedText() string {
	var parts []string
	for _, t := range a.texts {
		if t.ed.HasSelection() {
			parts = append(parts, t.ed.Selected())
		}
	}
	return strings.Join(parts, "\n")
}

// painted notes t, a Selectable text inside the area, as painted now.
func (a *SelectionAreaWidget) painted(t *RichTextWidget) { a.painting = append(a.painting, t) }

// pointer handles the pointer for t, a text inside the area: a press
// starts a selection in it, as a lone text's would, and a drag carries the
// selection into the texts it reaches.
func (a *SelectionAreaWidget) pointer(t *RichTextWidget, ev PointerEvent) bool {
	switch ev.Kind {
	case PointerDown:
		if ev.Button != MouseButtonLeft {
			return false
		}
		if ev.Mods.Shift && a.anchor != nil && slices.Contains(a.texts, a.anchor) && a.anchor != t {
			a.extend(t, t.offsetAt(ev.Pos))
			return true
		}
		a.clear(t)
		t.clicks.press(&t.ed, ev.Pos, t.offsetAt, ev.Mods.Shift)
		a.anchor, a.anchorAt = t, t.ed.Anchor
		a.focus, a.focusAt = t, t.ed.Caret
		return true
	case PointerDrag:
		u, at := a.textAt(ev.Pos)
		if u == nil || u == a.anchor || !slices.Contains(a.texts, a.anchor) {
			// Within the text it began in, the selection grows as a lone
			// text's does: by words after a double click.
			a.clear(t)
			t.clicks.drag(&t.ed, ev.Pos, t.offsetAt)
			a.anchor, a.anchorAt, a.focus, a.focusAt = t, t.ed.Anchor, t, t.ed.Caret
			return true
		}
		a.extend(u, at)
		return true
	case PointerUp, PointerTap, PointerMove, PointerEnter, PointerExit:
		return true
	}
	return false
}

// extend moves the selection's focus to offset at of u: the anchor's text
// is selected from the anchor on toward u, the texts between wholly, and
// u up to at.
func (a *SelectionAreaWidget) extend(u *RichTextWidget, at int) {
	i, j := slices.Index(a.texts, a.anchor), slices.Index(a.texts, u)
	if i < 0 || j < 0 {
		return
	}
	a.focus, a.focusAt, a.spans = u, at, i != j
	lo, hi := min(i, j), max(i, j)
	for k, t := range a.texts {
		switch {
		case k == i && k == j:
			t.ed.MoveTo(a.anchorAt, false)
			t.ed.MoveTo(at, true)
		case k == i:
			t.ed.MoveTo(a.anchorAt, false)
			t.ed.MoveTo(pick(i < j, len(t.ed.Text), 0), true)
		case k == j:
			t.ed.MoveTo(pick(i < j, 0, len(t.ed.Text)), false)
			t.ed.MoveTo(at, true)
		case k > lo && k < hi:
			t.ed.SelectAll()
		default:
			t.ed.MoveTo(t.ed.Caret, false)
		}
	}
}

// selectAll selects every text in the area.
func (a *SelectionAreaWidget) selectAll() {
	for _, t := range a.texts {
		t.ed.SelectAll()
	}
	if len(a.texts) > 0 {
		first, last := a.texts[0], a.texts[len(a.texts)-1]
		a.anchor, a.anchorAt, a.focus, a.focusAt = first, 0, last, len(last.ed.Text)
		a.spans = len(a.texts) > 1
	}
}

// clear drops the selection of every text but keep.
func (a *SelectionAreaWidget) clear(keep *RichTextWidget) {
	for _, t := range a.texts {
		if t != keep {
			t.ed.MoveTo(t.ed.Caret, false)
		}
	}
	a.spans = false
}

// textAt returns the text inside the area at the window point at, and the
// offset in it nearest at. Between texts it is the one level with at, or
// the one above it, at its end; above every text, the first, at its start.
func (a *SelectionAreaWidget) textAt(at Point) (*RichTextWidget, int) {
	for _, t := range a.texts {
		if t.rect.Contains(at) {
			return t, t.offsetAt(at)
		}
	}
	for _, t := range a.texts {
		if at.Y >= t.rect.Origin.Y && at.Y < t.rect.Origin.Y+t.rect.Size.H {
			return t, t.offsetAt(at)
		}
	}
	var above *RichTextWidget
	for _, t := range a.texts {
		if t.rect.Origin.Y <= at.Y {
			above = t
		}
	}
	if above != nil {
		return above, len(above.ed.Text)
	}
	if len(a.texts) > 0 {
		return a.texts[0], 0
	}
	return nil, 0
}
