package ggui

import (
	"testing"

	"github.com/ironpark/ggfx/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

func TestKoreanFallsBackToASystemFont(t *testing.T) {
	t.Parallel()
	if len(SystemFonts()) == 0 {
		t.Skip("no system CJK font on this machine")
	}
	face := fallbackFont().face(14, 0)
	if ef, ok := face.(*emojiFace); ok {
		face = ef.Face
	}
	if _, ok := face.(*text.MultiFace); !ok {
		t.Fatalf("default face is %T, want a MultiFace with system fallbacks", face)
	}
	if lineWidth("한글", face) <= 0 {
		t.Fatal("Korean measures as zero width: no glyphs")
	}
	bare := MustFont(goregular.TTF).NoFallback()
	if bare.face(14, 0).(*text.GoTextFace) == nil || lineWidth("한글", bare.face(14, 0)) >= lineWidth("한글", face) {
		t.Fatal("NoFallback still draws Korean, or the fallback did not change the measurement")
	}
}
