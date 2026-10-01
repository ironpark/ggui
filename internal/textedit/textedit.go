// Package textedit is the editing model behind ggui's TextInput: a string,
// a selection, undo history, and caret movement by grapheme cluster and by
// word. It knows nothing of layout, painting or input events.
package textedit

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ironpark/ggui/internal/fn"
)

// Editor is the model behind TextInput: a string and a selection. The
// caret sits at Caret; Anchor is where the selection started, so Anchor ==
// Caret means no selection. Offsets are bytes on rune boundaries.
type Editor struct {
	Text   string
	Anchor int
	Caret  int

	// Now is the clock that decides whether typed insertions undo as one;
	// nil is time.Now.
	Now func() time.Time

	// Undo history. pending is the state before the latest step, whole.
	// Every older state is kept as the difference from the state after it,
	// and every undone one as the difference from the state before it, so
	// a long text costs one copy rather than one a step. Insertions typed
	// in quick succession share one step.
	pending    *snapshot
	undo, redo []delta
	lastEdit   time.Time
	lastInsert bool
}

// snapshot is the editor's state at one point of the history.
type snapshot struct {
	text          string
	anchor, caret int
}

// delta turns one text into a snapshot: it replaces the cut bytes at at
// with text and sets the selection.
type delta struct {
	at, cut       int
	text          string
	anchor, caret int
}

// diff returns the delta from the text from to the state to, holding only
// the bytes that differ.
func diff(from string, to snapshot) delta {
	p := 0
	for p < len(from) && p < len(to.text) && from[p] == to.text[p] {
		p++
	}
	s := 0
	for s < len(from)-p && s < len(to.text)-p && from[len(from)-1-s] == to.text[len(to.text)-1-s] {
		s++
	}
	return delta{at: p, cut: len(from) - p - s, text: strings.Clone(to.text[p : len(to.text)-s]), anchor: to.anchor, caret: to.caret}
}

func (d delta) apply(from string) snapshot {
	return snapshot{from[:d.at] + d.text + from[d.at+d.cut:], d.anchor, d.caret}
}

func (e *Editor) state() snapshot { return snapshot{e.Text, e.Anchor, e.Caret} }

// push makes the current state the latest one undo returns to, keeping the
// one before it as a delta.
func (e *Editor) push() {
	if e.pending != nil {
		e.undo = append(e.undo, diff(e.Text, *e.pending))
		if len(e.undo) >= maxUndo {
			e.undo = e.undo[1:]
		}
	}
	s := e.state()
	e.pending = &s
}

func (e *Editor) now() time.Time {
	if e.Now == nil {
		return time.Now()
	}
	return e.Now()
}

// maxUndo bounds the history.
const maxUndo = 200

// undoCoalesce is how close two typed insertions must be to undo as one.
const undoCoalesce = 700 * time.Millisecond

// record takes a snapshot before an edit that replaces the selection with
// s. A plain insertion right after another joins the previous step, so a
// word typed undoes at once, while deletions and pastes stand alone.
func (e *Editor) record(s string) {
	now := e.now()
	insert := s != "" && !e.HasSelection() && utf8.RuneCountInString(s) == 1
	if insert && e.lastInsert && now.Sub(e.lastEdit) < undoCoalesce && e.pending != nil {
		e.lastEdit = now
		return
	}
	e.push()
	e.redo = e.redo[:0]
	e.lastEdit, e.lastInsert = now, insert
}

// Undo restores the state before the last edit and reports whether there
// was one.
func (e *Editor) Undo() bool {
	if e.pending == nil {
		return false
	}
	to := *e.pending
	e.redo = append(e.redo, diff(to.text, e.state()))
	e.pending = nil
	if n := len(e.undo); n > 0 {
		s := e.undo[n-1].apply(to.text)
		e.pending = &s
		e.undo = e.undo[:n-1]
	}
	e.restore(to)
	return true
}

// Redo reapplies the last undone edit and reports whether there was one.
func (e *Editor) Redo() bool {
	n := len(e.redo)
	if n == 0 {
		return false
	}
	to := e.redo[n-1].apply(e.Text)
	e.redo = e.redo[:n-1]
	e.push()
	e.restore(to)
	return true
}

func (e *Editor) restore(s snapshot) {
	e.Text, e.Anchor, e.Caret = s.text, s.anchor, s.caret
	e.lastInsert = false
}

