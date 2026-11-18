package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// sanitize makes text fit for a single line of a terminal: escape
// sequences and control characters out, tabs as spaces.
func sanitize(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if n := escapeLen(text[i:]); n > 0 {
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		i += size
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// escapeLen returns the length of the escape sequence s starts with, a
// CSI sequence such as a color, or 0.
func escapeLen(s string) int {
	if len(s) < 2 || s[0] != '\x1b' || s[1] != '[' {
		return 0
	}
	for i := 2; i < len(s); i++ {
		if c := s[i]; c >= 0x40 && c <= 0x7e {
			return i + 1
		}
	}
	return len(s)
}

// visibleWidth returns the columns s takes on a terminal, its escape
// sequences taking none.
func visibleWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if n := escapeLen(s[i:]); n > 0 {
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		w += runeWidth(r)
	}
	return w
}

// truncate cuts s to at most n columns, ending it with … when it was cut.
// Escape sequences are kept, and a reset follows a cut one so that the
// style does not run on.
func truncate(s string, n int) string {
	if visibleWidth(s) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	w, styled := 0, false
	for i := 0; i < len(s); {
		if l := escapeLen(s[i:]); l > 0 {
			b.WriteString(s[i : i+l])
			styled = true
			i += l
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runeWidth(r)
		if w+rw > n-1 {
			break
		}
		b.WriteString(s[i : i+size])
		w += rw
		i += size
	}
	b.WriteString("…")
	if styled {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// runeWidth is the columns r takes: two for the wide characters of East
// Asian scripts and most emoji, none for combining marks, one otherwise.
func runeWidth(r rune) int {
	switch {
	case unicode.Is(unicode.Mn, r), unicode.Is(unicode.Me, r), r == 0x200b:
		return 0
	case r >= 0x1100 && r <= 0x115f,
		r >= 0x2e80 && r <= 0xa4cf && r != 0x303f,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe4f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1faff,
		r >= 0x20000 && r <= 0x3fffd:
		return 2
	}
	return 1
}
