package widget

import (
	"slices"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/internal/textedit"
)

// SelectionAreaWidget lets a drag select across the Selectable texts inside
// it, as a page in a browser does. Build one with SelectionArea.
type SelectionAreaWidget struct {
	child ggui.Widget

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

var selectionAreaKey = ggui.NewEnvKey[*SelectionAreaWidget]("selection area")

// SelectionArea lets the reader select across the Selectable texts inside
// child at once: a drag from one runs through those painted after it, ⌘A
// or Ctrl+A in one selects them all, and ⌘C or Ctrl+C copies what is
// selected of each, a line apart. A conversation of many paragraphs, each
// a text of its own, then copies as it reads.
//
//	widget.SelectionArea(widget.Column(
//		widget.Text(question).Selectable(),
//		widget.Text(answer).Selectable(),
//	))
func SelectionArea(child ggui.Widget) *SelectionAreaWidget { return &SelectionAreaWidget{child: child} }

// Layout implements Widget.
func (a *SelectionAreaWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return a.child.Layout(c, env.With(selectionAreaKey, a))
}

// Paint implements Widget. A press on the area between its texts clears
// the selection.
//
// A frame painted in part paints inert, around what changed, and keeps the
// texts the last whole frame found, as it keeps that frame's hit regions:
// gathered from it, they would be the few around a hovered row, and a
// click would clear the selection of those alone.
func (a *SelectionAreaWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if !dst.TakesInput() {
		dst.Paint(a.child, r)
		return
	}
	dst.HitPointer(r, a)
	a.painting = a.painting[:0]
	dst.Paint(a.child, r)
	a.texts, a.painting = a.painting, a.texts
	clear(a.painting)
}

// HandlePointer implements PointerHandler: a press that reaches the area,
// past every text, clears the selection and lets the press go on.
func (a *SelectionAreaWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerDown && ev.Button == ggui.MouseButtonLeft && !ev.Mods.Shift {
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
func (a *SelectionAreaWidget) pointer(t *RichTextWidget, ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		if ev.Mods.Shift && a.anchor != nil && slices.Contains(a.texts, a.anchor) && a.anchor != t {
			a.extend(t, t.offsetAt(ev.Pos))
			return true
		}
		a.clear(t)
		t.clicks.Press(&t.ed, ggui.ClockTime(), ev.Pos, t.offsetAt, ev.Mods.Shift)
		a.anchor, a.anchorAt = t, t.ed.Anchor
		a.focus, a.focusAt = t, t.ed.Caret
		return true
	case ggui.PointerDrag:
		u, at := a.textAt(ev.Pos)
		if u == nil || u == a.anchor || !slices.Contains(a.texts, a.anchor) {
			// Within the text it began in, the selection grows as a lone
			// text's does: by words after a double click.
			a.clear(t)
			t.clicks.Drag(&t.ed, ev.Pos, t.offsetAt)
			a.anchor, a.anchorAt, a.focus, a.focusAt = t, t.ed.Anchor, t, t.ed.Caret
			return true
		}
		a.extend(u, at)
		return true
	case ggui.PointerUp, ggui.PointerTap, ggui.PointerMove, ggui.PointerEnter, ggui.PointerExit:
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
	// The texts it runs through are not the one the pointer reached, which
	// alone the frame would paint again.
	ggui.RepaintAll()
	for k, t := range a.texts {
		switch {
		case k == i && i == j:
			t.ed.MoveTo(a.anchorAt, false)
			t.ed.MoveTo(at, true)
		case k == i:
			t.ed.MoveTo(a.anchorAt, false)
			t.ed.MoveTo(gem.Pick(i < j, len(t.ed.Text), 0), true)
		case k == j:
			t.ed.MoveTo(gem.Pick(i < j, 0, len(t.ed.Text)), false)
			t.ed.MoveTo(at, true)
		case k > lo && k < hi:
			t.ed.SelectAll()
		default:
			t.ed.MoveTo(t.ed.Caret, false)
		}
	}
}

// hasSelection reports whether any text in the area currently has a selection.
func (a *SelectionAreaWidget) hasSelection() bool {
	for _, t := range a.texts {
		if t.ed.HasSelection() {
			return true
		}
	}
	return false
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
		ggui.RepaintAll()
	}
}

// clear drops the selection of every text but keep. A text it drops one
// in is painted again with the frame, though the pointer reached another
// or none: a click in one paragraph clears the rest of a selection.
func (a *SelectionAreaWidget) clear(keep *RichTextWidget) {
	for _, t := range a.texts {
		if t != keep && t.ed.HasSelection() {
			t.ed.MoveTo(t.ed.Caret, false)
			ggui.RepaintAll()
		}
	}
	a.spans = false
}

// textAt returns the text inside the area at the window point at, and the
// offset in it nearest at. Between texts it is the one level with at, or
// the one above it, at its end; above every text, the first, at its start.
func (a *SelectionAreaWidget) textAt(at ggui.Point) (*RichTextWidget, int) {
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

// Find selects the match of needle n places on, from 0, among the area's
// texts painted from the window's y top down, in paint order, and reports
// where it is painted, in window coordinates: a page's find, for a page
// whose rows are laid out only near the window. Case is ignored. ok is
// false, and the selection is left as it was, when there are no more than
// n matches.
func (a *SelectionAreaWidget) Find(needle string, top float64, n int) (at ggui.Rect, ok bool) {
	if needle == "" || n < 0 {
		return ggui.Rect{}, false
	}
	for _, t := range a.texts {
		if t.rect.Origin.Y+t.rect.Size.H <= top {
			continue
		}
		text := t.ed.Text
		for i := 0; ; {
			lo, hi := textedit.IndexFold(text, needle, i)
			if lo < 0 {
				break
			}
			if n > 0 {
				n--
				i = hi
				continue
			}
			a.clear(t)
			t.ed.MoveTo(lo, false)
			t.ed.MoveTo(hi, true)
			a.anchor, a.anchorAt, a.focus, a.focusAt = t, lo, t, hi
			ggui.RepaintAll()
			first := true
			t.para.Rects(lo, hi, t.rect.Size.W, t.alignment(), func(b ggui.Rect) {
				b.Origin = t.rect.Origin.Add(b.Origin)
				if first {
					at, first = b, false
				} else {
					at = at.Union(b)
				}
			})
			return at, true
		}
	}
	return ggui.Rect{}, false
}
