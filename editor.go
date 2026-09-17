package ggui

import (
	"image"
	"image/color"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/textinput"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// textEditor is the model behind TextInput: a string and a selection. The
// caret sits at caret; anchor is where the selection started, so anchor ==
// caret means no selection. Offsets are bytes on rune boundaries.
type textEditor struct {
	text   string
	anchor int
	caret  int
}

func (e *textEditor) setText(s string) {
	e.text = s
	e.anchor, e.caret = e.snap(e.anchor), e.snap(e.caret)
}

// snap clamps i into the text and back to the start of the rune it is in.
func (e *textEditor) snap(i int) int {
	i = clamp(i, 0, len(e.text))
	for i > 0 && i < len(e.text) && !utf8.RuneStart(e.text[i]) {
		i--
	}
	return i
}

// selection returns the selected byte range, lo <= hi.
func (e *textEditor) selection() (lo, hi int) {
	return min(e.anchor, e.caret), max(e.anchor, e.caret)
}

func (e *textEditor) hasSelection() bool { return e.anchor != e.caret }

func (e *textEditor) selected() string {
	lo, hi := e.selection()
	return e.text[lo:hi]
}

// replace puts s in place of the selection and leaves the caret after it.
func (e *textEditor) replace(s string) {
	lo, hi := e.selection()
	e.text = e.text[:lo] + s + e.text[hi:]
	e.caret = lo + len(s)
	e.anchor = e.caret
}

// moveTo puts the caret at pos, extending the selection or collapsing it.
func (e *textEditor) moveTo(pos int, extend bool) {
	e.caret = e.snap(pos)
	if !extend {
		e.anchor = e.caret
	}
}

// moveBy moves the caret one rune or one word left (dir < 0) or right.
// Without extend, a selection collapses to its edge in that direction first,
// as every text field does.
func (e *textEditor) moveBy(dir int, word, extend bool) {
	if !extend && e.hasSelection() && !word {
		lo, hi := e.selection()
		e.moveTo(pick(dir < 0, lo, hi), false)
		return
	}
	pos := e.caret
	switch {
	case dir < 0 && word:
		pos = prevWord(e.text, pos)
	case dir < 0:
		pos = prevRune(e.text, pos)
	case word:
		pos = nextWord(e.text, pos)
	default:
		pos = nextRune(e.text, pos)
	}
	e.moveTo(pos, extend)
}

// backspace deletes the selection, or the rune or word before the caret.
func (e *textEditor) backspace(word bool) {
	if !e.hasSelection() {
		e.anchor = pick(word, prevWord(e.text, e.caret), prevRune(e.text, e.caret))
	}
	e.replace("")
}

// deleteForward deletes the selection, or the rune or word after the caret.
func (e *textEditor) deleteForward(word bool) {
	if !e.hasSelection() {
		e.anchor = pick(word, nextWord(e.text, e.caret), nextRune(e.text, e.caret))
	}
	e.replace("")
}

func (e *textEditor) selectAll() { e.anchor, e.caret = 0, len(e.text) }

// selectWord selects the word around pos, or the run of spaces it is in.
func (e *textEditor) selectWord(pos int) {
	pos = e.snap(pos)
	if len(e.text) == 0 {
		return
	}
	if pos == len(e.text) {
		pos = prevRune(e.text, pos)
	}
	r, _ := utf8.DecodeRuneInString(e.text[pos:])
	class := runeClass(r)
	lo, hi := pos, pos
	for lo > 0 {
		p := prevRune(e.text, lo)
		if q, _ := utf8.DecodeRuneInString(e.text[p:]); runeClass(q) != class {
			break
		}
		lo = p
	}
	for hi < len(e.text) {
		if q, _ := utf8.DecodeRuneInString(e.text[hi:]); runeClass(q) != class {
			break
		}
		hi = nextRune(e.text, hi)
	}
	e.anchor, e.caret = lo, hi
}

func prevRune(s string, i int) int {
	if i <= 0 {
		return 0
	}
	_, n := utf8.DecodeLastRuneInString(s[:i])
	return i - n
}

func nextRune(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	_, n := utf8.DecodeRuneInString(s[i:])
	return i + n
}

// runeClass groups runes for word movement: spaces, word characters and
// punctuation each form their own runs.
func runeClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	}
	return 2
}

