package ui

import (
	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Variant is a named look for a kind of control, worked out from the theme
// at layout: ButtonOutline is the look of an outline button, ButtonSmall
// the size of a small one. The built-in variants are package variables;
// NewVariant makes more, and Restyle changes one for every control under a
// theme, which is what editing a variant of a shadcn/ui component does.
type Variant[S any] struct {
	name string
	key  ggui.EnvKey[func(uitheme.Theme) S]
	base func(uitheme.Theme) S
}

// NewVariant makes a variant called name whose look under a theme is
// style's:
//
//	var ButtonBrand = ui.NewVariant("brand", func(t theme.Theme) ui.ButtonStyle {
//		s := ui.ButtonPrimary.Base(t)
//		s.Fill, s.Hover = brand, t.Hovered(brand)
//		return s
//	})
//	ui.Button("Upgrade", upgrade).Variant(ButtonBrand)
func NewVariant[S any](name string, style func(uitheme.Theme) S) *Variant[S] {
	return &Variant[S]{name: name, key: ggui.NewEnvKey[func(uitheme.Theme) S]("ui variant " + name), base: style}
}

// Style is v's look under t: the one Restyle gave it there, or its own.
func (v *Variant[S]) Style(t uitheme.Theme) S {
	if style, ok := t.Get(v.key); ok {
		return style(t)
	}
	return v.base(t)
}

// Base is v's own look under t, whatever Restyle did, for a restyle that
// changes the original rather than starting over.
func (v *Variant[S]) Base(t uitheme.Theme) S { return v.base(t) }

// Restyle returns t with v drawn by style instead, for every control using
// v under the theme:
//
//	t = ui.ButtonPrimary.Restyle(t, func(t theme.Theme) ui.ButtonStyle {
//		s := ui.ButtonPrimary.Base(t)
//		s.Shadow = false
//		return s
//	})
func (v *Variant[S]) Restyle(t uitheme.Theme, style func(uitheme.Theme) S) uitheme.Theme {
	return t.Set(v.key, style)
}

// String returns the variant's name.
func (v *Variant[S]) String() string { return v.name }
