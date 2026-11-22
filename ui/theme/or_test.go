package theme

import (
	"image/color"
	"math"
	"testing"

	"github.com/ironpark/ggui"
)

type customEnvColor struct {
	key ggui.EnvKey[color.Color]
}

func (c customEnvColor) Resolve(env ggui.Env) color.Color {
	if v, ok := env.Get(c.key); ok {
		return v
	}
	return color.RGBA{R: 1, G: 2, B: 3, A: 255}
}

func (c customEnvColor) RGBA() (r, g, b, a uint32) { return 0, 0, 0, 0 }

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
	// Any ggui.EnvColor fallback resolves against the Env (and Theme).
	k := ggui.NewEnvKey[color.Color]("test-custom-fallback")
	envColor := color.RGBA{R: 0x44, G: 0x55, B: 0x66, A: 0xff}
	env := bare.Apply(ggui.Env{}).With(k, color.Color(envColor))
	if got := Var("brand").Or(customEnvColor{key: k}).Resolve(env); got != color.Color(envColor) {
		t.Fatalf("custom EnvColor fallback resolved to %v, want %v", got, envColor)
	}
	mixFb := Mix(Primary, Bg, 0.5)
	if got, want := Var("brand").Or(mixFb).Resolve(env), ggui.ResolveColor(mixFb, env); got != want {
		t.Fatalf("mixed fallback resolved to %v, want %v", got, want)
	}
}

func TestFadeNaNAndTwoGenerationCache(t *testing.T) {
	var base color.Color = color.RGBA{R: 100, G: 150, B: 200, A: 255}
	got := Fade(base, math.NaN())
	r, g, b, a := got.RGBA()
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Fatalf("Fade(base, NaN) = (%d,%d,%d,%d), want transparent", r, g, b, a)
	}
	static := Fade(base, 0.5)
	// Churn through more than mostFades unique opacities while keeping static warm.
	for i := range mostFades * 3 {
		_ = Fade(base, 0.01+float64(i)*0.0001)
		if i%64 == 0 {
			_ = Fade(base, 0.5)
		}
	}
	// A warm static color should still hit the cache (zero allocs).
	if allocs := testing.AllocsPerRun(20, func() {
		if Fade(base, 0.5) != static {
			t.Fatal("faded color mismatch")
		}
	}); allocs != 0 {
		t.Fatalf("warm Fade allocated %v times after opacity churn, want 0", allocs)
	}
}
