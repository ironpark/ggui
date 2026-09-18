package ggui

import (
	"image/color"
	"sync/atomic"
	"time"
)

// Styling has three layers. A TextStyle is a value: build one, merge others
// onto it, hand it to Text or Styled. An Env flows down the tree at layout
// time, so a Styled container sets the text style every descendant starts
// from, the way CSS inherits font and color. A Theme is the app's tokens
// (colors, spacing, named text styles), read at build time with UseTheme and
// swapped with SetTheme.

// TextStyle describes how text is drawn. The zero value of a field means
// "inherit": Merge lets a set field win over an unset one, and Text resolves
// what is still unset from the Env and then from built-in defaults.
type TextStyle struct {
	Font       *Font
	Size       float64 // pixels
	Color      color.Color
	LineHeight float64 // multiple of Size between baselines
}

// Merge returns s with every set field of o laid over it.
func (s TextStyle) Merge(o TextStyle) TextStyle {
	if o.Font != nil {
		s.Font = o.Font
	}
	if o.Size != 0 {
		s.Size = o.Size
	}
	if o.Color != nil {
		s.Color = o.Color
	}
	if o.LineHeight != 0 {
		s.LineHeight = o.LineHeight
	}
	return s
}

// resolved fills what is still unset with the built-in defaults.
func (s TextStyle) resolved() TextStyle {
	if s.Font == nil {
		s.Font = fallbackFont()
	}
	if s.Size == 0 {
		s.Size = DefaultTextSize
	}
	if s.Color == nil {
		s.Color = color.Black
	}
	if s.LineHeight == 0 {
		s.LineHeight = 1.2
	}
	return s
}

// Env is the set of inherited values a widget lays out under. The runtime
// hands the root one to the tree; containers pass it down unchanged, and
// Styled or Provide hand their child a modified copy. It is a value: adding
// to it never changes the parent's.
type Env struct {
	text     TextStyle
	vals     *envNode
	theme    Theme
	hasTheme bool
	rev      uint64 // advanced by every change; Cached compares it
}

// envRev numbers every distinct Env, so a layout cache can tell whether
// anything inherited changed without comparing the values themselves.
var envRev atomic.Uint64

func nextRev() uint64 { return envRev.Add(1) }

type envNode struct {
	key  any
	val  any
	next *envNode
}

// Text returns the inherited text style, the base a Text merges its own
// style onto.
func (e Env) Text() TextStyle { return e.text }

// WithText returns e with s merged onto the inherited text style.
func (e Env) WithText(s TextStyle) Env {
	merged := e.text.Merge(s)
	if sameAny(merged, e.text) {
		return e
	}
	return e.derive(textKey, merged, func() Env {
		e.text, e.rev = merged, nextRev()
		return e
	})
}

var textKey, themeKey = new(byte), new(byte)

// Theme returns the theme the tree is laid out under: what the runtime put
// in the root Env, or DefaultTheme for an Env made by hand, as in tests.
// Built-in controls take their colors from it at layout time; a Builder
// reads the theme with UseTheme instead.
func (e Env) Theme() Theme {
	if !e.hasTheme {
		return DefaultTheme()
	}
	return e.theme
}

// WithTheme returns e with t as the theme for the subtree below.
func (e Env) WithTheme(t Theme) Env {
	return e.derive(themeKey, t, func() Env {
		e.theme, e.hasTheme, e.rev = t, true, nextRev()
		return e
	})
}

// TextScaleKey holds the factor every Text and TextInput multiplies its
// size by, for a user who asked for larger text: Provide it above the tree
// or a subtree. Env.TextScale reads it, 1 by default.
var TextScaleKey = NewEnvKey[float64]("text scale")

// InputDisabled disables TextInput editing in a subtree without replacing the
// editor's own Disabled or DisabledWhen setting. Containers combine inherited
// and local values with OR; false must not enable an already-disabled ancestor.
var InputDisabled = NewEnvKey[bool]("input disabled")

// ReducedMotionKey asks widgets not to animate: transitions land at once,
// eased motions jump. Provide it above the tree; Env.Motion reads it.
var ReducedMotionKey = NewEnvKey[bool]("reduced motion")

// TextScale returns the factor text sizes are multiplied by under e.
func (e Env) TextScale() float64 {
	if s, ok := e.Get(TextScaleKey); ok && s > 0 {
		return s
	}
	return 1
}

