package ggui

import (
	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/reactive"

	"image"
	"image/color"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/textinput"
)

// textEditor is the model behind TextInput: a string and a selection. The
// caret sits at caret; anchor is where the selection started, so anchor ==
// caret means no selection. Offsets are bytes on rune boundaries.
type textEditor struct {
	text   string
	anchor int
	caret  int

	// Undo history: snapshots taken before each edit, and the ones undone.
	// Insertions typed in quick succession share one snapshot.
	undo, redo []editSnapshot
	lastEdit   time.Time
	lastInsert bool
}

// editSnapshot is the editor's state before an edit.
type editSnapshot struct {
	text          string
	anchor, caret int
}

// maxUndo bounds the history.
const maxUndo = 200

// undoCoalesce is how close two typed insertions must be to undo as one.
const undoCoalesce = 700 * time.Millisecond

// record takes a snapshot before an edit that replaces the selection with
// s. A plain insertion right after another joins the previous step, so a
// word typed undoes at once, while deletions and pastes stand alone.
func (e *textEditor) record(s string) {
	now := clock()
	insert := s != "" && !e.hasSelection() && utf8.RuneCountInString(s) == 1
	if insert && e.lastInsert && now.Sub(e.lastEdit) < undoCoalesce && len(e.undo) > 0 {
		e.lastEdit = now
		return
	}
	e.undo = append(e.undo, editSnapshot{e.text, e.anchor, e.caret})
	if len(e.undo) > maxUndo {
		e.undo = e.undo[1:]
	}
	e.redo = e.redo[:0]
	e.lastEdit, e.lastInsert = now, insert
}

// Undo restores the state before the last edit and reports whether there
// was one.
func (e *textEditor) Undo() bool {
	if len(e.undo) == 0 {
		return false
	}
	e.redo = append(e.redo, editSnapshot{e.text, e.anchor, e.caret})
	e.restore(e.undo[len(e.undo)-1])
	e.undo = e.undo[:len(e.undo)-1]
	return true
}

// Redo reapplies the last undone edit and reports whether there was one.
func (e *textEditor) Redo() bool {
	if len(e.redo) == 0 {
		return false
	}
	e.undo = append(e.undo, editSnapshot{e.text, e.anchor, e.caret})
	e.restore(e.redo[len(e.redo)-1])
	e.redo = e.redo[:len(e.redo)-1]
	return true
}

func (e *textEditor) restore(s editSnapshot) {
	e.text, e.anchor, e.caret = s.text, s.anchor, s.caret
	e.lastInsert = false
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
	e.record(s)
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
		pos = prevGrapheme(e.text, pos)
	case word:
		pos = nextWord(e.text, pos)
	default:
		pos = nextGrapheme(e.text, pos)
	}
	e.moveTo(pos, extend)
}

// backspace deletes the selection, or the grapheme or word before the caret.
func (e *textEditor) backspace(word bool) {
	if !e.hasSelection() {
		e.anchor = pick(word, prevWord(e.text, e.caret), prevGrapheme(e.text, e.caret))
	}
	e.replace("")
}

// deleteForward deletes the selection, or the grapheme or word after the caret.
func (e *textEditor) deleteForward(word bool) {
	if !e.hasSelection() {
		e.anchor = pick(word, nextWord(e.text, e.caret), nextGrapheme(e.text, e.caret))
	}
	e.replace("")
}

// Grapheme clusters, approximately: the caret and Backspace step over a
// base rune together with what attaches to it. Without a segmentation
// table this covers what shows up in practice: combining marks, variation
// selectors, emoji modifiers and tags, zero-width-joiner sequences, CRLF,
// and regional indicator pairs. Conjoining Hangul jamo are handled too,
// though text from an IME arrives precomposed.

// extends reports whether r attaches to the rune before it.
func extends(r rune) bool {
	switch {
	case unicode.Is(unicode.M, r): // combining marks
		return true
	case r >= 0xFE00 && r <= 0xFE0F, r >= 0xE0100 && r <= 0xE01EF: // variation selectors
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF: // emoji skin tones
		return true
	case r >= 0xE0020 && r <= 0xE007F: // emoji tags
		return true
	case r == 0x200D, r == 0x200C: // zero-width joiner and non-joiner
		return true
	case r >= 0x1160 && r <= 0x11FF: // Hangul jamo vowels and trailing consonants
		return true
	}
	return false
}

