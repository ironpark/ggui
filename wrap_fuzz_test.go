package ggui

import (
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/ironpark/ggfx/text/v2"
	"github.com/ironpark/ggui/internal/textedit"
	"golang.org/x/image/font/gofont/goregular"
)

// wrapFace is Go Regular at 14px with no fallbacks, so wrapping measures
// the same on every machine.
var wrapFace = sync.OnceValue(func() text.Face {
	return &text.GoTextFace{Source: MustFont(goregular.TTF).src, Size: 14}
})

// graphemes counts the grapheme clusters in s.
func graphemes(s string) int {
	n := 0
	for i := 0; i < len(s); i = textedit.NextGrapheme(s, i) {
		n++
	}
	return n
}

// onGrapheme reports whether byte offset i of s is a grapheme boundary.
func onGrapheme(s string, i int) bool {
	return i == 0 || i == len(s) || textedit.NextGrapheme(s, textedit.PrevGrapheme(s, i)) == i
}

// breaksInsideWord reports whether a line break at byte offset i of s
// cuts a grapheme cluster within a word. A break beside a space, a tab or
// a line feed is where wrapping breaks, even when a combining mark after
// the space or the CR of a CRLF would otherwise have joined across it.
func breaksInsideWord(s string, i int) bool {
	if i == 0 || i == len(s) || strings.IndexByte(" \t\n", s[i-1]) >= 0 || strings.IndexByte(" \t\n", s[i]) >= 0 {
		return false
	}
	return !onGrapheme(s, i)
}

