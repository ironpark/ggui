package ggui

import (
	"testing"
	"unicode/utf8"
)

func TestWrapSpansCoverTheTextWithinWidth(t *testing.T) {
	face := fallbackFont().face(14)
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
	spans := []lineSpan{{0, 3}, {4, 7}, {8, 8}}
	for pos, want := range map[int]int{0: 0, 3: 0, 4: 1, 7: 1, 8: 2, 20: 2} {
		if got := lineOf(spans, pos); got != want {
			t.Errorf("lineOf(%d) = %d, want %d", pos, got, want)
		}
	}
}
