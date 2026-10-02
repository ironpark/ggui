// Package theme supplies UI design tokens and maps them to core environment settings.
package theme

import (
	"image/color"
	"slices"
	"time"

	"github.com/ironpark/ggui"
)

// Theme is the app's design tokens. Read it at build time with Use;
// Text starts from Theme.Text through the root Env.
//
// Some tokens follow others: PrimaryHover and Selection follow Primary,
// CardFg and the other foregrounds follow Fg, RadiusSm and RadiusLg follow
// Radius, and so on, as each field's comment says. A token left unset
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
	Destructive, DestructiveFg color.Color // --destructive / --destructive-foreground
	Border                     color.Color // --border: outlines of inputs and dividers
	Input                      color.Color // the surface a text field or select paints; follows Bg
	Ring                       color.Color // --ring: the focus halo, separate from Primary
	Selection                  color.Color // selected text; follows Primary
	Scrim                      color.Color // dims the window behind a modal

	// Elevation, in the order a surface rises off the page.
	CardShadow    ggui.ShadowStyle // cards and the raised tab
	PanelShadow   ggui.ShadowStyle // menus, select lists, date pickers, toasts
	OverlayShadow ggui.ShadowStyle // dialogs

	Radius   float64 // --radius: corner radius for boxes that ask for one
	RadiusSm float64 // rows and pills inside a rounded container; follows Radius
	RadiusLg float64 // cards, dialogs and toasts; follows Radius

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

	MotionFast time.Duration // knobs, tab indicators, collapsing content
	MotionSlow time.Duration // entrances and exits

	// How far a state tints the color it starts from, as Mix takes it:
	// toward Fg under the pointer and while pressed, and toward transparent
	// while disabled.
	HoverMix, PressMix, DisabledMix float64
	// RingAlpha is the opacity of the soft halo around a focused field and
	// of a warning ring.
	RingAlpha float64

	ext  *tokenNode // extension tokens, a persistent list; see Set
	auto followed   // what Resolve filled in last; see Resolve
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

// Default is a neutral light theme inspired by shadcn/ui, in the default font.
// The palette is shadcn's zinc scale, so its CSS variables map across a token
// at a time.
func Default() Theme { return lightBase().Resolve() }

// lightBase is Default before Resolve: the tokens nothing else derives.
func lightBase() Theme {
	fg := color.RGBA{0x18, 0x18, 0x1b, 0xff}    // zinc-900
	quiet := color.RGBA{0xf4, 0xf4, 0xf5, 0xff} // zinc-100
	nearWhite := color.RGBA{0xfa, 0xfa, 0xfa, 0xff}
	return Theme{
		Chat:    defaultChat(),
		Text:    ggui.TextStyle{Size: 14, LineHeight: 1.4},
		Title:   ggui.TextStyle{Size: 24, Weight: ggui.WeightSemibold},
		Heading: ggui.TextStyle{Size: 18, Weight: ggui.WeightSemibold},
		Label:   ggui.TextStyle{Weight: ggui.WeightMedium},
		Caption: ggui.TextStyle{Size: 12},
		Mono:    ggui.TextStyle{Font: ggui.DefaultMonoFont()},

		Bg: color.White, Fg: fg,
		Primary: fg, PrimaryFg: nearWhite,
		Secondary: quiet, Muted: quiet,
		MutedFg:     color.RGBA{0x71, 0x71, 0x7a, 0xff}, // zinc-500
		Destructive: color.RGBA{0xd3, 0x2f, 0x2f, 0xff}, DestructiveFg: nearWhite,
		Border: color.RGBA{0xe4, 0xe4, 0xe7, 0xff}, // zinc-200
		Ring:   color.RGBA{0xa1, 0xa1, 0xaa, 0xff}, // zinc-400
		Scrim:  color.NRGBA{A: 0x60},

		// The sidebar is shadcn's near-white surface; the rest of its
		// palette follows the page's.
		Sidebar: nearWhite,

		CardShadow:    ggui.ShadowStyle{Offset: ggui.Pt(0, 1), Blur: 2, Color: color.NRGBA{A: 15}},
		PanelShadow:   ggui.ShadowStyle{Offset: ggui.Pt(0, 4), Blur: 10, Color: color.NRGBA{A: 26}},
		OverlayShadow: ggui.ShadowStyle{Offset: ggui.Pt(0, 12), Blur: 28, Color: color.NRGBA{A: 65}},

		Radius: 8,
		Space:  8, BorderWidth: 1, ControlSize: 16, ControlGap: 8, IconSize: 16, MenuWidth: 224,

		ButtonPad: ggui.Insets(8, 16), FieldPad: ggui.Insets(8, 12),
		ItemPad: ggui.Insets(6, 8), CardPad: ggui.Insets(24), PanelPad: ggui.Insets(4),
		TabPad: ggui.Insets(5, 10),

		MotionFast: 150 * time.Millisecond, MotionSlow: 240 * time.Millisecond,
		HoverMix: .06, PressMix: .08, DisabledMix: .5, RingAlpha: .5,
	}
}

