package theme

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"slices"
	"sync"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/fonts/geist"
	"github.com/ironpark/ggui/fonts/gofont"
	"github.com/ironpark/ggui/fonts/inter"
)

// Radius selects a Config's --radius, the length every corner is a step of.
// Default keeps shadcn/ui's stock 0.625rem. The others are the choices of
// shadcn/ui's theme builder; the chat corners follow like the rest.
type Radius string

const (
	RadiusDefault Radius = ""
	RadiusNone    Radius = "none"
	RadiusSmall   Radius = "small"
	RadiusMedium  Radius = "medium"
	RadiusLarge   Radius = "large"
)

// radiusValues are the --radius of each choice in logical pixels: shadcn's
// 0, 0.45rem, 0.625rem and 0.875rem.
var radiusValues = map[Radius]float64{RadiusNone: 0, RadiusSmall: 7.2, RadiusMedium: 10, RadiusLarge: 14}

// RadiusOptions returns supported choices, in increasing order after Default.
// None is square; Small, Medium and Large are a --radius of 7.2, 10 and 14
// logical pixels, which a style other than Nova takes its own steps of.
func RadiusOptions() []Radius {
	return []Radius{RadiusDefault, RadiusNone, RadiusSmall, RadiusMedium, RadiusLarge}
}

// FontFamily selects an offline font for Config. Default leaves the application
// font in control. Sans and Mono are the bundled Go fonts, with real bold faces;
// Inter, Geist and GeistMono are the official bundled families, each with
// medium, semibold and bold faces. Geist is the font of shadcn/ui's Nova preset.
// No choice downloads fonts. A default heading inherits the body font.
type FontFamily string

const (
	FontDefault   FontFamily = ""
	FontSans      FontFamily = "sans"
	FontMono      FontFamily = "mono"
	FontInter     FontFamily = "inter"
	FontGeist     FontFamily = "geist"
	FontGeistMono FontFamily = "geist-mono"
)

// Fonts returns supported font choices. Labels should distinguish the app's
// default font from the bundled Go Sans, Go Mono, Inter, Geist and Geist Mono
// families.
func Fonts() []FontFamily {
	return []FontFamily{FontDefault, FontSans, FontMono, FontInter, FontGeist, FontGeistMono}
}

// ChartBase is the Config.Chart that keeps the base palette's own chart
// colors whatever the accent is, as shadcn/ui's chart color "neutral" does.
const ChartBase AccentColor = "base"

// ChartColors returns the choices of Config.Chart: AccentBase, which follows
// the preset's accent, then ChartBase and every accent.
func ChartColors() []AccentColor {
	return append([]AccentColor{AccentBase, ChartBase}, AccentColors()[1:]...)
}

// MenuAccent selects how a menu marks the row under the pointer.
type MenuAccent string

const (
	// MenuAccentSubtle is the palette's own quiet accent.
	MenuAccentSubtle MenuAccent = ""
	// MenuAccentBold makes Accent and AccentFg the Primary pair, so
	// highlighted rows, and everything else drawn in Accent, take the
	// brand color.
	MenuAccentBold MenuAccent = "bold"
)

// MenuAccents returns the menu accent choices, Subtle first.
func MenuAccents() []MenuAccent { return []MenuAccent{MenuAccentSubtle, MenuAccentBold} }

// MenuColor selects the surface menus open on.
type MenuColor string

const (
	// MenuColorDefault draws menus in the theme they open under.
	MenuColorDefault MenuColor = ""
	// MenuColorInverted draws menus dark in a light theme, as shadcn/ui's
	// inverted menus are: Theme.Menu is the recipe's dark theme. A dark
	// theme's menus are dark already.
	MenuColorInverted MenuColor = "inverted"
)

// MenuColors returns the menu color choices, Default first. shadcn/ui's
// translucent menus blur what is behind them, which a ggui popup does not,
// so they are not among them.
func MenuColors() []MenuColor { return []MenuColor{MenuColorDefault, MenuColorInverted} }

