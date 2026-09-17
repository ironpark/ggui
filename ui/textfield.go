package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// TextFieldWidget is a TextInput in a themed box: Field background, a
// border that turns Accent while focused, the theme's padding and radius.
// Build one with TextField.
type TextFieldWidget struct {
	input *ggui.TextInputWidget
	box   *ggui.BoxWidget
	theme ggui.Theme
}

// TextField creates a text field bound to value.
func TextField(value *ggui.Signal[string]) *TextFieldWidget {
	f := &TextFieldWidget{input: ggui.TextInput(value)}
	f.box = ggui.Box(f.input)
	return f
}

// Placeholder sets the muted text shown while the value is empty.
func (f *TextFieldWidget) Placeholder(s string) *TextFieldWidget { f.input.Placeholder(s); return f }

// Password masks every rune with a bullet.
func (f *TextFieldWidget) Password() *TextFieldWidget { f.input.Password(); return f }

// MinWidth sets the width the field asks for when its parent leaves the
// width to it.
func (f *TextFieldWidget) MinWidth(w float64) *TextFieldWidget { f.input.MinWidth(w); return f }

// OnSubmit fires with the value when Enter is pressed.
func (f *TextFieldWidget) OnSubmit(fn func(string)) *TextFieldWidget { f.input.OnSubmit(fn); return f }

// OnChange fires with the value after every edit.
func (f *TextFieldWidget) OnChange(fn func(string)) *TextFieldWidget { f.input.OnChange(fn); return f }

// Input returns the editor inside, for Focused and the editor's own setters.
func (f *TextFieldWidget) Input() *ggui.TextInputWidget { return f.input }

// Layout implements Widget.
func (f *TextFieldWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	f.theme = t
	f.box.Pad(t.Space*0.75, t.Space).Radius(t.Radius).Fill(t.Field)
	return f.box.Layout(c, env)
}

// Paint implements Widget.
func (f *TextFieldWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	// The whole box, padding included, focuses and clicks into the editor.
	dst.HitPointer(r, f.input)
	dst.HitKey(r, f.input)
	dst.HitCursor(r, ebiten.CursorShapeText)
	f.box.Border(1, pick(f.input.Focused(), f.theme.Accent, f.theme.Border))
	dst.Paint(f.box, r)
}

// DividerWidget is a one-pixel line in the theme's Border color. Build one
