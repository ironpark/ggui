package ggui

import (
	"testing"
	"time"
)

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