// Dark is a dark counterpart of Default.
func Dark() Theme { return darkBase().Resolve() }

// darkBase is Dark before Resolve.
func darkBase() Theme {
	t := lightBase()
	dark := color.RGBA{0x18, 0x18, 0x1b, 0xff}  // zinc-900
	quiet := color.RGBA{0x27, 0x27, 0x2a, 0xff} // zinc-800
	t.Fg = color.RGBA{0xfa, 0xfa, 0xfa, 0xff}
	t.Bg = color.RGBA{0x09, 0x09, 0x0b, 0xff} // zinc-950
	t.Card = dark
	t.Border = color.RGBA{0x32, 0x32, 0x36, 0xff}
	t.Primary = color.RGBA{0xe4, 0xe4, 0xe7, 0xff}
	t.PrimaryFg = dark
	t.Secondary, t.Muted = quiet, quiet
	t.MutedFg = color.RGBA{0xa1, 0xa1, 0xaa, 0xff} // zinc-400
	t.Ring = color.RGBA{0x71, 0x71, 0x7a, 0xff}    // zinc-500
	t.Sidebar = dark
	return t
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
	{func(t *Theme) *color.Color { return &t.PrimaryHover }, func(t *Theme) color.Color { return compositeColor(t.Primary, t.Bg, .9) }},
	{func(t *Theme) *color.Color { return &t.SecondaryFg }, func(t *Theme) color.Color { return t.Fg }},
	{func(t *Theme) *color.Color { return &t.Accent }, func(t *Theme) color.Color { return t.Muted }},
	{func(t *Theme) *color.Color { return &t.AccentFg }, func(t *Theme) color.Color { return t.Fg }},
	{func(t *Theme) *color.Color { return &t.InputBorder }, func(t *Theme) color.Color { return t.Border }},
	{func(t *Theme) *color.Color { return &t.Input }, func(t *Theme) color.Color {
		// shadcn's dark inputs are --input at 30% over the page.
		if isDark(t.Bg) {
			return compositeColor(t.InputBorder, t.Bg, .3)
		}
		return t.Bg
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

// followed is what Resolve filled in: nil, or zero, where the token was set.
// It is a value, so equal themes stay equal and an Env holding one is
// reused across layouts.
type followed struct {
	colors             [nFollowers]color.Color
	radiusSm, radiusLg float64
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
// Apply resolves the theme it applies, so only code that reads a derived
// token from a theme it changed itself needs to call Resolve.
func (t Theme) Resolve() Theme {
	for i, f := range followers {
		if p := f.field(&t); *p != nil && sameColor(*p, t.auto.colors[i]) {
			*p = nil
		}
	}
	if t.auto.radiusSm != 0 && t.RadiusSm == t.auto.radiusSm {
		t.RadiusSm = 0
	}
	if t.auto.radiusLg != 0 && t.RadiusLg == t.auto.radiusLg {
		t.RadiusLg = 0
	}
	t.fillUnset()
	var a followed
	for i, f := range followers {
		if p := f.field(&t); *p == nil {
			*p = f.from(&t)
			a.colors[i] = *p
		}
	}
	if t.RadiusSm == 0 {
		t.RadiusSm = t.Radius * .75
		a.radiusSm = t.RadiusSm
	}
	if t.RadiusLg == 0 {
		t.RadiusLg = t.Radius * 1.5
		a.radiusLg = t.RadiusLg
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
	if t.Chat == (ChatTokens{}) {
		t.Chat = defaultChat()
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
