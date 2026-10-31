// SPDX-License-Identifier: Unlicense OR BSD-3-Clause

package font

import (
	"sync/atomic"

	"github.com/ironpark/ggui/internal/typesetting/font/opentype/tables"
)

// glyfTable is the 'glyf' table, each glyph parsed the first time it is
// asked for. A font has thousands of outlines, a CJK one tens of
// thousands, and a window draws a few hundred of them: parsing them all
// as the font loads held megabytes of points no one read. A Font is
// shared between goroutines, so a parsed glyph is kept atomically; two
// that parse one at once parse it the same.
type glyfTable struct {
	src    []byte
	loca   []uint32 // numGlyphs + 1 offsets into src
	glyphs []atomic.Pointer[tables.Glyph]
}

// newGlyfTable is the table src, its glyphs at loca's offsets, as
// tables.ParseLoca returns them; nil without any.
func newGlyfTable(src []byte, loca []uint32) *glyfTable {
	if len(loca) < 2 {
		return nil
	}
	return &glyfTable{src: src, loca: loca, glyphs: make([]atomic.Pointer[tables.Glyph], len(loca)-1)}
}

// Len is the number of glyphs.
func (g *glyfTable) Len() int {
	if g == nil {
		return 0
	}
	return len(g.glyphs)
}

// Glyph is glyph gid, which must be below Len. One without an outline,
// or whose data cannot be read, is empty.
func (g *glyfTable) Glyph(gid tables.GlyphID) tables.Glyph {
	slot := &g.glyphs[gid]
	if p := slot.Load(); p != nil {
		return *p
	}
	var out tables.Glyph
	start, end := g.loca[gid], g.loca[gid+1]
	if start < end && int(end) <= len(g.src) {
		if parsed, _, err := tables.ParseGlyph(g.src[start:end]); err == nil {
			out = parsed
		}
	}
	slot.Store(&out)
	return out
}
