package widget

import (
	"image"
	"image/color"
	"math"
	goruntime "runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/editlayout"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/paragraph"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/pkg/gem"
	"github.com/ironpark/ggui/property"
	"github.com/ironpark/ggui/runtime"
)

// caretBlink is how long the caret shows and hides for.
const caretBlink = 530 * time.Millisecond

// blinkWake returns when a caret that toggles every period since start next
// changes, for WakeAt.
func blinkWake(start, now time.Time, period time.Duration) time.Time {
	n := now.Sub(start)/period + 1
	return start.Add(n * period)
}

// isDarwin reports macOS, whose redo is Shift+Cmd+Z rather than Ctrl+Y.
const isDarwin = goruntime.GOOS == "darwin"

// px converts a logical length or coordinate to dst's Image pixels, as
// Canvas.Px does, in the float64 a GeoM takes.
func px(dst *ggui.Canvas, v float64) float64 { return v * dst.Scale() }

// TextInputWidget is a text editor bound to a StateValue[string]: typing writes
// the signal, and writing the signal updates the text. Build one with
// TextInput. It is one line that scrolls sideways until Multiline makes it
// wrap and grow. It draws only the text, selection and caret; ui.TextField
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
	ggui.Interactive

	value             ggui.Binding[string]
	placeholder       string
	style             ggui.TextStyle
	password          bool
	multiline         bool
	noWrap            bool
	minLines          int
	minWidth          float64
	onSubmit          func(string)
	onCommit          func(string)
	onChange          func(string)
	onKey             func(ggui.KeyEvent) bool
	claims            func(ggui.KeyEvent) bool
	onPasteImage      func(png []byte) bool
	onDescribe        func(*ggui.Node)
	onAction          func(ggui.Action) bool
	filter            func(string) string
	escapeUsed        bool
	inheritedDisabled bool

	ed textedit.Editor

	ime         ggui.IME
	imeFor      *imeOwner // the editor ime serves, which Adopt hands on
	composition string
	compCaret   int // caret inside composition, in bytes
	imeStart    int // the byte range of ed.Text the IME was told about
	imeEnd      int
	imeErr      error

	scroll float64 // how far the text is shifted left, or up when multiline, to show the caret
	width  float64 // the wrap width from the last Layout, when multiline
	blink  time.Time
	clicks textedit.Clicks

	// layout caches the wraps of the texts last asked for, which layout,
	// paint, the caret's blink and hit testing all ask for again, and each
	// hard line's wrap and visual layout by its text, so that an edit lays
	// out again only the lines it changed; see editlayout.
	layout  editlayout.Cache
	lang    string           // the Env's language, which right-to-left text is shaped in
	pieceOp text.DrawOptions // reused by paragraph.Draw
	// described caches what runs last measured for the screen reader, and
	// holds what it last handed out, moved to where the editor is.
	described struct {
		key           runsKey
		measured, out []ggui.TextRun
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

	// env is the last Layout's: an edit that changes the editor's height,
	// or a write to its binding, tells ggui.Invalidate about it, and the
	// Cached above the editor measures it again.
	env      ggui.Env
	resolved ggui.TextStyle
	// colors colours stretches of the text shown; see Colors. pieces is
	// the scratch colorPieces cuts a line into.
	colors      func(text string) []TextColor
	pieces      []TextColor
	muted       color.Color
	selection   color.Color
	rect        ggui.Rect       // where it was last painted, in logical pixels
	scale       float64         // the Canvas scale at that paint
	caretBounds image.Rectangle // the caret in logical pixels, for the IME
}

