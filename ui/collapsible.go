package ui

import (
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// CollapsibleWidget is a titled section that folds its content away.
// Build one with Collapsible.
type CollapsibleWidget struct {
	open    ggui.Binding[bool]
	title   *ggui.TextWidget
	content ggui.Widget
	body    ggui.Widget // content behind Presence, so it animates out
	ggui.Interactive

	theme     ggui.Theme
	motion    time.Duration
	pad       ggui.EdgeInsets
	titleSize ggui.Size
	headerH   float64
	bodySize  ggui.Size
}

// Collapsible creates a section whose content shows while open is true.
// The header toggles it on click, Space or Enter; the content fades and
// slides in and out.
func Collapsible(open ggui.Binding[bool], title string, content ggui.Widget) *CollapsibleWidget {
	c := &CollapsibleWidget{open: open, title: ggui.Text(title).NoWrap(), content: content}
	c.Role, c.Name = ggui.RoleDisclosure, title
	c.AutoKey()
	c.body = ggui.Presence(open, ggui.Transition(content).Fade().Slide(0, -6).Duration(knobDuration))
	return c
}

// Disabled greys the header out and ignores input while v is true.
func (c *CollapsibleWidget) Disabled(v bool) *CollapsibleWidget { c.Inert = v; return c }

// DisabledWhen follows r for Disabled without a rebuild.
func (c *CollapsibleWidget) DisabledWhen(r ggui.Reader[bool]) *CollapsibleWidget {
	c.InertWhen(r)
	return c
}

func (c *CollapsibleWidget) toggle() { c.open.Set(!c.open.Peek()) }

// Layout implements Widget.
func (c *CollapsibleWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c.Sync()
	t := env.Theme()
	c.theme = t
	c.motion = env.Motion(knobDuration)
	c.pad = t.FieldPad
	c.title.Color(pick(c.Inert, t.Muted, t.Fg))
	c.titleSize = c.title.Layout(ggui.Loose(ggui.Sz(max(cs.MaxW-c.pad.Left-c.pad.Right-controlSize-controlGap, 0), cs.MaxH)), env)
	c.headerH = c.titleSize.H + c.pad.Top + c.pad.Bottom
	body := ggui.Constraints{MinW: cs.MinW, MaxW: cs.MaxW, MaxH: max(cs.MaxH-c.headerH, 0)}
	c.bodySize = c.body.Layout(body, env)
	return cs.Constrain(ggui.Sz(max(c.titleSize.W+c.pad.Left+c.pad.Right+controlSize+controlGap, c.bodySize.W), c.headerH+c.bodySize.H))
}

// Paint implements Widget.
func (c *CollapsibleWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	header := ggui.Rct(r.Origin, ggui.Sz(r.Size.W, c.headerH))
	c.Hit(dst, header, c, ebiten.CursorShapePointer)
	if c.Hovered && !c.Inert {
		dst.FillRoundRect(header, t.Radius, subtle(t))
	}
	// The chevron turns from pointing right (0) to pointing down (1).
	v := dst.Ease(c.Anchor(header), chevronSlot, pick(c.open.Peek(), 1.0, 0.0), c.motion)
	cx, cy := r.Origin.X+c.pad.Left+controlSize*0.4, r.Origin.Y+c.headerH/2
	rot := func(x, y float64) ggui.Point {
		a := v * math.Pi / 2
		return ggui.Pt(cx+x*math.Cos(a)-y*math.Sin(a), cy+x*math.Sin(a)+y*math.Cos(a))
	}
	tip := rot(2, 0)
	dst.StrokeLine(rot(-2, -4), tip, 1.5, t.Muted)
	dst.StrokeLine(tip, rot(-2, 4), 1.5, t.Muted)
	dst.Paint(c.title, ggui.Rct(ggui.Pt(r.Origin.X+c.pad.Left+controlSize+controlGap, r.Origin.Y+c.pad.Top), c.titleSize))
	c.FocusRing(dst, header, t.Radius, t.Accent)
	dst.Paint(c.body, ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+c.headerH), c.bodySize))
}

// HandleKey implements KeyHandler: Space or Enter toggles.
func (c *CollapsibleWidget) HandleKey(ev ggui.KeyEvent) { c.Keyboard(ev, c.toggle) }

var chevronSlot = ggui.NewSlot[*ggui.Motion]("chevronSlot")

// HandlePointer implements PointerHandler.
func (c *CollapsibleWidget) HandlePointer(ev ggui.PointerEvent) bool { return c.Pointer(ev, c.toggle) }
