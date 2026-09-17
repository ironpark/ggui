package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// TextFieldWidget is a TextInput in a themed box: Field background, a
// border that turns Accent while focused, the theme's padding and radius.
// Build one with TextField.
type TextFieldWidget struct {
	input    *ggui.TextInputWidget
	box      *ggui.BoxWidget
	disabled bool
	theme    ggui.Theme
}

// TextField creates a text field bound to value.
func TextField(value *ggui.Signal[string]) *TextFieldWidget {
	f := &TextFieldWidget{input: ggui.TextInput(value)}
	f.box = ggui.Box(f.input)
	return f
}

// Disabled greys the field out and ignores input while v is true.
func (f *TextFieldWidget) Disabled(v bool) *TextFieldWidget {
	f.disabled = v
	f.input.Disabled(v)
	return f
}

// Placeholder sets the muted text shown while the value is empty.
func (f *TextFieldWidget) Placeholder(s string) *TextFieldWidget { f.input.Placeholder(s); return f }

// Password masks every rune with a bullet.
func (f *TextFieldWidget) Password() *TextFieldWidget { f.input.Password(); return f }

// MinWidth sets the width the field asks for when its parent leaves the
// width to it.
func (f *TextFieldWidget) MinWidth(w float64) *TextFieldWidget { f.input.MinWidth(w); return f }

// Multiline wraps the text and grows the field by the line; Enter breaks
// the line and ⌘/Ctrl+Enter submits.
func (f *TextFieldWidget) Multiline() *TextFieldWidget { f.input.Multiline(); return f }

// Lines sets the fewest lines a Multiline field is tall.
func (f *TextFieldWidget) Lines(n int) *TextFieldWidget { f.input.Lines(n); return f }

// OnSubmit fires with the value when Enter is pressed, or ⌘/Ctrl+Enter
// when Multiline.
func (f *TextFieldWidget) OnSubmit(fn func(string)) *TextFieldWidget { f.input.OnSubmit(fn); return f }

// OnChange fires with the value after every edit.
func (f *TextFieldWidget) OnChange(fn func(string)) *TextFieldWidget { f.input.OnChange(fn); return f }

// Input returns the editor inside, for Focused and the editor's own setters.
func (f *TextFieldWidget) Input() *ggui.TextInputWidget { return f.input }

// Layout implements Widget.
func (f *TextFieldWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	f.theme = t
	f.box.Padding(t.FieldPad).Radius(t.Radius).Fill(pick(f.disabled, t.Surface, t.Field))
	return f.box.Layout(c, env)
}

// Paint implements Widget.
func (f *TextFieldWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	// The whole box, padding included, focuses and clicks into the editor.
	if !f.disabled {
		dst.HitPointer(r, f.input)
		dst.HitKey(r, f.input)
		dst.HitCursor(r, ebiten.CursorShapeText)
	}
	f.box.Border(1, pick(f.input.Focused(), f.theme.Accent, f.theme.Border))
	dst.Paint(f.box, r)
}

// DividerWidget is a one-pixel line in the theme's Border color. Build one
