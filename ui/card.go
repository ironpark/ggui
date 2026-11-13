package ui

import (
	"image/color"
	"math"
	"strconv"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Card is a panel on the theme's Card surface, with a border and rounded
// corners, for grouping content. Around one child it pads every side by
// the theme's CardPad. Given sections, a CardHeader, CardContent or
// CardFooter among several children, it stacks them a CardPad apart, each
// padded at its sides, and a footer in last place runs to the card's
// bottom edge; a child that is not a section is laid out as CardContent.
// A CardMedia runs from side to side, and in first place to the card's
// top edge under its rounded corners.
//
//	ui.Card(
//		ui.CardHeader("Team", "Invite people to the project.").Action(ui.Button("Add", add).Size(ui.ButtonSmall)),
//		ui.CardContent(members),
//		ui.CardFooter(ui.Button("Save", save)),
//	)
//
// Fill, Border, Radius, Pad and Shadow override the theme for one card.
func Card(children ...ggui.Widget) *CardWidget {
	c := &CardWidget{radius: -1, borderWidth: -1, sectioned: len(children) != 1}
	for _, w := range children {
		if _, ok := w.(cardSection); ok {
			c.sectioned = true
		}
	}
	if !c.sectioned {
		c.box = ggui.Box(children[0])
		return c
	}
	parts := make([]ggui.Widget, len(children))
	for i, w := range children {
		if _, ok := w.(cardSection); !ok {
			w = CardContent(w)
		}
		parts[i] = w
	}
	if n := len(parts); n > 0 {
		_, c.footed = parts[n-1].(*CardFooterWidget)
		c.lead, _ = parts[0].(*CardMediaWidget)
		c.trail, _ = parts[n-1].(*CardMediaWidget)
	}
	c.column = ggui.Column(parts...).Align(ggui.AlignStretch)
	c.box = ggui.Box(c.column)
	return c
}

// CardWidget is a themed panel. Build one with Card.
type CardWidget struct {
	props       property.Owner
	box         *ggui.BoxWidget
	column      *ggui.ColumnWidget // the sections, when the card has them
	sectioned   bool
	footed      bool             // the last section is a footer, which takes the bottom padding's place
	lead, trail *CardMediaWidget // media in first and in last place, whose outer corners the card rounds
	small       bool
	pad         bool
	shadowSet   bool
	fill        color.Color // nil is the theme's Card
	border      color.Color // nil is the theme's Border
	borderWidth float64     // negative is the theme's BorderWidth
	radius      float64     // negative is the theme's RadiusLg
}

// cardSection is a part of a card that sees to its own sides: CardHeader,
// CardContent, CardFooter and CardMedia.
type cardSection interface{ cardSection() }

// cardContext is what a card tells its sections.
type cardContext struct {
	pad    ggui.EdgeInsets // the card's spacing, less the border the card already keeps clear
	small  bool
	radius float64 // of the corners a footer in last place fills, zero elsewhere
}

var cardContextKey = ggui.NewEnvKey[cardContext]("card sections")

// cardContextOf is the context of the card around a section, or the
// theme's own spacing for a section used outside one.
func cardContextOf(env ggui.Env, t *uitheme.Theme) cardContext {
	if ctx, ok := env.Get(cardContextKey); ok {
		return ctx
	}
	return cardContext{pad: t.CardPad}
}

// cardSpacing is the padding of a card and the gap between its sections:
// the theme's CardPad, or three quarters of it for a small card.
func cardSpacing(t *uitheme.Theme, small bool) ggui.EdgeInsets {
	p := t.CardPad
	if small {
		p = ggui.EdgeInsets{Top: p.Top * .75, Right: p.Right * .75, Bottom: p.Bottom * .75, Left: p.Left * .75}
	}
	return p
}

// Small tightens the card: three quarters of the theme's padding around
// and between its sections, and a header title at the body text's size.
func (c *CardWidget) Small() *CardWidget {
	defer property.Watch(&c.props, &c.small)()
	c.small = true
	return c
}

// Fill paints the card in c instead of the theme's Card surface; nil goes
// back to it.
func (c *CardWidget) Fill(col color.Color) *CardWidget {
	c.fill = col
	return c
}

// Border draws the card's outline w wide in col instead of the theme's; a
// zero w draws none.
func (c *CardWidget) Border(w float64, col color.Color) *CardWidget {
	c.borderWidth, c.border = max(w, 0), col
	return c
}

// Radius rounds the card's corners by r instead of the theme's RadiusLg.
func (c *CardWidget) Radius(r float64) *CardWidget {
	c.radius = max(r, 0)
	return c
}

// Shadow sets outer shadow layers without changing the card's layout.
func (c *CardWidget) Shadow(styles ...ggui.ShadowStyle) *CardWidget {
	defer property.Watch(&c.props, &c.shadowSet)()
	c.shadowSet = true
	c.box.Shadow(styles...)
	return c
}

// Pad overrides the theme's padding, with the shorthand Insets accepts.
func (c *CardWidget) Pad(sides ...float64) *CardWidget {
	defer property.Watch(&c.props, &c.pad)()
	c.box.Pad(sides...)
	c.pad = true
	return c
}

// Layout implements Widget.
func (c *CardWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	t := uitheme.Of(env)
	if !c.shadowSet {
		c.box.Shadow()
	}
	spacing := cardSpacing(t, c.small)
	border := pick(c.borderWidth >= 0, c.borderWidth, t.BorderWidth)
	radius := pick(c.radius >= 0, c.radius, t.RadiusLg)
	env = env.WithText(ggui.TextStyle{Color: t.CardFg})
	if c.sectioned {
		// The sections reach the card's border, which they keep clear
		// of, and pad their own sides by the rest.
		ctx := cardContext{pad: spacing, small: c.small}
		ctx.pad.Left, ctx.pad.Right = max(0, spacing.Left-border), max(0, spacing.Right-border)
		pad := ggui.EdgeInsets{Top: spacing.Top, Right: border, Bottom: spacing.Bottom, Left: border}
		if c.footed {
			pad.Bottom = border
			ctx.radius = max(0, radius-border)
		}
		// Leading media takes the top padding's place, shadcn's
		// has-[>img:first-child]:pt-0, under the card's top corners;
		// media in last place keeps the padding under it and has the
		// bottom corners, *:[img:last-child]:rounded-b-xl.
		if c.lead != nil {
			pad.Top = border
			c.lead.top = max(0, radius-border)
		}
		if c.trail != nil {
			c.trail.bottom = max(0, radius-border)
		}
		if !c.pad {
			c.box.Padding(pad)
		}
		c.column.Gap(spacing.Top)
		env = env.With(cardContextKey, ctx)
	} else if !c.pad {
		c.box.Padding(spacing)
	}
	c.box.Fill(colorOr(c.fill, t.Card)).
		Border(border, colorOr(c.border, fade(t.Fg, .1))).
		Radius(radius)
	return c.box.Layout(cs, env)
}

// Paint implements Widget. A card groups what is inside it, which a
// screen reader announces as a region it can step over.
func (c *CardWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup}, func(dst *ggui.Canvas) { dst.Paint(c.box, r) })
}

