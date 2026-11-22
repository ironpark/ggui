package ggui

import (
	"image"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/framestats"
	"github.com/ironpark/ggui/internal/render"
)

// TextureKind names a category of retained GPU image tracked by ggui's
// texture memory accounting (reported by inspect.ReadTextureBytes).
type TextureKind uint8

const (
	// TextureIcons accounts for rasterized icon masks retained on the GPU.
	TextureIcons TextureKind = TextureKind(framestats.Icons)
	// TextureImages accounts for decoded images retained on the GPU.
	TextureImages TextureKind = TextureKind(framestats.Images)
)

// UploadTexture uploads img to a GPU image in format, allocating an unmanaged
// texture when either dimension exceeds ggui's atlas threshold. Pass
// ggfx.PixelFormatAlpha to keep only img's alpha channel for tinted masks.
func UploadTexture(img image.Image, format ggfx.PixelFormat) *ggfx.Image {
	return render.NewTextureOf(img, format)
}

// ImageBytes returns the byte footprint ggui's texture accounting attributes
// to a w×h image of format: one byte per pixel for ggfx.PixelFormatAlpha,
// four for RGBA.
func ImageBytes(w, h int, format ggfx.PixelFormat) int64 {
	return framestats.ImageBytesOf(w, h, format)
}

// AccountTexture records delta bytes of kind retained (positive) or released
// (negative) in ggui's texture memory accounting. Unknown kinds are ignored.
func AccountTexture(kind TextureKind, delta int64) {
	switch kind {
	case TextureIcons, TextureImages:
		framestats.AddTexture(framestats.Texture(kind), delta)
	}
}
