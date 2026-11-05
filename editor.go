package ggui

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/paragraph"
	"github.com/ironpark/ggui/internal/platform/textinput"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/runtime"
)

// caretBlink is how long the caret shows and hides for.
const caretBlink = 530 * time.Millisecond

// TextInputWidget is a text editor bound to a StateValue[string]: typing writes
// the signal, and writing the signal updates the text. Build one with
// TextInput. It is one line that scrolls sideways until Multiline makes it
// wrap and grow. It draws only the text, selection and caret; TextField
// adds the themed box around it.
//
// Text comes in through the platform IME (a patched copy of Ebitengine's
// exp/textinput, in internal/platform/textinput), so composed
// scripts such as Korean and Japanese work, with the composition shown
// underlined in place. Keys: arrows (with Shift to select, Alt or Ctrl to
// jump words, ⌘ on macOS to reach the ends), Home and End, Backspace and
// Delete, ⌘/Ctrl+A, C, X and V, Enter for OnSubmit. Double-click selects a
// word, triple-click the line up to its break, dragging selects a range. A Multiline
// editor adds Up and Down, Home and End within the line, Enter for a line
// break and ⌘/Ctrl+Enter for OnSubmit.
type TextInputWidget struct {
	props property.Owner

	// Interactive carries the identity, name and disabled state every
	// control shares. The editor drives focus itself rather than through
	// Keyboard: a caret and an IME session are not a press.
	Interactive

	value             Binding[string]
	placeholder       string
	style             TextStyle
	password          bool
	multiline         bool
	noWrap            bool
	minLines          int
	minWidth          float64
	onSubmit          func(string)
	onCommit          func(string)
	onChange          func(string)
	onKey             func(KeyEvent) bool
	claims            func(KeyEvent) bool
	onPasteImage      func(png []byte) bool
	onDescribe        func(*Node)
	onAction          func(Action) bool
	filter            func(string) string
	escapeUsed        bool
	inheritedDisabled bool

	ed textedit.Editor

	ime         ime
	composition string
	compCaret   int // caret inside composition, in bytes
	imeStart    int // the byte range of ed.Text the IME was told about
	imeEnd      int
	imeErr      error

	scroll float64 // how far the text is shifted left, or up when multiline, to show the caret
	width  float64 // the wrap width from the last Layout, when multiline
	blink  time.Time
	clicks clickSelector

	// wrapped caches the wraps of the two texts last asked for, which
	// layout, paint, the caret's blink and hit testing all ask for again:
	// two, since while the IME composes, paint wraps the text with the
	// composition in it and the rest the text without.
	wrapped [2]wrapResult
	// lines caches the wrap of each hard line by its text, so that an
	// edit wraps again only the lines it changed: wrapping all of a long
	// text at each keystroke costs milliseconds.
	lines lineWraps
	// bidi holds the lines with right-to-left text in them laid out in
	// visual order, by their text: see bidiOf.
	bidi struct {
		style bidiStyle
		lines map[string]*bidiLine
	}
	lang    string           // the Env's language, which right-to-left text is shaped in
	pieceOp text.DrawOptions // reused by paragraph.Draw
	// described caches what runs last found for the screen reader.
	described struct {
		key  runsKey
		runs []TextRun
	}
	// composed caches the text with the composition in it, which every
	// paint, tick and EditingState asks for while the IME composes: built
	// again only when the text, the selection or the composition change,
	// not copied each time.
	composed struct {
		text, composition string
		lo, hi            int
		out               string
	}

	cache       *CachedWidget
	resolved    TextStyle
	muted       color.Color
	selection   color.Color
	rect        Rect            // where it was last painted, in logical pixels
	scale       float64         // the Canvas scale at that paint
	caretBounds image.Rectangle // the caret in logical pixels, for the IME
}

