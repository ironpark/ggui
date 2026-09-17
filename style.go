package ggui

import "image/color"

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
}

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
	e.text = e.text.Merge(s)
	return e
}

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
	e.theme, e.hasTheme = t, true
	return e
}

// Key names a value that can travel down the tree in an Env. Make one per
// concept with NewKey; the type parameter keeps reads and writes in step.
type Key[T any] struct {
	id   *byte
	name string
}

// NewKey creates a distinct Key; name is for messages only.
func NewKey[T any](name string) Key[T] { return Key[T]{id: new(byte), name: name} }

// With returns e with v stored under k for the subtree below.
func (e Env) With[T any](k Key[T], v T) Env {
	e.vals = &envNode{key: k, val: v, next: e.vals}
	return e
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
	CardPad   EdgeInsets // inside a card or panel
}

// DefaultTheme is a light theme in Go Regular.
func DefaultTheme() Theme {
	fg := color.RGBA{0x1f, 0x23, 0x28, 0xff}
	return Theme{
		Text:        TextStyle{Size: DefaultTextSize, Color: fg},
		Title:       TextStyle{Size: 24},
		Caption:     TextStyle{Size: 12, Color: color.RGBA{0x6b, 0x72, 0x7c, 0xff}},
		Fg:          fg,
		Bg:          color.White,
		Surface:     color.RGBA{0xf2, 0xf3, 0xf5, 0xff},
		Field:       color.White,
		Border:      color.RGBA{0xd0, 0xd4, 0xda, 0xff},
		Accent:      color.RGBA{0x2f, 0x6f, 0xeb, 0xff},
		AccentHover: color.RGBA{0x24, 0x5c, 0xc7, 0xff},
		OnAccent:    color.White,
		Selection:   color.RGBA{0x2f, 0x6f, 0xeb, 0x50},
		Muted:       color.RGBA{0x6b, 0x72, 0x7c, 0xff},
		Radius:      6,
		Space:       8,
		ButtonPad:   Insets(6, 16),
		FieldPad:    Insets(6, 8),
		ItemPad:     Insets(4, 8),
		CardPad:     Insets(16),
	}
}

// DarkTheme is a dark counterpart of DefaultTheme.
func DarkTheme() Theme {
	t := DefaultTheme()
	t.Fg = color.RGBA{0xe6, 0xe8, 0xec, 0xff}
	t.Text.Color = t.Fg
	t.Bg = color.RGBA{0x14, 0x16, 0x1a, 0xff}
	t.Surface = color.RGBA{0x23, 0x27, 0x2f, 0xff}
	t.Field = color.RGBA{0x1a, 0x1d, 0x23, 0xff}
	t.Border = color.RGBA{0x3a, 0x40, 0x4c, 0xff}
	t.Accent = color.RGBA{0x4f, 0x8c, 0xff, 0xff}
	t.AccentHover = color.RGBA{0x6c, 0x9f, 0xff, 0xff}
	t.OnAccent = color.RGBA{0x0e, 0x12, 0x1a, 0xff}
	t.Selection = color.RGBA{0x4f, 0x8c, 0xff, 0x60}
	t.Muted = color.RGBA{0x8a, 0x90, 0x9c, 0xff}
	t.Caption.Color = t.Muted
	return t
}

// Colors hold interfaces, so the signal notifies on every Set rather than
// risking a comparison of uncomparable dynamic types.
var theme = State(DefaultTheme()).WithEqual(nil)

// SetTheme replaces the theme. Builders that read it through UseTheme rebuild.
func SetTheme(t Theme) { theme.Set(t) }

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

// rootEnv is the Env the runtime lays the tree out under.
func rootEnv() Env {
	t := theme.Peek()
	return Env{}.WithTheme(t).WithText(t.Text)
}