// prevWord returns the start of the word before i: spaces are skipped, then
// the run of same-class runes before them.
func prevWord(s string, i int) int {
	for i > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:i])
		if !unicode.IsSpace(r) {
			break
		}
		i = prevRune(s, i)
	}
	if i == 0 {
		return 0
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	class := runeClass(r)
	for i > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:i])
		if runeClass(r) != class {
			break
		}
		i = prevRune(s, i)
	}
	return i
}

// nextWord returns the end of the word after i: the run of same-class runes
// at i, then the spaces after it.
func nextWord(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	class := runeClass(r)
	for i < len(s) {
		r, _ := utf8.DecodeRuneInString(s[i:])
		if runeClass(r) != class {
			break
		}
		i = nextRune(s, i)
	}
	for i < len(s) {
		r, _ := utf8.DecodeRuneInString(s[i:])
		if !unicode.IsSpace(r) {
			break
		}
		i = nextRune(s, i)
	}
	return i
}

// TextInputWidget is a single-line text editor bound to a Signal[string]:
// typing writes the signal, and writing the signal updates the text. Build
// one with TextInput. It draws only the text, selection and caret; TextField
// adds the themed box around it.
//
// Text comes in through the platform IME (exp/textinput), so composed
// scripts such as Korean and Japanese work, with the composition shown
// underlined in place. Keys: arrows (with Shift to select, Alt or Ctrl to
// jump words, ⌘ on macOS to reach the ends), Home and End, Backspace and
// Delete, ⌘/Ctrl+A, C, X and V, Enter for OnSubmit. Double-click selects a
// word, triple-click everything, dragging selects a range.
type TextInputWidget struct {
	value       *Signal[string]
	placeholder string
	style       TextStyle
	password    bool
	minWidth    float64
	onSubmit    func(string)
	onChange    func(string)

	ed      textEditor
	focused bool

	ime         ime
	composition string
	compCaret   int // caret inside composition, in bytes
	imeStart    int // the byte range of ed.text the IME was told about
	imeEnd      int
	imeErr      error

	scroll    float64 // how far the text is shifted left to show the caret
	blink     time.Time
	clicks    int
	lastClick time.Time
	lastPos   Point

	resolved  TextStyle
	muted     color.Color
	selection color.Color
	rect      Rect            // where it was last painted, in logical pixels
	scale     float64         // the Canvas scale at that paint
	caretPx   image.Rectangle // the caret in screen pixels, for the IME
}

// TextInput creates an editor bound to value.
func TextInput(value *Signal[string]) *TextInputWidget {
	t := &TextInputWidget{value: value, minWidth: 120}
	t.ime = newIME(t)
	t.ed.setText(value.Peek())
	t.ed.moveTo(len(t.ed.text), false)
	return t
}

// Placeholder sets the muted text shown while the value is empty.
func (t *TextInputWidget) Placeholder(s string) *TextInputWidget { t.placeholder = s; return t }

// Password masks every rune with a bullet.
func (t *TextInputWidget) Password() *TextInputWidget { t.password = true; return t }

// Style merges ts onto the widget's own text style.
func (t *TextInputWidget) Style(ts TextStyle) *TextInputWidget { t.style = t.style.Merge(ts); return t }

// MinWidth sets the width the editor asks for when its parent leaves the
// width to it; it fills a bounded width.
func (t *TextInputWidget) MinWidth(w float64) *TextInputWidget { t.minWidth = w; return t }

// OnSubmit fires with the value when Enter is pressed.
func (t *TextInputWidget) OnSubmit(fn func(string)) *TextInputWidget { t.onSubmit = fn; return t }

// OnChange fires with the value after every edit, after the signal is set.
func (t *TextInputWidget) OnChange(fn func(string)) *TextInputWidget { t.onChange = fn; return t }