// ReducedMotion reports whether the tree under e asked for no animation.
func (e Env) ReducedMotion() bool {
	r, _ := e.Get(ReducedMotionKey)
	return r
}

// Motion returns d, or zero when the tree under e asked for reduced
// motion: what a widget hands to Ease or a Transition.
func (e Env) Motion(d time.Duration) time.Duration {
	if e.ReducedMotion() {
		return 0
	}
	return d
}

// EnvKey names a value that can travel down the tree in an Env. Make one per
// concept with NewEnvKey; the type parameter keeps reads and writes in step.
type EnvKey[T any] struct {
	id   *byte
	name string
}

// NewEnvKey creates a distinct EnvKey; name is for messages only.
func NewEnvKey[T any](name string) EnvKey[T] { return EnvKey[T]{id: new(byte), name: name} }

// With returns e with v stored under k for the subtree below. Storing the
// value already there, by ==, leaves e unchanged, and storing the value
// stored last frame under the same parent yields the same revision, so
// caches below a Provide rebuilt every frame hold. InputDisabled is cumulative:
// once true in an ancestor, a descendant cannot clear it.
func (e Env) With[T any](k EnvKey[T], v T) Env {
	if k.id == InputDisabled.id {
		if disabled, _ := e.Get(InputDisabled); disabled {
			return e
		}
	}
	if cur, ok := e.Get(k); ok && sameAny(cur, v) {
		return e
	}
	return e.derive(k, v, func() Env {
		e.vals = &envNode{key: k, val: v, next: e.vals}
		e.rev = nextRev()
		return e
	})
}

// derivedKey names an Env derived from another: the parent's revision and
// what was added.
type derivedKey struct {
	from uint64
	key  any
	val  any
}

// envMemo remembers the Envs derived this frame and last, so the same
// derivation yields the same revision frame after frame. Canvas.nextFrame
// rotates it.
var envMemo struct {
	cur, prev map[derivedKey]Env
}

func rotateEnvMemo() {
	envMemo.prev, envMemo.cur = envMemo.cur, envMemo.prev
	clear(envMemo.cur)
}

// derive returns the Env derived from e with key and val last frame or
// this one, else fn's. A val that cannot be hashed is never memoized.
func (e Env) derive(key, val any, fn func() Env) (out Env) {
	k := derivedKey{e.rev, key, val}
	memoized := true
	func() {
		defer func() {
			if recover() != nil {
				memoized = false
			}
		}()
		var ok bool
		if out, ok = envMemo.cur[k]; ok {
			return
		}
		if out, ok = envMemo.prev[k]; ok {
			return
		}
		ok = false
		out = fn()
		if envMemo.cur == nil {
			envMemo.cur = map[derivedKey]Env{}
		}
		envMemo.cur[k] = out
	}()
	if !memoized {
		return fn()
	}
	return out
}

// Get returns the nearest value stored under k, if any ancestor set one.
func (e Env) Get[T any](k EnvKey[T]) (T, bool) {
	for n := e.vals; n != nil; n = n.next {
		if n.key == any(k) {
			return n.val.(T), true
		}
	}
	var zero T
	return zero, false
}

// Theme is the app's design tokens. Read it at build time with UseTheme;
// Text starts from Theme.Text through the root Env.
//
// Derive a theme from DefaultTheme or DarkTheme rather than writing a Theme
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

	Text    TextStyle // the base every Text inherits
	Title   TextStyle // merged onto Text for headings
	Caption TextStyle // merged onto Text for small secondary text

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
	CardShadow    ShadowStyle // cards and the raised tab
	PanelShadow   ShadowStyle // menus, select lists, date pickers, toasts
	OverlayShadow ShadowStyle // dialogs

	Radius   float64 // --radius: corner radius for boxes that ask for one
	RadiusSm float64 // rows and pills inside a rounded container
	RadiusLg float64 // cards, dialogs and toasts

	Space       float64 // the unit gaps and padding are multiples of
	BorderWidth float64 // the line Box.Border and the controls draw
	ControlSize float64 // a checkbox or radio glyph
	ControlGap  float64 // between a glyph and its label
	MenuWidth   float64 // a dropdown menu panel, which does not stretch

	ButtonPad EdgeInsets // inside a button
	FieldPad  EdgeInsets // inside a text field, select or other input
	ItemPad   EdgeInsets // around one row of a list or menu
	CardPad   EdgeInsets // inside a card
	PanelPad  EdgeInsets // inside a popup panel, around its items
	TabPad    EdgeInsets // inside one tab label

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
func (t Theme) Set[T any](k EnvKey[T], v T) Theme {
	t.ext = &tokenNode{key: k, val: v, next: t.ext}
	return t
}

