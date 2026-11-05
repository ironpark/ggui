package textlayout

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/textedit"
)

func TestWrapSpansCoverTheTextWithinWidth(t *testing.T) {
	t.Parallel()
	face := fontsys.Builtin().Face(14, 0, nil)
	for _, s := range []string{
		"", "one", "the quick brown fox jumps over the lazy dog",
		"line one\nline two\n\nafter blank", "  leading and   inner   spaces",
		"averyveryveryverylongwordwithoutanyspacesinit then short", "한글 텍스트도 잘 감싸야 합니다 한글 텍스트도",
	} {
		for _, w := range []float64{0, 40, 90, 1000} {
			spans := WrapSpans(s, face, w)
			if len(spans) == 0 {
				t.Errorf("%q at %v: no lines", s, w)
				continue
			}
			prev := 0
			for i, sp := range spans {
				if sp.Start < prev || sp.End < sp.Start || sp.End > len(s) {
					t.Fatalf("%q at %v: span %d = %+v out of order", s, w, i, sp)
				}
				for _, r := range s[prev:sp.Start] {
					if r != ' ' && r != '\t' && r != '\n' {
						t.Errorf("%q at %v: %q skipped between lines", s, w, r)
					}
				}
				line := s[sp.Start:sp.End]
				if w > 0 && utf8.RuneCountInString(line) > 1 && Width(line, face) > w {
					t.Errorf("%q at %v: line %q is too wide", s, w, line)
				}
				prev = sp.End
			}
		}
	}
	if got := len(WrapSpans("a\nb\n", face, 0)); got != 3 {
		t.Fatalf("trailing newline gives %d lines, want 3", got)
	}
	if got := len(WrapSpans("the quick brown fox", face, 1000)); got != 1 {
		t.Fatalf("wide line split into %d", got)
	}
}

func TestLineOf(t *testing.T) {
	t.Parallel()
	spans := []Span{{0, 3}, {4, 7}, {8, 8}}
	for pos, want := range map[int]int{0: 0, 3: 0, 4: 1, 7: 1, 8: 2, 20: 2} {
		if got := LineOf(spans, pos); got != want {
			t.Errorf("LineOf(%d) = %d, want %d", pos, got, want)
		}
	}
}

// TestWrapBreaksWhereUAX14Allows wraps with every rune ten wide, so the
// breaks are the line-breaking rules' and not a font's.
func TestWrapBreaksWhereUAX14Allows(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		s    string
		maxW float64
		want []string
	}{
		{"after a hyphen", "well-known", 60, []string{"well-", "known"}},
		{"after a slash", "and/or", 50, []string{"and/", "or"}},
		{"between ideographs, not before a full stop", "日本語の文章。次の文", 40, []string{"日本語の", "文章。次", "の文"}},
		{"between Hangul words at spaces", "한글 텍스트", 40, []string{"한글", "텍스트"}},
	} {
		width := func(a, b int) float64 { return 10 * float64(utf8.RuneCountInString(c.s[a:b])) }
		var got []string
		for _, sp := range wrapParagraph(c.s, width, c.maxW) {
			got = append(got, c.s[sp.Start:sp.End])
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %q at %v wraps to %q, want %q", c.name, c.s, c.maxW, got, c.want)
		}
	}
}

// BenchmarkWrapParagraph wraps a paragraph to a new width each time, as a
// window being resized does.
func BenchmarkWrapParagraph(b *testing.B) {
	face := wrapFace()
	s := benchParagraph()
	w := 200.0
	for b.Loop() {
		w = 200 + math.Mod(w*1.618, 300)
		WrapSpans(s, face, w)
	}
}

// benchParagraph is about 2,000 bytes of words that do not repeat.
func benchParagraph() string {
	var b strings.Builder
	x := uint32(1)
	for b.Len() < 2000 {
		n := 2 + x%8
		for range n {
			x = x*1664525 + 1013904223
			b.WriteByte(byte('a' + x>>24%26))
		}
		b.WriteByte(' ')
	}
	return b.String()
}

