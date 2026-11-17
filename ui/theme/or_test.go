package theme

import (
	"image/color"
	"testing"
)

// A Var with a fallback paints the theme's color by its name where the
// theme has one, and the fallback, faded as the color would be, where it
// has none; a fallback that is a token is the same theme's.
func TestOrStandsInForAColorTheThemeLacks(t *testing.T) {
	own := color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff}
	fallback := color.RGBA{R: 0xff, A: 0xff}
	bare := Default()
	made := bare.Set(CSSColor("brand"), color.Color(own))
	brand := Var("brand").Or(fallback)
	if got := brand.In(made); got != color.Color(own) {
		t.Fatalf("under a theme with the color: %v", got)
	}
	if got := brand.In(bare); got != color.Color(fallback) {
		t.Fatalf("under one without: %v", got)
	}
	if got := Var("brand").In(bare); got != nil {
		t.Fatalf("with no fallback: %v", got)
	}
	if got, want := Var("brand").Or(Primary).In(bare), Primary.In(bare); got != want {
		t.Fatalf("a token's fallback is %v, not the theme's %v", got, want)
	}
	if got := brand.Alpha(.5).In(bare); got == color.Color(fallback) || got == nil {
		t.Fatalf("faded, the fallback is %v", got)
	}
}