// withoutSpace drops the runes unicode.IsSpace reports, reading s as
// strings.Fields does, so invalid bytes are kept.
func withoutSpace(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError || !unicode.IsSpace(r) {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// wrapSeeds are texts worth wrapping: spaces in odd places, words wider
// than a line, scripts without spaces, emoji and joined sequences.
var wrapSeeds = []string{
	"", " ", "\n", "a\nb\n", "one", "the quick brown fox jumps over the lazy dog",
	"  leading and   inner   spaces  ", "tabs\tbetween\twords", "line one\nline two\n\nafter blank",
	"averyveryveryverylongwordwithoutanyspacesinit then short", "한글 텍스트도 잘 감싸야 합니다",
	"日本語のテキストは空白なしで折り返す", "ééé café", "👩🏽\u200d💻👨\u200d👩\u200d👧\u200d👦🇰🇷 flags",
	"\xff\xfe bad bytes \xc3", "nbsp joined em space", "a \u0301b", "\r\n windows",
}

func FuzzWrapSpans(f *testing.F) {
	for _, s := range wrapSeeds {
		for _, w := range []float64{0, 1, 20, 60, 1000} {
			f.Add(s, w)
		}
	}
	f.Add("x y", math.NaN())
	f.Add("x y", math.Inf(1))
	f.Add("x y", -5.0)
	f.Fuzz(func(t *testing.T, s string, maxW float64) {
		if len(s) > 512 {
			return // measuring is quadratic in a word's length
		}
		face := wrapFace()
		spans := wrapSpans(s, face, maxW)
		if len(spans) < strings.Count(s, "\n")+1 {
			t.Fatalf("%q at %v: %d lines for %d hard breaks", s, maxW, len(spans), strings.Count(s, "\n"))
		}
		prev := 0
		for i, sp := range spans {
			if sp.start < prev || sp.end < sp.start || sp.end > len(s) {
				t.Fatalf("%q at %v: span %d = %+v is out of order after %d", s, maxW, i, sp, prev)
			}
			// Between lines lie only the spaces a soft break fell on and
			// the hard breaks.
			if gap := strings.Trim(s[prev:sp.start], " \t\n"); gap != "" {
				t.Fatalf("%q at %v: %q was dropped before line %d", s, maxW, gap, i)
			}
			line := s[sp.start:sp.end]
			if strings.Contains(line, "\n") {
				t.Fatalf("%q at %v: line %d = %q holds a hard break", s, maxW, i, line)
			}
			// Invalid bytes make no clusters to keep whole.
			if utf8.ValidString(s) && (!utf8.ValidString(line) || breaksInsideWord(s, sp.start) || breaksInsideWord(s, sp.end)) {
				t.Fatalf("%q at %v: line %d = %q cuts a grapheme", s, maxW, i, line)
			}
			if maxW > 0 && graphemes(line) > 1 && lineWidth(line, face) > maxW {
				t.Fatalf("%q at %v: line %d = %q is %v wide", s, maxW, i, line, lineWidth(line, face))
			}
			if got := lineOf(spans, sp.start); got != i {
				t.Fatalf("%q at %v: lineOf(%d) = %d, want %d", s, maxW, sp.start, got, i)
			}
			prev = sp.end
		}
		if gap := strings.Trim(s[prev:], " \t\n"); gap != "" {
			t.Fatalf("%q at %v: %q was dropped after the last line", s, maxW, gap)
		}
		// With no width to wrap to, every paragraph is one line as it is.
		if !(maxW > 0) || math.IsInf(maxW, 1) {
			if want := strings.Count(s, "\n") + 1; len(spans) != want {
				t.Fatalf("%q at %v: %d lines, want one per paragraph (%d)", s, maxW, len(spans), want)
			}
		}
	})
}

func FuzzWrapText(f *testing.F) {
	for _, s := range wrapSeeds {
		for _, w := range []float64{0, 1, 20, 60, 1000} {
			f.Add(s, w)
		}
	}
	f.Fuzz(func(t *testing.T, s string, maxW float64) {
		if len(s) > 512 {
			return
		}
		face := wrapFace()
		lines := wrapText(s, face, maxW)
		if len(lines) < strings.Count(s, "\n")+1 {
			t.Fatalf("%q at %v: %d lines for %d hard breaks", s, maxW, len(lines), strings.Count(s, "\n"))
		}
		for i, line := range lines {
			if strings.Contains(line, "\n") {
				t.Fatalf("%q at %v: line %d = %q holds a hard break", s, maxW, i, line)
			}
			if maxW > 0 && graphemes(line) > 1 && lineWidth(line, face) > maxW {
				t.Fatalf("%q at %v: line %d = %q is %v wide", s, maxW, i, line, lineWidth(line, face))
			}
		}
		// The lines rejoin to the text: only the spaces breaks fell on
		// differ. Each line is stripped on its own, since bytes that are
		// invalid apart may join into a space.
		var joined strings.Builder
		for _, line := range lines {
			joined.WriteString(withoutSpace(line))
		}
		if got, want := joined.String(), withoutSpace(s); got != want {
			t.Fatalf("%q at %v: lines %q hold %q, want %q", s, maxW, lines, got, want)
		}
	})
}

func FuzzTextRunsCoverTheTextByGrapheme(f *testing.F) {
	for _, s := range wrapSeeds {
		f.Add(s)
	}
	f.Add("a👍b👍🏽c1\ufe0f\u20e3❤\ufe0f")
	f.Add("🏳\ufe0f\u200d🌈x🫩")
	data, err := os.ReadFile("fonts/notoemoji/NotoColorEmoji.ttf")
	if err != nil {
		f.Fatal(err)
	}
	emoji := MustFont(data)
	f.Fuzz(func(t *testing.T, s string) {
		face := withEmoji(wrapFace(), 14, &emojiChoice{font: emoji})
		var joined strings.Builder
		var last text.Face
		for run, rf := range textRuns(s, face) {
			if run == "" && s != "" {
				t.Fatalf("%q: an empty run", s)
			}
			if last != nil && rf == last {
				t.Fatalf("%q: two runs in a row in one face; they should be one", s)
			}
			at := joined.Len()
			if !onGrapheme(s, at) {
				t.Fatalf("%q: a run starts at %d, inside a grapheme", s, at)
			}
			joined.WriteString(run)
			last = rf
		}
		if joined.String() != s {
			t.Fatalf("%q: runs join to %q", s, joined.String())
		}
	})
}
