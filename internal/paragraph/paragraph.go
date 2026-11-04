package paragraph

import (
	"github.com/ironpark/ggui/internal/fn"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/pkg/layout"
)

// Paragraph is text in several styles laid out as one: it wraps across
// style boundaries, ends a cut line with "…", and orders each line's
// pieces by the bidi algorithm. Offsets are bytes of Text, the runs'
// strings one after another.
type Paragraph struct {
	Text  string
	Runs  []Run
	Base  Style // what an empty paragraph is measured in
	RTL   bool  // the paragraph's base direction is right to left
	Lang  string
	Lines []Line
	Size  layout.Size
}

// Run is one style's stretch of the text. Span is the caller's index for
// it.
type Run struct {
	Start, End int
	Style      Style
	Span       int
}

// Line is one laid-out line. S is what it draws: Text[Start:End], then
// "…" when the line was cut.
type Line struct {
	Start, End int
	S          string
	Styles     []LineStyle // their Run is the index of the run in Runs, -1 for Base
	Pieces     []Piece
	Width      float64
	Ascent     float64
	Descent    float64
	Baseline   float64 // below the paragraph's top
}

// StyleOf returns run i's style, or the base style for -1.
func (p *Paragraph) StyleOf(i int) Style {
	if i < 0 {
		return p.Base
	}
	return p.Runs[i].Style
}

// face returns run i's face at scale 1, or the base style's for -1.
func (p *Paragraph) face(i int) text.Face { return p.StyleOf(i).Face(1) }

// width measures Text[a:b], each run's part in its own face.
func (p *Paragraph) width(a, b int) float64 {
	w := 0.0
	for i, r := range p.Runs {
		lo, hi := max(a, r.Start), min(b, r.End)
		if lo < hi {
			w += textlayout.Width(p.Text[lo:hi], p.face(i))
		}
	}
	return w
}

