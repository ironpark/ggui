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
