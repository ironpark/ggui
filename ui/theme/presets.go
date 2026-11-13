package theme

import (
	_ "embed"
	"encoding/json"
	"image/color"
	"math"
	"slices"
	"sync"

	"github.com/ironpark/ggui"
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

// Style selects component geometry independently of the palette: how round
// each kind of corner is as a step of shadcn/ui's radius scale, and the
// padding, text size and chat geometry of one of its styles.
type Style string

// The styles, each mapped from the shadcn/ui style sheet of its name. Nova
// is the default; Vega is its roomier relative with smaller corners, Maia
// is round and roomy with filled fields, Lyra is square and set in small
// text, Mira is compact in small relaxed text, Luma is round and soft, and
// Rhea round with a capped card corner. shadcn/ui's Sera, square with
// underlined fields and spaced capitals, is not among them: a Theme has no
// token for either.
const (
	StyleNova Style = "nova"
	StyleVega Style = "vega"
	StyleMaia Style = "maia"
	StyleLyra Style = "lyra"
	StyleMira Style = "mira"
	StyleLuma Style = "luma"
	StyleRhea Style = "rhea"
)

// geometry is a Style: what it sets in a Theme besides the palette.
type geometry struct {
	radii                                 radiusSteps
	text, heading                         ggui.TextStyle // zero for Nova's
	button, field, item, card, panel, tab ggui.EdgeInsets
	chat                                  ChatTokens // padding and gaps; the corners are in radii
	// input is how opaque a field's fill, --input, is in the style's light
	// and dark themes: bg-transparent dark:bg-input/30 is {0, .3}.
	input [2]float64
}

// The steps of shadcn/ui's radius scale, as multiples of --radius.
const (
	roundedNone = 0
	roundedSm   = .6
	roundedMd   = .8
	roundedLg   = 1
	roundedXl   = 1.4
	rounded2xl  = 1.8
	rounded3xl  = 2.2
	rounded4xl  = 2.6
)

// styles are the geometry of each Style, read from the utilities its
// upstream style-<name>.css applies: a height of h-8 around a 20px line is
// 6 of vertical padding, px-2.5 is 10 of horizontal, and a radius is the
// step most components that have Nova's take there. The follow steps are
// in the order RadiusXs, RadiusSm, RadiusLg, RadiusXl, then Chat's bubble,
// attachment, small attachment, question choice and question input.
var styles = map[Style]geometry{
	StyleNova: {
		radii: radiusSteps{control: roundedLg, follow: [nRadii]float64{
			roundedSm, roundedMd, roundedXl, rounded2xl, roundedXl, roundedXl, roundedLg, roundedLg, roundedLg}},
		button: ggui.Insets(6, 10), field: ggui.Insets(6, 10), item: ggui.Insets(4, 6),
		card: ggui.Insets(16), panel: ggui.Insets(4), tab: ggui.Insets(3, 8),
		chat: ChatTokens{BubblePadding: ggui.Insets(9, 13), QuestionGap: 16, QuestionChoiceGap: 10,
			QuestionTextGap: 2, QuestionChoicePadding: ggui.Insets(10, 12)},
		input: [2]float64{0, .3},
	},
	StyleVega: {
		radii: radiusSteps{control: roundedMd, follow: [nRadii]float64{
			roundedSm, roundedSm, roundedXl, rounded2xl, roundedXl, roundedXl, roundedLg, roundedMd, roundedMd}},
		button: ggui.Insets(8, 10), field: ggui.Insets(8, 10), item: ggui.Insets(6, 8),
		card: ggui.Insets(24), panel: ggui.Insets(4), tab: ggui.Insets(5, 10),
		chat: ChatTokens{BubblePadding: ggui.Insets(9, 13), QuestionGap: 20, QuestionChoiceGap: 12,
			QuestionTextGap: 4, QuestionChoicePadding: ggui.Insets(14, 16)},
		input: [2]float64{0, .3},
	},
	StyleMaia: {
		// Buttons and fields are rounded-4xl pills 36px high, which
		// rounded-2xl, the corner of its menus and popovers, draws as well;
		// rows are rounded-xl and cards rounded-2xl. Its dialogs, rounded-4xl,
		// are rounder than RadiusLg.
		radii: radiusSteps{control: rounded2xl, follow: [nRadii]float64{
			roundedLg, roundedXl, rounded2xl, rounded2xl, rounded3xl, rounded2xl, roundedXl, rounded4xl, rounded4xl}},
		button: ggui.Insets(8, 12), field: ggui.Insets(8, 12), item: ggui.Insets(8, 12),
		card: ggui.Insets(24), panel: ggui.Insets(4), tab: ggui.Insets(5, 10),
		chat: ChatTokens{BubblePadding: ggui.Insets(11, 15), QuestionGap: 20, QuestionChoiceGap: 12,
			QuestionTextGap: 4, QuestionChoicePadding: ggui.Insets(14, 16)},
		input: [2]float64{.3, .3},
	},
	StyleLyra: {
		// Every corner is rounded-none, whatever --radius is.
		radii:   radiusSteps{control: roundedNone},
		text:    ggui.TextStyle{Size: 12, LineHeight: 16.0 / 12},
		heading: ggui.TextStyle{Size: 14, LineHeight: 20.0 / 14},
		button:  ggui.Insets(8, 10), field: ggui.Insets(8, 10), item: ggui.Insets(8, 8),
		card: ggui.Insets(16), panel: ggui.Insets(0), tab: ggui.Insets(3, 8),
		chat: ChatTokens{BubblePadding: ggui.Insets(9, 11), QuestionGap: 16, QuestionChoiceGap: 10,
			QuestionTextGap: 2, QuestionChoicePadding: ggui.Insets(10, 12)},
		input: [2]float64{0, .3},
	},
	StyleMira: {
		// Nova's corners on smaller controls: buttons and fields are
		// rounded-md, menus and cards rounded-lg, dialogs rounded-xl. Text is
		// text-xs/relaxed and card titles text-sm.
		radii: radiusSteps{control: roundedMd, follow: [nRadii]float64{
			roundedSm, roundedMd, roundedLg, roundedMd, roundedLg, roundedLg, roundedMd, roundedXl, roundedMd}},
		text:    ggui.TextStyle{Size: 12, LineHeight: 19.5 / 12},
		heading: ggui.TextStyle{Size: 14, LineHeight: 20.0 / 14},
		button:  ggui.Insets(4, 8), field: ggui.Insets(4, 8), item: ggui.Insets(4, 8),
		card: ggui.Insets(16), panel: ggui.Insets(4), tab: ggui.Insets(3, 8),
		chat: ChatTokens{BubblePadding: ggui.Insets(7, 11), QuestionGap: 12, QuestionChoiceGap: 10,
			QuestionTextGap: 2, QuestionChoicePadding: ggui.Insets(10, 12)},
		input: [2]float64{.2, .3},
	},
	StyleLuma: {
		radii: radiusSteps{control: rounded3xl, follow: [nRadii]float64{
			rounded2xl, rounded2xl, rounded4xl, rounded2xl, rounded3xl, rounded3xl, rounded2xl, rounded3xl, rounded3xl}},
		button: ggui.Insets(8, 12), field: ggui.Insets(8, 12), item: ggui.Insets(8, 12),
		card: ggui.Insets(24), panel: ggui.Insets(6), tab: ggui.Insets(5, 14),
		chat: ChatTokens{BubblePadding: ggui.Insets(11, 15), QuestionGap: 20, QuestionChoiceGap: 10,
			QuestionTextGap: 4, QuestionChoicePadding: ggui.Insets(12, 16)},
		input: [2]float64{.5, .5},
	},
	StyleRhea: {
		// Cards and dialogs are rounded-[min(var(--radius-4xl),24px)].
		radii: radiusSteps{control: rounded2xl, lgMax: 24, follow: [nRadii]float64{
			roundedXl, roundedXl, rounded4xl, rounded2xl, rounded3xl, rounded2xl, roundedXl, rounded2xl, rounded2xl}},
		button: ggui.Insets(6, 12), field: ggui.Insets(6, 10), item: ggui.Insets(6, 8),
		card: ggui.Insets(20), panel: ggui.Insets(4), tab: ggui.Insets(3, 8),
		chat: ChatTokens{BubblePadding: ggui.Insets(11, 13), QuestionGap: 20, QuestionChoiceGap: 12,
			QuestionTextGap: 4, QuestionChoicePadding: ggui.Insets(12, 16)},
		input: [2]float64{.5, .5},
	},
}

// Preset combines shadcn/ui's semantic palettes with ggui geometry tokens.
// Its zero value means Neutral/Nova with the base palette's own accent,
// which has the tokens of Default and Dark. Values are ordinary Themes:
// customize the returned copy freely. Unknown names panic.
type Preset struct {
	Base   BaseColor
	Accent AccentColor
	Style  Style
}

// Light is the preset's light theme.
func (p Preset) Light() Theme { return p.For(false) }

// Dark is the preset's dark theme.
func (p Preset) Dark() Theme { return p.For(true) }

// BaseColors returns the base palettes a Preset takes.
func BaseColors() []BaseColor {
	return []BaseColor{BaseNeutral, BaseStone, BaseZinc, BaseMauve, BaseOlive, BaseMist, BaseTaupe}
}

// AccentColors returns the accents a Preset takes, the base's own first.
func AccentColors() []AccentColor {
	return []AccentColor{AccentBase, AccentAmber, AccentBlue, AccentCyan, AccentEmerald, AccentFuchsia, AccentGreen, AccentIndigo, AccentLime, AccentOrange, AccentPink, AccentPurple, AccentRed, AccentRose, AccentSky, AccentTeal, AccentViolet, AccentYellow}
}

// Styles returns the styles a Preset takes, Nova first.
func Styles() []Style {
	return []Style{StyleNova, StyleVega, StyleMaia, StyleLyra, StyleMira, StyleLuma, StyleRhea}
}

// ChatTokens controls the geometry of conversation and questionnaire widgets.
// Values use logical pixels. Copy a theme's Chat field to customize these
// tokens. The radii follow Theme.Radius in the proportions of the theme's
// Style while they are zero, as RadiusLg does: under Nova a bubble and an
// attachment are rounded-xl, 1.4 of Radius, and a small attachment, a
// question's choices and its input rounded-lg, Radius itself.
type ChatTokens struct {
	BubbleRadius                                                    float64
	BubblePadding                                                   ggui.EdgeInsets
	AttachmentRadius, AttachmentXSRadius                            float64
	QuestionRadius, QuestionGap, QuestionChoiceGap, QuestionTextGap float64
	QuestionChoicePadding                                           ggui.EdgeInsets
	QuestionInputRadius                                             float64
}

// ChatTokens returns Chat, or Default's for a Theme not yet resolved
// without one; Resolve fills an omitted Chat's padding and gaps in the same
// way, and its corners from the theme's Radius.
func (t Theme) ChatTokens() ChatTokens {
	if t.Chat == (ChatTokens{}) {
		return defaultTheme().Chat
	}
	return t.Chat
}

// For is Dark when dark is true and Light otherwise.
func (p Preset) For(dark bool) Theme { return p.build(dark).Resolve() }

// build is For before Resolve: every token the palette has is set, not
// followed, even where it equals what it would follow.
func (p Preset) build(dark bool) Theme {
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
	style := p.Style
	if style == "" {
		style = StyleNova
	}
	g, ok := styles[style]
	if !ok {
		panic("ggui: unknown theme style " + string(p.Style))
	}
	t, mode := lightBase(), 0
	if dark {
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
	t.Chart = chartColors(tokens)
	// Nova is the zero style, so a Preset naming it is the zero Preset.
	if style != StyleNova {
		t.style = style
	}
	if g.text.Size != 0 {
		t.Text.Size, t.Text.LineHeight = g.text.Size, g.text.LineHeight
	}
	if g.heading.Size != 0 {
		t.Heading.Size, t.Heading.LineHeight = g.heading.Size, g.heading.LineHeight
	}
	t.ButtonPad, t.FieldPad, t.ItemPad = g.button, g.field, g.item
	t.CardPad, t.PanelPad, t.TabPad = g.card, g.panel, g.tab
	t.Chat = g.chat
	// The corners follow Radius, shadcn's default --radius of 0.625rem in
	// the style's step for a control.
	t.setRadius(defaultRadius)
	return t
}

// defaultRadius is shadcn/ui's stock --radius, 0.625rem.
const defaultRadius = 10

// compositeColor resolves a translucent token against its actual background.
func compositeColor(fg, bg color.Color, opacity float64) color.Color {
	f, b := color.NRGBAModel.Convert(fg).(color.NRGBA), color.NRGBAModel.Convert(bg).(color.NRGBA)
	a := float64(f.A) / 255 * opacity
	ch := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*a + float64(y)*(1-a))) }
	return color.NRGBA{ch(f.R, b.R), ch(f.G, b.G), ch(f.B, b.B), 255}
}

// shadcnJSON is the shadcn/ui theme registry, vendored: see the README
// for its source and LICENSE-shadcn.txt for its license.
//
//go:embed shadcn.json
var shadcnJSON []byte

var themePalettes = sync.OnceValue(func() map[string][2]map[string]color.Color {
	var data []struct {
		Name    string                                  `json:"name"`
		CSSVars struct{ Light, Dark map[string]string } `json:"cssVars"`
	}
	if err := json.Unmarshal(shadcnJSON, &data); err != nil {
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
