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
func NewTexture(img image.Image) *ggfx.Image { return NewTextureOf(img, ggfx.PixelFormatRGBA) }

// NewTextureOf is NewTexture in format f: [ggfx.PixelFormatAlpha] keeps
// only img's alpha, for a white mask to tint.
func NewTextureOf(img image.Image, f ggfx.PixelFormat) *ggfx.Image {
	op := &ggfx.NewImageFromImageOptions{Format: f}
	if b := img.Bounds(); Large(b.Dx(), b.Dy()) {
		op.Unmanaged = true
	}
	return ggfx.NewImageFromImageWithOptions(img, op)
}

// NewBlank makes a clear w×h image to draw into, a texture of exactly its
// size when it is large.
func NewBlank(w, h int) *ggfx.Image { return NewBlankOf(w, h, ggfx.PixelFormatRGBA) }

// NewBlankOf is NewBlank in format f: [ggfx.PixelFormatAlpha] for white
// coverage drawn into it, such as a mask.
func NewBlankOf(w, h int, f ggfx.PixelFormat) *ggfx.Image {
	return ggfx.NewImageWithOptions(image.Rect(0, 0, w, h), &ggfx.NewImageOptions{Unmanaged: Large(w, h), Format: f})
}
