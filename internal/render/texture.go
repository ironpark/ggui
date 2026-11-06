package render

import (
	"image"

	"github.com/ironpark/ggfx"
)

// LargeImage is the side past which an image is a texture of its own: the
// atlas would round it up to a page of powers of two, 2048×2048 and 16 MB
// for a 1254-pixel square, and it packs badly with the small images the
// atlas batches.
const LargeImage = 512

// Large reports whether a w×h image is past LargeImage on a side.
func Large(w, h int) bool { return w > LargeImage || h > LargeImage }

// NewTexture uploads img, a texture of exactly its size when it is large.
func NewTexture(img image.Image) *ggfx.Image {
	if b := img.Bounds(); Large(b.Dx(), b.Dy()) {
		return ggfx.NewImageFromImageWithOptions(img, &ggfx.NewImageFromImageOptions{Unmanaged: true})
	}
	return ggfx.NewImageFromImage(img)
}

// NewBlank makes a clear w×h image to draw into, a texture of exactly its
// size when it is large.
func NewBlank(w, h int) *ggfx.Image {
	if Large(w, h) {
		return ggfx.NewImageWithOptions(image.Rect(0, 0, w, h), &ggfx.NewImageOptions{Unmanaged: true})
	}
	return ggfx.NewImage(w, h)
}
