package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// emptyMeasure is the widest the text and the action of an empty state
// run, shadcn's max-w-sm.
const emptyMeasure = 384.0

// EmptyWidget presents an empty collection with optional media and an action.
type EmptyWidget struct {
	props              property.Owner
	title, description *ggui.TextWidget
	media, action      ggui.Widget
	outline            bool
	theme              *uitheme.Theme
	box                *ggui.BoxWidget
	stale              bool // Media or Action changed what box should hold
	row                *ggui.RowWidget
	column             *ggui.ColumnWidget // the header over the action
	header             *ggui.ColumnWidget // the title over the description
	lead               *ggui.BoxWidget    // the media, with the space under it
	text, content      *ggui.BoxWidget    // the header and the action, held to the measure
}

// emptyTitleTracking is the letter spacing of an Empty's title: shadcn's
// tracking-tight.
const emptyTitleTracking = -0.025

// Empty creates a centered empty-state panel. An empty description is
// left out.
func Empty(title, description string) *EmptyWidget {
	e := &EmptyWidget{title: ggui.Text(title).Align(.5).Tracking(emptyTitleTracking)}
	if description != "" {
		e.description = ggui.Text(description).Align(.5)
	}
	return e
}

// Media places an illustration, icon or other widget above the title.
func (e *EmptyWidget) Media(w ggui.Widget) *EmptyWidget {
	e.media, e.stale = w, true // the next layout builds the box around it
	return e
}

// MediaIcon places the icon of role above the title, on a small rounded
// tile of the theme's Muted surface.
func (e *EmptyWidget) MediaIcon(role icons.Role) *EmptyWidget {
	return e.Media(&emptyIcon{icon: Icon(role)})
}

// Action places a widget below the description.
func (e *EmptyWidget) Action(w ggui.Widget) *EmptyWidget {
	e.action, e.stale = w, true // the next layout builds the box around it
	return e
}

// Outline draws a dashed border around the panel, which has none of its
// own.
func (e *EmptyWidget) Outline() *EmptyWidget {
	defer property.Watch(&e.props, &e.outline)()
	e.outline = true
	return e
}

// Layout implements ggui.Widget.
func (e *EmptyWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer e.props.Layout().EndLayout()
	t := uitheme.Of(env)
	e.theme = t
	e.title.Style(t.Text.Merge(t.Label)).Color(t.Fg)
	if e.description != nil {
		relaxed := t.Text
		relaxed.LineHeight, relaxed.Color = 1.625, t.MutedFg
		e.description.Style(relaxed)
	}
	// Built once, and again only after Media or Action changes what it
	// holds, so the panel's layout caches survive a relayout.
	if e.box == nil || e.stale {
		e.lead = nil
		head := []ggui.Widget{}
		if e.media != nil {
			e.lead = ggui.Box(e.media)
			head = append(head, e.lead)
		}
		head = append(head, e.title)
		if e.description != nil {
			head = append(head, e.description)
		}
		e.header = ggui.Column(head...).Align(ggui.AlignCenter)
		e.text = ggui.Box(e.header)
		parts := []ggui.Widget{e.text}
		e.content = nil
		if e.action != nil {
			e.content = ggui.Box(e.action)
			parts = append(parts, e.content)
		}
		e.column = ggui.Column(parts...).Align(ggui.AlignCenter)
		e.row = ggui.Row(e.column).Justify(ggui.JustifyCenter)
		e.box = ggui.Box(e.row)
		e.stale = false
	}
	if e.lead != nil {
		e.lead.Pad(0, 0, t.Space, 0)
	}
	e.header.Gap(t.Space)
	e.text.MaxWidth(emptyMeasure)
	if e.content != nil {
		e.content.MaxWidth(emptyMeasure)
	}
	e.column.Gap(t.Space * 2)
	e.box.Pad(t.Space * 3)
	return e.box.Layout(c, env)
}

// Paint implements ggui.Widget.
func (e *EmptyWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if e.outline {
		attachmentDashedBorder(dst, r, e.theme.RadiusLg, e.theme.BorderWidth, e.theme.Border)
	}
	dst.Paint(e.box, r)
}

// emptyIcon is an empty state's media drawn as an icon on a muted tile.
type emptyIcon struct {
	icon  *icons.Widget
	theme *uitheme.Theme
	size  ggui.Size
}

func (m *emptyIcon) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	m.theme = t
	m.icon.Size(t.IconSize).Color(t.Fg)
	m.size = m.icon.Layout(ggui.Loose(ggui.Sz(t.Space*4, t.Space*4)), env)
	return c.Constrain(ggui.Sz(t.Space*4, t.Space*4))
}

func (m *emptyIcon) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.FillRoundRect(r, m.theme.Radius, m.theme.Muted)
	dst.Paint(m.icon, ggui.Rct(r.Origin.Add(ggui.Pt((r.Size.W-m.size.W)/2, (r.Size.H-m.size.H)/2)), m.size))
}
