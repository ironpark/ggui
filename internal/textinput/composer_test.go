//go:build !js

package textinput_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/ironpark/ggui/internal/textinput"
)

// recorder is a Composer whose callbacks log what they receive. It asks for
// no session of its own: tests attach the harness's.
type recorder struct {
	textinput.Composer
	log     []string
	commits []*textinput.Commit
}

func newRecorder() *recorder {
	r := &recorder{}
	r.OnNewSession = func() *textinput.SessionOptions { return nil }
	r.OnComposition = func(c *textinput.Composition) {
		start, end := c.SelectionRangeInBytes()
		r.log = append(r.log, fmt.Sprintf("preedit %q [%d,%d)", c.Text(), start, end))
	}
	r.OnCommit = func(c *textinput.Commit) {
		r.log = append(r.log, "commit "+c.Text())
		r.commits = append(r.commits, c)
	}
	r.OnEndByUser = func() { r.log = append(r.log, "ended by user") }
	return r
}

// take returns the log so far and clears it.
func (r *recorder) take() []string {
	l := r.log
	r.log = nil
	return l
}

// update runs one tick and fails the test on an error.
func (r *recorder) update(t *testing.T) bool {
	t.Helper()
	handled, err := r.Update()
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	return handled
}

// expectLog fails unless the recorder logged exactly want since the last take.
func expectLog(t *testing.T, r *recorder, when string, want ...string) {
	t.Helper()
	if got := r.take(); !slices.Equal(got, want) {
		t.Fatalf("%s: callbacks %q, want %q", when, got, want)
	}
}

const noPreedit = `preedit "" [0,0)`

func TestComposerShowsPreeditThenCommits(t *testing.T) {
	t.Parallel()
	d, r := textinput.NewSessionAround("ab", "cd"), newRecorder()
	d.Attach(&r.Composer)

	d.Send(textinput.TextInputState{Text: "かな", CompositionSelectionStartInBytes: 3, CompositionSelectionEndInBytes: 6})
	if !r.update(t) {
		t.Error("a tick with a live preedit was not handled; the caller would act on the key too")
	}
	expectLog(t, r, "composing", `preedit "かな" [3,6)`)

	d.Send(commitOf("仮名"))
	if !r.update(t) {
		t.Error("a tick that committed was not handled")
	}
	expectLog(t, r, "committing", "commit 仮名", noPreedit)
	if textinput.HasSession(&r.Composer) {
		t.Error("the Composer kept the session its commit ended")
	}
	c := r.commits[0]
	if before, after := c.IsSurroundingTextReplaced(); before || after {
		t.Errorf("IsSurroundingTextReplaced() = %v, %v for a plain insertion, want false, false", before, after)
	}
	if before, after := c.SurroundingText(); before != "ab" || after != "cd" {
		t.Errorf("SurroundingText() = %q, %q, want the session's \"ab\", \"cd\"", before, after)
	}
	if c.HasPassthroughKey() {
		t.Error("HasPassthroughKey() = true for a regular commit")
	}

	// With no session and none offered, a tick does nothing.
	if r.update(t) {
		t.Error("a tick with no session was handled")
	}
	expectLog(t, r, "idle without a session")
}

func TestComposerLeavesPassthroughCommitUnhandled(t *testing.T) {
	t.Parallel()
	d, r := textinput.NewSession(), newRecorder()
	d.Attach(&r.Composer)
	d.Send(textinput.TextInputState{Text: "a", CommitKind: textinput.CommitWithPassthroughKey,
		ReplacementStartInBytes: textinput.NoReplacement, ReplacementEndInBytes: textinput.NoReplacement})
	if r.update(t) {
		t.Error("a commit whose key passes through was handled; the caller would miss the key")
	}
	expectLog(t, r, "passthrough commit", "commit a", noPreedit)
	if !r.commits[0].HasPassthroughKey() {
		t.Error("HasPassthroughKey() = false for a passthrough commit")
	}
}

func TestComposerHandlesTheTickThatEmptiesThePreedit(t *testing.T) {
	t.Parallel()
	d, r := textinput.NewSession(), newRecorder()
	d.Attach(&r.Composer)
	if r.update(t) {
		t.Error("an idle session's tick was handled")
	}
	expectLog(t, r, "idle", noPreedit)

	d.Send(preeditOf("k"))
	r.update(t)
	d.Send(preeditOf("")) // backspace inside the IME
	if !r.update(t) {
		t.Error("the tick in which the IME emptied its preedit was not handled; the caller would delete a character too")
	}
	if !d.Compositing() {
		t.Error("the session does not report compositing in the tick its preedit emptied")
	}
	if r.update(t) {
		t.Error("the tick after the preedit emptied was still handled")
	}
}

