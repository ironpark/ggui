package theme_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/fonts/geist"
	"github.com/ironpark/ggui/fonts/gofont"
	"github.com/ironpark/ggui/fonts/inter"
	"github.com/ironpark/ggui/ui/theme"
)

func TestConfigZeroPreservesPresetAndDefaults(t *testing.T) {
	t.Parallel()
	light, dark := theme.Default(), theme.Dark()
	for _, isDark := range []bool{false, true} {
		got := (theme.Config{}).For(isDark)
		if want := (theme.Preset{}).For(isDark); got != want {
			t.Fatal("zero config differs from zero preset")
		}
		if got.Resolve() != got {
			t.Fatal("config is not stably resolved")
		}
	}
	c := theme.Config{Preset: theme.Preset{Base: theme.BaseStone, Accent: theme.AccentBlue, Style: theme.StyleRhea}, Chart: theme.AccentRose, Radius: theme.RadiusLarge, Font: theme.FontSans, Heading: theme.FontMono}
	result := c.Dark()
	result.Chat.BubbleRadius = 999
	result.Chart[0] = result.Fg
	if c.Dark().Chat.BubbleRadius == 999 || c.Dark().Chart[0] == result.Fg {
		t.Fatal("config results share mutable state")
	}
	if theme.Default() != light || theme.Dark() != dark {
		t.Fatal("building config changed existing defaults")
	}
	if c.Light() != c.For(false) || c.Dark() != c.For(true) {
		t.Fatal("appearance helpers disagree")
	}
}

// The chart colors follow the accent, as shadcn's chartColor defaults to its
// theme, unless Chart names the base's own or another accent's; either way
// only the five chart tokens change.
func TestConfigChartFollowsAccentUnlessChosen(t *testing.T) {
	t.Parallel()
	for _, dark := range []bool{false, true} {
		for _, base := range theme.BaseColors() {
			p := theme.Preset{Base: base, Accent: theme.AccentBlue, Style: theme.StyleRhea}
			for _, chart := range theme.ChartColors() {
				got := (theme.Config{Preset: p, Chart: chart}).For(dark)
				want := p.For(dark)
				switch chart {
				case theme.AccentBase:
				case theme.ChartBase:
					want.Chart = theme.Preset{Base: base, Style: p.Style}.For(dark).Chart
				default:
					want.Chart = theme.Preset{Base: base, Accent: chart, Style: p.Style}.For(dark).Chart
				}
				if got != want {
					t.Fatalf("chart modified other tokens: base=%s chart=%s dark=%v", base, chart, dark)
				}
			}
			if (theme.Config{Preset: p}).For(dark).Chart == (theme.Config{Preset: p, Chart: theme.ChartBase}).For(dark).Chart {
				t.Fatalf("an accent's charts are the base's: base=%s dark=%v", base, dark)
			}
		}
	}
}

// A radius choice is shadcn's --radius for it, and every corner, the chat
// corners among them, is the style's step of it.
func TestConfigRadiusReachesEveryCornerAndSurvivesResolve(t *testing.T) {
	t.Parallel()
	for _, style := range theme.Styles() {
		p := theme.Preset{Style: style}
		base := p.Light()
		for choice, radius := range map[theme.Radius]float64{theme.RadiusNone: 0, theme.RadiusSmall: 7.2, theme.RadiusMedium: 10, theme.RadiusLarge: 14} {
			got := (theme.Config{Preset: p, Radius: choice}).Light()
			css, err := base.ApplyCSS(fmt.Sprintf("--radius: %vpx", radius))
			if err != nil {
				t.Fatal(err)
			}
			if got != css {
				t.Fatalf("%s/%s differs from --radius %v: radii %v/%v/%v, chat %+v", style, choice, radius, got.RadiusSm, got.Radius, got.RadiusLg, got.Chat)
			}
			if choice == theme.RadiusMedium && got != base {
				t.Fatalf("%s: medium is not the stock radius", style)
			}
			if choice == theme.RadiusNone && (got.Radius != 0 || got.RadiusLg != 0 || got.Chat.BubbleRadius != 0) {
				t.Fatalf("%s: none left a corner round", style)
			}
			if got.CardPad != base.CardPad || got.ButtonPad != base.ButtonPad || got.Chat.BubblePadding != base.Chat.BubblePadding {
				t.Fatal("radius changed padding")
			}
			if got.Resolve() != got || theme.From(got.Apply(ggui.Env{})) != got {
				t.Fatal("radius lost on resolve/apply")
			}
		}
	}
	nova := (theme.Config{Radius: theme.RadiusLarge}).Light()
	if nova.Radius != 14 || nova.RadiusXs != 8.4 || nova.RadiusSm != 11.2 || nova.RadiusLg != 19.6 || nova.RadiusXl != 25.2 || nova.Chat.BubbleRadius != 19.6 {
		t.Fatalf("Nova large radii %v/%v/%v/%v/%v, bubble %v", nova.RadiusXs, nova.RadiusSm, nova.Radius, nova.RadiusLg, nova.RadiusXl, nova.Chat.BubbleRadius)
	}
}

