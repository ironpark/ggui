package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// inFieldCard tells a control it is painted in a FieldCard, which shows
// the keyboard's focus around itself: the control then draws no ring of
// its own.
var inFieldCard = ggui.NewEnvKey[bool]("field card")

// fieldDisabled tells a FieldTitle that the control it describes is
// disabled.
var fieldDisabled = ggui.NewEnvKey[bool]("field disabled")

// FieldCardWidget is a choice card: a bordered label around a checkbox, a
// radio or a switch and what describes it. Build one with FieldCard.
type FieldCardWidget struct {
	ggui.Interactive
	props        property.Owner
	control      ggui.Widget
	content      ggui.Widget
	controlFirst bool
	pending      bool // a click that has yet to move focus to the control

	row    *ggui.RowWidget
	first  bool // what row was built for
	pad    ggui.EdgeInsets
	theme  *uitheme.Theme
	size   ggui.Size // the row's
	rowEnv ggui.Env
}

// FieldCard puts control and the content that describes it in a card that
// is the control's label: shadcn's FieldLabel around a Field. A click
// anywhere in it acts on the control, the card is tinted while the control
// is checked and under the pointer, and it carries the keyboard's focus
// ring in place of the control. The content comes first and the control
// sits at the end; ControlFirst swaps them.
//
//	ui.FieldCard(ui.Radio(plan, "pro", "").Name("Pro"), ui.FieldContent(
//		ui.FieldTitle("Pro"),
//		ui.FieldDescription("For growing businesses."),
//	))
//
// A control without a name takes the content's FieldTitle for one.
// Field(...).Card() builds the same from a label and a description.
func FieldCard(control, content ggui.Widget) *FieldCardWidget {
	c := &FieldCardWidget{control: control, content: content}
	c.AutoKey()
	if n, ok := control.(Named); ok && !n.HasName() {
		if title := fieldTitleOf(content); title != "" {
			n.SetName(title)
		}
	}
	return c
}

// fieldTitleOf is the text of the FieldTitle that w is or holds first.
func fieldTitleOf(w ggui.Widget) string {
	switch w := w.(type) {
	case *FieldTitleWidget:
		return w.content
	case *FieldContentWidget:
		for _, child := range w.children {
			if title := fieldTitleOf(child); title != "" {
				return title
			}
		}
	}
	return ""
}

// ControlFirst puts the control before the content, as a checkbox sits
// beside what it chooses.
func (c *FieldCardWidget) ControlFirst() *FieldCardWidget {
	defer property.Watch(&c.props, &c.controlFirst)()
	c.controlFirst = true
	return c
}

// Key gives the card an identity across rebuilds.
func (c *FieldCardWidget) Key(key any) *FieldCardWidget { c.SetKey(key); return c }

// disabled reports whether the card's control takes no input.
func (c *FieldCardWidget) disabled() bool {
	n, ok := c.control.(interface{ IsInert() bool })
	return ok && n.IsInert()
}

// checked reports whether the card's control is on.
func (c *FieldCardWidget) checked() bool {
	d, ok := c.control.(ggui.Describer)
	if !ok {
		return false
	}
	n := d.Describe()
	return n.Checked == ggui.Tri(true) || n.Checked == ggui.TriMixed || n.Selected
}

// Layout implements ggui.Widget.
func (c *FieldCardWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	t := uitheme.Of(env)
	c.theme = t
	if c.row == nil || c.first != c.controlFirst {
		c.first = c.controlFirst
		// Beside a FieldContent the control starts at the top, a pixel
		// down, as shadcn's items-start and mt-px; beside anything else it
		// is centered.
		align, control := ggui.AlignCenter, c.control
		if _, ok := c.content.(*FieldContentWidget); ok {
			align, control = ggui.AlignStart, ggui.Padding(c.control, 1, 0, 0, 0)
		}
		if c.controlFirst {
			c.row = ggui.Row(control, ggui.Expanded(c.content)).Align(align)
		} else {
			c.row = ggui.Row(ggui.Expanded(c.content), control).Align(align)
		}
	}
	// shadcn's p-2.5 inside the label's one-pixel border.
	c.pad = ggui.Insets(t.Space*1.25 + t.BorderWidth)
	env = env.With(inFieldCard, true)
	if c.disabled() {
		env = env.With(fieldDisabled, true)
	}
	c.rowEnv = env
	c.size = c.row.Gap(t.Space).Layout(c.pad.Shrink(cs), env)
	return cs.Constrain(c.pad.Inflate(c.size))
}

// Paint implements ggui.Widget.
func (c *FieldCardWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	inert := c.disabled()
	dark := isDark(t.Bg)
	hovered := c.Hovered
	if h, ok := c.control.(interface{ hovered() bool }); ok && h.hovered() {
		hovered = true
	}
	focused := !inert && dst.FocusVisibleWithin(r)
	// The states have no transition in shadcn's FieldLabel: they switch.
	var fill color.Color
	border := t.Border
	if c.checked() {
		fill, border = fade(t.Primary, pick(dark, .1, .05)), fade(t.Primary, pick(dark, .2, .3))
	}
	if hovered && !inert {
		fill = fade(t.Muted, .5)
	}
	if focused {
		border = t.Ring
		haloStroke(dst, r, t.Radius, t.Halo(t.Ring))
	}
	if fill != nil {
		dst.FillRoundRect(r, t.Radius, fill)
	}
	dst.StrokeRoundRect(r, t.Radius, t.BorderWidth, border)
	// The card is under what it holds: the control keeps its own region,
	// and a link in the description its own.
	if !inert {
		dst.HitPointer(r, c)
		dst.HitCursor(r, ggui.CursorShapeDefault)
	}
	dst.Paint(c.row, ggui.Rct(r.Origin.Add(ggui.Pt(c.pad.Left, c.pad.Top)), ggui.Sz(max(r.Size.W-c.pad.Horizontal(), 0), max(r.Size.H-c.pad.Vertical(), 0))))
	if c.pending {
		c.pending = false
		if h := labelFocusTarget(c.control); h != nil && !inert {
			dst.RequestFocus(h)
		}
	}
}

