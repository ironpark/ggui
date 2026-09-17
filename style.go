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
var TextScaleKey = NewKey[float64]("text scale")

// ReducedMotionKey asks widgets not to animate: transitions land at once,
// eased motions jump. Provide it above the tree; Env.Motion reads it.
var ReducedMotionKey = NewKey[bool]("reduced motion")

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

// Key names a value that can travel down the tree in an Env. Make one per
// concept with NewKey; the type parameter keeps reads and writes in step.
type Key[T any] struct {
	id   *byte
	name string
}

// NewKey creates a distinct Key; name is for messages only.
func NewKey[T any](name string) Key[T] { return Key[T]{id: new(byte), name: name} }

// With returns e with v stored under k for the subtree below. Storing the
// value already there, by ==, leaves e unchanged, and storing the value
// stored last frame under the same parent yields the same revision, so
// caches below a Provide rebuilt every frame hold.
func (e Env) With[T any](k Key[T], v T) Env {
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
func (e Env) Get[T any](k Key[T]) (T, bool) {
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
type Theme struct {
	Text    TextStyle // the base every Text inherits
	Title   TextStyle // merged onto Text for headings
	Caption TextStyle // merged onto Text for small secondary text

	Fg, Bg      color.Color // default text and window colors
	Surface     color.Color // panels and cards
	Field       color.Color // text fields and other inputs
	Border      color.Color // outlines of inputs and dividers
	Accent      color.Color // primary buttons, checked controls, focus rings
	AccentHover color.Color
	OnAccent    color.Color // text and marks drawn on Accent
	Selection   color.Color // selected text
	Muted       color.Color // secondary text, placeholders, disabled controls

	Radius float64 // corner radius for boxes that ask for one
	Space  float64 // the unit gaps and padding are multiples of

	ButtonPad EdgeInsets // inside a button
	FieldPad  EdgeInsets // inside a text field, select or other input
	ItemPad   EdgeInsets // around one row of a list or menu
	CardPad   EdgeInsets // inside a card
	PanelPad  EdgeInsets // inside a popup panel, around its items

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
//	var DangerColor = ggui.NewKey[color.Color]("danger")
//	theme = theme.Set(DangerColor, color.RGBA{0xd3, 0x2f, 0x2f, 0xff})
func (t Theme) Set[T any](k Key[T], v T) Theme {
	t.ext = &tokenNode{key: k, val: v, next: t.ext}
	return t
}

// Get returns the token stored under k, if Set stored one.
func (t Theme) Get[T any](k Key[T]) (T, bool) {
	for n := t.ext; n != nil; n = n.next {
		if n.key == any(k) {
			return n.val.(T), true
		}
	}
	var zero T
	return zero, false
}

// DefaultTheme is a neutral light theme inspired by shadcn/ui, in Go Regular.
func DefaultTheme() Theme {
	fg := color.RGBA{0x18, 0x18, 0x1b, 0xff}
	muted := color.RGBA{0x71, 0x71, 0x7a, 0xff}
	return Theme{
		Text:    TextStyle{Size: 14, Color: fg, LineHeight: 1.4},
		Title:   TextStyle{Size: 24},
		Caption: TextStyle{Size: 12, Color: muted},
		Fg:      fg, Bg: color.White, Surface: color.White, Field: color.White,
		Border: color.RGBA{0xe4, 0xe4, 0xe7, 0xff},
		Accent: fg, AccentHover: color.RGBA{0x3f, 0x3f, 0x46, 0xff},
		OnAccent:  color.RGBA{0xfa, 0xfa, 0xfa, 0xff},
		Selection: color.RGBA{0xd4, 0xd4, 0xd8, 0xff},
		Muted:     muted, Radius: 8, Space: 8,
		ButtonPad: Insets(8, 16), FieldPad: Insets(8, 12),
		ItemPad: Insets(6, 8), CardPad: Insets(24), PanelPad: Insets(4),
	}
}

// DarkTheme is a dark counterpart of DefaultTheme.
func DarkTheme() Theme {
	t := DefaultTheme()
	t.Fg = color.RGBA{0xfa, 0xfa, 0xfa, 0xff}
	t.Text.Color = t.Fg
	t.Bg = color.RGBA{0x09, 0x09, 0x0b, 0xff}
	t.Surface = color.RGBA{0x18, 0x18, 0x1b, 0xff}
	t.Field = color.RGBA{0x20, 0x20, 0x23, 0xff}
	t.Border = color.RGBA{0x32, 0x32, 0x36, 0xff}
	t.Accent = color.RGBA{0xe4, 0xe4, 0xe7, 0xff}
	t.AccentHover = color.RGBA{0xd4, 0xd4, 0xd8, 0xff}
	t.OnAccent = color.RGBA{0x18, 0x18, 0x1b, 0xff}
	t.Selection = color.RGBA{0x3f, 0x3f, 0x46, 0xff}
	t.Muted = color.RGBA{0xa1, 0xa1, 0xaa, 0xff}
	t.Caption.Color = t.Muted
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
