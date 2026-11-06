package text

import (
	"bytes"
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/math/fixed"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/third_party/text/internal/testfonts"
)

// BenchmarkOutlineGlyphImage rasterizes a line's outline glyphs and uploads
// each, as a glyph cache miss does.
func BenchmarkOutlineGlyphImage(b *testing.B) {
	for _, c := range []struct {
		name string
		data []byte
		size float64
	}{{"latin13", goregular.TTF, 13}, {"cjk32", testfonts.MPlus1pRegular_ttf, 32}} {
		b.Run(c.name, func(b *testing.B) {
			src, err := NewGoTextFaceSource(bytes.NewReader(c.data))
			if err != nil {
				b.Fatal(err)
			}
			face := &GoTextFace{Source: src, Size: c.size}
			var rds []*glyphRenderData
			for _, g := range AppendLazyGlyphs(nil, "The quick 日本語のテキスト fox", face, nil) {
				if g.imageArgs.glyph != nil && g.imageArgs.glyph.render != nil && len(g.imageArgs.glyph.render.segments()) > 0 {
					rds = append(rds, g.imageArgs.glyph.render)
				}
			}
			// A glyph each format keeps its atlas page, as the glyph cache
			// does: without one, the page would go with each glyph freed.
			for _, f := range []ggfx.PixelFormat{ggfx.PixelFormatRGBA, ggfx.PixelFormatAlpha} {
				keep := ggfx.NewImageFromImageWithOptions(image.NewAlpha(image.Rect(0, 0, 4, 4)), &ggfx.NewImageFromImageOptions{Format: f})
				defer keep.Deallocate()
			}
			off := fixed.Point26_6{X: 16}
			b.ReportAllocs()
			for b.Loop() {
				for _, rd := range rds {
					if img := segmentsToImage(rd.segments(), off, rd.bounds); img != nil {
						img.Deallocate()
					}
				}
			}
		})
	}
}
