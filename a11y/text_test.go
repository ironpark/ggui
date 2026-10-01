package a11y

import "testing"

func TestAXUTF16OffsetsCountSurrogatePairs(t *testing.T) {
	// A platform text API counts in UTF-16 and ggui stores UTF-8, and the
	// two disagree the moment anything outside the basic plane appears.
	s := "aé😀b" // 1 + 2 + 4 + 1 bytes, 1 + 1 + 2 + 1 code units
	if got := axUTF16Len(s); got != 5 {
		t.Errorf("axUTF16Len = %d, want 5", got)
	}
	for _, c := range []struct{ u, b int }{{0, 0}, {1, 1}, {2, 3}, {4, 7}, {5, 8}, {9, 8}, {-1, 0}} {
		if got := axByteAt(s, c.u); got != c.b {
			t.Errorf("axByteAt(%d) = %d, want %d", c.u, got, c.b)
		}
	}
	// Halfway into a surrogate pair is not a place a caret can be, so it
	// resolves to the start of the rune.
	if got := axByteAt(s, 3); got != 3 {
		t.Errorf("axByteAt(3) = %d, want the start of the emoji (3)", got)
	}
	for _, c := range []struct{ b, u int }{{0, 0}, {1, 1}, {3, 2}, {7, 4}, {8, 5}} {
		if got := axUTF16At(s, c.b); got != c.u {
			t.Errorf("axUTF16At(%d) = %d, want %d", c.b, got, c.u)
		}
	}
	n := Node{Value: s, SelStart: 1, SelEnd: 7}
	if loc, length := Selection(n); loc != 1 || length != 3 {
		t.Errorf("selection = %d+%d, want 1+3 in UTF-16", loc, length)
	}
	if got := Selected(n); got != "é😀" {
		t.Errorf("selected text = %q, want %q", got, "é😀")
	}
	if a, b := ByteRange(n, 1, 3); a != 1 || b != 7 {
		t.Errorf("byte range = %d..%d, want 1..7", a, b)
	}
}

// wrapped is "héllo wörld" laid out on two lines, "héllo " then "wörld",
// with a stop every 10 units at each rune boundary.
func wrapped() SemNode {
	stops := func(from float64, bytes ...int) []TextStop {
		out := make([]TextStop, len(bytes))
		for i, b := range bytes {
			out[i] = TextStop{b, from + float64(i)*10}
		}
		return out
	}
	return SemNode{
		Node: Node{Role: RoleTextField, Value: "héllo wörld", Runs: []TextRun{
			{Start: 0, End: 7, Rect: Rct(Pt(10, 20), Sz(60, 16)), Stops: stops(0, 0, 1, 3, 4, 5, 6, 7)},
			{Start: 7, End: 13, Rect: Rct(Pt(10, 36), Sz(50, 16)), Stops: stops(0, 7, 8, 10, 11, 12, 13)},
		}},
		Full: Rct(Pt(0, 0), Sz(200, 60)),
	}
}

func TestLinesFollowTheFrozenRuns(t *testing.T) {
	t.Parallel()
	n := wrapped().Node
	if got := CharCount(n); got != 11 {
		t.Errorf("CharCount = %d, want 11 UTF-16 units", got)
	}
	// The boundary between two lines belongs to the first: a caret there
	// sits at the end of "héllo ".
	for _, c := range []struct{ u, line int }{{0, 0}, {5, 0}, {6, 0}, {7, 1}, {11, 1}, {99, 1}} {
		if got := LineForIndex(n, c.u); got != c.line {
			t.Errorf("LineForIndex(%d) = %d, want %d", c.u, got, c.line)
		}
	}
	for _, c := range []struct {
		line, loc, length int
		ok                bool
	}{{0, 0, 6, true}, {1, 6, 5, true}, {2, 0, 0, false}, {-1, 0, 0, false}} {
		loc, length, ok := RangeForLine(n, c.line)
		if loc != c.loc || length != c.length || ok != c.ok {
			t.Errorf("RangeForLine(%d) = %d+%d, %v; want %d+%d, %v", c.line, loc, length, ok, c.loc, c.length, c.ok)
		}
	}
	n.SelStart, n.SelEnd = 0, 10 // before the "r" of wörld
	if got := InsertionLine(n); got != 1 {
		t.Errorf("InsertionLine with the caret in wörld = %d, want 1", got)
	}
	n.SelEnd = 3
	if got := InsertionLine(n); got != 0 {
		t.Errorf("InsertionLine with the caret in héllo = %d, want 0", got)
	}
}

func TestAFieldWithoutLayoutIsOneLine(t *testing.T) {
	t.Parallel()
	n := Node{Value: "a😀"}
	if got := LineForIndex(n, 2); got != 0 {
		t.Errorf("LineForIndex = %d, want 0", got)
	}
	if loc, length, ok := RangeForLine(n, 0); loc != 0 || length != 3 || !ok {
		t.Errorf("RangeForLine(0) = %d+%d, %v; want the whole field 0+3", loc, length, ok)
	}
	if _, _, ok := RangeForLine(n, 1); ok {
		t.Error("RangeForLine(1) found a second line in an unwrapped field")
	}
	full := Rct(Pt(5, 5), Sz(80, 20))
	if got := RectForRange(SemNode{Node: n, Full: full}, 0, 1); got != full {
		t.Errorf("RectForRange without runs = %+v, want the whole field %+v", got, full)
	}
}

func TestRectForRangeCoversTheFirstLineOfTheRange(t *testing.T) {
	t.Parallel()
	n := wrapped()
	cases := []struct {
		name        string
		loc, length int
		want        Rect
	}{
		{"within a line", 1, 2, Rct(Pt(20, 20), Sz(20, 16))},             // "él"
		{"across the wrap", 4, 4, Rct(Pt(50, 20), Sz(20, 16))},           // "o " of the first line only
		{"a caret on the second line", 7, 0, Rct(Pt(20, 36), Sz(1, 16))}, // one unit wide
		{"at the very end", 11, 5, Rct(Pt(60, 36), Sz(1, 16))},
	}
	for _, c := range cases {
		if got := RectForRange(n, c.loc, c.length); got != c.want {
			t.Errorf("%s: RectForRange(%d, %d) = %+v, want %+v", c.name, c.loc, c.length, got, c.want)
		}
	}
}

// TestSetWantsDetailIsSeenByWantsDetail changes process-wide state, so it
// must not run in parallel.
func TestSetWantsDetailIsSeenByWantsDetail(t *testing.T) {
	prev := WantsDetail()
	t.Cleanup(func() { SetWantsDetail(prev) })
	SetWantsDetail(true)
	if !WantsDetail() {
		t.Error("WantsDetail = false after SetWantsDetail(true)")
	}
	SetWantsDetail(false)
	if WantsDetail() {
		t.Error("WantsDetail = true after SetWantsDetail(false)")
	}
}
