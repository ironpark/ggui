package ggui

import "image/color"

// EnvColor is a color the Env holds, such as a theme's primary color. A
// widget given one resolves it against the Env it is laid out in, so the
// widget follows a theme switch, or a subtree's own theme, without being
// rebuilt. Its RGBA is the value in the application's environment, for code
// that draws it without an Env.
//
// An EnvColor is compared with ==, as every color property is, so it must
// be comparable; the theme package's tokens are.
type EnvColor interface {
	color.Color
	Resolve(env Env) color.Color
}

// ResolveColor returns c as env has it: an EnvColor's value there, and any
// other color as it is. A custom widget that takes a color resolves it this
// way in Layout and paints the result.
func ResolveColor(c color.Color, env Env) color.Color {
	if e, ok := c.(EnvColor); ok {
		return e.Resolve(env)
	}
	return c
}

// IsEnvColor reports whether c is an EnvColor, which a widget can only
// paint once layout has resolved it.
func IsEnvColor(c color.Color) bool {
	_, ok := c.(EnvColor)
	return ok
}

// shownColor is a color property as it was given and as it is painted: the
// given color itself, or an EnvColor's value in the Env of the last layout.
type shownColor struct{ given, shown color.Color }

// set gives the property c. A widget lays out again for an EnvColor, given
// or replaced, since only layout knows its Env.
func (s *shownColor) set(c color.Color, changed func()) {
	if c == s.given {
		return
	}
	if IsEnvColor(c) || IsEnvColor(s.given) {
		changed()
	}
	s.given, s.shown = c, c
}

func (s *shownColor) layout(env Env) { s.shown = ResolveColor(s.given, env) }
