package ggui

import (
	"image/color"
	"slices"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/go-text/typesetting/bidi"
	gofont "github.com/go-text/typesetting/font"
	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/text/v2"
	"golang.org/x/text/language"

	"github.com/ironpark/ggui/internal/emojidata"
	"github.com/ironpark/ggui/internal/textedit"
)

// A line of text is drawn as pieces in visual order. Each piece is one
// style's stretch of one bidi level: the Unicode Bidirectional Algorithm
// gives every character a level, a line is cut where the level or the
// style changes, and rule L2 orders the pieces left to right. A left-to-
// right piece draws as any text does. A right-to-left one is shaped right
// to left, with the language of the tree, by each font of its fallback
// chain in turn, and those runs are laid out from the right: a fallback
// face draws the runs of a line in the order they were written, which is
// backwards for Arabic or Hebrew set in a Latin font's fallbacks.

// lineStyle is one style's stretch of a line: s[start:end] in font at
// size and weight. run is the caller's index for the style.
type lineStyle struct {
	start, end int
	font       *Font
	size       float64
	weight     FontWeight
	run        int
}

// piece is a stretch of a line drawn in one direction and one style, at x
// from the line's left edge and w wide, both at scale 1.
type piece struct {
	start, end int // in the line
	style      int // index into the line's styles
	rtl        bool
	x, w       float64
	segs       []rtlSeg // a right-to-left piece's runs, in logical order

	pos func(b int) float64 // x of offset b within the piece; built on first use
}

// rtlSeg is a run of a right-to-left piece drawn by one face of its chain:
// face indexes rtlChain.faces, or is -1 for the emoji font.
type rtlSeg struct {
	start, end int
	face       int
	w          float64
}

// hasRTL reports whether s holds a right-to-left character or an explicit
// right-to-left mark, embedding or isolate.
func hasRTL(s string) bool {
	quick := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 0xD6 {
			quick = true
			break
		}
	}
	if !quick {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 0x0590 && r <= 0x08FF, r >= 0xFB1D && r <= 0xFDFF, r >= 0xFE70 && r <= 0xFEFF,
			r >= 0x10800 && r <= 0x10FFF, r >= 0x1E800 && r <= 0x1EFFF,
			r == 0x200F, r == 0x202B, r == 0x202E, r == 0x2067:
			return true
		}
	}
	return false
}

// needsBidi reports whether a line must be reordered: it is in a right-to-
// left paragraph, or holds right-to-left text.
func needsBidi(s string, base TextDirection) bool { return base == DirRTL || hasRTL(s) }

var bidiPool = sync.Pool{New: func() any { return new(bidi.Paragraph) }}

// layoutLine cuts s into pieces in visual order and measures them at scale
// 1. A line with no right-to-left text in a left-to-right paragraph is one
// piece per style.
func layoutLine(s string, styles []lineStyle, base TextDirection, lang string) []piece {
	var out []piece
	if !needsBidi(s, base) {
		x := 0.0
		for i, st := range styles {
			if st.start == st.end {
				continue
			}
			w := lineWidth(s[st.start:st.end], st.font.face(st.size, st.weight))
			out = append(out, piece{start: st.start, end: st.end, style: i, x: x, w: w})
			x += w
		}
		return out
	}
	para := bidiPool.Get().(*bidi.Paragraph)
	defer bidiPool.Put(para)
	dir := bidi.LeftToRight
	if base == DirRTL {
		dir = bidi.RightToLeft
	}
	runs := para.SegmentString(s, dir)
	// Rune index to byte offset, for the runs' rune ranges.
	starts := make([]int, 0, len(s)+1)
	for i := range s {
		starts = append(starts, i)
	}
	starts = append(starts, len(s))
	var levels []bidi.Level
	for i := 0; i < runs.NumRuns(); i++ {
		r := runs.Run(i)
		a, b := starts[r.Start], starts[r.End]
		for si, st := range styles {
			lo, hi := max(a, st.start), min(b, st.end)
			if lo >= hi {
				continue
			}
			out = append(out, piece{start: lo, end: hi, style: si, rtl: r.Level%2 == 1})
			levels = append(levels, r.Level)
		}
	}
	order := visualOrder(levels)
	sorted := make([]piece, len(out))
	for i, li := range order {
		sorted[i] = out[li]
	}
	x := 0.0
	for i := range sorted {
		p := &sorted[i]
		st := styles[p.style]
		if p.rtl {
			p.segs, p.w = rtlSegs(s[p.start:p.end], p.start, rtlFaces(st.font, st.size, st.weight, lang, s[p.start:p.end]))
		} else {
			p.w = lineWidth(s[p.start:p.end], st.font.face(st.size, st.weight))
		}
		p.x = x
		x += p.w
	}
	return sorted
}

