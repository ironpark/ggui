package icns

import (
	"image"
	"image/draw"
	"math"
)

// Apple's icon grid: on a 1024-pixel canvas, a macOS app icon's body is
// 824 pixels square, centred, leaving 100 pixels on each side for the
// shadow and the room the Dock and Finder expect between icons. An icon
// drawn to the canvas's edges looks larger than every other in the Dock.
const (
	gridCanvas = 1024
	gridBody   = 824
)

// Inset returns src, which must be square, scaled into the body of
// Apple's icon grid on a transparent canvas of src's size: 824 of 1024
// pixels, centred. It is for artwork drawn edge to edge, as a Windows
// icon is; the body's shape, the rounded square macOS icons share, is
// the artwork's to have.
func Inset(src image.Image) *image.NRGBA {
	size := src.Bounds().Dx()
	body := int(math.Round(float64(size) * gridBody / gridCanvas))
	off := (size - body) / 2
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(dst, image.Rect(off, off, off+body, off+body), Resize(src, body), image.Point{}, draw.Src)
	return dst
}

// FullBleed reports whether src's artwork reaches the edges of its canvas,
// where an icon inset in Apple's grid has nothing but its shadow: a pixel
// at least half opaque within 1/32 of the size of an edge, 32 pixels of
// the 100 the grid leaves at 1024.
func FullBleed(src image.Image) bool {
	b := src.Bounds()
	band := max(1, b.Dx()/32)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		edgeRow := y < b.Min.Y+band || y >= b.Max.Y-band
		for x := b.Min.X; x < b.Max.X; x++ {
			if !edgeRow && x >= b.Min.X+band && x < b.Max.X-band {
				x = b.Max.X - band - 1 // skip the middle of the row
				continue
			}
			if _, _, _, a := src.At(x, y).RGBA(); a >= 0x8000 {
				return true
			}
		}
	}
	return false
}
