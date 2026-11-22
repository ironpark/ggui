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

// FontFamily selects a font for Config. FontDefault leaves the application font
// in control. The named choices correspond to opt-in bundled font packs
// registered with RegisterFont: FontSans and FontMono for fonts/gofont,
// FontInter for fonts/inter, and FontGeist and FontGeistMono for fonts/geist.
// Any other name registered with RegisterFont is also accepted. ui/theme does
// not import embedded font packages automatically; apps register whichever
// families they want. No choice downloads fonts. A default heading inherits the
// body font.
type FontFamily string

const (
	FontDefault   FontFamily = ""
	FontSans      FontFamily = "sans"
	FontMono      FontFamily = "mono"
	FontInter     FontFamily = "inter"
	FontGeist     FontFamily = "geist"
	FontGeistMono FontFamily = "geist-mono"
)

// builtInFonts defines the canonical order of named FontFamily choices and the
// RegisterFont aliases that satisfy each choice.
var builtInFonts = []struct {
	choice  FontFamily
	aliases []string
}{
	{FontSans, []string{"sans", "Go", "Go Sans"}},
	{FontMono, []string{"mono", "Go Mono"}},
	{FontInter, []string{"Inter"}},
	{FontGeist, []string{"Geist", "Geist Sans"}},
	{FontGeistMono, []string{"Geist Mono"}},
}

// Fonts returns FontDefault followed by the bundled font choices registered
// with RegisterFont, in canonical order, without loading any font faces.
// Labels should distinguish the app's default font from the bundled Go Sans,
// Go Mono, Inter, Geist and Geist Mono families.
func Fonts() []FontFamily {
	out := make([]FontFamily, 1, 1+len(builtInFonts))
	out[0] = FontDefault
	for _, entry := range builtInFonts {
		if hasConfigFont(entry.choice) {
			out = append(out, entry.choice)
		}
	}
	return out
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
// accent selects that accent's, changing only the five chart tokens. Font and
// Heading name families RegisterFont registered; Heading sets Title and
// Heading fonts, while Label and Caption inherit the body font. Code continues
// to use the theme's independent Mono style. Icons names a library
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
func (c Config) Light() *Theme { return c.For(false) }

// Dark is the recipe's dark theme.
func (c Config) Dark() *Theme { return c.For(true) }

// For builds a resolved light or dark theme. Invalid names panic, like Preset,
// and so does a font or icon library that is not registered. No existing
// defaults, preset values or environment preferences are modified.
func (c Config) For(dark bool) *Theme {
	t := c.Preset.For(dark)
	c.checkChoices()
	c.applyCharts(t, dark)
	c.applyMenu(t, dark)
	c.applyFonts(t)
	c.applyIcons(t)
	t.resolveInPlace()
	return t
}

// checkChoices panics on a choice the recipe does not have.
func (c Config) checkChoices() {
	if !slices.Contains(ChartColors(), c.Chart) {
		panic("ggui: unknown chart accent " + string(c.Chart))
	}
	if !slices.Contains(RadiusOptions(), c.Radius) {
		panic("ggui: unknown theme radius " + string(c.Radius))
	}
	if !slices.Contains(MenuAccents(), c.MenuAccent) {
		panic("ggui: unknown menu accent " + string(c.MenuAccent))
	}
	if !slices.Contains(MenuColors(), c.MenuColor) {
		panic("ggui: unknown menu color " + string(c.MenuColor))
	}
}

// applyCharts sets the chart colors of t to the accent's palette, unless
// the recipe keeps the accent's own.
func (c Config) applyCharts(t *Theme, dark bool) {
	if c.Chart == AccentBase {
		return
	}
	mode := 0
	if dark {
		mode = 1
	}
	t.Chart = chartColors(themePalettes()[c.chartName()][mode])
}

// chartName is the palette c.Chart names.
func (c Config) chartName() string {
	if c.Chart != ChartBase {
		return string(c.Chart)
	}
	if chart := string(c.Preset.Base); chart != "" {
		return chart
	}
	return string(BaseNeutral)
}

// applyMenu applies the recipe's menu accent, radius and inverted color.
func (c Config) applyMenu(t *Theme, dark bool) {
	if c.MenuAccent == MenuAccentBold {
		t.Accent, t.AccentFg = t.Primary, t.PrimaryFg
	}
	if c.Radius != RadiusDefault {
		t.setRadius(radiusValues[c.Radius])
	}
	if c.MenuColor == MenuColorInverted && !dark {
		t.menu = invertedMenu(c)
	}
}

// applyFonts sets the fonts the recipe names, panicking on one that is not
// registered.
func (c Config) applyFonts(t *Theme) {
	if c.Font != FontDefault {
		t.Text.Font = c.registeredFont(c.Font)
	}
	if c.Heading != FontDefault {
		font := c.registeredFont(c.Heading)
		t.Title.Font, t.Heading.Font = font, font
	}
}

func (c Config) registeredFont(font FontFamily) *ggui.Font {
	f := configFont(font)
	if f == nil {
		panic("ggui: font " + string(font) + " is not registered; see theme.RegisterFont")
	}
	return f
}

// applyIcons stores the icon library the recipe names, panicking on one
// that is not registered.
func (c Config) applyIcons(t *Theme) {
	if c.Icons == IconsDefault {
		return
	}
	node, ok := iconLibraries.Load(c.Icons)
	if !ok {
		panic("ggui: icon library " + string(c.Icons) + " is not registered; see theme.RegisterIcons")
	}
	// A Preset's theme has no tokens of its own, so the library's one
	// node is the whole list, shared by every theme that names it.
	t.ext = node.(*tokenNode)
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
	t, _ := invertedMenus.LoadOrStore(c, menu)
	return t.(*Theme)
}

// Menu is the theme a menu's panel and rows draw under: t itself, or, for
// a light theme of a Config whose MenuColor is MenuColorInverted, the
// recipe's dark theme. A menu applies it to its panel:
//
//	env = theme.Of(env).Menu().Apply(env)
func (t *Theme) Menu() *Theme {
	if t == nil {
		return defaultTheme()
	}
	if t.menu != nil {
		return t.menu
	}
	return t
}

// MenuEnv is env under the theme of Of(env).Menu: env itself unless the
// theme's menus are drawn in another. A menu-like control lays its popup's
// content out under it.
func MenuEnv(env ggui.Env) ggui.Env {
	if t := Of(env); t.menu != nil {
		return t.menu.Apply(env)
	}
	return env
}

func fontAliases(font FontFamily) []string {
	for _, entry := range builtInFonts {
		if entry.choice == font {
			return entry.aliases
		}
	}
	return nil
}

func hasConfigFont(font FontFamily) bool {
	if font == FontDefault {
		return true
	}
	if aliases := fontAliases(font); aliases != nil {
		return slices.ContainsFunc(aliases, hasFontNamed)
	}
	return hasFontNamed(string(font))
}

func configFont(font FontFamily) *ggui.Font {
	if font == FontDefault {
		return nil
	}
	if aliases := fontAliases(font); aliases != nil {
		for _, alias := range aliases {
			if f := fontNamed(alias); f != nil {
				return f
			}
		}
		return nil
	}
	return fontNamed(string(font))
}

// GoCode returns a formatted Go expression that reproduces this configuration
// and appearance, for example theme.Config{...}.For(true). The expression needs
// the import "github.com/ironpark/ggui/ui/theme" under its usual name, and, for
// a recipe that names a font or icon library, those fonts or libraries
// registered where it runs (via RegisterFont or RegisterIcons). It is
// deterministic, uses only supported string choices, and does not apply a
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
