// Package textlayout measures and wraps text set in a face: the width of a
// line and of each of its prefixes, lines broken where UAX #14 allows and
// between grapheme clusters when a word is wider than the line, and a line
// cut short with an ellipsis. A face that holds an emoji font (see
// fontsys.EmojiFace) is measured as it draws, each emoji in its own font.
package textlayout

import (
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/third_party/text"
)

// Width is how wide s draws in face.
func Width(s string, face text.Face) float64 {
	width := 0.0
	for run, f := range fontsys.TextRuns(s, face) {
		width += text.AdvanceAt(run, len(run), f)
	}
	return width
}

// PrefixWidths shapes s once and returns the width of s[:b] for any byte
// offset b on a rune boundary, which measuring each prefix would shape
// once per offset. An offset inside a glyph cluster, a ligature say, gets
// the share of the cluster's width that the runes before it have.
func PrefixWidths(s string, face text.Face) func(b int) float64 {
	type cluster struct {
		start, end int
		x, w       float64 // the width before the cluster, and its own
	}
	var cs []cluster
	var glyphs []text.LazyGlyph
	off := 0
	for run, f := range fontsys.TextRuns(s, face) {
		glyphs = text.AppendLazyGlyphs(glyphs[:0], run, f, nil)
		for _, g := range glyphs {
			cs = append(cs, cluster{start: off + g.StartIndexInBytes, end: off + g.EndIndexInBytes, w: g.AdvanceX})
		}
		off += len(run)
	}
	// Glyphs come in visual order and a cluster may have several; put
	// them in the order of the text, one entry a cluster.
	slices.SortStableFunc(cs, func(a, b cluster) int { return a.start - b.start })
	merged := cs[:0]
	x := 0.0
	for _, c := range cs {
		if n := len(merged); n > 0 && merged[n-1].start == c.start {
			merged[n-1].w += c.w
		} else {
			c.x = x
			merged = append(merged, c)
		}
		x += c.w
	}
	return func(b int) float64 {
		i := sort.Search(len(merged), func(i int) bool { return merged[i].end > b })
		if i == len(merged) {
			return x
		}
		c := merged[i]
		if b <= c.start {
			return c.x
		}
		return c.x + c.w*float64(utf8.RuneCountInString(s[c.start:b]))/float64(utf8.RuneCountInString(s[c.start:c.end]))
	}
}

// Wrap breaks s into lines no wider than maxW. Hard line breaks are
// kept; soft breaks fall where UAX #14 allows, or between grapheme clusters
// when a single word is wider than the line.
func Wrap(s string, face text.Face, maxW float64) []string {
	var out []string
	for para := range strings.SplitSeq(s, "\n") {
		out = append(out, wrapLine(para, face, maxW)...)
	}
	return out
}

// wrapLine wraps a line without hard breaks as wrapParagraph does, the
// spaces a break falls on left out.
func wrapLine(line string, face text.Face, maxW float64) []string {
	spans := wrapParagraph(line, func(a, b int) float64 { return Width(line[a:b], face) }, maxW)
	lines := make([]string, len(spans))
	for i, sp := range spans {
		lines[i] = line[sp.Start:sp.End]
	}
	return lines
}

// Ellipsis ends a line cut short.
const Ellipsis = "…"

// Ellipsize cuts s at a grapheme boundary and ends it with "…", so that it
// fits in maxW. When cut is false, s that already fits comes back whole.
// Spaces before the cut go, and the ellipsis is all that is left when not
// even one grapheme fits beside it.
func Ellipsize(s string, face text.Face, maxW float64, cut bool) string {
	if !cut && Width(s, face) <= maxW {
		return s
	}
	keep := EllipsizeBy(s, 0, len(s), PrefixWidths(s, face), maxW-Width(Ellipsis, face))
	return strings.TrimRight(s[:keep], " \t") + Ellipsis
}

// EllipsizeBy returns where s[a:b] ends so that it fits in room, at a
// grapheme boundary of s, width(x) measuring s[a:x].
func EllipsizeBy(s string, a, b int, width func(int) float64, room float64) int {
	keep := a
	for i := a; i < b; {
		end := textedit.NextGrapheme(s, i)
		if end > b || width(end) > room {
			break
		}
		keep, i = end, end
	}
	return keep
}

// Span is one drawn line of a wrapped string as byte offsets: the text
// drawn is s[start:end]. A hard line break or the spaces a soft break fell
// on sit between one span's end and the next one's start.
type Span struct{ Start, End int }

// WrapSpans is Wrap for an editor: the same breaks, as offsets into s
// rather than copies, so a caret position maps to a line and back.
func WrapSpans(s string, face text.Face, maxW float64) []Span {
	return WrapSpansBy(s, func(a, b int) float64 { return Width(s[a:b], face) }, maxW)
}

// WrapSpansBy is WrapSpans with width measuring s[a:b] however the caller
// draws it, so text in several styles wraps as one paragraph.
func WrapSpansBy(s string, width func(a, b int) float64, maxW float64) []Span {
	var out []Span
	start := 0
	for {
		stop := len(s)
		if i := strings.IndexByte(s[start:], '\n'); i >= 0 {
			stop = start + i
		}
		off := start
		for _, sp := range wrapParagraph(s[start:stop], func(a, b int) float64 { return width(off+a, off+b) }, maxW) {
			out = append(out, Span{start + sp.Start, start + sp.End})
		}
		if stop == len(s) {
			return out
		}
		start = stop + 1
	}
}

// wrapParagraph wraps text without hard breaks, width measuring p[a:b]:
// greedily at the break opportunities of UAX #14, and between grapheme
// clusters when a word alone is wider than maxW. The spaces before a soft
// break end no line: they lie between it and the next.
func wrapParagraph(p string, width func(a, b int) float64, maxW float64) []Span {
	if maxW <= 0 || width(0, len(p)) <= maxW {
		return []Span{{0, len(p)}}
	}
	var lines []Span
	lineStart, lineEnd := 0, 0 // the line being filled: its first byte and the end of its last word
	wstart := 0
	for _, brk := range lineBreaks(p) {
		wend := brk
		for wend > wstart && (p[wend-1] == ' ' || p[wend-1] == '\t') {
			wend--
		}
		start := wstart
		wstart = brk
		if wend == start {
			continue // spaces alone, before the first word
		}
		if lineEnd > lineStart && width(lineStart, wend) > maxW {
			// Does not fit after what is on the line: break before it.
			lines = append(lines, Span{lineStart, lineEnd})
			lineStart, lineEnd = start, start
		}
		if width(lineStart, wend) <= maxW {
			lineEnd = wend
			continue
		}
		// A word wider than the line, alone on it: break between grapheme clusters.
		for j := lineStart; j < wend; {
			k := textedit.NextGrapheme(p, j)
			if k < wend && width(lineStart, textedit.NextGrapheme(p, k)) > maxW {
				lines = append(lines, Span{lineStart, k})
				lineStart, j = k, k
				continue
			}
			j = k
		}
		lineEnd = wend
	}
	return append(lines, Span{lineStart, lineEnd})
}

// BreakSpans splits s at its line breaks only.
func BreakSpans(s string) []Span {
	spans := make([]Span, 0, strings.Count(s, "\n")+1)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			spans = append(spans, Span{start, i})
			start = i + 1
		}
	}
	return append(spans, Span{start, len(s)})
}

// LineOf returns the index of the span that holds byte offset pos: the
// last one starting at or before it.
func LineOf(spans []Span, pos int) int {
	i := sort.Search(len(spans), func(i int) bool { return spans[i].Start > pos })
	return max(i-1, 0)
}
