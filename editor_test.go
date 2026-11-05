package ggui

import (
	"image"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"
	"unsafe"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/runtime"
)

// fakeIME stands in for the platform IME: a test sets its composition or
// commit and the next tick delivers it.
type fakeIME struct {
	t         *TextInputWidget
	compose   string
	commit    string
	confirmed int
	cancelled int
	handled   bool
	// passthrough commits with the key that ended the composition passed on
	// to the editor, as Korean IMEs do with Enter.
	passthrough bool
}

func (f *fakeIME) Update() (bool, error) {
	if f.commit != "" {
		f.t.imeSession()
		f.t.imeCommit(f.commit)
		f.commit = ""
		f.t.imeComposition("", 0)
		return !f.passthrough, nil
	}
	if f.compose != "" {
		f.t.imeComposition(f.compose, len(f.compose))
		return true, nil
	}
	return f.handled, nil
}

func (f *fakeIME) Confirm() {
	f.confirmed++
	f.compose = ""
	if f.t.composition != "" {
		f.t.imeSession()
		f.t.imeCommit(f.t.composition)
	}
	f.t.imeComposition("", 0)
}

func (f *fakeIME) Rebase(string, string) {}

func (f *fakeIME) Cancel() {
	f.cancelled++
	f.compose = ""
	f.t.imeComposition("", 0)
}

// useFakeIME gives the editors the test goroutine makes, and the ones its
// probes build, a fake IME. A subtest runs on a goroutine of its own and
// calls it again.
func useFakeIME(t *testing.T) {
	t.Helper()
	w := activeWorld()
	prev := w.ime
	w.ime = func(w *TextInputWidget) ime { return &fakeIME{t: w} }
	t.Cleanup(func() { w.ime = prev })
}

// typeKeys dispatches key presses to the focused region.
func typeKeys(in *inputState, mods Mods, keys ...KeyboardKey) {
	in.dispatch(frameInput{keys: keys, mods: mods})
}

func focusedInput(t *testing.T, value *StateValue[string]) (*TextInputWidget, *inputState) {
	t.Helper()
	useFakeIME(t)
	w := TextInput(value)
	var in inputState
	paintFrame(&in, w, Sz(200, 20))
	in.dispatch(frameInput{pos: Pt(190, 10), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(190, 10), up: []MouseButton{MouseButtonLeft}})
	if !w.Focused() {
		t.Fatal("click did not focus the input")
	}
	return w, &in
}

func TestTextInputEditsWriteTheSignal(t *testing.T) {
	t.Parallel()
	value := State("hello")
	changes := 0
	w, in := focusedInput(t, value)
	w.OnChange(func(string) { changes++ })
	if w.ed.Caret != len("hello") {
		t.Fatalf("click past the end put the caret at %d, want %d", w.ed.Caret, len("hello"))
	}
	typeKeys(in, Mods{}, KeyBackspace, KeyBackspace)
	if Untrack(value.Get) != "hel" || changes != 2 {
		t.Fatalf("value = %q after two backspaces, changes = %d", Untrack(value.Get), changes)
	}
	typeKeys(in, Mods{Shift: true}, KeyHome)
	typeKeys(in, Mods{}, KeyDelete)
	if Untrack(value.Get) != "" {
		t.Fatalf("shift+home then delete left %q", Untrack(value.Get))
	}
}

func TestTextInputFollowsExternalWrites(t *testing.T) {
	t.Parallel()
	value := State("abc")
	w, in := focusedInput(t, value)
	value.Set("")
	paintFrame(in, w, Sz(200, 20))
	if w.ed.Text != "" || w.ed.Caret != 0 {
		t.Fatalf("editor text %q caret %d after the signal was cleared", w.ed.Text, w.ed.Caret)
	}
}

func TestTextInputSubmitAndClipboard(t *testing.T) {
	t.Parallel()
	// No Probe runs these frames, so stand in for the loop whose
	// clipboard the editor reaches.
	clip := &runtime.MemoryClipboard{}
	loop := &frameLoop{clipboard: clip}
	setRunning(loop)
	defer reactive.ClearRunningLoop(loop)

	value := State("copy me")
	var submitted []string
	w, in := focusedInput(t, value)
	w.OnSubmit(func(s string) { submitted = append(submitted, s) })
	cmd := MustChord("cmd+a").held()
	typeKeys(in, cmd, KeyA)
	typeKeys(in, cmd, KeyC)
	if clip.Read() != "copy me" {
		t.Fatalf("clipboard = %q after select all + copy", clip.Read())
	}
	typeKeys(in, cmd, KeyX)
	if Untrack(value.Get) != "" {
		t.Fatalf("cut left %q", Untrack(value.Get))
	}
	clip.Write("line1\nline2")
	typeKeys(in, cmd, KeyV)
	if Untrack(value.Get) != "line1 line2" {
		t.Fatalf("paste gave %q, want newlines folded to spaces", Untrack(value.Get))
	}
	typeKeys(in, Mods{}, KeyEnter)
	if len(submitted) != 1 || submitted[0] != "line1 line2" {
		t.Fatalf("submitted = %v", submitted)
	}
}

func TestTextInputDragSelectsAndDoubleClickSelectsWord(t *testing.T) {
	t.Parallel()
	value := State("hello world")
	w, in := focusedInput(t, value)
	// Drag from the left edge to the far right selects everything.
	in.dispatch(frameInput{pos: Pt(0, 10), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(199, 10)})
	in.dispatch(frameInput{pos: Pt(199, 10), up: []MouseButton{MouseButtonLeft}})
	if w.ed.Selected() != "hello world" {
		t.Fatalf("drag selected %q, want everything", w.ed.Selected())
	}
	click := func(p Point) {
		in.dispatch(frameInput{pos: p, down: []MouseButton{MouseButtonLeft}})
		in.dispatch(frameInput{pos: p, up: []MouseButton{MouseButtonLeft}})
	}
	x := w.advance("hello wo")
	click(Pt(x, 10))
	click(Pt(x, 10))
	if w.ed.Selected() != "world" {
		t.Fatalf("double-click selected %q, want \"world\"", w.ed.Selected())
	}
	click(Pt(x, 10))
	if w.ed.Selected() != "hello world" {
		t.Fatalf("triple-click selected %q, want the line", w.ed.Selected())
	}
}

func TestTextInputTripleClickSelectsTheLineNotTheText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State("first\nsecond line\nthird")).Multiline()
	var in inputState
	paintFrame(&in, w, Sz(300, 100))
	p := Pt(w.advance("sec"), w.spacing()+5)
	for range 3 {
		in.dispatch(frameInput{pos: p, down: []MouseButton{MouseButtonLeft}})
		in.dispatch(frameInput{pos: p, up: []MouseButton{MouseButtonLeft}})
	}
	if w.ed.Selected() != "second line" {
		t.Fatalf("triple-click selected %q, want the second line", w.ed.Selected())
	}
}

func TestTextInputWrapsOnceForAnUnchangedText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State("one two three four five six seven")).Multiline()
	w.Layout(Loose(Sz(80, Unbounded)), Env{})
	a, b := w.spans(w.ed.Text), w.spans(w.ed.Text)
	if len(a) < 2 || &a[0] != &b[0] {
		t.Fatal("the same text at the same width was wrapped again")
	}
	w.Layout(Loose(Sz(400, Unbounded)), Env{})
	if c := w.spans(w.ed.Text); len(c) != 1 {
		t.Fatalf("after widening, %d lines; want the cache to follow the width", len(c))
	}
}

