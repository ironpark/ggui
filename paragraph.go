package ggui

import (
	"github.com/ironpark/ggfx/text/v2"
)

// paragraph is text in several styles laid out as one: it wraps across
// style boundaries, ends a cut line with "…", and orders each line's
// pieces by the bidi algorithm. Offsets are bytes of text, the styles'
// strings one after another.
type paragraph struct {
	text  string
	runs  []paraRun
	base  TextStyle // resolved; what an empty paragraph is measured in
	dir   TextDirection
	lang  string
	lines []paraLine
	size  Size
}

// paraRun is one style's stretch of the text, from the span at index span.
type paraRun struct {
	start, end int
	style      TextStyle // resolved
	span       int
}

// paraLine is one laid-out line. s is what it draws: text[start:end], then
// "…" when the line was cut.
type paraLine struct {
	start, end int
	s          string
	styles     []lineStyle
	pieces     []piece
	width      float64
	ascent     float64
	descent    float64
	baseline   float64 // below the paragraph's top
}

// style returns run i's resolved style, or the base style for -1.
func (p *paragraph) style(i int) TextStyle {
	if i < 0 {
		return p.base
	}
	return p.runs[i].style
}

// face returns run i's face at scale 1, or the base style's for -1.
func (p *paragraph) face(i int) text.Face {
	st := p.style(i)
	return st.Font.face(st.Size, st.Weight)
}

// width measures text[a:b], each run's part in its own face.
func (p *paragraph) width(a, b int) float64 {
	w := 0.0
	for i, r := range p.runs {
		lo, hi := max(a, r.start), min(b, r.end)
		if lo < hi {
			w += lineWidth(p.text[lo:hi], p.face(i))
		}
	}
	return w
}

// prefix returns the width of text[a:x] for any x in [a, b], shaping each
// run once.
func (p *paragraph) prefix(a, b int) func(x int) float64 {
	type part struct {
		lo, hi int
		before float64
		w      func(int) float64
	}
	var parts []part
	total := 0.0
	for i, r := range p.runs {
		lo, hi := max(a, r.start), min(b, r.end)
		if lo >= hi {
			continue
		}
		w := prefixWidths(p.text[lo:hi], p.face(i))
		parts = append(parts, part{lo, hi, total, w})
		total += w(hi - lo)
	}
	return func(x int) float64 {
		for _, pt := range parts {
			if x <= pt.hi {
				return pt.before + pt.w(max(x-pt.lo, 0))
			}
		}
		return total
	}
}

// runAt returns the run holding offset pos, the one before it when pos
// ends the text or falls in an empty span, or -1 when there is none.
func (p *paragraph) runAt(pos int) int {
	at := -1
	for i, r := range p.runs {
		if r.start > pos {
			break
		}
		at = i
		if pos < r.end {
			break
		}
	}
	return at
}

// layout breaks the text into lines no wider than maxW, as Text does: at
// spaces when wrap is set, else at hard breaks only; at most maxLines when
// that is positive, the last cut with "…" when text goes on past it; and
// with ellipsis every line too wide cut to fit.
func (p *paragraph) layout(maxW float64, wrap bool, maxLines int, ellipsis bool) {
	spans := wrapSpansBy(p.text, p.width, pick(wrap, maxW, 0))
	truncated := maxLines > 0 && len(spans) > maxLines
	if truncated {
		spans = spans[:maxLines]
	}
	p.lines = p.lines[:0]
	for i, sp := range spans {
		ln := paraLine{start: sp.start, end: sp.end}
		ell, cut := -1, truncated && i == len(spans)-1 || ellipsis && p.width(sp.start, sp.end) > maxW
		if cut {
			ln.end, ell = p.cut(sp.start, sp.end, maxW)
		}
		p.buildLine(&ln, cut, ell)
		p.lines = append(p.lines, ln)
	}
	// Baselines: a line's ascent below the previous one's descent, with
	// the larger of the two lines' leading between, so that one style
	// spaces its lines exactly as Text does.
	w, y := 0.0, 0.0
	for i := range p.lines {
		ln := &p.lines[i]
		if i == 0 {
			y = ln.ascent
		} else {
			prev := &p.lines[i-1]
			y += prev.descent + ln.ascent + max(p.leading(prev), p.leading(ln))
		}
		ln.baseline = y
		w = max(w, ln.width)
	}
	h := 0.0
	if n := len(p.lines); n > 0 {
		h = p.lines[n-1].baseline + p.lines[n-1].descent
	}
	p.size = Sz(w, h)
}

