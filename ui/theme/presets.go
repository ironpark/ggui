package theme

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"slices"
	"sync"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/themedata"
)

// BaseColor selects the neutral surfaces and foregrounds of a Preset.
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

// Style selects component geometry independently of the palette.
type Style string

const (
	StyleNova Style = "nova"
	StyleRhea Style = "rhea"
)

// Preset combines shadcn/ui's semantic palettes with ggui geometry tokens.
// Its zero value means Neutral/Nova with the base palette's own accent.
// Values are ordinary Themes: customize the returned copy freely. Existing
// Default and Dark remain unchanged. Unknown names panic.
type Preset struct {
	Base   BaseColor
	Accent AccentColor
	Style  Style
}

func (p Preset) Light() Theme { return p.For(false) }
func (p Preset) Dark() Theme  { return p.For(true) }
func BaseColors() []BaseColor {
	return []BaseColor{BaseNeutral, BaseStone, BaseZinc, BaseMauve, BaseOlive, BaseMist, BaseTaupe}
}
func AccentColors() []AccentColor {
	return []AccentColor{AccentBase, AccentAmber, AccentBlue, AccentCyan, AccentEmerald, AccentFuchsia, AccentGreen, AccentIndigo, AccentLime, AccentOrange, AccentPink, AccentPurple, AccentRed, AccentRose, AccentSky, AccentTeal, AccentViolet, AccentYellow}
}
func Styles() []Style { return []Style{StyleNova, StyleRhea} }

// ChatTokens controls the geometry of conversation and questionnaire widgets.
// Values use logical pixels. Copy a theme's Chat field to customize these tokens.
type ChatTokens struct {
	BubbleRadius                                                    float64
	BubblePadding                                                   ggui.EdgeInsets
	AttachmentRadius, AttachmentXSRadius                            float64
	QuestionRadius, QuestionGap, QuestionChoiceGap, QuestionTextGap float64
	QuestionChoicePadding                                           ggui.EdgeInsets
	QuestionInputRadius                                             float64
}

func novaChat() ChatTokens {
	return ChatTokens{12, ggui.Insets(11, 13), 12, 8, 8, 16, 10, 2, ggui.Insets(10, 12), 8}
}
func rheaChat() ChatTokens {
	return ChatTokens{24, ggui.Insets(13, 13), 16, 12, 16, 20, 12, 4, ggui.Insets(12, 16), 16}
}
func defaultChat() ChatTokens {
	return ChatTokens{24, ggui.Insets(13, 13), 16, 12, 10, 16, 10, 2, ggui.Insets(10, 12), 10}
}

// ChatTokens returns Chat, or Default's for a Theme not yet resolved
// without one; Resolve fills an omitted Chat in the same way.
func (t Theme) ChatTokens() ChatTokens {
	if t.Chat == (ChatTokens{}) {
		return defaultChat()
	}
	return t.Chat
}

// For is Dark when dark is true and Light otherwise.
func (p Preset) For(dark bool) Theme {
	base := p.Base
	if base == "" {
		base = BaseNeutral
	}
	if !slices.Contains(BaseColors(), base) {
		panic("ggui: unknown theme base " + string(base))
	}
	if !slices.Contains(AccentColors(), p.Accent) {
		panic("ggui: unknown theme accent " + string(p.Accent))
	}
	// The bases are unresolved, so every token the palette sets below is
	// set, not followed, even where it equals what it would follow.
	t, mode := lightBase(), 0
	if dark {
		t, mode = darkBase(), 1
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
	// PrimaryHover, Selection, Input and the text colors follow these.
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
	t.Sidebar, t.SidebarFg = tokens["sidebar"], tokens["sidebar-foreground"]
	t.SidebarPrimary, t.SidebarPrimaryFg = tokens["sidebar-primary"], tokens["sidebar-primary-foreground"]
	t.SidebarAccent, t.SidebarAccentFg = tokens["sidebar-accent"], tokens["sidebar-accent-foreground"]
	t.SidebarBorder, t.SidebarRing = tokens["sidebar-border"], tokens["sidebar-ring"]
	for i := range t.Chart {
		t.Chart[i] = tokens[fmt.Sprintf("chart-%d", i+1)]
	}
	t.RadiusSm, t.Radius, t.RadiusLg = 6, 8, 10
	t.Chat = novaChat()
	switch p.Style {
	case "", StyleNova:
	case StyleRhea:
		t.RadiusSm, t.Radius, t.RadiusLg = 8, 12, 16
		t.ButtonPad, t.FieldPad = ggui.Insets(6, 12), ggui.Insets(6, 12)
		t.Chat = rheaChat()
	default:
		panic("ggui: unknown theme style " + string(p.Style))
	}
	return t.Resolve()
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
					c, err := ParseColor(v)
					if err != nil {
						panic(err)
					}
					pair[i][k] = c
				}
			}
		}
		out[entry.Name] = pair
	}
	return out
})