func TestTextInputComposesThenCommits(t *testing.T) {
	t.Parallel()
	value := State("ab")
	w, in := focusedInput(t, value)
	typeKeys(in, Mods{}, KeyArrowLeft)
	f := w.ime.(*fakeIME)
	f.compose = "ㅎ"
	in.dispatch(frameInput{keys: []KeyboardKey{KeyBackspace}})
	// The IME swallowed the key: nothing was deleted.
	if Untrack(value.Get) != "ab" || w.composition != "ㅎ" {
		t.Fatalf("value = %q composition = %q; the IME should have swallowed the key", Untrack(value.Get), w.composition)
	}
	if shown, caret := w.rendered(); shown != "aㅎb" || caret != 1+len("ㅎ") {
		t.Fatalf("rendered = %q caret %d", shown, caret)
	}
	f.compose, f.commit = "", "한"
	in.dispatch(frameInput{})
	if Untrack(value.Get) != "a한b" || w.composition != "" || w.ed.Caret != 1+len("한") {
		t.Fatalf("value = %q composition = %q caret %d after commit", Untrack(value.Get), w.composition, w.ed.Caret)
	}
	// Moving the caret confirms whatever is being composed: the IME went
	// idle with "ㄱ" still shown, and End commits it before moving.
	f.compose = "ㄱ"
	in.dispatch(frameInput{})
	f.compose = ""
	typeKeys(in, Mods{}, KeyEnd)
	if Untrack(value.Get) != "a한ㄱb" || f.confirmed == 0 {
		t.Fatalf("value = %q, confirmed %d times", Untrack(value.Get), f.confirmed)
	}
}

func TestTextInputSurvivesRebuildWithCaret(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := State("abc")
	build := func() Widget { return TextInput(value) }
	var in inputState
	first := build()
	paintFrame(&in, first, Sz(200, 20))
	clickAt(&in, Pt(100, 10))
	typeKeys(&in, Mods{}, KeyArrowLeft)
	second := build().(*TextInputWidget)
	paintFrame(&in, second, Sz(200, 20))
	typeKeys(&in, Mods{}, KeyBackspace)
	if !second.Focused() || Untrack(value.Get) != "ac" {
		t.Fatalf("rebuilt input focused = %v, value = %q; want focus and caret carried over", second.Focused(), Untrack(value.Get))
	}
}

func TestTextInputPasswordHidesText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State("abc")).Password()
	w.Layout(Loose(Sz(200, 20)), Env{})
	if w.display("abc") != "•••" {
		t.Fatalf("display = %q", w.display("abc"))
	}
	if w.advance("abc") != w.advance("xyz") {
		t.Fatal("masked strings of equal length measure differently")
	}
}

func TestTextInputFillsBoundedWidth(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State(""))
	if got := w.Layout(Loose(Sz(300, 100)), Env{}); got.W != 300 {
		t.Fatalf("width = %v, want 300", got.W)
	}
	if got := w.Layout(Loose(Sz(Unbounded, 100)), Env{}); got.W != 120 {
		t.Fatalf("unbounded width = %v, want the 120 minimum", got.W)
	}
}

func TestTextInputMultilineWrapsGrowsAndNavigates(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State("")).Multiline()
	one := w.Layout(Loose(Sz(200, Unbounded)), Env{})
	if one.H != w.linesHeight(3) || one.W != 200 {
		t.Fatalf("empty multiline is %v, want 3 lines tall and 200 wide", one)
	}
	long := "the quick brown fox jumps over the lazy dog and keeps on running far away"
	w.value.Set(long)
	reactive.Flush() // the editor follows its binding through an effect
	grown := w.Layout(Loose(Sz(200, Unbounded)), Env{})
	spans := w.spans(long)
	if len(spans) < 3 || grown.H != w.linesHeight(len(spans)) {
		t.Fatalf("wrapped into %d lines at height %v", len(spans), grown.H)
	}
	if fixed := w.Layout(Tight(Sz(200, 30)), Env{}); fixed.H != 30 {
		t.Fatalf("a tight height was not honoured: %v", fixed)
	}

	var in inputState
	paintFrame(&in, w, Sz(200, grown.H))
	in.dispatch(frameInput{pos: Pt(1, 5), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(1, 5), up: []MouseButton{MouseButtonLeft}})
	if !w.Focused() || w.ed.Caret != 0 {
		t.Fatalf("click at the top left: focused %v caret %d", w.Focused(), w.ed.Caret)
	}
	typeKeys(&in, Mods{}, KeyEnd)
	if w.ed.Caret != spans[0].End {
		t.Fatalf("End went to %d, want the end of the first line %d", w.ed.Caret, spans[0].End)
	}
	typeKeys(&in, Mods{}, KeyArrowDown)
	if got := textlayout.LineOf(spans, w.ed.Caret); got != 1 {
		t.Fatalf("Down landed on line %d", got)
	}
	typeKeys(&in, Mods{}, KeyHome)
	if w.ed.Caret != spans[1].Start {
		t.Fatalf("Home on line 2 went to %d, want %d", w.ed.Caret, spans[1].Start)
	}
	typeKeys(&in, Mods{}, KeyEnter)
	if got := Untrack(w.value.Get); got[spans[1].Start] != '\n' {
		t.Fatalf("Enter did not insert a line break: %q", got)
	}
	submitted := ""
	w.OnSubmit(func(s string) { submitted = s })
	typeKeys(&in, MustChord("cmd+enter").held(), KeyEnter)
	if submitted == "" {
		t.Fatal("Cmd+Enter did not submit")
	}
	// A click on the second line lands there.
	paintFrame(&in, w, Sz(200, grown.H))
	in.dispatch(frameInput{pos: Pt(5, w.spacing()+2), down: []MouseButton{MouseButtonLeft}})
	if got := textlayout.LineOf(w.spans(w.ed.Text), w.ed.Caret); got != 1 {
		t.Fatalf("click on the second line put the caret on line %d", got)
	}
}

func TestUndoKeys(t *testing.T) {
	t.Parallel()
	v := State("hello")
	in := TextInput(v)
	p := NewProbe(in, Sz(200, 30))
	defer p.Close()
	p.Click(Pt(190, 10))
	p.Type(Mods{}, KeyBackspace, KeyBackspace)
	if Untrack(v.Get) != "hel" {
		t.Fatalf("value %q after two backspaces", Untrack(v.Get))
	}
	p.Key("cmd+z")
	if Untrack(v.Get) != "hell" {
		t.Fatalf("value %q after undo, want hell", Untrack(v.Get))
	}
	p.Key("cmd+shift+z")
	if Untrack(v.Get) != "hel" {
		t.Fatalf("value %q after redo, want hel", Untrack(v.Get))
	}
}

