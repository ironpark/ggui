package ggui

import (
	"testing"
	"time"
)

func TestEditorMovesByRuneAndWord(t *testing.T) {
	var e textEditor
	e.setText("héllo wörld  foo")
	e.moveTo(len(e.text), false)
	e.moveBy(-1, true, false)
	if e.text[e.caret:] != "foo" {
		t.Fatalf("word left landed before %q, want \"foo\"", e.text[e.caret:])
	}
	e.moveBy(-1, true, false)
	if e.text[e.caret:] != "wörld  foo" {
		t.Fatalf("word left landed before %q, want \"wörld  foo\"", e.text[e.caret:])
	}
	e.moveBy(1, false, false)
	e.moveBy(1, false, false)
	if e.text[e.caret:] != "rld  foo" {
		t.Fatalf("two runes right landed before %q; multibyte rune split?", e.text[e.caret:])
	}
	e.moveBy(1, true, false)
	if e.text[e.caret:] != "foo" {
		t.Fatalf("word right landed before %q, want \"foo\" (spaces skipped)", e.text[e.caret:])
	}
}

func TestEditorSelectionCollapsesTowardMovement(t *testing.T) {
	var e textEditor
	e.setText("abcdef")
	e.moveTo(1, false)
	e.moveTo(4, true)
	if lo, hi := e.selection(); lo != 1 || hi != 4 || e.selected() != "bcd" {
		t.Fatalf("selection = [%d,%d) %q, want [1,4) \"bcd\"", lo, hi, e.selected())
	}
	e.moveBy(-1, false, false)
	if e.caret != 1 || e.hasSelection() {
		t.Fatalf("left collapsed to %d with selection %v, want 1 and none", e.caret, e.hasSelection())
	}
	e.moveTo(4, true)
	e.moveBy(1, false, false)
	if e.caret != 4 {
		t.Fatalf("right collapsed to %d, want 4", e.caret)
	}
}

func TestEditorBackspaceDeleteAndReplace(t *testing.T) {
	var e textEditor
	e.setText("한글 입력")
	e.moveTo(len(e.text), false)
	e.backspace(false)
	if e.text != "한글 입" {
		t.Fatalf("backspace removed a byte, not a rune: %q", e.text)
	}
	e.backspace(true)
	if e.text != "한글 " {
		t.Fatalf("word backspace = %q, want \"한글 \"", e.text)
	}
	e.moveTo(0, false)
	e.deleteForward(false)
	if e.text != "글 " {
		t.Fatalf("delete = %q, want \"글 \"", e.text)
	}
	e.selectAll()
	e.replace("x")
	if e.text != "x" || e.caret != 1 || e.hasSelection() {
		t.Fatalf("replace all = %q caret %d", e.text, e.caret)
	}
}

func TestEditorSelectWord(t *testing.T) {
	var e textEditor
	e.setText("foo bar.baz")
	e.selectWord(5)
	if e.selected() != "bar" {
		t.Fatalf("selectWord(5) = %q, want \"bar\"", e.selected())
	}
	e.selectWord(len(e.text))
	if e.selected() != "baz" {
		t.Fatalf("selectWord at end = %q, want \"baz\"", e.selected())
	}
	e.setText("")
	e.selectWord(0)
}

func TestEditorSnapsOffsetsToRuneStarts(t *testing.T) {
	var e textEditor
	e.setText("aé")
	e.caret, e.anchor = 2, 2 // inside é
	e.setText("aéb")
	if e.caret != 1 {
		t.Fatalf("caret snapped to %d, want 1", e.caret)
	}
}

// fakeIME stands in for the platform IME: a test sets its composition or
// commit and the next tick delivers it.
type fakeIME struct {
	t         *TextInputWidget
	compose   string
	commit    string
	confirmed int
	cancelled int
	handled   bool
}

