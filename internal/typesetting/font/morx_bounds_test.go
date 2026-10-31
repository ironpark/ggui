package font

import (
	"bytes"
	"os"
	"testing"

	ot "github.com/ironpark/ggui/internal/typesetting/font/opentype"
	"github.com/ironpark/ggui/internal/typesetting/font/opentype/tables"
)

// A ligature subtable's list, whose length the table does not give, ends
// with the subtable: read on to the end of the table, Apple's emoji font
// held two million ligatures, eight megabytes, for its 2,470.
func TestMorxLigaturesEndWithTheirSubtable(t *testing.T) {
	b, err := os.ReadFile("/System/Library/Fonts/Apple Color Emoji.ttc")
	if err != nil {
		t.Skip(err)
	}
	lds, err := ot.NewLoaders(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := lds[0].RawTable(ot.MustNewTag("morx"))
	if err != nil {
		t.Skip("no morx table")
	}
	f, err := NewFont(lds[0])
	if err != nil {
		t.Fatal(err)
	}
	morx, _, err := tables.ParseMorx(raw, f.nGlyphs)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, c := range morx.Chains {
		for _, s := range c.Subtables {
			if lig, ok := s.Data.(tables.MorxSubtableLigature); ok {
				total += len(lig.Ligatures)
			}
		}
	}
	if total == 0 || 2*total > len(raw) {
		t.Fatalf("%d ligatures in a %d byte table", total, len(raw))
	}
}