// Bold menus take the brand pair for Accent, as shadcn's menuAccent does,
// and leave the sidebar's own accent alone.
func TestConfigMenuAccentAndColor(t *testing.T) {
	t.Parallel()
	p := theme.Preset{Accent: theme.AccentBlue}
	for _, dark := range []bool{false, true} {
		plain := p.For(dark)
		bold := (theme.Config{Preset: p, MenuAccent: theme.MenuAccentBold}).For(dark)
		if bold.Accent != plain.Primary || bold.AccentFg != plain.PrimaryFg || bold.SidebarAccent != plain.SidebarAccent {
			t.Fatalf("bold menu accent: Accent %v, AccentFg %v, SidebarAccent %v", bold.Accent, bold.AccentFg, bold.SidebarAccent)
		}
		if bold.Resolve() != bold {
			t.Fatal("bold accent lost on resolve")
		}
		if plain.Menu() != plain {
			t.Fatal("a default menu is not drawn in its own theme")
		}
	}
	c := theme.Config{Preset: p, MenuColor: theme.MenuColorInverted}
	if got, want := c.Light().Menu(), p.Dark(); got != want {
		t.Fatalf("an inverted light menu is not the recipe's dark theme: Bg %v", got.Bg)
	}
	if c.Dark().Menu() != c.Dark() || c.Dark() != p.Dark() {
		t.Fatal("an inverted dark menu changed the dark theme")
	}
	if c.Light() != c.Light() || c.Light().Bg != p.Light().Bg {
		t.Fatal("inverted menus made a recipe's light themes differ")
	}
}

type testIcons string

// An icon library is a token the recipe stores in its themes, the same one
// each time so that they stay equal.
func TestConfigIconsStoresTheRegisteredSet(t *testing.T) {
	t.Parallel()
	key := ggui.NewEnvKey[testIcons]("test icons")
	theme.RegisterIcons("test-set", key, testIcons("outline"))
	c := theme.Config{Icons: "test-set"}
	if got, ok := c.Dark().Get(key); !ok || got != "outline" {
		t.Fatalf("the theme holds %q, %v", got, ok)
	}
	if c.Light() != c.Light() {
		t.Fatal("an icon library made a recipe's themes differ")
	}
	if _, ok := (theme.Config{}).Light().Get(key); ok {
		t.Fatal("the default recipe holds an icon set")
	}
	if !slices.Contains(theme.IconLibraries(), "test-set") || theme.IconLibraries()[0] != theme.IconsDefault {
		t.Fatalf("libraries %v", theme.IconLibraries())
	}
}

func TestConfigFontAndHeadingResolveIndependently(t *testing.T) {
	t.Parallel()
	c := theme.Config{Font: theme.FontMono, Heading: theme.FontSans}
	th := c.Light()
	if th.Text.Font != gofont.Mono() || th.Title.Font != gofont.Font() || th.Heading.Font != gofont.Font() {
		t.Fatal("font choices not applied")
	}
	if th.Mono.Font != (theme.Preset{}).Light().Mono.Font {
		t.Fatal("body font replaced the code font")
	}
	env := th.Apply(ggui.Env{}.With(ggui.TextScaleKey, 1.5).With(ggui.ReducedMotionKey, true))
	if env.ResolveText(th.Title).Font != gofont.Font() || env.ResolveText(th.Caption).Font != gofont.Mono() {
		t.Fatal("named styles inherited wrong fonts")
	}
	c.Heading = theme.FontDefault
	inherited := c.Dark()
	inheritedEnv := inherited.Apply(ggui.Env{})
	if inherited.Title.Font != nil || inheritedEnv.ResolveText(inherited.Title).Font != gofont.Mono() {
		t.Fatal("default heading does not inherit body")
	}
	if env.TextScale() != 1.5 || !env.ReducedMotion() {
		t.Fatal("application preferences lost")
	}
}

func TestConfigOptionsAreIndependentAndInvalidNamesPanic(t *testing.T) {
	t.Parallel()
	radii, fonts := theme.RadiusOptions(), theme.Fonts()
	radii[0], fonts[0] = "invalid", "invalid"
	if theme.RadiusOptions()[0] != theme.RadiusDefault || theme.Fonts()[0] != theme.FontDefault {
		t.Fatal("option slices share backing state")
	}
	for _, c := range []theme.Config{
		{Preset: theme.Preset{Base: "bad"}}, {Preset: theme.Preset{Accent: "bad"}}, {Preset: theme.Preset{Style: "bad"}},
		{Chart: "bad"}, {Radius: "bad"}, {Font: "bad"}, {Heading: "bad"},
		{MenuAccent: "bad"}, {MenuColor: "bad"}, {Icons: "never registered"},
	} {
		for _, build := range []func(){func() { c.Light() }, func() { c.Dark() }, func() { c.GoCode(false) }} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("accepted invalid config %+v", c)
					}
				}()
				build()
			}()
		}
	}
}

