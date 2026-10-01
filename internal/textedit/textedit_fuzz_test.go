package textedit

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ironpark/ggui/internal/fn"
)

// palette is what the editor fuzz target types: scripts and sequences whose
// clusters span several runes, alongside plain ASCII.
var palette = []string{
	"a", "Z", " ", "_", ".", "héllo", "é", "\u0301", "\u200d", "\u200c",
	"\U0001F468\u200d\U0001F469\u200d\U0001F467", "\U0001F44D\U0001F3FD",
	"\U0001F1F0\U0001F1F7", "\U0001F1FA", "1\ufe0f\u20e3", "❤\ufe0f",
	"\U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F",
	"한글", "각", "漢字", "שלום", "مرحبا", "क्ष",
	"\r\n", "\n", "\t",
}

// boundaries returns the cluster boundaries of s, walking NextGrapheme from
// the start, in order.
func boundaries(s string) []int {
	b := []int{0}
	for i := 0; i < len(s); {
		i = NextGrapheme(s, i)
		b = append(b, i)
	}
	return b
}

// hasExtender reports whether s holds a rune that attaches to the one before
// it.
func hasExtender(s string) bool { return strings.ContainsFunc(s, extends) }

// FuzzGraphemeBoundaries checks the segmentation helpers on any string:
// each step makes progress within bounds and lands on a rune start, walking
// forward and backward finds the same clusters, and text without
// multi-rune clusters splits exactly at its runes.
func FuzzGraphemeBoundaries(f *testing.F) {
	for _, s := range palette {
		f.Add(s)
	}
	for _, s := range []string{
		"", "abc", "a\r\nb", "\r\r\n\n", "\U0001F1F0\U0001F1F7\U0001F1FA", "é\u0302x",
		"\U0001F468\u200d\u200d\U0001F469", "\xff\u0301", "a\xe2\x80", "\n\u0301", "a\u200d\nb",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 256 {
			return // PrevGrapheme walks the line, so checking every offset is quadratic
		}
		valid := utf8.ValidString(s)
		for i := 0; i <= len(s); i++ {
			n, p := NextGrapheme(s, i), PrevGrapheme(s, i)
			if i < len(s) && (n <= i || n > len(s)) || i == len(s) && n != len(s) {
				t.Fatalf("NextGrapheme(%q, %d) = %d, want in (%d, %d]", s, i, n, i, len(s))
			}
			if i > 0 && (p >= i || p < 0) || i == 0 && p != 0 {
				t.Fatalf("PrevGrapheme(%q, %d) = %d, want in [0, %d)", s, i, p, i)
			}
			_, size := utf8.DecodeRuneInString(s[i:])
			if got := NextRune(s, i); got != i+size {
				t.Fatalf("NextRune(%q, %d) = %d, want %d", s, i, got, i+size)
			}
			_, size = utf8.DecodeLastRuneInString(s[:i])
			if got := prevRune(s, i); got != i-size {
				t.Fatalf("prevRune(%q, %d) = %d, want %d", s, i, got, i-size)
			}
			if valid && i < len(s) && utf8.RuneStart(s[i]) {
				if n < len(s) && !utf8.RuneStart(s[n]) || !utf8.RuneStart(s[p]) {
					t.Fatalf("stepping from rune start %d in %q left a rune: next %d, prev %d", i, s, n, p)
				}
			}
		}

		forward := boundaries(s)
		{
			backward := []int{len(s)}
			for i := len(s); i > 0; {
				i = PrevGrapheme(s, i)
				backward = append(backward, i)
			}
			slices.Reverse(backward)
			if !slices.Equal(forward, backward) {
				t.Fatalf("clusters of %q walking forward %v, backward %v", s, forward, backward)
			}
		}

		// A cluster starts with an extending rune only at the start, or after
		// a flag or a line break, which take nothing after them.
		for _, b := range forward {
			if b == 0 || b == len(s) {
				continue
			}
			r, _ := utf8.DecodeRuneInString(s[b:])
			q, _ := utf8.DecodeLastRuneInString(s[:b])
			if extends(r) && !isRegionalIndicator(q) && q != '\n' && q != '\r' {
				t.Fatalf("cluster at %d of %q starts with %U after %U, which it should extend", b, s, r, q)
			}
		}

		// Reference: without extenders, flags or CR every rune is a cluster.
		if valid && !hasExtender(s) && !strings.ContainsFunc(s, isRegionalIndicator) && !strings.Contains(s, "\r") {
			runes := []int{}
			for i := range s {
				runes = append(runes, i)
			}
			runes = append(runes, len(s))
			if !slices.Equal(forward, runes) {
				t.Fatalf("clusters of %q at %v, want every rune %v", s, forward, runes)
			}
		}
	})
}

