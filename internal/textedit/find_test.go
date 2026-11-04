package textedit

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestIndexFoldIgnoresCase(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		s, needle string
		from      int
		lo, hi    int
	}{
		{"Hello World", "world", 0, 6, 11},
		{"ÄBC äbc", "äbc", 1, 5, 9},
		{"abc", "x", 0, -1, -1},
		{"abc", "", 0, -1, -1},
		{"Kelvin", "kelvin", 0, 0, 8}, // the Kelvin sign lowers to k
		{"abc", "abc", -3, 0, 3},
		{"ab", "abc", 0, -1, -1},
	} {
		if lo, hi := IndexFold(c.s, c.needle, c.from); lo != c.lo || hi != c.hi {
			t.Errorf("IndexFold(%q, %q, %d) = %d, %d; want %d, %d", c.s, c.needle, c.from, lo, hi, c.lo, c.hi)
		}
	}
}

// A page's find over a document with no match: every position tried.
func BenchmarkIndexFold(b *testing.B) {
	b.SetBytes(int64(len(benchDoc)))
	b.ReportAllocs()
	for b.Loop() {
		IndexFold(benchDoc, "Brown Fax", 0)
	}
}

func BenchmarkIndexFoldASCII(b *testing.B) {
	doc := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 16<<10/45)
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		IndexFold(doc, "Lazy Cat", 0)
	}
}

// indexFoldSlow is IndexFold as it was before its ASCII fast path: every
// position decoded and compared rune by rune.
func indexFoldSlow(s, needle string, from int) (int, int) {
	if needle == "" {
		return -1, -1
	}
	for i := max(from, 0); i < len(s); {
	next:
		for j, k := i, 0; ; {
			if k == len(needle) {
				return i, j
			}
			if j >= len(s) {
				break next
			}
			r, w := utf8.DecodeRuneInString(s[j:])
			p, pw := utf8.DecodeRuneInString(needle[k:])
			if r != p && unicode.ToLower(r) != unicode.ToLower(p) {
				break next
			}
			j, k = j+w, k+pw
		}
		_, w := utf8.DecodeRuneInString(s[i:])
		i += w
	}
	return -1, -1
}

func FuzzIndexFold(f *testing.F) {
	f.Add("Hello World", "WORLD", 0)
	f.Add("Kelvin K k", "k", 1)
	f.Add("ÄBC äbc \xff", "Äb", 2)
	f.Fuzz(func(t *testing.T, s, needle string, from int) {
		lo, hi := IndexFold(s, needle, from)
		wlo, whi := indexFoldSlow(s, needle, from)
		if lo != wlo || hi != whi {
			t.Fatalf("IndexFold(%q, %q, %d) = %d, %d; the plain search finds %d, %d", s, needle, from, lo, hi, wlo, whi)
		}
	})
}
