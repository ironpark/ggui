package theme_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui"
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

func TestConfigChartChoiceIsIndependentOfAccent(t *testing.T) {
	t.Parallel()
	for _, dark := range []bool{false, true} {
		for _, base := range theme.BaseColors() {
			for _, chart := range theme.AccentColors() {
				p := theme.Preset{Base: base, Accent: theme.AccentBlue, Style: theme.StyleRhea}
				got := (theme.Config{Preset: p, Chart: chart}).For(dark)
				want := p.For(dark)
				want.Chart = theme.Preset{Base: base, Accent: chart, Style: p.Style}.For(dark).Chart
				if got != want {
					t.Fatalf("chart modified other tokens: base=%s chart=%s dark=%v", base, chart, dark)
				}
			}
		}
	}
}

func TestConfigRadiusReachesEveryCornerAndSurvivesResolve(t *testing.T) {
	t.Parallel()
	for _, style := range theme.Styles() {
		p := theme.Preset{Style: style}
		base := p.Light()
		for choice, radius := range map[theme.Radius]float64{theme.RadiusNone: 0, theme.RadiusSmall: 4, theme.RadiusMedium: 8, theme.RadiusLarge: 16} {
			got := (theme.Config{Preset: p, Radius: choice}).Light()
			factor := radius / base.Radius
			if got.Radius != radius || got.RadiusSm != radius*.8 || got.RadiusLg != radius*1.4 {
				t.Fatalf("radii %s: %v/%v/%v", choice, got.RadiusSm, got.Radius, got.RadiusLg)
			}
			corners := []float64{got.Chat.BubbleRadius, got.Chat.AttachmentRadius, got.Chat.AttachmentXSRadius, got.Chat.QuestionRadius, got.Chat.QuestionInputRadius}
			bases := []float64{base.Chat.BubbleRadius, base.Chat.AttachmentRadius, base.Chat.AttachmentXSRadius, base.Chat.QuestionRadius, base.Chat.QuestionInputRadius}
			for i, v := range corners {
				if v != bases[i]*factor {
					t.Fatalf("chat corner %d stayed unchanged for %s", i, choice)
				}
			}
			if got.CardPad != base.CardPad || got.ButtonPad != base.ButtonPad || got.Chat.BubblePadding != base.Chat.BubblePadding {
				t.Fatal("radius changed padding")
			}
			if got.Resolve() != got || theme.From(got.Apply(ggui.Env{})) != got {
				t.Fatal("radius lost on resolve/apply")
			}
		}
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
	c := theme.Config{Preset: theme.Preset{Base: theme.BaseOlive, Accent: theme.AccentCyan, Style: theme.StyleRhea}, Chart: theme.AccentAmber, Radius: theme.RadiusSmall, Font: theme.FontMono, Heading: theme.FontSans}
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
 want := (theme.Config{Preset:theme.Preset{Base:theme.BaseOlive,Accent:theme.AccentCyan,Style:theme.StyleRhea},Chart:theme.AccentAmber,Radius:theme.RadiusSmall,Font:theme.FontMono,Heading:theme.FontSans}).Dark()
 if got != want { panic("exported configuration changed the theme") }
 zero := %s
 if zero != (theme.Preset{}).Light() { panic("zero export changed the preset") }
 interTheme := %s
 if interTheme != (theme.Config{Font:theme.FontInter,Heading:theme.FontInter}).Light() { panic("Inter export changed the font") }
 fmt.Print("ok")
}`, code, (theme.Config{}).GoCode(false), (theme.Config{Font: theme.FontInter, Heading: theme.FontInter}).GoCode(false))
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
