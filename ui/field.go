package ui

import (
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// FieldWidget is a labelled input: a caption above any control, a muted
// description below it, and an error below that while one is set. Build
// one with Field. The label names the control for Probe.Find and the
// inspector when the control has no name of its own.
//
// While it holds an error the field is invalid: its caption turns
// destructive, and the control inside draws its invalid state.
type FieldWidget struct {
	props        property.Owner
	label        string
	input        ggui.Widget
	help         string
	err          property.Value[string]
	errs         property.Value[[]string]
	horizontal   bool
	controlFirst bool
	carded       bool

	caption *LabelWidget
	note    *ggui.TextWidget // the description
	alert   *ggui.TextWidget // the error
	alerts  fieldErrorList   // the errors, when there are several
	column  *ggui.ColumnWidget
	row     *ggui.RowWidget  // the body of a Horizontal field, holding column
	card    *FieldCardWidget // the body of a Card field
	shape   fieldShape       // what column, row and card were built for
}

// fieldShape is what decides which widgets a Field's body holds.
type fieldShape struct {
	noted, alerted, horizontal, controlFirst bool
	listed, carded                           bool
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
	f.caption = Label(label).For(input)
	f.note, f.alert = ggui.Text(""), ggui.Text("")
	return f
}

// Help sets the muted description under the input.
func (f *FieldWidget) Help(s string) *FieldWidget {
	defer property.Watch(&f.props, &f.help)()
	f.help = s
	return f
}

// Error sets the error: while it is not empty it is shown under the input
// and its description in the theme's Destructive color, and the field is
// invalid.
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

// Errors sets every error the field holds at once, as a form library
// reports them: shadcn's FieldError with errors. Empty and repeated
// messages are dropped; one that is left shows as Error's does, and
// several as a bulleted list. The field is invalid while any is left.
func (f *FieldWidget) Errors(messages []string) *FieldWidget {
	if f.errs.Set(messages) {
		f.props.Changed()
	}
	return f
}

// BindErrors follows a list of error messages without owning its reader.
func (f *FieldWidget) BindErrors(r ggui.Readable[[]string]) *FieldWidget {
	if f.errs.Bind(r, "BindErrors") {
		f.props.Changed()
	}
	return f
}

// messages are the errors to show: Error's, then those of Errors, each
// once.
func (f *FieldWidget) messages() []string {
	single, list := f.err.Get(), f.errs.Get()
	if single == "" && len(list) == 0 {
		return nil
	}
	out := make([]string, 0, len(list)+1)
	for _, m := range append([]string{single}, list...) {
		if m != "" && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// Card makes the field a choice card, shadcn's FieldLabel around a Field:
// a bordered box holding the label, with the description and the error
// under it, beside the control. A click anywhere in it acts on the
// control, and it is tinted while the control is checked. The control sits
// at the end unless ControlFirst puts it at the start. See FieldCard.
//
//	ui.Field("Kubernetes", ui.Radio(target, "k8s", "")).
//		Help("Run GPU workloads on a K8s cluster.").Card()
func (f *FieldWidget) Card() *FieldWidget {
	defer property.Watch(&f.props, &f.carded)()
	f.carded = true
	return f
}

// Horizontal lays the field out as a row, shadcn's horizontal orientation:
// the label, with the description and the error under it, takes the width
// that is left and the control sits at the end, as a switch does in a list
// of settings. ControlFirst puts the control at the start instead.
func (f *FieldWidget) Horizontal() *FieldWidget {
	defer property.Watch(&f.props, &f.horizontal)()
	f.horizontal = true
	return f
}

// ControlFirst makes the field Horizontal with the control before the
// label, as a checkbox or a radio sits beside what it chooses.
func (f *FieldWidget) ControlFirst() *FieldWidget {
	defer property.Watch(&f.props, &f.horizontal)()
	defer property.Watch(&f.props, &f.controlFirst)()
	f.horizontal, f.controlFirst = true, true
	return f
}

// Layout implements Widget.
func (f *FieldWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer f.props.Layout().EndLayout()
	t := uitheme.Of(env)
	errors := f.messages()
	e := ""
	if len(errors) == 1 {
		e = errors[0]
	} else if len(errors) > 1 && !slices.Equal(errors, f.alerts.messages) {
		f.alerts.set(errors)
	}
	// The description and the error are body text, as shadcn's text-sm,
	// and both show: the error does not take the description's place.
	f.note.Content(f.help).Style(t.Text).Color(t.MutedFg)
	f.alert.Content(e).Style(t.Text).Color(t.Destructive)
	// The body is rebuilt only when a part comes or goes, so its layout
	// cache and the caption's survive a relayout.
	if shape := (fieldShape{f.help != "", len(errors) > 0, f.horizontal, f.controlFirst, len(errors) > 1, f.carded}); f.column == nil || shape != f.shape {
		f.shape = shape
		parts := make([]ggui.Widget, 0, 4)
		if shape.carded {
			parts = append(parts, FieldTitle(f.label))
		} else {
			parts = append(parts, f.caption)
		}
		if !shape.horizontal && !shape.carded {
			parts = append(parts, f.input)
		}
		if shape.noted && shape.carded {
			parts = append(parts, FieldDescription(f.help))
		} else if shape.noted {
			parts = append(parts, f.note)
		}
		if shape.listed {
			parts = append(parts, &f.alerts)
		} else if shape.alerted {
			parts = append(parts, f.alert)
		}
		f.column, f.row, f.card = ggui.Column(parts...).Align(ggui.AlignStretch), nil, nil
		if shape.carded {
			// A title alone is centered on the control; with text under
			// it the two are a FieldContent and start at the top.
			content := parts[0]
			if len(parts) > 1 {
				content = FieldContent(parts...)
			}
			f.card = FieldCard(f.input, content)
			if shape.controlFirst {
				f.card.ControlFirst()
			}
		} else if shape.horizontal {
			// Beside one line of label the control is centered; beside a
			// label with text under it, it starts at the top.
			align := pick(shape.noted || shape.alerted, ggui.AlignStart, ggui.AlignCenter)
			if shape.controlFirst {
				f.row = ggui.Row(f.input, ggui.Expanded(f.column)).Align(align)
			} else {
				f.row = ggui.Row(ggui.Expanded(f.column), f.input).Align(align)
			}
		}
	}
	if len(errors) > 0 {
		env = env.With(fieldInvalid, true)
	}
	if f.card != nil {
		return f.card.Layout(c, env)
	}
	if f.row != nil {
		// shadcn's FieldContent: the label and its text a hair apart.
		f.column.Gap(t.Space / 4)
		return f.row.Gap(t.Space).Layout(c, env)
	}
	return f.column.Gap(t.Space).Layout(c, env)
}

// Paint implements Widget.
func (f *FieldWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if f.card != nil {
		dst.Paint(f.card, r)
		return
	}
	if f.row != nil {
		dst.Paint(f.row, r)
		return
	}
	dst.Paint(f.column, r)
}

// FieldGroupWidget stacks fields with the room a form gives them. Build one
// with FieldGroup.
type FieldGroupWidget struct {
	column *ggui.ColumnWidget
}

// FieldGroup stacks fields, and anything else a form holds between them
// such as a FieldSeparator or a row of buttons, in a column as wide as it
// is given with two and a half of the theme's Space between them: shadcn's
// FieldGroup.
//
//	ui.FieldGroup(
//		ui.Field("Name", ui.TextField(name)),
//		ui.Field("Email", ui.TextField(email)).Help("We never share it"),
//	)
func FieldGroup(children ...ggui.Widget) *FieldGroupWidget {
	return &FieldGroupWidget{column: ggui.Column(children...).Align(ggui.AlignStretch)}
}

// Layout implements Widget.
func (g *FieldGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return g.column.Gap(uitheme.Of(env).Space*2.5).Layout(c, env)
}

// Paint implements Widget.
func (g *FieldGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(g.column, r) }

// FieldSetWidget groups related fields under a legend. Build one with
// FieldSet.
type FieldSetWidget struct {
	props       property.Owner
	children    []ggui.Widget
	legend      string
	description string

	title, note *ggui.TextWidget
	body, outer *ggui.ColumnWidget
	shape       fieldShape // what body and outer were built for
}

// FieldSet groups related fields, two of the theme's Space apart: shadcn's
// FieldSet. Legend heads it and Description explains it; assistive
// technology reads the set as a group named by its legend.
//
//	ui.FieldSet(
//		ui.Field("Street", ui.TextField(street)),
//		ui.Field("City", ui.TextField(city)),
//	).Legend("Address").Description("Where we deliver.")
func FieldSet(children ...ggui.Widget) *FieldSetWidget {
	return &FieldSetWidget{children: children, title: ggui.Text(""), note: ggui.Text("")}
}

// Legend sets the heading of the set, in medium text a step larger than
// the body's.
func (s *FieldSetWidget) Legend(text string) *FieldSetWidget {
	defer property.Watch(&s.props, &s.legend)()
	s.legend = text
	return s
}

// Description sets the muted text under the legend.
func (s *FieldSetWidget) Description(text string) *FieldSetWidget {
	defer property.Watch(&s.props, &s.description)()
	s.description = text
	return s
}

// Layout implements Widget.
func (s *FieldSetWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	t := uitheme.Of(env)
	// shadcn's text-base font-medium legend: the body text, medium, a
	// quarter of a Space larger.
	legend := t.Text.Merge(t.Label)
	legend.Size = t.Text.Size + t.Space/4
	s.title.Content(s.legend).Style(legend).Color(t.Fg)
	s.note.Content(s.description).Style(t.Text).Color(t.MutedFg)
	if shape := (fieldShape{noted: s.description != "", alerted: s.legend != ""}); s.outer == nil || shape != s.shape {
		s.shape = shape
		parts := make([]ggui.Widget, 0, len(s.children)+1)
		if shape.noted {
			parts = append(parts, s.note)
		}
		s.body = ggui.Column(append(parts, s.children...)...).Align(ggui.AlignStretch)
		if shape.alerted {
			s.outer = ggui.Column(s.title, s.body).Align(ggui.AlignStretch)
		} else {
			s.outer = ggui.Column(s.body).Align(ggui.AlignStretch)
		}
	}
	s.body.Gap(t.Space * 2)
	// The legend sits three quarters of a Space above the fields, and
	// right on a description, which is its own.
	s.outer.Gap(pick(s.shape.noted, 0, t.Space*.75))
	return s.outer.Layout(c, env)
}

// Paint implements Widget.
func (s *FieldSetWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: s.legend}, func(dst *ggui.Canvas) { dst.Paint(s.outer, r) })
}

// FieldSeparatorWidget is a rule between the parts of a form, with an
// optional word in the middle of it. Build one with FieldSeparator.
type FieldSeparatorWidget struct {
	props   property.Owner
	content string
	text    *ggui.TextWidget
	size    ggui.Size // the text's
	theme   *uitheme.Theme
}

// FieldSeparator is a hairline across a FieldGroup, in the theme's Border
// color: shadcn's FieldSeparator. Label sets a word in the middle of it.
func FieldSeparator() *FieldSeparatorWidget { return &FieldSeparatorWidget{text: ggui.Text("")} }

// Label breaks the line around muted text, such as "Or continue with".
func (s *FieldSeparatorWidget) Label(text string) *FieldSeparatorWidget {
	defer property.Watch(&s.props, &s.content)()
	s.content = text
	return s
}

// Layout implements Widget. The separator takes half a Space of height,
// what is left of shadcn's 20px row after its negative margins, and its
// text is centered on that, reaching into the gaps around it.
func (s *FieldSeparatorWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	t := uitheme.Of(env)
	s.theme = t
	s.size = ggui.Size{}
	if s.content != "" {
		s.text.Content(s.content).Style(t.Text).Color(t.MutedFg).NoWrap()
		s.size = s.text.Layout(ggui.Loose(ggui.Sz(max(c.MaxW-4*t.Space, 0), c.MaxH)), env)
	}
	return c.Constrain(ggui.Sz(bounded(c.MaxW, s.size.W+4*t.Space), t.Space/2))
}

// Paint implements Widget.
func (s *FieldSeparatorWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	line := func(x, w float64) {
		if w > 0 {
			dst.FillRect(ggui.Rct(ggui.Pt(x, r.Origin.Y+(r.Size.H-t.BorderWidth)/2), ggui.Sz(w, t.BorderWidth)), t.Border)
		}
	}
	if s.content == "" {
		line(r.Origin.X, r.Size.W)
		return
	}
	// The line stops a Space short of the text on either side, rather
	// than running under a patch of the page's background.
	at := ggui.Rct(ggui.Pt(r.Origin.X+(r.Size.W-s.size.W)/2, r.Origin.Y+(r.Size.H-s.size.H)/2), s.size)
	line(r.Origin.X, at.Origin.X-t.Space-r.Origin.X)
	line(at.Origin.X+at.Size.W+t.Space, r.Origin.X+r.Size.W-at.Origin.X-at.Size.W-t.Space)
	dst.Paint(s.text, at)
}