func TestComposerConfirmCommitsThePreedit(t *testing.T) {
	t.Parallel()
	d, r := textinput.NewSessionAround("x", "y"), newRecorder()
	d.Attach(&r.Composer)
	d.Send(preeditOf("にほん"))
	r.update(t)
	r.take()

	r.Confirm()
	expectLog(t, r, "confirm", "commit にほん", noPreedit)
	if !d.Closed() || textinput.HasSession(&r.Composer) {
		t.Errorf("after Confirm the session is closed %v and held %v, want closed and dropped", d.Closed(), textinput.HasSession(&r.Composer))
	}
	c := r.commits[0]
	if before, after := c.IsSurroundingTextReplaced(); before || after {
		t.Errorf("a confirmed preedit replaces surrounding text (%v, %v), want an insertion at the caret", before, after)
	}
	if before, after := c.SurroundingText(); before != "x" || after != "y" {
		t.Errorf("SurroundingText() = %q, %q, want \"x\", \"y\"", before, after)
	}

	r.Confirm()
	r.Cancel()
	expectLog(t, r, "confirm and cancel with no session")
}

func TestComposerConfirmWithoutPreeditCommitsNothing(t *testing.T) {
	t.Parallel()
	d, r := textinput.NewSession(), newRecorder()
	d.Attach(&r.Composer)
	r.update(t)
	r.take()
	r.Confirm()
	expectLog(t, r, "confirm with nothing composed", noPreedit)
}

func TestComposerCancelDiscardsThePreedit(t *testing.T) {
	t.Parallel()
	d, r := textinput.NewSession(), newRecorder()
	d.Attach(&r.Composer)
	d.Send(preeditOf("か"))
	r.update(t)
	r.take()
	r.Cancel()
	expectLog(t, r, "cancel", noPreedit)
	if !d.Closed() {
		t.Error("Cancel left the session open")
	}
}

func TestComposerUserEndingCommitsThePreedit(t *testing.T) {
	t.Parallel()
	d, r := textinput.NewSession(), newRecorder()
	d.Attach(&r.Composer)
	d.Send(preeditOf("か"))
	r.update(t)
	r.take()

	d.EndByUser()
	if r.update(t) {
		t.Error("the tick the user ended text input in was handled")
	}
	expectLog(t, r, "user ending", "commit か", noPreedit, "ended by user")
}

func TestComposerUserEndingAfterCommitStopsAfterIt(t *testing.T) {
	t.Parallel()
	d, r := textinput.NewSession(), newRecorder()
	d.Attach(&r.Composer)
	d.Send(commitOf("a"))
	d.EndByUser()
	if !r.update(t) {
		t.Error("the tick with a commit was not handled")
	}
	expectLog(t, r, "commit then user ending", "commit a", noPreedit, "ended by user")
}

func TestComposerPlatformTeardownDropsThePreedit(t *testing.T) {
	t.Parallel()
	d, r := textinput.NewSession(), newRecorder()
	d.Attach(&r.Composer)
	d.Send(preeditOf("か"))
	r.update(t)
	r.take()
	d.End()
	r.update(t)
	expectLog(t, r, "platform teardown", noPreedit)
}

func TestComposerReportsSessionError(t *testing.T) {
	t.Parallel()
	d, r := textinput.NewSession(), newRecorder()
	d.Attach(&r.Composer)
	boom := errors.New("boom")
	d.Send(textinput.TextInputState{Error: boom})
	if _, err := r.Update(); !errors.Is(err, boom) {
		t.Fatalf("Update() error = %v, want the platform's %v", err, boom)
	}
	expectLog(t, r, "session error", noPreedit)
	if !d.Closed() || textinput.HasSession(&r.Composer) {
		t.Error("a failed session was left open or held")
	}
}

func TestComposerRejectsInvalidSurroundingText(t *testing.T) {
	t.Parallel()
	for _, opts := range []textinput.SessionOptions{{TextBeforeCaret: "a\xff"}, {TextAfterCaret: "\xc3"}} {
		c := textinput.Composer{OnNewSession: func() *textinput.SessionOptions { return &opts }}
		if handled, err := c.Update(); err == nil || handled {
			t.Errorf("Update with %+v = %v, %v, want an error", opts, handled, err)
		}
	}
}

