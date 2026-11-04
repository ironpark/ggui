//go:build !js

package textinput_test

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
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

// FuzzComposerSession drives a Composer through a random sequence of
// platform states (preedits, commits, endings, errors) and application
// calls (ticks, Confirm, Cancel, new sessions), and checks that it never
// panics, delivers each commit at most once and in the order sent, delivers
// every platform commit when nothing discarded one, and hands out preedit
// selections and surrounding text within their text.
//
// Every state sent carries its sequence number: a platform commit reads
// "c<n>" and a preedit "p<n>…", so a commit of either kind names the state
// it came from.
func FuzzComposerSession(f *testing.F) {
	f.Add("ab", "cd", []byte{0, 1, 5, 0, 1, 1, 0, 5, 0})
	f.Add("", "", []byte{1, 0, 1, 1, 1, 2, 8, 0, 5, 0, 8, 0, 5, 0, 8, 0, 5, 0})
	f.Add("한글", "", []byte{0, 7, 3, 0, 5, 0, 8, 0, 0, 3, 6, 0, 5, 0})
	f.Add("x", "y", []byte{0, 2, 7, 0, 0, 2, 5, 0, 4, 0, 5, 0, 2, 0, 5, 0})
	f.Add("abc", "def", []byte{1, 0xf1, 9, 0, 5, 0, 8, 0, 0, 1, 5, 0})
	f.Fuzz(func(t *testing.T, before, after string, ops []byte) {
		before, after = strings.ToValidUTF8(before, "?"), strings.ToValidUTF8(after, "?")
		total := before + after
		d := textinput.NewSessionAround(before, after)
		var c textinput.Composer
		d.Attach(&c)

		var (
			seq       int   // the sequence number of the next state
			delivered []int // the sequence numbers of the commits delivered
			platform  []int // those of the platform commits sent
			lossy     bool  // whether a call could discard a commit sent
			errSent   bool  // whether an error state was sent
		)
		c.OnNewSession = func() *textinput.SessionOptions { return nil } // the harness attaches sessions
		c.OnComposition = func(comp *textinput.Composition) {
			start, end := comp.SelectionRangeInBytes()
			if s := comp.Text(); s != "" && !strings.HasPrefix(s, "p") || start < 0 || start > end || end > len(comp.Text()) {
				t.Fatalf("preedit %q with selection [%d,%d) is not one sent", comp.Text(), start, end)
			}
		}
		c.OnCommit = func(cm *textinput.Commit) {
			text := cm.Text()
			n, err := seqOf(text)
			if err != nil {
				t.Fatalf("commit %q is no state sent", text)
			}
			if len(delivered) > 0 && n <= delivered[len(delivered)-1] {
				t.Fatalf("commit %q delivered after state %d; commits delivered %v, want each once, in order", text, delivered[len(delivered)-1], delivered)
			}
			delivered = append(delivered, n)

			b, a := cm.SurroundingText()
			rb, ra := cm.IsSurroundingTextReplaced()
			if !strings.HasPrefix(total, b) || !strings.HasSuffix(total, a) || len(b)+len(a) > len(total) {
				t.Fatalf("SurroundingText() = %q, %q lies outside %q", b, a, total)
			}
			if rb != (b != before) || ra != (a != after) {
				t.Fatalf("IsSurroundingTextReplaced() = %v, %v but SurroundingText() = %q, %q around %q|%q", rb, ra, b, a, before, after)
			}
			if text[0] == 'p' && (rb || ra) {
				t.Fatalf("committing preedit %q replaces surrounding text", text)
			}
		}

		next := func() byte {
			if len(ops) == 0 {
				return 0
			}
			b := ops[0]
			ops = ops[1:]
			return b
		}
		update := func() {
			if _, err := c.Update(); err != nil && !errSent {
				t.Fatalf("Update() = %v with no error sent", err)
			}
		}
		for len(ops) > 0 {
			op, arg := next(), next()
			switch op % 10 {
			case 0: // preedit, with a selection inside it
				text := "p" + strconv.Itoa(seq) + strings.Repeat("か", int(arg%4))
				start := int(arg>>2) % (len(text) + 1)
				d.Send(textinput.TextInputState{Text: text, CompositionSelectionStartInBytes: start, CompositionSelectionEndInBytes: len(text)})
			case 1, 2: // commit, maybe replacing text around a caret, maybe passing its key on
				st := textinput.TextInputState{Text: "c" + strconv.Itoa(seq), CommitKind: textinput.CommitRegular,
					ReplacementStartInBytes: textinput.NoReplacement, ReplacementEndInBytes: textinput.NoReplacement}
				if op%10 == 2 {
					st.CommitKind = textinput.CommitWithPassthroughKey
				}
				if arg&1 != 0 {
					st.ReplacementRelativeToCaret = true
					st.ReplacementStartInBytes, st.ReplacementEndInBytes = int(int8(arg))>>3, int(arg>>1&7)-2
				}
				platform = append(platform, seq)
				d.Send(st)
			case 3:
				d.EndByUser()
			case 4:
				d.End()
			case 5:
				update()
			case 6:
				c.Confirm()
				lossy = true // a commit waiting in the closed channel is gone
			case 7:
				c.Cancel()
				lossy = true
			case 8: // the application starts the next session
				if !textinput.HasSession(&c) {
					d.StartNext()
					d.Attach(&c)
				}
			case 9:
				if arg < 16 { // rarely: the platform fails
					d.Send(textinput.TextInputState{Error: errors.New("platform error")})
					errSent, lossy = true, true
				}
			}
			seq++
		}

		// Drain: each session takes at most one commit.
		for range len(platform) + 2 {
			if !textinput.HasSession(&c) {
				d.StartNext()
				d.Attach(&c)
			}
			update()
		}
		if lossy {
			return
		}
		for _, n := range platform {
			found := false
			for _, m := range delivered {
				found = found || m == n
			}
			if !found {
				t.Fatalf("platform commit c%d was never delivered; sent %v, delivered %v", n, platform, delivered)
			}
		}
	})
}

// seqOf returns the sequence number a commit's text starts with, after its
// kind letter.
func seqOf(text string) (int, error) {
	if text == "" || text[0] != 'c' && text[0] != 'p' {
		return 0, errors.New("no kind letter")
	}
	digits := text[1:]
	if i := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
		digits = digits[:i]
	}
	return strconv.Atoi(digits)
}