func TestTextInputKeyHookLeavesCompositionWithIME(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	calls := 0
	w := TextInput(State("")).OnKey(func(ev KeyEvent) bool { calls++; return ev.Key == KeyArrowDown })
	w.HandleKey(KeyEvent{Kind: KeyPress, Key: KeyArrowDown})
	if calls != 1 {
		t.Fatal("key hook did not receive navigation")
	}
	w.imeComposition("ㅎ", len("ㅎ"))
	w.HandleKey(KeyEvent{Kind: KeyPress, Key: KeyArrowDown})
	if calls != 1 {
		t.Fatal("hook intercepted an IME composition")
	}
	w.HandleKey(KeyEvent{Kind: KeyPress, Key: KeyEscape})
	if calls != 1 || !w.ConsumesKey(KeyEvent{Kind: KeyPress, Key: KeyEscape}) {
		t.Fatal("composition Escape escaped to a popup")
	}
	w.HandleKey(KeyEvent{Kind: KeyPress, Key: KeyEscape})
	if calls != 2 || w.ConsumesKey(KeyEvent{Kind: KeyPress, Key: KeyEscape}) {
		t.Fatal("ordinary Escape was not released")
	}
}

func TestNoWrapBreaksOnlyAtLineBreaks(t *testing.T) {
	useFakeIME(t)
	w := TextInput(State("a long first line that would wrap\nsecond")).NoWrap()
	if got := w.spans(w.ed.Text); len(got) != 2 || got[1] != (textlayout.Span{Start: 34, End: 40}) {
		t.Fatalf("spans = %v, want the two lines as written", got)
	}
	var in inputState
	paintFrame(&in, w, Sz(60, 60))
	w.ed.MoveTo(36, false)
	w.HandleKey(KeyEvent{Kind: KeyPress, Key: KeyArrowUp})
	if w.ed.Caret > 33 {
		t.Fatalf("Up left the caret at %d, not on the first line", w.ed.Caret)
	}
}

func TestReplaceIsOneUndoStep(t *testing.T) {
	v := State("ab")
	w, _ := focusedInput(t, v)
	w.Select(1, 1)
	w.Replace("\n    ")
	if got := Peek(v); got != "a\n    b" {
		t.Fatalf("value = %q", got)
	}
	w.HandleKey(KeyEvent{Kind: KeyPress, Key: KeyZ, Mods: Mods{Meta: runtimeIsDarwin(), Ctrl: !runtimeIsDarwin()}})
	if got := Peek(v); got != "ab" {
		t.Fatalf("after undo value = %q, want ab", got)
	}
}

func TestTheComposedTextIsBuiltOncePerChange(t *testing.T) {
	t.Parallel()
	value := State("ab")
	w, in := focusedInput(t, value)
	typeKeys(in, Mods{}, KeyArrowLeft)
	f := w.ime.(*fakeIME)
	f.compose = "ㅎ"
	w.HandleTick()
	a, _ := w.rendered()
	b := w.EditingState().Text
	if a != "aㅎb" || unsafe.StringData(a) != unsafe.StringData(b) {
		t.Fatalf("rendered %q, then %q built again", a, b)
	}
	f.compose = "하"
	w.HandleTick()
	if c, _ := w.rendered(); c != "a하b" {
		t.Fatalf("after the composition changed, rendered %q", c)
	}
}

func TestNoWrapLayoutCountsLinesWithoutTheirTable(t *testing.T) {
	useFakeIME(t)
	w := TextInput(State("1\n2\n3\n4\n5\n6")).NoWrap()
	one := TextInput(State("one")).NoWrap()
	h3 := w.Layout(Loose(Sz(200, Unbounded)), Env{}).H
	h1 := one.Layout(Loose(Sz(200, Unbounded)), Env{}).H
	if h3 <= h1 || w.wrapped[0] != nil {
		t.Fatalf("six lines %v tall against one's %v; line table made: %v", h3, h1, w.wrapped[0] != nil)
	}
}

// Enter over a syllable being composed reaches OnKey after the syllable is
// in the text when the IME passes the key on, as a Korean one does, so a
// field that sends on Enter sends the whole word; an IME that keeps the key
// to confirm its conversion, as a Japanese one does, sends nothing.
func TestEnterThatEndsACompositionSeesItsCommit(t *testing.T) {
	t.Parallel()
	for _, passthrough := range []bool{true, false} {
		value := State("안녕")
		w, in := focusedInput(t, value)
		var sent []string
		w.OnKey(func(ev KeyEvent) bool {
			if ev.Key == KeyEnter {
				sent = append(sent, Untrack(value.Get))
				return true
			}
			return false
		})
		f := w.ime.(*fakeIME)
		f.compose = "하"
		in.dispatch(frameInput{})
		f.compose, f.commit, f.passthrough = "", "하", passthrough
		typeKeys(in, Mods{}, KeyEnter)
		want := []string{"안녕하"}
		if !passthrough {
			want = nil
		}
		if !slices.Equal(sent, want) || Untrack(value.Get) != "안녕하" {
			t.Errorf("passthrough %v: sent %q, value %q; want %q", passthrough, sent, Untrack(value.Get), want)
		}
	}
}

// A write from outside leaves a caret that was at the end at the new end,
// so text appended or swapped in is typed after; one inside the text stays.
func TestOutsideWritesKeepACaretAtTheEnd(t *testing.T) {
	t.Parallel()
	value := State("")
	w, _ := focusedInput(t, value)
	value.Set("see /tmp/a.png")
	reactive.Flush()
	if w.ed.Caret != len("see /tmp/a.png") {
		t.Fatalf("caret %d after a write into an empty field, want the end", w.ed.Caret)
	}
	w.ed.MoveTo(3, false)
	value.Set("see /tmp/a.png and b")
	reactive.Flush()
	if w.ed.Caret != 3 {
		t.Fatalf("caret %d, want it left inside the text", w.ed.Caret)
	}
}

// A paste of a clipboard holding an image and no text goes to
// OnPasteImage; one with text pastes the text.
func TestPasteOfAnImageGoesToItsHook(t *testing.T) {
	useFakeIME(t)
	value := State("")
	var got []byte
	p := NewProbe(TextInput(value).OnPasteImage(func(png []byte) bool { got = png; return true }), Sz(200, 20))
	defer p.Close()
	var clip *runtime.MemoryClipboard
	p.OnFrame(func() { clip, _ = CurrentClipboard().(*runtime.MemoryClipboard) })
	p.Click(Pt(10, 10))
	paste := Mods{Meta: runtimeIsDarwin(), Ctrl: !runtimeIsDarwin()}
	clip.WriteImage([]byte("png"))
	p.Type(paste, KeyV)
	if string(got) != "png" || Untrack(value.Get) != "" {
		t.Fatalf("hook got %q, value %q", got, Untrack(value.Get))
	}
	clip.Write("text")
	p.Type(paste, KeyV)
	if Untrack(value.Get) != "text" {
		t.Fatalf("value %q after pasting text", Untrack(value.Get))
	}
}

// A key a text input claims is its own while it claims it: Tab reaches
// its OnKey rather than moving the focus, and moves it again once the
// input lets it go.
func TestTextInputClaimsTheKeysItIsAsked(t *testing.T) {
	t.Parallel()
	claim := true
	seen := 0
	a := TextInput(State("")).Name("a").
		ClaimKeys(func(ev KeyEvent) bool { return claim && ev.Key == KeyTab }).
		OnKey(func(ev KeyEvent) bool { seen++; return ev.Key == KeyTab })
	b := TextInput(State("")).Name("b")
	p := NewProbe(Column(a, b), Sz(200, 100))
	defer p.Close()
	p.Frame()
	field, _ := p.Semantics().Find(RoleTextField, "a")
	p.Click(field.Rect.Center())
	focused := func() string { n, _ := p.Semantics().Focused(); return n.Name }
	p.Type(Mods{}, KeyTab)
	if seen != 1 || focused() != "a" {
		t.Fatalf("a claimed Tab: OnKey saw %d keys, %q focused", seen, focused())
	}
	claim = false
	p.Type(Mods{}, KeyTab)
	if seen != 1 || focused() != "b" {
		t.Fatalf("a let Tab go: OnKey saw %d keys, %q focused", seen, focused())
	}
}

