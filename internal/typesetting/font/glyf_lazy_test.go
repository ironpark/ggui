package font

import (
	"bytes"
	"os"
	"reflect"
	"sync"
	"testing"

	ot "github.com/ironpark/ggui/internal/typesetting/font/opentype"
	"github.com/ironpark/ggui/internal/typesetting/font/opentype/tables"
	"golang.org/x/image/font/gofont/goregular"
)

// glyfFonts are fonts with a 'glyf' table: Go's, which every system has,
// and macOS's, one of them variable, where the system has them.
func glyfFonts(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{"goregular": goregular.TTF}
	for _, path := range []string{systemVariableFont, "/System/Library/Fonts/Supplemental/Arial.ttf", "/System/Library/Fonts/NewYork.ttf"} {
		if b, err := os.ReadFile(path); err == nil {
			out[path] = b
		}
	}
	return out
}

func loadGlyfFont(t *testing.T, b []byte) (*Font, *ot.Loader) {
	t.Helper()
	lds, err := ot.NewLoaders(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFont(lds[0])
	if err != nil {
		t.Fatal(err)
	}
	if f.glyf.Len() == 0 {
		t.Fatal("no glyf table")
	}
	return f, lds[0]
}

// A glyph is parsed when it is asked for, not as the font loads, and is
// the glyph parsing the whole table would have given.
func TestGlyfGlyphsAreParsedWhenAskedAsTheWholeTableWould(t *testing.T) {
	for name, b := range glyfFonts(t) {
		f, ld := loadGlyfFont(t, b)
		for i := range f.glyf.glyphs {
			if f.glyf.glyphs[i].Load() != nil {
				t.Fatalf("%s: glyph %d parsed as the font loaded", name, i)
			}
		}
		raw, _ := ld.RawTable(ot.MustNewTag("glyf"))
		locaRaw, _ := ld.RawTable(ot.MustNewTag("loca"))
		loca, err := tables.ParseLoca(locaRaw, f.nGlyphs, f.head.IndexToLocFormat == 1)
		if err != nil {
			t.Fatal(err)
		}
		whole, err := tables.ParseGlyf(raw, loca)
		if err != nil {
			t.Fatal(err)
		}
		if len(whole) != f.glyf.Len() {
			t.Fatalf("%s: %d glyphs, want %d", name, f.glyf.Len(), len(whole))
		}
		for i := range whole {
			if got := f.glyf.Glyph(tables.GlyphID(i)); !reflect.DeepEqual(got, whole[i]) {
				t.Fatalf("%s: glyph %d is %+v, want %+v", name, i, got, whole[i])
			}
		}
	}
}

// Goroutines sharing a font may ask for one glyph at once.
func TestGlyfGlyphsParseConcurrently(t *testing.T) {
	f, _ := loadGlyfFont(t, goregular.TTF)
	want := f.glyf.Len()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for gid := range want {
				f.glyf.Glyph(tables.GlyphID(gid))
			}
		})
	}
	wg.Wait()
}