// editorState is what an edit or undo step may change.
type editorState struct {
	text          string
	anchor, caret int
}

func stateOf(e *Editor) editorState { return editorState{e.Text, e.Anchor, e.Caret} }

// FuzzEditorOperations decodes a sequence of editing operations and checks
// after each one that the text stays valid UTF-8, the caret and anchor stay
// in the text on rune starts, edits change exactly what they claim, moves
// from cluster boundaries land on cluster boundaries, and that undoing every
// edit gives back the initial text, then redoing them the final one.
func FuzzEditorOperations(f *testing.F) {
	f.Add("", []byte{0, 3, 0, 17, 1, 12, 13})
	f.Add("héllo wörld", []byte{5, 0, 7, 1, 2, 11, 3, 0, 5, 14, 12, 12, 13})
	f.Add("é \U0001F468\u200d\U0001F469 \U0001F1F0\U0001F1F7\r\nx", []byte{9, 0, 5, 0, 5, 0, 1, 3, 6, 1, 14, 12})
	f.Add("한글 입력", []byte{0, 17, 0, 18, 15, 100, 0, 19, 2, 12, 14, 13, 10, 0, 5})
	f.Add("abc", []byte{10, 0, 4, 1, 12, 13, 13, 16, 7, 6, 3})
	f.Add("क्ष שלום", []byte{8, 1, 7, 0, 4, 12})
	// Double-click at the end of a word ending in a mark, and a word move
	// over a space that carries one: both once stopped inside the cluster.
	f.Add("00\u032d", []byte("\xa42"))
	f.Add("00 \u0301", []byte("y0\"!+0*"))
	// A word move back over a flag after a joiner that opens a line.
	f.Add("0", []byte("\"1\"\"\"Z)"))
	f.Fuzz(func(t *testing.T, initial string, ops []byte) {
		initial = strings.ToValidUTF8(initial, "?")
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		e := Editor{Now: func() time.Time { return now }}
		e.SetText(initial)
		e.MoveTo(len(initial), false)

		next := func() byte {
			if len(ops) == 0 {
				return 0
			}
			b := ops[0]
			ops = ops[1:]
			return b
		}
		edits := 0
		for len(ops) > 0 {
			op, arg := next(), next()
			old := stateOf(&e)
			oldBounds := boundaries(old.text)
			onBoundary := func(i int) bool { _, ok := slices.BinarySearch(oldBounds, i); return ok }
			aligned := onBoundary(old.anchor) && onBoundary(old.caret)
			lo, hi := e.Selection()
			extend := arg&1 != 0

			// motion is set for a move that must keep an aligned caret aligned,
			// word for a word-wise one.
			motion, word := false, false
			// want is the text an edit must leave, and wantCaret its caret.
			want, wantCaret := old.text, -1
			switch op % 18 {
			case 0: // type or paste
				s := palette[int(arg)%len(palette)]
				e.Replace(s)
				edits++
				want, wantCaret = old.text[:lo]+s+old.text[hi:], lo+len(s)
			case 1, 2: // backspace, by cluster or word
				word = op%18 == 2
				if !e.HasSelection() {
					lo = fn.Pick(word, prevWord(old.text, old.caret), PrevGrapheme(old.text, old.caret))
					if aligned && !word && !onBoundary(lo) {
						t.Fatalf("backspace in %q at %d would stop inside a cluster, at %d", old.text, old.caret, lo)
					}
				}
				e.Backspace(word)
				edits++
				want, wantCaret = old.text[:lo]+old.text[hi:], lo
			case 3, 4: // delete forward, by cluster or word
				word = op%18 == 4
				if !e.HasSelection() {
					hi = fn.Pick(word, nextWord(old.text, old.caret), NextGrapheme(old.text, old.caret))
				}
				e.DeleteForward(word)
				edits++
				want, wantCaret = old.text[:lo]+old.text[hi:], lo
			case 5, 6: // arrows
				e.MoveBy(fn.Pick(op%18 == 5, -1, 1), false, extend)
				motion = true
			case 7, 8: // word arrows
				e.MoveBy(fn.Pick(op%18 == 7, -1, 1), true, extend)
				motion, word = true, true
			case 9: // home and end
				e.MoveTo(fn.Pick(arg&2 != 0, len(e.Text), 0), extend)
				motion = true
			case 10:
				e.SelectAll()
				motion = true
			case 11: // double-click on a cluster boundary
				e.SelectWord(oldBounds[int(arg)%len(oldBounds)])
				motion, word = true, true
			case 12:
				e.Undo()
			case 13:
				e.Redo()
			case 14: // undo then redo is no change
				if e.Undo() {
					if !e.Redo() {
						t.Fatalf("redo after an undo of %q reported nothing to redo", old.text)
					}
					if got := stateOf(&e); got != old {
						t.Fatalf("undo then redo gave %+v, want %+v", got, old)
					}
				}
			case 15: // pause typing
				now = now.Add(time.Duration(arg) * 10 * time.Millisecond)
			case 16: // click anywhere
				e.MoveTo(int(arg)*len(e.Text)/255, extend)
			case 17: // a write from outside, as through the bound signal
				e.SetText(e.Text[:e.Snap(int(arg)*len(e.Text)/255)] + palette[int(arg)%len(palette)])
				if e.pending == nil {
					initial = e.Text // nothing to undo: it is where undo stops
				}
			}

			if !utf8.ValidString(e.Text) {
				t.Fatalf("op %d left invalid UTF-8 %q", op%18, e.Text)
			}
			for name, i := range map[string]int{"anchor": e.Anchor, "caret": e.Caret} {
				if i < 0 || i > len(e.Text) || e.Snap(i) != i {
					t.Fatalf("op %d left the %s at %d in %q, off a rune start or out of [0,%d]", op%18, name, i, e.Text, len(e.Text))
				}
			}
			_ = e.Selected()
			if wantCaret >= 0 && (e.Text != want || e.Caret != wantCaret || e.HasSelection()) {
				t.Fatalf("op %d on %+v gave %q caret %d anchor %d, want %q caret %d and no selection",
					op%18, old, e.Text, e.Caret, e.Anchor, want, wantCaret)
			}
			if motion && e.Text != old.text {
				t.Fatalf("move op %d changed the text from %q to %q", op%18, old.text, e.Text)
			}
			if motion && aligned && !(onBoundary(e.Anchor) && onBoundary(e.Caret)) {
				t.Fatalf("move op %d from %+v left anchor %d caret %d inside a cluster; boundaries %v",
					op%18, old, e.Anchor, e.Caret, oldBounds)
			}
		}

		if edits >= maxUndo {
			return // the oldest steps are gone
		}
		final := stateOf(&e)
		undone := 0
		for e.Undo() {
			undone++
		}
		if e.Text != initial {
			t.Fatalf("undoing every edit left %q, want the initial %q", e.Text, initial)
		}
		for range undone {
			if !e.Redo() {
				t.Fatalf("redo ran out before restoring %d undone steps", undone)
			}
		}
		if got := stateOf(&e); got != final {
			t.Fatalf("redoing every undone step gave %+v, want %+v", got, final)
		}
	})
}
