package ui

import (
	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// EmptyWidget presents an empty collection with optional media and an action.
type EmptyWidget struct {
	title, description *ggui.TextWidget
	media, action      ggui.Widget
	box                *ggui.BoxWidget
	stale              bool // Media or Action changed what box should hold
	row                *ggui.RowWidget
	column             *ggui.ColumnWidget
}

// Empty creates a centered empty-state panel.
func Empty(title, description string) *EmptyWidget {
	return &EmptyWidget{title: ggui.Text(title).Align(.5), description: ggui.Text(description).Align(.5)}
}

// Media places an illustration, icon or other widget above the title.
func (e *EmptyWidget) Media(w ggui.Widget) *EmptyWidget {
	e.media, e.stale = w, true // the next layout builds the box around it
	return e
}

// Action places a widget below the description.
func (e *EmptyWidget) Action(w ggui.Widget) *EmptyWidget {
	e.action, e.stale = w, true // the next layout builds the box around it
	return e
}

// Layout implements ggui.Widget.
func (e *EmptyWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.From(env)
	e.title.Style(t.Text).Color(t.Fg)
	e.description.Style(t.Caption).Color(t.MutedFg)
	// Built once, and again only after Media or Action changes what it
	// holds, so the panel's layout caches survive a relayout.
	if e.box == nil || e.stale {
		parts := []ggui.Widget{}
		if e.media != nil {
			parts = append(parts, e.media)
		}
		parts = append(parts, e.title, e.description)
		if e.action != nil {
			parts = append(parts, e.action)
		}
		e.column = ggui.Column(parts...).Align(ggui.AlignCenter)
		e.row = ggui.Row(e.column).Justify(ggui.JustifyCenter)
		e.box = ggui.Box(e.row)
		e.stale = false
	}
	e.column.Gap(t.Space)
	e.box.Padding(t.CardPad).Border(t.BorderWidth, t.Border).Radius(t.Radius)
	return e.box.Layout(c, env)
}

// Paint implements ggui.Widget.
func (e *EmptyWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(e.box, r) }