func TestComposerAsksForSessionEachTick(t *testing.T) {
	t.Parallel()
	asked := 0
	c := textinput.Composer{OnNewSession: func() *textinput.SessionOptions { asked++; return nil }}
	for range 2 {
		if handled, err := c.Update(); handled || err != nil {
			t.Fatalf("Update with no session offered = %v, %v, want false, nil", handled, err)
		}
	}
	if asked != 2 {
		t.Errorf("OnNewSession called %d times in 2 ticks, want 2", asked)
	}
}

func TestCommitSurroundingTextReplacement(t *testing.T) {
	t.Parallel()
	nr := textinput.NoReplacement
	for _, c := range []struct {
		name                  string
		start, end            int
		relative              bool
		replBefore, replAfter bool
		wantBefore, wantAfter string
	}{
		{"none", nr, nr, false, false, false, "abc", "def"},
		{"half a sentinel", nr, 5, false, false, false, "abc", "def"},
		{"across the caret", 1, 4, false, true, true, "a", "ef"},
		{"after the caret", 4, 5, false, true, true, "abcd", "f"},
		{"before the caret", 1, 2, false, true, true, "a", "cdef"},
		{"relative, one back", -1, 0, true, true, false, "ab", "def"},
		{"relative, none", 0, 0, true, false, false, "abc", "def"},
		{"relative, clamped to the text", -10, 10, true, true, true, "", ""},
		{"relative, end before start", 2, -5, true, true, true, "abcde", "f"},
	} {
		d := textinput.NewSessionAround("abc", "def")
		d.Send(textinput.TextInputState{Text: "X", CommitKind: textinput.CommitRegular,
			ReplacementStartInBytes: c.start, ReplacementEndInBytes: c.end, ReplacementRelativeToCaret: c.relative})
		if err := d.Update(); err != nil {
			t.Fatalf("%s: Update: %v", c.name, err)
		}
		commit := d.Commit()
		if commit == nil {
			t.Fatalf("%s: no commit", c.name)
		}
		if b, a := commit.IsSurroundingTextReplaced(); b != c.replBefore || a != c.replAfter {
			t.Errorf("%s: IsSurroundingTextReplaced() = %v, %v, want %v, %v", c.name, b, a, c.replBefore, c.replAfter)
		}
		if b, a := commit.SurroundingText(); b != c.wantBefore || a != c.wantAfter {
			t.Errorf("%s: SurroundingText() = %q, %q, want %q, %q", c.name, b, a, c.wantBefore, c.wantAfter)
		}
	}
}

func TestSessionMirrorsPreeditSelection(t *testing.T) {
	t.Parallel()
	d := textinput.NewSession()
	d.Send(textinput.TextInputState{Text: "にほん", CompositionSelectionStartInBytes: 3, CompositionSelectionEndInBytes: 9})
	if err := d.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	c := d.Composition()
	if start, end := c.SelectionRangeInBytes(); c.Text() != "にほん" || start != 3 || end != 9 {
		t.Errorf("Composition() = %q [%d,%d), want \"にほん\" [3,9)", c.Text(), start, end)
	}
	d.Cancel()
	if d.Compositing() {
		t.Error("a cancelled session still reports compositing")
	}
}

// Cancelling abandons the target: what was queued behind the session's
// commit belongs to it, and must not reach the next session.
func TestCancelDropsStatesQueuedForTheAbandonedTarget(t *testing.T) {
	t.Parallel()
	d := textinput.NewSession()
	d.Send(commitOf("a"))
	d.Send(preeditOf("b"))
	if got := d.Queued(); got != 1 {
		t.Fatalf("%d states queued behind the commit, want 1", got)
	}
	d.Cancel()
	d.Cancel() // no-op once closed
	if !d.Closed() {
		t.Fatal("Cancel left the session open")
	}
	if got := d.Queued(); got != 0 {
		t.Errorf("%d states still queued after Cancel, want 0", got)
	}
	d.StartNext()
	if err := d.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if d.Compositing() || d.Commit() != nil {
		t.Error("the next session received the abandoned target's states")
	}
}

// commitOf returns a regular commit of text at the caret.
func commitOf(text string) textinput.TextInputState {
	return textinput.TextInputState{Text: text, CommitKind: textinput.CommitRegular,
		ReplacementStartInBytes: textinput.NoReplacement, ReplacementEndInBytes: textinput.NoReplacement}
}

// preeditOf returns a composition of text with the caret at its end.
func preeditOf(text string) textinput.TextInputState {
	return textinput.TextInputState{Text: text, CompositionSelectionStartInBytes: len(text), CompositionSelectionEndInBytes: len(text)}
}