// TextInput creates an editor bound to value.
func TextInput(value ggui.Binding[string]) *TextInputWidget {
	t := &TextInputWidget{value: value, minWidth: 120}
	t.Role = ggui.RoleTextField
	t.AutoKey()
	// The IME edits whichever editor owns it now: one rebuilt over this
	// one takes it over, with what it composes (Adopt).
	o := &imeOwner{t: t}
	t.imeFor = o
	t.ime = ggui.NewIME(ggui.IMEHandler{
		Session:     func() (image.Rectangle, string, string) { return o.t.imeSession() },
		Composition: func(text string, caret int) { o.t.imeComposition(text, caret) },
		Commit:      func(text string) { o.t.imeCommit(text) },
		Replace:     func(before, text, after string) { o.t.imeReplace(before, text, after) },
		End:         func() { o.t.Interactive.Focused = false },
	})
	t.layout.Face = func() text.Face { return t.face(t.measureScale()) }
	t.layout.Style = func() paragraph.Style { return t.bidiStyle() }
	t.ed.Now = ggui.ClockTime // Probe.Advance reaches typing coalesced into one undo
	t.ed.SetText(ggui.Untrack(value.Get))
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
		ggui.Invalidate(t.env)
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
func (t *TextInputWidget) BindDisabled(r ggui.Readable[bool]) *TextInputWidget {
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
func (t *TextInputWidget) BindName(r ggui.Readable[string]) *TextInputWidget {
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

// TextColor colours the bytes [Start, End) of the text an editor shows.
// Color may be one the environment resolves, as a theme's.
type TextColor struct {
	Start, End int
	Color      color.Color
}

// Colors colours stretches of the text as fn says, asked with the text
// shown, the IME's composition in it, each time it is drawn, so it should
// be cheap or remember its last answer; the rest has the style's colour.
// Stretches are in order and do not overlap. A password keeps the one
// colour.
func (t *TextInputWidget) Colors(fn func(text string) []TextColor) *TextInputWidget {
	t.colors = fn
	return t
}

// Password masks every rune with a bullet.
func (t *TextInputWidget) Password() *TextInputWidget { t.password = true; return t }

// Style merges ts onto the widget's own text style.
func (t *TextInputWidget) Style(ts ggui.TextStyle) *TextInputWidget {
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
	if c, ok := t.value.(ggui.Committer); ok {
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
func (t *TextInputWidget) OnKey(fn func(ggui.KeyEvent) bool) *TextInputWidget { t.onKey = fn; return t }

// ClaimKeys keeps the keys fn reports with the editor while it has the
// focus, ahead of what the runtime does with them first: Tab moving the
// focus, a shortcut or a menu action on the same chord. They reach OnKey,
// then the editor. fn is asked at each press, so it can claim a key only
// while it means something, as Tab while a completion list shows.
func (t *TextInputWidget) ClaimKeys(fn func(ggui.KeyEvent) bool) *TextInputWidget {
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
func (t *TextInputWidget) OnDescribe(fn func(*ggui.Node)) *TextInputWidget {
	t.onDescribe = fn
	return t
}

// OnAction gives fn the first say on an action an assistive technology aims
// at the editor: one it returns true for is done, and any other the editor
// carries out as it would without it. It is not called while the editor is
// disabled.
func (t *TextInputWidget) OnAction(fn func(ggui.Action) bool) *TextInputWidget {
	t.onAction = fn
	return t
}

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

// engine is the style the text is set in, as the text engine sets it, and
// the generation of its fonts: what Layout resolved, or before any Layout,
// the editor's own.
func (t *TextInputWidget) engine() (paragraph.Style, uint64) {
	if t.resolved.Font == nil {
		return textEngineStyle(t.style)
	}
	return textEngineStyle(t.resolved)
}

// textStyle is the style engine returns, without the generation.
func (t *TextInputWidget) textStyle() paragraph.Style {
	st, _ := t.engine()
	return st
}

// measureScale is the display scale used for measuring, wrapping, caret
// positioning and hit-testing so they match the face Paint draws in.
func (t *TextInputWidget) measureScale() float64 {
	if t.scale > 0 && !math.IsNaN(t.scale) && !math.IsInf(t.scale, 0) {
		return t.scale
	}
	return sanitizeScale(ggui.UseScale())
}

// bidiStyle is textStyle with Size scaled to physical pixels so paragraph
// lays out and measures bidi lines in the same face Paint draws them in.
func (t *TextInputWidget) bidiStyle() paragraph.Style {
	st := t.textStyle()
	st.Size *= t.measureScale()
	return st
}

// face is the face the text is drawn in at scale times its size, its
// tracking with it.
func (t *TextInputWidget) face(scale float64) text.Face {
	st := t.textStyle()
	size := st.Size * scale
	return st.Font.SpacedFaceFor(size, st.Weight, st.Tracking*size, st.Emoji)
}

// advance is the logical width of s as drawn.
func (t *TextInputWidget) advance(s string) float64 {
	scale := t.measureScale()
	return textlayout.Width(t.display(s), t.face(scale)) / scale
}

// prefixWidths measures the line sp once and returns the width of its text
// up to a byte offset b of the text within it, where advance would
// measure each prefix.
func (t *TextInputWidget) prefixWidths(sp textlayout.Span) func(b int) float64 {
	scale := t.measureScale()
	width := textlayout.PrefixWidths(t.display(t.ed.Text[sp.Start:sp.End]), t.face(scale))
	from := t.toDisplay(sp.Start)
	return func(b int) float64 { return width(t.toDisplay(b)-from) / scale }
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
	scale := t.measureScale()
	m := t.face(scale).Metrics()
	return (m.HAscent + m.HDescent) / scale
}

// Baseline implements Baseliner: the first line's, an ascent below the top.
func (t *TextInputWidget) Baseline() (float64, bool) {
	if t.resolved.Font == nil {
		return 0, false
	}
	scale := t.measureScale()
	return t.face(scale).Metrics().HAscent / scale, true
}

// spacing is the distance between baselines when multiline.
func (t *TextInputWidget) spacing() float64 {
	st := t.textStyle()
	return st.Size * st.LineHeight
}

// measureKey is what measuring the text depends on beside the text: the
// face and the language right-to-left text is shaped in.
func (t *TextInputWidget) measureKey() editlayout.Key {
	st, gen := t.engine()
	return editlayout.Key{
		Generation: gen,
		Font:       st.Font,
		Weight:     st.Weight,
		Size:       st.Size * t.measureScale(),
		Tracking:   st.Tracking,
		Lang:       t.lang,
	}
}

// wrapStyle is what the editor's wrap depends on now beside its text.
func (t *TextInputWidget) wrapStyle() editlayout.Style {
	ws := editlayout.Style{Key: t.measureKey()}
	switch {
	case !t.multiline:
		ws.Mode = editlayout.OneLine
	case t.noWrap:
		ws.Mode = editlayout.BreakOnly
	default:
		ws.Mode, ws.Width = editlayout.WrapWidth, t.width*t.measureScale()
	}
	return ws
}

// spans wraps s at the editor's width, or leaves it one line. The result
// is shared, and lasts until two other texts are wrapped, which reuse its
// memory: callers must not change it or keep it.
func (t *TextInputWidget) spans(s string) []textlayout.Span { return t.wrap(s).Spans }

// wrap returns s wrapped as spans does, with what is known of each line.
func (t *TextInputWidget) wrap(s string) *editlayout.Wrap {
	return t.layout.Wrap(s, t.wrapStyle(), &t.ed)
}

// linesHeight is the height of n wrapped lines.
func (t *TextInputWidget) linesHeight(n int) float64 {
	return float64(max(n, 1)-1)*t.spacing() + t.height()
}

// Layout implements Widget.
func (t *TextInputWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout().EndLayout()
	t.Sync()
	t.inheritedDisabled, _ = env.Get(ggui.InputDisabledKey)
	t.resolved = env.ResolveText(t.style)
	t.lang = env.Language()
	t.env = env
	t.scale = sanitizeScale(ggui.UseScale())
	style := env.EditorStyle()
	if t.IsDisabled() {
		t.resolved.Color = style.Muted
	}
	t.muted, t.selection = style.Muted, style.Selection
	w := gem.Bounded(c.MaxW, t.minWidth)
	if !t.multiline {
		return c.Constrain(ggui.Sz(w, t.height()))
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
	return c.Constrain(ggui.Sz(t.width, t.linesHeight(n)))
}

// Paint implements Widget.
func (t *TextInputWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t.Sync()
	// Where it is being painted is recorded first: describing it reports
	// where its characters are, which is measured from here.
	t.rect = r
	if dst != nil {
		t.scale = dst.Scale()
	}
	// A disabled editor takes no input but is still read out.
	dst.Describe(r, t)
	if !t.IsDisabled() {
		dst.HitPointer(r, t)
		dst.HitKey(r, t)
		dst.HitCursor(r, ggui.CursorShapeText)
	}
	if t.multiline {
		t.paintLines(dst, r)
		return
	}

	shown, caret := t.rendered()
	h := t.height()
	bl := t.bidiAt(t.wrap(shown), 0)
	var caretX, total float64
	if bl != nil {
		scale := t.measureScale()
		caretX, total = bl.X(caret)/scale, bl.Width()/scale
	} else {
		caretX, total = t.advance(shown[:caret]), t.advance(shown)
	}
	t.scroll = gem.Clamp(t.scroll, 0, max(total-r.Size.W+1, 0))
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
		t.spanX(shown, textlayout.Span{End: len(shown)}, bl, lo, hi, 0, func(a, b float64) {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+a, r.Origin.Y), ggui.Sz(b-a, h)), t.selection)
		})
	}

	op := &text.DrawOptions{}
	op.GeoM.Translate(px(dst, x0), px(dst, r.Origin.Y))
	if shown == "" && t.placeholder != "" {
		op.ColorScale.ScaleWithColor(t.muted)
		fontsys.DrawText(clip.Image, placeholderLines(t.placeholder, false)[0], t.face(dst.Scale()), op)
	} else {
		t.drawText(clip, shown, textlayout.Span{End: len(shown)}, bl, t.textColors(shown), x0, r.Origin.Y, op)
	}

	if t.composition != "" {
		lo, _ := t.ed.Selection()
		y := r.Origin.Y + h - 1
		t.spanX(shown, textlayout.Span{End: len(shown)}, bl, lo, lo+len(t.composition), 0, func(a, b float64) {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+a, y), ggui.Sz(b-a, 1)), t.resolved.Color)
		})
	}

	if t.Focused() {
		if now := ggui.FrameTime(); (now.Sub(t.blink)/caretBlink)%2 == 0 {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+caretX, r.Origin.Y), ggui.Sz(1, h)), t.resolved.Color)
		}
		ggui.WakeAt(blinkWake(t.blink, ggui.FrameTime(), caretBlink))
	}
}

