package ui

import (
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
)

// CollapsibleWidget is a titled section that folds its content away.
// Build one with Collapsible.
type CollapsibleWidget struct {
	open       ggui.Binding[bool]
	title      *ggui.TextWidget
	content    ggui.Widget
	body       ggui.Widget // content behind Presence, so it animates out
	transition *ggui.TransitionWidget
	ggui.Interactive

	theme     ggui.Theme
	env       ggui.Env
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
	c.transition = ggui.Transition(content).Fade().Slide(0, -6)
	c.body = ggui.Presence(open, c.transition)
	return c
}

// Disabled greys the header out and ignores input while v is true.
func (c *CollapsibleWidget) Disabled(v bool) *CollapsibleWidget { c.SetInert(v); return c }

// DisabledWhen follows r for Disabled without a rebuild.
func (c *CollapsibleWidget) DisabledWhen(r ggui.Reader[bool]) *CollapsibleWidget {
	c.InertWhen(r)
	return c
}

func (c *CollapsibleWidget) toggle() { c.open.Set(!c.open.Peek()) }

// Describe implements ggui.Describer: a disclosure reports whether its
// content is showing, so the expand and collapse actions mean something.
func (c *CollapsibleWidget) Describe() ggui.Node {
	open := c.open.Peek()
	return ggui.Node{
		Role:     ggui.RoleDisclosure,
		Name:     c.Name,
		Expanded: ggui.Expandable(open),
		Disabled: c.Inert,
		Actions:  ggui.ActionPress | ggui.ActionFocus | pick(open, ggui.ActionCollapse, ggui.ActionExpand),
	}
}

// Act implements ggui.Actor: expanding and collapsing say which way to go,
// where pressing only says to change.
func (c *CollapsibleWidget) Act(a ggui.Action) bool {
	if c.Inert {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		c.open.Set(true)
	case ggui.ActionCollapse:
		c.open.Set(false)
	default:
		return false
	}
	return true
}

// Layout implements Widget.
func (c *CollapsibleWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c.Sync()
	t := env.Theme()
	c.theme = t
	c.env = env
	c.motion = env.Motion(t.MotionFast)
	c.transition.Duration(t.MotionFast)
	c.pad = t.FieldPad
	c.title.Color(pick(c.Inert, t.MutedFg, t.Fg))
	c.titleSize = c.title.Layout(ggui.Loose(ggui.Sz(max(cs.MaxW-c.pad.Left-c.pad.Right-t.ControlSize-t.ControlGap, 0), cs.MaxH)), env)
	c.headerH = c.titleSize.H + c.pad.Top + c.pad.Bottom
	body := ggui.Constraints{MinW: cs.MinW, MaxW: cs.MaxW, MaxH: max(cs.MaxH-c.headerH, 0)}
	c.bodySize = c.body.Layout(body, env)
	return cs.Constrain(ggui.Sz(max(c.titleSize.W+c.pad.Left+c.pad.Right+t.ControlSize+t.ControlGap, c.bodySize.W), c.headerH+c.bodySize.H))
}

// Paint implements Widget.
func (c *CollapsibleWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	header := ggui.Rct(r.Origin, ggui.Sz(r.Size.W, c.headerH))
	c.Hit(dst, header, c, ggui.CursorShapePointer)
	if c.Hovered && !c.Inert {
		dst.FillRoundRect(header, t.Radius, t.Muted)
	}
	// The chevron turns from pointing right (0) to pointing down (1).
	v := dst.Ease(c.Anchor(header), chevronSlot, pick(c.open.Peek(), 1.0, 0.0), c.motion)
	cx, cy := r.Origin.X+c.pad.Left+t.ControlSize*0.4, r.Origin.Y+c.headerH/2
	paintIcon(dst, c.env, icons.ChevronRight, ggui.Rct(ggui.Pt(cx-8, cy-8), ggui.Sz(16, 16)), t.MutedFg, v*math.Pi/2)
	dst.Paint(c.title, ggui.Rct(ggui.Pt(r.Origin.X+c.pad.Left+t.ControlSize+t.ControlGap, r.Origin.Y+c.pad.Top), c.titleSize))
	c.FocusRing(dst, header, t.Radius, t.Ring)
	dst.Paint(c.body, ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+c.headerH), c.bodySize))
}

// HandleKey implements KeyHandler: Space or Enter toggles.
func (c *CollapsibleWidget) HandleKey(ev ggui.KeyEvent) { c.Keyboard(ev, c.toggle) }

var chevronSlot = ggui.NewSlot[*ggui.Motion]("chevronSlot")

// HandlePointer implements PointerHandler.
func (c *CollapsibleWidget) HandlePointer(ev ggui.PointerEvent) bool { return c.Pointer(ev, c.toggle) }
