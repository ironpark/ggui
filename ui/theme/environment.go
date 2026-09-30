package theme

import (
	"image/color"

	"github.com/ironpark/ggui"
)

var key = ggui.NewEnvKey[Theme]("ui theme")

// TitleKey and CaptionKey hold named text styles for layout-time resolution.
var (
	TitleKey   = ggui.NewEnvKey[ggui.TextStyle]("ui title")
	CaptionKey = ggui.NewEnvKey[ggui.TextStyle]("ui caption")
)

// Importing theme installs the default UI environment. Applications can replace
// it with Set, Bind, or ggui.SetEnv before their first frame.
func init() { Set(Default()) }

// From returns the theme inherited by env, or the default light theme.
func From(env ggui.Env) Theme {
	if t, ok := env.Get(key); ok {
		return t
	}
	return Default()
}

// Apply supplies this theme and the corresponding core styles to env.
// Unrelated environment values, including accessibility preferences, survive.
func (t Theme) Apply(env ggui.Env) ggui.Env {
	return env.With(key, t).WithText(t.Text).
		With(TitleKey, t.Title).With(CaptionKey, t.Caption).
		With(ggui.BackgroundKey, t.Bg).With(ggui.SpacingKey, t.Space).
		With(ggui.EditorStyleKey, ggui.EditorStyle{Muted: t.MutedFg, Selection: t.Selection}).
		With(ggui.ScrollStyleKey, ggui.ScrollStyle{Color: t.MutedFg, HoverColor: t.Fg, Duration: t.MotionFast}).
		With(ggui.PopupDurationKey, t.MotionFast)
}

// Use reads the application theme and subscribes the current builder or effect.
func Use() Theme { return From(ggui.UseEnv()) }

// Set replaces the application theme while preserving other environment
// values, and matches the windows' title bars to it.
func Set(t Theme) {
	ggui.SetEnv(t.Apply(ggui.Untrack(ggui.UseEnv).WithTextReplaced(t.Text)))
	if t.Bg != nil {
		ggui.SetAppearance(appearanceOf(t.Bg))
	}
}

// appearanceOf is the platform appearance that suits a theme whose
// background is bg: dark when bg is closer to black than to white.
func appearanceOf(bg color.Color) ggui.Appearance {
	r, g, b, _ := bg.RGBA()
	if r+g+b < 3*0x8000 {
		return ggui.AppearanceDark
	}
	return ggui.AppearanceLight
}

// Bind applies on while sw is true and off otherwise through a Watch.
// Its first application runs when effects flush; the returned function stops it.
func Bind(sw ggui.Readable[bool], on, off Theme) func() {
	return ggui.Watch(sw, func(v bool) {
		if v {
			Set(on)
		} else {
			Set(off)
		}
	})
}

// With applies a theme to a subtree at layout time.
func With(t Theme, child ggui.Widget) *ggui.EnvWidget {
	return ggui.WithEnv(t.Apply, child)
}
