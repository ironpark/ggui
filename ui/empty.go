package ui

import "github.com/ironpark/ggui"

// EmptyWidget presents an empty collection with optional media and an action.
type EmptyWidget struct {
	title, description *ggui.TextWidget
	media, action      ggui.Widget
	box                *ggui.BoxWidget
}

// Empty creates a centered empty-state panel.
func Empty(title, description string) *EmptyWidget {
	return &EmptyWidget{title: ggui.Text(title).Align(.5), description: ggui.Text(description).Align(.5)}
}

// Media places an illustration, icon or other widget above the title.
func (e *EmptyWidget) Media(w ggui.Widget) *EmptyWidget { e.media = w; return e }

// Action places a widget below the description.
func (e *EmptyWidget) Action(w ggui.Widget) *EmptyWidget { e.action = w; return e }

// Layout implements ggui.Widget.
func (e *EmptyWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	e.title.Style(t.Text).Color(t.Fg)
	e.description.Style(t.Caption).Color(t.Muted)
	parts := []ggui.Widget{}
	if e.media != nil {
		parts = append(parts, e.media)
	}
	parts = append(parts, e.title, e.description)
	if e.action != nil {
		parts = append(parts, e.action)
	}
	e.box = ggui.Box(ggui.Row(ggui.Column(parts...).Gap(t.Space).Align(ggui.AlignCenter)).Justify(ggui.JustifyCenter)).Padding(t.CardPad).Border(1, t.Border).Radius(t.Radius)
	return e.box.Layout(c, env)
}

// Paint implements ggui.Widget.
func (e *EmptyWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(e.box, r) }
