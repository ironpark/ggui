package theme

import (
	"image/color"
	"strconv"

	"github.com/ironpark/ggui"
)

// Color is one of the theme's colors by name rather than by value: a widget
// given one paints the color the theme it is laid out under has, so it
// follows a theme switch, and a subtree's own theme, with nothing rebuilt.
// It is how a builder colors what it builds; Use, which reads the colors
// themselves, belongs in a Reactive or in a custom widget's Layout.
//
//	ggui.Text("Saved").Color(theme.MutedFg)
//	ggui.Box().Fill(theme.Destructive.Alpha(.1))
//
// Drawn without an Env, as a custom widget's raw Canvas calls do, it is the
// color the application's theme has.
type Color struct {
	name string
	// fade is how much opacity Alpha has taken away, so that the zero
	// value is the color as the theme has it.
	fade float32
}

// The theme's colors as Color tokens: one for every color field of Theme,
// named alike. Var reaches the rest.
var (
	Bg, Fg                         = Var("background"), Var("foreground")
	Card, CardFg                   = Var("card"), Var("card-foreground")
	Popover, PopoverFg             = Var("popover"), Var("popover-foreground")
	Primary, PrimaryFg             = Var("primary"), Var("primary-foreground")
	PrimaryHover                   = Var("primary-hover")
	Secondary, SecondaryFg         = Var("secondary"), Var("secondary-foreground")
	Muted, MutedFg                 = Var("muted"), Var("muted-foreground")
	Accent, AccentFg               = Var("accent"), Var("accent-foreground")
	Destructive, DestructiveFg     = Var("destructive"), Var("destructive-foreground")
	Border, InputBorder, Ring      = Var("border"), Var("input"), Var("ring")
	Input                          = Var("input-surface")
	Selection, Scrim               = Var("selection"), Var("scrim")
	Sidebar, SidebarFg             = Var("sidebar"), Var("sidebar-foreground")
	SidebarPrimary                 = Var("sidebar-primary")
	SidebarPrimaryFg               = Var("sidebar-primary-foreground")
	SidebarAccent, SidebarAccentFg = Var("sidebar-accent"), Var("sidebar-accent-foreground")
	SidebarBorder, SidebarRing     = Var("sidebar-border"), Var("sidebar-ring")
)

// Var is the theme color shadcn/ui names --name, such as "chart-1", or one
// an app added under that name: a variable ApplyCSS or FromCSS read with no
// field of its own, or a color Theme.Set stored under CSSColor(name).
//
//	warning := theme.Var("warning")
//
// A name the theme has no color for paints nothing.
func Var(name string) Color { return Color{name: name} }

// otherColors are the Theme fields no CSS variable sets.
var otherColors = map[string]func(*Theme) *color.Color{
	"primary-hover": func(t *Theme) *color.Color { return &t.PrimaryHover },
	"input-surface": func(t *Theme) *color.Color { return &t.Input },
	"selection":     func(t *Theme) *color.Color { return &t.Selection },
	"scrim":         func(t *Theme) *color.Color { return &t.Scrim },
}

// Alpha is the color at alpha times its opacity, from 0 to 1.
func (c Color) Alpha(alpha float64) Color {
	c.fade = 1 - (1-c.fade)*float32(min(max(alpha, 0), 1))
	return c
}

// In returns the color t has.
func (c Color) In(t Theme) color.Color {
	field := cssColors[c.name]
	if field == nil {
		field = otherColors[c.name]
	}
	var v color.Color
	if field != nil {
		if v = *field(&t); v == nil {
			// A field that follows another is nil until Resolve fills it.
			t = t.Resolve()
			v = *field(&t)
		}
	} else {
		v, _ = t.Get(CSSColor(c.name))
	}
	if c.fade != 0 {
		v = Fade(v, float64(1-c.fade))
	}
	return v
}

// Resolve implements ggui.EnvColor: the color the theme env holds has.
func (c Color) Resolve(env ggui.Env) color.Color { return c.In(From(env)) }

// RGBA implements color.Color: the color the application's theme has.
func (c Color) RGBA() (r, g, b, a uint32) { return rgba(c.Resolve(appEnv())) }

// String names the token, for the inspector and test failures.
func (c Color) String() string {
	if c.fade != 0 {
		return "--" + c.name + " at " + strconv.FormatFloat(float64(1-c.fade), 'g', 3, 32)
	}
	return "--" + c.name
}

// appEnv is the application's environment, read without subscribing.
func appEnv() ggui.Env { return ggui.Untrack(ggui.UseEnv) }

func rgba(c color.Color) (r, g, b, a uint32) {
	if c == nil {
		return 0, 0, 0, 0
	}
	return c.RGBA()
}

// faded is Fade of an EnvColor other than a Color, kept until an Env
// resolves it.
type faded struct {
	c     ggui.EnvColor
	alpha float64
}

func (f faded) Resolve(env ggui.Env) color.Color { return Fade(f.c.Resolve(env), f.alpha) }
func (f faded) RGBA() (r, g, b, a uint32)        { return rgba(f.Resolve(appEnv())) }

// mixed is Mix of colors one of which is an EnvColor, kept until an Env
// resolves it.
type mixed struct {
	a, b   color.Color
	amount float64
}

func (m mixed) Resolve(env ggui.Env) color.Color {
	return Mix(ggui.ResolveColor(m.a, env), ggui.ResolveColor(m.b, env), m.amount)
}
func (m mixed) RGBA() (r, g, b, a uint32) { return rgba(m.Resolve(appEnv())) }
