package textedit

import (
	"unicode"
	"unicode/utf8"
)

// IndexFold returns where needle is first found in s at or after byte from,
// ignoring case, as the byte range [lo, hi) of s; lo is -1 when it is not
// found. The range is s's own, which may differ in length from needle.
func IndexFold(s, needle string, from int) (lo, hi int) {
	if needle == "" {
		return -1, -1
	}
	// Most of s is passed over on its first byte: an ASCII one that is not
	// the needle's first rune, in either case, is rejected without decoding.
	p0, _ := utf8.DecodeRuneInString(needle)
	l0 := unicode.ToLower(p0)
	for i := max(from, 0); i < len(s); {
		if c := rune(s[i]); c < utf8.RuneSelf && c != p0 && lowerASCII(c) != l0 {
			i++
			continue
		}
		if j, ok := prefixFold(s[i:], needle); ok {
			return i, i + j
		}
		_, w := utf8.DecodeRuneInString(s[i:])
		i += w
	}
	return -1, -1
}

// prefixFold reports whether s begins with prefix, ignoring case, and how
// many bytes of s that takes.
func prefixFold(s, prefix string) (int, bool) {
	i := 0
	for _, p := range prefix {
		if i >= len(s) {
			return 0, false
		}
		r, w := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r, w = utf8.DecodeRuneInString(s[i:])
		}
		if r != p && unicode.ToLower(r) != unicode.ToLower(p) {
			return 0, false
		}
		i += w
	}
	return i, true
}

// lowerASCII is unicode.ToLower for an ASCII rune.
func lowerASCII(r rune) rune {
	if 'A' <= r && r <= 'Z' {
		return r + 'a' - 'A'
	}
	return r
}
