package a11y

import (
	"sort"
	"sync/atomic"
)

// A text field is the one node an assistive technology does not merely read
// out: it walks it, character by character and line by line, and moves the
// caret as it goes. macOS asks for that through a protocol of its own, in
// UTF-16 offsets, and ggui stores UTF-8 bytes, so everything below converts
// between the two. The answers come from the frozen snapshot -- Node.Value,
// Node.SelStart and SelEnd, and the Runs a text widget froze into it -- and
// never from the live widget, which belongs to another thread.

// axDetail reports whether anything is reading the tree closely enough to
// want a text node's full layout. Building it costs a measurement per
// character on every frame, which is not a price to pay while nothing is
// listening; a widget asks this before filling Node.Runs.
func WantsDetail() bool { return axWantsDetail.Load() }

// SetWantsDetail forces the answer WantsDetail gives. The bridge sets it
// itself as assistive technologies come and go; a test that wants a widget
// to freeze its text layout without one attached sets it by hand.
func SetWantsDetail(on bool) { axWantsDetail.Store(on) }

var axWantsDetail atomic.Bool

// axUTF16Len is the length of s in UTF-16 code units, which is what every
// offset crossing into AppKit is counted in.
func axUTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// axByteAt converts a UTF-16 offset into a byte offset in s, clamped to the
// ends. An offset landing between the halves of a surrogate pair resolves
// to the start of the rune, which is where a caret can actually go.
func axByteAt(s string, u int) int {
	if u <= 0 {
		return 0
	}
	n := 0
	for i, r := range s {
		w := 1
		if r > 0xFFFF {
			w = 2
		}
		if n >= u || n+w > u {
			return i
		}
		n += w
	}
	return len(s)
}

// axUTF16At converts a byte offset in s into a UTF-16 offset.
func axUTF16At(s string, b int) int {
	n := 0
	for i, r := range s {
		if i >= b {
			return n
		}
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// CharCount is the length of the field, in the units AppKit counts in.
func CharCount(n Node) int {
	if n.runUnits != nil {
		return n.runUnits[len(n.Runs)]
	}
	return axUTF16Len(n.Value)
}

// indexRuns fills runUnits in one walk of Value, if every run starts on a
// rune boundary of it and none before the one ahead of it: anything else
// is left to the walk from the start, which takes it as it is.
func (n *Node) indexRuns() {
	if len(n.Runs) == 0 {
		return
	}
	units := make([]int, len(n.Runs)+1)
	i, u := 0, 0
	for b, r := range n.Value {
		for ; i < len(n.Runs) && n.Runs[i].Start <= b; i++ {
			if n.Runs[i].Start != b {
				return
			}
			units[i] = u
		}
		u++
		if r > 0xFFFF {
			u++
		}
	}
	for ; i < len(n.Runs); i++ {
		if n.Runs[i].Start != len(n.Value) {
			return
		}
		units[i] = u
	}
	units[len(n.Runs)] = u
	n.runUnits = units
}

// unitsAt is axUTF16At over Value, from the start of b's line.
func (n Node) unitsAt(b int) int {
	b0, u0 := 0, 0
	if n.runUnits != nil {
		if i := sort.Search(len(n.Runs), func(i int) bool { return n.Runs[i].Start > b }) - 1; i >= 0 {
			b0, u0 = n.Runs[i].Start, n.runUnits[i]
		}
	}
	return u0 + axUTF16At(n.Value[b0:], b-b0)
}

// byteAt is axByteAt over Value, from the start of u's line.
func (n Node) byteAt(u int) int {
	b0, u0 := 0, 0
	if n.runUnits != nil {
		if i := sort.Search(len(n.Runs), func(i int) bool { return n.runUnits[i] > u }) - 1; i >= 0 {
			b0, u0 = n.Runs[i].Start, n.runUnits[i]
		}
	}
	return b0 + axByteAt(n.Value[b0:], u-u0)
}

// Selection is the selected range, as a UTF-16 location and length. An
// empty selection is the caret, which is what it usually is.
func Selection(n Node) (loc, length int) {
	lo := n.unitsAt(n.SelStart)
	return lo, n.unitsAt(n.SelEnd) - lo
}

// Selected is the selected text.
func Selected(n Node) string {
	loc, length := Selection(n)
	return StringForRange(n, loc, length)
}

// ByteRange converts a UTF-16 location and length into the byte range an
// Action carries, clamped to the text.
func ByteRange(n Node, loc, length int) (start, end int) {
	start = n.byteAt(loc)
	end = n.byteAt(loc + max(length, 0))
	return start, max(end, start)
}

// StringForRange is the text a UTF-16 range covers.
func StringForRange(n Node, loc, length int) string {
	start, end := ByteRange(n, loc, length)
	return n.Value[start:end]
}

// LineForIndex is the line a UTF-16 offset falls on, counting from zero.
// A field that never said how it was laid out is one line, which is the
// honest answer for a field that is.
func LineForIndex(n Node, u int) int {
	b := n.byteAt(u)
	// Runs are in order, so the first that ends at or past b is found by
	// halving.
	if i := sort.Search(len(n.Runs), func(i int) bool { return b <= n.Runs[i].End }); i < len(n.Runs) {
		return i
	}
	return max(len(n.Runs)-1, 0)
}

// RangeForLine is the UTF-16 range a line covers, and false for a line
// number the field does not have.
func RangeForLine(n Node, line int) (loc, length int, ok bool) {
	if line < 0 || line >= len(n.Runs) {
		if line == 0 {
			return 0, CharCount(n), true
		}
		return 0, 0, false
	}
	// To the start of the line, then on through it alone.
	r := n.Runs[line]
	start, end := min(max(r.Start, 0), len(n.Value)), min(max(r.End, 0), len(n.Value))
	loc = n.unitsAt(start)
	if end <= start {
		return loc, 0, true
	}
	return loc, axUTF16At(n.Value[start:], end-start), true
}

// InsertionLine is the line the caret is on, which is what VoiceOver says
// when the user moves between lines.
func InsertionLine(n Node) int { return LineForIndex(n, n.unitsAt(n.SelEnd)) }

// RectForRange is the area a UTF-16 range covers, in the same logical
// coordinates as SemNode.Full. A range spanning several lines comes back as
// the part of it on the first, since the caller wants somewhere to put a
// cursor and a union of lines is not that. A range on a field that froze no
// layout falls back to the whole field.
func RectForRange(n SemNode, loc, length int) Rect {
	start, end := ByteRange(n.Node, loc, length)
	for _, r := range n.Runs {
		if start > r.End {
			continue
		}
		a := r.At(start)
		b := r.At(min(end, r.End))
		return Rct(Pt(r.Rect.Origin.X+a, r.Rect.Origin.Y), Sz(max(b-a, 1), r.Rect.Size.H))
	}
	return n.Full
}