func TestTextInputFilterNativeCommitsAndCaret(t *testing.T) {
	t.Parallel()
	v := State("")
	e := TextInput(v).Filter(func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsDigit(r) {
				return r
			}
			return -1
		}, s)
	})
	e.imeCommit("1a2-3")
	if v.Get() != "123" || e.ed.Caret != 3 {
		t.Fatalf("filtered commit %q caret %d", v.Get(), e.ed.Caret)
	}
	e.Select(1, 2)
	e.imeCommit("x9")
	if v.Get() != "193" || e.ed.Caret != 2 {
		t.Fatal("replacement")
	}
	e.imeStart, e.imeEnd = 0, 3
	e.imeReplace("1", "z8", "3")
	if v.Get() != "183" || e.ed.Caret != 2 {
		t.Fatal("IME replace")
	}
	e.Layout(Loose(Sz(200, 40)), Env{})
	e.PaintCustom(nil, Rct(Point{}, Sz(200, 40)), func(_ *Canvas, _ Rect, s TextInputState) Rect {
		if s.Text != "183" {
			t.Fatal(s)
		}
		return Rct(Pt(32, 8), Sz(1, 16))
	})
	if e.caretBounds.Min.X != 32 {
		t.Fatal("custom IME caret")
	}
}

// Native window APIs consume logical coordinates even on HiDPI displays.
func TestTextInputIMECaretUsesLogicalCoordinates(t *testing.T) {
	t.Parallel()
	for _, scale := range []float64{1, 1.5, 2} {
		for _, multiline := range []bool{false, true} {
			e := TextInput(State(""))
			e.multiline = multiline
			e.Layout(Loose(Sz(200, 80)), Env{})
			c := &Canvas{scale: scale}
			e.Paint(c, Rct(Pt(32, 8), Sz(200, 80)))
			want := image.Rect(32, 8, 33, int(8+e.height()))
			if got := e.imeSession().CaretBounds; got != want {
				t.Fatalf("scale %g multiline %v: got %v, want %v", scale, multiline, got, want)
			}
			e.PaintCustom(c, Rct(Point{}, Sz(200, 80)), func(*Canvas, Rect, TextInputState) Rect {
				return Rct(Pt(32, 8), Sz(1, 16))
			})
			if got := e.imeSession().CaretBounds; got != image.Rect(32, 8, 33, 24) {
				t.Fatalf("scale %g custom: got %v", scale, got)
			}
		}
	}
}

func TestTextInputIsFoundByPlaceholderThenName(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State("")).Placeholder("Search")
	p := NewProbe(w, Sz(200, 30))
	defer p.Close()
	if _, ok := p.FindRole(RoleTextField, "Search"); !ok {
		t.Fatal("an unnamed field is not found by its placeholder")
	}
	w.Name("Query")
	if _, ok := p.FindRole(RoleTextField, "Query"); !ok {
		t.Fatal("a named field is not found by its name")
	}
	name := State("First")
	w.BindName(name)
	if _, ok := p.FindRole(RoleTextField, "First"); !ok {
		t.Fatal("a field is not found by its bound name")
	}
	name.Set("Second")
	if _, ok := p.FindRole(RoleTextField, "Second"); !ok || !w.HasName() {
		t.Fatal("the field's name did not follow its binding")
	}
}

func TestTextInputBindDisabledStopsInputAndActions(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value, disabled := State("abc"), State(true)
	w := TextInput(value).BindDisabled(disabled)
	p := NewProbe(w, Sz(200, 30))
	defer p.Close()
	p.Click(Pt(195, 5))
	p.Type(Mods{}, KeyBackspace)
	if w.Focused() || Untrack(value.Get) != "abc" {
		t.Fatalf("a disabled field focused %v, value %q; want no focus and no edit", w.Focused(), Untrack(value.Get))
	}
	node, ok := p.Semantics().Find(RoleTextField, "")
	if !ok || !node.Disabled {
		t.Fatalf("a disabled field is described as %+v, want Disabled", node.Node)
	}
	p.Perform(node.ID, Action{Kind: ActionSetValue, Text: "set"})
	if got := Untrack(value.Get); got != "abc" {
		t.Fatalf("an assistive action set a disabled field to %q", got)
	}

	disabled.Set(false)
	p.Click(Pt(195, 5))
	p.Type(Mods{}, KeyBackspace)
	if got := Untrack(value.Get); !w.Focused() || got != "ab" {
		t.Fatalf("re-enabled field: focused %v, value %q; want focus and \"ab\"", w.Focused(), got)
	}
	node, _ = p.Semantics().Find(RoleTextField, "")
	p.Perform(node.ID, Action{Kind: ActionSetValue, Text: "set"})
	if got := Untrack(value.Get); got != "set" {
		t.Fatalf("ActionSetValue gave %q, want \"set\"", got)
	}
}

func TestTextInputInheritsDisabledFromItsSubtree(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := State("abc")
	w := TextInput(value)
	p := NewProbe(Provide(InputDisabledKey, true, w), Sz(200, 30))
	defer p.Close()
	p.Click(Pt(195, 5))
	p.Type(Mods{}, KeyBackspace)
	if !w.IsDisabled() || w.Focused() || Untrack(value.Get) != "abc" {
		t.Fatalf("under InputDisabledKey: disabled %v, focused %v, value %q", w.IsDisabled(), w.Focused(), Untrack(value.Get))
	}
}

func TestTextInputSizesFromStyleLinesAndMinWidth(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	free := Loose(Sz(Unbounded, Unbounded))
	plain := TextInput(State(""))
	one := plain.Layout(free, Env{})
	if got := TextInput(State("")).MinWidth(50).Layout(free, Env{}); got.W != 50 {
		t.Fatalf("MinWidth(50) asks for %v wide with the width left to it", got.W)
	}
	big := TextInput(State("")).Style(TextStyle{Size: 2 * DefaultTextSize})
	if got := big.Layout(free, Env{}); got.H <= one.H*1.5 {
		t.Fatalf("a field at twice the text size is %v tall, against %v", got.H, one.H)
	}
	two := TextInput(State("")).Lines(2)
	if got := two.Layout(Loose(Sz(200, Unbounded)), Env{}); !two.multiline || got.H != two.linesHeight(2) {
		t.Fatalf("Lines(2) is %v tall (multiline %v), want two lines, %v", got.H, two.multiline, two.linesHeight(2))
	}
	least := TextInput(State("")).Lines(0)
	if got := least.Layout(Loose(Sz(200, Unbounded)), Env{}); got.H != least.linesHeight(1) {
		t.Fatalf("Lines(0) is %v tall, want at least one line", got.H)
	}
}