// CardHeaderWidget is the top section of a Card: a title over a
// description, with an optional action at the top right. Build one with
// CardHeader.
type CardHeaderWidget struct {
	title, description *ggui.TextWidget
	action             ggui.Widget
	stale              bool // Action changed what the row should hold
	box                *ggui.BoxWidget
	row                *ggui.RowWidget
	column             *ggui.ColumnWidget
}

// CardHeader creates a card's header titled title. An empty description is
// left out.
func CardHeader(title, description string) *CardHeaderWidget {
	h := &CardHeaderWidget{title: ggui.Text(title).Role(ggui.RoleHeading)}
	if description != "" {
		h.description = ggui.Text(description)
	}
	return h
}

func (h *CardHeaderWidget) cardSection() {}

// Action places a widget at the header's top right, beside the title and
// the description: a button, a badge, a menu.
func (h *CardHeaderWidget) Action(w ggui.Widget) *CardHeaderWidget {
	h.action, h.stale = w, true // the next layout builds the row around it
	return h
}

// Layout implements ggui.Widget.
func (h *CardHeaderWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	ctx := cardContextOf(env, t)
	if h.box == nil || h.stale {
		text := []ggui.Widget{h.title}
		if h.description != nil {
			text = append(text, h.description)
		}
		h.column = ggui.Column(text...).Align(ggui.AlignStretch)
		parts := []ggui.Widget{ggui.Expanded(h.column)}
		if h.action != nil {
			parts = append(parts, h.action)
		}
		h.row = ggui.Row(parts...).Align(ggui.AlignStart)
		h.box = ggui.Box(h.row)
		h.stale = false
	}
	title := t.Text.Merge(t.Heading)
	if ctx.small {
		title.Size = t.Text.Size
	}
	h.title.Style(title).Color(t.CardFg)
	if h.description != nil {
		h.description.Style(t.Text).Color(t.MutedFg)
	}
	// shadcn's grid gap-1 is between the rows and the columns alike.
	h.column.Gap(t.Space / 2)
	h.row.Gap(t.Space / 2)
	h.box.Padding(ggui.EdgeInsets{Left: ctx.pad.Left, Right: ctx.pad.Right})
	return h.box.Layout(c, env)
}

