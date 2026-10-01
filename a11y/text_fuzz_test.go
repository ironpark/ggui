package a11y

import (
	"math"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

// runeStarts returns every byte offset a range loop over s stops at, plus
// len(s): the places a caret can be.
func runeStarts(s string) []int {
	var out []int
	for i := range s {
		out = append(out, i)
	}
	return append(out, len(s))
}

// layOut splits s into contiguous runs, breaking before a rune whenever the
// matching bit of breaks is set, with a stop 8 units apart at every rune
// boundary of each run.
func layOut(s string, breaks uint64) []TextRun {
	bounds := runeStarts(s)
	var runs []TextRun
	start := 0
	for k, b := range bounds[1:] {
		if b != len(s) && breaks&(1<<(uint(k)%64)) == 0 {
			continue
		}
		r := TextRun{Start: start, End: b, Rect: Rct(Pt(0, float64(len(runs))*10), Sz(100, 10))}
		for _, stop := range bounds {
			if stop >= start && stop <= b {
				r.Stops = append(r.Stops, TextStop{stop, float64(stop-start) * 8})
			}
		}
		runs = append(runs, r)
		start = b
	}
	return runs
}

// FuzzTextOffsets checks the UTF-8 to UTF-16 conversions a platform text API
// goes through: every byte offset they produce is in range and on a rune
// boundary, the two directions agree, and the line and rectangle queries
// built on them stay inside the layout they were given.
func FuzzTextOffsets(f *testing.F) {
	f.Add("", 0, 0, uint64(0))
	f.Add("hello", 2, 3, uint64(0))
	f.Add("aé😀b", 3, 1, uint64(0b10))
	f.Add("héllo wörld", 7, -4, uint64(1<<5))
	f.Add("👨‍👩‍👧 family", 4, 9, uint64(0xff))
	f.Add("\xff\xfe broken \xc3", -3, 100, uint64(0x5555))
	f.Add("line one\nline two\n", 12, 5, uint64(1<<8|1<<16))
	f.Fuzz(func(t *testing.T, s string, u, length int, breaks uint64) {
		starts := runeStarts(s)
		isStart := map[int]bool{}
		for _, b := range starts {
			isStart[b] = true
		}
		// A range loop and []rune agree on what a rune is, invalid bytes
		// included, so utf16.Encode is a reference for the length.
		units := len(utf16.Encode([]rune(s)))
		if got := axUTF16Len(s); got != units {
			t.Fatalf("axUTF16Len(%q) = %d, want %d", s, got, units)
		}
		for _, b := range starts {
			if got := axByteAt(s, axUTF16At(s, b)); got != b {
				t.Fatalf("rune boundary %d of %q went to UTF-16 and back as %d", b, s, got)
			}
		}
		b := axByteAt(s, u)
		if b < 0 || b > len(s) || !isStart[b] {
			t.Fatalf("axByteAt(%q, %d) = %d, not a rune boundary in range", s, u, b)
		}
		if back := axUTF16At(s, b); back > max(u, 0) {
			t.Fatalf("axByteAt(%q, %d) = %d moved forward to unit %d", s, u, b, back)
		} else if u >= 0 && u <= units && back < u-1 {
			// Only the second half of a surrogate pair may snap back, and
			// then by one unit.
			t.Fatalf("axByteAt(%q, %d) = %d snapped back to unit %d", s, u, b, back)
		}
		if b2 := axByteAt(s, u+1); u < math.MaxInt && b2 < b {
			t.Fatalf("axByteAt is not monotonic: unit %d at byte %d, unit %d at byte %d", u, b, u+1, b2)
		}

		n := Node{Value: s, Runs: layOut(s, breaks)}
		start, end := ByteRange(n, u, length)
		if start > end || !isStart[start] || !isStart[end] {
			t.Fatalf("ByteRange(%q, %d, %d) = %d..%d, not an ordered pair of rune boundaries", s, u, length, start, end)
		}
		got := StringForRange(n, u, length)
		if utf8.ValidString(s) && !utf8.ValidString(got) {
			t.Fatalf("StringForRange(%q, %d, %d) = %q cut a rune", s, u, length, got)
		}

		lines := max(len(n.Runs), 1)
		line := LineForIndex(n, u)
		if line < 0 || line >= lines {
			t.Fatalf("LineForIndex(%d) = %d with %d lines", u, line, lines)
		}
		if len(n.Runs) > 0 {
			if r := n.Runs[line]; b > r.End || (line > 0 && b <= r.Start) {
				t.Fatalf("byte %d reported on line %d, which covers %d..%d", b, line, r.Start, r.End)
			}
		}
		total := 0
		for i := range lines {
			loc, ln, ok := RangeForLine(n, i)
			if !ok || loc != total || ln < 0 {
				t.Fatalf("RangeForLine(%d) = %d+%d, %v; want a range starting at unit %d", i, loc, ln, ok, total)
			}
			total += ln
		}
		if total != units {
			t.Fatalf("lines of %q cover %d units, want all %d", s, total, units)
		}

		n.SelStart, n.SelEnd = min(start, end), max(start, end)
		if loc, ln := Selection(n); loc < 0 || ln < 0 || loc+ln > units {
			t.Fatalf("Selection of bytes %d..%d = %d+%d, outside %d units", start, end, loc, ln, units)
		}
		if sel := Selected(n); sel != s[start:end] {
			t.Fatalf("Selected = %q, want the selected bytes %q", sel, s[start:end])
		}
		if il := InsertionLine(n); il < 0 || il >= lines {
			t.Fatalf("InsertionLine = %d with %d lines", il, lines)
		}

		full := Rct(Pt(-1, -1), Sz(1000, 1000))
		rect := RectForRange(SemNode{Node: n, Full: full}, u, length)
		if len(n.Runs) == 0 {
			if rect != full {
				t.Fatalf("RectForRange without runs = %+v, want the whole field", rect)
			}
			return
		}
		if rect.Size.W < 1 || rect.Size.H != 10 || rect.Origin.X < 0 || rect.Origin.X+rect.Size.W > float64(len(s))*8+1 {
			t.Fatalf("RectForRange(%d, %d) = %+v, outside the laid-out text", u, length, rect)
		}
	})
}
