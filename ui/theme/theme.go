// Package theme supplies UI design tokens and maps them to core environment settings.
package theme

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"time"

	"github.com/ironpark/ggui"
)

// Theme is the app's design tokens. A widget reads it from its Env with
// From; a builder colors what it builds with Color tokens such as Primary,
// which widgets resolve the same way. Text starts from Theme.Text through
// the root Env.
//
// Some tokens follow others: PrimaryHover and Selection follow Primary,
// CardFg and the other foregrounds follow Fg, the other radii, the chat
// corners among them, follow Radius, and so on, as each field's comment says. A token left unset
// follows its source, and so does one still holding the value it was given
// that way, so changing Primary on a copy of Default moves its hover too.
// Setting the token itself pins it. Resolve fills the followers in; the
// Default, Dark and Preset themes, and From, return resolved themes.
//
// Unset colors with nothing to follow, and unset sizes a control cannot do
// without, such as ControlSize, are Default's, so a Theme literal draws.
type Theme struct {
	// Additional shadcn semantic pairs.
	CardFg, PopoverFg                color.Color // follow Fg
	Accent, AccentFg                 color.Color // follow Muted and Fg
	InputBorder                      color.Color // CSS --input, follows Border; Input below is the painted surface
	Sidebar, SidebarFg               color.Color // follow Card and Fg
	SidebarPrimary, SidebarPrimaryFg color.Color // follow Primary and PrimaryFg
	SidebarAccent, SidebarAccentFg   color.Color // follow Accent and AccentFg
	SidebarBorder, SidebarRing       color.Color // follow Border and Ring
	Chart                            [5]color.Color
	Chat                             ChatTokens

	Text    ggui.TextStyle // the base every Text inherits; its Color follows Fg
	Title   ggui.TextStyle // merged onto Text for page headings
	Heading ggui.TextStyle // merged onto Text for dialog, sheet and card titles
	Label   ggui.TextStyle // merged onto Text for the labels of buttons, tabs and badges
	Caption ggui.TextStyle // merged onto Text for small secondary text; its Color follows MutedFg
	Mono    ggui.TextStyle // merged onto Text for code and keyboard keys

	// Colors follow shadcn/ui's semantic tokens: a surface, and the
	// foreground drawn on it. The comment on each names the CSS variable it
	// answers to, so a palette written for shadcn ports across directly;
	// FromCSS reads one as it is.
	Bg, Fg                     color.Color // --background / --foreground
	Card                       color.Color // --card: cards and other raised inline surfaces; follows Bg
	Popover                    color.Color // --popover: menus, dialogs and anything floating; follows Card
	Primary, PrimaryFg         color.Color // --primary / --primary-foreground
	PrimaryHover               color.Color // Primary under the pointer; follows Primary
	Secondary, SecondaryFg     color.Color // --secondary / --secondary-foreground; SecondaryFg follows Fg
	Muted, MutedFg             color.Color // --muted / --muted-foreground
	Destructive, DestructiveFg color.Color // --destructive; DestructiveFg is retained for custom solid variants
	Border                     color.Color // --border: outlines of inputs and dividers
	// Input is the surface a text field or select paints over whatever it
	// sits on, as shadcn's bg-transparent dark:bg-input/30 does: unless set,
	// it is InputBorder at the opacity of the theme's Style, which under Nova
	// is clear in a light theme and 30% in a dark one. Paint it over the
	// surface, not instead of it.
	Input     color.Color
	Ring      color.Color // --ring: the focus halo, separate from Primary
	Selection color.Color // selected text; follows Primary
	Scrim     color.Color // dims the window behind a modal

	// Elevation, in the order a surface rises off the page.
	CardShadow    ggui.ShadowStyle // cards and the raised tab
	PanelShadow   ggui.ShadowStyle // menus, select lists, date pickers
	OverlayShadow ggui.ShadowStyle // sheets, toasts and submenus: shadcn's shadow-lg

	// The radii are shadcn's scale of --radius, each named a step smaller
	// than Tailwind names it: Radius is rounded-lg, the corner of a button
	// or a field, and the rest follow it in the proportions of the theme's
	// Style, which under Nova are the ones given here. Chat's corners
	// follow it too.
	Radius   float64 // --radius, rounded-lg: buttons, fields and boxes that ask for one
	RadiusXs float64 // rounded-sm, 0.6 of Radius: chips, keys and the smallest insets
	RadiusSm float64 // rounded-md, 0.8: rows and pills inside a rounded container
	RadiusLg float64 // rounded-xl, 1.4: cards and dialogs
	RadiusXl float64 // rounded-2xl, 1.8: toasts

	Space       float64 // the unit gaps and padding are multiples of
	BorderWidth float64 // the line Box.Border and the controls draw
	ControlSize float64 // a checkbox or radio glyph; switches and sliders scale with it
	ControlGap  float64 // between a glyph and its label
	IconSize    float64 // chevrons, checks and other icons inside a control
	MenuWidth   float64 // a dropdown menu panel, which does not stretch

	ButtonPad ggui.EdgeInsets // inside a button
	FieldPad  ggui.EdgeInsets // inside a text field, select or other input
	ItemPad   ggui.EdgeInsets // around one row of a list or menu
	CardPad   ggui.EdgeInsets // inside a card
	PanelPad  ggui.EdgeInsets // inside a popup panel, around its items
	TabPad    ggui.EdgeInsets // inside one tab label

	MotionFast    time.Duration // hover and control transitions
	MotionSlow    time.Duration // disclosures and sheets
	MotionOverlay time.Duration // dialog and popup entrances/exits
	MotionDrawer  time.Duration // a drawer sliding in or out

	// How far a state tints the color it starts from, as Mix takes it:
	// toward Fg under the pointer and while pressed, and toward transparent
	// while disabled.
	HoverMix, PressMix, DisabledMix float64
	// RingAlpha is the opacity of the soft halo around a focused field and
	// of a warning ring.
	RingAlpha float64

	ext   *tokenNode // extension tokens, a persistent list; see Set
	auto  followed   // what Resolve filled in last; see Resolve
	style Style      // whose proportions the radii follow Radius in; "" is Nova
	menu  *Theme     // what Menu returns, nil for the theme itself
}

