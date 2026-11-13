package render

import (
	"bytes"
	"image"
	"image/draw"
	"io"

	"github.com/ironpark/ggfx"
	ggsvg "github.com/ironpark/ggfx/svg"
	"github.com/ironpark/ggui/internal/third_party/typesetting/font"
	"github.com/ironpark/ggui/internal/third_party/typesetting/shaping"
)

func (r *Renderer) drawSVG(g shaping.Glyph, svg font.GlyphSVG, img draw.Image, x, y float32) error {
	pixWidth := int(fixed266ToFloat(g.Width) * r.PixScale)
	pixHeight := int(fixed266ToFloat(-g.Height) * r.PixScale)
	pix, err := renderSVGStream(bytes.NewReader(svg.Source), pixWidth, pixHeight)
	if err != nil {
		return err
	}

	rect := image.Rect(int(fixed266ToFloat(g.XBearing)*r.PixScale), int(fixed266ToFloat(-g.YBearing)*r.PixScale),
		pixWidth, pixHeight)
	draw.Draw(img, rect.Add(image.Point{X: int(x), Y: int(y)}), pix, image.Point{}, draw.Over)

	// ignore the svg.Outline shapes, as they are a fallback which we won't use
	return nil
}

func renderSVGStream(stream io.Reader, width, height int) (*image.RGBA, error) {
	doc, err := ggsvg.Parse(stream)
	if err != nil {
		return nil, err
	}

	vx, vy, vw, vh := doc.ViewBox()
	iconAspect := float32(vw / vh)
	viewAspect := float32(width) / float32(height)
	imgW, imgH := width, height
	if viewAspect > iconAspect {
		imgW = int(float32(height) * iconAspect)
	} else if viewAspect < iconAspect {
		imgH = int(float32(width) / iconAspect)
	}

	var g ggfx.GeoM
	g.Translate(-vx, -vy)
	g.Scale(float64(imgW)/vw, float64(imgH)/vh)

	out := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	doc.Draw(out, g)
	return out, nil
}