// IconLibrary names an icon set for Config.Icons, registered by RegisterIcons.
type IconLibrary string

// The icon sets ggui ships, under the names they are registered by
// convention. IconsDefault leaves the choice to the application.
const (
	IconsDefault   IconLibrary = ""
	IconsLucide    IconLibrary = "lucide"
	IconsTabler    IconLibrary = "tabler"
	IconsHeroicons IconLibrary = "heroicons"
)

// iconLibraries holds the token each registered library stores in a theme:
// one node per library, so that themes of one recipe stay equal.
var iconLibraries sync.Map // IconLibrary -> *tokenNode

// RegisterIcons makes an icon set a choice of Config.Icons: a Config naming
// it stores set in its themes under key, as Theme.Set does. ui/theme cannot
// import the icon packages, which import it, so the application, or the
// package of the set, registers what it links:
//
//	theme.RegisterIcons(theme.IconsTabler, icons.SetKey, tabler.Set())
//
// Registering a name again replaces it.
func RegisterIcons[T any](name IconLibrary, key ggui.EnvKey[T], set T) {
	if name == IconsDefault {
		panic("ggui: an icon library needs a name")
	}
	iconLibraries.Store(name, &tokenNode{key: key, val: set})
}

// IconLibraries returns IconsDefault and then the registered libraries by name.
func IconLibraries() []IconLibrary {
	out := []IconLibrary{}
	iconLibraries.Range(func(name, _ any) bool {
		out = append(out, name.(IconLibrary))
		return true
	})
	slices.Sort(out)
	return append([]IconLibrary{IconsDefault}, out...)
}

// Config is a reusable, comparable theme recipe for a theme builder or an app,
// with the choices of shadcn/ui's own builder. Its zero value is exactly
// Preset{}: Neutral/Nova, the base palette's accent and chart colors, default
// radii, subtle menus, and the application's font and icons.
//
// Chart follows Preset.Accent while it is AccentBase, as shadcn/ui's chart
// color follows its theme; ChartBase keeps the base palette's charts and an
// accent selects that accent's, changing only the five chart tokens. Heading
// sets Title and Heading fonts; Label and Caption inherit the body font. Code
// continues to use the theme's independent Mono style. Icons names a library
// RegisterIcons registered.
//
// Config does not apply a theme globally. Use For/Light/Dark with Set, Bind,
// BindMode or With; the returned Theme is an ordinary copy for further
// customization.
type Config struct {
	Preset     Preset
	Chart      AccentColor
	Radius     Radius
	Font       FontFamily
	Heading    FontFamily
	MenuAccent MenuAccent
	MenuColor  MenuColor
	Icons      IconLibrary
}

// Light is the recipe's light theme.
func (c Config) Light() Theme { return c.For(false) }

// Dark is the recipe's dark theme.
func (c Config) Dark() Theme { return c.For(true) }