// Paint implements ggui.Widget.
func (h *CardHeaderWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(h.box, r) }

// CardContentWidget is the body section of a Card, padded at its sides.
// Build one with CardContent.
type CardContentWidget struct {
	box *ggui.BoxWidget
}

// CardContent creates a card's body around child.
func CardContent(child ggui.Widget) *CardContentWidget {
	return &CardContentWidget{box: ggui.Box(child)}
}

func (c *CardContentWidget) cardSection() {}

// Layout implements ggui.Widget.
func (c *CardContentWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	ctx := cardContextOf(env, uitheme.Of(env))
	c.box.Padding(ggui.EdgeInsets{Left: ctx.pad.Left, Right: ctx.pad.Right})
	return c.box.Layout(cs, env)
}

// Paint implements ggui.Widget.
func (c *CardContentWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.box, r) }

// CardMediaWidget is a section of a Card that runs from one side of it to
// the other, with no padding: a cover image, a map, a chart. Build one
// with CardMedia.
type CardMediaWidget struct {
	child       ggui.Widget
	top, bottom float64 // the radius of its top and of its bottom corners, set by the card
}

// CardMedia creates a section that shows child from edge to edge of the
// card. In first place it also reaches the card's top edge, where the
// card's rounded corners cut it; an image usually comes inside an
// AspectRatio, which gives it its height.
//
//	ui.Card(
//		ui.CardMedia(ui.AspectRatio(16.0/9, ggui.Image(cover).Fit(ggui.FitCover))),
//		ui.CardHeader("Lisbon", "Three days by the river."),
//	)
func CardMedia(child ggui.Widget) *CardMediaWidget { return &CardMediaWidget{child: child} }

func (m *CardMediaWidget) cardSection() {}

// Layout implements ggui.Widget.
func (m *CardMediaWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return m.child.Layout(c, env)
}

// Paint implements ggui.Widget.
func (m *CardMediaWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	radius := max(m.top, m.bottom)
	if radius <= 0 {
		dst.Clip(r).Paint(m.child, r)
		return
	}
	// A round rect that reaches past the side whose corners stay square.
	shape := r
	if m.top <= 0 {
		shape.Origin.Y -= radius
		shape.Size.H += radius
	}
	if m.bottom <= 0 {
		shape.Size.H += radius
	}
	dst.ClipRoundRect(shape, radius, func(dst *ggui.Canvas) { dst.Clip(r).Paint(m.child, r) })
}

// CardFooterWidget is the bottom section of a Card: a band on a muted
// surface under a line, holding the card's actions in a row. Build one
// with CardFooter.
type CardFooterWidget struct {
	box    *ggui.BoxWidget
	row    *ggui.RowWidget
	theme  *uitheme.Theme
	radius float64
}

// CardFooter creates a card's footer holding children in a row.
func CardFooter(children ...ggui.Widget) *CardFooterWidget {
	f := &CardFooterWidget{row: ggui.Row(children...).Align(ggui.AlignCenter)}
	f.box = ggui.Box(f.row)
	return f
}

func (f *CardFooterWidget) cardSection() {}

// Justify distributes the footer's children along it: ggui.JustifyEnd for
// actions at the right, ggui.SpaceBetween for one at each end.
func (f *CardFooterWidget) Justify(j ggui.Justify) *CardFooterWidget {
	f.row.Justify(j)
	return f
}