// tokenNode is one extension token; the list is shared between the themes
// derived from one another and never mutated.
type tokenNode struct {
	key  any
	val  any
	next *tokenNode
}

// Set returns t with v stored under k, a token of the caller's own that
// travels with the theme: a control set's colors, a brand's spacing. t is
// not changed, so a theme can be derived from another.
//
//	var DangerColor = ggui.NewEnvKey[color.Color]("danger")
//	theme = theme.Set(DangerColor, color.RGBA{0xd3, 0x2f, 0x2f, 0xff})
func (t Theme) Set[T any](k ggui.EnvKey[T], v T) Theme {
	t.ext = &tokenNode{key: k, val: v, next: t.ext}
	return t
}

// Get returns the token stored under k, if Set stored one.
func (t Theme) Get[T any](k ggui.EnvKey[T]) (T, bool) {
	for n := t.ext; n != nil; n = n.next {
		if n.key == any(k) {
			return n.val.(T), true
		}
	}
	var zero T
	return zero, false
}

// Default is shadcn/ui's stock light theme: the Neutral palette in the Nova
// style, with the tokens of Preset{}.Light(). Unlike a Preset's, a color of
// it that has the value it would follow does follow, so changing Primary or
// Fg on a copy moves what derives from them.
func Default() Theme { return stock(false) }

// Dark is the dark counterpart of Default, with the tokens of Preset{}.Dark().
func Dark() Theme { return stock(true) }

// stock is the zero Preset with every color that equals what it would
// follow left to follow it.
func stock(dark bool) Theme {
	t := Preset{}.build(dark)
	resolved := t.Resolve()
	for _, f := range followers {
		if p := f.field(&t); *p != nil && sameColor(*p, f.from(&resolved)) {
			*p = nil
		}
	}
	return t.Resolve()
}