// For builds a resolved light or dark theme. Invalid names panic, like Preset,
// and so does an icon library that is not registered. No existing defaults,
// preset values or environment preferences are modified.
func (c Config) For(dark bool) Theme {
	t := c.Preset.For(dark)
	if !slices.Contains(ChartColors(), c.Chart) {
		panic("ggui: unknown chart accent " + string(c.Chart))
	}
	if !slices.Contains(RadiusOptions(), c.Radius) {
		panic("ggui: unknown theme radius " + string(c.Radius))
	}
	if !slices.Contains(Fonts(), c.Font) {
		panic("ggui: unknown body font " + string(c.Font))
	}
	if !slices.Contains(Fonts(), c.Heading) {
		panic("ggui: unknown heading font " + string(c.Heading))
	}
	if !slices.Contains(MenuAccents(), c.MenuAccent) {
		panic("ggui: unknown menu accent " + string(c.MenuAccent))
	}
	if !slices.Contains(MenuColors(), c.MenuColor) {
		panic("ggui: unknown menu color " + string(c.MenuColor))
	}
	if c.Chart != AccentBase {
		chart := string(c.Chart)
		if c.Chart == ChartBase {
			if chart = string(c.Preset.Base); chart == "" {
				chart = string(BaseNeutral)
			}
		}
		mode := 0
		if dark {
			mode = 1
		}
		t.Chart = chartColors(themePalettes()[chart][mode])
	}
	if c.MenuAccent == MenuAccentBold {
		t.Accent, t.AccentFg = t.Primary, t.PrimaryFg
	}
	if c.Radius != RadiusDefault {
		t.setRadius(radiusValues[c.Radius])
	}
	if c.Font != FontDefault {
		t.Text.Font = configFont(c.Font)
	}
	if c.Heading != FontDefault {
		font := configFont(c.Heading)
		t.Title.Font, t.Heading.Font = font, font
	}
	if c.Icons != IconsDefault {
		node, ok := iconLibraries.Load(c.Icons)
		if !ok {
			panic("ggui: icon library " + string(c.Icons) + " is not registered; see theme.RegisterIcons")
		}
		// A Preset's theme has no tokens of its own, so the library's one
		// node is the whole list, shared by every theme that names it.
		t.ext = node.(*tokenNode)
	}
	if c.MenuColor == MenuColorInverted && !dark {
		t.menu = invertedMenu(c)
	}
	return t.Resolve()
}

// invertedMenus keeps the dark theme each recipe's inverted menus draw
// under, one per recipe, so that its light themes stay equal.
var invertedMenus sync.Map // Config -> *Theme

// invertedMenu is the dark theme of c for its menus.
func invertedMenu(c Config) *Theme {
	if t, ok := invertedMenus.Load(c); ok {
		return t.(*Theme)
	}
	d := c
	d.MenuColor = MenuColorDefault
	menu := d.For(true)
	t, _ := invertedMenus.LoadOrStore(c, &menu)
	return t.(*Theme)
}

// Menu is the theme a menu's panel and rows draw under: t itself, or, for
// a light theme of a Config whose MenuColor is MenuColorInverted, the
// recipe's dark theme. A menu applies it to its panel:
//
//	env = theme.Of(env).Menu().Apply(env)
func (t Theme) Menu() Theme {
	if t.menu != nil {
		return *t.menu
	}
	return t
}

func configFont(font FontFamily) *ggui.Font {
	switch font {
	case FontSans:
		return gofont.Font()
	case FontMono:
		return gofont.Mono()
	case FontInter:
		return inter.Font()
	case FontGeist:
		return geist.Font()
	case FontGeistMono:
		return geist.Mono()
	default:
		return nil
	}
}

// GoCode returns a formatted Go expression that reproduces this configuration
// and appearance, for example theme.Config{...}.For(true). The expression needs
// the import "github.com/ironpark/ggui/ui/theme" under its usual name, and, for
// a recipe that names an icon library, that library registered where it runs.
// It is deterministic, uses only supported string choices, and does not apply a
// theme. Invalid configurations panic just as For does.
func (c Config) GoCode(dark bool) string {
	_ = c.For(dark)
	source := fmt.Sprintf(`theme.Config{
 Preset: theme.Preset{Base: %q, Accent: %q, Style: %q},
 Chart: %q,
 Radius: %q,
 Font: %q,
 Heading: %q,
 MenuAccent: %q,
 MenuColor: %q,
 Icons: %q,
}.For(%t)`, c.Preset.Base, c.Preset.Accent, c.Preset.Style, c.Chart, c.Radius, c.Font, c.Heading,
		c.MenuAccent, c.MenuColor, c.Icons, dark)
	fset := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fset, "", source, 0)
	if err != nil {
		panic(err)
	}
	var out bytes.Buffer
	if err := format.Node(&out, fset, expr); err != nil {
		panic(err)
	}
	return out.String()
}