// visualOrder returns the indexes of runs at levels in left-to-right
// order, by rule L2: from the highest level down to the lowest odd one,
// reverse every maximal sequence at that level or above.
func visualOrder(levels []bidi.Level) []int {
	order := make([]int, len(levels))
	for i := range order {
		order[i] = i
	}
	if len(levels) < 2 {
		return order
	}
	hi, lowOdd := bidi.Level(0), bidi.Level(127)
	for _, l := range levels {
		hi = max(hi, l)
		if l%2 == 1 {
			lowOdd = min(lowOdd, l)
		}
	}
	for lvl := hi; lvl >= lowOdd && lvl > 0; lvl-- {
		for i := 0; i < len(order); {
			if levels[order[i]] < lvl {
				i++
				continue
			}
			j := i
			for j < len(order) && levels[order[j]] >= lvl {
				j++
			}
			slices.Reverse(order[i:j])
			i = j
		}
	}
	return order
}

// width returns the line's width: where its last piece ends.
func piecesWidth(ps []piece) float64 {
	if len(ps) == 0 {
		return 0
	}
	last := ps[len(ps)-1]
	return last.x + last.w
}

// rtlSegs cuts s, a right-to-left run starting at off in its line, into
// runs by the face of chain that draws each grapheme, and measures them.
func rtlSegs(s string, off int, chain *rtlChain) ([]rtlSeg, float64) {
	var segs []rtlSeg
	total := 0.0
	start, cur := 0, -2
	flush := func(end int) {
		if cur == -2 || end == start {
			return
		}
		w := text.AdvanceAt(s[start:end], end-start, chain.face(cur))
		segs = append(segs, rtlSeg{start: off + start, end: off + end, face: cur, w: w})
		total += w
	}
	for i := 0; i < len(s); {
		end := textedit.NextGrapheme(s, i)
		f := chain.pick(s[i:end])
		if f != cur {
			flush(i)
			start, cur = i, f
		}
		i = end
	}
	flush(len(s))
	return segs, total
}

// rtlChain is a Font's faces at one size, weight and language, set right
// to left, with what each covers.
type rtlChain struct {
	srcs  []*text.GoTextFaceSource
	faces []text.Face
	emoji *emojiFace // the emoji fallback, if the font has one
}