// Layout implements ggui.Widget.
func (f *CardFooterWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	ctx := cardContextOf(env, t)
	f.theme, f.radius = t, ctx.radius
	f.row.Gap(t.Space)
	f.box.Padding(ggui.EdgeInsets{Top: ctx.pad.Top, Right: ctx.pad.Right, Bottom: ctx.pad.Bottom, Left: ctx.pad.Left})
	return f.box.Layout(c, env)
}

// Paint implements ggui.Widget.
func (f *CardFooterWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := f.theme
	// Half the Muted surface over the card's, mixed rather than faded so
	// the square top and the rounded bottom can overlap.
	band := mix(t.Card, t.Muted, .5)
	dst.FillRoundRect(r, f.radius, band)
	if f.radius > 0 {
		dst.FillRect(ggui.Rct(r.Origin, ggui.Sz(r.Size.W, max(0, r.Size.H-f.radius))), band)
	}
	dst.FillRect(ggui.Rct(r.Origin, ggui.Sz(r.Size.W, t.BorderWidth)), t.Border)
	dst.Paint(f.box, r)
}

// BadgeStyle is the look of a badge variant under a theme.
type BadgeStyle struct {
	Fill, Label, Border color.Color // nil Fill or Border draws none
	// HoverFill and HoverLabel take the place of Fill and Label under
	// the pointer, and Underline draws a line under the label there; nil
	// and false leave the badge as it is.
	HoverFill, HoverLabel color.Color
	Underline             bool
	// LinkFill and LinkLabel are HoverFill and HoverLabel for a badge
	// that acts when tapped, where they are set: the looks shadcn/ui
	// gives a badge only when it is a link.
	LinkFill, LinkLabel color.Color
}

// BadgeVariant is a look for Badge.Variant.
type BadgeVariant = Variant[BadgeStyle]

// The badge variants of shadcn/ui. Restyle one to change every badge drawn
// with it under a theme, or make more with NewVariant.
var (
	BadgePrimary = NewVariant("primary", func(t uitheme.Theme) BadgeStyle {
		return BadgeStyle{Fill: t.Primary, Label: t.PrimaryFg, LinkFill: fade(t.Primary, .8)}
	})
	BadgeSecondary = NewVariant("secondary", func(t uitheme.Theme) BadgeStyle {
		return BadgeStyle{Fill: t.Secondary, Label: t.SecondaryFg, LinkFill: fade(t.Secondary, .8)}
	})
	BadgeOutline = NewVariant("outline", func(t uitheme.Theme) BadgeStyle {
		return BadgeStyle{Label: t.Fg, Border: t.Border, LinkFill: t.Muted, LinkLabel: t.MutedFg}
	})
	BadgeDestructive = NewVariant("destructive", func(t uitheme.Theme) BadgeStyle {
		return BadgeStyle{Fill: fade(t.Destructive, pick(isDark(t.Bg), .2, .1)), Label: t.Destructive, LinkFill: fade(t.Destructive, .2)}
	})
	// BadgeGhost is a bare label in the page's text color, on the Muted
	// surface under the pointer.
	BadgeGhost = NewVariant("ghost", func(t uitheme.Theme) BadgeStyle {
		return BadgeStyle{Label: t.Fg, HoverFill: fade(t.Muted, pick(isDark(t.Bg), .5, 1)), HoverLabel: t.MutedFg}
	})
	// BadgeLink is a bare label in the Primary color, underlined under
	// the pointer.
	BadgeLink = NewVariant("link", func(t uitheme.Theme) BadgeStyle {
		return BadgeStyle{Label: t.Primary, Underline: true}
	})
)

// BadgeWidget is a small pill with a short label. Build one with Badge.
// It takes no input unless OnTap gives it an action: the looks a variant
// has under the pointer follow where the pointer is without claiming it,
// so a badge inside a button or a tab leaves the hover and the click to
// what it sits in.
type BadgeWidget struct {
	ggui.Interactive
	props          property.Owner
	action         func()
	motion         time.Duration
	hover          float64 // how far into its look under the pointer the badge was painted
	text           *ggui.TextWidget
	variant        *BadgeVariant
	radius         float64 // set by Radius; negative is a pill
	lead, trail    icons.Role
	style          BadgeStyle
	pad            ggui.EdgeInsets
	size           ggui.Size
	iconSide, gap  float64
	leads, trails  bool
	theme          *uitheme.Theme
	env            ggui.Env
	leadAt, textAt float64 // where the leading icon and the label start, from the left edge
	trailAt        float64
}

