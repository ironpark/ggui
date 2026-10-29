package ggui

import (
	"slices"
	"testing"

	"github.com/ironpark/ggui/internal/text"
	"golang.org/x/image/font/gofont/goregular"
)

func TestKoreanFallsBackToASystemFont(t *testing.T) {
	t.Parallel()
	if !systemFontDrawsKorean() {
		t.Skip("no system font draws Korean on this machine")
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

// systemFontDrawsKorean reports whether a system font has glyphs for
// Korean. A machine can have system fonts and none of them for Korean, as
// a Linux one with DejaVu Sans alone does, where the fallback draws the
// same .notdef boxes as the built-in font.
func systemFontDrawsKorean() bool {
	for _, f := range SystemFonts() {
		glyphs := text.AppendGlyphs(nil, "한글", f.face(14, 0), nil)
		if len(glyphs) > 0 && !slices.ContainsFunc(glyphs, func(g text.Glyph) bool { return g.GID == 0 }) {
			return true
		}
	}
	return false
}

// TestGlyphsNoListedFontHasComeFromAnyFontThatDoes draws a rune none of
// SystemFonts has, which the default font finds in whichever system font
// has it.
func TestGlyphsNoListedFontHasComeFromAnyFontThatDoes(t *testing.T) {
	t.Parallel()
	drawn := func(s string, face text.Face) bool {
		glyphs := text.AppendGlyphs(nil, s, face, nil)
		return len(glyphs) > 0 && !slices.ContainsFunc(glyphs, func(g text.Glyph) bool { return g.GID == 0 })
	}
	var s string
	for _, r := range []rune{'𓀀', 'ᚠ', 'Ꭰ', 'ⴰ', 'ሀ', 'ก', 'ა', 'अ'} {
		if systemFallback(r) == nil || slices.ContainsFunc(SystemFonts(), func(f *Font) bool { return drawn(string(r), f.face(14, 0)) }) {
			continue
		}
		s = string(r)
		break
	}
	if s == "" {
		t.Skip("no system font has a rune that the listed ones lack")
	}
	face := fallbackFont().face(14, 0)
	if ef, ok := face.(*emojiFace); ok {
		face = ef.Face
	}
	if !drawn(s, face) {
		t.Fatalf("%q draws .notdef, though %s has it", s, systemFallback([]rune(s)[0]).meta().Metadata().Family)
	}
	if !drawn("a"+s+"b", face) || lineWidth("a"+s+"b", face) <= lineWidth("ab", face) {
		t.Fatalf("%q between Latin letters is not drawn", s)
	}
}
