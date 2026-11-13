package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// AlertWidget is an inline notice with a title, description and optional
// icon and action.
type AlertWidget struct {
	props              property.Owner
	title, description *ggui.TextWidget
	action             ggui.Widget
	role               icons.Role // the leading icon; empty is none
	destructive        bool
	box                *ggui.BoxWidget
	stale              bool // Icon or Action changed what box should hold
	row                *ggui.RowWidget
	column             *ggui.ColumnWidget
	icon               *icons.Widget
	iconBox            *ggui.BoxWidget
	actionSize         ggui.Size
	inset              float64 // of the action from the top and right edges
}

// Alert creates an inline notice. It does not interrupt focus or open a
// modal. An empty description is left out.
func Alert(title, description string) *AlertWidget {
	a := &AlertWidget{title: ggui.Text(title)}
	if description != "" {
		a.description = ggui.Text(description)
	}
	return a
}

// Destructive uses the theme's Destructive color for the heading, the
// description and the icon.
func (a *AlertWidget) Destructive() *AlertWidget {
	defer property.Watch(&a.props, &a.destructive)()
	a.destructive = true
	return a
}

// Icon leads the notice with the icon of role, in the color of its title.
// An empty role removes it.
func (a *AlertWidget) Icon(role icons.Role) *AlertWidget {
	if role != a.role {
		a.role, a.stale = role, true // the next layout builds the box around it
	}
	return a
}

// Action places a widget at the notice's top right, such as a retry
// button; the text keeps clear of it.
func (a *AlertWidget) Action(w ggui.Widget) *AlertWidget {
	a.action, a.stale = w, true // the next layout builds the box around it
	return a
}

// Layout implements ggui.Widget.
func (a *AlertWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer a.props.Layout().EndLayout()
	t := uitheme.Of(env)
	fg := t.CardFg
	if a.destructive {
		fg = t.Destructive
	}
	a.title.Style(t.Text.Merge(t.Label)).Color(fg)
	if a.description != nil {
		a.description.Style(t.Text).Color(pick(a.destructive, fade(t.Destructive, .9), t.MutedFg))
	}
	// The box, row and column are built once, and again only after Icon
	// or Action changes what they hold, so their layout caches survive a
	// relayout.
	if a.box == nil || a.stale {
		text := []ggui.Widget{a.title}
		if a.description != nil {
			text = append(text, a.description)
		}
		a.column = ggui.Column(text...).Align(ggui.AlignStretch)
		parts := []ggui.Widget{}
		a.icon, a.iconBox = nil, nil
		if a.role != "" {
			a.icon = Icon(a.role)
			a.iconBox = ggui.Box(a.icon)
			parts = append(parts, a.iconBox)
		}
		a.row = ggui.Row(append(parts, ggui.Expanded(a.column))...).Align(ggui.AlignStart)
		a.box = ggui.Box(a.row)
		a.stale = false
	}
	if a.icon != nil {
		// Level with the title's line rather than its top.
		a.icon.Size(t.IconSize).Color(fg)
		a.iconBox.Pad(t.Space/4, 0, 0, 0)
	}
	a.column.Gap(t.Space / 4)
	a.row.Gap(t.Space)
	pad := ggui.Insets(t.Space, t.Space*1.25)
	a.inset, a.actionSize = t.Space, ggui.Size{}
	if a.action != nil {
		// The action hangs at the top right, and the text stops short
		// of it.
		a.actionSize = a.action.Layout(ggui.Loose(ggui.Sz(max(0, c.MaxW-2*a.inset), max(0, c.MaxH-2*a.inset))), env)
		pad.Right = a.inset + a.actionSize.W + t.Space
	}
	a.box.Padding(pad).Fill(t.Card).Border(t.BorderWidth, t.Border).Radius(t.Radius)
	size := a.box.Layout(c, env)
	if a.action != nil {
		size.H = max(size.H, a.actionSize.H+2*a.inset)
	}
	return c.Constrain(size)
}

// Paint implements ggui.Widget. The notice is a status to a screen reader,
// the nearest there is to shadcn's role="alert", holding its text and its
// action.
func (a *AlertWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleStatus}, func(dst *ggui.Canvas) {
		dst.Paint(a.box, r)
		if a.action != nil {
			dst.Paint(a.action, ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-a.inset-a.actionSize.W, r.Origin.Y+a.inset), a.actionSize))
		}
	})
}
