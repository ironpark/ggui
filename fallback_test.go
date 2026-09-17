package ggui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

func TestKoreanFallsBackToASystemFont(t *testing.T) {
	if len(SystemFonts()) == 0 {
		t.Skip("no system CJK font on this machine")
	}
	face := fallbackFont().face(14)
	if _, ok := face.(*text.MultiFace); !ok {
		t.Fatalf("default face is %T, want a MultiFace with system fallbacks", face)
	}
	if text.Advance("한글", face) <= 0 {
		t.Fatal("Korean measures as zero width: no glyphs")
	}
	bare := MustFont(goregular.TTF).NoFallback()
	if bare.face(14).(*text.GoTextFace) == nil || text.Advance("한글", bare.face(14)) >= text.Advance("한글", face) {
		t.Fatal("NoFallback still draws Korean, or the fallback did not change the measurement")
	}
}
