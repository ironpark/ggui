package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// FieldWidget is a labelled input: a caption above any control, help text
// below it, and an error in place of the help while one is set. Build one
// with Field. The label names the control for Probe.Find and the
// inspector when the control has no name of its own.
type FieldWidget struct {
	props property.Owner
	label string
	input ggui.Widget
	help  string
	err   property.Value[string]

	caption *ggui.TextWidget
	note    *ggui.TextWidget
	column  *ggui.ColumnWidget
}

var fieldInvalid = ggui.NewEnvKey[bool]("field invalid")

// Named is a control that accepts an explicit name. HasName excludes
// placeholders and built-in fallback names.
type Named interface {
	SetName(string)
	HasName() bool
}

// Field puts label above input.
//
//	ui.Field("Email", ui.TextField(email).Placeholder("you@example.com")).
//		Help("We never share it").BindError(emailError)
func Field(label string, input ggui.Widget) *FieldWidget {
	f := &FieldWidget{label: label, input: input}
	if n, ok := input.(Named); ok {
		if !n.HasName() {
			n.SetName(label)
		}
	}
	f.caption = ggui.Text(label).NoWrap()
	f.note = ggui.Text("")
	return f
}

// Help sets the muted text under the input.
func (f *FieldWidget) Help(s string) *FieldWidget {
	defer property.Watch(&f.props, &f.help)()
	f.help = s
	return f
}

// Error follows r: while it is not empty it is shown under the input in
// the theme's Destructive color, in place of the help text, without a rebuild.
func (f *FieldWidget) Error(v string) *FieldWidget {
	if f.err.Set(v) {
		f.props.Changed()
	}
	return f
}

// BindError follows an error message without owning its reader.
func (f *FieldWidget) BindError(r ggui.Readable[string]) *FieldWidget {
	if f.err.Bind(r, "BindError") {
		f.props.Changed()
	}
	return f
}

// Layout implements Widget.
func (f *FieldWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer f.props.Layout()()
	t := uitheme.From(env)
	note, col := f.help, t.MutedFg
	invalid := false
	if e := f.err.Get(); e != "" {
		note, col = e, t.Destructive
		invalid = true
	}
	f.caption.Style(t.Text).Color(t.Fg)
	f.note.Content(note).Style(t.Caption).Color(col)
	parts := []ggui.Widget{f.caption, f.input}
	if note != "" {
		parts = append(parts, f.note)
	}
	f.column = ggui.Column(parts...).Gap(t.Space / 2).Align(ggui.AlignStretch)
	if invalid {
		env = env.With(fieldInvalid, true)
	}
	return f.column.Layout(c, env)
}

// Paint implements Widget.
func (f *FieldWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(f.column, r) }
