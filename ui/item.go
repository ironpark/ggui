package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ItemWidget is one row of a list: media on the left, a title and an
// optional description in the middle, and an action on the right. Build one
// with Item. It is the composition helper a settings list, a file row or a
// chosen-option summary is made of; it takes no input of its own, so the
// widget given to Action keeps its own keyboard and pointer behavior.
type ItemWidget struct {
	props              property.Owner
	title, description *ggui.TextWidget
	media, action      ggui.Widget
	outline            bool
	text               *ggui.ColumnWidget
	row                *ggui.RowWidget
	box                *ggui.BoxWidget
}

// Item creates a row titled title. An empty description is left out.
//
//	ui.Item("Backups", "Last run 2 hours ago").
//		Media(ui.Avatar("Ada")).
//		Action(ui.Button("Run", run).Outline())
func Item(title, description string) *ItemWidget {
	i := &ItemWidget{title: ggui.Text(title).NoWrap()}
	if description != "" {
		i.description = ggui.Text(description)
	}
	return i
}

// Media places a widget before the text: an avatar, an icon, a thumbnail.
func (i *ItemWidget) Media(w ggui.Widget) *ItemWidget { i.media = w; return i }

// Action places a widget after the text, pushed to the right edge.
func (i *ItemWidget) Action(w ggui.Widget) *ItemWidget { i.action = w; return i }

// Outline draws the row as a bordered card rather than bare text.
func (i *ItemWidget) Outline() *ItemWidget {
	defer property.Watch(&i.props, &i.outline)()
	i.outline = true
	return i
}

// build makes the row once. An item is a list row, so a fifty-row list
// would otherwise allocate its whole tree afresh on every frame; the shape
// is fixed at construction, and only the styling follows the theme.
func (i *ItemWidget) build() {
	text := []ggui.Widget{ggui.Widget(i.title)}
	if i.description != nil {
		text = append(text, i.description)
	}
	i.text = ggui.Column(text...).Align(ggui.AlignStretch)
	parts := []ggui.Widget{}
	if i.media != nil {
		parts = append(parts, i.media)
	}
	parts = append(parts, ggui.Expanded(i.text))
	if i.action != nil {
		parts = append(parts, i.action)
	}
	i.row = ggui.Row(parts...)
	i.box = ggui.Box(i.row)
}

// Layout implements ggui.Widget.
func (i *ItemWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer i.props.Layout()()
	if i.box == nil {
		i.build()
	}
	t := uitheme.From(env)
	i.title.Style(t.Text).Color(t.Fg)
	if i.description != nil {
		i.description.Style(t.Caption).Color(t.MutedFg)
	}
	i.text.Gap(t.Space / 4)
	i.row.Gap(t.Space * 1.5)
	i.box.Padding(t.ItemPad)
	if i.outline {
		i.box.Fill(t.Card).Border(t.BorderWidth, t.Border).Radius(t.Radius).Padding(t.CardPad)
	}
	return i.box.Layout(c, env)
}

// Paint implements ggui.Widget. A row groups what is inside it, so a screen
// reader reads the title, the description and the action as one item.
func (i *ItemWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleListItem}, func(dst *ggui.Canvas) { dst.Paint(i.box, r) })
}
