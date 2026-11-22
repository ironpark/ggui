package widget

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/internal/editlayout"
	"github.com/ironpark/ggui/internal/paragraph"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/property"
)

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
	return faceOf(t.textStyle(), scale)
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
	shown, _ := t.rendered()
	var n int
	if t.noWrap {
		// Lines break only at newlines, so counting them gives the height
		// without the line table, which paint makes if it draws.
		n = strings.Count(shown, "\n") + 1
	} else {
		n = len(t.spans(shown))
	}
	n = max(n, t.minLines)
	return c.Constrain(ggui.Sz(t.width, t.linesHeight(n)))
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