func TestTextInputBaselineIsTheFirstLineAscent(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State("x"))
	if _, ok := w.Baseline(); ok {
		t.Fatal("a field reports a baseline before it has been laid out")
	}
	w.Layout(Loose(Sz(200, 30)), Env{})
	got, ok := w.Baseline()
	if want := w.face(1).Metrics().HAscent; !ok || got != want {
		t.Fatalf("baseline = %v, %v; want the ascent %v", got, ok, want)
	}
	// Beside a box, which sits on its bottom edge, the field's baseline
	// lines up with that edge.
	row := Row(Box().Size(10, 40), w).Align(AlignBaseline)
	row.Layout(Loose(Sz(300, 100)), Env{})
	if b, ok := row.Baseline(); !ok || !closeTo(b, 40) {
		t.Fatalf("row baseline = %v, %v; want the field's, on the box's bottom at 40", b, ok)
	}
}

func TestTextInputKeyKeepsCaretAndFocusWhenMoved(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := State("hello")
	moved := State(false)
	var input *TextInputWidget
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			input = TextInput(value).Key("query")
			if moved.Get() {
				return Column(Box().Size(10, 40), input)
			}
			return Column(input)
		})
	}, Sz(200, 100))
	defer p.Close()
	p.Click(Pt(195, 5))
	p.Key("left", "left")
	moved.Set(true)
	p.Frame()
	p.Key("backspace")
	if got := Untrack(value.Get); !input.Focused() || got != "helo" {
		t.Fatalf("the moved field: focused %v, value %q; want focus and the caret carried over (\"helo\")", input.Focused(), got)
	}
}

// editorProbe focuses a TextInput holding text in a probe, with the caret
// at the end, and returns the editor, the probe and the bound value. The
// space below the editor takes no input, so a click there blurs it.
func editorProbe(t *testing.T, text string, configure ...func(*TextInputWidget)) (*TextInputWidget, *Probe, *StateValue[string]) {
	t.Helper()
	useFakeIME(t)
	value := State(text)
	w := TextInput(value)
	for _, fn := range configure {
		fn(w)
	}
	p := NewProbe(Column(w, Box().Size(300, 100)), Sz(300, 140))
	t.Cleanup(p.Close)
	p.Click(Pt(295, 5))
	if !w.Focused() {
		t.Fatal("a click on the editor did not focus it")
	}
	return w, p, value
}

// selection returns what the editor has selected and where its caret is.
func selection(w *TextInputWidget) (string, int) { return w.ed.Selected(), w.ed.Caret }

func TestTextInputArrowsMoveByGraphemeAndWord(t *testing.T) {
	t.Parallel()
	w, p, _ := editorProbe(t, "hello big world")
	steps := []struct {
		chord    string
		selected string
		caret    int
	}{
		{"left", "", 14},
		{"home", "", 0},
		{"end", "", 15},
		{"alt+left", "", 10},          // to the start of "world"
		{"alt+shift+left", "big ", 6}, // extends by a word
		{"right", "", 10},             // a selection collapses to its end
		{"shift+home", "hello big ", 0},
		{"left", "", 0},      // ...or to its start
		{"alt+right", "", 6}, // over "hello" and the space after it
		{"shift+end", "big world", 15},
		{"escape", "", 15}, // drops the selection, keeping the caret
	}
	for _, s := range steps {
		p.Key(s.chord)
		if sel, caret := selection(w); sel != s.selected || caret != s.caret {
			t.Fatalf("after %s: selected %q, caret %d; want %q, %d", s.chord, sel, caret, s.selected, s.caret)
		}
	}
}

func TestTextInputStepsOverWholeGraphemes(t *testing.T) {
	t.Parallel()
	text := "aé👍🏽"
	w, p, value := editorProbe(t, text)
	p.Key("left")
	if w.ed.Caret != len("aé") {
		t.Fatalf("Left from the end put the caret at %d, inside the emoji with its modifier", w.ed.Caret)
	}
	p.Key("left")
	if w.ed.Caret != 1 {
		t.Fatalf("Left put the caret at %d, inside e and its accent", w.ed.Caret)
	}
	p.Key("delete")
	if got := Untrack(value.Get); got != "a👍🏽" {
		t.Fatalf("Delete left %q, want the accented e gone whole", got)
	}
}

func TestTextInputDeletesByGraphemeOrWord(t *testing.T) {
	t.Parallel()
	_, p, value := editorProbe(t, "one two three")
	steps := []struct{ chord, want string }{
		{"backspace", "one two thre"},
		{"alt+backspace", "one two "},
		{"home", "one two "},
		{"delete", "ne two "},
		{"alt+delete", "two "},
		{"shift+end", "two "},
		{"backspace", ""},
		{"backspace", ""}, // nothing left to delete
	}
	for _, s := range steps {
		p.Key(s.chord)
		if got := Untrack(value.Get); got != s.want {
			t.Fatalf("after %s: value %q, want %q", s.chord, got, s.want)
		}
	}
}

func TestTextInputEscapeReachesAShortcutUnlessItCancelsComposition(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "ab")
	escapes := 0
	p.Shortcut("escape", func() { escapes++ })
	p.Key("escape")
	if escapes != 1 {
		t.Fatalf("Escape with nothing composing ran the shortcut %d times, want 1", escapes)
	}
	f := w.ime.(*fakeIME)
	f.compose = "ㅎ"
	p.Move(Pt(5, 5)) // the IME runs on the frame
	f.compose = ""   // and goes idle with the composition still shown
	if w.composition != "ㅎ" {
		t.Fatalf("composition = %q, want the IME's", w.composition)
	}
	cancelled := f.cancelled
	p.Key("escape")
	if escapes != 1 || w.composition != "" || f.cancelled != cancelled+1 {
		t.Fatalf("Escape while composing: shortcut ran %d times, composition %q, cancelled %d more; want it to cancel only", escapes, w.composition, f.cancelled-cancelled)
	}
	if got := Untrack(value.Get); got != "ab" {
		t.Fatalf("a cancelled composition left %q", got)
	}
}

func TestTextInputCommitReplacesTheSelection(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "hello world")
	p.Key("alt+shift+left")
	f := w.ime.(*fakeIME)
	f.commit = "there"
	p.Move(Pt(5, 5))
	if got := Untrack(value.Get); got != "hello there" || w.ed.Caret != len(got) {
		t.Fatalf("committing over the selection gave %q with the caret at %d", got, w.ed.Caret)
	}
}

func TestTextInputBlurConfirmsCompositionAndCommits(t *testing.T) {
	t.Parallel()
	var committed []string
	w, p, value := editorProbe(t, "a", func(w *TextInputWidget) {
		w.OnCommit(func(s string) { committed = append(committed, s) })
	})
	f := w.ime.(*fakeIME)
	f.compose = "ㄱ"
	p.Move(Pt(5, 5))
	f.compose = ""
	p.Click(Pt(150, 100)) // below the editor
	if w.Focused() {
		t.Fatal("a click outside the editor left it focused")
	}
	if got := Untrack(value.Get); got != "aㄱ" {
		t.Fatalf("blur left %q, want the composition confirmed into the value", got)
	}
	if len(committed) != 1 || committed[0] != "aㄱ" {
		t.Fatalf("OnCommit got %q, want one call with the confirmed value", committed)
	}
	p.Click(Pt(295, 5))
	p.Key("enter")
	if len(committed) != 2 {
		t.Fatalf("Enter committed %d times in all, want a second OnCommit", len(committed))
	}
}

