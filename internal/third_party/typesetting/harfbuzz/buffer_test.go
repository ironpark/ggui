package harfbuzz

import "testing"

func TestDeleteGlyphsInplaceBeforePositionsAllocated(t *testing.T) {
	// Reproduces AAT morx + OpenType GPOS crash where morx inserts glyphs
	// (growing Info beyond Pos) and marks a glyph deleted before position()
	// calls clearPositions().
	b := NewBuffer()
	b.Info = []GlyphInfo{
		{Glyph: 10, Cluster: 0},
		{Glyph: 0xFFFF, Cluster: 1}, // AAT deleted glyph
		{Glyph: 20, Cluster: 1},     // inserted by morx
	}
	b.Pos = make([]GlyphPosition, 2) // pre-substitution length

	b.deleteGlyphsInplace(func(info *GlyphInfo) bool {
		return info.Glyph == 0xFFFF
	})

	if len(b.Info) != 2 {
		t.Fatalf("len(Info) = %d, want 2", len(b.Info))
	}
	if b.Info[0].Glyph != 10 || b.Info[1].Glyph != 20 {
		t.Fatalf("Info = %+v, want glyphs [10, 20]", b.Info)
	}
}

func TestDeleteGlyphsInplaceWithPositions(t *testing.T) {
	b := NewBuffer()
	b.Info = []GlyphInfo{
		{Glyph: 10, Cluster: 0},
		{Glyph: 0xFFFF, Cluster: 1},
		{Glyph: 20, Cluster: 2},
	}
	b.Pos = []GlyphPosition{
		{XAdvance: 100},
		{XAdvance: 200},
		{XAdvance: 300},
	}

	b.deleteGlyphsInplace(func(info *GlyphInfo) bool {
		return info.Glyph == 0xFFFF
	})

	if len(b.Info) != 2 || len(b.Pos) != 2 {
		t.Fatalf("lens = (%d, %d), want (2, 2)", len(b.Info), len(b.Pos))
	}
	if b.Pos[0].XAdvance != 100 || b.Pos[1].XAdvance != 300 {
		t.Fatalf("Pos = %+v, want XAdvance [100, 300]", b.Pos)
	}
}
