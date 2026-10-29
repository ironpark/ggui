package font

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"

	ot "github.com/ironpark/ggui/internal/typesetting/font/opentype"
)

// systemVariableFont is a font with a gvar table found on macOS; the tests
// skip where the system does not have it.
const systemVariableFont = "/System/Library/Fonts/SFNS.ttf"

func loadSystemFont(t *testing.T) *Font {
	t.Helper()
	b, err := os.ReadFile(systemVariableFont)
	if err != nil {
		t.Skip(err)
	}
	lds, err := ot.NewLoaders(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFont(lds[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(f.gvar.variations) == 0 {
		t.Fatal("no glyph variations")
	}
	return f
}

// A glyph's variations are decoded when it is drawn at a variation, not
// when the font loads, and drawing it at one changes its outline.
func TestGlyphVariationsDecodeWhenDrawn(t *testing.T) {
	f := loadSystemFont(t)
	for i := range f.gvar.variations {
		if f.gvar.variations[i].Load() != nil {
			t.Fatalf("glyph %d decoded as the font loaded", i)
		}
	}
	gid, ok := f.Cmap.Lookup('a')
	if !ok {
		t.Fatal("no glyph for a")
	}
	regular := fmt.Sprint(NewFace(f).GlyphData(gid))
	bold := NewFace(f)
	bold.SetVariations([]Variation{{Tag: ot.MustNewTag("wght"), Value: 900}})
	if fmt.Sprint(bold.GlyphData(gid)) == regular {
		t.Error("a is drawn alike at weights 400 and 900")
	}
	if f.gvar.variations[gid].Load() == nil {
		t.Error("a was drawn at a variation but not decoded")
	}
}

// A font is shared by the faces drawing in it, so glyphs are decoded from
// several goroutines at once.
func TestGlyphVariationsDecodeConcurrently(t *testing.T) {
	f := loadSystemFont(t)
	n := len(f.gvar.variations)
	var wg sync.WaitGroup
	results := make([][][]tupleVariation, 4)
	for w := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[w] = make([][]tupleVariation, n)
			for i := range n {
				results[w][i] = f.gvar.glyphVariations(gID(i))
			}
		}()
	}
	wg.Wait()
	for w := 1; w < len(results); w++ {
		if !reflect.DeepEqual(results[w], results[0]) {
			t.Fatalf("goroutine %d decoded different variations", w)
		}
	}
}