func TestTextInputSurroundingTextReplacementKeepsTheRest(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := State("teh cat")
	w := TextInput(value)
	w.ed.MoveTo(len("teh"), false)
	w.imeSession()
	// An autocorrect rewrote the text the session was told about.
	w.imeReplace("the", " cat", "")
	if got := Untrack(value.Get); got != "the cat" {
		t.Fatalf("replacement gave %q", got)
	}
	if w.ed.Caret != len("the cat") {
		t.Fatalf("caret at %d, want after the replaced text", w.ed.Caret)
	}
}

func TestTextInputTypingUndoesAWordAtATime(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "")
	p.Advance(0) // pins the probe's clock, which decides what undoes together
	f := w.ime.(*fakeIME)
	commit := func(s string) {
		f.commit = s
		p.Move(Pt(5, 5))
	}
	commit("a")
	commit("b")
	p.Advance(time.Second)
	commit("c")
	if got := Untrack(value.Get); got != "abc" {
		t.Fatalf("typed %q, want abc", got)
	}
	p.Key("cmd+z")
	if got := Untrack(value.Get); got != "ab" {
		t.Fatalf("undo after a pause gave %q, want ab: c was typed a second later", got)
	}
	p.Key("cmd+z")
	if got := Untrack(value.Get); got != "" {
		t.Fatalf("undo gave %q, want a and b, typed together, undone at once", got)
	}
	changes := 0
	w.OnChange(func(string) { changes++ })
	p.Key("cmd+z")
	if changes != 0 {
		t.Fatal("undo with nothing left wrote the value")
	}
	// Ctrl+Y redoes except on macOS, where Shift+Cmd+Z does.
	p.Key("cmd+y")
	want := "ab"
	if runtimeIsDarwin() {
		want = ""
	}
	if got := Untrack(value.Get); got != want {
		t.Fatalf("cmd+y gave %q, want %q on this platform", got, want)
	}
}

func TestTextInputClipboardKeys(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "cut me")
	p.Clipboard().Write("before")
	p.Key("cmd+c")
	if got := p.Clipboard().Read(); got != "before" {
		t.Fatalf("copy with nothing selected wrote %q", got)
	}
	p.Key("cmd+x")
	if got := Untrack(value.Get); got != "cut me" {
		t.Fatalf("cut with nothing selected left %q", got)
	}
	p.Key("alt+shift+left", "cmd+x")
	if got, clip := Untrack(value.Get), p.Clipboard().Read(); got != "cut " || clip != "me" {
		t.Fatalf("cut gave value %q, clipboard %q; want \"cut \" and \"me\"", got, clip)
	}
	p.Key("home", "cmd+v")
	if got := Untrack(value.Get); got != "mecut " || w.ed.Caret != 2 {
		t.Fatalf("paste gave %q with the caret at %d, want \"mecut \" at 2", got, w.ed.Caret)
	}
	// A key without the command modifier is not a clipboard shortcut.
	p.Key("alt+v")
	if got := Untrack(value.Get); got != "mecut " {
		t.Fatalf("Alt+V changed the value to %q", got)
	}
}

func TestTextInputPasswordIsNeverCopied(t *testing.T) {
	t.Parallel()
	_, p, value := editorProbe(t, "hunter2", func(w *TextInputWidget) { w.Password() })
	p.Clipboard().Write("before")
	p.Key("cmd+a", "cmd+c")
	if got := p.Clipboard().Read(); got != "before" {
		t.Fatalf("copying a password put %q on the clipboard", got)
	}
	p.Key("cmd+x")
	if got, clip := Untrack(value.Get), p.Clipboard().Read(); got != "" || clip != "before" {
		t.Fatalf("cutting a password left value %q, clipboard %q; want it deleted and not copied", got, clip)
	}
}

func TestTextInputMultilinePasteKeepsLineBreaks(t *testing.T) {
	t.Parallel()
	_, p, value := editorProbe(t, "", func(w *TextInputWidget) { w.Multiline() })
	p.Clipboard().Write("one\r\ntwo\rthree\nfour")
	p.Key("cmd+v")
	if got := Untrack(value.Get); got != "one\ntwo\nthree\nfour" {
		t.Fatalf("paste gave %q, want every CRLF and CR turned into a line feed", got)
	}
	// Meta+Up and Down reach the ends of the text: the macOS form.
	p.Type(Mods{Meta: true}, KeyArrowUp)
	if got := Untrack(value.Get); got != "one\ntwo\nthree\nfour" {
		t.Fatalf("Meta+Up edited the text: %q", got)
	}
}

func TestTextInputTripleClickSelectsAll(t *testing.T) {
	t.Parallel()
	w, p, _ := editorProbe(t, "one two")
	p.Advance(0)
	at := Pt(w.advance("o"), 5)
	p.Click(at)
	p.Click(at)
	if sel, _ := selection(w); sel != "one" {
		t.Fatalf("double click selected %q, want the word", sel)
	}
	p.Click(at)
	if sel, _ := selection(w); sel != "one two" {
		t.Fatalf("triple click selected %q, want everything", sel)
	}
	// A click after a pause starts over.
	p.Advance(time.Second)
	p.Click(at)
	if sel, caret := selection(w); sel != "" || caret != 1 {
		t.Fatalf("a later click selected %q with the caret at %d, want a caret at 1", sel, caret)
	}
}

// Ctrl jumps words on every platform: off macOS it is also Cmd, but no
// Cmd shortcut uses an arrow or a delete.
func TestTextInputWordKeysWithCtrl(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "hello big world")
	p.Type(Mods{Ctrl: true}, KeyArrowLeft)
	if w.ed.Caret != len("hello big ") {
		t.Fatalf("Ctrl+Left put the caret at %d, want the start of the last word", w.ed.Caret)
	}
	p.Type(Mods{Ctrl: true}, KeyBackspace)
	if got := Untrack(value.Get); got != "hello world" {
		t.Fatalf("Ctrl+Backspace left %q, want the word before the caret gone", got)
	}
}

func TestPasswordSelectionIsReportedInBullets(t *testing.T) {
	t.Parallel()
	w, _, _ := editorProbe(t, "héllo", func(w *TextInputWidget) { w.Password() })
	w.Act(Action{Kind: ActionSetSelection, SelStart: len(bullet), SelEnd: 3 * len(bullet)})
	if lo, hi := w.ed.Selection(); w.ed.Text[lo:hi] != "él" {
		t.Fatalf("selecting bullets 1 to 3 selected %q, want \"él\"", w.ed.Text[lo:hi])
	}
	n := w.Describe()
	if n.Value != "•••••" || n.SelStart != len(bullet) || n.SelEnd != 3*len(bullet) {
		t.Fatalf("described as %q selecting %d-%d, want five bullets selecting %d-%d", n.Value, n.SelStart, n.SelEnd, len(bullet), 3*len(bullet))
	}
}

// Meta+Left and Right move to the ends of the caret's line in a multiline
// field, as Cmd+Left and Right do on macOS, not to the ends of the text.
func TestTextInputMetaArrowsKeepToTheLine(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "one\ntwo\nthree", func(w *TextInputWidget) { w.Multiline() })
	w.Select(len("one\ntw"), len("one\ntw"))
	p.Type(Mods{Meta: true}, KeyArrowLeft)
	if lo, hi := w.ed.Anchor, w.ed.Caret; lo != len("one\n") || hi != lo {
		t.Fatalf("Meta+Left put the caret at %d..%d, want the start of the second line, %d", lo, hi, len("one\n"))
	}
	p.Type(Mods{Meta: true, Shift: true}, KeyArrowRight)
	if sel, _ := selection(w); sel != "two" {
		t.Fatalf("Shift+Meta+Right selected %q, want the line", sel)
	}
	if got := Untrack(value.Get); got != "one\ntwo\nthree" {
		t.Fatalf("moving edited the text: %q", got)
	}
}