// Wrapping keeps no reference to the text it wrapped: the caches it fills,
// its own line breaks' and the shaper's, copy what they key on. An editor
// wraps a new copy of its document at every keystroke, and a cache that
// kept the lines it was given would keep a whole document for each.
func TestWrapKeepsNoTextItWasGiven(t *testing.T) {
	var b strings.Builder
	for i := range 200 {
		fmt.Fprintf(&b, "line %d of a document, 한글 and é and a word long enough to wrap\n", i)
	}
	buf := []byte(b.String())
	freed := make(chan struct{})
	runtime.AddCleanup(&buf[0], func(ch chan struct{}) { close(ch) }, freed)
	doc := unsafe.String(&buf[0], len(buf))
	before := WrapSpans(doc, wrapFace(), 120)
	WrapSpans(doc, fontsys.Builtin().Face(14, 0, nil), 120)
	if again := WrapSpans(strings.Clone(doc), wrapFace(), 120); !slices.Equal(before, again) {
		t.Fatalf("the same text wrapped from the caches as %v, want %v", again, before)
	}
	// Stepping by grapheme remembers the line it last stepped in, and the
	// text it is in: step in another.
	textedit.NextGrapheme("elsewhere", 0)
	buf, doc = nil, ""
	for range 20 {
		runtime.GC()
		select {
		case <-freed:
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatal("the text wrapped is still reachable after it was dropped")
}

// breakWordByCluster is breakWord as it was, trying each cluster in turn:
// what breakWord must find, in fewer measures.
func breakWordByCluster(p string, lineStart, wend int, width func(a, b int) float64, maxW float64, lines []Span) ([]Span, int) {
	for j := lineStart; j < wend; {
		k := textedit.NextGrapheme(p, j)
		if k < wend && width(lineStart, textedit.NextGrapheme(p, k)) > maxW {
			lines = append(lines, Span{lineStart, k})
			lineStart, j = k, k
			continue
		}
		j = k
	}
	return lines, lineStart
}

// A word too wide for a line breaks where trying each cluster in turn
// breaks it: a URL, a run of Hangul (which breaks at spaces only), a run
// of emoji sequences, at widths from under a cluster to most of the word.
func TestBreakWordBreaksWhereEachClusterInTurnDoes(t *testing.T) {
	t.Parallel()
	face := fontsys.Builtin().Face(14, 0, nil)
	words := []string{
		"https://example.com/a/very/long/path/that/keeps/going?with=query&and=more#fragment",
		strings.Repeat("가나다라마바사아자차카타파하", 6),
		strings.Repeat("👩‍💻👨‍👩‍👧🇰🇷👍🏽", 8),
		strings.Repeat("éa", 40),
	}
	for _, w := range words {
		width := func(a, b int) float64 { return Width(w[a:b], face) }
		for _, maxW := range []float64{1, 9, 31, 80, 157, 400} {
			for _, start := range []int{0, textedit.NextGrapheme(w, 0)} {
				got, gotStart := breakWord(w, start, len(w), width, maxW, nil)
				want, wantStart := breakWordByCluster(w, start, len(w), width, maxW, nil)
				if !slices.Equal(got, want) || gotStart != wantStart {
					t.Errorf("breakWord(%q from %d, %v) = %v, %d; want %v, %d", w, start, maxW, got, gotStart, want, wantStart)
				}
			}
		}
	}
}

// BenchmarkWrapLongWord wraps a word of 5,000 characters, a pasted URL or
// a line of minified code, to a new width each time.
func BenchmarkWrapLongWord(b *testing.B) {
	face := wrapFace()
	s := strings.Repeat("abcdefghij", 500)
	w := 200.0
	for b.Loop() {
		w = 200 + math.Mod(w*1.618, 300)
		WrapSpans(s, face, w)
	}
}
