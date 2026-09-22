// Package theme supplies UI design tokens and maps them to core environment settings.
package theme

import (
	"image/color"
	"time"

	"github.com/ironpark/ggui"
)

// Theme is the app's design tokens. Read it at build time with Use;
// Text starts from Theme.Text through the root Env.
//
// Derive a theme from Default or Dark rather than writing a Theme
// literal: a zero field is a zero size, not an inherited one, so a literal
// that omits ControlSize paints no checkbox.
type Theme struct {
	// Additional shadcn semantic pairs. Nil values on hand-built legacy themes
	// fall back to the existing general foreground/surface tokens in controls.
	CardFg, PopoverFg                color.Color
	Accent, AccentFg                 color.Color
	InputBorder                      color.Color // CSS --input; Input below is the painted surface
	Sidebar, SidebarFg               color.Color
	SidebarPrimary, SidebarPrimaryFg color.Color
	SidebarAccent, SidebarAccentFg   color.Color
	SidebarBorder, SidebarRing       color.Color
	Chart                            [5]color.Color
	Chat                             ChatTokens

	Text    ggui.TextStyle // the base every Text inherits
	Title   ggui.TextStyle // merged onto Text for headings
	Caption ggui.TextStyle // merged onto Text for small secondary text

	// Colors follow shadcn/ui's semantic tokens: a surface, and the
	// foreground drawn on it. The comment on each names the CSS variable it
	// answers to, so a palette written for shadcn ports across directly.
	Bg, Fg                     color.Color // --background / --foreground
	Card                       color.Color // --card: cards and other raised inline surfaces
	Popover                    color.Color // --popover: menus, dialogs and anything floating
	Primary, PrimaryFg         color.Color // --primary / --primary-foreground
	PrimaryHover               color.Color // Primary under the pointer
	Secondary, SecondaryFg     color.Color // --secondary / --secondary-foreground
	Muted, MutedFg             color.Color // --muted / --muted-foreground
	Destructive, DestructiveFg color.Color // --destructive / --destructive-foreground
	Border                     color.Color // --border: outlines of inputs and dividers
	Input                      color.Color // --input: the surface a text field or select paints
	Ring                       color.Color // --ring: the focus halo, separate from Primary
	Selection                  color.Color // selected text
	Scrim                      color.Color // dims the window behind a modal

	// Elevation, in the order a surface rises off the page.
	CardShadow    ggui.ShadowStyle // cards and the raised tab
	PanelShadow   ggui.ShadowStyle // menus, select lists, date pickers, toasts
	OverlayShadow ggui.ShadowStyle // dialogs

	Radius   float64 // --radius: corner radius for boxes that ask for one
	RadiusSm float64 // rows and pills inside a rounded container
	RadiusLg float64 // cards, dialogs and toasts

	Space       float64 // the unit gaps and padding are multiples of
	BorderWidth float64 // the line Box.Border and the controls draw
	ControlSize float64 // a checkbox or radio glyph
	ControlGap  float64 // between a glyph and its label
	MenuWidth   float64 // a dropdown menu panel, which does not stretch

	ButtonPad ggui.EdgeInsets // inside a button
	FieldPad  ggui.EdgeInsets // inside a text field, select or other input
	ItemPad   ggui.EdgeInsets // around one row of a list or menu
	CardPad   ggui.EdgeInsets // inside a card
	PanelPad  ggui.EdgeInsets // inside a popup panel, around its items
	TabPad    ggui.EdgeInsets // inside one tab label

	MotionFast time.Duration // knobs, tab indicators, collapsing content
	MotionSlow time.Duration // entrances and exits

	// How far a state tints the color it starts from, as Mix takes it.
	HoverMix, PressMix, DisabledMix float64

	ext *tokenNode // extension tokens, a persistent list; see Set
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

// Default is a neutral light theme inspired by shadcn/ui, in Go Regular.
// The palette is shadcn's zinc scale, so its CSS variables map across a token
// at a time.
func Default() Theme {
	fg := color.RGBA{0x18, 0x18, 0x1b, 0xff}      // zinc-900
	quiet := color.RGBA{0xf4, 0xf4, 0xf5, 0xff}   // zinc-100
	mutedFg := color.RGBA{0x71, 0x71, 0x7a, 0xff} // zinc-500
	nearWhite := color.RGBA{0xfa, 0xfa, 0xfa, 0xff}
	border := color.RGBA{0xe4, 0xe4, 0xe7, 0xff} // zinc-200
	ring := color.RGBA{0xa1, 0xa1, 0xaa, 0xff}   // zinc-400
	return Theme{
		Chat:    defaultChat(),
		Text:    ggui.TextStyle{Size: 14, Color: fg, LineHeight: 1.4},
		Title:   ggui.TextStyle{Size: 24},
		Caption: ggui.TextStyle{Size: 12, Color: mutedFg},

		Bg: color.White, Fg: fg,
		Card: color.White, CardFg: fg,
		Popover: color.White, PopoverFg: fg,
		Primary: fg, PrimaryFg: nearWhite,
		PrimaryHover: color.RGBA{0x3f, 0x3f, 0x46, 0xff}, // zinc-700
		Secondary:    quiet, SecondaryFg: fg,
		Muted: quiet, MutedFg: mutedFg,
		Accent: quiet, AccentFg: fg,
		Destructive: color.RGBA{0xd3, 0x2f, 0x2f, 0xff}, DestructiveFg: nearWhite,
		Border:      border,
		Input:       color.White,
		InputBorder: border,
		Ring:        ring,
		Selection:   color.RGBA{0xd4, 0xd4, 0xd8, 0xff}, // zinc-300
		Scrim:       color.NRGBA{A: 0x60},

		// The sidebar palette follows shadcn: a near-white surface that
		// reuses the page's foreground, accent, border and ring.
		Sidebar: nearWhite, SidebarFg: fg,
		SidebarPrimary: fg, SidebarPrimaryFg: nearWhite,
		SidebarAccent: quiet, SidebarAccentFg: fg,
		SidebarBorder: border, SidebarRing: ring,

		CardShadow:    ggui.ShadowStyle{Offset: ggui.Pt(0, 1), Blur: 2, Color: color.NRGBA{A: 15}},
		PanelShadow:   ggui.ShadowStyle{Offset: ggui.Pt(0, 4), Blur: 10, Color: color.NRGBA{A: 26}},
		OverlayShadow: ggui.ShadowStyle{Offset: ggui.Pt(0, 12), Blur: 28, Color: color.NRGBA{A: 65}},

		Radius: 8, RadiusSm: 6, RadiusLg: 12,
		Space: 8, BorderWidth: 1, ControlSize: 16, ControlGap: 8, MenuWidth: 224,

		ButtonPad: ggui.Insets(8, 16), FieldPad: ggui.Insets(8, 12),
		ItemPad: ggui.Insets(6, 8), CardPad: ggui.Insets(24), PanelPad: ggui.Insets(4),
		TabPad: ggui.Insets(5, 10),

		MotionFast: 150 * time.Millisecond, MotionSlow: 240 * time.Millisecond,
		HoverMix: .06, PressMix: .08, DisabledMix: .55,
	}
}

// Dark is a dark counterpart of Default.
func Dark() Theme {
	t := Default()
	dark := color.RGBA{0x18, 0x18, 0x1b, 0xff}  // zinc-900
	quiet := color.RGBA{0x27, 0x27, 0x2a, 0xff} // zinc-800
	t.Fg = color.RGBA{0xfa, 0xfa, 0xfa, 0xff}
	t.Text.Color = t.Fg
	t.Bg = color.RGBA{0x09, 0x09, 0x0b, 0xff} // zinc-950
	t.Card, t.CardFg, t.Popover, t.PopoverFg = dark, t.Fg, dark, t.Fg
	t.Input = color.RGBA{0x20, 0x20, 0x23, 0xff}
	t.Border = color.RGBA{0x32, 0x32, 0x36, 0xff}
	t.InputBorder = t.Border
	t.Primary = color.RGBA{0xe4, 0xe4, 0xe7, 0xff}
	t.PrimaryHover = color.RGBA{0xd4, 0xd4, 0xd8, 0xff}
	t.PrimaryFg = dark
	t.Secondary, t.SecondaryFg = quiet, t.Fg
	t.Muted = quiet
	t.MutedFg = color.RGBA{0xa1, 0xa1, 0xaa, 0xff} // zinc-400
	t.Accent, t.AccentFg = quiet, t.Fg
	t.Ring = color.RGBA{0x71, 0x71, 0x7a, 0xff} // zinc-500
	t.Selection = color.RGBA{0x3f, 0x3f, 0x46, 0xff}
	t.Caption.Color = t.MutedFg
	t.Sidebar, t.SidebarFg = dark, t.Fg
	t.SidebarPrimary, t.SidebarPrimaryFg = t.Primary, t.PrimaryFg
	t.SidebarAccent, t.SidebarAccentFg = quiet, t.Fg
	t.SidebarBorder, t.SidebarRing = t.Border, t.Ring
	return t
}
