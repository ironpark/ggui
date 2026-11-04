package font

import "testing"

// The extents of a glyph are kept once set, until a reset, and a page of
// them is made only when one of its glyphs is set.
func TestExtentsCacheKeepsWhatIsSetInPages(t *testing.T) {
	ec := newExtentsCache(1000)
	if _, ok := ec.get(5); ok {
		t.Fatal("an unset glyph has extents")
	}
	want := GlyphExtents{XBearing: 1, YBearing: 2, Width: 3, Height: 4}
	ec.set(700, want)
	if got, ok := ec.get(700); !ok || got != want {
		t.Fatalf("get(700) = %v, %v", got, ok)
	}
	if _, ok := ec.get(701); ok {
		t.Fatal("a neighbour of a set glyph has extents")
	}
	made := 0
	for _, p := range ec.pages {
		if p != nil {
			made++
		}
	}
	if made != 1 {
		t.Fatalf("%d pages made for one glyph", made)
	}
	ec.set(1000, want) // out of range: ignored
	if _, ok := ec.get(1000); ok {
		t.Fatal("a glyph past the font's has extents")
	}
	ec.reset()
	if _, ok := ec.get(700); ok {
		t.Fatal("extents survive a reset")
	}
}
