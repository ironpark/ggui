package ggui

import "github.com/ironpark/ggui/internal/property"

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
		Name:     t.para.text,
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
	return t.para.offsetAt(Pt(at.X-t.rect.Origin.X, at.Y-t.rect.Origin.Y), t.rect.Size.W, t.alignment())
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