// Get returns the token stored under k, if Set stored one.
func (t Theme) Get[T any](k EnvKey[T]) (T, bool) {
	for n := t.ext; n != nil; n = n.next {
		if n.key == any(k) {
			return n.val.(T), true
		}
	}
	var zero T
	return zero, false
}

// DefaultTheme is a neutral light theme inspired by shadcn/ui, in Go Regular.
// The palette is shadcn's zinc scale, so its CSS variables map across a token
// at a time.
func DefaultTheme() Theme {
	fg := color.RGBA{0x18, 0x18, 0x1b, 0xff}      // zinc-900
	quiet := color.RGBA{0xf4, 0xf4, 0xf5, 0xff}   // zinc-100
	mutedFg := color.RGBA{0x71, 0x71, 0x7a, 0xff} // zinc-500
	nearWhite := color.RGBA{0xfa, 0xfa, 0xfa, 0xff}
	border := color.RGBA{0xe4, 0xe4, 0xe7, 0xff} // zinc-200
	ring := color.RGBA{0xa1, 0xa1, 0xaa, 0xff}   // zinc-400
	return Theme{
		Chat:    defaultChat(),
		Text:    TextStyle{Size: 14, Color: fg, LineHeight: 1.4},
		Title:   TextStyle{Size: 24},
		Caption: TextStyle{Size: 12, Color: mutedFg},

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

		CardShadow:    ShadowStyle{Offset: Pt(0, 1), Blur: 2, Color: color.NRGBA{A: 15}},
		PanelShadow:   ShadowStyle{Offset: Pt(0, 4), Blur: 10, Color: color.NRGBA{A: 26}},
		OverlayShadow: ShadowStyle{Offset: Pt(0, 12), Blur: 28, Color: color.NRGBA{A: 65}},

		Radius: 8, RadiusSm: 6, RadiusLg: 12,
		Space: 8, BorderWidth: 1, ControlSize: 16, ControlGap: 8, MenuWidth: 224,

		ButtonPad: Insets(8, 16), FieldPad: Insets(8, 12),
		ItemPad: Insets(6, 8), CardPad: Insets(24), PanelPad: Insets(4),
		TabPad: Insets(5, 10),

		MotionFast: 150 * time.Millisecond, MotionSlow: 240 * time.Millisecond,
		HoverMix: .06, PressMix: .08, DisabledMix: .55,
	}
}

// DarkTheme is a dark counterpart of DefaultTheme.
func DarkTheme() Theme {
	t := DefaultTheme()
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

// Colors hold interfaces, so the signal notifies on every Set rather than
// risking a comparison of uncomparable dynamic types.
var theme = State(DefaultTheme()).WithEqual(nil)

// SetTheme replaces the theme. Builders that read it through UseTheme rebuild.
func SetTheme(t Theme) { theme.Set(t); themeGen.Add(1) }

// themeGen counts SetTheme calls, so rootEnv is rebuilt, with a new
// revision, only when the theme changed.
var themeGen atomic.Uint64

// BindTheme follows a boolean signal with the theme: on while it is true,
// off otherwise, starting now. It returns a dispose function, like Watch.
//
//	dark := ggui.State(false)
//	ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme())
func BindTheme(sw Reader[bool], on, off Theme) (dispose func()) {
	return Watch(sw, func(v bool) { SetTheme(pick(v, on, off)) })
}

// UseTheme returns the current theme and, inside a Builder or Effect,
// subscribes it to theme changes.
func UseTheme() Theme { return theme.Get() }

// rootEnv is the Env the runtime lays the tree out under. It is the same
// value frame after frame until SetTheme, so caches below it hold.
func rootEnv() Env {
	gen := themeGen.Load()
	if !rootCache.env.hasTheme || rootCache.gen != gen {
		t := theme.Peek()
		rootCache.env, rootCache.gen = Env{}.WithTheme(t).WithText(t.Text), gen
	}
	return rootCache.env
}

var rootCache struct {
	gen uint64
	env Env
}
