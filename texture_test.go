package ggui

import (
	"image"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/framestats"
)

func TestTextureUploadAndAccounting(t *testing.T) {
	if got := ImageBytes(16, 8, ggfx.PixelFormatAlpha); got != 128 {
		t.Fatalf("ImageBytes alpha = %d, want 128", got)
	}
	if got := ImageBytes(16, 8, ggfx.PixelFormatRGBA); got != 512 {
		t.Fatalf("ImageBytes RGBA = %d, want 512", got)
	}

	src := image.NewRGBA(image.Rect(0, 0, 12, 12))
	alpha := UploadTexture(src, ggfx.PixelFormatAlpha)
	defer alpha.Deallocate()
	if alpha.Format() != ggfx.PixelFormatAlpha {
		t.Fatalf("alpha texture format = %v, want %v", alpha.Format(), ggfx.PixelFormatAlpha)
	}

	rgba := UploadTexture(src, ggfx.PixelFormatRGBA)
	defer rgba.Deallocate()
	if rgba.Format() != ggfx.PixelFormatRGBA {
		t.Fatalf("rgba texture format = %v, want %v", rgba.Format(), ggfx.PixelFormatRGBA)
	}

	before := framestats.ReadTextures()
	AccountTexture(TextureIcons, 256)
	AccountTexture(TextureImages, 1024)
	AccountTexture(TextureKind(99), 4096) // unknown kind ignored safely
	during := framestats.ReadTextures()
	if during.Icons-before.Icons != 256 {
		t.Fatalf("Icons delta = %d, want 256", during.Icons-before.Icons)
	}
	if during.Images-before.Images != 1024 {
		t.Fatalf("Images delta = %d, want 1024", during.Images-before.Images)
	}
	AccountTexture(TextureIcons, -256)
	AccountTexture(TextureImages, -1024)
	after := framestats.ReadTextures()
	if after.Icons != before.Icons || after.Images != before.Images {
		t.Fatalf("accounting did not balance: before=%+v after=%+v", before, after)
	}
}
