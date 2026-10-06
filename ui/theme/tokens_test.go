package theme_test

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func same(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar>>8 == br>>8 && ag>>8 == bg>>8 && ab>>8 == bb>>8 && aa>>8 == ba>>8
}

// A token is the color of the theme it is resolved under: a field, a field
// with no CSS variable, a variable an app added, and any of them faded.
func TestColorResolvesUnderItsTheme(t *testing.T) {
	light, dark := uitheme.Default().Resolve(), uitheme.Dark().Resolve()
	warning := color.RGBA{200, 150, 0, 255}
	dark = dark.Set(uitheme.CSSColor("warning"), color.Color(warning))
	light.Chart[1] = color.RGBA{10, 20, 30, 255}
	for _, tc := range []struct {
		token uitheme.Color
		in    uitheme.Theme
		want  color.Color
	}{
		{uitheme.Primary, light, light.Primary},
		{uitheme.Primary, dark, dark.Primary},
		{uitheme.MutedFg, dark, dark.MutedFg},
		{uitheme.Selection, dark, dark.Selection},
		{uitheme.Var("chart-2"), light, light.Chart[1]},
		{uitheme.Var("warning"), dark, warning},
		{uitheme.Primary.Alpha(.5), dark, uitheme.Fade(dark.Primary, .5)},
		{uitheme.Primary.Alpha(.5).Alpha(.5), dark, uitheme.Fade(dark.Primary, .25)},
	} {
		if got := tc.token.In(tc.in); !same(got, tc.want) {
			t.Errorf("%v: got %v, want %v", tc.token, got, tc.want)
		}
	}
	if got := uitheme.Var("warning").In(light); got != nil {
		t.Errorf("a variable the theme lacks gave %v", got)
	}
	if uitheme.Fade(uitheme.Primary, .5) != uitheme.Primary.Alpha(.5) {
		t.Error("Fade of a token is not that token at the alpha")
	}
	mixed := uitheme.Mix(uitheme.Primary, color.White, .5)
	if got, want := ggui.ResolveColor(mixed, dark.Apply(ggui.Env{})), uitheme.Mix(dark.Primary, color.White, .5); !same(got, want) {
		t.Errorf("Mix of a token resolved to %v, want %v", got, want)
	}
}

// A token resolves in the new theme when the theme switches, while the
// builder that used it runs once.
func TestTokensFollowAThemeSwitchWithoutARebuild(t *testing.T) {
	dark := ggui.State(false)
	builds := 0
	var seen color.Color
	p := ggui.ProbeBuilder(func() ggui.Widget {
		builds++
		return ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
			seen = ggui.ResolveColor(uitheme.Primary, env)
			return c.Constrain(ggui.Sz(10, 10))
		}, func(*ggui.Canvas, ggui.Rect) {})
	}, ggui.Sz(40, 40))
	defer p.Close()
	p.Setup(func() { uitheme.Bind(dark, uitheme.Dark(), uitheme.Default()) })
	for _, d := range []bool{false, true, false} {
		dark.Set(d)
		p.Frame()
		want := uitheme.Default().Primary
		if d {
			want = uitheme.Dark().Primary
		}
		if !same(seen, want) {
			t.Errorf("dark=%v: resolved to %v, want %v", d, seen, want)
		}
	}
	if builds != 1 {
		t.Errorf("built %d times, want 1", builds)
	}
}