// TextInput creates an editor bound to value.
func TextInput(value Binding[string]) *TextInputWidget {
	t := &TextInputWidget{value: value, minWidth: 120}
	t.Role = RoleTextField
	t.AutoKey()
	t.ime = newIME(t)
	t.ed.Now = func() time.Time { return activeWorld().frame.raw() } // Probe.Advance reaches typing coalesced into one undo
	t.ed.SetText(Untrack(value.Get))
	t.ed.MoveTo(len(t.ed.Text), false)
	// Follow the binding rather than polling it in Layout: a write from
	// outside reaches the editor even when the layout above it is cached.
	// The effect's first run is the value it was just built with, and the
	// editor's own commit writes what ed already holds, so both are no-ops.
	reactive.Observe(func() {
		v := value.Get()
		if v == t.ed.Text {
			return
		}
		if t.composition != "" {
			t.ime.Cancel() // the write replaces what was being composed
		}
		// A caret at the end stays there, after a path appended or a
		// message recalled into the field. Text loaded into an empty field
		// not being typed in, as a file opened, is read from its start.
		atEnd := !t.ed.HasSelection() && t.ed.Caret == len(t.ed.Text) && (t.ed.Text != "" || t.Focused())
		t.ed.SetText(v)
		if atEnd {
			t.ed.MoveTo(len(v), false)
		}
		t.cache.invalidate()
		// The IME's next edit applies to the new text, and an IME that
		// rewrites what it committed, as the Korean one does each
		// syllable, reads it back.
		lo, hi := t.ed.Selection()
		t.imeStart, t.imeEnd = 0, len(t.ed.Text)
		t.ime.Rebase(t.ed.Text[:lo], t.ed.Text[hi:])
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

// IsDisabled reports the effective state, including InputDisabledKey inherited at
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
// takes input. A password's contents are the bullets drawn for it, which
// tell a screen reader its length and where the caret is, but not what it
// says. ui.TextField describes the padded box it draws around this
// editor with the same handler, which is why the two are one node and not
// two: the first description of a handler in a frame is the one kept.
func (t *TextInputWidget) Describe() Node {
	role, name := t.Semantics()
	lo, hi := t.ed.Selection()
	n := Node{
		Role:     role,
		Name:     name,
		Value:    t.display(t.ed.Text),
		Disabled: t.IsDisabled(),
		Actions:  ActionFocus | ActionSetValue | ActionSetSelection,
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
		// The runs are kept for the next description; what the callback
		// changes is its own copy.
		if n.Runs != nil {
			n = freezeNode(n)
		}
		t.onDescribe(&n)
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
	st := t.resolved
	if st.Font == nil {
		st = t.style.resolved()
	}
	key := runsKey{t.ed.Text, t.wrapStyle(), t.lang, fontGen(), st.Font, st.Weight, st.Size, st.LineHeight, t.multiline, t.rect.Origin, t.scroll}
	if d := &t.described; d.runs != nil && d.key == key {
		return d.runs
	}
	out := t.measureRuns()
	t.described.key, t.described.runs = key, out
	return out
}

// runsKey is what runs depends on: the text, its wrap, its face and
// where the editor was painted, scrolled.
type runsKey struct {
	text       string
	wrap       wrapStyle
	lang       string
	generation uint64
	font       *Font
	weight     FontWeight
	size       float64
	lineHeight float64
	multiline  bool
	origin     Point
	scroll     float64
}

// measureRuns is runs, measured.
func (t *TextInputWidget) measureRuns() []TextRun {
	spans := t.spans(t.ed.Text)
	out := make([]TextRun, 0, len(spans))
	h := t.height()
	for i, sp := range spans {
		x, y := t.rect.Origin.X, t.rect.Origin.Y
		if t.multiline {
			y += float64(i)*t.spacing() - t.scroll
		} else {
			x -= t.scroll
		}
		width, w := t.prefixWidths(sp), 0.0
		if bl := t.bidiOf(t.ed.Text[sp.Start:sp.End]); bl != nil {
			width, w = func(b int) float64 { return bl.x(b - sp.Start) }, bl.width()
		} else {
			w = width(sp.End)
		}
		r := TextRun{Start: sp.Start, End: sp.End, Rect: Rct(Pt(x, y), Sz(w, h))}
		for b := sp.Start; ; b = textedit.NextRune(t.ed.Text, b) {
			r.Stops = append(r.Stops, TextStop{Byte: b, X: width(b)})
			if b >= sp.End {
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
	if t.onAction != nil && t.onAction(a) {
		return true
	}
	switch a.Kind {
	case ActionSetValue:
		t.ed.SetText(a.Text)
		t.commit()
		return true
	case ActionSetSelection:
		t.ed.MoveTo(t.fromDisplay(a.SelStart), false)
		t.ed.MoveTo(t.fromDisplay(a.SelEnd), true)
		return true
	}
	return false
}

// ConsumesKey implements KeyConsumer: editing keys stay with the editor.
// Escape passes through unless it cancelled composition or OnKey consumed it.
func (t *TextInputWidget) ConsumesKey(ev KeyEvent) bool {
	return ev.Kind == KeyPress && (ev.Key != KeyEscape || t.escapeUsed)
}

// ClaimsChord implements ChordClaimer: the editing chords stay with the
// editor while it has the focus, ahead of any shortcut or menu action on
// the same chord. Those are the clipboard, undo and select-all chords, and
// the modified arrows, Home, End and deletes; ⌘/Ctrl+Enter too when the
// editor submits with it; and any key ClaimKeys claims.
func (t *TextInputWidget) ClaimsChord(ev KeyEvent) bool {
	m := ev.Mods
	if ev.Kind != KeyPress || t.IsDisabled() {
		return false
	}
	if t.claims != nil && t.composition == "" && t.claims(ev) {
		return true
	}
	if !(m.Ctrl || m.Alt || m.Meta) {
		return false
	}
	switch ev.Key {
	case KeyA, KeyC, KeyX, KeyV, KeyZ:
		return m.Cmd()
	case KeyY:
		return m.Cmd() && !runtimeIsDarwin()
	case KeyArrowLeft, KeyArrowRight, KeyHome, KeyEnd, KeyBackspace, KeyDelete:
		return true
	case KeyArrowUp, KeyArrowDown:
		return t.multiline
	case KeyEnter, KeyNumpadEnter:
		return t.multiline && t.onSubmit != nil && m.Cmd()
	}
	return false
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

// NoWrap makes the editor Multiline with each line as long as it runs,
// broken only where the text breaks it, as a code editor's are. Nothing is
// measured to find the lines, so a long text stays cheap to edit.
func (t *TextInputWidget) NoWrap() *TextInputWidget {
	defer property.Watch(&t.props, &t.noWrap)()
	t.noWrap = true
	return t.Multiline()
}

// Replace puts s in place of the selection, as typing or a paste does: it
// is one step of the undo history and reaches the binding and OnChange.
// It lets a caller build commands of its own, such as an Enter that
// indents the new line.
func (t *TextInputWidget) Replace(s string) {
	if t.IsDisabled() {
		return
	}
	t.ime.Confirm()
	t.ed.Replace(s)
	t.commit()
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
	if c, ok := t.value.(Committer); ok {
		c.Commit()
	}
	if t.onCommit != nil {
		t.onCommit(t.ed.Text)
	}
}

// OnChange fires with the value after every edit, after the signal is set.
func (t *TextInputWidget) OnChange(fn func(string)) *TextInputWidget { t.onChange = fn; return t }

// OnKey handles a key press before editor commands when no IME composition is
// active. Return true to consume it. Focus, text and IME events stay with the
// editor. This lets searchable lists use Up, Down and Enter without replacing
// the editor's platform input driver.
func (t *TextInputWidget) OnKey(fn func(KeyEvent) bool) *TextInputWidget { t.onKey = fn; return t }

// ClaimKeys keeps the keys fn reports with the editor while it has the
// focus, ahead of what the runtime does with them first: Tab moving the
// focus, a shortcut or a menu action on the same chord. They reach OnKey,
// then the editor. fn is asked at each press, so it can claim a key only
// while it means something, as Tab while a completion list shows.
func (t *TextInputWidget) ClaimKeys(fn func(KeyEvent) bool) *TextInputWidget {
	t.claims = fn
	return t
}

// OnPasteImage takes a paste of a clipboard that holds an image and no
// text, as a screenshot leaves it: fn gets the image as PNG and reports
// whether it took it. The editor holds text alone, so without it such a
// paste pastes nothing.
func (t *TextInputWidget) OnPasteImage(fn func(png []byte) bool) *TextInputWidget {
	t.onPasteImage = fn
	return t
}

// OnDescribe lets a control built around the editor say what it is: fn
// amends the node the editor describes, which already carries its text,
// caret and character positions. A number field makes it a spin button with
// a range and the actions that step it; OnAction then carries those out.
// The editor stays the element, so its focus, input and text interface are
// the control's without anything forwarding them.
func (t *TextInputWidget) OnDescribe(fn func(*Node)) *TextInputWidget { t.onDescribe = fn; return t }

// OnAction gives fn the first say on an action an assistive technology aims
// at the editor: one it returns true for is done, and any other the editor
// carries out as it would without it. It is not called while the editor is
// disabled.
func (t *TextInputWidget) OnAction(fn func(Action) bool) *TextInputWidget { t.onAction = fn; return t }

// Focused reports whether the editor has keyboard focus.
func (t *TextInputWidget) Focused() bool { return t.Interactive.Focused }

// display returns s as drawn: itself, or bullets in password mode.
func (t *TextInputWidget) display(s string) string {
	if !t.password {
		return s
	}
	return strings.Repeat(bullet, utf8.RuneCountInString(s))
}

// bullet is what a password field draws for each character.
const bullet = "•"

// toDisplay maps the byte offset b in the text to the same place in what
// display returns for it.
func (t *TextInputWidget) toDisplay(b int) int {
	if !t.password {
		return b
	}
	return len(bullet) * utf8.RuneCountInString(t.ed.Text[:b])
}

// fromDisplay maps the byte offset b in the displayed text back to the
// text, the inverse of toDisplay.
func (t *TextInputWidget) fromDisplay(b int) int {
	if !t.password {
		return b
	}
	i := 0
	for n := b / len(bullet); n > 0 && i < len(t.ed.Text); n-- {
		i = textedit.NextRune(t.ed.Text, i)
	}
	return i
}

func (t *TextInputWidget) face(scale float64) text.Face {
	st := t.resolved
	if st.Font == nil {
		st = t.style.resolved()
	}
	return st.Font.face(st.Size*scale, st.Weight)
}

// advance is the logical width of s as drawn.
func (t *TextInputWidget) advance(s string) float64 { return textlayout.Width(t.display(s), t.face(1)) }

// prefixWidths measures the line sp once and returns the width of its text
// up to a byte offset b of the text within it, where advance would
// measure each prefix.
func (t *TextInputWidget) prefixWidths(sp textlayout.Span) func(b int) float64 {
	width := textlayout.PrefixWidths(t.display(t.ed.Text[sp.Start:sp.End]), t.face(1))
	from := t.toDisplay(sp.Start)
	return func(b int) float64 { return width(t.toDisplay(b) - from) }
}

// rendered is the text with the composition inserted where the caret is,
// and the caret's offset into it.
func (t *TextInputWidget) rendered() (s string, caret int) {
	lo, hi := t.ed.Selection()
	if t.composition == "" {
		return t.ed.Text, t.ed.Caret
	}
	c := &t.composed
	if c.out == "" || c.text != t.ed.Text || c.composition != t.composition || c.lo != lo || c.hi != hi {
		c.text, c.composition, c.lo, c.hi = t.ed.Text, t.composition, lo, hi
		c.out = t.ed.Text[:lo] + t.composition + t.ed.Text[hi:]
	}
	return c.out, lo + t.compCaret
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

// spanKey is what a wrap depends on.
type spanKey struct {
	text  string
	style wrapStyle
}

// wrapStyle is what a wrap depends on beside the text.
type wrapStyle struct {
	generation uint64
	font       *Font
	weight     FontWeight
	size       float64
	width      float64
	noWrap     bool
}

// wrapResult is a text's wrap.
type wrapResult struct {
	key   spanKey
	spans []textlayout.Span
}

// lineWraps holds the wraps of hard lines in one style, each line's spans
// relative to its start, by the line's text. An entry not used by the
// latest wrap goes once there are many more entries than lines.
type lineWraps struct {
	style  wrapStyle
	byText map[string]*lineWrap
	gen    uint64
}

type lineWrap struct {
	spans []textlayout.Span
	used  uint64 // the gen of the last wrap that used it
}

// wrapStyle is what the editor's wrap depends on now beside its text.
func (t *TextInputWidget) wrapStyle() wrapStyle {
	if t.noWrap {
		return wrapStyle{noWrap: true}
	}
	st := t.resolved
	if st.Font == nil {
		st = t.style.resolved()
	}
	return wrapStyle{fontGen(), st.Font, st.Weight, st.Size, t.width, false}
}

// spans wraps s at the editor's width, or leaves it one line. The result
// is shared: callers must not change it.
func (t *TextInputWidget) spans(s string) []textlayout.Span {
	if !t.multiline {
		return []textlayout.Span{{Start: 0, End: len(s)}}
	}
	key := spanKey{s, t.wrapStyle()}
	w := &t.wrapped
	if w[0].spans != nil && w[0].key == key {
		return w[0].spans
	}
	if w[1].spans != nil && w[1].key == key {
		w[0], w[1] = w[1], w[0]
		return w[0].spans
	}
	var spans []textlayout.Span
	if t.noWrap {
		spans = textlayout.BreakSpans(s)
	} else {
		spans = t.wrapLines(s, key.style)
	}
	w[1] = w[0]
	w[0] = wrapResult{key, spans}
	return spans
}

// wrapLines wraps s line by line, as textlayout.WrapSpans does all at
// once, wrapping only the lines t.lines does not hold in style.
func (t *TextInputWidget) wrapLines(s string, style wrapStyle) []textlayout.Span {
	c := &t.lines
	if c.byText == nil || c.style != style {
		c.style, c.byText = style, map[string]*lineWrap{}
	}
	c.gen++
	face := t.face(1)
	out := make([]textlayout.Span, 0, len(t.wrapped[0].spans)+1)
	lines := 0
	for start := 0; ; {
		end := len(s)
		if i := strings.IndexByte(s[start:], '\n'); i >= 0 {
			end = start + i
		}
		line := s[start:end]
		lw := c.byText[line]
		if lw == nil {
			// The key is a copy: the line is cut from a text the next
			// keystroke replaces, which it would otherwise keep.
			lw = &lineWrap{spans: textlayout.WrapSpans(line, face, style.width)}
			c.byText[strings.Clone(line)] = lw
		}
		lw.used = c.gen
		for _, sp := range lw.spans {
			out = append(out, textlayout.Span{Start: start + sp.Start, End: start + sp.End})
		}
		lines++
		if end == len(s) {
			break
		}
		start = end + 1
	}
	// The two texts wrapped last may differ in a line or two; past
	// those, what no wrap uses now is an edit's leftover.
	if len(c.byText) > 2*lines+64 {
		for k, lw := range c.byText {
			if lw.used < c.gen-1 {
				delete(c.byText, k)
			}
		}
	}
	return out
}

// linesHeight is the height of n wrapped lines.
func (t *TextInputWidget) linesHeight(n int) float64 {
	return float64(max(n, 1)-1)*t.spacing() + t.height()
}

// Layout implements Widget.
func (t *TextInputWidget) Layout(c Constraints, env Env) Size {
	defer t.props.Layout().EndLayout()
	t.Sync()
	t.inheritedDisabled, _ = env.Get(InputDisabledKey)
	t.resolved = env.ResolveText(t.style)
	t.lang = env.Language()
	t.cache, _ = env.Get(cacheOwner)
	style := env.EditorStyle()
	if t.IsDisabled() {
		t.resolved.Color = style.Muted
	}
	t.muted, t.selection = style.Muted, style.Selection
	w := bounded(c.MaxW, t.minWidth)
	if !t.multiline {
		return c.Constrain(Sz(w, t.height()))
	}
	t.width = max(c.MinW, w)
	var n int
	if t.noWrap {
		// Lines break only at newlines, so counting them gives the height
		// without the line table, which paint makes if it draws.
		n = strings.Count(t.ed.Text, "\n") + 1
	} else {
		n = len(t.spans(t.ed.Text))
	}
	n = max(n, t.minLines)
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
	bl := t.bidiOf(shown)
	var caretX, total float64
	if bl != nil {
		caretX, total = bl.x(caret), bl.width()
	} else {
		caretX, total = t.advance(shown[:caret]), t.advance(shown)
	}
	t.scroll = clamp(t.scroll, 0, max(total-r.Size.W+1, 0))
	if caretX-t.scroll > r.Size.W-1 {
		t.scroll = caretX - r.Size.W + 1
	}
	if caretX-t.scroll < 0 {
		t.scroll = caretX
	}
	x0 := r.Origin.X - t.scroll
	t.caretBounds = image.Rect(
		int(x0+caretX), int(r.Origin.Y),
		int(x0+caretX)+1, int(r.Origin.Y+h),
	)
	if dst == nil || dst.Image == nil {
		return
	}
	clip := dst.Clip(r)

	if t.Focused() && t.composition == "" && t.ed.HasSelection() {
		lo, hi := t.ed.Selection()
		t.spanX(shown, textlayout.Span{End: len(shown)}, bl, lo, hi, func(a, b float64) {
			clip.FillRect(Rct(Pt(x0+a, r.Origin.Y), Sz(b-a, h)), t.selection)
		})
	}

	op := &text.DrawOptions{}
	op.GeoM.Translate(dst.px(x0), dst.px(r.Origin.Y))
	if shown == "" && t.placeholder != "" {
		op.ColorScale.ScaleWithColor(t.muted)
		fontsys.DrawText(clip.Image, t.placeholder, t.face(dst.Scale()), op)
	} else {
		op.ColorScale.ScaleWithColor(t.resolved.Color)
		op.GeoM.Reset()
		t.drawLine(clip, shown, bl, x0, r.Origin.Y, op)
	}

	if t.composition != "" {
		lo, _ := t.ed.Selection()
		y := r.Origin.Y + h - 1
		t.spanX(shown, textlayout.Span{End: len(shown)}, bl, lo, lo+len(t.composition), func(a, b float64) {
			clip.FillRect(Rct(Pt(x0+a, y), Sz(b-a, 1)), t.resolved.Color)
		})
	}

	if t.Focused() {
		if now := FrameTime(); (now.Sub(t.blink)/caretBlink)%2 == 0 {
			clip.FillRect(Rct(Pt(x0+caretX, r.Origin.Y), Sz(1, h)), t.resolved.Color)
		}
		WakeAt(blinkWake(t.blink, FrameTime(), caretBlink))
	}
}

// paintLines is Paint for a Multiline editor: wrapped lines that scroll
// vertically to keep the caret in view.
func (t *TextInputWidget) paintLines(dst *Canvas, r Rect) {
	t.width = r.Size.W
	shown, caret := t.rendered()
	spans := t.spans(shown)
	li := textlayout.LineOf(spans, caret)
	caretX := t.xIn(shown, spans[li], caret)
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
	t.caretBounds = image.Rect(
		int(x0+caretX), int(y0+caretY),
		int(x0+caretX)+1, int(y0+caretY+h),
	)
	if dst == nil || dst.Image == nil {
		return
	}
	clip := dst.Clip(r)
	lineY := func(i int) float64 { return y0 + float64(i)*t.spacing() }
	// shows reports whether line i is in r: a line scrolled out of it is
	// neither measured nor drawn.
	shows := func(i int) bool { return lineY(i)+h >= r.Origin.Y && lineY(i) <= r.Origin.Y+r.Size.H }

	// eachLine calls fn with the part of [lo, hi) that falls on each line
	// in view, as x offsets, extending to the line's end when the range
	// runs on.
	eachLine := func(lo, hi int, fn func(i int, a, b float64)) {
		for i, sp := range spans {
			if hi < sp.Start || lo > sp.End || !shows(i) {
				continue
			}
			// A range that runs on past the line takes a space's width at
			// its end, which shows the line break in it.
			runsOn := hi > sp.End && i+1 < len(spans)
			bl := t.bidiOf(shown[sp.Start:sp.End])
			if bl == nil {
				a, b := max(lo, sp.Start), min(hi, sp.End)
				ax, bx := t.advance(shown[sp.Start:a]), t.advance(shown[sp.Start:b])
				if runsOn {
					bx += t.advance(" ")
				}
				fn(i, ax, bx)
				continue
			}
			t.spanX(shown, sp, bl, lo, hi, func(a, b float64) { fn(i, a, b) })
			if runsOn {
				end := bl.width()
				fn(i, end, end+t.advance(" "))
			}
		}
	}

	if t.Focused() && t.composition == "" && t.ed.HasSelection() {
		lo, hi := t.ed.Selection()
		eachLine(lo, hi, func(i int, a, b float64) {
			clip.FillRect(Rct(Pt(x0+a, lineY(i)), Sz(b-a, h)), t.selection)
		})
	}

	op := &text.DrawOptions{}
	if shown == "" && t.placeholder != "" {
		op.GeoM.Translate(dst.px(x0), dst.px(y0))
		op.ColorScale.ScaleWithColor(t.muted)
		fontsys.DrawText(clip.Image, t.placeholder, t.face(dst.Scale()), op)
	} else {
		op.ColorScale.ScaleWithColor(t.resolved.Color)
		for i, sp := range spans {
			if !shows(i) {
				continue
			}
			line := shown[sp.Start:sp.End]
			op.GeoM.Reset()
			t.drawLine(clip, line, t.bidiOf(line), x0, lineY(i), op)
		}
	}

	if t.composition != "" {
		lo, _ := t.ed.Selection()
		eachLine(lo, lo+len(t.composition), func(i int, a, b float64) {
			clip.FillRect(Rct(Pt(x0+a, lineY(i)+h-1), Sz(b-a, 1)), t.resolved.Color)
		})
	}

	if t.Focused() {
		if now := FrameTime(); (now.Sub(t.blink)/caretBlink)%2 == 0 {
			clip.FillRect(Rct(Pt(x0+caretX, y0+caretY), Sz(1, h)), t.resolved.Color)
		}
		WakeAt(blinkWake(t.blink, FrameTime(), caretBlink))
	}
}

// bidiStyle is what a line's visual layout depends on beside its text.
type bidiStyle struct {
	generation uint64
	font       *Font
	weight     FontWeight
	size       float64
	lang       string
}

// bidiLine is a line holding right-to-left text, laid out in visual order
// by paragraph as Text lays out such a line: the caret, the selection, a
// click and the drawing all go by its pieces. A line of left-to-right text
// alone is measured and drawn as it is.
type bidiLine struct {
	s      string
	pieces []paragraph.Piece
	styles []paragraph.LineStyle
}

// x is where the caret at offset b of the line stands.
func (l *bidiLine) x(b int) float64 { return paragraph.CaretX(l.s, l.pieces, l.styles, b) }

// at is the offset of the line nearest x.
func (l *bidiLine) at(x float64) int { return paragraph.CaretAt(l.s, l.pieces, l.styles, x) }

func (l *bidiLine) width() float64 { return paragraph.Width(l.pieces) }

// bidiOf returns line laid out in visual order, or nil when it needs no
// reordering: it holds no right-to-left text, or is a password's bullets.
// The layouts are kept by the line's text, so that a line an edit did not
// change is not laid out again.
func (t *TextInputWidget) bidiOf(line string) *bidiLine {
	if t.password || !paragraph.NeedsBidi(line, false) {
		return nil
	}
	st := t.resolved
	if st.Font == nil {
		st = t.style.resolved()
	}
	c := &t.bidi
	if key := (bidiStyle{fontGen(), st.Font, st.Weight, st.Size, t.lang}); c.lines == nil || c.style != key || len(c.lines) > 256 {
		c.style, c.lines = key, map[string]*bidiLine{}
	}
	if l := c.lines[line]; l != nil {
		return l
	}
	line = strings.Clone(line) // the key outlives the text it was cut from
	styles := []paragraph.LineStyle{{Start: 0, End: len(line), Style: st.engine()}}
	l := &bidiLine{s: line, pieces: paragraph.LayoutLine(line, styles, false, t.lang), styles: styles}
	c.lines[line] = l
	return l
}

// xIn is where the caret at offset b of s stands on the line sp of it.
func (t *TextInputWidget) xIn(s string, sp textlayout.Span, b int) float64 {
	if bl := t.bidiOf(s[sp.Start:sp.End]); bl != nil {
		return bl.x(b - sp.Start)
	}
	return t.advance(s[sp.Start:b])
}

// spanX calls fn with the stretches, from the line's left edge, that the
// offsets lo to hi of s cover on the line sp of it, laid out as bl, or
// as it is when bl is nil: one stretch, or one per piece of a reordered
// line.
func (t *TextInputWidget) spanX(s string, sp textlayout.Span, bl *bidiLine, lo, hi int, fn func(a, b float64)) {
	a, b := max(lo, sp.Start), min(hi, sp.End)
	if bl == nil {
		fn(t.advance(s[sp.Start:a]), t.advance(s[sp.Start:b]))
		return
	}
	paragraph.SpanX(bl.s, bl.pieces, bl.styles, a-sp.Start, b-sp.Start, fn)
}

// drawLine draws line, laid out as bl or as it is when bl is nil, with its
// top left at x, y. op holds the color, and a GeoM drawLine moves.
func (t *TextInputWidget) drawLine(dst *Canvas, line string, bl *bidiLine, x, y float64, op *text.DrawOptions) {
	if bl == nil {
		op.GeoM.Translate(dst.px(x), dst.px(y))
		fontsys.DrawText(dst.Image, t.display(line), t.face(dst.Scale()), op)
		return
	}
	col := t.resolved.Color
	baseline := t.face(dst.Scale()).Metrics().HAscent
	paragraph.Draw(dst.Image, &t.pieceOp, bl.s, bl.pieces, bl.styles, dst.Scale(), baseline,
		func(int) color.Color { return col },
		func(op *text.DrawOptions) { op.GeoM.Translate(dst.px(x), dst.px(y)) })
}

// indexAt returns the byte offset in the text nearest to a logical point.
func (t *TextInputWidget) indexAt(p Point) int {
	spans := t.spans(t.ed.Text)
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
func (t *TextInputWidget) indexInLine(sp textlayout.Span, x float64) int {
	if bl := t.bidiOf(t.ed.Text[sp.Start:sp.End]); bl != nil {
		return sp.Start + bl.at(x)
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
	spans := t.spans(t.ed.Text)
	li := textlayout.LineOf(spans, t.ed.Caret)
	x := t.xIn(t.ed.Text, spans[li], t.ed.Caret)
	to := li + dir
	switch {
	case to < 0:
		t.ed.MoveTo(0, extend)
	case to >= len(spans):
		t.ed.MoveTo(len(t.ed.Text), extend)
	default:
		t.ed.MoveTo(t.indexInLine(spans[to], x), extend)
	}
}

// lineBounds returns the start and end of the line the caret is on.
func (t *TextInputWidget) lineBounds() (int, int) {
	spans := t.spans(t.ed.Text)
	sp := spans[textlayout.LineOf(spans, t.ed.Caret)]
	return sp.Start, sp.End
}

// commit writes the editor's text to the signal after an edit.
func (t *TextInputWidget) commit() {
	t.blink = Now()
	if t.filter != nil {
		// Map both selection boundaries through the same normalization, so
		// removed characters cannot leave the caret beyond the accepted text.
		a, c := len(t.filter(t.ed.Text[:t.ed.Anchor])), len(t.filter(t.ed.Text[:t.ed.Caret]))
		t.ed.SetText(t.filter(t.ed.Text))
		t.ed.Anchor, t.ed.Caret = t.ed.Snap(a), t.ed.Snap(c)
	}
	if t.ed.Text == Untrack(t.value.Get) {
		return
	}
	t.value.Set(t.ed.Text)
	if t.multiline {
		t.cache.invalidate()
	}
	if t.onChange != nil {
		t.onChange(t.ed.Text)
	}
}

// ime is the editor's view of the platform IME. The real one wraps
// textinput.Composer; tests substitute a fake that feeds compositions and
// commits by hand.
type ime interface {
	Update() (handled bool, err error)
	Confirm()
	Cancel()
	// Rebase points a session at new text around the caret, written
	// outside the IME.
	Rebase(before, after string)
}

// newIME builds the IME driver for a new editor: the platform's, or the
// one a test gave the world the editor is made in.
func newIME(t *TextInputWidget) ime {
	for w := range ownerWorld().chain() {
		if w.ime != nil {
			return w.ime(t)
		}
	}
	return newPlatformIME(t)
}

// newPlatformIME builds the platform's IME driver for an editor.
func newPlatformIME(t *TextInputWidget) ime {
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

// composerIME drives textinput.Composer, but only inside a running app:
// the package panics when used before ggfx has chosen its backend.
type composerIME struct {
	textinput.Composer
}

func (c *composerIME) Rebase(before, after string) { textinput.Rebase(&c.Composer, before, after) }

func (c *composerIME) Update() (bool, error) {
	if !appRunning.Load() {
		return false, nil
	}
	return c.Composer.Update()
}

// IME callbacks, after examples/textinput in the Ebitengine repository.

func (t *TextInputWidget) imeSession() *textinput.SessionOptions {
	lo, hi := t.ed.Selection()
	t.imeStart, t.imeEnd = 0, len(t.ed.Text)
	return &textinput.SessionOptions{
		CaretBounds:     t.caretBounds,
		TextBeforeCaret: t.ed.Text[:lo],
		TextAfterCaret:  t.ed.Text[hi:],
	}
}

// imeComposition shows preedit text at the caret; "" clears it.
func (t *TextInputWidget) imeComposition(text string, caret int) {
	t.composition, t.compCaret = text, caret
	t.blink = Now()
}

// imeCommit inserts committed text in place of the selection.
func (t *TextInputWidget) imeCommit(text string) {
	t.ed.Replace(text)
	t.commit()
}

// imeReplace applies a commit that rewrote the surrounding text the session
// was given: before + text + after replace that whole range.
func (t *TextInputWidget) imeReplace(before, text, after string) {
	t.ed.Text = t.ed.Text[:t.imeStart] + before + text + after + t.ed.Text[t.imeEnd:]
	t.ed.MoveTo(t.imeStart+len(before)+len(text), false)
	t.commit()
}

// HandleTick implements TickHandler: it runs the IME while focused.
func (t *TextInputWidget) HandleTick() bool {
	if !t.Focused() || t.IsDisabled() {
		return false
	}
	textinput.UpdateCaret(t.caretBounds)
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
		// Text arrives through the IME, on every platform. Outside a running
		// app, as under a Probe, there is no platform IME, so typed text
		// arrives here instead, as it would without one.
		if _, platform := t.ime.(*composerIME); platform && !appRunning.Load() {
			t.imeCommit(ev.Text)
		}
	case KeyPress:
		t.escapeUsed = ev.Key == KeyEscape && t.composition != ""
		if t.composition == "" && t.onKey != nil && t.onKey(ev) {
			t.escapeUsed = ev.Key == KeyEscape
			return
		}
		// Escape drops an edit a Committer holds back, and only then stays
		// with the editor, so a dialog around a clean field still closes.
		if c, ok := t.value.(Committer); ok && ev.Key == KeyEscape && t.composition == "" && c.Revert() {
			t.escapeUsed = true
			return
		}
		t.key(ev.Key, ev.Mods)
	}
}

// wordWise reports whether m asks for the word-wise form of a motion or a
// delete: Alt (Option) anywhere, or Ctrl without Meta. Ctrl stands in for
// Cmd off macOS, but no Cmd shortcut uses an arrow or a delete, which is
// why Ctrl+Left jumps a word on Windows and Linux.
func wordWise(m Mods) bool { return m.Alt || (m.Ctrl && !m.Meta) }

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
			t.ed.Replace("\n")
			return true
		}
		if t.onSubmit != nil {
			t.onSubmit(t.ed.Text)
		}
		t.committed()
	case KeyEscape:
		t.ime.Cancel()
		t.ed.MoveTo(t.ed.Caret, false)
		return true
	case KeyBackspace:
		t.ime.Confirm()
		t.ed.Backspace(wordWise(m))
		return true
	case KeyDelete:
		t.ime.Confirm()
		t.ed.DeleteForward(wordWise(m))
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
			t.ed.MoveTo(pick(k == KeyArrowUp, 0, len(t.ed.Text)), m.Shift)
		} else {
			t.moveLine(pick(k == KeyArrowUp, -1, 1), m.Shift)
		}
		return true
	case KeyArrowLeft, KeyArrowRight:
		t.ime.Confirm()
		dir := pick(k == KeyArrowLeft, -1, 1)
		if m.Meta {
			// The macOS form of Home and End: the ends of the line the caret
			// is on, which in a single-line field are the ends of the text.
			lo, hi := 0, len(t.ed.Text)
			if t.multiline {
				lo, hi = t.lineBounds()
			}
			t.ed.MoveTo(pick(dir < 0, lo, hi), m.Shift)
		} else {
			t.ed.MoveBy(dir, wordWise(m), m.Shift)
		}
		return true
	case KeyHome, KeyEnd:
		t.ime.Confirm()
		lo, hi := 0, len(t.ed.Text)
		if t.multiline && !m.Cmd() {
			lo, hi = t.lineBounds()
		}
		t.ed.MoveTo(pick(k == KeyHome, lo, hi), m.Shift)
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
			t.ed.SelectAll()
		}
		return true
	case KeyC:
		if m.Cmd() && t.ed.HasSelection() && !t.password {
			currentClipboard().Write(t.ed.Selected())
		}
		return true
	case KeyX:
		if m.Cmd() && t.ed.HasSelection() {
			t.ime.Confirm()
			if !t.password {
				currentClipboard().Write(t.ed.Selected())
			}
			t.ed.Replace("")
		}
		return true
	case KeyV:
		if m.Cmd() {
			t.ime.Confirm()
			clip := currentClipboard().Read()
			if clip == "" && t.onPasteImage != nil {
				if png, ok := runtime.ReadImage(currentClipboard()); ok && t.onPasteImage(png) {
					return false
				}
			}
			s := strings.ReplaceAll(clip, "\r\n", "\n")
			s = strings.ReplaceAll(s, "\r", "\n")
			if !t.multiline {
				s = strings.ReplaceAll(s, "\n", " ")
			}
			t.ed.Replace(s)
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
		t.clicks.press(&t.ed, ev.Pos, t.indexAt, false)
		t.blink = Now()
		return true
	case PointerDrag:
		if t.clicks.drag(&t.ed, ev.Pos, t.indexAt) {
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

// clickSelector turns a pointer's clicks on text into a selection: one
// places the caret, a quick second near the first selects a word, a
// third the line, and a drag after one click extends the selection.
type clickSelector struct {
	clicks    int
	lastClick time.Time
	lastPos   Point
}

// press counts a left press at pos and selects in ed for it, at reports
// the text offset for a point. extend, as Shift does, extends the
// selection to pos instead and starts the count over.
func (c *clickSelector) press(ed *textedit.Editor, pos Point, at func(Point) int, extend bool) {
	now := activeWorld().frame.raw()
	if now.Sub(c.lastClick) < 400*time.Millisecond && near(pos, c.lastPos) {
		c.clicks++
	} else {
		c.clicks = 1
	}
	c.lastClick, c.lastPos = now, pos
	idx := at(pos)
	switch {
	case extend:
		c.clicks = 1
		ed.MoveTo(idx, true)
	case c.clicks == 1:
		ed.MoveTo(idx, false)
	case c.clicks == 2:
		ed.SelectWord(idx)
	default:
		ed.SelectLine(idx)
	}
}

// drag extends ed's selection to pos after a single click, and reports
// whether it did.
func (c *clickSelector) drag(ed *textedit.Editor, pos Point, at func(Point) int) bool {
	if c.clicks != 1 {
		return false
	}
	ed.MoveTo(at(pos), true)
	return true
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
	t.clicks = p.clicks
	if p.ed.Text != Untrack(t.value.Get) {
		t.ed.SetText(Untrack(t.value.Get))
	}
	t.blink = Now()
}

// TextInputState is a read-only snapshot for custom editor presentations.
// Offsets are UTF-8 byte boundaries in Text. Composing reports an IME preedit.
type TextInputState struct {
	Text               string
	Anchor, Caret      int
	Focused, Composing bool
	// CompStart and CompEnd are where the composition is in Text, while
	// Composing.
	CompStart, CompEnd int
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
	a := t.ed.Anchor
	if t.composition != "" {
		a = caret
	}
	st := TextInputState{Text: s, Anchor: a, Caret: caret, Focused: t.Focused(), Composing: t.composition != ""}
	if st.Composing {
		st.CompStart = caret - t.compCaret
		st.CompEnd = st.CompStart + len(t.composition)
	}
	return st
}

// Select sets the selection at UTF-8 byte offsets, clamped to rune boundaries.
func (t *TextInputWidget) Select(anchor, caret int) {
	if t.IsDisabled() {
		return
	}
	t.ime.Confirm()
	t.ed.MoveTo(anchor, false)
	t.ed.MoveTo(caret, true)
	t.blink = Now()
}

// PaintCustom replaces the standard text drawing, for segmented editors such
// as Input OTP. The callback returns the logical caret bounds used by the native
// IME. The caller owns hit regions and semantics and forwards keyboard events
// and HandleTick to this editor. Layout must have run first.
func (t *TextInputWidget) PaintCustom(dst *Canvas, r Rect, paint func(*Canvas, Rect, TextInputState) Rect) {
	t.rect, t.scale = r, dst.Scale()
	caret := paint(dst, r, t.EditingState())
	t.caretBounds = image.Rect(int(caret.Origin.X), int(caret.Origin.Y), int(caret.Origin.X+caret.Size.W), int(caret.Origin.Y+caret.Size.H))
}
