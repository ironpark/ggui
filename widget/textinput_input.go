package widget

import (
	"math"
	goruntime "runtime"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/editlayout"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/pkg/gem"
	"github.com/ironpark/ggui/runtime"
)

// isDarwin reports macOS, whose redo is Shift+Cmd+Z rather than Ctrl+Y.
const isDarwin = goruntime.GOOS == "darwin"

// ConsumesKey implements KeyConsumer: editing keys stay with the editor.
// Escape passes through unless it cancelled composition or OnKey consumed it.
func (t *TextInputWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	return ev.Kind == ggui.KeyPress && (ev.Key != ggui.KeyEscape || t.escapeUsed)
}

// ClaimsChord implements ChordClaimer: the editing chords stay with the
// editor while it has the focus, ahead of any shortcut or menu action on
// the same chord. Those are the clipboard, undo and select-all chords, and
// the modified arrows, Home, End and deletes; ⌘/Ctrl+Enter too when the
// editor submits with it; and any key ClaimKeys claims.
func (t *TextInputWidget) ClaimsChord(ev ggui.KeyEvent) bool {
	m := ev.Mods
	if ev.Kind != ggui.KeyPress || t.IsDisabled() {
		return false
	}
	if t.claims != nil && t.composition == "" && t.claims(ev) {
		return true
	}
	if !(m.Ctrl || m.Alt || m.Meta) {
		return false
	}
	switch ev.Key {
	case ggui.KeyA, ggui.KeyC, ggui.KeyX, ggui.KeyV, ggui.KeyZ:
		return m.Cmd()
	case ggui.KeyY:
		return m.Cmd() && !isDarwin
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyHome, ggui.KeyEnd, ggui.KeyBackspace, ggui.KeyDelete:
		return true
	case ggui.KeyArrowUp, ggui.KeyArrowDown:
		return t.multiline
	case ggui.KeyEnter, ggui.KeyNumpadEnter:
		return t.multiline && t.onSubmit != nil && m.Cmd()
	}
	return false
}

// indexAt returns the byte offset in the text nearest to a logical point.
func (t *TextInputWidget) indexAt(p ggui.Point) int {
	r := t.wrap(t.ed.Text)
	spans := r.Spans
	li := 0
	if t.multiline {
		li = gem.Clamp(int((p.Y-t.rect.Origin.Y+t.scroll)/t.spacing()), 0, len(spans)-1)
	}
	x := p.X - t.rect.Origin.X
	if !t.multiline {
		x += t.scroll
	}
	return t.indexOnLine(r, li, x)
}

// indexOnLine returns the grapheme boundary on line i of r nearest x.
// Measuring partial emoji sequences can give the same width as the whole
// glyph, so rune boundaries would put the caret inside a joined emoji or
// modifier.
func (t *TextInputWidget) indexOnLine(r *editlayout.Wrap, i int, x float64) int {
	sp := r.Spans[i]
	if bl := t.bidiAt(r, i); bl != nil {
		return sp.Start + bl.At(x*t.measureScale())
	}
	width := t.prefixWidths(sp)
	best, bestDist := sp.Start, math.Inf(1)
	for i := sp.Start; ; i = textedit.NextGrapheme(t.ed.Text, i) {
		d := math.Abs(width(i) - x)
		if d < bestDist {
			best, bestDist = i, d
		}
		if i >= sp.End {
			break
		}
	}
	return best
}

// moveLine moves the caret to the nearest position on the line above
// (dir < 0) or below, keeping its x.
func (t *TextInputWidget) moveLine(dir int, extend bool) {
	r := t.wrap(t.ed.Text)
	spans := r.Spans
	li := textlayout.LineOf(spans, t.ed.Caret)
	x := t.xAt(r, li, t.ed.Caret)
	to := li + dir
	switch {
	case to < 0:
		t.ed.MoveTo(0, extend)
	case to >= len(spans):
		t.ed.MoveTo(len(t.ed.Text), extend)
	default:
		t.ed.MoveTo(t.indexOnLine(r, to, x), extend)
	}
}