// paintLines is Paint for a Multiline editor: wrapped lines that scroll
// vertically to keep the caret in view.
func (t *TextInputWidget) paintLines(dst *ggui.Canvas, r ggui.Rect) {
	t.width = r.Size.W
	shown, caret := t.rendered()
	wrapped := t.wrap(shown)
	spans := wrapped.Spans
	li := textlayout.LineOf(spans, caret)
	caretX := t.xAt(wrapped, li, caret)
	caretY := float64(li) * t.spacing()
	h := t.height()
	t.scroll = gem.Clamp(t.scroll, 0, max(t.linesHeight(len(spans))-r.Size.H, 0))
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
			runOn := 0.0
			if hi > sp.End && i+1 < len(spans) {
				runOn = t.advance(" ")
			}
			t.spanX(shown, sp, t.bidiAt(wrapped, i), lo, hi, runOn, func(a, b float64) { fn(i, a, b) })
		}
	}

	if t.Focused() && t.composition == "" && t.ed.HasSelection() {
		lo, hi := t.ed.Selection()
		eachLine(lo, hi, func(i int, a, b float64) {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+a, lineY(i)), ggui.Sz(b-a, h)), t.selection)
		})
	}

	op := &text.DrawOptions{}
	if shown == "" && t.placeholder != "" {
		// The text drawer stacks no lines of its own: each line of the
		// placeholder goes on a line of the field, as typed text would.
		op.ColorScale.ScaleWithColor(t.muted)
		for i, line := range placeholderLines(t.placeholder, true) {
			if !shows(i) {
				continue
			}
			op.GeoM.Reset()
			op.GeoM.Translate(px(dst, x0), px(dst, lineY(i)))
			fontsys.DrawText(clip.Image, line, t.face(dst.Scale()), op)
		}
	} else {
		colors := t.textColors(shown)
		for i, sp := range spans {
			if shows(i) {
				t.drawText(clip, shown, sp, t.bidiAt(wrapped, i), colors, x0, lineY(i), op)
			}
		}
	}

	if t.composition != "" {
		lo, _ := t.ed.Selection()
		eachLine(lo, lo+len(t.composition), func(i int, a, b float64) {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+a, lineY(i)+h-1), ggui.Sz(b-a, 1)), t.resolved.Color)
		})
	}

	if t.Focused() {
		if now := ggui.FrameTime(); (now.Sub(t.blink)/caretBlink)%2 == 0 {
			clip.FillRect(ggui.Rct(ggui.Pt(x0+caretX, y0+caretY), ggui.Sz(1, h)), t.resolved.Color)
		}
		ggui.WakeAt(blinkWake(t.blink, ggui.FrameTime(), caretBlink))
	}
}