// lightBase is what a Preset lays its palette and Style over: the Neutral
// light colors nothing else derives, and Nova's geometry, before Resolve.
func lightBase() Theme {
	n := themePalettes()["neutral"][0]
	fg, quiet, nearWhite := n["foreground"], n["muted"], n["primary-foreground"]
	g := styles[StyleNova]
	return Theme{
		Chart:   chartColors(n),
		Chat:    g.chat,
		Text:    ggui.TextStyle{Size: 14, LineHeight: 20.0 / 14},
		Title:   ggui.TextStyle{Size: 24, Weight: ggui.WeightSemibold},
		Heading: ggui.TextStyle{Size: 16, LineHeight: 22.0 / 16, Weight: ggui.WeightMedium},
		Label:   ggui.TextStyle{Weight: ggui.WeightMedium},
		Caption: ggui.TextStyle{Size: 12, LineHeight: 16.0 / 12},
		Mono:    ggui.TextStyle{Font: ggui.DefaultMonoFont()},

		Bg: color.White, Fg: fg,
		Primary: n["primary"], PrimaryFg: nearWhite,
		Secondary: quiet, Muted: quiet,
		MutedFg:     n["muted-foreground"],
		Destructive: n["destructive"], DestructiveFg: nearWhite,
		Border: n["border"],
		Ring:   n["ring"],
		Scrim:  color.NRGBA{A: 26},

		CardShadow:  ggui.ShadowStyle{Offset: ggui.Pt(0, 1), Blur: 2, Color: color.NRGBA{A: 15}},
		PanelShadow: ggui.ShadowStyle{Offset: ggui.Pt(0, 4), Blur: 10, Color: color.NRGBA{A: 26}},
		// shadow-lg: 0 10px 15px -3px black at 10%.
		OverlayShadow: ggui.ShadowStyle{Offset: ggui.Pt(0, 10), Blur: 15, Spread: -3, Color: color.NRGBA{A: 26}},

		Radius: 10,
		Space:  8, BorderWidth: 1, ControlSize: 16, ControlGap: 8, IconSize: 16, MenuWidth: 224,

		ButtonPad: g.button, FieldPad: g.field,
		ItemPad: g.item, CardPad: g.card, PanelPad: g.panel,
		TabPad: g.tab,

		MotionFast: 150 * time.Millisecond, MotionSlow: 200 * time.Millisecond, MotionOverlay: 100 * time.Millisecond,
		MotionDrawer: 450 * time.Millisecond,
		HoverMix:     .06, PressMix: .08, DisabledMix: .5, RingAlpha: .5,
	}
}

// chartColors are a palette's five series colors, chart-1 to chart-5.
func chartColors(tokens map[string]color.Color) (c [5]color.Color) {
	for i := range c {
		c[i] = tokens[fmt.Sprintf("chart-%d", i+1)]
	}
	return c
}

// nFollowers is the number of colors that follow another.
const nFollowers = 21

// follower is a color token that follows another unless set.
type follower struct {
	field func(*Theme) *color.Color
	from  func(*Theme) color.Color
}