func TestConfigGoCodeCompilesAndReproducesTheme(t *testing.T) {
	t.Parallel()
	c := theme.Config{Preset: theme.Preset{Base: theme.BaseOlive, Accent: theme.AccentCyan, Style: theme.StyleRhea}, Chart: theme.AccentAmber, Radius: theme.RadiusSmall, Font: theme.FontMono, Heading: theme.FontSans, MenuAccent: theme.MenuAccentBold, MenuColor: theme.MenuColorInverted}
	code := c.GoCode(true)
	if code != c.GoCode(true) || !strings.Contains(code, "\n") {
		t.Fatal("export is unstable or not formatted")
	}
	for _, expr := range []string{code, (theme.Config{}).GoCode(false)} {
		if _, err := parser.ParseExpr(expr); err != nil {
			t.Fatalf("invalid exported expression: %v", err)
		}
	}
	// Compile the actual exported expression against this checkout, so renamed
	// fields, omitted font choices or a wrong appearance are observable failures.
	source := fmt.Sprintf(`package main
import("fmt"; "github.com/ironpark/ggui/ui/theme")
func main(){
 got := %s
 want := (theme.Config{Preset:theme.Preset{Base:theme.BaseOlive,Accent:theme.AccentCyan,Style:theme.StyleRhea},Chart:theme.AccentAmber,Radius:theme.RadiusSmall,Font:theme.FontMono,Heading:theme.FontSans,MenuAccent:theme.MenuAccentBold,MenuColor:theme.MenuColorInverted}).Dark()
 if got.Accent != got.Primary { panic("exported configuration lost its bold menus") }
 if got != want { panic("exported configuration changed the theme") }
 zero := %s
 if zero != (theme.Preset{}).Light() { panic("zero export changed the preset") }
 interTheme := %s
 if interTheme != (theme.Config{Font:theme.FontInter,Heading:theme.FontInter}).Light() { panic("Inter export changed the font") }
 inverted := %s
 if inverted.Menu().Bg != (theme.Preset{}).Dark().Bg || inverted.Bg != (theme.Preset{}).Light().Bg { panic("inverted export lost its dark menus") }
 fmt.Print("ok")
}`, code, (theme.Config{}).GoCode(false), (theme.Config{Font: theme.FontInter, Heading: theme.FontInter}).GoCode(false),
		(theme.Config{MenuColor: theme.MenuColorInverted}).GoCode(false))
	if _, err := parser.ParseFile(token.NewFileSet(), "export.go", source, 0); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "export.go")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("go", "run", file).CombinedOutput()
	if err != nil || string(out) != "ok" {
		t.Fatalf("generated Go did not reproduce theme: %v\n%s", err, out)
	}
}

func TestConfigInterIsExplicitAndPreservesDefaultRecipe(t *testing.T) {
	t.Parallel()
	c := theme.Config{Font: theme.FontInter, Heading: theme.FontInter}
	for _, dark := range []bool{false, true} {
		got := c.For(dark)
		if got.Text.Font != inter.Font() || got.Title.Font != inter.Font() || got.Heading.Font != inter.Font() {
			t.Fatal("Inter did not reach body and heading styles")
		}
		if (theme.Config{}).For(dark) != (theme.Preset{}).For(dark) {
			t.Fatal("Inter changed the default recipe")
		}
		if got.Mono.Font != (theme.Preset{}).For(dark).Mono.Font {
			t.Fatal("Inter replaced the code font")
		}
	}
	found := false
	for _, font := range theme.Fonts() {
		if font == theme.FontInter {
			found = true
		}
	}
	if !found {
		t.Fatal("Inter missing from font choices")
	}
}

func TestConfigGeistReachesBodyHeadingAndCSS(t *testing.T) {
	t.Parallel()
	got := (theme.Config{Font: theme.FontGeist, Heading: theme.FontGeistMono}).Dark()
	if got.Text.Font != geist.Font() || got.Title.Font != geist.Mono() || got.Heading.Font != geist.Mono() {
		t.Fatal("Geist did not reach body and heading styles")
	}
	if !slices.Contains(theme.Fonts(), theme.FontGeist) || !slices.Contains(theme.Fonts(), theme.FontGeistMono) {
		t.Fatal("Geist missing from font choices")
	}
}

// A menu under MenuEnv draws in the recipe's menu theme: the dark one for
// an inverted light theme, and the theme itself otherwise.
func TestMenuEnvTakesTheMenuTheme(t *testing.T) {
	t.Parallel()
	inverted := theme.Config{MenuColor: theme.MenuColorInverted}
	light, dark := inverted.Light(), inverted.Dark()
	env := light.Apply(ggui.Env{})
	if got := theme.Of(theme.MenuEnv(env)).Popover; got != dark.Popover {
		t.Fatalf("an inverted light theme's menu Popover is %v, want the dark one %v", got, dark.Popover)
	}
	plain := theme.Config{}.Light()
	env = plain.Apply(ggui.Env{})
	if got := theme.Of(theme.MenuEnv(env)).Popover; got != plain.Popover {
		t.Fatalf("a plain theme's menu Popover is %v, want its own %v", got, plain.Popover)
	}
}