// Badge creates a pill labelled s on the Primary surface; Variant, or
// Secondary, Outline, Destructive, Ghost and Link, change its look.
func Badge(s string) *BadgeWidget {
	b := &BadgeWidget{text: ggui.Text(s).NoWrap(), variant: BadgePrimary, radius: -1}
	b.Role = ggui.RoleLink
	b.SetName(s)
	b.AutoKey()
	return b
}

// OnTap makes the badge a link that runs fn on a click, or on Space or
// Enter while it has the keyboard focus: it shows the pointer cursor and
// a focus ring, and takes its variant's look for a link under the
// pointer. A nil fn takes the action away. The caller decides what the
// link opens.
func (b *BadgeWidget) OnTap(fn func()) *BadgeWidget {
	defer property.Watch(&b.props, &b.action)()
	b.action = fn
	return b
}

// Variant draws the badge in v, one of the Badge variables or a variant of
// your own; see NewVariant.
func (b *BadgeWidget) Variant(v *BadgeVariant) *BadgeWidget {
	defer property.Watch(&b.props, &b.variant)()
	b.variant = v
	return b
}

// Secondary fills the badge with the Secondary color.
func (b *BadgeWidget) Secondary() *BadgeWidget { return b.Variant(BadgeSecondary) }

// Primary fills the badge with the Primary color.
func (b *BadgeWidget) Primary() *BadgeWidget { return b.Variant(BadgePrimary) }

// Outline draws the badge as a border with no fill.
func (b *BadgeWidget) Outline() *BadgeWidget { return b.Variant(BadgeOutline) }

// Destructive fills the badge with the Destructive color.
func (b *BadgeWidget) Destructive() *BadgeWidget { return b.Variant(BadgeDestructive) }

// Ghost draws the badge as a bare label in the page's text color.
func (b *BadgeWidget) Ghost() *BadgeWidget { return b.Variant(BadgeGhost) }

// Link draws the badge as a bare label in the Primary color.
func (b *BadgeWidget) Link() *BadgeWidget { return b.Variant(BadgeLink) }

// Icon draws the icon of role before the label, in the label's color. An
// empty role removes it.
func (b *BadgeWidget) Icon(role icons.Role) *BadgeWidget {
	defer property.Watch(&b.props, &b.lead)()
	b.lead = role
	return b
}

// TrailingIcon draws the icon of role after the label, in the label's
// color. An empty role removes it.
func (b *BadgeWidget) TrailingIcon(role icons.Role) *BadgeWidget {
	defer property.Watch(&b.props, &b.trail)()
	b.trail = role
	return b
}

// Radius gives the badge corners of radius r instead of a pill's.
func (b *BadgeWidget) Radius(r float64) *BadgeWidget {
	defer property.Watch(&b.props, &b.radius)()
	b.radius = max(r, 0)
	return b
}

// Layout implements Widget.
func (b *BadgeWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout().EndLayout()
	t := uitheme.Of(env)
	b.theme, b.env = t, env
	b.motion = env.Motion(t.MotionFast)
	b.style = b.variant.Style(*t)
	b.leads, b.trails = b.lead != "", b.trail != ""
	// An icon is three quarters of the theme's, half a space from the
	// label, and the side it is on is padded a quarter less.
	b.iconSide, b.gap = t.IconSize*.75, t.Space/2
	b.pad = ggui.Insets(2, t.Space)
	extra := 0.0
	if b.leads {
		b.pad.Left = t.Space * .75
		extra += b.iconSide + b.gap
	}
	if b.trails {
		b.pad.Right = t.Space * .75
		extra += b.iconSide + b.gap
	}
	b.text.Style(t.Caption.Merge(t.Label)).Color(b.style.Label)
	inner := b.pad.Shrink(c).Loosen()
	inner.MaxW = max(0, inner.MaxW-extra)
	b.size = b.text.Layout(inner, env)
	size := b.pad.Inflate(ggui.Sz(b.size.W+extra, b.size.H))
	if env.TextScale() <= 1 && b.size.H <= 20 {
		size.H = 20
	} else {
		size.H = max(20, size.H)
	}
	b.leadAt, b.textAt = b.pad.Left, b.pad.Left
	if b.leads {
		b.textAt += b.iconSide + b.gap
	}
	b.trailAt = b.textAt + b.size.W + b.gap
	return c.Constrain(size)
}

