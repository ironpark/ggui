package ui

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// itemSize is one of an item's three densities.
type itemSize uint8

const (
	itemDefault itemSize = iota
	itemSmall
	itemExtraSmall
)

// itemLook is how an item's surface is drawn.
type itemLook uint8

const (
	itemBare itemLook = iota
	itemOutline
	itemMuted
)

// ItemWidget is one row of a list: media on the left, a title and an
// optional description in the middle, and an action on the right. Build one
// with Item. It is the composition helper a settings list, a file row or a
// chosen-option summary is made of; it takes no input of its own, so the
// widget given to Action keeps its own keyboard and pointer behavior,
// unless OnTap makes the whole row a link.
type ItemWidget struct {
	ggui.Interactive
	props              property.Owner
	onTap              func()
	motion             time.Duration
	hover              float64 // how far onto the surface under the pointer the row was painted
	title, description *ggui.TextWidget
	media, action      ggui.Widget
	header, footer     ggui.Widget
	look               itemLook
	size               itemSize
	theme              *uitheme.Theme

	// Where the parts sit inside the row, from the last Layout.
	mediaAt, textAt, actionAt, headerAt, footerAt ggui.Rect
	titleSize, descriptionSize                    ggui.Size
	textGap                                       float64
}

// Item creates a row titled title. An empty description is left out.
//
//	ui.Item("Backups", "Last run 2 hours ago").
//		Media(ui.Avatar("Ada")).
//		Action(ui.Button("Run", run).Outline())
func Item(title, description string) *ItemWidget {
	i := &ItemWidget{title: ggui.Text(title).MaxLines(1)}
	i.Role = ggui.RoleLink
	i.SetName(title)
	i.AutoKey()
	if description != "" {
		i.description = ggui.Text(description).MaxLines(2)
	}
	return i
}

// Media places a widget before the text: an avatar, an icon, a thumbnail.
// Beside a description it sits at the top, level with the title.
func (i *ItemWidget) Media(w ggui.Widget) *ItemWidget {
	i.media = w
	i.props.Changed()
	return i
}

// MediaIcon places the icon of role before the text, at the theme's icon
// size.
func (i *ItemWidget) MediaIcon(role icons.Role) *ItemWidget {
	return i.Media(&itemIcon{icon: Icon(role)})
}

// Action places a widget after the text, pushed to the right edge.
func (i *ItemWidget) Action(w ggui.Widget) *ItemWidget {
	i.action = w
	i.props.Changed()
	return i
}

// Header places a widget across the row, above the media and the text.
func (i *ItemWidget) Header(w ggui.Widget) *ItemWidget {
	i.header = w
	i.props.Changed()
	return i
}

// Footer places a widget across the row, below the media and the text.
func (i *ItemWidget) Footer(w ggui.Widget) *ItemWidget {
	i.footer = w
	i.props.Changed()
	return i
}

// OnTap makes the row a link that runs fn on a click, or on Space or Enter
// while it has the keyboard focus: the row takes the Muted surface under
// the pointer, shows the pointer cursor and a focus ring, and is a link
// named by its title. What Action placed in the row still takes the input
// over itself. A nil fn makes the row plain again.
func (i *ItemWidget) OnTap(fn func()) *ItemWidget {
	defer property.Watch(&i.props, &i.onTap)()
	i.onTap = fn
	return i
}

// Outline draws a border around the row.
func (i *ItemWidget) Outline() *ItemWidget {
	defer property.Watch(&i.props, &i.look)()
	i.look = itemOutline
	return i
}

// Muted draws the row on a faint surface.
func (i *ItemWidget) Muted() *ItemWidget {
	defer property.Watch(&i.props, &i.look)()
	i.look = itemMuted
	return i
}

// Small marks the row as one of a denser list: an ItemGroup of small items
// sets them closer together.
func (i *ItemWidget) Small() *ItemWidget {
	defer property.Watch(&i.props, &i.size)()
	i.size = itemSmall
	return i
}

// ExtraSmall tightens the row: less padding, a smaller description, and no
// gap between it and the title.
func (i *ItemWidget) ExtraSmall() *ItemWidget {
	defer property.Watch(&i.props, &i.size)()
	i.size = itemExtraSmall
	return i
}