// Focused reports whether the editor has keyboard focus.
func (t *TextInputWidget) Focused() bool { return t.focused }

// display returns s as drawn: itself, or bullets in password mode.
func (t *TextInputWidget) display(s string) string {
	if !t.password {
		return s
	}
	return strings.Repeat("•", utf8.RuneCountInString(s))
}

func (t *TextInputWidget) face(scale float64) text.Face {
	st := t.resolved
	if st.Font == nil {
		st = t.style.resolved()
	}
	return st.Font.face(st.Size * scale)
}

// advance is the logical width of s as drawn.
func (t *TextInputWidget) advance(s string) float64 { return text.Advance(t.display(s), t.face(1)) }

// rendered is the text with the composition inserted where the caret is,
// and the caret's offset into it.
func (t *TextInputWidget) rendered() (s string, caret int) {
	lo, hi := t.ed.selection()
	if t.composition == "" {
		return t.ed.text, t.ed.caret
	}
	return t.ed.text[:lo] + t.composition + t.ed.text[hi:], lo + t.compCaret
}

func (t *TextInputWidget) height() float64 {
	m := t.face(1).Metrics()
	return m.HAscent + m.HDescent
}

// Layout implements Widget.
func (t *TextInputWidget) Layout(c Constraints, env Env) Size {
	t.resolved = env.Text().Merge(t.style).resolved()
	th := env.Theme()
	t.muted, t.selection = th.Muted, th.Selection
	if v := t.value.Peek(); v != t.ed.text {
		t.ed.setText(v)
	}
	return c.Constrain(Sz(bounded(c.MaxW, t.minWidth), t.height()))
}

// Paint implements Widget.
func (t *TextInputWidget) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, t)
	dst.HitKey(r, t)
	dst.HitCursor(r, ebiten.CursorShapeText)
	t.rect, t.scale = r, dst.Scale()

	shown, caret := t.rendered()
	h := t.height()
	caretX := t.advance(shown[:caret])
	total := t.advance(shown)
	t.scroll = clamp(t.scroll, 0, max(total-r.Size.W+1, 0))
	if caretX-t.scroll > r.Size.W-1 {
		t.scroll = caretX - r.Size.W + 1
	}
	if caretX-t.scroll < 0 {
		t.scroll = caretX
	}
	x0 := r.Origin.X - t.scroll
	t.caretPx = image.Rect(
		int(dst.px(x0+caretX)), int(dst.px(r.Origin.Y)),
		int(dst.px(x0+caretX))+1, int(dst.px(r.Origin.Y+h)),
	)
	if dst == nil || dst.Image == nil {
		return
	}
	clip := dst.Clip(r)

	if t.focused && t.composition == "" && t.ed.hasSelection() {
		lo, hi := t.ed.selection()
		a, b := t.advance(t.ed.text[:lo]), t.advance(t.ed.text[:hi])
		clip.FillRect(Rct(Pt(x0+a, r.Origin.Y), Sz(b-a, h)), t.selection)
	}

	op := &text.DrawOptions{}
	op.GeoM.Translate(dst.px(x0), dst.px(r.Origin.Y))
	if shown == "" && t.placeholder != "" {
		op.ColorScale.ScaleWithColor(t.muted)
		text.Draw(clip.Image, t.placeholder, t.face(dst.Scale()), op)
	} else {
		op.ColorScale.ScaleWithColor(t.resolved.Color)
		text.Draw(clip.Image, t.display(shown), t.face(dst.Scale()), op)
	}

	if t.composition != "" {
		lo, _ := t.ed.selection()
		a := t.advance(t.ed.text[:lo])
		b := a + t.advance(t.composition)
		y := r.Origin.Y + h - 1
		clip.FillRect(Rct(Pt(x0+a, y), Sz(b-a, 1)), t.resolved.Color)
	}

	if t.focused && (time.Since(t.blink)/(530*time.Millisecond))%2 == 0 {
		clip.FillRect(Rct(Pt(x0+caretX, r.Origin.Y), Sz(1, h)), t.resolved.Color)
	}
}