// Paint implements Widget.
func (b *BadgeWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	st := b.style
	radius := pick(b.radius >= 0, b.radius, r.Size.H/2)
	overFill, overLabel := st.HoverFill, st.HoverLabel
	hovered := false
	if b.action != nil {
		b.Hit(dst, r, b, ggui.CursorShapePointer)
		overFill, overLabel = colorOr(st.LinkFill, overFill), colorOr(st.LinkLabel, overLabel)
		hovered = b.Hovered && !b.IsInert()
	} else if overFill != nil || overLabel != nil || st.Underline {
		// Read rather than claimed: the pointer stays with whatever the
		// badge is inside.
		p, ok := dst.Pointer()
		hovered = ok && r.Contains(p) && dst.TakesInput()
	}
	fill, label := st.Fill, st.Label
	b.hover = ease(dst, b.theme, b.Anchor(r), badgeHoverSlot, pick(hovered, 1.0, 0.0), b.motion)
	if k := b.hover; k > 0 {
		if overFill != nil {
			fill = pick(fill == nil, fade(overFill, k), mix(colorOr(fill, overFill), overFill, k))
		}
		if overLabel != nil {
			label = mix(label, overLabel, k)
		}
	}
	b.text.Color(label)
	if fill != nil {
		dst.FillRoundRect(r, radius, fill)
	}
	if st.Border != nil {
		dst.StrokeRoundRect(r, radius, b.theme.BorderWidth, st.Border)
	}
	iconY := r.Origin.Y + (r.Size.H-b.iconSide)/2
	if b.leads {
		paintIcon(dst, b.env, b.lead, ggui.Rct(ggui.Pt(r.Origin.X+b.leadAt, iconY), ggui.Sz(b.iconSide, b.iconSide)), label, 0)
	}
	textAt := ggui.Pt(r.Origin.X+b.textAt, r.Origin.Y+(r.Size.H-b.size.H)/2)
	dst.Paint(b.text, ggui.Rct(textAt, b.size))
	if st.Underline && b.hover > 0 {
		if base, ok := b.text.Baseline(); ok {
			// shadcn's underline-offset-4, as a link button's.
			dst.FillRect(ggui.Rct(ggui.Pt(textAt.X, textAt.Y+base+b.theme.Space/2), ggui.Sz(b.size.W, 1)), fade(label, b.hover))
		}
	}
	if b.trails {
		paintIcon(dst, b.env, b.trail, ggui.Rct(ggui.Pt(r.Origin.X+b.trailAt, iconY), ggui.Sz(b.iconSide, b.iconSide)), label, 0)
	}
	if b.action != nil {
		focusRing(dst, &b.Interactive, b.theme, r, radius, b.theme.Ring, b.motion)
	}
}

// badgeHoverSlot keeps a badge's easing into its look under the pointer.
var badgeHoverSlot = ggui.NewSlot[*ggui.Motion]("badge hover")

// HandlePointer implements ggui.PointerHandler, for a badge with an action.
func (b *BadgeWidget) HandlePointer(ev ggui.PointerEvent) bool { return b.Pointer(ev, b.action) }

// HandleKey implements ggui.KeyHandler: Space or Enter runs the action.
func (b *BadgeWidget) HandleKey(ev ggui.KeyEvent) { b.Keyboard(ev, b.action) }

// Describe implements ggui.Describer: a badge with an action is a link
// named by its label.
func (b *BadgeWidget) Describe() ggui.Node {
	return ggui.Node{Role: b.Role, Name: b.SemanticName(), Actions: ggui.ActionFocus | ggui.ActionPress}
}

// Act implements ggui.Actor: a press runs the action.
func (b *BadgeWidget) Act(a ggui.Action) bool {
	if a.Kind != ggui.ActionPress || b.action == nil {
		return false
	}
	b.action()
	return true
}

// ProgressWidget is a bar filled to a fraction, under an optional label
// and value. Build one with Progress.
type ProgressWidget struct {
	props  property.Owner
	value  ggui.Readable[float64]
	height float64
	theme  *uitheme.Theme
	env    ggui.Env
	motion time.Duration

	label     *ggui.TextWidget // nil without Label
	name      string
	labelSize ggui.Size
	shows     bool // the value is written at the right, above the bar
	format    func(float64) string
	reading   *ggui.TextWidget // the value as text, laid out when it changes
	readSize  ggui.Size
	readFor   float64 // the value reading shows
	readOK    bool
	head, gap float64 // the row above the bar and the space under it
}

