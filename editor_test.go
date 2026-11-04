package ggui

import (
	"image"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unsafe"

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
	if h3 <= h1 || w.wrapped.spans != nil {
		t.Fatalf("six lines %v tall against one's %v; line table made: %v", h3, h1, w.wrapped.spans != nil)
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
