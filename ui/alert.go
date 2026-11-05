package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// AlertWidget is an inline notice with a title, description and optional action.
type AlertWidget struct {
	props              property.Owner
	title, description *ggui.TextWidget
	action             ggui.Widget
	destructive        bool
	box                *ggui.BoxWidget
	stale              bool // Action changed what box should hold
	column             *ggui.ColumnWidget
}

// Alert creates an inline notice. It does not interrupt focus or open a modal.
func Alert(title, description string) *AlertWidget {
	return &AlertWidget{title: ggui.Text(title), description: ggui.Text(description)}
}

// Destructive uses the theme's Destructive color for the heading and border.
func (a *AlertWidget) Destructive() *AlertWidget {
	defer property.Watch(&a.props, &a.destructive)()
	a.destructive = true
	return a
}

// Action places a widget below the notice, such as a retry button.
func (a *AlertWidget) Action(w ggui.Widget) *AlertWidget {
	a.action, a.stale = w, true // the next layout builds the box around it
	return a
}

// Layout implements ggui.Widget.
func (a *AlertWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer a.props.Layout().EndLayout()
	t := uitheme.From(env)
	fg, border := t.Fg, t.Border
	if a.destructive {
		fg, border = t.Destructive, mix(t.Border, t.Destructive, .3)
	}
	a.title.Style(t.Text).Color(fg)
	a.description.Style(t.Text).Color(pick(a.destructive, fade(t.Destructive, .9), t.MutedFg))
	// The box and column are built once, and again only after Action
	// changes what they hold, so their layout caches survive a relayout.
	if a.box == nil || a.stale {
		parts := []ggui.Widget{a.title, a.description}
		if a.action != nil {
			parts = append(parts, a.action)
		}
		a.column = ggui.Column(parts...)
		a.box = ggui.Box(a.column)
		a.stale = false
	}
	a.column.Gap(t.Space / 2)
	a.box.Pad(t.Space*2).Fill(t.Card).Border(t.BorderWidth, border).Radius(t.Radius)
	return a.box.Layout(c, env)
}

// Paint implements ggui.Widget.
func (a *AlertWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(a.box, r) }