// Layout implements ggui.Widget.
func (i *ItemWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer i.props.Layout().EndLayout()
	t := uitheme.Of(env)
	i.theme = t
	// shadcn's transition-colors duration-100, two thirds of the 150ms
	// the theme's MotionFast stands for.
	i.motion = env.Motion(t.MotionFast * 2 / 3)
	// The padding counts the border every item keeps room for, drawn or
	// not, so the three looks line up in one list.
	pad, gap := ggui.Insets(t.Space*1.25+t.BorderWidth, t.Space*1.5+t.BorderWidth), t.Space*1.25
	i.textGap = t.Space / 2
	description := t.Text
	if i.size == itemExtraSmall {
		pad, gap = ggui.Insets(t.Space+t.BorderWidth, t.Space*1.25+t.BorderWidth), t.Space
		i.textGap = 0
		description = t.Text.Merge(t.Caption)
	}
	i.title.Style(t.Text.Merge(t.Label)).Color(t.Fg)
	if i.description != nil {
		description.Color = t.MutedFg
		i.description.Style(description)
	}

	inner := pad.Shrink(c).Loosen()
	bounded := !math.IsInf(inner.MaxW, 1)
	loose := ggui.Loose(ggui.Sz(inner.MaxW, inner.MaxH))
	var mediaSize, actionSize ggui.Size
	left := inner.MaxW
	if i.media != nil {
		mediaSize = i.media.Layout(loose, env)
		left -= mediaSize.W + gap
	}
	if i.action != nil {
		actionSize = i.action.Layout(ggui.Loose(ggui.Sz(max(0, left), inner.MaxH)), env)
		left -= actionSize.W + gap
	}
	left = max(0, left)
	text := ggui.Loose(ggui.Sz(left, inner.MaxH))
	i.titleSize = i.title.Layout(text, env)
	textW, textH := i.titleSize.W, i.titleSize.H
	if i.description != nil {
		i.descriptionSize = i.description.Layout(text, env)
		textW, textH = max(textW, i.descriptionSize.W), textH+i.textGap+i.descriptionSize.H
	}
	if bounded {
		textW = left // the text takes what the media and the action leave
	}
	rowH := max(mediaSize.H, textH, actionSize.H)
	width := textW
	if i.media != nil {
		width += mediaSize.W + gap
	}
	if i.action != nil {
		width += actionSize.W + gap
	}

	y := pad.Top
	wide := ggui.Constraints{MinW: width, MaxW: width, MaxH: inner.MaxH}
	i.headerAt, i.footerAt = ggui.Rect{}, ggui.Rect{}
	if i.header != nil {
		i.headerAt = ggui.Rct(ggui.Pt(pad.Left, y), i.header.Layout(wide, env))
		y += i.headerAt.Size.H + gap
	}
	x := pad.Left
	if i.media != nil {
		// Beside a description the media stands at the top, nudged down
		// to the title's line; alone with a title it is centered on it.
		at := y + (rowH-mediaSize.H)/2
		if i.description != nil {
			at = y + min(t.Space/4, rowH-mediaSize.H)
		}
		i.mediaAt = ggui.Rct(ggui.Pt(x, at), mediaSize)
		x += mediaSize.W + gap
	}
	i.textAt = ggui.Rct(ggui.Pt(x, y+(rowH-textH)/2), ggui.Sz(textW, textH))
	if i.action != nil {
		i.actionAt = ggui.Rct(ggui.Pt(x+textW+gap, y+(rowH-actionSize.H)/2), actionSize)
	}
	y += rowH
	if i.footer != nil {
		y += gap
		i.footerAt = ggui.Rct(ggui.Pt(pad.Left, y), i.footer.Layout(wide, env))
		y += i.footerAt.Size.H
	}
	return c.Constrain(ggui.Sz(width+pad.Left+pad.Right, y+pad.Bottom))
}