func (f *fakeIME) Update() (bool, error) {
	if f.commit != "" {
		f.t.imeSession()
		f.t.imeCommit(f.commit)
		f.commit = ""
		f.t.imeComposition("", 0)
		return true, nil
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

func (f *fakeIME) Cancel() {
	f.cancelled++
	f.compose = ""
	f.t.imeComposition("", 0)
}

func useFakeIME(t *testing.T) {
	t.Helper()
	prev := newIME
	newIME = func(w *TextInputWidget) ime { return &fakeIME{t: w} }
	t.Cleanup(func() { newIME = prev })
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
	value := State("hello")
	changes := 0
	w, in := focusedInput(t, value)
	w.OnChange(func(string) { changes++ })
	if w.ed.caret != len("hello") {
		t.Fatalf("click past the end put the caret at %d, want %d", w.ed.caret, len("hello"))
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
	value := State("abc")
	w, in := focusedInput(t, value)
	value.Set("")
	paintFrame(in, w, Sz(200, 20))
	if w.ed.text != "" || w.ed.caret != 0 {
		t.Fatalf("editor text %q caret %d after the signal was cleared", w.ed.text, w.ed.caret)
	}
}

func TestTextInputSubmitAndClipboard(t *testing.T) {
	prev := currentClipboard()
	defer SetClipboard(prev)
	clip := &MemoryClipboard{}
	SetClipboard(clip)

	value := State("copy me")
	var submitted []string
	w, in := focusedInput(t, value)
	w.OnSubmit(func(s string) { submitted = append(submitted, s) })
	cmd := Mods{Meta: true, Ctrl: true}
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
	value := State("hello world")
	w, in := focusedInput(t, value)
	// Drag from the left edge to the far right selects everything.
	in.dispatch(frameInput{pos: Pt(0, 10), down: []MouseButton{MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(199, 10)})
	in.dispatch(frameInput{pos: Pt(199, 10), up: []MouseButton{MouseButtonLeft}})
	if w.ed.selected() != "hello world" {
		t.Fatalf("drag selected %q, want everything", w.ed.selected())
	}
	click := func(p Point) {
		in.dispatch(frameInput{pos: p, down: []MouseButton{MouseButtonLeft}})
		in.dispatch(frameInput{pos: p, up: []MouseButton{MouseButtonLeft}})
	}
	x := w.advance("hello wo")
	click(Pt(x, 10))
	click(Pt(x, 10))
	if w.ed.selected() != "world" {
		t.Fatalf("double-click selected %q, want \"world\"", w.ed.selected())
	}
}

func TestTextInputComposesThenCommits(t *testing.T) {
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
	if Untrack(value.Get) != "a한b" || w.composition != "" || w.ed.caret != 1+len("한") {
		t.Fatalf("value = %q composition = %q caret %d after commit", Untrack(value.Get), w.composition, w.ed.caret)
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
	useFakeIME(t)
	w := TextInput(State("")).Multiline()
	one := w.Layout(Loose(Sz(200, Unbounded)), Env{})
	if one.H != w.linesHeight(3) || one.W != 200 {
		t.Fatalf("empty multiline is %v, want 3 lines tall and 200 wide", one)
	}
	long := "the quick brown fox jumps over the lazy dog and keeps on running far away"
	w.value.Set(long)
	effects.flush() // the editor follows its binding through an effect
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
	if !w.focused || w.ed.caret != 0 {
		t.Fatalf("click at the top left: focused %v caret %d", w.focused, w.ed.caret)
	}
	typeKeys(&in, Mods{}, KeyEnd)
	if w.ed.caret != spans[0].end {
		t.Fatalf("End went to %d, want the end of the first line %d", w.ed.caret, spans[0].end)
	}
	typeKeys(&in, Mods{}, KeyArrowDown)
	if got := lineOf(spans, w.ed.caret); got != 1 {
		t.Fatalf("Down landed on line %d", got)
	}
	typeKeys(&in, Mods{}, KeyHome)
	if w.ed.caret != spans[1].start {
		t.Fatalf("Home on line 2 went to %d, want %d", w.ed.caret, spans[1].start)
	}
	typeKeys(&in, Mods{}, KeyEnter)
	if got := Untrack(w.value.Get); got[spans[1].start] != '\n' {
		t.Fatalf("Enter did not insert a line break: %q", got)
	}
	submitted := ""
	w.OnSubmit(func(s string) { submitted = s })
	typeKeys(&in, Mods{Meta: true, Ctrl: true}, KeyEnter)
	if submitted == "" {
		t.Fatal("Cmd+Enter did not submit")
	}
	// A click on the second line lands there.
	paintFrame(&in, w, Sz(200, grown.H))
	in.dispatch(frameInput{pos: Pt(5, w.spacing()+2), down: []MouseButton{MouseButtonLeft}})
	if got := lineOf(w.spans(w.ed.text), w.ed.caret); got != 1 {
		t.Fatalf("click on the second line put the caret on line %d", got)
	}
}

func TestCaretStepsByGrapheme(t *testing.T) {
	var e textEditor
	// e + combining acute, a family emoji joined with ZWJ, a flag pair, CRLF.
	e.setText("é\U0001F468‍\U0001F469\U0001F1F0\U0001F1F7\r\nx")
	e.moveTo(0, false)
	steps := []int{}
	for e.caret < len(e.text) {
		e.moveBy(1, false, false)
		steps = append(steps, e.caret)
	}
	if len(steps) != 5 {
		t.Fatalf("%d steps over the text, want 5 clusters: %v", len(steps), steps)
	}
	e.moveTo(len(e.text), false)
	e.backspace(false)
	e.backspace(false)
	if e.text != "é\U0001F468‍\U0001F469\U0001F1F0\U0001F1F7" {
		t.Fatalf("after two backspaces: %q", e.text)
	}
	e.backspace(false)
	if e.text != "é\U0001F468‍\U0001F469" {
		t.Fatalf("backspace over the flag pair: %q", e.text)
	}
	e.moveTo(0, false)
	e.moveBy(1, false, false)
	if e.caret != len("é") {
		t.Fatalf("caret %d after one step, want past the combining mark", e.caret)
	}
}

func TestUndoRedo(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	defer SetClock(func() time.Time { return now })()
	var e textEditor
	e.replace("a")
	e.replace("b")
	e.replace("c") // one word typed quickly: one step
	now = now.Add(time.Second)
	e.replace(" ")
	now = now.Add(time.Second)
	e.replace("d")
	if e.text != "abc d" {
		t.Fatalf("text %q", e.text)
	}
	e.Undo()
	if e.text != "abc " {
		t.Fatalf("after one undo %q, want the last letter gone", e.text)
	}
	e.Undo()
	e.Undo()
	if e.text != "" || e.caret != 0 {
		t.Fatalf("after three undos %q caret %d, want empty", e.text, e.caret)
	}
	if e.Undo() {
		t.Fatal("undo with nothing to undo reported true")
	}
	e.Redo()
	if e.text != "abc" {
		t.Fatalf("after redo %q, want abc", e.text)
	}
	e.replace("!")
	if e.Redo() {
		t.Fatal("an edit did not clear the redo stack")
	}
}

func TestUndoKeys(t *testing.T) {
	v := State("hello")
	in := TextInput(v)
	p := NewProbe(in, Sz(200, 30))
	defer p.Close()
	p.Click(Pt(190, 10))
	p.Type(Mods{}, KeyBackspace, KeyBackspace)
	if Untrack(v.Get) != "hel" {
		t.Fatalf("value %q after two backspaces", Untrack(v.Get))
	}
	cmd := Mods{Meta: true}
	if !runtimeIsDarwin() {
		cmd = Mods{Ctrl: true}
	}
	p.Type(cmd, KeyZ)
	if Untrack(v.Get) != "hell" {
		t.Fatalf("value %q after undo, want hell", Untrack(v.Get))
	}
	p.Type(Mods{Shift: cmd.Shift || true, Meta: cmd.Meta, Ctrl: cmd.Ctrl}, KeyZ)
	if Untrack(v.Get) != "hel" {
		t.Fatalf("value %q after redo, want hel", Untrack(v.Get))
	}
}

func TestTextInputKeyHookLeavesCompositionWithIME(t *testing.T) {
	old := newIME
	newIME = func(w *TextInputWidget) ime { return &fakeIME{t: w} }
	defer func() { newIME = old }()
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