// editorDoc is a text of n lines, some wrapping at the widths the tests
// lay it out at, some blank.
func editorDoc(n int) string {
	words := strings.Fields("the quick brown fox jumps over the lazy dog and keeps on running far away 한글 é")
	var lines []string
	for i := range n {
		if i%7 == 3 {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, strings.Join(words[:1+i%len(words)], " "))
	}
	return strings.Join(lines, "\n")
}

// A multiline editor wraps an edit again line by line, keeping the lines
// it did not change; what it finds must be what wrapping the whole text
// afresh finds, after edits anywhere, edits that split and join lines,
// and a new width or size, which change every line.
func TestTextInputWrapsEditsAsTheWholeText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State(editorDoc(60))).Multiline()
	layout := func(width float64) { w.Layout(Loose(Sz(width, Unbounded)), Env{}) }
	check := func(what string) {
		t.Helper()
		got := w.spans(w.ed.Text)
		if want := textlayout.WrapSpans(w.ed.Text, w.face(1), w.width); !slices.Equal(got, want) {
			t.Fatalf("after %s, the wrap is %v, want %v", what, got, want)
		}
	}
	layout(160)
	check("the first layout")
	edit := func(what string, at func() int, s string) {
		t.Helper()
		w.ed.MoveTo(at(), false)
		w.ed.Replace(s)
		layout(160)
		check(what)
	}
	edit("typing at the start", func() int { return 0 }, "x")
	edit("typing in the middle", func() int { return len(w.ed.Text) / 2 }, " more words to wrap ")
	edit("typing at the end", func() int { return len(w.ed.Text) }, " the end")
	edit("splitting a line", func() int { return len(w.ed.Text) / 3 }, "\n")
	edit("pasting lines", func() int { return len(w.ed.Text) / 4 }, "one\ntwo three four five six seven\n\nfour")
	// Joining lines: delete the line break after the middle.
	w.ed.MoveTo(strings.IndexByte(w.ed.Text[len(w.ed.Text)/2:], '\n')+len(w.ed.Text)/2+1, false)
	w.ed.Backspace(false)
	layout(160)
	check("joining two lines")
	w.ed.SelectAll()
	w.ed.Replace(editorDoc(20))
	layout(160)
	check("replacing the text")
	layout(97)
	check("narrowing")
	w.Style(TextStyle{Size: 23})
	layout(97)
	check("a new size")
	for i := range 300 { // edits leave lines no wrap uses, which go
		edit("typing", func() int { return 0 }, string(rune('a'+i%26)))
	}
	if n, lines := len(w.lines.byText), strings.Count(w.ed.Text, "\n")+1; n > 2*lines+64 {
		t.Errorf("%d lines kept for a text of %d", n, lines)
	}
}

// Edits anywhere, of any size, with and without line breaks, wrap as the
// whole text afresh would, wrapped to a width or at line breaks only; so
// does the text with a composition in it, asked for in turn with the text.
func TestTextInputWrapsRandomEditsAsTheWholeText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	rng := rand.New(rand.NewPCG(1, 2))
	pieces := []string{"", "x", " ", "\n", "\n\n", "two words", "a line\nand another ", "한글 e\u0301", "שלום", strings.Repeat("long ", 12)}
	for _, noWrap := range []bool{false, true} {
		w := TextInput(State(editorDoc(25))).Multiline()
		if noWrap {
			w.NoWrap()
		}
		w.Layout(Loose(Sz(140, Unbounded)), Env{})
		want := func(s string) []textlayout.Span {
			if noWrap {
				return textlayout.BreakSpans(s)
			}
			return textlayout.WrapSpans(s, w.face(1), w.width)
		}
		for i := range 400 {
			lo := w.ed.Snap(rng.IntN(len(w.ed.Text) + 1))
			hi := w.ed.Snap(min(lo+rng.IntN(40), len(w.ed.Text)))
			w.ed.Anchor, w.ed.Caret = lo, hi
			w.ed.Replace(pieces[rng.IntN(len(pieces))])
			if got := w.spans(w.ed.Text); !slices.Equal(got, want(w.ed.Text)) {
				t.Fatalf("noWrap %v, edit %d: %q wraps as %v, want %v", noWrap, i, w.ed.Text, got, want(w.ed.Text))
			}
			if i%10 == 0 {
				w.composition = "조합"
				shown, _ := w.rendered()
				if got := w.spans(shown); !slices.Equal(got, want(shown)) {
					t.Fatalf("noWrap %v, edit %d: composed %q wraps as %v, want %v", noWrap, i, shown, got, want(shown))
				}
				w.composition = ""
			}
		}
	}
}

// While the IME composes, paint wraps the text with the composition in
// it and the rest the text without: each is wrapped once, as the whole
// text would wrap, however often the two are asked for in turn.
func TestTextInputComposingWrapsEachTextOnce(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State(editorDoc(30))).Multiline()
	w.Layout(Loose(Sz(160, Unbounded)), Env{})
	w.ed.MoveTo(len(w.ed.Text)/2, false)
	w.composition, w.compCaret = "한글 조합 중인 긴 글자", len("한글")
	shown, _ := w.rendered()
	plain, composed := w.spans(w.ed.Text), w.spans(shown)
	if want := textlayout.WrapSpans(shown, w.face(1), w.width); !slices.Equal(composed, want) {
		t.Fatalf("the composed text wraps as %v, want %v", composed, want)
	}
	for range 3 {
		if a, b := w.spans(w.ed.Text), w.spans(shown); &a[0] != &plain[0] || &b[0] != &composed[0] {
			t.Fatal("asking for the text and the composed text in turn wrapped them again")
		}
	}
}

// A keystroke and the frame after it in a multiline editor of 300 lines:
// the line typed in is wrapped again, not the text. A sentence is typed
// into the middle and the text put back, so the line stays a line's
// length however long the benchmark runs.
func BenchmarkTextInputKeystrokeFrame(b *testing.B) {
	doc := editorDoc(300)
	w := TextInput(State(doc)).Multiline()
	var in inputState
	paintFrame(&in, w, Sz(300, 400))
	const typed = "a few words typed "
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i%len(typed) == 0 {
			w.ed.SetText(doc)
			w.ed.MoveTo(len(doc)/2, false)
		}
		w.ed.Replace(typed[i%len(typed) : i%len(typed)+1])
		i++
		paintFrame(&in, w, Sz(300, 400))
	}
}

// The same in a text of 10,000 lines: what a keystroke costs beside the
// line it changed, finding which lines those are and keeping the rest.
func BenchmarkTextInputKeystrokeFrameLongText(b *testing.B) {
	doc := editorDoc(10000)
	w := TextInput(State(doc)).Multiline()
	var in inputState
	paintFrame(&in, w, Sz(300, 400))
	const typed = "a few words typed "
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i%len(typed) == 0 {
			w.ed.SetText(doc)
			w.ed.MoveTo(len(doc)/2, false)
		}
		w.ed.Replace(typed[i%len(typed) : i%len(typed)+1])
		i++
		paintFrame(&in, w, Sz(300, 400))
	}
}