// Paint implements ggui.Widget. A row groups what is inside it, so a screen
// reader reads the title, the description and the action as one item.
func (i *ItemWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := i.theme
	i.hover = 0
	if i.onTap != nil {
		// shadcn's [a]:hover:bg-muted, whatever the row's own surface.
		i.hover = ease(dst, t, i.Anchor(r), itemHoverSlot, pick(i.Hovered && !i.IsInert(), 1.0, 0.0), i.motion)
	}
	var fill color.Color
	if i.look == itemMuted {
		fill = fade(t.Muted, .5)
	}
	if i.hover > 0 {
		fill = pick(fill == nil, fade(t.Muted, i.hover), mix(colorOr(fill, t.Muted), t.Muted, i.hover))
	}
	if fill != nil {
		dst.FillRoundRect(r, t.Radius, fill)
	}
	if i.look == itemOutline {
		dst.StrokeRoundRect(r, t.Radius, t.BorderWidth, t.Border)
	}
	at := func(part ggui.Rect) ggui.Rect { return ggui.Rct(r.Origin.Add(part.Origin), part.Size) }
	dst.Node(r, ggui.Node{Role: ggui.RoleListItem}, func(dst *ggui.Canvas) {
		if i.onTap != nil {
			// Registered before the parts, which are painted over it and
			// so keep their own input.
			i.Hit(dst, r, i, ggui.CursorShapePointer)
		}
		if i.header != nil {
			dst.Paint(i.header, at(i.headerAt))
		}
		if i.media != nil {
			dst.Paint(i.media, at(i.mediaAt))
		}
		text := at(i.textAt)
		dst.Paint(i.title, ggui.Rct(text.Origin, i.titleSize))
		if i.description != nil {
			dst.Paint(i.description, ggui.Rct(text.Origin.Add(ggui.Pt(0, i.titleSize.H+i.textGap)), i.descriptionSize))
		}
		if i.action != nil {
			dst.Paint(i.action, at(i.actionAt))
		}
		if i.footer != nil {
			dst.Paint(i.footer, at(i.footerAt))
		}
	})
	if i.onTap != nil {
		// The ring is a shadow, which transition-colors does not ease.
		focusRing(dst, &i.Interactive, t, r, t.Radius, t.Ring, 0)
	}
}

// itemHoverSlot keeps a tappable item's easing onto its hover surface.
var itemHoverSlot = ggui.NewSlot[*ggui.Motion]("item hover")

// HandlePointer implements ggui.PointerHandler, for an item made a link.
func (i *ItemWidget) HandlePointer(ev ggui.PointerEvent) bool { return i.Pointer(ev, i.onTap) }

// HandleKey implements ggui.KeyHandler: Space or Enter runs the action.
func (i *ItemWidget) HandleKey(ev ggui.KeyEvent) { i.Keyboard(ev, i.onTap) }

// Describe implements ggui.Describer: an item with an action is a link
// named by its title.
func (i *ItemWidget) Describe() ggui.Node {
	return ggui.Node{Role: i.Role, Name: i.SemanticName(), Actions: ggui.ActionFocus | ggui.ActionPress}
}

// Act implements ggui.Actor: a press runs the action.
func (i *ItemWidget) Act(a ggui.Action) bool {
	if a.Kind != ggui.ActionPress || i.onTap == nil {
		return false
	}
	i.onTap()
	return true
}

// itemIcon is an item's media drawn as an icon at the theme's size.
type itemIcon struct{ icon *icons.Widget }

func (m *itemIcon) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	m.icon.Size(uitheme.Of(env).IconSize)
	return m.icon.Layout(c, env)
}

func (m *itemIcon) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(m.icon, r) }

// ItemGroupWidget is a list of items, set apart by a gap that follows
// their size. Build one with ItemGroup.
type ItemGroupWidget struct {
	column *ggui.ColumnWidget
	items  []ggui.Widget
}

// ItemGroup stacks items into a list. An ItemSeparator between two of them
// draws a line there.
//
//	ui.ItemGroup(
//		ui.Item("Ada Lovelace", "Analytical engine").Outline(),
//		ui.Item("Grace Hopper", "Compilers").Outline(),
//	)
func ItemGroup(items ...ggui.Widget) *ItemGroupWidget {
	return &ItemGroupWidget{column: ggui.Column(items...).Align(ggui.AlignStretch), items: items}
}

// Layout implements ggui.Widget.
func (g *ItemGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	// The densest item sets the gap for the list.
	size := itemDefault
	for _, w := range g.items {
		if item, ok := w.(*ItemWidget); ok {
			size = max(size, item.size)
		}
	}
	gap := t.Space * 2
	switch size {
	case itemSmall:
		gap = t.Space * 1.25
	case itemExtraSmall:
		gap = t.Space
	}
	g.column.Gap(gap)
	return g.column.Layout(c, env)
}

// Paint implements ggui.Widget. The group is a list to a screen reader.
func (g *ItemGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleList}, func(dst *ggui.Canvas) { dst.Paint(g.column, r) })
}

// ItemSeparator is a line between two items of an ItemGroup, with a Space
// above and below it beside the group's gap, shadcn's my-2.
func ItemSeparator() *DividerWidget {
	d := Divider()
	d.margin = 1
	return d
}
