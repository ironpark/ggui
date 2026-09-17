package ui

import (
	"time"

	"github.com/ironpark/ggui"
)

// Card is a Surface panel with a border, rounded corners and padding, for
// grouping content. It is a Box, so its setters stay available.
func Card(child ggui.Widget) *CardWidget { return &CardWidget{box: ggui.Box(child)} }

// CardWidget is a themed panel. Build one with Card.
type CardWidget struct {
	box *ggui.BoxWidget
	pad bool
}

// Pad overrides the theme's padding, with the shorthand Insets accepts.
func (c *CardWidget) Pad(sides ...float64) *CardWidget { c.box.Pad(sides...); c.pad = true; return c }

// Layout implements Widget.
func (c *CardWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	if !c.pad {
		c.box.Padding(t.CardPad)
	}
	c.box.Fill(t.Surface).Border(1, t.Border).Radius(t.Radius + 4)
	return c.box.Layout(cs, env)
}

// Paint implements Widget.
func (c *CardWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.box, r) }

// BadgeWidget is a small pill of text. Build one with Badge.
type BadgeWidget struct {
	text   *ggui.TextWidget
	accent bool
	pad    ggui.EdgeInsets
	size   ggui.Size
	theme  ggui.Theme
}

// Badge creates a muted pill labelled s; Accent colors it.
func Badge(s string) *BadgeWidget { return &BadgeWidget{text: ggui.Text(s).NoWrap()} }

// Accent fills the badge with the accent color.
func (b *BadgeWidget) Accent() *BadgeWidget { b.accent = true; return b }

// Layout implements Widget.
func (b *BadgeWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	b.theme = t
	b.pad = ggui.Insets(2, t.Space*0.75)
	b.text.Style(t.Caption).Color(pick(b.accent, t.OnAccent, t.Fg))
	b.size = b.text.Layout(b.pad.Shrink(c).Loosen(), env)
	return c.Constrain(b.pad.Inflate(b.size))
}

// Paint implements Widget.
func (b *BadgeWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := b.theme
	dst.FillRoundRect(r, r.Size.H/2, pick(b.accent, t.Accent, subtle(t)))
	dst.Paint(b.text, ggui.Rct(ggui.Pt(r.Origin.X+b.pad.Left, r.Origin.Y+(r.Size.H-b.size.H)/2), b.size))
}

// ProgressWidget is a bar filled to a fraction. Build one with Progress.
type ProgressWidget struct {
	value  ggui.Reader[float64]
	height float64
	theme  ggui.Theme
	motion time.Duration
}

// Progress creates a bar that shows value, a fraction from 0 to 1, read
// every frame, and eases toward it as it changes.
func Progress(value ggui.Reader[float64]) *ProgressWidget {
	return &ProgressWidget{value: value, height: 6}
}

var progressSlot = ggui.NewSlot[*ggui.Motion]("progressSlot")

// Height sets the bar's thickness.
func (p *ProgressWidget) Height(h float64) *ProgressWidget { p.height = h; return p }

// Layout implements Widget.
func (p *ProgressWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	p.theme = env.Theme()
	p.motion = env.Motion(knobDuration)
	return c.Constrain(ggui.Sz(bounded(c.MaxW, defaultStripe), p.height))
}

// Paint implements Widget.
func (p *ProgressWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := p.theme
	v := dst.Ease(ggui.Anchor{Rect: r}, progressSlot, clamp(p.value.Get(), 0, 1), p.motion)
	dst.FillRoundRect(r, r.Size.H/2, t.Border)
	if w := r.Size.W * v; w > 0 {
		dst.FillRoundRect(ggui.Rct(r.Origin, ggui.Sz(w, r.Size.H)), r.Size.H/2, t.Accent)
	}
}