// placeholderLines splits a placeholder into the lines a field shows: one
// per line of a multiline field, and one line with spaces for its breaks
// in a single-line field, which has room for no other.
func placeholderLines(s string, multiline bool) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if multiline {
		return strings.Split(s, "\n")
	}
	return []string{strings.ReplaceAll(s, "\n", " ")}
}

// bidiAt returns line i of r laid out in visual order, or nil when it
// needs no reordering: it holds no right-to-left text, or is a password's
// bullets. The layout is kept with the line's wrap, so a line an edit did
// not change is not laid out again.
func (t *TextInputWidget) bidiAt(r *editlayout.Wrap, i int) *editlayout.Line {
	if t.password {
		return nil
	}
	return t.layout.Bidi(r, i)
}

// xAt is where the caret at offset b of r's text stands on its line i.
func (t *TextInputWidget) xAt(r *editlayout.Wrap, i, b int) float64 {
	sp := r.Spans[i]
	if bl := t.bidiAt(r, i); bl != nil {
		return bl.X(b-sp.Start) / t.measureScale()
	}
	return t.advance(r.Text()[sp.Start:b])
}

// spanX calls fn with the stretches, from the line's left edge, that the
// offsets lo to hi of s cover on the line sp of it, laid out as bl, or
// as it is when bl is nil: one stretch, or one per piece of a reordered
// line. runOn more is covered past the line's end.
func (t *TextInputWidget) spanX(s string, sp textlayout.Span, bl *editlayout.Line, lo, hi int, runOn float64, fn func(a, b float64)) {
	a, b := max(lo, sp.Start), min(hi, sp.End)
	if bl == nil {
		fn(t.advance(s[sp.Start:a]), t.advance(s[sp.Start:b])+runOn)
		return
	}
	scale := t.measureScale()
	bl.SpanX(a-sp.Start, b-sp.Start, func(x0, x1 float64) { fn(x0/scale, x1/scale) })
	if runOn > 0 {
		end := bl.Width() / scale
		fn(end, end+runOn)
	}
}

