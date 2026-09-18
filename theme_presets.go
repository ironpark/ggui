package ggui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"strings"
	"sync"

	"github.com/ironpark/ggui/internal/themedata"
)

// BaseColor selects the neutral surfaces and foregrounds of a ThemePreset.
type BaseColor string

const (
	BaseNeutral BaseColor = "neutral"
	BaseStone   BaseColor = "stone"
	BaseZinc    BaseColor = "zinc"
	BaseMauve   BaseColor = "mauve"
	BaseOlive   BaseColor = "olive"
	BaseMist    BaseColor = "mist"
	BaseTaupe   BaseColor = "taupe"
)

// AccentColor selects primary/secondary and chart colors without replacing the
// base surfaces. AccentBase preserves the base palette's own colors.
type AccentColor string

const (
	AccentBase    AccentColor = ""
	AccentAmber   AccentColor = "amber"
	AccentBlue    AccentColor = "blue"
	AccentCyan    AccentColor = "cyan"
	AccentEmerald AccentColor = "emerald"
	AccentFuchsia AccentColor = "fuchsia"
	AccentGreen   AccentColor = "green"
	AccentIndigo  AccentColor = "indigo"
	AccentLime    AccentColor = "lime"
	AccentOrange  AccentColor = "orange"
	AccentPink    AccentColor = "pink"
	AccentPurple  AccentColor = "purple"
	AccentRed     AccentColor = "red"
	AccentRose    AccentColor = "rose"
	AccentSky     AccentColor = "sky"
	AccentTeal    AccentColor = "teal"
	AccentViolet  AccentColor = "violet"
	AccentYellow  AccentColor = "yellow"
)

// ThemeStyle selects component geometry independently of the palette.
type ThemeStyle string

const (
	StyleNova ThemeStyle = "nova"
	StyleRhea ThemeStyle = "rhea"
)

// ThemePreset combines shadcn/ui's semantic palettes with ggui geometry tokens.
// Its zero value means Neutral/Nova with the base palette's own accent.
// Values are ordinary Themes: customize the returned copy freely. Existing
// DefaultTheme and DarkTheme remain unchanged. Unknown names panic.
type ThemePreset struct {
	Base   BaseColor
	Accent AccentColor
	Style  ThemeStyle
}

func (p ThemePreset) Light() Theme { return p.theme(false) }
func (p ThemePreset) Dark() Theme  { return p.theme(true) }
func BaseColors() []BaseColor {
	return []BaseColor{BaseNeutral, BaseStone, BaseZinc, BaseMauve, BaseOlive, BaseMist, BaseTaupe}
}
func AccentColors() []AccentColor {
	return []AccentColor{AccentBase, AccentAmber, AccentBlue, AccentCyan, AccentEmerald, AccentFuchsia, AccentGreen, AccentIndigo, AccentLime, AccentOrange, AccentPink, AccentPurple, AccentRed, AccentRose, AccentSky, AccentTeal, AccentViolet, AccentYellow}
}
func ThemeStyles() []ThemeStyle { return []ThemeStyle{StyleNova, StyleRhea} }

// ChatTokens controls the geometry of conversation and questionnaire widgets.
// Values use logical pixels. Copy a theme's Chat field to customize these tokens.
type ChatTokens struct {
	BubbleRadius                                                    float64
	BubblePadding                                                   EdgeInsets
	AttachmentRadius, AttachmentXSRadius                            float64
	QuestionRadius, QuestionGap, QuestionChoiceGap, QuestionTextGap float64
	QuestionChoicePadding                                           EdgeInsets
	QuestionInputRadius                                             float64
}

func novaChat() ChatTokens {
	return ChatTokens{12, Insets(11, 13), 12, 8, 8, 16, 10, 2, Insets(10, 12), 8}
}
func rheaChat() ChatTokens {
	return ChatTokens{24, Insets(13, 13), 16, 12, 16, 20, 12, 4, Insets(12, 16), 16}
}
func defaultChat() ChatTokens {
	t := novaChat()
	r := rheaChat()
	t.BubbleRadius, t.BubblePadding, t.AttachmentRadius, t.AttachmentXSRadius = r.BubbleRadius, r.BubblePadding, r.AttachmentRadius, r.AttachmentXSRadius
	t.QuestionRadius, t.QuestionInputRadius = 10, 10
	return t
}

// ChatTokens resolves an omitted Chat field for manually constructed Themes.
func (t Theme) ChatTokens() ChatTokens {
	if t.Chat == (ChatTokens{}) {
		return defaultChat()
	}
	return t.Chat
}

