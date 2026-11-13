package ui

import (
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// TextFieldWidget is a TextInput in a themed box: Field background, a
// border that turns Accent while focused, the theme's padding and radius.
// Build one with TextField.
type TextFieldWidget struct {
	props            property.Owner
	multiline        bool
	ggui.Interactive // only disabled is used; the editor tracks its own focus
	input            *ggui.TextInputWidget
	box              *ggui.BoxWidget
	theme            *uitheme.Theme
	plain            bool
	invalid          bool
	motion           time.Duration // MotionFast, or zero under reduced motion
}

// TextField creates a text field bound to value.
func TextField(value ggui.Binding[string]) *TextFieldWidget {
	f := &TextFieldWidget{input: ggui.TextInput(value)}
	f.box = ggui.Box(f.input)
	return f
}

// Disabled greys the field out and ignores input while v is true.
func (f *TextFieldWidget) Disabled(v bool) *TextFieldWidget {
	f.SetInert(v)
	return f
}

// BindDisabled follows r for Disabled without a rebuild.
func (f *TextFieldWidget) BindDisabled(r ggui.Readable[bool]) *TextFieldWidget {
	f.BindInert(r)
	return f
}

// Placeholder sets the muted text shown while the value is empty.
func (f *TextFieldWidget) Placeholder(s string) *TextFieldWidget { f.input.Placeholder(s); return f }

// Name names the field for Probe.Find and the inspector; the placeholder
// serves until one is set.
func (f *TextFieldWidget) Name(s string) *TextFieldWidget { f.input.Name(s); return f }

// BindName binds the field's name to r; see ggui.Interactive.BindName.
func (f *TextFieldWidget) BindName(r ggui.Readable[string]) *TextFieldWidget {
	f.input.BindName(r)
	return f
}

// SetName is Name, for Field.
func (f *TextFieldWidget) SetName(s string) { f.input.Name(s) }

// HasName reports whether the editor has an explicit name.
func (f *TextFieldWidget) HasName() bool { return f.input.HasName() }

// Semantics reports the same name and role as the editor.
func (f *TextFieldWidget) Semantics() (ggui.Role, string) { return f.input.Semantics() }

// Password masks every rune with a bullet.
func (f *TextFieldWidget) Password() *TextFieldWidget { f.input.Password(); return f }

// MinWidth sets the width the field asks for when its parent leaves the
// width to it.
func (f *TextFieldWidget) MinWidth(w float64) *TextFieldWidget { f.input.MinWidth(w); return f }

// Multiline wraps the text and grows the field by the line; Enter breaks
// the line and ⌘/Ctrl+Enter submits.
func (f *TextFieldWidget) Multiline() *TextFieldWidget { return f.Lines(1) }

// NoWrap makes the field Multiline with lines that do not wrap.
func (f *TextFieldWidget) NoWrap() *TextFieldWidget { f.Multiline(); f.input.NoWrap(); return f }

// Lines sets the fewest lines a Multiline field is tall.
func (f *TextFieldWidget) Lines(n int) *TextFieldWidget {
	defer property.Watch(&f.props, &f.multiline)()
	f.multiline = true
	f.input.Lines(n)
	return f
}

// OnKey handles non-composing key presses before the editor; return true to consume.
func (f *TextFieldWidget) OnKey(fn func(ggui.KeyEvent) bool) *TextFieldWidget {
	f.input.OnKey(fn)
	return f
}

// ClaimKeys keeps the keys fn reports with the field while it has the
// focus, ahead of Tab moving the focus and shortcuts; see
// ggui.TextInputWidget.ClaimKeys.
func (f *TextFieldWidget) ClaimKeys(fn func(ggui.KeyEvent) bool) *TextFieldWidget {
	f.input.ClaimKeys(fn)
	return f
}

// OnPasteImage takes a paste of an image; see TextInputWidget.OnPasteImage.
func (f *TextFieldWidget) OnPasteImage(fn func(png []byte) bool) *TextFieldWidget {
	f.input.OnPasteImage(fn)
	return f
}

// OnSubmit fires with the value when Enter is pressed, or ⌘/Ctrl+Enter
// when Multiline.
func (f *TextFieldWidget) OnSubmit(fn func(string)) *TextFieldWidget { f.input.OnSubmit(fn); return f }

// OnChange fires with the value after every edit.
func (f *TextFieldWidget) OnChange(fn func(string)) *TextFieldWidget { f.input.OnChange(fn); return f }

// OnCommit fires with the value when the field loses focus or submits.
func (f *TextFieldWidget) OnCommit(fn func(string)) *TextFieldWidget { f.input.OnCommit(fn); return f }

// OnDescribe amends the node the field describes; see
// ggui.TextInputWidget.OnDescribe.
func (f *TextFieldWidget) OnDescribe(fn func(*ggui.Node)) *TextFieldWidget {
	f.input.OnDescribe(fn)
	return f
}

// OnAction gives fn the first say on an assistive technology's action; see
// ggui.TextInputWidget.OnAction.
func (f *TextFieldWidget) OnAction(fn func(ggui.Action) bool) *TextFieldWidget {
	f.input.OnAction(fn)
	return f
}

// Key gives the editor an identity, so a rebuilt field that also moved
// keeps its caret and focus. The editor registers the field's regions, so
// its identity is the one that counts, not Interactive's.
func (f *TextFieldWidget) Key(k any) *TextFieldWidget { f.input.Key(k); return f }

// Filter installs a prefix-preserving function every edit passes through;
// see TextInputWidget.Filter.
func (f *TextFieldWidget) Filter(fn func(string) string) *TextFieldWidget {
	f.input.Filter(fn)
	return f
}

// Style merges ts onto the editor's text style.
func (f *TextFieldWidget) Style(ts ggui.TextStyle) *TextFieldWidget { f.input.Style(ts); return f }

// Input returns the editor inside, for Focused and the editor's own setters.
func (f *TextFieldWidget) Input() *ggui.TextInputWidget { return f.input }

// Layout implements Widget.
func (f *TextFieldWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer f.props.Layout().EndLayout()
	f.Sync()
	t := uitheme.Of(env)
	f.theme = t
	f.motion = env.Motion(t.MotionFast)
	f.invalid, _ = env.Get(fieldInvalid)
	if f.plain {
		// A bare editor is padded by a field's side padding above and
		// below, and by the theme's Space left and right.
		f.box.Fill(nil).Padding(ggui.Insets(t.FieldPad.Left, t.Space)).Radius(t.Radius).Border(0, nil)
		return f.box.Layout(c, env.With(ggui.InputDisabledKey, f.IsInert()))
	}
	pad := t.FieldPad
	if f.multiline {
		pad = ggui.Insets(t.Space, t.FieldPad.Left)
	}
	// Only the shape is set here: the colors follow the field's state in
	// Paint, where fieldChrome eases them.
	fieldBoxShape(f.box, t, pad)
	// A line of text and the padding, or shadcn's four-line minimum of a
	// textarea: eight of the theme's Space.
	c.MinH = min(c.MaxH, max(c.MinH, pick(f.multiline, t.Space*8, 20+t.FieldPad.Top+t.FieldPad.Bottom)))
	return f.box.Layout(c, env.With(ggui.InputDisabledKey, f.IsInert()))
}

// Baseline implements ggui.Baseliner: the editor's, inside the padding.
func (f *TextFieldWidget) Baseline() (float64, bool) { return f.box.Baseline() }

// Paint implements Widget.
func (f *TextFieldWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	// The whole box, padding included, focuses and clicks into the editor.
	inert := f.input.IsDisabled()
	if inert {
		dst.Describe(r, f.input)
	} else {
		f.Hit(dst, r, f.input, ggui.CursorShapeText)
	}
	if !f.plain {
		at := ggui.Anchor{Rect: r, ID: f.input.HitID()}
		fieldChrome(dst, at, f.box, f.theme, r, f.theme.Radius, fieldState{f.input.Focused(), f.invalid, inert}, f.motion)
	}
	dst.Paint(f.box, r)
}

// DividerWidget is a one-pixel line in the theme's Border color. Build one