// pick returns the index of the face that draws grapheme g: the emoji
// font's for an emoji, else the first face that has its first rune, else
// the last, as a fallback face picks.
func (c *rtlChain) pick(g string) int {
	if c.emoji != nil && emojidata.IsCluster(g) && c.emoji.colorFace() != nil {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(g)
	for i, src := range c.srcs {
		if f, ok := src.UnsafeInternal().(*gofont.Face); ok {
			if _, has := f.NominalGlyph(r); has {
				return i
			}
		}
	}
	return len(c.faces) - 1
}

func (c *rtlChain) face(i int) text.Face {
	if i < 0 {
		return c.emoji.colorFace()
	}
	return c.faces[i]
}

type rtlKey struct {
	font   *Font
	size   float64
	weight FontWeight
	lang   string
	gen    uint64
}

var rtlCache struct {
	sync.Mutex
	gen    uint64
	chains map[rtlKey]*rtlChain
}

// rtlFaces returns f's chain at size and weight, shaping in lang, or when
// that is unset in the script of sample.
func rtlFaces(f *Font, size float64, w FontWeight, lang, sample string) *rtlChain {
	for f.alias != nil {
		f = f.alias()
	}
	tag := shapingLanguage(lang, sample)
	gen := fontGen()
	key := rtlKey{f, size, w, tag.String(), gen}
	rtlCache.Lock()
	defer rtlCache.Unlock()
	if rtlCache.gen != gen {
		rtlCache.chains, rtlCache.gen = nil, gen
	}
	if c, ok := rtlCache.chains[key]; ok {
		return c
	}
	// The same faces Font.face puts in its fallback chain, in its order.
	c := &rtlChain{}
	add := func(own *Font, axis bool) {
		g := own.goFace(size, w, axis)
		g.Direction = text.DirectionRightToLeft
		g.Language = tag
		c.srcs = append(c.srcs, own.source())
		c.faces = append(c.faces, g)
	}
	f.mu.Lock()
	own, axis := f.forWeight(w)
	fallbacks, noFallback := f.fallbacks, f.noFallback
	f.mu.Unlock()
	add(own, axis)
	if fallbacks == nil && !noFallback {
		fallbacks = SystemFonts()
	}
	for _, fb := range fallbacks {
		if fb != f {
			add(fb.forWeight(w))
		}
	}
	if !noFallback {
		c.emoji, _ = f.face(size, w).(*emojiFace)
	}
	if rtlCache.chains == nil {
		rtlCache.chains = map[rtlKey]*rtlChain{}
	}
	rtlCache.chains[key] = c
	return c
}

// shapingLanguage is lang as a tag, or, when lang is unset, the
// undetermined language in the script of the first right-to-left letter
// of sample, which tells the shaper what it is shaping.
func shapingLanguage(lang, sample string) language.Tag {
	if lang != "" {
		if t, err := language.Parse(lang); err == nil {
			return t
		}
	}
	for _, r := range sample {
		for _, sc := range rtlScripts {
			if unicode.Is(sc.table, r) {
				return language.Make("und-" + sc.code)
			}
		}
	}
	return language.Und
}

var rtlScripts = []struct {
	code  string
	table *unicode.RangeTable
}{
	{"Arab", unicode.Arabic}, {"Hebr", unicode.Hebrew}, {"Syrc", unicode.Syriac},
	{"Thaa", unicode.Thaana}, {"Nkoo", unicode.Nko}, {"Adlm", unicode.Adlam},
}

// offsetX returns the x, from the line's left edge at scale 1, of offset b
// within p: a right-to-left piece counts from its right edge.
func (p *piece) offsetX(s string, styles []lineStyle, lang string, b int) float64 {
	if p.pos == nil {
		st := styles[p.style]
		if !p.rtl {
			w := prefixWidths(s[p.start:p.end], st.font.face(st.size, st.weight))
			p.pos = func(b int) float64 { return w(b - p.start) }
		} else {
			chain := rtlFaces(st.font, st.size, st.weight, lang, s[p.start:p.end])
			widths := make([]func(int) float64, len(p.segs))
			for i, sg := range p.segs {
				widths[i] = prefixWidths(s[sg.start:sg.end], chain.face(sg.face))
			}
			p.pos = func(b int) float64 {
				x := 0.0
				for i, sg := range p.segs {
					if b < sg.end {
						return x + widths[i](max(b-sg.start, 0))
					}
					x += sg.w
				}
				return x
			}
		}
	}
	b = min(max(b, p.start), p.end)
	if p.rtl {
		return p.x + p.w - p.pos(b)
	}
	return p.x + p.pos(b)
}

// caretAt returns the offset in s nearest x on a line laid out as ps: past
// either end, the offset at that visual edge.
func caretAt(s string, ps []piece, styles []lineStyle, lang string, x float64) int {
	if len(ps) == 0 {
		return 0
	}
	edge := func(p *piece, right bool) int { return pick(p.rtl != right, p.end, p.start) }
	if x <= ps[0].x {
		return edge(&ps[0], false)
	}
	if last := &ps[len(ps)-1]; x >= last.x+last.w {
		return edge(last, true)
	}
	for i := range ps {
		p := &ps[i]
		if x > p.x+p.w {
			continue
		}
		best, bestD := p.start, -1.0
		for b := p.start; ; b = textedit.NextGrapheme(s, b) {
			d := p.offsetX(s, styles, lang, b) - x
			d = max(d, -d)
			if bestD < 0 || d < bestD {
				best, bestD = b, d
			}
			if b >= p.end {
				break
			}
		}
		return best
	}
	return len(s)
}

// spanX returns the stretches, from the line's left edge, that the offsets
// lo to hi of the line cover, one per piece they touch.
func spanX(s string, ps []piece, styles []lineStyle, lang string, lo, hi int, yield func(x0, x1 float64)) {
	for i := range ps {
		p := &ps[i]
		a, b := max(lo, p.start), min(hi, p.end)
		if a >= b {
			continue
		}
		x0, x1 := p.offsetX(s, styles, lang, a), p.offsetX(s, styles, lang, b)
		yield(min(x0, x1), max(x0, x1))
	}
}

// drawPieces draws a laid-out line. scale is the Canvas's, baseline the
// line's baseline below where place puts the line's origin, in image
// pixels, and col the color of a style; place moves a freshly translated
// GeoM to the line.
func drawPieces(dst *ggfx.Image, s string, ps []piece, styles []lineStyle, lang string, scale, baseline float64, col func(style int) color.Color, place func(op *text.DrawOptions)) {
	if dst == nil || dst.Bounds().Empty() {
		return
	}
	for i := range ps {
		p := &ps[i]
		st := styles[p.style]
		op := &text.DrawOptions{}
		op.ColorScale.ScaleWithColor(col(p.style))
		if !p.rtl {
			face := st.font.face(st.size*scale, st.weight)
			op.GeoM.Translate(p.x*scale, baseline-face.Metrics().HAscent)
			place(op)
			drawText(dst, s[p.start:p.end], face, op)
			continue
		}
		chain := rtlFaces(st.font, st.size*scale, st.weight, lang, s[p.start:p.end])
		right := p.x + p.w
		for _, sg := range p.segs {
			right -= sg.w
			face := chain.face(sg.face)
			op.GeoM.Reset()
			op.GeoM.Translate(right*scale, baseline-face.Metrics().HAscent)
			place(op)
			op.LayoutOptions.PrimaryAlign = text.AlignStart
			if g, ok := face.(*text.GoTextFace); ok && g.Direction == text.DirectionRightToLeft {
				// A right-to-left face starts at the right; its end is the left.
				op.LayoutOptions.PrimaryAlign = text.AlignEnd
			}
			text.Draw(dst, s[sg.start:sg.end], face, op)
		}
	}
}
