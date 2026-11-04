package paragraph

import (
	"image/color"
	"slices"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/internal/third_party/typesetting/bidi"
	"github.com/ironpark/ggui/pkg/gem"
	"golang.org/x/text/language"

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

// LineStyle is one style's stretch of a line: s[Start:End] in Style. Run
// is the caller's index for the style.
type LineStyle struct {
	Start, End int
	Style      Style
	Run        int
}

// Piece is a stretch of a line drawn in one direction and one style, at x
// from the line's left edge and w wide, both at scale 1.
type Piece struct {
	Start, End int // in the line
	Style      int // index into the line's styles
	RTL        bool
	X, W       float64
	segs       []rtlSeg  // a right-to-left Piece's runs, in logical order
	lang       shapeLang // what a right-to-left Piece is shaped in

	pos func(b int) float64 // x of offset b within the Piece; built on first use

	// glyphs are the piece's glyphs laid out in glyphFace, the face it was
	// last drawn in: kept until the line is laid out again, so that a
	// frame that draws the line as it was, as a scroll's does, lays none
	// of it out again.
	glyphs    []text.LazyGlyph
	glyphFace text.Face
}

// rtlSeg is a run of a right-to-left piece drawn by one face of its chain:
// face indexes rtlChain.faces, or is -1 for the emoji font.
type rtlSeg struct {
	Start, End int
	face       int
	W          float64
}

// HasRTL reports whether s holds a right-to-left character or an explicit
// right-to-left mark, embedding or isolate.
func HasRTL(s string) bool {
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

// NeedsBidi reports whether a line must be reordered: it is in a right-to-
// left paragraph, or holds right-to-left text.
func NeedsBidi(s string, rtl bool) bool { return rtl || HasRTL(s) }

var bidiPool = sync.Pool{New: func() any { return new(bidi.Paragraph) }}

// LayoutLine cuts s into pieces in visual order and measures them at scale
// 1. A line with no right-to-left text in a left-to-right paragraph is one
// piece per style.
func LayoutLine(s string, styles []LineStyle, rtl bool, lang string) []Piece {
	var out []Piece
	if !NeedsBidi(s, rtl) {
		x := 0.0
		for i, st := range styles {
			if st.Start == st.End {
				continue
			}
			w := textlayout.Width(s[st.Start:st.End], st.Style.Face(1))
			out = append(out, Piece{Start: st.Start, End: st.End, Style: i, X: x, W: w})
			x += w
		}
		return out
	}
	para := bidiPool.Get().(*bidi.Paragraph)
	defer bidiPool.Put(para)
	dir := bidi.LeftToRight
	if rtl {
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
			lo, hi := max(a, st.Start), min(b, st.End)
			if lo >= hi {
				continue
			}
			out = append(out, Piece{Start: lo, End: hi, Style: si, RTL: r.Level%2 == 1})
			levels = append(levels, r.Level)
		}
	}
	order := visualOrder(levels)
	sorted := make([]Piece, len(out))
	for i, li := range order {
		sorted[i] = out[li]
	}
	x := 0.0
	for i := range sorted {
		p := &sorted[i]
		st := styles[p.Style]
		if p.RTL {
			p.lang = newShapeLang(lang, s[p.Start:p.End])
			p.segs, p.W = rtlSegs(s[p.Start:p.End], p.Start, rtlFaces(st.Style, 1, p.lang))
		} else {
			p.W = textlayout.Width(s[p.Start:p.End], st.Style.Face(1))
		}
		p.X = x
		x += p.W
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

// Width returns the line's width: where its last piece ends.
func Width(ps []Piece) float64 {
	if len(ps) == 0 {
		return 0
	}
	last := ps[len(ps)-1]
	return last.X + last.W
}

// rtlSegs cuts s, a right-to-left run starting at off in its line, into
// runs by the face of chain that draws each grapheme, and measures them.
func rtlSegs(s string, off int, chain *rtlChain) ([]rtlSeg, float64) {
	var segs []rtlSeg
	total := 0.0
	textedit.GraphemeRuns(s, chain.pick, func(start, end, face int) bool {
		w := text.AdvanceAt(s[start:end], end-start, chain.face(face))
		segs = append(segs, rtlSeg{Start: off + start, End: off + end, face: face, W: w})
		total += w
		return true
	})
	return segs, total
}

// rtlChain is a Font's faces at one size, weight and language, set right
// to left, in the order its Picker finds them: a font is loaded only once a
// glyph only it has is drawn, as in fontsys.Font.Face's chain.
type rtlChain struct {
	// mu guards picker and faces, which grow as pick finds system fonts:
	// a chain is shared by every line laid out in it.
	mu     sync.Mutex
	picker *fontsys.Picker
	faces  []text.Face        // the picker's fonts' faces, in its order
	last   int                // the face a rune no font has falls to: the chain's last own one
	emoji  *fontsys.EmojiFace // the emoji fallback, if the font has one

	size   float64
	weight fontsys.Weight
	tag    language.Tag
}

// pick returns the index of the face that draws grapheme g: the emoji
// font's for an emoji, else the face of the font the picker finds for its
// first rune, else the chain's last, as a fallback face picks.
func (c *rtlChain) pick(g string) int {
	if c.emoji != nil && fontsys.IsEmojiCluster(g) && c.emoji.ColorFace() != nil {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(g)
	c.mu.Lock()
	defer c.mu.Unlock()
	i, ok := c.picker.Pick(r)
	if !ok {
		return c.last
	}
	for len(c.faces) < c.picker.Len() {
		cf := c.picker.Font(len(c.faces))
		c.faces = append(c.faces, cf.Font.ChainFaceWith(c.size, c.weight, cf.Axis, c.setRTL))
	}
	return i
}

// setRTL sets g to shape right to left in c's language.
func (c *rtlChain) setRTL(g *text.GoTextFace) {
	g.Direction = text.DirectionRightToLeft
	g.Language = c.tag
}

func (c *rtlChain) face(i int) text.Face {
	if i < 0 {
		return c.emoji.ColorFace()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.faces[i]
}

type rtlKey struct {
	font   *fontsys.Font
	size   float64
	weight fontsys.Weight
	emoji  *fontsys.EmojiChoice
	lang   string
	gen    uint64
}

// rtlCache holds the chains asked for lately in two generations, as
// envMemo does: once the current one is full it becomes the previous,
// and a chain asked for again from there moves back.
var rtlCache struct {
	sync.Mutex
	gen       uint64
	cur, prev map[rtlKey]*rtlChain
}

// rtlCacheMin is the size below which rtlCache never ages anything out.
const rtlCacheMin = 64

// rtlFaces returns st's chain at scale, shaping in lang.
func rtlFaces(st Style, scale float64, lang shapeLang) *rtlChain {
	f, size := st.Font.Resolve(), st.Size*scale
	gen := fontsys.Generation()
	key := rtlKey{f, size, st.Weight, st.Emoji, lang.key, gen}
	rtlCache.Lock()
	defer rtlCache.Unlock()
	if rtlCache.gen != gen {
		rtlCache.cur, rtlCache.prev, rtlCache.gen = nil, nil, gen
	}
	if c, ok := rtlCache.cur[key]; ok {
		return c
	}
	c, ok := rtlCache.prev[key]
	if !ok {
		c = newRTLChain(f, size, st.Weight, st.Emoji, lang.tag)
	}
	if rtlCache.cur == nil {
		rtlCache.cur = map[rtlKey]*rtlChain{}
	}
	if len(rtlCache.cur) >= max(rtlCacheMin, len(rtlCache.prev)) {
		rtlCache.prev, rtlCache.cur = rtlCache.cur, rtlCache.prev
		if rtlCache.cur == nil {
			rtlCache.cur = map[rtlKey]*rtlChain{}
		}
		clear(rtlCache.cur)
	}
	rtlCache.cur[key] = c
	return c
}

// newRTLChain builds f's chain at size and weight, shaping in tag: the
// same faces fontsys.Font.Face puts in its fallback chain, in its order,
// with the emoji font choice resolves to.
func newRTLChain(f *fontsys.Font, size float64, sw fontsys.Weight, choice *fontsys.EmojiChoice, tag language.Tag) *rtlChain {
	c := &rtlChain{picker: f.Picker(sw), size: size, weight: sw, tag: tag}
	for i := range c.picker.Len() {
		cf := c.picker.Font(i)
		c.faces = append(c.faces, cf.Font.ChainFaceWith(size, sw, cf.Axis, c.setRTL))
	}
	c.last = len(c.faces) - 1
	if f.DrawsEmoji() {
		c.emoji, _ = f.FaceFor(size, sw, choice).(*fontsys.EmojiFace)
	}
	return c
}

// shapeLang is the language a right-to-left piece is shaped in, resolved
// once at layout, with its tag's string, which keys rtlCache.
type shapeLang struct {
	tag language.Tag
	key string
}

// newShapeLang is shapingLanguage's tag for lang and sample. Each tag is
// made once: a line is laid out again at every width.
func newShapeLang(lang, sample string) shapeLang {
	// The script counts even with a lang, which may not parse.
	k := shapeLangKey{lang: lang, script: rtlScriptOf(sample)}
	shapeLangs.Lock()
	defer shapeLangs.Unlock()
	if l, ok := shapeLangs.m[k]; ok {
		return l
	}
	tag := shapingLanguage(lang, sample)
	l := shapeLang{tag, tag.String()}
	if shapeLangs.m == nil {
		shapeLangs.m = map[shapeLangKey]shapeLang{}
	}
	shapeLangs.m[k] = l
	return l
}

// shapeLangKey is what decides newShapeLang's result: lang, and the index
// in rtlScripts of the sample's script, -1 for none.
type shapeLangKey struct {
	lang   string
	script int
}

var shapeLangs struct {
	sync.Mutex
	m map[shapeLangKey]shapeLang
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
	if i := rtlScriptOf(sample); i >= 0 {
		return language.Make("und-" + rtlScripts[i].code)
	}
	return language.Und
}

// rtlScriptOf returns the index in rtlScripts of the script of the first
// right-to-left letter of s, or -1.
func rtlScriptOf(s string) int {
	for _, r := range s {
		for i, sc := range rtlScripts {
			if unicode.Is(sc.table, r) {
				return i
			}
		}
	}
	return -1
}

var rtlScripts = []struct {
	code  string
	table *unicode.RangeTable
}{
	{"Arab", unicode.Arabic}, {"Hebr", unicode.Hebrew}, {"Syrc", unicode.Syriac},
	{"Thaa", unicode.Thaana}, {"Nkoo", unicode.Nko}, {"Adlm", unicode.Adlam},
}

// OffsetX returns the x, from the line's left edge at scale 1, of offset b
// within p: a right-to-left piece counts from its right edge.
func (p *Piece) OffsetX(s string, styles []LineStyle, b int) float64 {
	if p.pos == nil {
		st := styles[p.Style]
		if !p.RTL {
			w := textlayout.PrefixWidths(s[p.Start:p.End], st.Style.Face(1))
			p.pos = func(b int) float64 { return w(b - p.Start) }
		} else {
			chain := rtlFaces(st.Style, 1, p.lang)
			widths := make([]func(int) float64, len(p.segs))
			for i, sg := range p.segs {
				widths[i] = textlayout.PrefixWidths(s[sg.Start:sg.End], chain.face(sg.face))
			}
			p.pos = func(b int) float64 {
				x := 0.0
				for i, sg := range p.segs {
					if b < sg.End {
						return x + widths[i](max(b-sg.Start, 0))
					}
					x += sg.W
				}
				return x
			}
		}
	}
	b = min(max(b, p.Start), p.End)
	if p.RTL {
		return p.X + p.W - p.pos(b)
	}
	return p.X + p.pos(b)
}

// CaretAt returns the offset in s nearest x on a line laid out as ps: past
// either end, the offset at that visual edge.
func CaretAt(s string, ps []Piece, styles []LineStyle, x float64) int {
	if len(ps) == 0 {
		return 0
	}
	edge := func(p *Piece, right bool) int { return gem.Pick(p.RTL != right, p.End, p.Start) }
	if x <= ps[0].X {
		return edge(&ps[0], false)
	}
	if last := &ps[len(ps)-1]; x >= last.X+last.W {
		return edge(last, true)
	}
	for i := range ps {
		p := &ps[i]
		if x > p.X+p.W {
			continue
		}
		best, bestD := p.Start, -1.0
		for b := p.Start; ; b = textedit.NextGrapheme(s, b) {
			d := p.OffsetX(s, styles, b) - x
			d = max(d, -d)
			if bestD < 0 || d < bestD {
				best, bestD = b, d
			}
			if b >= p.End {
				break
			}
		}
		return best
	}
	return len(s)
}

// SpanX returns the stretches, from the line's left edge, that the offsets
// lo to hi of the line cover, one per piece they touch.
func SpanX(s string, ps []Piece, styles []LineStyle, lo, hi int, yield func(x0, x1 float64)) {
	for i := range ps {
		p := &ps[i]
		a, b := max(lo, p.Start), min(hi, p.End)
		if a >= b {
			continue
		}
		x0, x1 := p.OffsetX(s, styles, a), p.OffsetX(s, styles, b)
		yield(min(x0, x1), max(x0, x1))
	}
}

// Draw draws a laid-out line. scale is the Canvas's, baseline the
// line's baseline below where place puts the line's origin, in image
// pixels, and col the color of a style; place moves a freshly translated
// GeoM to the line. op is scratch space the caller keeps, so that drawing
// allocates no options.
func Draw(dst *ggfx.Image, op *text.DrawOptions, s string, ps []Piece, styles []LineStyle, scale, baseline float64, col func(style int) color.Color, place func(op *text.DrawOptions)) {
	if dst == nil || dst.Bounds().Empty() {
		return
	}
	for i := range ps {
		p := &ps[i]
		st := styles[p.Style]
		*op = text.DrawOptions{}
		c := col(p.Style)
		if c == nil {
			c = color.Black // as a style resolves no color
		}
		op.ColorScale.ScaleWithColor(c)
		if !p.RTL {
			face := st.Style.Face(scale)
			op.GeoM.Translate(p.X*scale, baseline-face.Metrics().HAscent)
			place(op)
			t := s[p.Start:p.End]
			plain := fontsys.PlainFace(face, t)
			if plain == nil {
				fontsys.DrawText(dst, t, face, op)
				continue
			}
			if p.glyphFace != plain {
				p.glyphs = text.AppendLazyGlyphs(p.glyphs[:0], t, plain, nil)
				p.glyphFace = plain
			}
			text.DrawLazyGlyphs(dst, p.glyphs, op)
			continue
		}
		chain := rtlFaces(st.Style, scale, p.lang)
		right := p.X + p.W
		for _, sg := range p.segs {
			right -= sg.W
			face := chain.face(sg.face)
			op.GeoM.Reset()
			op.GeoM.Translate(right*scale, baseline-face.Metrics().HAscent)
			place(op)
			op.LayoutOptions.PrimaryAlign = text.AlignStart
			if g, ok := face.(*text.GoTextFace); ok && g.Direction == text.DirectionRightToLeft {
				// A right-to-left face starts at the right; its end is the left.
				op.LayoutOptions.PrimaryAlign = text.AlignEnd
			}
			text.Draw(dst, s[sg.Start:sg.End], face, op)
		}
	}
}
