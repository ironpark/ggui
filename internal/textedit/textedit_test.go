package textedit

import (
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestEditorMovesByRuneAndWord(t *testing.T) {
	var e Editor
	e.SetText("héllo wörld  foo")
	e.MoveTo(len(e.Text), false)
	e.MoveBy(-1, true, false)
	if e.Text[e.Caret:] != "foo" {
		t.Fatalf("word left landed before %q, want \"foo\"", e.Text[e.Caret:])
	}
	e.MoveBy(-1, true, false)
	if e.Text[e.Caret:] != "wörld  foo" {
		t.Fatalf("word left landed before %q, want \"wörld  foo\"", e.Text[e.Caret:])
	}
	e.MoveBy(1, false, false)
	e.MoveBy(1, false, false)
	if e.Text[e.Caret:] != "rld  foo" {
		t.Fatalf("two runes right landed before %q; multibyte rune split?", e.Text[e.Caret:])
	}
	e.MoveBy(1, true, false)
	if e.Text[e.Caret:] != "foo" {
		t.Fatalf("word right landed before %q, want \"foo\" (spaces skipped)", e.Text[e.Caret:])
	}
}

func TestEditorSelectionCollapsesTowardMovement(t *testing.T) {
	var e Editor
	e.SetText("abcdef")
	e.MoveTo(1, false)
	e.MoveTo(4, true)
	if lo, hi := e.Selection(); lo != 1 || hi != 4 || e.Selected() != "bcd" {
		t.Fatalf("selection = [%d,%d) %q, want [1,4) \"bcd\"", lo, hi, e.Selected())
	}
	e.MoveBy(-1, false, false)
	if e.Caret != 1 || e.HasSelection() {
		t.Fatalf("left collapsed to %d with selection %v, want 1 and none", e.Caret, e.HasSelection())
	}
	e.MoveTo(4, true)
	e.MoveBy(1, false, false)
	if e.Caret != 4 {
		t.Fatalf("right collapsed to %d, want 4", e.Caret)
	}
}

func TestEditorBackspaceDeleteAndReplace(t *testing.T) {
	var e Editor
	e.SetText("한글 입력")
	e.MoveTo(len(e.Text), false)
	e.Backspace(false)
	if e.Text != "한글 입" {
		t.Fatalf("backspace removed a byte, not a rune: %q", e.Text)
	}
	e.Backspace(true)
	if e.Text != "한글 " {
		t.Fatalf("word backspace = %q, want \"한글 \"", e.Text)
	}
	e.MoveTo(0, false)
	e.DeleteForward(false)
	if e.Text != "글 " {
		t.Fatalf("delete = %q, want \"글 \"", e.Text)
	}
	e.SelectAll()
	e.Replace("x")
	if e.Text != "x" || e.Caret != 1 || e.HasSelection() {
		t.Fatalf("replace all = %q caret %d", e.Text, e.Caret)
	}
}

func TestEditorSelectLine(t *testing.T) {
	var e Editor
	e.SetText("one\ntwo three\n\nfour")
	for _, c := range []struct {
		at   int
		want string
	}{{1, "one"}, {3, "one"}, {6, "two three"}, {14, ""}, {len(e.Text), "four"}} {
		e.SelectLine(c.at)
		if got := e.Selected(); got != c.want {
			t.Errorf("SelectLine(%d) selected %q, want %q", c.at, got, c.want)
		}
	}
}

func TestEditorSelectWord(t *testing.T) {
	var e Editor
	e.SetText("foo bar.baz")
	e.SelectWord(5)
	if e.Selected() != "bar" {
		t.Fatalf("selectWord(5) = %q, want \"bar\"", e.Selected())
	}
	e.SelectWord(len(e.Text))
	if e.Selected() != "baz" {
		t.Fatalf("selectWord at end = %q, want \"baz\"", e.Selected())
	}
	e.SetText("")
	e.SelectWord(0)
}

func TestCaretStepsByGrapheme(t *testing.T) {
	var e Editor
	// e + combining acute, a family emoji joined with ZWJ, a flag pair, CRLF.
	e.SetText("é\U0001F468\u200d\U0001F469\U0001F1F0\U0001F1F7\r\nx")
	e.MoveTo(0, false)
	steps := []int{}
	for e.Caret < len(e.Text) {
		e.MoveBy(1, false, false)
		steps = append(steps, e.Caret)
	}
	if len(steps) != 5 {
		t.Fatalf("%d steps over the text, want 5 clusters: %v", len(steps), steps)
	}
	e.MoveTo(len(e.Text), false)
	e.Backspace(false)
	e.Backspace(false)
	if e.Text != "é\U0001F468\u200d\U0001F469\U0001F1F0\U0001F1F7" {
		t.Fatalf("after two backspaces: %q", e.Text)
	}
	e.Backspace(false)
	if e.Text != "é\U0001F468\u200d\U0001F469" {
		t.Fatalf("backspace over the flag pair: %q", e.Text)
	}
	e.MoveTo(0, false)
	e.MoveBy(1, false, false)
	if e.Caret != len("é") {
		t.Fatalf("caret %d after one step, want past the combining mark", e.Caret)
	}
}

func TestUndoRedo(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := Editor{Now: func() time.Time { return now }}
	e.Replace("a")
	e.Replace("b")
	e.Replace("c") // one word typed quickly: one step
	now = now.Add(time.Second)
	e.Replace(" ")
	now = now.Add(time.Second)
	e.Replace("d")
	if e.Text != "abc d" {
		t.Fatalf("text %q", e.Text)
	}
	e.Undo()
	if e.Text != "abc " {
		t.Fatalf("after one undo %q, want the last letter gone", e.Text)
	}
	e.Undo()
	e.Undo()
	if e.Text != "" || e.Caret != 0 {
		t.Fatalf("after three undos %q caret %d, want empty", e.Text, e.Caret)
	}
	if e.Undo() {
		t.Fatal("undo with nothing to undo reported true")
	}
	e.Redo()
	if e.Text != "abc" {
		t.Fatalf("after redo %q, want abc", e.Text)
	}
	e.Replace("!")
	if e.Redo() {
		t.Fatal("an edit did not clear the redo stack")
	}
}

func TestEditorSnapsOffsetsToRuneStarts(t *testing.T) {
	var e Editor
	e.SetText("aé")
	e.Caret, e.Anchor = 2, 2 // inside é
	e.SetText("aéb")
	if e.Caret != 1 {
		t.Fatalf("caret snapped to %d, want 1", e.Caret)
	}
}

func TestUndoKeepsOnlyTheLatestSteps(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := Editor{Now: func() time.Time { return now }}
	for range maxUndo + 5 {
		now = now.Add(time.Second) // each insertion a step of its own
		e.Replace("x")
	}
	undone := 0
	for e.Undo() {
		undone++
	}
	if undone != maxUndo {
		t.Fatalf("undid %d steps, want the history capped at %d", undone, maxUndo)
	}
	if want := 5; len(e.Text) != want {
		t.Fatalf("after undoing all %d kept steps the text is %q, want the %d oldest insertions left", maxUndo, e.Text, want)
	}
}

func TestPasteAndDeletionUndoAlone(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := Editor{Now: func() time.Time { return now }}
	e.Replace("a")
	e.Replace("bc") // a paste, even right after typing
	e.Replace("d")
	e.Backspace(false)
	e.Undo()
	if e.Text != "abcd" {
		t.Fatalf("undoing the backspace gave %q, want \"abcd\"", e.Text)
	}
	e.Undo()
	if e.Text != "abc" {
		t.Fatalf("undoing the typing after a paste gave %q, want \"abc\"", e.Text)
	}
	e.Undo()
	if e.Text != "a" {
		t.Fatalf("undoing the paste gave %q, want \"a\"", e.Text)
	}
}

func TestWordMoveWithSelectionMovesFromCaret(t *testing.T) {
	t.Parallel()
	var e Editor
	e.SetText("one two three")
	e.MoveTo(4, false)
	e.MoveTo(6, true) // "tw" selected, caret after it
	e.MoveBy(1, true, false)
	if e.Caret != len("one two ") || e.HasSelection() {
		t.Fatalf("word right from a selection put the caret at %d (selection %v), want %d past the word the caret is in",
			e.Caret, e.HasSelection(), len("one two "))
	}
	e.MoveBy(-1, true, true)
	if lo, hi := e.Selection(); lo != 4 || hi != 8 {
		t.Fatalf("word left extending selected [%d,%d), want [4,8)", lo, hi)
	}
}

func TestBackspaceAndDeleteAtTextEdgesKeepText(t *testing.T) {
	t.Parallel()
	var e Editor
	e.SetText("ab")
	e.MoveTo(0, false)
	e.Backspace(false)
	e.Backspace(true)
	e.MoveTo(2, false)
	e.DeleteForward(false)
	e.DeleteForward(true)
	if e.Text != "ab" || e.Caret != 2 {
		t.Fatalf("deleting past the edges gave %q caret %d, want \"ab\" caret 2", e.Text, e.Caret)
	}
}

func TestUndoKeepsOnlyWhatEachStepChanged(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := Editor{Now: func() time.Time { return now }}
	e.SetText(strings.Repeat("x", 1<<20))
	for range 100 {
		e.Replace("y")
		now = now.Add(time.Second)
	}
	held := 0
	for _, d := range e.undo {
		held += len(d.text)
	}
	if held > 100 {
		t.Fatalf("the undo history holds %d bytes of text for 100 one-byte steps", held)
	}
	for range 100 {
		if !e.Undo() {
			t.Fatal("ran out of undo")
		}
	}
	if e.Text != strings.Repeat("x", 1<<20) {
		t.Fatal("undoing every step did not restore the text")
	}
}

// CommonPrefix and CommonSuffix compare a block at a time and then byte by
// byte: what they find must be where the texts first differ, on either
// side of a block's edge.
func TestCommonPrefixAndSuffixFindTheFirstDifference(t *testing.T) {
	t.Parallel()
	base := strings.Repeat("abcdefgh", 40)
	for _, at := range []int{0, 1, 63, 64, 65, 128, 200, len(base) - 1} {
		other := base[:at] + "#" + base[at+1:]
		if got := CommonPrefix(base, other); got != at {
			t.Errorf("CommonPrefix differing at %d = %d", at, got)
		}
		if got, want := CommonSuffix(base, other), len(base)-at-1; got != want {
			t.Errorf("CommonSuffix differing at %d = %d, want %d", at, got, want)
		}
	}
	if CommonPrefix(base, base[:100]) != 100 || CommonSuffix(base, base[100:]) != len(base)-100 || CommonPrefix("", base) != 0 {
		t.Error("a text and a part of it share that part")
	}
}

// Changed tells where the last Replace changed the text, for those very
// strings only: another text, even an equal one, is the caller's to
// compare.
func TestChangedReportsTheLastReplace(t *testing.T) {
	t.Parallel()
	var e Editor
	e.SetText("hello world")
	e.Anchor, e.Caret = 6, 11
	before := e.Text
	from := e.Rev(before)
	if from == 0 || e.Rev(before) != from {
		t.Fatalf("Rev of the editor's text = %d, then %d", from, e.Rev(before))
	}
	if e.Rev(strings.Clone(before)) != 0 {
		t.Error("an equal copy of the text was given the text's name")
	}
	e.Replace("there!")
	at, cut, ins, ok := e.Changed(from, e.Text)
	if !ok || at != 6 || cut != 5 || ins != 6 {
		t.Fatalf("Changed = %d, %d, %d, %v; want 6, 5, 6, true", at, cut, ins, ok)
	}
	if _, _, _, ok := e.Changed(from, strings.Clone(e.Text)); ok {
		t.Error("an equal copy of the text after was taken for it")
	}
	if _, _, _, ok := e.Changed(e.Rev(e.Text), e.Text); ok {
		t.Error("the text after was taken for the text before")
	}
	after := e.Text
	e.SetText("other")
	if _, _, _, ok := e.Changed(from, e.Text); ok {
		t.Error("a text SetText made was taken for the last Replace's")
	}
	// Text is a field: one assigned outside the editor, even back to the
	// text Replace made, is not taken for it once something else has been.
	e.Text = after
	if _, _, _, ok := e.Changed(from, e.Text); ok {
		t.Error("a text assigned after SetText was taken for the last Replace's")
	}
}

// A text the editor assigned outside its methods gets a name of its own,
// so a Replace from it is not taken for one from the text before.
func TestChangedSeesTextAssignedDirectly(t *testing.T) {
	t.Parallel()
	var e Editor
	e.SetText("one two")
	stale := e.Rev(e.Text)
	e.Text = "three four"
	e.MoveTo(len(e.Text), false)
	e.Replace("!")
	if _, _, _, ok := e.Changed(stale, e.Text); ok {
		t.Error("a Replace from an assigned text was taken for one from the text before it")
	}
}

// The text before the last Replace is not kept by the editor: once the
// undo history and the caller let go of it, it is freed. The undo history
// keeps the start of a typed run, so the text between two keystrokes of
// one run is held by nothing else.
func TestReplaceKeepsNoTextBefore(t *testing.T) {
	now := time.Unix(0, 0)
	e := Editor{Now: func() time.Time { return now }}
	e.SetText(strings.Repeat("x", 1<<16))
	e.MoveTo(100, false)
	e.Replace("a")
	freed := make(chan struct{})
	runtime.AddCleanup(unsafe.StringData(e.Text), func(ch chan struct{}) { close(ch) }, freed)
	e.Replace("b")
	for range 20 {
		runtime.GC()
		select {
		case <-freed:
			if e.Undo(); e.Text != strings.Repeat("x", 1<<16) {
				t.Fatal("undo lost the text before the typed run")
			}
			return
		default:
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the text before the last Replace is still alive")
}
