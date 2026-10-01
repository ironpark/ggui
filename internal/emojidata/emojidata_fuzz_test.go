package emojidata

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// inRangesLinear is the reference for inRanges: a plain scan, which needs
// neither sorted nor disjoint ranges.
func inRangesLinear(r rune, ranges [][2]rune) bool {
	for _, p := range ranges {
		if p[0] <= r && r <= p[1] {
			return true
		}
	}
	return false
}

// isClusterRef restates IsCluster's documented rules over the linear scan.
func isClusterRef(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	switch {
	case strings.ContainsRune(s, '\ufe0e'):
		return false
	case strings.ContainsRune(s, '\u20e3') && strings.ContainsRune("#*0123456789", r):
		return true
	case strings.ContainsRune(s, '\ufe0f'):
		return inRangesLinear(r, codepoints[:])
	}
	return inRangesLinear(r, presentation[:])
}

// mayHoldRef is the reference for MayHold on valid UTF-8: whether any rune
// is at or above U+2000, the first code point encoded with lead byte 0xE2.
func mayHoldRef(s string) bool {
	for _, r := range s {
		if r >= 0x2000 {
			return true
		}
	}
	return false
}

func TestTablesAreSortedAndDisjoint(t *testing.T) {
	t.Parallel()
	for name, table := range map[string][][2]rune{"codepoints": codepoints[:], "presentation": presentation[:]} {
		for i, p := range table {
			if p[0] > p[1] {
				t.Errorf("%s[%d] = %#x..%#x runs backwards", name, i, p[0], p[1])
			}
			if i > 0 && table[i-1][1] >= p[0] {
				t.Errorf("%s[%d] = %#x..%#x overlaps or precedes %#x..%#x; binary search needs sorted disjoint ranges",
					name, i, p[0], p[1], table[i-1][0], table[i-1][1])
			}
		}
	}
}

// Every code point that presents as emoji by default is an emoji code point,
// and encodes with a lead byte MayHold looks for.
func TestPresentationIsWithinCodepointsAndMayHold(t *testing.T) {
	t.Parallel()
	for _, p := range presentation {
		for _, r := range []rune{p[0], p[1]} {
			if !inRangesLinear(r, codepoints[:]) {
				t.Errorf("%U presents as emoji but is not an emoji code point", r)
			}
			if !MayHold(string(r)) {
				t.Errorf("MayHold(%q) = false for an emoji-presentation code point", r)
			}
		}
	}
}

// Binary search agrees with the linear scan at and around every range edge,
// where an off-by-one would show.
func TestInRangesAgreesWithLinearScanAtEdges(t *testing.T) {
	t.Parallel()
	for name, table := range map[string][][2]rune{"codepoints": codepoints[:], "presentation": presentation[:]} {
		for _, p := range table {
			for _, r := range []rune{p[0] - 1, p[0], p[0] + 1, p[1] - 1, p[1], p[1] + 1} {
				if got, want := inRanges(r, table), inRangesLinear(r, table); got != want {
					t.Errorf("inRanges(%U, %s) = %v, want %v", r, name, got, want)
				}
			}
		}
	}
}

func TestIsClusterPresentationSelectors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		s    string
		want bool
	}{
		{"", false},
		{"#\u20e3", true},              // keycap without the variation selector
		{"*\ufe0f\u20e3", true},        // keycap
		{"a\u20e3", false},             // a keycap needs # * or a digit
		{"1\ufe0e\u20e3", false},       // text presentation wins over the keycap
		{"❤", false},                   // heavy heart: text by default
		{"❤\ufe0f", true},              // and emoji when asked
		{"a️", false},                  // the selector makes no emoji of a letter
		{"\U0001F44D\U0001F3FD", true}, // skin tone modifier
		{"\U0001F1F0\U0001F1F7", true}, // flag
		{"\U0001F468\u200d\U0001F469", true},
		{"\xff", false},
	} {
		if got := IsCluster(c.s); got != c.want {
			t.Errorf("IsCluster(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

// FuzzIsCluster checks that IsCluster and MayHold never panic, agree with
// their references, and that MayHold never rules out a cluster IsCluster
// accepts, which is what lets the renderer skip the cluster walk.
func FuzzIsCluster(f *testing.F) {
	for _, s := range []string{
		"", "a", "😀", "©", "©\ufe0f", "😀\ufe0e", "1\ufe0f\u20e3", "#\u20e3",
		"\U0001F1F0\U0001F1F7", "\U0001F468\u200d\U0001F469\u200d\U0001F467",
		"\U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F",
		"⌚", "™\ufe0f", "\xe2", "\xff\ufe0f", "한", "\u20e3",
	} {
		f.Add(s)
	}
	for _, p := range presentation[:8] {
		f.Add(string(p[0]))
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := IsCluster(s)
		if want := isClusterRef(s); got != want {
			t.Fatalf("IsCluster(%q) = %v, reference says %v", s, got, want)
		}
		may := MayHold(s)
		if want := mayHoldRef(s); utf8.ValidString(s) && may != want {
			t.Fatalf("MayHold(%q) = %v, reference says %v", s, may, want)
		}
		if got && !may {
			t.Fatalf("IsCluster(%q) = true but MayHold = false: the renderer would skip an emoji", s)
		}
		if IsCluster(s + "\ufe0e") {
			t.Fatalf("IsCluster(%q + U+FE0E) = true; the text selector must win", s)
		}
	})
}