// drawText draws the line sp of s, laid out as bl or as it is when bl is
// nil, with its top left at x, y: in the style's colour, and the pieces
// colors colours each in its own, the line drawn again within where each
// piece stands, so it is shaped once, in either direction. op is reused.
func (t *TextInputWidget) drawText(dst *ggui.Canvas, s string, sp textlayout.Span, bl *editlayout.Line, colors []TextColor, x, y float64, op *text.DrawOptions) {
	line := s[sp.Start:sp.End]
	if len(colors) == 0 {
		t.drawLine(dst, line, bl, x, y, op, t.resolved.Color)
		return
	}
	t.pieces = colorPieces(t.pieces[:0], sp, colors)
	h, w := t.height(), t.lineWidth(s, sp, bl)
	for _, p := range t.pieces {
		col := t.resolved.Color
		if p.Color != nil {
			col = ggui.ResolveColor(p.Color, t.env)
		}
		t.spanX(s, sp, bl, p.Start, p.End, 0, func(a, b float64) {
			// A glyph may stand out past the line's ends, as an italic's
			// does: the clip there runs on.
			if a <= 0 {
				a -= h
			}
			if b >= w {
				b += h
			}
			t.drawLine(dst.Clip(ggui.Rct(ggui.Pt(x+a, y), ggui.Sz(b-a, h))), line, bl, x, y, op, col)
		})
	}
}

// lineWidth is how wide the line sp of s is laid out as bl, or as it is
// when bl is nil.
func (t *TextInputWidget) lineWidth(s string, sp textlayout.Span, bl *editlayout.Line) float64 {
	if bl != nil {
		return bl.Width() / t.measureScale()
	}
	return t.advance(s[sp.Start:sp.End])
}

// drawLine draws line, laid out as bl or as it is when bl is nil, with its
// top left at x, y, in col. op is reused.
func (t *TextInputWidget) drawLine(dst *ggui.Canvas, line string, bl *editlayout.Line, x, y float64, op *text.DrawOptions, col color.Color) {
	if bl == nil {
		op.GeoM.Reset()
		op.ColorScale.Reset()
		op.GeoM.Translate(px(dst, x), px(dst, y))
		op.ColorScale.ScaleWithColor(col)
		fontsys.DrawText(dst.Image, t.display(line), t.face(dst.Scale()), op)
		return
	}
	scale := sanitizeScale(dst.Scale())
	baseline := t.face(scale).Metrics().HAscent
	bl.Draw(dst.Image, &t.pieceOp, scale/t.measureScale(), baseline,
		func(int) color.Color { return col },
		func(op *text.DrawOptions) { op.GeoM.Translate(px(dst, x), px(dst, y)) })
}