func (p ThemePreset) theme(dark bool) Theme {
	base := p.Base
	if base == "" {
		base = BaseNeutral
	}
	valid := false
	for _, b := range BaseColors() {
		valid = valid || base == b
	}
	if !valid {
		panic("ggui: unknown theme base " + string(base))
	}
	valid = false
	for _, a := range AccentColors() {
		valid = valid || p.Accent == a
	}
	if !valid {
		panic("ggui: unknown theme accent " + string(p.Accent))
	}
	t := DefaultTheme()
	mode := 0
	if dark {
		t = DarkTheme()
		mode = 1
	}
	tokens := make(map[string]color.Color)
	for k, v := range themePalettes()[string(base)][mode] {
		tokens[k] = v
	}
	if p.Accent != "" {
		for k, v := range themePalettes()[string(p.Accent)][mode] {
			tokens[k] = v
		}
	}
	t.Bg, t.Fg = tokens["background"], tokens["foreground"]
	t.Card, t.CardFg = tokens["card"], tokens["card-foreground"]
	t.Popover, t.PopoverFg = tokens["popover"], tokens["popover-foreground"]
	t.Primary, t.PrimaryFg = tokens["primary"], tokens["primary-foreground"]
	t.Secondary, t.SecondaryFg = tokens["secondary"], tokens["secondary-foreground"]
	t.Muted, t.MutedFg = tokens["muted"], tokens["muted-foreground"]
	t.Accent, t.AccentFg = tokens["accent"], tokens["accent-foreground"]
	t.Border, t.InputBorder, t.Ring = tokens["border"], tokens["input"], tokens["ring"]
	t.Destructive = tokens["destructive"]
	t.DestructiveFg = color.White
	// ggui's Input is a painted surface; CSS --input is a border token. Keep both.
	t.Input = t.Bg
	if dark {
		t.Input = compositeColor(tokens["input"], t.Bg, .3)
	}
	t.PrimaryHover = compositeColor(t.Primary, t.Bg, .9)
	t.Selection = compositeColor(t.Primary, t.Bg, .25)
	t.Sidebar, t.SidebarFg = tokens["sidebar"], tokens["sidebar-foreground"]
	t.SidebarPrimary, t.SidebarPrimaryFg = tokens["sidebar-primary"], tokens["sidebar-primary-foreground"]
	t.SidebarAccent, t.SidebarAccentFg = tokens["sidebar-accent"], tokens["sidebar-accent-foreground"]
	t.SidebarBorder, t.SidebarRing = tokens["sidebar-border"], tokens["sidebar-ring"]
	for i := range t.Chart {
		t.Chart[i] = tokens[fmt.Sprintf("chart-%d", i+1)]
	}
	t.Text.Color, t.Caption.Color = t.Fg, t.MutedFg
	t.RadiusSm, t.Radius, t.RadiusLg = 6, 8, 10
	t.Chat = novaChat()
	switch p.Style {
	case "", StyleNova:
	case StyleRhea:
		t.RadiusSm, t.Radius, t.RadiusLg = 8, 12, 16
		t.ButtonPad, t.FieldPad = Insets(6, 12), Insets(6, 12)
		t.Chat = rheaChat()
	default:
		panic("ggui: unknown theme style " + string(p.Style))
	}
	return t
}

// compositeColor resolves a translucent token against its actual background.
func compositeColor(fg, bg color.Color, opacity float64) color.Color {
	f, b := color.NRGBAModel.Convert(fg).(color.NRGBA), color.NRGBAModel.Convert(bg).(color.NRGBA)
	a := float64(f.A) / 255 * opacity
	ch := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*a + float64(y)*(1-a))) }
	return color.NRGBA{ch(f.R, b.R), ch(f.G, b.G), ch(f.B, b.B), 255}
}

var themePalettes = sync.OnceValue(func() map[string][2]map[string]color.Color {
	var data []struct {
		Name    string                                  `json:"name"`
		CSSVars struct{ Light, Dark map[string]string } `json:"cssVars"`
	}
	if err := json.Unmarshal(themedata.JSON, &data); err != nil {
		panic(err)
	}
	out := make(map[string][2]map[string]color.Color)
	for _, entry := range data {
		var pair [2]map[string]color.Color
		for i, src := range []map[string]string{entry.CSSVars.Light, entry.CSSVars.Dark} {
			pair[i] = make(map[string]color.Color)
			for k, v := range src {
				if k != "radius" {
					pair[i][k] = parseOKLCH(v)
				}
			}
		}
		out[entry.Name] = pair
	}
	return out
})

// shadcn tokens are OKLCH in CSS. Convert to sRGB at the rendering boundary;
// out-of-gamut channels are clipped, and alpha is retained for borders/rings.
func parseOKLCH(s string) color.Color {
	s = strings.TrimSuffix(strings.TrimPrefix(s, "oklch("), ")")
	var l, c, h, a float64
	a = 1
	parts := strings.Split(s, "/")
	if _, err := fmt.Sscanf(parts[0], "%f %f %f", &l, &c, &h); err != nil {
		panic(err)
	}
	if len(parts) == 2 {
		alpha := strings.TrimSpace(parts[1])
		percent := strings.HasSuffix(alpha, "%")
		if _, err := fmt.Sscanf(strings.TrimSuffix(alpha, "%"), "%f", &a); err != nil {
			panic(err)
		}
		if percent {
			a /= 100
		}
	}
	angle := h * math.Pi / 180
	x, y := c*math.Cos(angle), c*math.Sin(angle)
	ll, mm, ss := l+.3963377774*x+.2158037573*y, l-.1055613458*x-.0638541728*y, l-.0894841775*x-1.291485548*y
	ll, mm, ss = ll*ll*ll, mm*mm*mm, ss*ss*ss
	ch := func(v float64) uint8 {
		if v <= .0031308 {
			v *= 12.92
		} else {
			v = 1.055*math.Pow(v, 1/2.4) - .055
		}
		return uint8(math.Round(max(0, min(1, v)) * 255))
	}
	return color.NRGBA{ch(4.0767416621*ll - 3.3077115913*mm + .2309699292*ss), ch(-1.2684380046*ll + 2.6097574011*mm - .3413193965*ss), ch(-.0041960863*ll - .7034186147*mm + 1.707614701*ss), uint8(math.Round(max(0, min(1, a)) * 255))}
}