// indexAt returns the byte offset in the text nearest to logical x.
func (t *TextInputWidget) indexAt(x float64) int {
	local := x - t.rect.Origin.X + t.scroll
	best, bestDist := 0, local
	if bestDist < 0 {
		bestDist = -bestDist
	}
	for i := 0; i <= len(t.ed.text); i = nextRune(t.ed.text, i) {
		d := t.advance(t.ed.text[:i]) - local
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			best, bestDist = i, d
		}
		if i == len(t.ed.text) {
			break
		}
	}
	return best
}

// commit writes the editor's text to the signal after an edit.
func (t *TextInputWidget) commit() {
	t.blink = time.Now()
	if t.ed.text == t.value.Peek() {
		return
	}
	t.value.Set(t.ed.text)
	if t.onChange != nil {
		t.onChange(t.ed.text)
	}
}

// ime is the editor's view of the platform IME. The real one wraps
// textinput.Composer; tests substitute a fake that feeds compositions and
// commits by hand.
type ime interface {
	Update() (handled bool, err error)
	Confirm()
	Cancel()
}

// newIME builds the IME driver for a new editor. Tests replace it.
var newIME = func(t *TextInputWidget) ime {
	c := &composerIME{}
	c.OnNewSession = t.imeSession
	c.OnComposition = func(comp *textinput.Composition) {
		start, _ := comp.SelectionRangeInBytes()
		t.imeComposition(comp.Text(), start)
	}
	c.OnCommit = func(commit *textinput.Commit) {
		if before, after := commit.IsSurroundingTextReplaced(); before || after {
			nb, na := commit.SurroundingText()
			t.imeReplace(nb, commit.Text(), na)
			return
		}
		t.imeCommit(commit.Text())
	}
	c.OnEndByUser = func() { t.focused = false }
	return c
}

// composerIME drives textinput.Composer, but only inside a running game:
// the package panics when used before Ebitengine has chosen its backend.
type composerIME struct {
	textinput.Composer
}

func (c *composerIME) Update() (bool, error) {
	if !appRunning.Load() {
		return false, nil
	}
	return c.Composer.Update()
}

// IME callbacks, after examples/textinput in the Ebitengine repository.

func (t *TextInputWidget) imeSession() *textinput.SessionOptions {
	lo, hi := t.ed.selection()
	t.imeStart, t.imeEnd = 0, len(t.ed.text)
	return &textinput.SessionOptions{
		CaretBounds:     t.caretPx,
		TextBeforeCaret: t.ed.text[:lo],
		TextAfterCaret:  t.ed.text[hi:],
	}
}

// imeComposition shows preedit text at the caret; "" clears it.
func (t *TextInputWidget) imeComposition(text string, caret int) {
	t.composition, t.compCaret = text, caret
	t.blink = time.Now()
}

// imeCommit inserts committed text in place of the selection.
func (t *TextInputWidget) imeCommit(text string) {
	t.ed.replace(text)
	t.commit()
}

// imeReplace applies a commit that rewrote the surrounding text the session
// was given: before + text + after replace that whole range.
func (t *TextInputWidget) imeReplace(before, text, after string) {
	t.ed.text = t.ed.text[:t.imeStart] + before + text + after + t.ed.text[t.imeEnd:]
	t.ed.moveTo(t.imeStart+len(before)+len(text), false)
	t.commit()
}

// HandleTick implements TickHandler: it runs the IME while focused.
func (t *TextInputWidget) HandleTick() bool {
	if !t.focused {
		return false
	}
	handled, err := t.ime.Update()
	if err != nil {
		t.imeErr = err
	}
	return handled
}

// HandleKey implements KeyHandler.
func (t *TextInputWidget) HandleKey(ev KeyEvent) {
	switch ev.Kind {
	case KeyFocus:
		t.focused = true
		t.blink = time.Now()
	case KeyBlur:
		t.ime.Confirm()
		t.focused = false
	case KeyText:
		// Text arrives through the IME, on every platform.
	case KeyPress:
		t.key(ev.Key, ev.Mods)
	}
}

