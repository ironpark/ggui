package ggui

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ironpark/ggui/internal/property"
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

// Find selects the match of needle n places on, from 0, among the area's
// texts painted from the window's y top down, in paint order, and reports
// where it is painted, in window coordinates: a page's find, for a page
// whose rows are laid out only near the window. Case is ignored. ok is
// false, and the selection is left as it was, when there are no more than
// n matches.
func (a *SelectionAreaWidget) Find(needle string, top float64, n int) (at Rect, ok bool) {
	if needle == "" || n < 0 {
		return Rect{}, false
	}
	for _, t := range a.texts {
		if t.rect.Origin.Y+t.rect.Size.H <= top {
			continue
		}
		text := t.ed.Text
		for i := 0; ; {
			lo, hi := IndexFold(text, needle, i)
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
			first := true
			t.para.Rects(lo, hi, t.rect.Size.W, t.alignment(), func(b Rect) {
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
	return Rect{}, false
}

// IndexFold returns where needle is first found in s at or after byte from,
// ignoring case, as the byte range [lo, hi) of s; lo is -1 when it is not
// found. The range is s's own, which may differ in length from needle.
func IndexFold(s, needle string, from int) (lo, hi int) {
	if needle == "" {
		return -1, -1
	}
	for i := max(from, 0); i < len(s); {
		if j, ok := prefixFold(s[i:], needle); ok {
			return i, i + j
		}
		_, w := utf8.DecodeRuneInString(s[i:])
		i += w
	}
	return -1, -1
}

// prefixFold reports whether s begins with prefix, ignoring case, and how
// many bytes of s that takes.
func prefixFold(s, prefix string) (int, bool) {
	i := 0
	for _, p := range prefix {
		if i >= len(s) {
			return 0, false
		}
		r, w := utf8.DecodeRuneInString(s[i:])
		if r != p && unicode.ToLower(r) != unicode.ToLower(p) {
			return 0, false
		}
		i += w
	}
	return i, true
}

// Selectable lets the reader select the paragraph's text and copy it: a
// drag selects from where it began, a double click a word, a triple click
// the line, a Shift-click extends; once a click has focused it ⌘A or
// Ctrl+A selects everything and ⌘C or Ctrl+C copies the selection. The
// selection is drawn in the Env's EditorStyle Selection color while the
// text has the focus. It stays out of the Tab order.
func (t *RichTextWidget) Selectable() *RichTextWidget {
	defer property.Watch(&t.props, &t.selectable)()
	t.selectable = true
	return t
}

// Selectable makes the text selectable and copyable, as
// RichTextWidget.Selectable does.
func (t *TextWidget) Selectable() *TextWidget {
	if t.rich == nil {
		t.rich = RichText().Selectable()
		t.props.Changed()
	}
	return t
}

// Selection returns the selected byte range of Text, lo <= hi; lo == hi
// when nothing is selected.
func (t *RichTextWidget) Selection() (lo, hi int) { return t.ed.Selection() }

// SelectedText returns the selected text.
func (t *RichTextWidget) SelectedText() string { return t.ed.Selected() }

// layoutRich lays a Selectable Text out as a one-span RichText in style,
// the Text's own merged onto the one it inherits.
func (t *TextWidget) layoutRich(c Constraints, env Env, style TextStyle) Size {
	r := t.rich
	if len(r.cur) != 1 || r.cur[0].text != t.value {
		r.spans.Set([]TextSpan{Span(t.value)})
	}
	r.style = style
	r.wrap, r.ellipsis, r.maxLines = t.wrap, t.ellipsis, t.maxLines
	r.align, r.alignSet, r.role = t.align, t.alignSet, t.role
	return r.Layout(c, env)
}

// Describe implements Describer: text whose selection a screen reader can
// read and move.
func (t *RichTextWidget) Describe() Node {
	lo, hi := t.ed.Selection()
	return Node{
		Role:     roleOr(t.role, RoleText),
		Name:     t.para.Text,
		Actions:  ActionFocus | ActionSetSelection,
		SelStart: lo,
		SelEnd:   hi,
	}
}

// Act implements Actor: a screen reader moves the selection.
func (t *RichTextWidget) Act(a Action) bool {
	if a.Kind != ActionSetSelection || !t.selectable {
		return false
	}
	t.ed.MoveTo(a.SelStart, false)
	t.ed.MoveTo(a.SelEnd, true)
	return true
}

// TabStop implements TabStopper: selectable text is focused by a click,
// never by Tab.
func (t *RichTextWidget) TabStop() bool { return false }

// ConsumesKey implements KeyConsumer: Escape clears a selection, and the
// arrows move one; with nothing selected Escape passes on.
func (t *RichTextWidget) ConsumesKey(ev KeyEvent) bool {
	if ev.Kind != KeyPress {
		return false
	}
	switch ev.Key {
	case KeyEscape:
		return t.ed.HasSelection() || t.area != nil && t.area.spans
	case KeyArrowLeft, KeyArrowRight, KeyHome, KeyEnd:
		return ev.Mods.Shift
	}
	return false
}

// ClaimsChord implements ChordClaimer: select-all and copy stay with the
// focused text, ahead of a shortcut on the same chord.
func (t *RichTextWidget) ClaimsChord(ev KeyEvent) bool {
	return ev.Kind == KeyPress && t.selectable && ev.Mods.Cmd() && (ev.Key == KeyA || ev.Key == KeyC)
}

// HandleKey implements KeyHandler.
func (t *RichTextWidget) HandleKey(ev KeyEvent) {
	switch ev.Kind {
	case KeyFocus:
		t.focused = true
	case KeyBlur:
		t.focused = false
	case KeyPress:
		m := ev.Mods
		switch ev.Key {
		case KeyA:
			if m.Cmd() && t.area != nil {
				t.area.selectAll()
			} else if m.Cmd() {
				t.ed.SelectAll()
			}
		case KeyC:
			if m.Cmd() && t.area != nil && t.area.spans {
				currentClipboard().Write(t.area.SelectedText())
			} else if m.Cmd() && t.ed.HasSelection() {
				currentClipboard().Write(t.ed.Selected())
			}
		case KeyEscape:
			if t.area != nil {
				t.area.clear(nil)
			}
			t.ed.MoveTo(t.ed.Caret, false)
		case KeyArrowLeft, KeyArrowRight:
			if m.Shift {
				t.ed.MoveBy(pick(ev.Key == KeyArrowLeft, -1, 1), wordWise(m), true)
			}
		case KeyHome, KeyEnd:
			if m.Shift {
				t.ed.MoveTo(pick(ev.Key == KeyHome, 0, len(t.ed.Text)), true)
			}
		}
	}
}

// HandlePointer implements PointerHandler: a press places the selection's
// start, a drag extends it, and repeated clicks select a word, then the
// line.
func (t *RichTextWidget) HandlePointer(ev PointerEvent) bool {
	if t.area != nil {
		return t.area.pointer(t, ev)
	}
	switch ev.Kind {
	case PointerDown:
		if ev.Button != MouseButtonLeft {
			return false
		}
		t.clicks.press(&t.ed, ev.Pos, t.offsetAt, ev.Mods.Shift)
		return true
	case PointerDrag:
		t.clicks.drag(&t.ed, ev.Pos, t.offsetAt)
		return true
	case PointerUp, PointerTap, PointerMove, PointerEnter, PointerExit:
		return true
	}
	return false
}

// CaptureTouchDrag implements TouchDragCapturer: a touch drag that began
// on selectable text selects rather than scrolls.
func (t *RichTextWidget) CaptureTouchDrag() bool { return t.selectable }

// offsetAt returns the text offset nearest the window point at.
func (t *RichTextWidget) offsetAt(at Point) int {
	return t.para.OffsetAt(Pt(at.X-t.rect.Origin.X, at.Y-t.rect.Origin.Y), t.rect.Size.W, t.alignment())
}

// Adopt implements Adopter: a rebuilt paragraph at the same place keeps
// the previous one's focus and, while the text is the same, its selection.
func (t *RichTextWidget) Adopt(prev any) {
	p, ok := prev.(*RichTextWidget)
	if !ok || p == t {
		return
	}
	t.focused = p.focused
	t.clicks = p.clicks
	if p.ed.Text == t.ed.Text {
		t.ed.Anchor, t.ed.Caret = p.ed.Anchor, p.ed.Caret
	}
}