// leading is the space a line's styles ask for beyond its glyphs: the
// largest line height there less the ascent and descent.
func (p *paragraph) leading(ln *paraLine) float64 {
	spacing := 0.0
	for _, st := range ln.styles {
		r := p.style(st.run)
		spacing = max(spacing, r.Size*r.LineHeight)
	}
	if len(ln.styles) == 0 {
		r := p.style(p.runAt(ln.start))
		spacing = r.Size * r.LineHeight
	}
	return spacing - ln.ascent - ln.descent
}

// cut returns where text[a:b] ends so that it and "…" fit in maxW, at a
// grapheme boundary with the spaces before it dropped, and the run whose
// style draws the "…".
func (p *paragraph) cut(a, b int, maxW float64) (int, int) {
	ell := p.runAt(max(b-1, a))
	room := maxW - lineWidth(ellipsisText, p.face(ell))
	keep := ellipsizeBy(p.text, a, b, p.prefix(a, b), room)
	for keep > a && (p.text[keep-1] == ' ' || p.text[keep-1] == '\t') {
		keep--
	}
	return keep, ell
}

// buildLine fills in ln's styles, pieces and metrics. A cut line ends in
// "…" drawn in run ell's style, or the base style for -1.
func (p *paragraph) buildLine(ln *paraLine, cut bool, ell int) {
	ln.s = p.text[ln.start:ln.end]
	for i, r := range p.runs {
		lo, hi := max(ln.start, r.start), min(ln.end, r.end)
		if lo >= hi {
			continue
		}
		st := r.style
		ln.styles = append(ln.styles, lineStyle{start: lo - ln.start, end: hi - ln.start, font: st.Font, size: st.Size, weight: st.Weight, run: i})
	}
	if cut {
		st := p.style(ell)
		n := len(ln.s)
		ln.s += ellipsisText
		ln.styles = append(ln.styles, lineStyle{start: n, end: len(ln.s), font: st.Font, size: st.Size, weight: st.Weight, run: ell})
	}
	for _, st := range ln.styles {
		m := st.font.face(st.size, st.weight).Metrics()
		ln.ascent, ln.descent = max(ln.ascent, m.HAscent), max(ln.descent, m.HDescent)
	}
	if len(ln.styles) == 0 {
		m := p.face(p.runAt(ln.start)).Metrics()
		ln.ascent, ln.descent = m.HAscent, m.HDescent
	}
	ln.pieces = layoutLine(ln.s, ln.styles, p.dir, p.lang)
	ln.width = piecesWidth(ln.pieces)
}

// lineX is where line i starts in a paragraph w wide aligned at align.
func (p *paragraph) lineX(i int, w, align float64) float64 {
	return (w - p.lines[i].width) * align
}

// lineAt returns the line at y below the paragraph's top: the one whose
// band, split halfway between baselines, holds it.
func (p *paragraph) lineAt(y float64) int {
	for i := 0; i+1 < len(p.lines); i++ {
		a, b := &p.lines[i], &p.lines[i+1]
		if y < (a.baseline+a.descent+b.baseline-b.ascent)/2 {
			return i
		}
	}
	return max(len(p.lines)-1, 0)
}

// offsetAt returns the text offset nearest at, a point relative to the
// paragraph's top-left corner in a box w wide aligned at align.
func (p *paragraph) offsetAt(at Point, w, align float64) int {
	if len(p.lines) == 0 {
		return 0
	}
	i := p.lineAt(at.Y)
	ln := &p.lines[i]
	b := caretAt(ln.s, ln.pieces, ln.styles, at.X-p.lineX(i, w, align))
	return ln.start + min(b, ln.end-ln.start)
}

// rects yields the boxes, relative to the paragraph's top-left corner,
// that cover text[lo:hi] in a box w wide aligned at align.
func (p *paragraph) rects(lo, hi int, w, align float64, yield func(Rect)) {
	for i := range p.lines {
		ln := &p.lines[i]
		a, b := max(lo, ln.start), min(hi, ln.end)
		if a >= b {
			continue
		}
		x := p.lineX(i, w, align)
		top := ln.baseline - ln.ascent
		spanX(ln.s, ln.pieces, ln.styles, a-ln.start, b-ln.start, func(x0, x1 float64) {
			yield(Rct(Pt(x+x0, top), Sz(x1-x0, ln.ascent+ln.descent)))
		})
	}
}
