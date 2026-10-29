package font

type glyphExtents struct {
	valid   bool
	extents GlyphExtents
}

// extentsCache keeps the extents of a face's glyphs as they are asked for.
//
// ggui: in pages of extentsPage glyphs, each made when one of its glyphs is
// first asked about, rather than a slot for every glyph as the face is
// made: a CJK font's faces have tens of thousands, of which text draws a
// few hundred.
type extentsCache struct {
	n     int // glyphs
	pages [][]glyphExtents
}

const extentsPage = 256

func newExtentsCache(n int) extentsCache {
	return extentsCache{n: n, pages: make([][]glyphExtents, (n+extentsPage-1)/extentsPage)}
}

func (ec extentsCache) get(gid GID) (GlyphExtents, bool) {
	if int(gid) >= ec.n {
		return GlyphExtents{}, false
	}
	page := ec.pages[int(gid)/extentsPage]
	if page == nil {
		return GlyphExtents{}, false
	}
	ge := page[int(gid)%extentsPage]
	return ge.extents, ge.valid
}

func (ec extentsCache) set(gid GID, extents GlyphExtents) {
	if int(gid) >= ec.n {
		return
	}
	p := &ec.pages[int(gid)/extentsPage]
	if *p == nil {
		*p = make([]glyphExtents, extentsPage)
	}
	(*p)[int(gid)%extentsPage] = glyphExtents{valid: true, extents: extents}
}

func (ec extentsCache) reset() {
	clear(ec.pages)
}

func (f *Face) GlyphExtents(glyph GID) (GlyphExtents, bool) {
	if e, ok := f.extentsCache.get(glyph); ok {
		return e, ok
	}
	e, ok := f.glyphExtentsRaw(glyph)
	if ok {
		f.extentsCache.set(glyph, e)
	}
	return e, ok
}
