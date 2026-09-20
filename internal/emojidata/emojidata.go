// Package emojidata classifies text by Unicode's emoji properties, so the
// text renderer can hand an emoji grapheme to the emoji font and leave
// everything else to the text face.
package emojidata

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// MayHold is a cheap byte scan: every emoji-presentation code point,
// U+FE0F and U+20E3 encode with a lead byte of 0xE2 or above, so a string
// without one holds no emoji and needs no cluster walk.
func MayHold(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0xE2 {
			return true
		}
	}
	return false
}

// IsCluster reports whether the grapheme cluster s renders as an emoji: a
// keycap sequence, an emoji-capable code point with the emoji variation
// selector, or a code point that presents as emoji by default. U+FE0E asks
// for text presentation and wins.
func IsCluster(s string) bool {
	if strings.ContainsRune(s, '\ufe0e') {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	if strings.ContainsRune(s, '\u20e3') && (r == '#' || r == '*' || r >= '0' && r <= '9') {
		return true
	}
	if strings.ContainsRune(s, '\ufe0f') {
		return inRanges(r, codepoints[:])
	}
	return inRanges(r, presentation[:])
}

func inRanges(r rune, ranges [][2]rune) bool {
	_, ok := slices.BinarySearchFunc(ranges, r, func(p [2]rune, r rune) int {
		if p[1] < r {
			return -1
		}
		if p[0] > r {
			return 1
		}
		return 0
	})
	return ok
}
