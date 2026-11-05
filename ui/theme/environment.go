package theme

import (
	"image/color"
	"sync"

	"github.com/ironpark/ggui"
)

// key holds the applied theme by pointer, so a widget keeps a pointer to
// it rather than a copy; intern makes equal themes share one, so applying
// the same theme again leaves the Env, and the caches under it, unchanged.
var key = ggui.NewEnvKey[*Theme]("ui theme")

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
func From(env ggui.Env) Theme { return *Of(env) }

// Of returns the theme inherited by env, or the default light theme, as a
// pointer shared by everything under the same theme: a widget keeps it
// from Layout to Paint without copying the theme, and a theme switch
// reaches it as a new Env and so a new pointer at the next layout. Treat
// it as read-only; copy it (or use From) to change a token.
func Of(env ggui.Env) *Theme {
	if t, ok := env.Get(key); ok {
		return t
	}
	return defaultTheme()
}

// defaultTheme is Default made once, for an Env no theme was applied to.
var defaultTheme = sync.OnceValue(func() *Theme {
	t := Default()
	return &t
})

// interned holds the themes Apply supplied last, most recent first, so
// that equal themes share a pointer. A theme is compared by ==; a few
// entries cover an app's themes and the overrides inside them, and one
// beyond them costs an Env revision, as a changed theme does.
var interned struct {
	sync.Mutex
	themes [8]*Theme
}

// intern returns the shared pointer to a theme equal to t.
func intern(t Theme) *Theme {
	interned.Lock()
	defer interned.Unlock()
	list := &interned.themes
	for i, p := range list {
		if p != nil && sameTheme(*p, t) {
			copy(list[1:i+1], list[:i])
			list[0] = p
			return p
		}
	}
	p := &t
	copy(list[1:], list[:len(list)-1])
	list[0] = p
	return p
}

// sameTheme is a == b, false where the comparison panics on a token of a
// type == cannot compare.
func sameTheme(a, b Theme) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}

// Apply supplies this theme, resolved, and the corresponding core styles to
// env. Unrelated environment values, including accessibility preferences,
// survive.
func (t Theme) Apply(env ggui.Env) ggui.Env {
	t = t.Resolve()
	return env.With(key, intern(t)).WithText(t.Text).
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
