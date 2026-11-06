package text

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/bitmapfont/v4"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/third_party/text/internal/testfonts"
)

// A monochrome glyph's image is alpha-only, a color glyph's RGBA.
func TestGlyphImageFormats(t *testing.T) {
	latin, err := NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	formats := func(name string, face Face, s string, want ggfx.PixelFormat) {
		t.Helper()
		n := 0
		for _, g := range AppendLazyGlyphs(nil, s, face, nil) {
			img := g.Image()
			if img == nil {
				continue
			}
			n++
			if got := img.Format(); got != want {
				t.Errorf("%s: glyph %d of format %v, want %v", name, g.GID, got, want)
			}
		}
		if n == 0 {
			t.Errorf("%s: no glyph images", name)
		}
	}
	formats("outline", &GoTextFace{Source: latin, Size: 16}, "Hello, world", ggfx.PixelFormatAlpha)
	formats("GoXFace", NewGoXFace(bitmapfont.Face), "Hello", ggfx.PixelFormatAlpha)
	for _, c := range []struct{ font, s string }{
		{"chromacheck-svg.ttf", ""},
		{"chromacheck-cbdt.ttf", ""},
		{"chromacheck-sbix.ttf", ""},
		{"chromacheck-colr.ttf", ""},
	} {
		data, err := os.ReadFile(filepath.Join("testdata", c.font))
		if err != nil {
			t.Fatal(err)
		}
		src, err := NewGoTextFaceSource(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		formats(c.font, &GoTextFace{Source: src, Size: 16}, c.s, ggfx.PixelFormatRGBA)
	}
}

// A monochrome glyph's alpha image draws, tinted, scaled and at a subpixel
// offset, exactly as the white RGBA mask it was before.
func TestAlphaGlyphDrawsAsWhiteRGBA(t *testing.T) {
	for _, data := range [][]byte{goregular.TTF, testfonts.MPlus1pRegular_ttf} {
		src, err := NewGoTextFaceSource(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		face := &GoTextFace{Source: src, Size: 13}
		for _, g := range AppendLazyGlyphs(nil, "Ag 日本", face, &LayoutOptions{}) {
			img := g.Image()
			if img == nil {
				continue
			}
			if img.Format() != ggfx.PixelFormatAlpha {
				t.Fatalf("glyph %d of format %v", g.GID, img.Format())
			}
			ref := ggfx.NewImageFromImage(g.Pixels())
			for _, filter := range []ggfx.Filter{ggfx.FilterNearest, ggfx.FilterLinear} {
				draw := func(src *ggfx.Image) []byte {
					dst := ggfx.NewImage(64, 64)
					defer dst.Deallocate()
					op := &ggfx.DrawImageOptions{Filter: filter}
					op.GeoM.Scale(1.5, 1.5)
					op.GeoM.Translate(3.3, 4.7)
					op.ColorScale.Scale(0.2, 0.5, 0.7, 0.8)
					dst.DrawImage(src, op)
					pix := make([]byte, 4*64*64)
					dst.ReadPixels(pix)
					return pix
				}
				got, want := draw(img), draw(ref)
				worst, at := 0, -1
				for i := range got {
					if d := int(got[i]) - int(want[i]); max(d, -d) > worst {
						worst, at = max(d, -d), i
					}
				}
				// Nearest is exact; linear may round a channel the other way.
				if tolerance := map[ggfx.Filter]int{ggfx.FilterLinear: 1}[filter]; worst > tolerance {
					t.Errorf("glyph %d, filter %v: the alpha image draws other than the RGBA mask, by %d at (%d, %d) ch %d: %v vs %v", g.GID, filter, worst, at/4%64, at/4/64, at%4, got[at/4*4:at/4*4+4], want[at/4*4:at/4*4+4])
				}
			}
			ref.Deallocate()
		}
	}
}

// The alpha pool lets go of a buffer past a glyph 128 pixels square, as the
// RGBA pool does.
func TestPooledAlpha(t *testing.T) {
	a := newPooledAlpha(10, 7)
	if a.Stride != 10 || a.Rect != image.Rect(0, 0, 10, 7) || len(a.Pix) != 70 {
		t.Fatalf("pooled alpha: stride %d rect %v len %d", a.Stride, a.Rect, len(a.Pix))
	}
	a.Pix[3] = 0xff
	releasePooledAlpha(a)
	b := newPooledAlpha(10, 7)
	for i, p := range b.Pix {
		if p != 0 {
			t.Fatalf("a pooled alpha not clear at %d", i)
		}
	}
	releasePooledAlpha(b)
	big := newPooledAlpha(200, 200)
	releasePooledAlpha(big)
	if c := newPooledAlpha(200, 200); &c.Pix[0] == &big.Pix[0] {
		t.Fatal("the pool kept a buffer past its cap")
	}
}
