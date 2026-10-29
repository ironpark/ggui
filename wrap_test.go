package ggui

import (
	"slices"
	"testing"
	"unicode/utf8"
)

func TestWrapSpansCoverTheTextWithinWidth(t *testing.T) {
	t.Parallel()
	face := fallbackFont().face(14, 0)
	for _, s := range []string{
		"", "one", "the quick brown fox jumps over the lazy dog",
		"line one\nline two\n\nafter blank", "  leading and   inner   spaces",
		"averyveryveryverylongwordwithoutanyspacesinit then short", "한글 텍스트도 잘 감싸야 합니다 한글 텍스트도",
	} {
		for _, w := range []float64{0, 40, 90, 1000} {
			spans := wrapSpans(s, face, w)
			if len(spans) == 0 {
				t.Errorf("%q at %v: no lines", s, w)
				continue
			}
			prev := 0
			for i, sp := range spans {
				if sp.start < prev || sp.end < sp.start || sp.end > len(s) {
					t.Fatalf("%q at %v: span %d = %+v out of order", s, w, i, sp)
				}
				for _, r := range s[prev:sp.start] {
					if r != ' ' && r != '\t' && r != '\n' {
						t.Errorf("%q at %v: %q skipped between lines", s, w, r)
					}
				}
				line := s[sp.start:sp.end]
				if w > 0 && utf8.RuneCountInString(line) > 1 && lineWidth(line, face) > w {
					t.Errorf("%q at %v: line %q is too wide", s, w, line)
				}
				prev = sp.end
			}
		}
	}
	if got := len(wrapSpans("a\nb\n", face, 0)); got != 3 {
		t.Fatalf("trailing newline gives %d lines, want 3", got)
	}
	if got := len(wrapSpans("the quick brown fox", face, 1000)); got != 1 {
		t.Fatalf("wide line split into %d", got)
	}
}

func TestLineOf(t *testing.T) {
	t.Parallel()
	spans := []lineSpan{{0, 3}, {4, 7}, {8, 8}}
	for pos, want := range map[int]int{0: 0, 3: 0, 4: 1, 7: 1, 8: 2, 20: 2} {
		if got := lineOf(spans, pos); got != want {
			t.Errorf("lineOf(%d) = %d, want %d", pos, got, want)
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
		for _, sp := range wrapParagraphBy(c.s, width, c.maxW) {
			got = append(got, c.s[sp.start:sp.end])
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %q at %v wraps to %q, want %q", c.name, c.s, c.maxW, got, c.want)
		}
	}
}
