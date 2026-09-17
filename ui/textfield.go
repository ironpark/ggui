package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// TextFieldWidget is a TextInput in a themed box: Field background, a
// border that turns Accent while focused, the theme's padding and radius.
// Build one with TextField.
type TextFieldWidget struct {
	ggui.Interactive // only disabled is used; the editor tracks its own focus
	input            *ggui.TextInputWidget
	box              *ggui.BoxWidget
	theme            ggui.Theme
}

// TextField creates a text field bound to value.
func TextField(value ggui.Binding[string]) *TextFieldWidget {
	f := &TextFieldWidget{input: ggui.TextInput(value)}
	f.box = ggui.Box(f.input)
	return f
}

// Disabled greys the field out and ignores input while v is true.
func (f *TextFieldWidget) Disabled(v bool) *TextFieldWidget {
	f.Inert = v
	f.input.Disabled(v)
	return f
}

// DisabledWhen follows r for Disabled without a rebuild.
func (f *TextFieldWidget) DisabledWhen(r ggui.Reader[bool]) *TextFieldWidget {
	f.InertWhen(r)
	f.input.DisabledWhen(r)
	return f
}

// Placeholder sets the muted text shown while the value is empty.
func (f *TextFieldWidget) Placeholder(s string) *TextFieldWidget { f.input.Placeholder(s); return f }

// Label names the field for Probe.Find and the inspector; the placeholder
// serves until one is set.
func (f *TextFieldWidget) Label(s string) *TextFieldWidget { f.input.Label(s); return f }

// SetName is Label, for Field.
func (f *TextFieldWidget) SetName(s string) { f.input.Label(s) }

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

// OnKey handles non-composing key presses before the editor; return true to consume.
func (f *TextFieldWidget) OnKey(fn func(ggui.KeyEvent) bool) *TextFieldWidget {
	f.input.OnKey(fn)
	return f
}

// OnSubmit fires with the value when Enter is pressed, or ⌘/Ctrl+Enter
// when Multiline.
func (f *TextFieldWidget) OnSubmit(fn func(string)) *TextFieldWidget { f.input.OnSubmit(fn); return f }

// OnChange fires with the value after every edit.
func (f *TextFieldWidget) OnChange(fn func(string)) *TextFieldWidget { f.input.OnChange(fn); return f }

// OnCommit fires with the value when the field loses focus or submits.
func (f *TextFieldWidget) OnCommit(fn func(string)) *TextFieldWidget { f.input.OnCommit(fn); return f }

// Input returns the editor inside, for Focused and the editor's own setters.
func (f *TextFieldWidget) Input() *ggui.TextInputWidget { return f.input }

// Layout implements Widget.
func (f *TextFieldWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	f.Sync()
	t := env.Theme()
	f.theme = t
	f.box.Padding(t.FieldPad).Radius(t.Radius).Fill(pick(f.Inert, t.Surface, t.Field))
	return f.box.Layout(c, env)
}

// Paint implements Widget.
func (f *TextFieldWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	// The whole box, padding included, focuses and clicks into the editor.
	f.Hit(dst, r, f.input, ebiten.CursorShapeText)
	f.box.Border(1, pick(f.input.Focused(), f.theme.Accent, f.theme.Border))
	dst.Paint(f.box, r)
}

// DividerWidget is a one-pixel line in the theme's Border color. Build one