// HandlePointer implements ggui.PointerHandler: a click on the card is a
// click on its control.
func (c *FieldCardWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if c.disabled() {
		return false
	}
	return c.Pointer(ev, func() {
		c.pending = true
		pressLabelTarget(c.control)
	})
}

// FieldContentWidget stacks what describes a control. Build one with
// FieldContent.
type FieldContentWidget struct {
	children []ggui.Widget
	column   *ggui.ColumnWidget
}

// FieldContent stacks a FieldTitle, a FieldDescription and anything else
// that describes a control a quarter of the theme's Space apart: shadcn's
// FieldContent, the text beside the control of a FieldCard.
func FieldContent(children ...ggui.Widget) *FieldContentWidget {
	return &FieldContentWidget{children: children, column: ggui.Column(children...).Align(ggui.AlignStretch)}
}

// Layout implements ggui.Widget.
func (f *FieldContentWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return f.column.Gap(uitheme.Of(env).Space/4).Layout(c, env)
}

// Paint implements ggui.Widget.
func (f *FieldContentWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(f.column, r) }

// fieldSnug and fieldNormal are the line heights of a field's title and
// description: Tailwind's leading-snug and leading-normal.
const (
	fieldSnug   = 1.375
	fieldNormal = 1.5
)

// FieldTitleWidget is the title of what a control chooses. Build one with
// FieldTitle.
type FieldTitleWidget struct {
	content string
	text    *ggui.TextWidget
}

// FieldTitle is medium body text on a snug line: shadcn's FieldTitle, the
// first line of a FieldContent. It dims with a disabled control and turns
// destructive in an invalid Field.
func FieldTitle(text string) *FieldTitleWidget {
	return &FieldTitleWidget{content: text, text: ggui.Text(text)}
}

// Layout implements ggui.Widget.
func (f *FieldTitleWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	style := t.Text.Merge(t.Label)
	style.LineHeight = fieldSnug
	col := t.Fg
	if invalid, _ := env.Get(fieldInvalid); invalid {
		col = t.Destructive
	}
	if off, _ := env.Get(fieldDisabled); off {
		col = t.Disabled(col)
	}
	return f.text.Style(style).Color(col).Layout(c, env)
}

// Paint implements ggui.Widget.
func (f *FieldTitleWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(f.text, r) }

// Baseline implements ggui.Baseliner.
func (f *FieldTitleWidget) Baseline() (float64, bool) { return f.text.Baseline() }

// FieldDescriptionWidget is the muted text that explains a field. Build
// one with FieldDescription.
type FieldDescriptionWidget struct {
	text *ggui.TextWidget
}

// FieldDescription is muted body text: shadcn's FieldDescription, under
// the FieldTitle of a FieldContent. Field's Help sets the same under a
// label.
func FieldDescription(text string) *FieldDescriptionWidget {
	return &FieldDescriptionWidget{text: ggui.Text(text)}
}

// Layout implements ggui.Widget.
func (f *FieldDescriptionWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	style := t.Text
	style.LineHeight = fieldNormal
	return f.text.Style(style).Color(t.MutedFg).Layout(c, env)
}

// Paint implements ggui.Widget.
func (f *FieldDescriptionWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(f.text, r) }

// fieldErrorList is the errors of a Field that holds more than one: a
// bulleted list, each message half a Space under the last.
type fieldErrorList struct {
	messages []string
	rows     []*ggui.TextWidget
	sizes    []ggui.Size
	theme    *uitheme.Theme
}

// set replaces the messages, keeping the rows of the ones that stay.
func (l *fieldErrorList) set(messages []string) {
	l.messages = messages
	for len(l.rows) < len(messages) {
		l.rows = append(l.rows, ggui.Text(""))
	}
	l.rows = l.rows[:len(messages)]
	l.sizes = make([]ggui.Size, len(messages))
}

// Layout implements ggui.Widget: shadcn's ml-4 list-disc gap-1.
func (l *fieldErrorList) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	l.theme = t
	indent, gap := 2*t.Space, t.Space/2
	size := ggui.Size{}
	for i, row := range l.rows {
		row.Content(l.messages[i]).Style(t.Text).Color(t.Destructive)
		l.sizes[i] = row.Layout(ggui.Loose(ggui.Sz(max(c.MaxW-indent, 0), ggui.Unbounded)), env)
		size.W = max(size.W, indent+l.sizes[i].W)
		size.H += l.sizes[i].H
		if i > 0 {
			size.H += gap
		}
	}
	return c.Constrain(size)
}

// Paint implements ggui.Widget.
func (l *fieldErrorList) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := l.theme
	indent, gap := 2*t.Space, t.Space/2
	y := r.Origin.Y
	for i, row := range l.rows {
		// The disc sits in the indent, on the middle of the first line.
		line := t.Text.Size * t.Text.LineHeight
		if line <= 0 {
			line = l.sizes[i].H
		}
		dst.FillCircle(ggui.Pt(r.Origin.X+indent-t.Space*.875, y+line/2), t.Text.Size*.18, t.Destructive)
		dst.Paint(row, ggui.Rct(ggui.Pt(r.Origin.X+indent, y), l.sizes[i]))
		y += l.sizes[i].H + gap
	}
}
