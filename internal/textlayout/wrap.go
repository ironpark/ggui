// Package textlayout measures and wraps text set in a face: the width of a
// line and of each of its prefixes, lines broken where UAX #14 allows and
// between grapheme clusters when a word is wider than the line, and a line
// cut short with an ellipsis. A face that holds an emoji font (see
// fontsys.EmojiFace) is measured as it draws, each emoji in its own font.
package textlayout

import (
	"iter"
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
	// Text with no emoji in it is one run, measured without the iterator,
	// whose closure and sum would go to the heap: wrapping measures a
	// line once a word.
	if f := fontsys.PlainFace(face, s); f != nil {
		return text.AdvanceAt(s, len(s), f)
	}
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
	for start, line := range Lines(s) {
		for _, sp := range wrapParagraph(line, func(a, b int) float64 { return width(start+a, start+b) }, maxW) {
			out = append(out, Span{start + sp.Start, start + sp.End})
		}
	}
	return out
}

// Lines yields the hard lines of s, each without its line break, and the
// offset in s where it starts: one empty line for an empty s, and an empty
// last line after a line break that ends s, as a caret can stand there.
func Lines(s string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		for start := 0; ; {
			end := len(s)
			if i := strings.IndexByte(s[start:], '\n'); i >= 0 {
				end = start + i
			}
			if !yield(start, s[start:end]) || end == len(s) {
				return
			}
			start = end + 1
		}
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
		// The line with this word on it, measured once: again only when
		// the word starts a new line.
		w := width(lineStart, wend)
		if lineEnd > lineStart && w > maxW {
			// Does not fit after what is on the line: break before it.
			lines = append(lines, Span{lineStart, lineEnd})
			lineStart, lineEnd = start, start
			w = width(lineStart, wend)
		}
		if w <= maxW {
			lineEnd = wend
			continue
		}
		// A word wider than the line, alone on it: break between grapheme clusters.
		lines, lineStart = breakWord(p, lineStart, wend, width, maxW, lines)
		lineEnd = wend
	}
	return append(lines, Span{lineStart, lineEnd})
}

// breakWord breaks p[lineStart:wend], a word too wide for a line, between
// grapheme clusters, appending each full line to lines, and returns them
// and where the last, which may take more words, starts. A line ends
// before the first cluster that takes it past maxW, and holds one cluster
// at least.
//
// Line widths grow with the clusters on them, so where a line ends is
// found by doubling the clusters tried and then halving between the last
// two tries: a few measures a line, where trying each cluster in turn
// would measure the line once per cluster on it, which for a long URL or
// a pasted run of text is thousands of measures of a line's length.
func breakWord(p string, lineStart, wend int, width func(a, b int) float64, maxW float64, lines []Span) ([]Span, int) {
	// The word's clusters alone, as offsets from its start, base: the
	// whole paragraph's would cost each long word in it the paragraph.
	// They are kept, by the word, for the next wrap of it at another
	// width.
	base := lineStart
	bounds := textedit.Bounds(p[base:wend])
	at := func(k int) int { return base + bounds[k] }
	// A line from lineStart ending at cluster k is too wide when the
	// cluster after it would not fit on it as well: it ends there.
	too := func(k int) bool { return width(lineStart, at(k+1)) > maxW }
	end := len(bounds) - 1 // the lines end before cluster end, the word's end
	for k := 1; k < end; {
		// Every candidate before k fits; find the first that does not.
		first, step := end, 1
		for k < end {
			probe := min(k+step-1, end-1)
			if too(probe) {
				first = k + sort.Search(probe-k, func(i int) bool { return too(k + i) })
				break
			}
			k, step = probe+1, step*2
		}
		if first == end {
			break
		}
		lines = append(lines, Span{lineStart, at(first)})
		lineStart, k = at(first), first+1
	}
	return lines, lineStart
}

// BreakSpans splits s at its line breaks only.
func BreakSpans(s string) []Span {
	spans := make([]Span, 0, strings.Count(s, "\n")+1)
	for start, line := range Lines(s) {
		spans = append(spans, Span{start, start + len(line)})
	}
	return spans
}

// LineOf returns the index of the span that holds byte offset pos: the
// last one starting at or before it.
func LineOf(spans []Span, pos int) int {
	i := sort.Search(len(spans), func(i int) bool { return spans[i].Start > pos })
	return max(i-1, 0)
}