// A frame of the same editor while the IME composes and nothing changes.
func BenchmarkTextInputComposingFrame(b *testing.B) {
	w := TextInput(State(editorDoc(300))).Multiline()
	var in inputState
	paintFrame(&in, w, Sz(300, 400))
	w.ed.MoveTo(len(w.ed.Text)/2, false)
	w.composition = "ㅎ"
	b.ReportAllocs()
	for b.Loop() {
		paintFrame(&in, w, Sz(300, 400))
	}
}

// What the screen reader is told is measured again only when the text,
// its wrap or its style change: a scroll or a move only moves it.
func TestTextInputRunsFollowTheTextAndThePaint(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State(editorDoc(12))).Multiline()
	var in inputState
	paintFrame(&in, w, Sz(160, 80))
	same := func(what string) {
		t.Helper()
		got, want := w.runs(), w.measureRuns()
		for i := range want {
			want[i].Rect.Origin = want[i].Rect.Origin.Add(w.rect.Origin).Sub(Pt(0, w.scroll))
		}
		if !slices.EqualFunc(got, want, func(a, b TextRun) bool {
			return a.Start == b.Start && a.End == b.End && a.Rect == b.Rect && slices.Equal(a.Stops, b.Stops)
		}) {
			t.Fatalf("after %s, the runs are %v, want %v", what, got, want)
		}
	}
	measured := func() *TextRun { w.runs(); return &w.described.measured[0] }
	same("the first paint")
	if a, b := measured(), measured(); a != b {
		t.Fatal("the runs of an unchanged editor were measured again")
	}
	w.ed.MoveTo(0, false)
	w.ed.Replace("new words ")
	same("an edit")
	before := measured()
	w.scroll += 7
	same("a scroll")
	if measured() != before {
		t.Fatal("a scroll measured the runs again")
	}
	paintFrame(&in, w, Sz(120, 80))
	same("a narrower paint")
}

// Right-to-left text draws right to left, and the caret, a click and the
// selection go where it is drawn: the caret before the first letter of a
// Hebrew word stands at its right, a click at its right puts the caret
// there, and a click at its left after the word. Each holds wherever the
// machine's fonts draw Hebrew, since the order is the characters'.
func TestTextInputPlacesTheCaretInRightToLeftText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	for _, multiline := range []bool{false, true} {
		w := TextInput(State("שלום עולם")).Style(TextStyle{Size: 30})
		if multiline {
			w.Multiline()
		}
		var in inputState
		paintFrame(&in, w, Sz(600, 50))
		bl := w.bidiAt(w.wrap(w.ed.Text), 0)
		if bl == nil {
			t.Fatal("a Hebrew line was not laid out in visual order")
		}
		total := bl.width()
		caretAt := func(b int) float64 {
			w.ed.MoveTo(b, false)
			paintFrame(&in, w, Sz(600, 50))
			return float64(w.caretBounds.Min.X)
		}
		if x := caretAt(0); x < total-2 {
			t.Errorf("multiline %v: the caret at the start stands at %v, want the right end, %v", multiline, x, total)
		}
		if a, b := caretAt(len("ש")), caretAt(len("של")); !(b < a && a < total) {
			t.Errorf("multiline %v: the caret moves right as it goes through the word: %v then %v", multiline, a, b)
		}
		if i := w.indexAt(Pt(total-1, 10)); i != 0 {
			t.Errorf("multiline %v: a click at the right end puts the caret at %d, want 0", multiline, i)
		}
		if i := w.indexAt(Pt(1, 10)); i != len(w.ed.Text) {
			t.Errorf("multiline %v: a click at the left end puts the caret at %d, want %d", multiline, i, len(w.ed.Text))
		}
		// The first word selected is the right of the line.
		var xs []float64
		w.spanX(w.ed.Text, textlayout.Span{End: len(w.ed.Text)}, bl, 0, len("שלום"), 0, func(a, b float64) { xs = append(xs, a, b) })
		if len(xs) != 2 || xs[0] < total/3 || xs[1] < total-2 {
			t.Errorf("multiline %v: the first word selected covers %v of %v, want its right end", multiline, xs, total)
		}
	}
}

// An OnDescribe callback amends the node without being handed the runs
// kept for the next description, which it could otherwise change; runs it
// sets itself are the node's.
func TestTextInputDescribeKeepsItsRunsFromTheCallback(t *testing.T) {
	useFakeIME(t)
	a11y.SetWantsDetail(true)
	t.Cleanup(func() { a11y.SetWantsDetail(false) })
	saw := -1
	var own []TextRun
	w := TextInput(State("two words")).OnDescribe(func(n *Node) {
		saw = len(n.Runs)
		if own != nil {
			n.Runs = own
		}
	})
	var in inputState
	paintFrame(&in, w, Sz(200, 30))
	n := w.Describe()
	if saw != 0 || len(n.Runs) == 0 || &n.Runs[0] != &w.runs()[0] {
		t.Fatalf("the callback saw %d runs; the node has %d, kept: %v", saw, len(n.Runs), len(n.Runs) > 0 && &n.Runs[0] == &w.runs()[0])
	}
	own = []TextRun{{Start: 0, End: 3}}
	if n := w.Describe(); len(n.Runs) != 1 || n.Runs[0].End != 3 {
		t.Fatalf("the callback's own runs became %v", n.Runs)
	}
}

// Typing in a long line keeps no copy of the line per keystroke: a
// single-line field keeps none of its text, and a long paragraph's copies
// go once they add up to more than the text twice over.
func TestTextInputKeepsNoCopyOfALongLinePerKeystroke(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	long := strings.Repeat("a long line of words ", 500)
	for _, multiline := range []bool{false, true} {
		w := TextInput(State(long))
		if multiline {
			w.Multiline()
		}
		w.Layout(Loose(Sz(300, Unbounded)), Env{})
		w.ed.MoveTo(len(long)/2, false)
		for range 300 {
			w.ed.Replace("x")
			w.spans(w.ed.Text)
		}
		if c := &w.lines; c.bytes > 2*len(w.ed.Text)+4096 || !multiline && len(c.byText) != 0 {
			t.Errorf("multiline %v: %d lines of %d bytes kept for a text of %d", multiline, len(c.byText), c.bytes, len(w.ed.Text))
		}
	}
}

// A frame of a 300-line editor with a screen reader attached, scrolled a
// little each time: the runs move, measured once.
func BenchmarkTextInputScrollFrameWithScreenReader(b *testing.B) {
	a11y.SetWantsDetail(true)
	b.Cleanup(func() { a11y.SetWantsDetail(false) })
	w := TextInput(State(editorDoc(300))).Multiline()
	var in inputState
	paintFrame(&in, w, Sz(300, 400))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		w.scroll = float64(i % 500)
		i++
		w.Describe()
	}
}

// A keystroke and the frame after it in a single-line field holding a
// long text.
func BenchmarkTextInputKeystrokeFrameOneLine(b *testing.B) {
	doc := strings.Repeat("a long line of words ", 500)
	w := TextInput(State(doc))
	var in inputState
	paintFrame(&in, w, Sz(300, 30))
	const typed = "a few words typed "
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i%len(typed) == 0 {
			w.ed.SetText(doc)
			w.ed.MoveTo(len(doc)/2, false)
		}
		w.ed.Replace(typed[i%len(typed) : i%len(typed)+1])
		i++
		paintFrame(&in, w, Sz(300, 30))
	}
}