// prefix returns the width of Text[a:x] for any x in [a, b], shaping each
// run once.
func (p *Paragraph) prefix(a, b int) func(x int) float64 {
	type part struct {
		lo, hi int
		before float64
		w      func(int) float64
	}
	var parts []part
	total := 0.0
	for i, r := range p.Runs {
		lo, hi := max(a, r.Start), min(b, r.End)
		if lo >= hi {
			continue
		}
		w := textlayout.PrefixWidths(p.Text[lo:hi], p.face(i))
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

// RunAt returns the run holding offset pos, the one before it when pos
// ends the text or falls in an empty run, or -1 when there is none.
func (p *Paragraph) RunAt(pos int) int {
	at := -1
	for i, r := range p.Runs {
		if r.Start > pos {
			break
		}
		at = i
		if pos < r.End {
			break
		}
	}
	return at
}

// Layout breaks the text into lines no wider than maxW, as Text does: at
// spaces when wrap is set, else at hard breaks only; at most maxLines when
// that is positive, the last cut with "…" when text goes on past it; and
// with ellipsis every line too wide cut to fit.
func (p *Paragraph) Layout(maxW float64, wrap bool, maxLines int, ellipsis bool) {
	spans := textlayout.WrapSpansBy(p.Text, p.width, fn.Pick(wrap, maxW, 0))
	truncated := maxLines > 0 && len(spans) > maxLines
	if truncated {
		spans = spans[:maxLines]
	}
	p.Lines = p.Lines[:0]
	for i, sp := range spans {
		ln := Line{Start: sp.Start, End: sp.End}
		ell, cut := -1, truncated && i == len(spans)-1 || ellipsis && p.width(sp.Start, sp.End) > maxW
		if cut {
			ln.End, ell = p.cut(sp.Start, sp.End, maxW)
		}
		p.buildLine(&ln, cut, ell)
		p.Lines = append(p.Lines, ln)
	}
	// Baselines: a line's ascent below the previous one's descent, with
	// the larger of the two lines' leading between, so that one style
	// spaces its lines exactly as Text does.
	w, y := 0.0, 0.0
	for i := range p.Lines {
		ln := &p.Lines[i]
		if i == 0 {
			y = ln.Ascent
		} else {
			prev := &p.Lines[i-1]
			y += prev.Descent + ln.Ascent + max(p.leading(prev), p.leading(ln))
		}
		ln.Baseline = y
		w = max(w, ln.Width)
	}
	h := 0.0
	if n := len(p.Lines); n > 0 {
		h = p.Lines[n-1].Baseline + p.Lines[n-1].Descent
	}
	p.Size = layout.Sz(w, h)
}

// leading is the space a line's styles ask for beyond its glyphs: the
// largest line height there less the ascent and descent.
func (p *Paragraph) leading(ln *Line) float64 {
	spacing := 0.0
	for _, st := range ln.Styles {
		r := p.StyleOf(st.Run)
		spacing = max(spacing, r.Size*r.LineHeight)
	}
	if len(ln.Styles) == 0 {
		r := p.StyleOf(p.RunAt(ln.Start))
		spacing = r.Size * r.LineHeight
	}
	return spacing - ln.Ascent - ln.Descent
}

// cut returns where Text[a:b] ends so that it and "…" fit in maxW, at a
// grapheme boundary with the spaces before it dropped, and the run whose
// style draws the "…".
func (p *Paragraph) cut(a, b int, maxW float64) (int, int) {
	ell := p.RunAt(max(b-1, a))
	room := maxW - textlayout.Width(textlayout.Ellipsis, p.face(ell))
	keep := textlayout.EllipsizeBy(p.Text, a, b, p.prefix(a, b), room)
	for keep > a && (p.Text[keep-1] == ' ' || p.Text[keep-1] == '\t') {
		keep--
	}
	return keep, ell
}

// buildLine fills in ln's styles, pieces and metrics. A cut line ends in
// "…" drawn in run ell's style, or the base style for -1.
func (p *Paragraph) buildLine(ln *Line, cut bool, ell int) {
	ln.S = p.Text[ln.Start:ln.End]
	for i, r := range p.Runs {
		lo, hi := max(ln.Start, r.Start), min(ln.End, r.End)
		if lo >= hi {
			continue
		}
		ln.Styles = append(ln.Styles, LineStyle{Start: lo - ln.Start, End: hi - ln.Start, Style: r.Style, Run: i})
	}
	if cut {
		n := len(ln.S)
		ln.S += textlayout.Ellipsis
		ln.Styles = append(ln.Styles, LineStyle{Start: n, End: len(ln.S), Style: p.StyleOf(ell), Run: ell})
	}
	for _, st := range ln.Styles {
		m := st.Style.Face(1).Metrics()
		ln.Ascent, ln.Descent = max(ln.Ascent, m.HAscent), max(ln.Descent, m.HDescent)
	}
	if len(ln.Styles) == 0 {
		m := p.face(p.RunAt(ln.Start)).Metrics()
		ln.Ascent, ln.Descent = m.HAscent, m.HDescent
	}
	ln.Pieces = LayoutLine(ln.S, ln.Styles, p.RTL, p.Lang)
	ln.Width = Width(ln.Pieces)
}

// LineX is where line i starts in a paragraph w wide aligned at align.
func (p *Paragraph) LineX(i int, w, align float64) float64 {
	return (w - p.Lines[i].Width) * align
}

// LineAt returns the line at y below the paragraph's top: the one whose
// band, split halfway between baselines, holds it.
func (p *Paragraph) LineAt(y float64) int {
	for i := 0; i+1 < len(p.Lines); i++ {
		a, b := &p.Lines[i], &p.Lines[i+1]
		if y < (a.Baseline+a.Descent+b.Baseline-b.Ascent)/2 {
			return i
		}
	}
	return max(len(p.Lines)-1, 0)
}

// OffsetAt returns the text offset nearest at, a point relative to the
// paragraph's top-left corner in a box w wide aligned at align.
func (p *Paragraph) OffsetAt(at layout.Point, w, align float64) int {
	if len(p.Lines) == 0 {
		return 0
	}
	i := p.LineAt(at.Y)
	ln := &p.Lines[i]
	b := CaretAt(ln.S, ln.Pieces, ln.Styles, at.X-p.LineX(i, w, align))
	return ln.Start + min(b, ln.End-ln.Start)
}

// Rects yields the boxes, relative to the paragraph's top-left corner,
// that cover Text[lo:hi] in a box w wide aligned at align.
func (p *Paragraph) Rects(lo, hi int, w, align float64, yield func(layout.Rect)) {
	for i := range p.Lines {
		ln := &p.Lines[i]
		a, b := max(lo, ln.Start), min(hi, ln.End)
		if a >= b {
			continue
		}
		x := p.LineX(i, w, align)
		top := ln.Baseline - ln.Ascent
		SpanX(ln.S, ln.Pieces, ln.Styles, a-ln.Start, b-ln.Start, func(x0, x1 float64) {
			yield(layout.Rct(layout.Pt(x+x0, top), layout.Sz(x1-x0, ln.Ascent+ln.Descent)))
		})
	}
}