// Progress creates a bar that shows value, a fraction from 0 to 1, read
// every frame, and eases toward it as it changes.
func Progress(value ggui.Readable[float64]) *ProgressWidget {
	return &ProgressWidget{value: value, height: 4}
}

var progressSlot = ggui.NewSlot[*ggui.Motion]("progressSlot")

// Height sets the bar's thickness.
func (p *ProgressWidget) Height(h float64) *ProgressWidget {
	defer property.Watch(&p.props, &p.height)()
	p.height = h
	return p
}

// Label writes s above the bar's left end and names the bar for a screen
// reader. An empty s removes it.
func (p *ProgressWidget) Label(s string) *ProgressWidget {
	had := p.label != nil
	p.name = s
	switch {
	case s == "":
		p.label = nil
	case p.label == nil:
		p.label = ggui.Text(s).NoWrap().Ellipsis()
	default:
		p.label.Content(s)
	}
	if had != (p.label != nil) {
		p.props.Changed()
	}
	return p
}

// ShowValue writes the value above the bar's right end, as a percentage.
func (p *ProgressWidget) ShowValue() *ProgressWidget {
	defer property.Watch(&p.props, &p.shows)()
	p.shows = true
	return p
}

// FormatValue writes the value above the bar's right end as format says
// it, "3 of 12" say, instead of as a percentage. format is called when the
// value changes, not on every frame.
func (p *ProgressWidget) FormatValue(format func(fraction float64) string) *ProgressWidget {
	p.format, p.readOK = format, false
	return p.ShowValue()
}

// Layout implements Widget.
func (p *ProgressWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer p.props.Layout().EndLayout()
	t := uitheme.Of(env)
	p.theme, p.env = t, env
	p.motion = env.Motion(t.MotionFast)
	width := bounded(c.MaxW, defaultStripe)
	p.head, p.gap = 0, 0
	if p.label != nil {
		p.label.Style(t.Text.Merge(t.Label)).Color(t.Fg)
		p.labelSize = p.label.Layout(ggui.Loose(ggui.Sz(width, math.Inf(1))), env)
		p.head = p.labelSize.H
	}
	if p.shows {
		if p.reading == nil {
			p.reading = ggui.Text("").NoWrap()
		}
		p.reading.Style(t.Text).Color(t.MutedFg)
		// Laid out here for the row's height and under this Env; Paint
		// writes it again when the value it reads has changed.
		p.read(clamp(ggui.Untrack(p.value.Get), 0, 1), true)
		p.head = max(p.head, p.readSize.H)
	}
	if p.head > 0 {
		p.gap = t.Space * 1.5
	}
	return c.Constrain(ggui.Sz(width, p.head+p.gap+p.height))
}

// read writes the value's text for v, when it does not say it already,
// and lays it out when it changed or relayout asks.
func (p *ProgressWidget) read(v float64, relayout bool) {
	if !p.readOK || v != p.readFor {
		s := ""
		if p.format != nil {
			s = p.format(v)
		} else {
			s = strconv.Itoa(int(math.Round(v*100))) + "%"
		}
		p.reading.Content(s)
		p.readFor, p.readOK, relayout = v, true, true
	}
	if relayout {
		p.readSize = p.reading.Layout(ggui.Loose(ggui.Sz(math.Inf(1), math.Inf(1))), p.env)
	}
}

// Paint implements Widget.
func (p *ProgressWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := p.theme
	target := clamp(p.value.Get(), 0, 1)
	bar := ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+r.Size.H-p.height), ggui.Sz(r.Size.W, p.height))
	if p.head <= 0 {
		bar = r
	}
	v := ease(dst, p.theme, ggui.Anchor{Rect: bar}, progressSlot, target, p.motion)
	// Assistive technology reports the data, independently of visual easing.
	node := ggui.Node{Role: ggui.RoleProgress, Name: p.name, Min: 0, Max: 1, Now: target}
	if p.label != nil {
		dst.Paint(p.label, ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(p.head-p.labelSize.H)/2), p.labelSize))
	}
	if p.shows && p.reading != nil {
		p.read(target, false)
		dst.Paint(p.reading, ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-p.readSize.W, r.Origin.Y+(p.head-p.readSize.H)/2), p.readSize))
	}
	dst.Leaf(bar, node)
	dst.FillRoundRect(bar, bar.Size.H/2, t.Muted)
	if fill := progressFill(bar, v, p.env.Direction() == ggui.DirRTL); fill.Size.W > 0 {
		// The indicator is square, and the track's round ends cut it:
		// shadcn's h-full bg-primary inside overflow-hidden rounded-full.
		dst.Clip(fill).FillRoundRect(bar, bar.Size.H/2, t.Primary)
	}
}

