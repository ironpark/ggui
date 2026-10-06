package theme

import (
	"image/color"

	"github.com/ironpark/ggui"
)

var key = ggui.NewEnvKey[Theme]("ui theme")

// TitleKey, HeadingKey, LabelKey, CaptionKey and MonoKey hold the theme's
// named text styles for layout-time resolution, as TextWidget.StyleKey
// takes them.
var (
	TitleKey   = ggui.NewEnvKey[ggui.TextStyle]("ui title")
	HeadingKey = ggui.NewEnvKey[ggui.TextStyle]("ui heading")
	LabelKey   = ggui.NewEnvKey[ggui.TextStyle]("ui label")
	CaptionKey = ggui.NewEnvKey[ggui.TextStyle]("ui caption")
	MonoKey    = ggui.NewEnvKey[ggui.TextStyle]("ui mono")
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

// Apply supplies this theme, resolved, and the corresponding core styles to
// env. Unrelated environment values, including accessibility preferences,
// survive.
func (t Theme) Apply(env ggui.Env) ggui.Env {
	t = t.Resolve()
	return env.With(key, t).WithText(t.Text).
		With(TitleKey, t.Title).With(HeadingKey, t.Heading).With(LabelKey, t.Label).
		With(CaptionKey, t.Caption).With(MonoKey, t.Mono).
		With(ggui.BackgroundKey, t.Bg).With(ggui.SpacingKey, t.Space).
		With(ggui.EditorStyleKey, ggui.EditorStyle{Muted: t.MutedFg, Selection: t.Selection}).
		With(ggui.ScrollStyleKey, ggui.ScrollStyle{Color: t.MutedFg, HoverColor: t.Fg, Duration: t.MotionFast}).
		With(ggui.PopupDurationKey, t.MotionFast)
}

// Use reads the application theme and subscribes the current Reactive or
// effect. A builder colors what it builds with tokens such as Primary
// instead: read here, the theme is a snapshot a theme switch leaves behind.
func Use() Theme { return From(ggui.UseEnv()) }

// Set replaces the application theme while preserving other environment
// values, and matches the windows' title bars to it.
func Set(t Theme) {
	t = t.Resolve()
	ggui.SetEnv(t.Apply(ggui.Untrack(ggui.UseEnv).WithTextReplaced(t.Text)))
	ggui.SetAppearance(appearanceOf(t.Bg))
}

// appearanceOf is the platform appearance that suits a theme whose
// background is bg: dark when bg is closer to black than to white.
func appearanceOf(bg color.Color) ggui.Appearance {
	if isDark(bg) {
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

// Override changes some tokens of the theme the subtree inherits, as a CSS
// variable set on an element does. Tokens that follow the ones fn changes
// follow them here too:
//
//	theme.Override(func(t *theme.Theme) { t.Primary = brand }, form)
func Override(fn func(*Theme), child ggui.Widget) *ggui.EnvWidget {
	return ggui.WithEnv(func(env ggui.Env) ggui.Env {
		t := From(env)
		fn(&t)
		return t.Apply(env)
	}, child)
}
