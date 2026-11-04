package fontsys

import (
	"github.com/ironpark/ggui/internal/third_party/text"
)

// Picker finds, rune by rune, the font that draws it, in the order Face's
// fallback chain looks: the chain's own fonts first, then, for a rune none
// of them has, a system font that has it, which joins the end of the chain.
// Whoever shapes the runes makes their faces of the fonts it picks, so a
// right-to-left run or a renderer of its own can draw exactly what a Face
// would.
//
// A Picker is not safe for concurrent use; its fonts are, and Pick may run
// on any goroutine while others pick from the same fonts.
type Picker struct {
	fonts  []ChainFont
	metas  []*text.GoTextFaceSource // each font's cmap, read without its glyphs
	system bool
	weight Weight
	found  map[*Font]int
}

// Picker returns a Picker for f's chain at weight w.
func (f *Font) Picker(w Weight) *Picker {
	f = f.Resolve()
	f.mu.Lock()
	fonts, _ := f.chainLocked(w)
	p := &Picker{fonts: fonts, system: f.systemFallback(), weight: w}
	f.mu.Unlock()
	p.metas = make([]*text.GoTextFaceSource, 0, len(fonts))
	for _, cf := range fonts {
		p.metas = append(p.metas, cf.Font.Meta())
	}
	return p
}

// DrawsEmoji reports whether f's faces set an emoji font beside its own.
func (f *Font) DrawsEmoji() bool {
	f = f.Resolve()
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.noFallback
}

// Len is how many fonts the chain has: its own, and the system fonts Pick
// has added.
func (p *Picker) Len() int { return len(p.fonts) }

// Font returns the chain's font at i.
func (p *Picker) Font(i int) ChainFont { return p.fonts[i] }

// Pick returns the index of the font that draws r, and false when none
// does: no font of the chain has it, and no system font, or the chain
// looks in none.
func (p *Picker) Pick(r rune) (int, bool) {
	for i, src := range p.metas {
		if HasGlyph(src, r) {
			return i, true
		}
	}
	if !p.system {
		return 0, false
	}
	fb := SystemFallback(r)
	if fb == nil {
		return 0, false
	}
	if i, ok := p.found[fb]; ok {
		return i, true
	}
	font, axis := fb.ForWeight(p.weight)
	p.fonts = append(p.fonts, ChainFont{font, axis})
	p.metas = append(p.metas, font.Meta())
	if p.found == nil {
		p.found = map[*Font]int{}
	}
	p.found[fb] = len(p.fonts) - 1
	return len(p.fonts) - 1, true
}

// HasGlyph reports whether src's cmap maps r to a glyph, from any
// goroutine without a lock.
func HasGlyph(src *text.GoTextFaceSource, r rune) bool { return src.HasRune(r) }