// followers is in dependency order: a token comes after what it follows.
var followers = [nFollowers]follower{
	{func(t *Theme) *color.Color { return &t.Text.Color }, func(t *Theme) color.Color { return t.Fg }},
	{func(t *Theme) *color.Color { return &t.Caption.Color }, func(t *Theme) color.Color { return t.MutedFg }},
	{func(t *Theme) *color.Color { return &t.Card }, func(t *Theme) color.Color { return t.Bg }},
	{func(t *Theme) *color.Color { return &t.CardFg }, func(t *Theme) color.Color { return t.Fg }},
	{func(t *Theme) *color.Color { return &t.Popover }, func(t *Theme) color.Color { return t.Card }},
	{func(t *Theme) *color.Color { return &t.PopoverFg }, func(t *Theme) color.Color { return t.CardFg }},
	{func(t *Theme) *color.Color { return &t.PrimaryHover }, func(t *Theme) color.Color { return compositeColor(t.Primary, t.Bg, .8) }},
	{func(t *Theme) *color.Color { return &t.SecondaryFg }, func(t *Theme) color.Color { return t.Fg }},
	{func(t *Theme) *color.Color { return &t.Accent }, func(t *Theme) color.Color { return t.Muted }},
	{func(t *Theme) *color.Color { return &t.AccentFg }, func(t *Theme) color.Color { return t.Fg }},
	{func(t *Theme) *color.Color { return &t.InputBorder }, func(t *Theme) color.Color { return t.Border }},
	{func(t *Theme) *color.Color { return &t.Input }, func(t *Theme) color.Color {
		// shadcn's inputs are --input at the style's opacity, Nova's
		// bg-transparent dark:bg-input/30, over whatever surface they sit
		// on rather than over the page.
		alpha := styles[t.styleName()].input[0]
		if isDark(t.Bg) {
			alpha = styles[t.styleName()].input[1]
		}
		c := color.NRGBAModel.Convert(t.InputBorder).(color.NRGBA)
		if c.A = uint8(math.Round(float64(c.A) * alpha)); c.A == 0 {
			return color.NRGBA{}
		}
		return c
	}},
	{func(t *Theme) *color.Color { return &t.Selection }, func(t *Theme) color.Color { return compositeColor(t.Primary, t.Bg, .25) }},
	{func(t *Theme) *color.Color { return &t.Sidebar }, func(t *Theme) color.Color { return t.Card }},
	{func(t *Theme) *color.Color { return &t.SidebarFg }, func(t *Theme) color.Color { return t.Fg }},
	{func(t *Theme) *color.Color { return &t.SidebarPrimary }, func(t *Theme) color.Color { return t.Primary }},
	{func(t *Theme) *color.Color { return &t.SidebarPrimaryFg }, func(t *Theme) color.Color { return t.PrimaryFg }},
	{func(t *Theme) *color.Color { return &t.SidebarAccent }, func(t *Theme) color.Color { return t.Accent }},
	{func(t *Theme) *color.Color { return &t.SidebarAccentFg }, func(t *Theme) color.Color { return t.AccentFg }},
	{func(t *Theme) *color.Color { return &t.SidebarBorder }, func(t *Theme) color.Color { return t.Border }},
	{func(t *Theme) *color.Color { return &t.SidebarRing }, func(t *Theme) color.Color { return t.Ring }},
}

// The radii that follow Radius, in the order of radiusSteps.follow.
const (
	stepXs = iota
	stepSm
	stepLg
	stepXl
	stepBubble
	stepAttachment
	stepAttachmentXS
	stepQuestion
	stepQuestionInput
	nRadii
)

// radiusSteps are a Style's corners as multiples of --radius: Radius
// itself, the radii that follow it, and the most RadiusLg grows to, zero for
// no limit.
type radiusSteps struct {
	control float64
	follow  [nRadii]float64
	lgMax   float64
}

// radii are t's radii that follow Radius, in the order of radiusSteps.follow.
func (t *Theme) radii() [nRadii]*float64 {
	return [nRadii]*float64{&t.RadiusXs, &t.RadiusSm, &t.RadiusLg, &t.RadiusXl,
		&t.Chat.BubbleRadius, &t.Chat.AttachmentRadius, &t.Chat.AttachmentXSRadius,
		&t.Chat.QuestionRadius, &t.Chat.QuestionInputRadius}
}

// setRadius sets Radius to what a --radius of base is in t's Style, and
// leaves the other radii to follow it.
func (t *Theme) setRadius(base float64) {
	t.Radius = snap(base * styles[t.styleName()].radii.control)
	for _, p := range t.radii() {
		*p = 0
	}
	t.auto.radii = [nRadii]float64{}
}

// styleName is the Style t's radii follow Radius in.
func (t *Theme) styleName() Style {
	if t.style == "" {
		return StyleNova
	}
	return t.style
}