// lineBounds returns the start and end of the line the caret is on.
func (t *TextInputWidget) lineBounds() (int, int) {
	spans := t.spans(t.ed.Text)
	sp := spans[textlayout.LineOf(spans, t.ed.Caret)]
	return sp.Start, sp.End
}

// HandleKey implements KeyHandler.
func (t *TextInputWidget) HandleKey(ev ggui.KeyEvent) {
	if t.IsDisabled() && ev.Kind != ggui.KeyBlur {
		return
	}
	switch ev.Kind {
	case ggui.KeyFocus:
		t.Interactive.Focused = true
		t.blink = ggui.Now()
	case ggui.KeyBlur:
		t.ime.Confirm()
		t.Interactive.Focused = false
		t.committed()
	case ggui.KeyText:
		// Text arrives through the IME, on every platform. Outside a running
		// app, as under a Probe, there is no platform IME, so typed text
		// arrives here instead, as it would without one, and the IME
		// commits it then.
		t.ime.Typed(ev.Text)
	case ggui.KeyPress:
		t.escapeUsed = ev.Key == ggui.KeyEscape && t.composition != ""
		if t.composition == "" && t.onKey != nil && t.onKey(ev) {
			t.escapeUsed = ev.Key == ggui.KeyEscape
			return
		}
		// Escape drops an edit a Committer holds back, and only then stays
		// with the editor, so a dialog around a clean field still closes.
		if c, ok := t.value.(ggui.Committer); ok && ev.Key == ggui.KeyEscape && t.composition == "" && c.Revert() {
			t.escapeUsed = true
			return
		}
		t.key(ev.Key, ev.Mods)
	}
}

// key applies one key press. It is split by what the key does, and each
// group reports whether anything it changed has to reach the binding: a key
// the editor does not act on, or one that bailed out (Undo with nothing to
// undo, an arrow in a single-line field), leaves the value alone.
func (t *TextInputWidget) key(k ggui.KeyboardKey, m ggui.Mods) {
	if t.editKey(k, m) || t.clipboardKey(k, m) || t.historyKey(k, m) {
		t.commit()
		return
	}
	t.moveKey(k, m)
}

// editKey handles the keys that change the text: Enter, Escape and the two
// deletes. Enter in a single-line field submits instead, which commits
// through committed rather than through the caller.
func (t *TextInputWidget) editKey(k ggui.KeyboardKey, m ggui.Mods) bool {
	switch k {
	case ggui.KeyEnter, ggui.KeyNumpadEnter:
		t.ime.Confirm()
		if t.multiline && !m.Cmd() {
			t.ed.Replace("\n")
			return true
		}
		if t.onSubmit != nil {
			t.onSubmit(t.ed.Text)
		}
		t.committed()
	case ggui.KeyEscape:
		t.ime.Cancel()
		t.ed.MoveTo(t.ed.Caret, false)
		t.blink = ggui.Now()
	case ggui.KeyBackspace:
		t.ime.Confirm()
		t.ed.Backspace(textedit.WordWise(m.Alt, m.Ctrl, m.Meta))
		return true
	case ggui.KeyDelete:
		t.ime.Confirm()
		t.ed.DeleteForward(textedit.WordWise(m.Alt, m.Ctrl, m.Meta))
		return true
	}
	return false
}