// textColors is what Colors says of shown, nil for none or a password.
func (t *TextInputWidget) textColors(shown string) []TextColor {
	if t.colors == nil || t.password || shown == "" {
		return nil
	}
	return t.colors(shown)
}

// colorPieces appends to out the pieces of the line sp that colors
// colours, those between them with no Color: in order, none empty,
// covering the line. A stretch out of order, overlapping the one before,
// is left out.
func colorPieces(out []TextColor, sp textlayout.Span, colors []TextColor) []TextColor {
	add := func(a, b int, c color.Color) {
		if a < b {
			out = append(out, TextColor{Start: a, End: b, Color: c})
		}
	}
	at := sp.Start
	for _, c := range colors {
		a, b := max(c.Start, sp.Start), min(c.End, sp.End)
		if b <= a || a < at {
			continue
		}
		add(at, a, nil)
		add(a, b, c.Color)
		at = b
	}
	add(at, sp.End, nil)
	return out
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

// commit writes the editor's text to the signal after an edit.
func (t *TextInputWidget) commit() {
	t.blink = ggui.Now()
	if t.filter != nil {
		// Map both selection boundaries through the same normalization, so
		// removed characters cannot leave the caret beyond the accepted text.
		a, c := len(t.filter(t.ed.Text[:t.ed.Anchor])), len(t.filter(t.ed.Text[:t.ed.Caret]))
		t.ed.SetText(t.filter(t.ed.Text))
		t.ed.Anchor, t.ed.Caret = t.ed.Snap(a), t.ed.Snap(c)
	}
	if t.ed.Text == ggui.Untrack(t.value.Get) {
		return
	}
	t.value.Set(t.ed.Text)
	if t.multiline {
		ggui.Invalidate(t.env)
	}
	if t.onChange != nil {
		t.onChange(t.ed.Text)
	}
}

// IME callbacks, the editor's ggui.IMEHandler, after examples/textinput in
// the Ebitengine repository. The IME is the platform's when an app runs;
// tests substitute a fake through ggui.SetIME that feeds compositions and
// commits by hand.

// noIME is the IME of an editor another rebuilt over it took its own
// from: it composes nothing.
type noIME struct{}

func (noIME) Update(image.Rectangle) (bool, error) { return false, nil }
func (noIME) Confirm()                             {}
func (noIME) Cancel()                              {}
func (noIME) Rebase(string, string)                {}
func (noIME) Typed(string)                         {}

// imeOwner is the editor an IME serves. It is the IME's handler's one way
// to the editor, so that an editor rebuilt over another takes the IME over
// by pointing it at itself.
type imeOwner struct{ t *TextInputWidget }

// imeSession tells a new session where the caret is and the text around
// it, which is the range a replacing commit replaces.
func (t *TextInputWidget) imeSession() (caret image.Rectangle, before, after string) {
	lo, hi := t.ed.Selection()
	t.imeStart, t.imeEnd = 0, len(t.ed.Text)
	return t.caretBounds, t.ed.Text[:lo], t.ed.Text[hi:]
}

// imeComposition shows preedit text at the caret; "" clears it.
func (t *TextInputWidget) imeComposition(text string, caret int) {
	t.composition, t.compCaret = text, caret
	t.blink = ggui.Now()
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
	handled, err := t.ime.Update(t.caretBounds)
	if err != nil {
		t.imeErr = err
	}
	return handled
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
	if t.editKey(k, m) || t.moveKey(k, m) || t.clipboardKey(k, m) || t.historyKey(k, m) {
		t.commit()
	}
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
		return true
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
func (t *TextInputWidget) moveKey(k ggui.KeyboardKey, m ggui.Mods) bool {
	switch k {
	case ggui.KeyArrowUp, ggui.KeyArrowDown:
		if !t.multiline {
			return false
		}
		t.ime.Confirm()
		if m.Meta {
			t.ed.MoveTo(gem.Pick(k == ggui.KeyArrowUp, 0, len(t.ed.Text)), m.Shift)
		} else {
			t.moveLine(gem.Pick(k == ggui.KeyArrowUp, -1, 1), m.Shift)
		}
		return true
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
		return true
	case ggui.KeyHome, ggui.KeyEnd:
		t.ime.Confirm()
		lo, hi := 0, len(t.ed.Text)
		if t.multiline && !m.Cmd() {
			lo, hi = t.lineBounds()
		}
		t.ed.MoveTo(gem.Pick(k == ggui.KeyHome, lo, hi), m.Shift)
		return true
	}
	return false
}

// clipboardKey handles the Cmd/Ctrl shortcuts that move text in and out of
// the field: select all, copy, cut and paste. A password field is never
// copied out of, though cutting still deletes.
func (t *TextInputWidget) clipboardKey(k ggui.KeyboardKey, m ggui.Mods) bool {
	switch k {
	case ggui.KeyA:
		if m.Cmd() {
			t.ime.Confirm()
			t.ed.SelectAll()
		}
		return true
	case ggui.KeyC:
		if m.Cmd() && t.ed.HasSelection() && !t.password {
			ggui.CurrentClipboard().Write(t.ed.Selected())
		}
		return true
	case ggui.KeyX:
		if m.Cmd() && t.ed.HasSelection() {
			t.ime.Confirm()
			if !t.password {
				ggui.CurrentClipboard().Write(t.ed.Selected())
			}
			t.ed.Replace("")
		}
		return true
	case ggui.KeyV:
		if m.Cmd() {
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
		}
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
		if isDarwin || !t.ed.Redo() {
			return false
		}
		t.ime.Confirm()
		return true
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
		t.clicks.Press(&t.ed, ggui.ClockTime(), ev.Pos, t.indexAt, false)
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

// Adopt implements Adopter: a rebuilt editor at the same Rect carries on
// with the previous one's focus, caret and selection, and its IME, with
// what the IME is composing. A form rebuilt by its own field's write, as
// each syllable a Korean IME commits writes it, so goes on composing the
// next one: confirming it there would commit its first letter alone.
func (t *TextInputWidget) Adopt(prev any) {
	p, ok := prev.(*TextInputWidget)
	if !ok {
		return
	}
	// The IME moves over, once however often the regions ask: the editor
	// left behind keeps one that does nothing, so nothing it still runs,
	// as its binding's effect, reaches the IME in use.
	if p.imeFor != nil {
		t.ime.Cancel() // its own, which never started a session
		t.ime, t.imeFor = p.ime, p.imeFor
		t.imeFor.t = t
		t.composition, t.compCaret, t.imeStart, t.imeEnd = p.composition, p.compCaret, p.imeStart, p.imeEnd
		p.ime, p.imeFor, p.composition = noIME{}, nil, ""
	}
	t.Interactive.Adopt(prev)
	t.ed, t.scroll, t.width = p.ed, p.scroll, p.width
	t.clicks = p.clicks
	if v := ggui.Untrack(t.value.Get); p.ed.Text != v {
		// Written from outside meanwhile: what was composed was for text
		// that is gone, as the binding's own effect drops it.
		if t.composition != "" {
			t.ime.Cancel()
		}
		t.ed.SetText(v)
		lo, hi := t.ed.Selection()
		t.imeStart, t.imeEnd = 0, len(t.ed.Text)
		t.ime.Rebase(t.ed.Text[:lo], t.ed.Text[hi:])
	}
	t.blink = ggui.Now()
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
	t.blink = ggui.Now()
}

// PaintCustom replaces the standard text drawing, for segmented editors such
// as Input OTP. The callback returns the logical caret bounds used by the native
// IME. The caller owns hit regions and semantics and forwards keyboard events
// and HandleTick to this editor. Layout must have run first.
func (t *TextInputWidget) PaintCustom(dst *ggui.Canvas, r ggui.Rect, paint func(*ggui.Canvas, ggui.Rect, TextInputState) ggui.Rect) {
	t.rect = r
	if dst != nil {
		t.scale = dst.Scale()
	}
	caret := paint(dst, r, t.EditingState())
	t.caretBounds = image.Rect(int(caret.Origin.X), int(caret.Origin.Y), int(caret.Origin.X+caret.Size.W), int(caret.Origin.Y+caret.Size.H))
}