// SetText replaces the text from outside the editor. What was undone can
// no longer be redone onto it; undo still returns to the states before.
func (e *Editor) SetText(s string) {
	if s != e.Text {
		e.redo = e.redo[:0]
	}
	e.Text = s
	e.Anchor, e.Caret = e.Snap(e.Anchor), e.Snap(e.Caret)
}

// Snap clamps i into the text and back to the start of the rune it is in.
func (e *Editor) Snap(i int) int {
	i = min(max(i, 0), len(e.Text))
	for i > 0 && i < len(e.Text) && !utf8.RuneStart(e.Text[i]) {
		i--
	}
	return i
}

// Selection returns the selected byte range, lo <= hi.
func (e *Editor) Selection() (lo, hi int) {
	return min(e.Anchor, e.Caret), max(e.Anchor, e.Caret)
}

func (e *Editor) HasSelection() bool { return e.Anchor != e.Caret }

func (e *Editor) Selected() string {
	lo, hi := e.Selection()
	return e.Text[lo:hi]
}

// Replace puts s in place of the selection and leaves the caret after it.
func (e *Editor) Replace(s string) {
	e.record(s)
	lo, hi := e.Selection()
	e.Text = e.Text[:lo] + s + e.Text[hi:]
	e.Caret = lo + len(s)
	e.Anchor = e.Caret
}

// MoveTo puts the caret at pos, extending the selection or collapsing it.
func (e *Editor) MoveTo(pos int, extend bool) {
	e.Caret = e.Snap(pos)
	if !extend {
		e.Anchor = e.Caret
	}
}

// MoveBy moves the caret one rune or one word left (dir < 0) or right.
// Without extend, a selection collapses to its edge in that direction first,
// as every text field does.
func (e *Editor) MoveBy(dir int, word, extend bool) {
	if !extend && e.HasSelection() && !word {
		lo, hi := e.Selection()
		e.MoveTo(fn.Pick(dir < 0, lo, hi), false)
		return
	}
	pos := e.Caret
	switch {
	case dir < 0 && word:
		pos = prevWord(e.Text, pos)
	case dir < 0:
		pos = PrevGrapheme(e.Text, pos)
	case word:
		pos = nextWord(e.Text, pos)
	default:
		pos = NextGrapheme(e.Text, pos)
	}
	e.MoveTo(pos, extend)
}

// Backspace deletes the selection, or the grapheme or word before the caret.
func (e *Editor) Backspace(word bool) {
	if !e.HasSelection() {
		e.Anchor = fn.Pick(word, prevWord(e.Text, e.Caret), PrevGrapheme(e.Text, e.Caret))
	}
	e.Replace("")
}

// DeleteForward deletes the selection, or the grapheme or word after the caret.
func (e *Editor) DeleteForward(word bool) {
	if !e.HasSelection() {
		e.Anchor = fn.Pick(word, nextWord(e.Text, e.Caret), NextGrapheme(e.Text, e.Caret))
	}
	e.Replace("")
}

// Grapheme clusters, approximately: the caret and Backspace step over a
// base rune together with what attaches to it. Without a segmentation
// table this covers what shows up in practice: combining marks, variation
// selectors, emoji modifiers and tags, zero-width-joiner sequences, CRLF,
// and regional indicator pairs. Conjoining Hangul jamo are handled too,
// though text from an IME arrives precomposed.

// extends reports whether r attaches to the rune before it.
func extends(r rune) bool {
	switch {
	case unicode.Is(unicode.M, r): // combining marks
		return true
	case r >= 0xFE00 && r <= 0xFE0F, r >= 0xE0100 && r <= 0xE01EF: // variation selectors
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF: // emoji skin tones
		return true
	case r >= 0xE0020 && r <= 0xE007F: // emoji tags
		return true
	case r == 0x200D, r == 0x200C: // zero-width joiner and non-joiner
		return true
	case r >= 0x1160 && r <= 0x11FF: // Hangul jamo vowels and trailing consonants
		return true
	}
	return false
}

