package font

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	ot "github.com/ironpark/ggui/internal/typesetting/font/opentype"
	"github.com/ironpark/ggui/internal/typesetting/font/opentype/tables"
)

// A format 4 cmap read in place maps every BMP character as the spec reads
// the subtable, by lookup and by iteration.
func TestCmap4MapsAsTheSpecReadsIt(t *testing.T) {
	fonts := glyfFonts(t)
	for _, path := range []string{"/System/Library/Fonts/AppleSDGothicNeo.ttc", "/System/Library/Fonts/Helvetica.ttc"} {
		if b, err := os.ReadFile(path); err == nil {
			fonts[path] = b
		}
	}
	checked := 0
	for name, b := range fonts {
		lds, err := ot.NewLoaders(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := lds[0].RawTable(ot.MustNewTag("cmap"))
		if err != nil {
			continue
		}
		table, _, err := tables.ParseCmap(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, rec := range table.Records {
			sub, ok := rec.Subtable.(tables.CmapSubtable4)
			if !ok {
				continue
			}
			cm, err := newCmap4(sub)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			want := map[rune]GID{}
			for c := rune(0); c <= 0xffff; c++ {
				// Glyph 0 is .notdef: a character mapped to it is missing.
				g, ok := specCmap4(sub, uint16(c))
				ok = ok && g != 0
				if ok {
					want[c] = g
				}
				got, gotOK := cm.Lookup(c)
				if gotOK = gotOK && got != 0; gotOK != ok || (ok && got != g) {
					t.Fatalf("%s: %U looked up as %d %v, want %d %v", name, c, got, gotOK, g, ok)
				}
			}
			for it := cm.Iter(); it.Next(); {
				r, g := it.Char()
				if w, ok := want[r]; ok && w != g {
					t.Fatalf("%s: %U iterated as %d, want %d", name, r, g, w)
				}
			}
			checked++
			break
		}
	}
	if checked == 0 {
		t.Skip("no font with a format 4 cmap")
	}
}

// specCmap4 maps c as the OpenType spec reads a format 4 subtable.
func specCmap4(sub tables.CmapSubtable4, c uint16) (GID, bool) {
	segs := len(sub.EndCode)
	for i := range segs {
		if c > sub.EndCode[i] {
			continue
		}
		if c < sub.StartCode[i] || sub.StartCode[i] == 0xffff {
			return 0, false
		}
		if sub.IdRangeOffsets[i] == 0 {
			return GID(c + sub.IdDelta[i]), true
		}
		index := int(sub.IdRangeOffsets[i])/2 + i - segs + int(c-sub.StartCode[i])
		g := binary.BigEndian.Uint16(sub.GlyphIDArray[2*index:])
		if g == 0 {
			return 0, false
		}
		return GID(g + sub.IdDelta[i]), true
	}
	return 0, false
}
