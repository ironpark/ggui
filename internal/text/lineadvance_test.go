package text

import (
	"bytes"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/ironpark/ggui/internal/text/internal/testfonts"
)

// The advance a line's glyphs add up to is the one advanceAt finds for
// its end, which a MultiFace placed each face's run by.
func TestALinesGlyphsAdvanceAsAdvanceAtSays(t *testing.T) {
	for _, data := range [][]byte{goregular.TTF, testfonts.MPlus1pRegular_ttf, testfonts.TerminusTTF_ttf} {
		src, err := NewGoTextFaceSource(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		for _, size := range []float64{11, 13.5, 26} {
			for _, dir := range []Direction{DirectionLeftToRight, DirectionRightToLeft} {
				f := &GoTextFace{Source: src, Size: size, Direction: dir}
				for _, line := range []string{"", "a", "Hello, world", "fi fl ffi", "안녕하세요 세계", "日本語のテキスト", "é ä", "مرحبا بالعالم", "mixed مرحبا text", "tab\tand  spaces", "line\nbreak"} {
					_, got := f.appendLazyGlyphsForLineAdvance(nil, line, 0, 0, 0, glyphCull{})
					if want := f.advanceAt(line, len(line)); got != want {
						t.Errorf("%s at %v %v: glyphs advance %v, advanceAt %v", line, size, dir, got, want)
					}
				}
			}
		}
	}
}

// BenchmarkMultiFaceLine lays a line out that two faces share, as Latin
// and Hangul in a fallback chain do, as each frame that draws it does.
func BenchmarkMultiFaceLine(b *testing.B) {
	latin, _ := NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	cjk, _ := NewGoTextFaceSource(bytes.NewReader(testfonts.MPlus1pRegular_ttf))
	m, err := NewMultiFace(&GoTextFace{Source: latin, Size: 13}, &GoTextFace{Source: cjk, Size: 13})
	if err != nil {
		b.Fatal(err)
	}
	line := "The cache keeps 日本語 sources once, その後 in the cache that read them 日本."
	var glyphs []LazyGlyph
	b.ReportAllocs()
	for b.Loop() {
		glyphs = m.appendLazyGlyphsForLine(glyphs[:0], line, 0, 0, 0, glyphCull{})
	}
}