func isRegionalIndicator(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

// NextGrapheme returns the byte offset after the cluster starting at i.
func NextGrapheme(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	r, n := utf8.DecodeRuneInString(s[i:])
	j := i + n
	if r == '\r' && j < len(s) && s[j] == '\n' {
		return j + 1
	}
	if r == '\r' || r == '\n' {
		return j // a line break is a cluster of its own
	}
	if isRegionalIndicator(r) {
		if r2, n2 := utf8.DecodeRuneInString(s[j:]); isRegionalIndicator(r2) {
			return j + n2
		}
		return j
	}
	for j < len(s) {
		r2, n2 := utf8.DecodeRuneInString(s[j:])
		if !extends(r2) {
			break
		}
		j += n2
		if r2 == 0x200D && j < len(s) && s[j] != '\r' && s[j] != '\n' {
			// What follows a joiner belongs to the cluster, unless it
			// breaks the line.
			_, n3 := utf8.DecodeRuneInString(s[j:])
			j += n3
		}
	}
	return j
}

// PrevGrapheme returns the byte offset of the cluster ending at i.
func PrevGrapheme(s string, i int) int {
	if i <= 0 {
		return 0
	}
	if s[i-1] == '\n' {
		if i >= 2 && s[i-2] == '\r' {
			return i - 2
		}
		return i - 1
	}
	// Walk clusters from the start of the line; text fields are short.
	start := i
	for start > 0 && s[start-1] != '\n' {
		start--
	}
	for j := start; j < i; {
		k := NextGrapheme(s, j)
		if k >= i {
			return j
		}
		j = k
	}
	return prevRune(s, i)
}

// SelectLine selects the line around i, from the hard line break before
// it to the one after, which stays unselected: what a triple-click selects.
func (e *Editor) SelectLine(i int) {
	i = e.Snap(i)
	start := strings.LastIndexByte(e.Text[:i], '\n') + 1
	end := len(e.Text)
	if n := strings.IndexByte(e.Text[i:], '\n'); n >= 0 {
		end = i + n
	}
	e.Anchor, e.Caret = start, end
}

func (e *Editor) SelectAll() { e.Anchor, e.Caret = 0, len(e.Text) }

// SelectWord selects the word around pos, or the run of spaces it is in.
func (e *Editor) SelectWord(pos int) {
	pos = e.Snap(pos)
	if len(e.Text) == 0 {
		return
	}
	// The cluster holding pos, or at the end the last one; its first rune
	// gives the class.
	pos = PrevGrapheme(e.Text, NextRune(e.Text, pos))
	r, _ := utf8.DecodeRuneInString(e.Text[pos:])
	class := runeClass(r)
	e.Anchor, e.Caret = runStart(e.Text, pos, class), runEnd(e.Text, pos, class)
}

func prevRune(s string, i int) int {
	if i <= 0 {
		return 0
	}
	_, n := utf8.DecodeLastRuneInString(s[:i])
	return i - n
}

// NextRune returns the byte offset after the rune starting at i.
func NextRune(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	_, n := utf8.DecodeRuneInString(s[i:])
	return i + n
}

// spaceClass is the runeClass of spaces.
const spaceClass = 0

// runeClass groups runes for word movement: spaces, word characters and
// punctuation each form their own runs. A rune that extends a cluster, a
// combining mark or a joiner, has no class of its own: it goes with the
// rune it is attached to; see runStart and runEnd.
func runeClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return spaceClass
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	}
	return 2
}

// prevWord returns the start of the word before i: spaces are skipped, then
// the run of same-class runes before them.
func prevWord(s string, i int) int {
	i = runStart(s, i, spaceClass)
	if i == 0 {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s[PrevGrapheme(s, i):])
	return runStart(s, i, runeClass(r))
}

// runStart returns the start of the run of class that ends at i, cluster
// by cluster, each taking the class of its first rune. The clusters are
// found forward from the start of each line, as NextGrapheme finds them, so
// the edges agree with the arrows'.
func runStart(s string, i, class int) int {
	for i > 0 {
		line := i - 1
		for line > 0 && s[line-1] != '\n' {
			line--
		}
		var starts []int
		for j := line; j < i; j = NextGrapheme(s, j) {
			starts = append(starts, j)
		}
		for k := len(starts) - 1; k >= 0; k-- {
			if r, _ := utf8.DecodeRuneInString(s[starts[k]:]); runeClass(r) != class {
				return i
			}
			i = starts[k]
		}
	}
	return i
}

// runEnd returns the end of the run of class that starts at i, cluster by
// cluster, each taking the class of its first rune.
func runEnd(s string, i, class int) int {
	for i < len(s) {
		if r, _ := utf8.DecodeRuneInString(s[i:]); runeClass(r) != class {
			break
		}
		i = NextGrapheme(s, i)
	}
	return i
}

// nextWord returns the end of the word after i: the run of same-class runes
// at i, then the spaces after it.
func nextWord(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return runEnd(s, runEnd(s, i, runeClass(r)), spaceClass)
}
