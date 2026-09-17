package ggui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
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
func typeKeys(in *inputState, mods Mods, keys ...ebiten.Key) {
	in.dispatch(frameInput{keys: keys, mods: mods})
}

func focusedInput(t *testing.T, value *Signal[string]) (*TextInputWidget, *inputState) {
	t.Helper()
	useFakeIME(t)
	w := TextInput(value)
	var in inputState
	paintFrame(&in, w, Sz(200, 20))
	in.dispatch(frameInput{pos: Pt(190, 10), down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(190, 10), up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
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
	typeKeys(in, Mods{}, ebiten.KeyBackspace, ebiten.KeyBackspace)
	if value.Peek() != "hel" || changes != 2 {
		t.Fatalf("value = %q after two backspaces, changes = %d", value.Peek(), changes)
	}
	typeKeys(in, Mods{Shift: true}, ebiten.KeyHome)
	typeKeys(in, Mods{}, ebiten.KeyDelete)
	if value.Peek() != "" {
		t.Fatalf("shift+home then delete left %q", value.Peek())
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
	typeKeys(in, cmd, ebiten.KeyA)
	typeKeys(in, cmd, ebiten.KeyC)
	if clip.Read() != "copy me" {
		t.Fatalf("clipboard = %q after select all + copy", clip.Read())
	}
	typeKeys(in, cmd, ebiten.KeyX)
	if value.Peek() != "" {
		t.Fatalf("cut left %q", value.Peek())
	}
	clip.Write("line1\nline2")
	typeKeys(in, cmd, ebiten.KeyV)
	if value.Peek() != "line1 line2" {
		t.Fatalf("paste gave %q, want newlines folded to spaces", value.Peek())
	}
	typeKeys(in, Mods{}, ebiten.KeyEnter)
	if len(submitted) != 1 || submitted[0] != "line1 line2" {
		t.Fatalf("submitted = %v", submitted)
	}
}

func TestTextInputDragSelectsAndDoubleClickSelectsWord(t *testing.T) {
	value := State("hello world")
	w, in := focusedInput(t, value)
	// Drag from the left edge to the far right selects everything.
	in.dispatch(frameInput{pos: Pt(0, 10), down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	in.dispatch(frameInput{pos: Pt(199, 10)})
	in.dispatch(frameInput{pos: Pt(199, 10), up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
	if w.ed.selected() != "hello world" {
		t.Fatalf("drag selected %q, want everything", w.ed.selected())
	}
	click := func(p Point) {
		in.dispatch(frameInput{pos: p, down: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
		in.dispatch(frameInput{pos: p, up: []ebiten.MouseButton{ebiten.MouseButtonLeft}})
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
	typeKeys(in, Mods{}, ebiten.KeyArrowLeft)
	f := w.ime.(*fakeIME)
	f.compose = "ㅎ"
	in.dispatch(frameInput{keys: []ebiten.Key{ebiten.KeyBackspace}})
	// The IME swallowed the key: nothing was deleted.
	if value.Peek() != "ab" || w.composition != "ㅎ" {
		t.Fatalf("value = %q composition = %q; the IME should have swallowed the key", value.Peek(), w.composition)
	}
	if shown, caret := w.rendered(); shown != "aㅎb" || caret != 1+len("ㅎ") {
		t.Fatalf("rendered = %q caret %d", shown, caret)
	}
	f.compose, f.commit = "", "한"
	in.dispatch(frameInput{})
	if value.Peek() != "a한b" || w.composition != "" || w.ed.caret != 1+len("한") {
		t.Fatalf("value = %q composition = %q caret %d after commit", value.Peek(), w.composition, w.ed.caret)
	}
	// Moving the caret confirms whatever is being composed: the IME went
	// idle with "ㄱ" still shown, and End commits it before moving.
	f.compose = "ㄱ"
	in.dispatch(frameInput{})
	f.compose = ""
	typeKeys(in, Mods{}, ebiten.KeyEnd)
	if value.Peek() != "a한ㄱb" || f.confirmed == 0 {
		t.Fatalf("value = %q, confirmed %d times", value.Peek(), f.confirmed)
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
	typeKeys(&in, Mods{}, ebiten.KeyArrowLeft)
	second := build().(*TextInputWidget)
	paintFrame(&in, second, Sz(200, 20))
	typeKeys(&in, Mods{}, ebiten.KeyBackspace)
	if !second.Focused() || value.Peek() != "ac" {
		t.Fatalf("rebuilt input focused = %v, value = %q; want focus and caret carried over", second.Focused(), value.Peek())
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