// progressFill is the part of a progress bar's track the indicator covers
// at v, from the end reading starts at.
func progressFill(bar ggui.Rect, v float64, rtl bool) ggui.Rect {
	fill := ggui.Rct(bar.Origin, ggui.Sz(bar.Size.W*clamp(v, 0, 1), bar.Size.H))
	return mirrorHorizontal(fill, bar, rtl)
}

// ProgressRingWidget is a ring filled clockwise from the top to a fraction:
// Progress where a bar does not fit. Build one with ProgressRing.
type ProgressRingWidget struct {
	props        property.Owner
	value        ggui.Readable[float64]
	size, stroke float64
	fill, track  color.Color
	theme        *uitheme.Theme
	motion       time.Duration

	arc   ggui.Path // the filled part, kept while the ring and value hold
	arcAt ggui.Rect
	arcV  float64
}

// ProgressRing creates a ring that shows value, a fraction from 0 to 1,
// read every frame, and eases toward it as it changes.
func ProgressRing(value ggui.Readable[float64]) *ProgressRingWidget {
	return &ProgressRingWidget{value: value, size: 16, stroke: 2}
}

// Size sets the ring's diameter.
func (p *ProgressRingWidget) Size(d float64) *ProgressRingWidget {
	defer property.Watch(&p.props, &p.size)()
	p.size = d
	return p
}

// Stroke sets the ring's thickness.
func (p *ProgressRingWidget) Stroke(w float64) *ProgressRingWidget {
	defer property.Watch(&p.props, &p.stroke)()
	p.stroke = w
	return p
}

// Color sets the filled part's colour; nil is the theme's primary.
func (p *ProgressRingWidget) Color(c color.Color) *ProgressRingWidget {
	defer property.Watch(&p.props, &p.fill)()
	p.fill = c
	return p
}

// Track sets the colour of the ring under the filled part; nil is the
// theme's Muted, the track of a Progress bar.
func (p *ProgressRingWidget) Track(c color.Color) *ProgressRingWidget {
	defer property.Watch(&p.props, &p.track)()
	p.track = c
	return p
}

// Layout implements Widget.
func (p *ProgressRingWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer p.props.Layout().EndLayout()
	p.theme = uitheme.Of(env)
	p.motion = env.Motion(p.theme.MotionFast)
	return c.Constrain(ggui.Sz(p.size, p.size))
}

// Paint implements Widget.
func (p *ProgressRingWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	target := clamp(p.value.Get(), 0, 1)
	v := ease(dst, p.theme, ggui.Anchor{Rect: r}, progressSlot, target, p.motion)
	// Assistive technology reports the data, independently of visual easing.
	dst.Leaf(r, ggui.Node{Role: ggui.RoleProgress, Min: 0, Max: 1, Now: target})
	d := min(r.Size.W, r.Size.H) - p.stroke
	if d <= 0 {
		return
	}
	center := ggui.Pt(r.Origin.X+r.Size.W/2, r.Origin.Y+r.Size.H/2)
	ring := ggui.Rct(ggui.Pt(center.X-d/2, center.Y-d/2), ggui.Sz(d, d))
	dst.StrokeRoundRect(ring, d/2, p.stroke, pick[color.Color](p.track != nil, p.track, p.theme.Muted))
	if v <= 0 {
		return
	}
	if r != p.arcAt || v != p.arcV {
		p.arcAt, p.arcV = r, v
		p.arc.Reset()
		p.arc.Arc(center.X, center.Y, d/2, -math.Pi/2, -math.Pi/2+2*math.Pi*v)
	}
	dst.StrokePath(&p.arc, p.stroke, pick[color.Color](p.fill != nil, p.fill, p.theme.Primary))
}