func (t *TextInputWidget) key(k ebiten.Key, m Mods) {
	word := m.Alt || (!m.Cmd() && m.Ctrl)
	switch k {
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		t.ime.Confirm()
		if t.onSubmit != nil {
			t.onSubmit(t.ed.text)
		}
		return
	case ebiten.KeyEscape:
		t.ime.Cancel()
		t.ed.moveTo(t.ed.caret, false)
	case ebiten.KeyBackspace:
		t.ime.Confirm()
		t.ed.backspace(word)
	case ebiten.KeyDelete:
		t.ime.Confirm()
		t.ed.deleteForward(word)
	case ebiten.KeyArrowLeft, ebiten.KeyArrowRight:
		t.ime.Confirm()
		dir := pick(k == ebiten.KeyArrowLeft, -1, 1)
		if m.Meta {
			t.ed.moveTo(pick(dir < 0, 0, len(t.ed.text)), m.Shift)
		} else {
			t.ed.moveBy(dir, word, m.Shift)
		}
	case ebiten.KeyHome:
		t.ime.Confirm()
		t.ed.moveTo(0, m.Shift)
	case ebiten.KeyEnd:
		t.ime.Confirm()
		t.ed.moveTo(len(t.ed.text), m.Shift)
	case ebiten.KeyA:
		if m.Cmd() {
			t.ime.Confirm()
			t.ed.selectAll()
		}
	case ebiten.KeyC:
		if m.Cmd() && t.ed.hasSelection() && !t.password {
			currentClipboard().Write(t.ed.selected())
		}
	case ebiten.KeyX:
		if m.Cmd() && t.ed.hasSelection() {
			t.ime.Confirm()
			if !t.password {
				currentClipboard().Write(t.ed.selected())
			}
			t.ed.replace("")
		}
	case ebiten.KeyV:
		if m.Cmd() {
			t.ime.Confirm()
			s := strings.ReplaceAll(currentClipboard().Read(), "\n", " ")
			s = strings.ReplaceAll(s, "\r", "")
			t.ed.replace(s)
		}
	default:
		return
	}
	t.commit()
}

// HandlePointer implements PointerHandler: clicks place the caret, drags
// and repeated clicks select.
func (t *TextInputWidget) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerDown:
		if ev.Button != ebiten.MouseButtonLeft {
			return false
		}
		t.ime.Confirm()
		now := time.Now()
		if now.Sub(t.lastClick) < 400*time.Millisecond && near(ev.Pos, t.lastPos) {
			t.clicks++
		} else {
			t.clicks = 1
		}
		t.lastClick, t.lastPos = now, ev.Pos
		idx := t.indexAt(ev.Pos.X)
		switch t.clicks {
		case 1:
			t.ed.moveTo(idx, false)
		case 2:
			t.ed.selectWord(idx)
		default:
			t.ed.selectAll()
		}
		t.blink = now
		return true
	case PointerDrag:
		if t.clicks == 1 {
			t.ed.moveTo(t.indexAt(ev.Pos.X), true)
			t.blink = time.Now()
		}
		return true
	case PointerUp, PointerTap, PointerMove, PointerEnter, PointerExit:
		return true
	}
	return false
}

func near(a, b Point) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx+dy*dy < 16
}

// Adopt implements Adopter: a rebuilt editor at the same Rect carries on
// with the previous one's focus, caret and selection.
func (t *TextInputWidget) Adopt(prev any) {
	p, ok := prev.(*TextInputWidget)
	if !ok {
		return
	}
	p.ime.Confirm()
	t.ed, t.scroll, t.focused = p.ed, p.scroll, p.focused
	t.clicks, t.lastClick, t.lastPos = p.clicks, p.lastClick, p.lastPos
	if p.ed.text != t.value.Peek() {
		t.ed.setText(t.value.Peek())
	}
	t.blink = time.Now()
}
