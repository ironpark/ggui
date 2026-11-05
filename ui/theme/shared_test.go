package theme_test

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/theme"
)

func TestOfSharesOneThemePerValue(t *testing.T) {
	t.Parallel()
	a := theme.Of(theme.Default().Apply(ggui.Env{}))
	b := theme.Of(theme.Default().Apply(ggui.Env{}))
	if a != b {
		t.Fatal("equal themes applied twice do not share a pointer")
	}
	if dark := theme.Of(theme.Dark().Apply(ggui.Env{})); dark == a || dark.Bg == a.Bg {
		t.Fatal("a different theme shares the pointer")
	}
	if theme.Of(ggui.Env{}) != theme.Of(ggui.Env{}) {
		t.Fatal("the default theme is made again for every Env without one")
	}
	if *theme.Of(ggui.Env{}) != theme.Default() {
		t.Fatal("Of without a theme is not Default")
	}
}

func TestFromCopiesWhatOfShares(t *testing.T) {
	t.Parallel()
	env := theme.Default().Apply(ggui.Env{})
	copied := theme.From(env)
	copied.Primary = color.RGBA{1, 2, 3, 255}
	if theme.Of(env).Primary == copied.Primary {
		t.Fatal("changing From's copy changed the applied theme")
	}
}

func BenchmarkFromWithoutATheme(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = theme.From(ggui.Env{})
	}
}

// Applying a theme already applied, as a theme.With subtree does at every
// layout.
func BenchmarkApply(b *testing.B) {
	t := theme.Default()
	env := ggui.Env{}
	t.Apply(env)
	b.ReportAllocs()
	for b.Loop() {
		t.Apply(env)
	}
}

// More distinct themes than any small cache, applied in turn under one
// parent, each keep their pointer and Env.
func TestManyThemesKeepTheirEnvs(t *testing.T) {
	base := ggui.Env{}.With(ggui.SpacingKey, 5.)
	envs := make([]ggui.Env, 48)
	for round := range 3 {
		for i := range envs {
			th := theme.Default()
			th.Space = float64(i + 1)
			env := th.Apply(base)
			if round > 0 && theme.Of(env) != theme.Of(envs[i]) {
				t.Fatalf("theme %d got a new pointer in round %d", i, round)
			}
			envs[i] = env
		}
	}
}