// moveKey handles the keys that move the caret. Shift extends the selection
// and Meta jumps to the ends of the text; in a multiline field Up and Down
// step by line and Home and End stay within one.
func (t *TextInputWidget) moveKey(k ggui.KeyboardKey, m ggui.Mods) {
	switch k {
	case ggui.KeyArrowUp, ggui.KeyArrowDown:
		if !t.multiline {
			return
		}
		t.ime.Confirm()
		if m.Meta {
			t.ed.MoveTo(gem.Pick(k == ggui.KeyArrowUp, 0, len(t.ed.Text)), m.Shift)
		} else {
			t.moveLine(gem.Pick(k == ggui.KeyArrowUp, -1, 1), m.Shift)
		}
		t.blink = ggui.Now()
	case ggui.KeyArrowLeft, ggui.KeyArrowRight:
		t.ime.Confirm()
		dir := gem.Pick(k == ggui.KeyArrowLeft, -1, 1)
		if m.Meta {
			// The macOS form of Home and End: the ends of the line the caret
			// is on, which in a single-line field are the ends of the text.
			lo, hi := 0, len(t.ed.Text)
			if t.multiline {
				lo, hi = t.lineBounds()
			}
			t.ed.MoveTo(gem.Pick(dir < 0, lo, hi), m.Shift)
		} else {
			t.ed.MoveBy(dir, textedit.WordWise(m.Alt, m.Ctrl, m.Meta), m.Shift)
		}
		t.blink = ggui.Now()
	case ggui.KeyHome, ggui.KeyEnd:
		t.ime.Confirm()
		lo, hi := 0, len(t.ed.Text)
		if t.multiline && !m.Cmd() {
			lo, hi = t.lineBounds()
		}
		t.ed.MoveTo(gem.Pick(k == ggui.KeyHome, lo, hi), m.Shift)
		t.blink = ggui.Now()
	}
}

// clipboardKey handles the Cmd/Ctrl shortcuts that move text in and out of
// the field: select all, copy, cut and paste. A password field is never
// copied out of, though cutting still deletes. Reports whether the text
// mutated.
func (t *TextInputWidget) clipboardKey(k ggui.KeyboardKey, m ggui.Mods) bool {
	if !m.Cmd() {
		return false
	}
	switch k {
	case ggui.KeyA:
		t.ime.Confirm()
		t.ed.SelectAll()
		t.blink = ggui.Now()
	case ggui.KeyC:
		if t.ed.HasSelection() && !t.password {
			ggui.CurrentClipboard().Write(t.ed.Selected())
		}
	case ggui.KeyX:
		if t.ed.HasSelection() {
			t.ime.Confirm()
			if !t.password {
				ggui.CurrentClipboard().Write(t.ed.Selected())
			}
			t.ed.Replace("")
			return true
		}
	case ggui.KeyV:
		t.ime.Confirm()
		clip := ggui.CurrentClipboard().Read()
		if clip == "" && t.onPasteImage != nil {
			if png, ok := runtime.ReadImage(ggui.CurrentClipboard()); ok && t.onPasteImage(png) {
				return false
			}
		}
		s := strings.ReplaceAll(clip, "\r\n", "\n")
		s = strings.ReplaceAll(s, "\r", "\n")
		if !t.multiline {
			s = strings.ReplaceAll(s, "\n", " ")
		}
		t.ed.Replace(s)
		return true
	}
	return false
}

// historyKey handles the Cmd/Ctrl shortcuts for undo and redo. Both report
// false with nothing left on the stack, so an exhausted history writes
// nothing back.
func (t *TextInputWidget) historyKey(k ggui.KeyboardKey, m ggui.Mods) bool {
	if !m.Cmd() {
		return false
	}
	switch k {
	case ggui.KeyZ:
		t.ime.Confirm()
		if m.Shift {
			return t.ed.Redo()
		}
		return t.ed.Undo()
	case ggui.KeyY:
		// Ctrl+Y is redo everywhere but macOS, which uses Shift+Cmd+Z.
		if isDarwin {
			return false
		}
		t.ime.Confirm()
		return t.ed.Redo()
	}
	return false
}

// CaptureDrag implements ggui.DragCapturer: a drag that selects in a field
// inside a drag source selects, and does not drag the source.
func (t *TextInputWidget) CaptureDrag() bool { return !t.IsDisabled() }

// HandlePointer implements PointerHandler: clicks place the caret, drags
// and repeated clicks select.
func (t *TextInputWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if t.IsDisabled() {
		return false
	}
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		t.ime.Confirm()
		t.clicks.Press(&t.ed, ggui.ClockTime(), ev.Pos, t.indexAt, ev.Mods.Shift)
		t.blink = ggui.Now()
		return true
	case ggui.PointerDrag:
		if t.clicks.Drag(&t.ed, ev.Pos, t.indexAt) {
			t.blink = ggui.Now()
		}
		return true
	case ggui.PointerUp, ggui.PointerTap, ggui.PointerMove, ggui.PointerEnter, ggui.PointerExit:
		return true
	}
	return false
}