// snap rounds away the error of multiplying and dividing by a step, so a
// radius of 14 is 14.
func snap(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// followed is what Resolve filled in: nil, or zero, where the token was set.
// It is a value, so equal themes stay equal and an Env holding one is
// reused across layouts.
type followed struct {
	colors [nFollowers]color.Color
	radii  [nRadii]float64
}

// Resolve fills in the tokens that follow others, and gives every unset
// color with nothing to follow, and every unset size a control cannot do
// without, Default's value. A token still holding the value an earlier
// Resolve gave it follows its source again, so
//
//	t := theme.Default()
//	t.Primary = brand
//	theme.Set(t) // PrimaryHover and Selection follow brand
//
// A radius left at zero follows Radius, so a square corner in a rounded
// theme is a Radius of zero rather than one of the others.
//
// Apply resolves the theme it applies, so only code that reads a derived
// token from a theme it changed itself needs to call Resolve.
func (t Theme) Resolve() Theme {
	for i, f := range followers {
		if p := f.field(&t); *p != nil && sameColor(*p, t.auto.colors[i]) {
			*p = nil
		}
	}
	for i, p := range t.radii() {
		if t.auto.radii[i] != 0 && *p == t.auto.radii[i] {
			*p = 0
		}
	}
	t.fillUnset()
	var a followed
	for i, f := range followers {
		if p := f.field(&t); *p == nil {
			*p = f.from(&t)
			a.colors[i] = *p
		}
	}
	steps, base := styles[t.styleName()].radii, 0.
	if steps.control != 0 {
		base = t.Radius / steps.control
	}
	for i, p := range t.radii() {
		if *p == 0 {
			*p = snap(base * steps.follow[i])
			if i == stepLg && steps.lgMax != 0 {
				*p = min(*p, steps.lgMax)
			}
			a.radii[i] = *p
		}
	}
	t.auto = a
	return t
}

// pin marks the follower at p as set, so it keeps its value even if that
// is the one it would follow.
func (t *Theme) pin(p *color.Color) {
	for i, f := range followers {
		if f.field(t) == p {
			t.auto.colors[i] = nil
		}
	}
}

// fillUnset gives the tokens with nothing to follow Default's value where
// they are unset.
func (t *Theme) fillUnset() {
	colors := []*color.Color{&t.Bg, &t.Fg, &t.Primary, &t.PrimaryFg, &t.Secondary, &t.Muted, &t.MutedFg,
		&t.Destructive, &t.DestructiveFg, &t.Border, &t.Ring, &t.Scrim}
	sizes := []*float64{&t.ControlSize, &t.IconSize, &t.MenuWidth}
	if slices.ContainsFunc(colors, func(p *color.Color) bool { return *p == nil }) ||
		slices.ContainsFunc(sizes, func(p *float64) bool { return *p == 0 }) {
		d := lightBase()
		for i, v := range []color.Color{d.Bg, d.Fg, d.Primary, d.PrimaryFg, d.Secondary, d.Muted, d.MutedFg,
			d.Destructive, d.DestructiveFg, d.Border, d.Ring, d.Scrim} {
			if *colors[i] == nil {
				*colors[i] = v
			}
		}
		for i, v := range []float64{d.ControlSize, d.IconSize, d.MenuWidth} {
			if *sizes[i] == 0 {
				*sizes[i] = v
			}
		}
	}
	if t.Mono.Font == nil {
		t.Mono.Font = ggui.DefaultMonoFont()
	}
	// A Chat with corners alone, or none, takes Nova's padding and gaps;
	// the corners follow Radius.
	if c := &t.Chat; c.BubblePadding == (ggui.EdgeInsets{}) && c.QuestionChoicePadding == (ggui.EdgeInsets{}) &&
		c.QuestionGap == 0 && c.QuestionChoiceGap == 0 && c.QuestionTextGap == 0 {
		n := styles[StyleNova].chat
		c.BubblePadding, c.QuestionChoicePadding = n.BubblePadding, n.QuestionChoicePadding
		c.QuestionGap, c.QuestionChoiceGap, c.QuestionTextGap = n.QuestionGap, n.QuestionChoiceGap, n.QuestionTextGap
	}
}

// sameColor reports whether a and b are both unset or draw the same color.
func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// isDark reports whether c is closer to black than to white.
func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r+g+b < 3*0x8000
}
