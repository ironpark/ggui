package theme_test

import (
	"slices"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/theme"
)

func TestModeChoosesLightDarkOrTheSystems(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		mode         theme.Mode
		system, dark bool
		label        string
	}{
		{theme.ModeLight, true, false, "Light"}, {theme.ModeDark, false, true, "Dark"},
		{theme.ModeSystem, true, true, "System"}, {theme.ModeSystem, false, false, "System"},
		{"", true, true, "System"},
	} {
		if got := tc.mode.Dark(tc.system); got != tc.dark || tc.mode.String() != tc.label {
			t.Errorf("%q with a system dark=%v: dark=%v, label %q", string(tc.mode), tc.system, got, tc.mode)
		}
	}
	if !slices.Equal(theme.Modes(), []theme.Mode{theme.ModeLight, theme.ModeDark, theme.ModeSystem}) {
		t.Errorf("modes %v", theme.Modes())
	}
}

// BindMode applies the theme the mode names; under a Probe the desktop is
// light, so System is the light theme.
func TestBindModeAppliesTheChosenAppearance(t *testing.T) {
	t.Parallel()
	preserveEnv(t)
	mode := ggui.State(theme.ModeDark)
	recipe := theme.Config{Preset: theme.Preset{Accent: theme.AccentBlue}}
	var seen theme.Theme
	p := ggui.NewProbe(ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
		seen = theme.From(env)
		return c.Constrain(ggui.Sz(10, 10))
	}, func(*ggui.Canvas, ggui.Rect) {}), ggui.Sz(20, 20)).Setup(func() {
		theme.BindMode(mode, recipe.Dark(), recipe.Light())
	})
	defer p.Close()
	for _, tc := range []struct {
		mode theme.Mode
		want theme.Theme
	}{{theme.ModeDark, recipe.Dark()}, {theme.ModeLight, recipe.Light()}, {theme.ModeDark, recipe.Dark()}, {theme.ModeSystem, recipe.Light()}} {
		mode.Set(tc.mode)
		p.Frame()
		if seen != tc.want {
			t.Fatalf("mode %v drew with Bg %v, want %v", tc.mode, seen.Bg, tc.want.Bg)
		}
	}
}