func isRegionalIndicator(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

// nextGrapheme returns the byte offset after the cluster starting at i.
func nextGrapheme(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	r, n := utf8.DecodeRuneInString(s[i:])
	j := i + n
	if r == '\r' && j < len(s) && s[j] == '\n' {
		return j + 1
	}
	if isRegionalIndicator(r) {
		if r2, n2 := utf8.DecodeRuneInString(s[j:]); isRegionalIndicator(r2) {
			return j + n2
		}
		return j
	}
	for j < len(s) {
		r2, n2 := utf8.DecodeRuneInString(s[j:])
		if !extends(r2) {
			break
		}
		j += n2
		if r2 == 0x200D && j < len(s) {
			// What follows a joiner belongs to the cluster.
			_, n3 := utf8.DecodeRuneInString(s[j:])
			j += n3
		}
	}
	return j
}

// prevGrapheme returns the byte offset of the cluster ending at i.
func prevGrapheme(s string, i int) int {
	if i <= 0 {
		return 0
	}
	if s[i-1] == '\n' {
		if i >= 2 && s[i-2] == '\r' {
			return i - 2
		}
		return i - 1
	}
	// Walk clusters from the start of the line; text fields are short.
	start := i
	for start > 0 && s[start-1] != '\n' {
		start--
	}
	for j := start; j < i; {
		k := nextGrapheme(s, j)
		if k >= i {
			return j
		}
		j = k
	}
	return prevRune(s, i)
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

// TextInputWidget is a text editor bound to a StateValue[string]: typing writes
// the signal, and writing the signal updates the text. Build one with
// TextInput. It is one line that scrolls sideways until Multiline makes it
// wrap and grow. It draws only the text, selection and caret; TextField
// adds the themed box around it.
//
// Text comes in through the platform IME (a patched copy of Ebitengine's
// exp/textinput, in internal/textinput), so composed
// scripts such as Korean and Japanese work, with the composition shown
// underlined in place. Keys: arrows (with Shift to select, Alt or Ctrl to
// jump words, ⌘ on macOS to reach the ends), Home and End, Backspace and
// Delete, ⌘/Ctrl+A, C, X and V, Enter for OnSubmit. Double-click selects a
// word, triple-click everything, dragging selects a range. A Multiline
// editor adds Up and Down, Home and End within the line, Enter for a line
// break and ⌘/Ctrl+Enter for OnSubmit.
type TextInputWidget struct {
	props property.
		// Interactive carries the identity, name and disabled state every
		// control shares. The editor drives focus itself rather than through
		// Keyboard: a caret and an IME session are not a press.
		Owner

	Interactive

	value             Binding[string]
	placeholder       string
	style             TextStyle
	password          bool
	multiline         bool
	minLines          int
	minWidth          float64
	onSubmit          func(string)
	onCommit          func(string)
	onChange          func(string)
	onKey             func(KeyEvent) bool
	filter            func(string) string
	escapeUsed        bool
	inheritedDisabled bool

	ed textEditor

	ime         ime
	composition string
	compCaret   int // caret inside composition, in bytes
	imeStart    int // the byte range of ed.text the IME was told about
	imeEnd      int
	imeErr      error

	scroll    float64 // how far the text is shifted left, or up when multiline, to show the caret
	width     float64 // the wrap width from the last Layout, when multiline
	blink     time.Time
	clicks    int
	lastClick time.Time
	lastPos   Point

	cache     *CachedWidget
	resolved  TextStyle
	muted     color.Color
	selection color.Color
	rect      Rect            // where it was last painted, in logical pixels
	scale     float64         // the Canvas scale at that paint
	caretPx   image.Rectangle // the caret in screen pixels, for the IME
}

// TextInput creates an editor bound to value.
func TextInput(value Binding[string]) *TextInputWidget {
	t := &TextInputWidget{value: value, minWidth: 120}
	t.Role = RoleTextField
	t.AutoKey()
	t.ime = newIME(t)
	t.ed.setText(Untrack(value.Get))
	t.ed.moveTo(len(t.ed.text), false)
	// Follow the binding rather than polling it in Layout: a write from
	// outside reaches the editor even when the layout above it is cached.
	// The effect's first run is the value it was just built with, and the
	// editor's own commit writes what ed already holds, so both are no-ops.
	reactive.Observe(func() {
		v := value.Get()
		if v == t.ed.text {
			return
		}
		t.ed.setText(v)
		t.cache.invalidate()
	})
	return t
}

// BindDisabled follows r for Disabled without a rebuild. Like every other
// control the editor reads r through Sync in Layout and Paint: Disabled
// changes colour and whether input is accepted, both settled in Paint, so
// nothing has to be measured again.
func (t *TextInputWidget) BindDisabled(r Readable[bool]) *TextInputWidget {
	t.BindInert(r)
	return t
}

// Disabled shows the text in the muted color and takes no input while v
// is true.
func (t *TextInputWidget) Disabled(v bool) *TextInputWidget {
	t.SetInert(v)
	return t
}

// Placeholder sets the muted text shown while the value is empty.
func (t *TextInputWidget) Placeholder(s string) *TextInputWidget {
	defer property.Watch(&t.props, &t.placeholder)()
	t.placeholder = s
	return t
}

// Name names the field for Probe.Find and the inspector; the placeholder
// serves until one is set.
// SetName and HasName come from Interactive, so a container that names what
// it holds reaches the editor the same way it reaches any other control.
func (t *TextInputWidget) Name(s string) *TextInputWidget { t.SetName(s); return t }

// BindName binds the field's name to r; see Interactive.BindName.
func (t *TextInputWidget) BindName(r Readable[string]) *TextInputWidget {
	t.Interactive.BindName(r)
	return t
}

// IsDisabled reports the effective state, including InputDisabled inherited at
// the most recent Layout. It does not subscribe to the disabled binding.
func (t *TextInputWidget) IsDisabled() bool { return t.IsInert() || t.inheritedDisabled }

// Key gives the editor an identity, so a rebuilt one that also moved keeps
// its caret and focus. Without one the keyed component it was built in
// identifies it, else its Rect.
func (t *TextInputWidget) Key(k any) *TextInputWidget { t.Interactive.SetKey(k); return t }

// Semantics implements Semantic.
func (t *TextInputWidget) Semantics() (Role, string) {
	if t.SemanticName() != "" {
		return RoleTextField, t.SemanticName()
	}
	return RoleTextField, t.placeholder
}

// Describe implements Describer: the field, its contents and whether it
// takes input. ui.TextField describes the padded box it draws around this
// editor with the same handler, which is why the two are one node and not
// two: the first description of a handler in a frame is the one kept.
func (t *TextInputWidget) Describe() Node {
	role, name := t.Semantics()
	lo, hi := t.ed.selection()
	n := Node{
		Role:     role,
		Name:     name,
		Value:    t.ed.text,
		Disabled: t.IsDisabled(),
		Actions:  ActionFocus | ActionSetValue | ActionSetSelection,
		SelStart: lo,
		SelEnd:   hi,
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
func (t *TextInputWidget) runs() []TextRun {
	spans := t.spans(t.ed.text)
	out := make([]TextRun, 0, len(spans))
	h := t.height()
	for i, sp := range spans {
		x, y := t.rect.Origin.X, t.rect.Origin.Y
		if t.multiline {
			y += float64(i)*t.spacing() - t.scroll
		} else {
			x -= t.scroll
		}
		line := t.ed.text[sp.start:sp.end]
		r := TextRun{Start: sp.start, End: sp.end, Rect: Rct(Pt(x, y), Sz(t.advance(line), h))}
		for b := sp.start; ; b = nextRune(t.ed.text, b) {
			r.Stops = append(r.Stops, TextStop{Byte: b, X: t.advance(t.ed.text[sp.start:b])})
			if b >= sp.end {
				break
			}
		}
		out = append(out, r)
	}
	return out
}

// Act implements Actor: the platform's text API replaces the contents
// outright, which is what a dictation or a braille display does, and moves
// the caret, which is what a screen reader does as it reads along.
func (t *TextInputWidget) Act(a Action) bool {
	if t.IsDisabled() {
		return false
	}
	switch a.Kind {
	case ActionSetValue:
		t.ed.setText(a.Text)
		t.commit()
		return true
	case ActionSetSelection:
		t.ed.moveTo(a.SelStart, false)
		t.ed.moveTo(a.SelEnd, true)
		return true
	}
	return false
}

// ConsumesKey implements KeyConsumer: editing keys stay with the editor.
// Escape passes through unless it cancelled composition or OnKey consumed it.
func (t *TextInputWidget) ConsumesKey(ev KeyEvent) bool {
	return ev.Kind == KeyPress && (ev.Key != KeyEscape || t.escapeUsed)
}

// Password masks every rune with a bullet.
func (t *TextInputWidget) Password() *TextInputWidget { t.password = true; return t }

// Style merges ts onto the widget's own text style.
func (t *TextInputWidget) Style(ts TextStyle) *TextInputWidget {
	defer property.Watch(&t.props, &t.style)()
	t.style = t.style.Merge(ts)
	return t
}

// MinWidth sets the width the editor asks for when its parent leaves the
// width to it; it fills a bounded width.
func (t *TextInputWidget) MinWidth(w float64) *TextInputWidget {
	defer property.Watch(&t.props, &t.minWidth)()
	t.minWidth = w
	return t
}

// Multiline wraps the text at the editor's width and grows it by the line,
// starting at three lines tall; see Lines. Enter inserts a line break and
// ⌘/Ctrl+Enter submits.
func (t *TextInputWidget) Multiline() *TextInputWidget {
	t.multiline = true
	if t.minLines == 0 {
		t.minLines = 3
	}
	return t
}

// Lines sets the fewest lines a Multiline editor is tall, and makes the
// editor Multiline.
func (t *TextInputWidget) Lines(n int) *TextInputWidget {
	defer property.Watch(&t.props, &t.minLines)()
	defer property.Watch(&t.props, &t.multiline)()
	t.multiline, t.minLines = true, max(n, 1)
	return t
}

// OnSubmit fires with the value when Enter is pressed.
func (t *TextInputWidget) OnSubmit(fn func(string)) *TextInputWidget { t.onSubmit = fn; return t }

// OnCommit fires with the value when the editor loses focus or submits,
// for work too costly to do on every keystroke.
func (t *TextInputWidget) OnCommit(fn func(string)) *TextInputWidget { t.onCommit = fn; return t }

func (t *TextInputWidget) committed() {
	if t.onCommit != nil {
		t.onCommit(t.ed.text)
	}
}

// OnChange fires with the value after every edit, after the signal is set.
func (t *TextInputWidget) OnChange(fn func(string)) *TextInputWidget { t.onChange = fn; return t }

// OnKey handles a key press before editor commands when no IME composition is
// active. Return true to consume it. Focus, text and IME events stay with the
// editor. This lets searchable lists use Up, Down and Enter without replacing
// the editor's platform input driver.
func (t *TextInputWidget) OnKey(fn func(KeyEvent) bool) *TextInputWidget { t.onKey = fn; return t }

// Focused reports whether the editor has keyboard focus.
func (t *TextInputWidget) Focused() bool { return t.Interactive.Focused }

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
func (t *TextInputWidget) advance(s string) float64 { return lineWidth(t.display(s), t.face(1)) }

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

// Baseline implements Baseliner: the first line's, an ascent below the top.
func (t *TextInputWidget) Baseline() (float64, bool) {
	if t.resolved.Font == nil {
		return 0, false
	}
	return t.face(1).Metrics().HAscent, true
}

// spacing is the distance between baselines when multiline.
func (t *TextInputWidget) spacing() float64 {
	st := t.resolved
	if st.Font == nil {
		st = t.style.resolved()
	}
	return st.Size * st.LineHeight
}

// spans wraps s at the editor's width, or leaves it one line.
func (t *TextInputWidget) spans(s string) []lineSpan {
	if !t.multiline {
		return []lineSpan{{0, len(s)}}
	}
	return wrapSpans(s, t.face(1), t.width)
}

// linesHeight is the height of n wrapped lines.
func (t *TextInputWidget) linesHeight(n int) float64 {
	return float64(max(n, 1)-1)*t.spacing() + t.height()
}

// Layout implements Widget.
func (t *TextInputWidget) Layout(c Constraints, env Env) Size {
	defer t.props.Layout()()
	t.Sync()
	t.inheritedDisabled, _ = env.Get(InputDisabled)
	t.resolved = env.Text().Merge(t.style).resolved()
	t.resolved.Size *= env.TextScale()
	t.cache, _ = env.Get(cacheOwner)
	th := env.Theme()
	if t.IsDisabled() {
		t.resolved.Color = th.MutedFg
	}
	t.muted, t.selection = th.MutedFg, th.Selection
	w := bounded(c.MaxW, t.minWidth)
	if !t.multiline {
		return c.Constrain(Sz(w, t.height()))
	}
	t.width = max(c.MinW, w)
	n := max(len(t.spans(t.ed.text)), t.minLines)
	return c.Constrain(Sz(t.width, t.linesHeight(n)))
}

// Paint implements Widget.
func (t *TextInputWidget) Paint(dst *Canvas, r Rect) {
	t.Sync()
	// Where it is being painted is recorded first: describing it reports
	// where its characters are, which is measured from here.
	t.rect, t.scale = r, dst.Scale()
	// A disabled editor takes no input but is still read out.
	dst.Describe(r, t)
	if !t.IsDisabled() {
		dst.HitPointer(r, t)
		dst.HitKey(r, t)
		dst.HitCursor(r, CursorShapeText)
	}
	if t.multiline {
		t.paintLines(dst, r)
		return
	}

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

	if t.Focused() && t.composition == "" && t.ed.hasSelection() {
		lo, hi := t.ed.selection()
		a, b := t.advance(t.ed.text[:lo]), t.advance(t.ed.text[:hi])
		clip.FillRect(Rct(Pt(x0+a, r.Origin.Y), Sz(b-a, h)), t.selection)
	}

	op := &text.DrawOptions{}
	op.GeoM.Translate(dst.px(x0), dst.px(r.Origin.Y))
	if shown == "" && t.placeholder != "" {
		op.ColorScale.ScaleWithColor(t.muted)
		drawText(clip.Image, t.placeholder, t.face(dst.Scale()), op)
	} else {
		op.ColorScale.ScaleWithColor(t.resolved.Color)
		drawText(clip.Image, t.display(shown), t.face(dst.Scale()), op)
	}

	if t.composition != "" {
		lo, _ := t.ed.selection()
		a := t.advance(t.ed.text[:lo])
		b := a + t.advance(t.composition)
		y := r.Origin.Y + h - 1
		clip.FillRect(Rct(Pt(x0+a, y), Sz(b-a, 1)), t.resolved.Color)
	}

	if t.Focused() && (Now().Sub(t.blink)/(530*time.Millisecond))%2 == 0 {
		clip.FillRect(Rct(Pt(x0+caretX, r.Origin.Y), Sz(1, h)), t.resolved.Color)
	}
}

// paintLines is Paint for a Multiline editor: wrapped lines that scroll
// vertically to keep the caret in view.
func (t *TextInputWidget) paintLines(dst *Canvas, r Rect) {
	t.width = r.Size.W
	shown, caret := t.rendered()
	spans := t.spans(shown)
	li := lineOf(spans, caret)
	caretX := t.advance(shown[spans[li].start:caret])
	caretY := float64(li) * t.spacing()
	h := t.height()
	t.scroll = clamp(t.scroll, 0, max(t.linesHeight(len(spans))-r.Size.H, 0))
	if caretY+h-t.scroll > r.Size.H {
		t.scroll = caretY + h - r.Size.H
	}
	if caretY-t.scroll < 0 {
		t.scroll = caretY
	}
	x0, y0 := r.Origin.X, r.Origin.Y-t.scroll
	t.caretPx = image.Rect(
		int(dst.px(x0+caretX)), int(dst.px(y0+caretY)),
		int(dst.px(x0+caretX))+1, int(dst.px(y0+caretY+h)),
	)
	if dst == nil || dst.Image == nil {
		return
	}
	clip := dst.Clip(r)
	lineY := func(i int) float64 { return y0 + float64(i)*t.spacing() }

	// eachLine calls fn with the part of [lo, hi) that falls on each line,
	// as x offsets, extending to the line's end when the range runs on.
	eachLine := func(lo, hi int, fn func(i int, a, b float64)) {
		for i, sp := range spans {
			if hi < sp.start || lo > sp.end {
				continue
			}
			a, b := max(lo, sp.start), min(hi, sp.end)
			ax, bx := t.advance(shown[sp.start:a]), t.advance(shown[sp.start:b])
			if hi > sp.end && i+1 < len(spans) {
				bx += t.advance(" ")
			}
			fn(i, ax, bx)
		}
	}

	if t.Focused() && t.composition == "" && t.ed.hasSelection() {
		lo, hi := t.ed.selection()
		eachLine(lo, hi, func(i int, a, b float64) {
			clip.FillRect(Rct(Pt(x0+a, lineY(i)), Sz(b-a, h)), t.selection)
		})
	}

	op := &text.DrawOptions{}
	if shown == "" && t.placeholder != "" {
		op.GeoM.Translate(dst.px(x0), dst.px(y0))
		op.ColorScale.ScaleWithColor(t.muted)
		drawText(clip.Image, t.placeholder, t.face(dst.Scale()), op)
	} else {
		op.ColorScale.ScaleWithColor(t.resolved.Color)
		for i, sp := range spans {
			if lineY(i)+h < r.Origin.Y || lineY(i) > r.Origin.Y+r.Size.H {
				continue
			}
			op.GeoM.Reset()
			op.GeoM.Translate(dst.px(x0), dst.px(lineY(i)))
			drawText(clip.Image, t.display(shown[sp.start:sp.end]), t.face(dst.Scale()), op)
		}
	}

	if t.composition != "" {
		lo, _ := t.ed.selection()
		eachLine(lo, lo+len(t.composition), func(i int, a, b float64) {
			clip.FillRect(Rct(Pt(x0+a, lineY(i)+h-1), Sz(b-a, 1)), t.resolved.Color)
		})
	}

	if t.Focused() && (Now().Sub(t.blink)/(530*time.Millisecond))%2 == 0 {
		clip.FillRect(Rct(Pt(x0+caretX, y0+caretY), Sz(1, h)), t.resolved.Color)
	}
}

// indexAt returns the byte offset in the text nearest to a logical point.
func (t *TextInputWidget) indexAt(p Point) int {
	spans := t.spans(t.ed.text)
	li := 0
	if t.multiline {
		li = clamp(int((p.Y-t.rect.Origin.Y+t.scroll)/t.spacing()), 0, len(spans)-1)
	}
	x := p.X - t.rect.Origin.X
	if !t.multiline {
		x += t.scroll
	}
	return t.indexInLine(spans[li], x)
}

// indexInLine returns the grapheme boundary within sp nearest x. Measuring
// partial emoji sequences can give the same width as the whole glyph, so
// rune boundaries would put the caret inside a joined emoji or modifier.
func (t *TextInputWidget) indexInLine(sp lineSpan, x float64) int {
	best, bestDist := sp.start, math.Inf(1)
	for i := sp.start; ; i = nextGrapheme(t.ed.text, i) {
		d := math.Abs(t.advance(t.ed.text[sp.start:i]) - x)
		if d < bestDist {
			best, bestDist = i, d
		}
		if i >= sp.end {
			break
		}
	}
	return best
}

// moveLine moves the caret to the nearest position on the line above
// (dir < 0) or below, keeping its x.
func (t *TextInputWidget) moveLine(dir int, extend bool) {
	spans := t.spans(t.ed.text)
	li := lineOf(spans, t.ed.caret)
	x := t.advance(t.ed.text[spans[li].start:t.ed.caret])
	to := li + dir
	switch {
	case to < 0:
		t.ed.moveTo(0, extend)
	case to >= len(spans):
		t.ed.moveTo(len(t.ed.text), extend)
	default:
		t.ed.moveTo(t.indexInLine(spans[to], x), extend)
	}
}

// lineBounds returns the start and end of the line the caret is on.
func (t *TextInputWidget) lineBounds() (int, int) {
	spans := t.spans(t.ed.text)
	sp := spans[lineOf(spans, t.ed.caret)]
	return sp.start, sp.end
}

// commit writes the editor's text to the signal after an edit.
func (t *TextInputWidget) commit() {
	t.blink = Now()
	if t.filter != nil {
		// Map both selection boundaries through the same normalization, so
		// removed characters cannot leave the caret beyond the accepted text.
		a, c := len(t.filter(t.ed.text[:t.ed.anchor])), len(t.filter(t.ed.text[:t.ed.caret]))
		t.ed.setText(t.filter(t.ed.text))
		t.ed.anchor, t.ed.caret = t.ed.snap(a), t.ed.snap(c)
	}
	if t.ed.text == Untrack(t.value.Get) {
		return
	}
	t.value.Set(t.ed.text)
	if t.multiline {
		t.cache.invalidate()
	}
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
	c.OnEndByUser = func() { t.Interactive.Focused = false }
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
	t.blink = Now()
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
	if !t.Focused() || t.IsDisabled() {
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
	if t.IsDisabled() && ev.Kind != KeyBlur {
		return
	}
	switch ev.Kind {
	case KeyFocus:
		t.Interactive.Focused = true
		t.blink = Now()
	case KeyBlur:
		t.ime.Confirm()
		t.Interactive.Focused = false
		t.committed()
	case KeyText:
		// Text arrives through the IME, on every platform.
	case KeyPress:
		t.escapeUsed = ev.Key == KeyEscape && t.composition != ""
		if t.composition == "" && t.onKey != nil && t.onKey(ev) {
			t.escapeUsed = ev.Key == KeyEscape
			return
		}
		t.key(ev.Key, ev.Mods)
	}
}

// wordWise reports whether m asks for the word-wise form of a motion or a
// delete: Alt anywhere, or Ctrl where it is not standing in for Cmd.
func wordWise(m Mods) bool { return m.Alt || (!m.Cmd() && m.Ctrl) }

// key applies one key press. It is split by what the key does, and each
// group reports whether anything it changed has to reach the binding: a key
// the editor does not act on, or one that bailed out (Undo with nothing to
// undo, an arrow in a single-line field), leaves the value alone.
func (t *TextInputWidget) key(k KeyboardKey, m Mods) {
	if t.editKey(k, m) || t.moveKey(k, m) || t.clipboardKey(k, m) || t.historyKey(k, m) {
		t.commit()
	}
}

// editKey handles the keys that change the text: Enter, Escape and the two
// deletes. Enter in a single-line field submits instead, which commits
// through committed rather than through the caller.
func (t *TextInputWidget) editKey(k KeyboardKey, m Mods) bool {
	switch k {
	case KeyEnter, KeyNumpadEnter:
		t.ime.Confirm()
		if t.multiline && !m.Cmd() {
			t.ed.replace("\n")
			return true
		}
		if t.onSubmit != nil {
			t.onSubmit(t.ed.text)
		}
		t.committed()
	case KeyEscape:
		t.ime.Cancel()
		t.ed.moveTo(t.ed.caret, false)
		return true
	case KeyBackspace:
		t.ime.Confirm()
		t.ed.backspace(wordWise(m))
		return true
	case KeyDelete:
		t.ime.Confirm()
		t.ed.deleteForward(wordWise(m))
		return true
	}
	return false
}

// moveKey handles the keys that move the caret. Shift extends the selection
// and Meta jumps to the ends of the text; in a multiline field Up and Down
// step by line and Home and End stay within one.
func (t *TextInputWidget) moveKey(k KeyboardKey, m Mods) bool {
	switch k {
	case KeyArrowUp, KeyArrowDown:
		if !t.multiline {
			return false
		}
		t.ime.Confirm()
		if m.Meta {
			t.ed.moveTo(pick(k == KeyArrowUp, 0, len(t.ed.text)), m.Shift)
		} else {
			t.moveLine(pick(k == KeyArrowUp, -1, 1), m.Shift)
		}
		return true
	case KeyArrowLeft, KeyArrowRight:
		t.ime.Confirm()
		dir := pick(k == KeyArrowLeft, -1, 1)
		if m.Meta {
			t.ed.moveTo(pick(dir < 0, 0, len(t.ed.text)), m.Shift)
		} else {
			t.ed.moveBy(dir, wordWise(m), m.Shift)
		}
		return true
	case KeyHome, KeyEnd:
		t.ime.Confirm()
		lo, hi := 0, len(t.ed.text)
		if t.multiline && !m.Cmd() {
			lo, hi = t.lineBounds()
		}
		t.ed.moveTo(pick(k == KeyHome, lo, hi), m.Shift)
		return true
	}
	return false
}

// clipboardKey handles the Cmd/Ctrl shortcuts that move text in and out of
// the field: select all, copy, cut and paste. A password field is never
// copied out of, though cutting still deletes.
func (t *TextInputWidget) clipboardKey(k KeyboardKey, m Mods) bool {
	switch k {
	case KeyA:
		if m.Cmd() {
			t.ime.Confirm()
			t.ed.selectAll()
		}
		return true
	case KeyC:
		if m.Cmd() && t.ed.hasSelection() && !t.password {
			currentClipboard().Write(t.ed.selected())
		}
		return true
	case KeyX:
		if m.Cmd() && t.ed.hasSelection() {
			t.ime.Confirm()
			if !t.password {
				currentClipboard().Write(t.ed.selected())
			}
			t.ed.replace("")
		}
		return true
	case KeyV:
		if m.Cmd() {
			t.ime.Confirm()
			s := strings.ReplaceAll(currentClipboard().Read(), "\r\n", "\n")
			s = strings.ReplaceAll(s, "\r", "\n")
			if !t.multiline {
				s = strings.ReplaceAll(s, "\n", " ")
			}
			t.ed.replace(s)
		}
		return true
	}
	return false
}

// historyKey handles the Cmd/Ctrl shortcuts for undo and redo. Both report
// false with nothing left on the stack, so an exhausted history writes
// nothing back.
func (t *TextInputWidget) historyKey(k KeyboardKey, m Mods) bool {
	if !m.Cmd() {
		return false
	}
	switch k {
	case KeyZ:
		t.ime.Confirm()
		if m.Shift {
			return t.ed.Redo()
		}
		return t.ed.Undo()
	case KeyY:
		// Ctrl+Y is redo everywhere but macOS, which uses Shift+Cmd+Z.
		if runtimeIsDarwin() || !t.ed.Redo() {
			return false
		}
		t.ime.Confirm()
		return true
	}
	return false
}

// HandlePointer implements PointerHandler: clicks place the caret, drags
// and repeated clicks select.
func (t *TextInputWidget) HandlePointer(ev PointerEvent) bool {
	if t.IsDisabled() {
		return false
	}
	switch ev.Kind {
	case PointerDown:
		if ev.Button != MouseButtonLeft {
			return false
		}
		t.ime.Confirm()
		now := clock()
		if now.Sub(t.lastClick) < 400*time.Millisecond && near(ev.Pos, t.lastPos) {
			t.clicks++
		} else {
			t.clicks = 1
		}
		t.lastClick, t.lastPos = now, ev.Pos
		idx := t.indexAt(ev.Pos)
		switch t.clicks {
		case 1:
			t.ed.moveTo(idx, false)
		case 2:
			t.ed.selectWord(idx)
		default:
			t.ed.selectAll()
		}
		t.blink = Now()
		return true
	case PointerDrag:
		if t.clicks == 1 {
			t.ed.moveTo(t.indexAt(ev.Pos), true)
			t.blink = Now()
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
	t.Interactive.Adopt(prev)
	t.ed, t.scroll, t.width = p.ed, p.scroll, p.width
	t.clicks, t.lastClick, t.lastPos = p.clicks, p.lastClick, p.lastPos
	if p.ed.text != Untrack(t.value.Get) {
		t.ed.setText(Untrack(t.value.Get))
	}
	t.blink = Now()
}
