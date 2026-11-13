package theme

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/fonts/gofont"
	"github.com/ironpark/ggui/fonts/inter"
)

// Radius selects a Config's corner treatment. Default preserves the preset's
// geometry. Other choices use logical pixels and scale its chat corners too.
type Radius string

const (
	RadiusDefault Radius = ""
	RadiusNone    Radius = "none"
	RadiusSmall   Radius = "small"
	RadiusMedium  Radius = "medium"
	RadiusLarge   Radius = "large"
)

// RadiusOptions returns supported choices, in increasing order after Default.
// None is square; Small, Medium and Large use 4, 8 and 16 logical pixels.
func RadiusOptions() []Radius {
	return []Radius{RadiusDefault, RadiusNone, RadiusSmall, RadiusMedium, RadiusLarge}
}

// FontFamily selects an offline font for Config. Default leaves the application
// font in control. Sans and Mono are the bundled Go fonts, with real bold faces;
// Inter is the official bundled Inter family, including medium, semibold and
// bold faces. No choice downloads fonts. A default heading inherits the body font.
type FontFamily string

const (
	FontDefault FontFamily = ""
	FontSans    FontFamily = "sans"
	FontMono    FontFamily = "mono"
	FontInter   FontFamily = "inter"
)

// Fonts returns supported font choices. Labels should distinguish the app's
// default font from the bundled Go Sans, Go Mono and Inter families.
func Fonts() []FontFamily { return []FontFamily{FontDefault, FontSans, FontMono, FontInter} }

// Config is a reusable, comparable theme recipe for a theme builder or an app.
// Its zero value is exactly Preset{}: Neutral/Nova, the base palette's accent
// and chart colors, default radii, and the application's font.
//
// Chart is independent of Preset.Accent. AccentBase uses the selected base's
// chart colors; a colored Chart changes only the five chart tokens. Heading
// sets Title and Heading fonts; Label and Caption inherit the body font. Code
// continues to use the theme's independent Mono style.
//
// Config does not apply a theme globally. Use For/Light/Dark with Set, Bind or
// With; the returned Theme is an ordinary copy for further customization.
type Config struct {
	Preset  Preset
	Chart   AccentColor
	Radius  Radius
	Font    FontFamily
	Heading FontFamily
}

func (c Config) Light() Theme { return c.For(false) }
func (c Config) Dark() Theme  { return c.For(true) }

// For builds a resolved light or dark theme. Invalid names panic, like Preset.
// No existing defaults, preset values or environment preferences are modified.
func (c Config) For(dark bool) Theme {
	t := c.Preset.For(dark)
	if !slices.Contains(AccentColors(), c.Chart) {
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
	base := c.Preset.Base
	if base == "" {
		base = BaseNeutral
	}
	chart := string(base)
	if c.Chart != AccentBase {
		chart = string(c.Chart)
	}
	mode := 0
	if dark {
		mode = 1
	}
	t.Chart = chartColors(themePalettes()[chart][mode])
	if c.Radius != RadiusDefault {
		radius := map[Radius]float64{RadiusNone: 0, RadiusSmall: 4, RadiusMedium: 8, RadiusLarge: 16}[c.Radius]
		factor := radius / t.Radius
		t.Radius, t.RadiusSm, t.RadiusLg = radius, radius*.8, radius*1.4
		t.Chat.BubbleRadius *= factor
		t.Chat.AttachmentRadius *= factor
		t.Chat.AttachmentXSRadius *= factor
		t.Chat.QuestionRadius *= factor
		t.Chat.QuestionInputRadius *= factor
	}
	if c.Font != FontDefault {
		t.Text.Font = configFont(c.Font)
	}
	if c.Heading != FontDefault {
		font := configFont(c.Heading)
		t.Title.Font, t.Heading.Font = font, font
	}
	return t.Resolve()
}

func configFont(font FontFamily) *ggui.Font {
	switch font {
	case FontSans:
		return gofont.Font()
	case FontMono:
		return gofont.Mono()
	case FontInter:
		return inter.Font()
	default:
		return nil
	}
}

// GoCode returns a formatted Go expression that reproduces this configuration
// and appearance, for example theme.Config{...}.For(true). The expression needs
// the import "github.com/ironpark/ggui/ui/theme" under its usual name. It is
// deterministic, uses only supported string choices, and does not apply a theme.
// Invalid configurations panic just as For does.
func (c Config) GoCode(dark bool) string {
	_ = c.For(dark)
	source := fmt.Sprintf(`theme.Config{
 Preset: theme.Preset{Base: %q, Accent: %q, Style: %q},
 Chart: %q,
 Radius: %q,
 Font: %q,
 Heading: %q,
}.For(%t)`, c.Preset.Base, c.Preset.Accent, c.Preset.Style, c.Chart, c.Radius, c.Font, c.Heading, dark)
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
