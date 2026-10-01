//go:build !js

package textinput_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/textinput"
)

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
