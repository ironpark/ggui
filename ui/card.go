package ui

import (
	"image/color"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Card is a panel on the theme's Card surface, with a border, rounded
// corners and padding, for grouping content. Fill, Border, Radius, Pad and
// Shadow override the theme for one card.
func Card(child ggui.Widget) *CardWidget {
	return &CardWidget{box: ggui.Box(child), radius: -1, borderWidth: -1}
}

// CardWidget is a themed panel. Build one with Card.
type CardWidget struct {
	props       property.Owner
	box         *ggui.BoxWidget
	pad         bool
	shadowSet   bool
	fill        color.Color // nil is the theme's Card
	border      color.Color // nil is the theme's Border
	borderWidth float64     // negative is the theme's BorderWidth
	radius      float64     // negative is the theme's RadiusLg
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
	defer c.props.Layout()()
	t := uitheme.From(env)
	if !c.shadowSet {
		c.box.Shadow(t.CardShadow)
	}
	if !c.pad {
		c.box.Padding(t.CardPad)
	}
	c.box.Fill(colorOr(c.fill, t.Card)).
		Border(pick(c.borderWidth >= 0, c.borderWidth, t.BorderWidth), colorOr(c.border, t.Border)).
		Radius(pick(c.radius >= 0, c.radius, t.RadiusLg))
	return c.box.Layout(cs, env.WithText(ggui.TextStyle{Color: t.CardFg}))
}

// Paint implements Widget. A card groups what is inside it, which a
// screen reader announces as a region it can step over.
func (c *CardWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup}, func(dst *ggui.Canvas) { dst.Paint(c.box, r) })
}

// BadgeStyle is the look of a badge variant under a theme.
type BadgeStyle struct {
	Fill, Label, Border color.Color // nil Fill or Border draws none
}

// BadgeVariant is a look for Badge.Variant.
type BadgeVariant = Variant[BadgeStyle]

// The badge variants of shadcn/ui. Restyle one to change every badge drawn
// with it under a theme, or make more with NewVariant.
var (
	BadgePrimary = NewVariant("primary", func(t uitheme.Theme) BadgeStyle {
		return BadgeStyle{Fill: t.Primary, Label: t.PrimaryFg}
	})
	BadgeSecondary = NewVariant("secondary", func(t uitheme.Theme) BadgeStyle {
		return BadgeStyle{Fill: t.Secondary, Label: t.SecondaryFg}
	})
	BadgeOutline = NewVariant("outline", func(t uitheme.Theme) BadgeStyle {
		return BadgeStyle{Label: t.Fg, Border: t.Border}
	})
	BadgeDestructive = NewVariant("destructive", func(t uitheme.Theme) BadgeStyle {
		return BadgeStyle{Fill: t.Destructive, Label: t.DestructiveFg}
	})
)

// BadgeWidget is a small pill with a short label. Build one with Badge.
type BadgeWidget struct {
	props   property.Owner
	text    *ggui.TextWidget
	variant *BadgeVariant
	radius  float64 // set by Radius; negative is a pill
	style   BadgeStyle
	pad     ggui.EdgeInsets
	size    ggui.Size
	theme   uitheme.Theme
}

// Badge creates a pill labelled s on the Secondary surface; Variant, or
// Primary, Outline and Destructive, change its look.
func Badge(s string) *BadgeWidget {
	return &BadgeWidget{text: ggui.Text(s).NoWrap(), variant: BadgeSecondary, radius: -1}
}

// Variant draws the badge in v, one of the Badge variables or a variant of
// your own; see NewVariant.
func (b *BadgeWidget) Variant(v *BadgeVariant) *BadgeWidget {
	defer property.Watch(&b.props, &b.variant)()
	b.variant = v
	return b
}

// Primary fills the badge with the Primary color.
func (b *BadgeWidget) Primary() *BadgeWidget { return b.Variant(BadgePrimary) }

// Outline draws the badge as a border with no fill.
func (b *BadgeWidget) Outline() *BadgeWidget { return b.Variant(BadgeOutline) }

// Destructive fills the badge with the Destructive color.
func (b *BadgeWidget) Destructive() *BadgeWidget { return b.Variant(BadgeDestructive) }

// Radius gives the badge corners of radius r instead of a pill's.
func (b *BadgeWidget) Radius(r float64) *BadgeWidget {
	defer property.Watch(&b.props, &b.radius)()
	b.radius = max(r, 0)
	return b
}

// Layout implements Widget.
func (b *BadgeWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout()()
	t := uitheme.From(env)
	b.theme = t
	b.style = b.variant.Style(t)
	b.pad = ggui.Insets(2, t.Space*0.75)
	b.text.Style(t.Caption.Merge(t.Label)).Color(b.style.Label)
	b.size = b.text.Layout(b.pad.Shrink(c).Loosen(), env)
	return c.Constrain(b.pad.Inflate(b.size))
}

// Paint implements Widget.
func (b *BadgeWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	radius := pick(b.radius >= 0, b.radius, r.Size.H/2)
	if b.style.Fill != nil {
		dst.FillRoundRect(r, radius, b.style.Fill)
	}
	if b.style.Border != nil {
		dst.StrokeRoundRect(r, radius, b.theme.BorderWidth, b.style.Border)
	}
	dst.Paint(b.text, ggui.Rct(ggui.Pt(r.Origin.X+b.pad.Left, r.Origin.Y+(r.Size.H-b.size.H)/2), b.size))
}

// ProgressWidget is a bar filled to a fraction. Build one with Progress.
type ProgressWidget struct {
	props  property.Owner
	value  ggui.Readable[float64]
	height float64
	theme  uitheme.Theme
	motion time.Duration
}

// Progress creates a bar that shows value, a fraction from 0 to 1, read
// every frame, and eases toward it as it changes.
func Progress(value ggui.Readable[float64]) *ProgressWidget {
	return &ProgressWidget{value: value, height: 6}
}

var progressSlot = ggui.NewSlot[*ggui.Motion]("progressSlot")

// Height sets the bar's thickness.
func (p *ProgressWidget) Height(h float64) *ProgressWidget {
	defer property.Watch(&p.props, &p.height)()
	p.height = h
	return p
}

// Layout implements Widget.
func (p *ProgressWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer p.props.Layout()()
	p.theme = uitheme.From(env)
	p.motion = env.Motion(uitheme.From(env).MotionFast)
	return c.Constrain(ggui.Sz(bounded(c.MaxW, defaultStripe), p.height))
}

// Paint implements Widget.
func (p *ProgressWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := p.theme
	v := dst.Ease(ggui.Anchor{Rect: r}, progressSlot, clamp(p.value.Get(), 0, 1), p.motion)
	dst.Leaf(r, ggui.Node{Role: ggui.RoleProgress, Min: 0, Max: 1, Now: v})
	dst.FillRoundRect(r, r.Size.H/2, t.Border)
	if w := r.Size.W * v; w > 0 {
		dst.FillRoundRect(ggui.Rct(r.Origin, ggui.Sz(w, r.Size.H)), r.Size.H/2, t.Primary)
	}
}
