package ui

import (
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// CollapsibleWidget is a titled section that folds its content away.
// Build one with Collapsible.
type CollapsibleWidget struct {
	open     ggui.Binding[bool]
	title    *ggui.TextWidget
	content  ggui.Widget
	reveal   ggui.Motion
	progress float64
	ggui.Interactive

	theme     uitheme.Theme
	env       ggui.Env
	motion    time.Duration
	pad       ggui.EdgeInsets
	titleSize ggui.Size
	headerH   float64
	bodySize  ggui.Size
}

// Collapsible creates a section whose content shows while open is true.
// The header toggles it on click, Space or Enter; the content
// reveals and clips vertically in and out.
func Collapsible(open ggui.Binding[bool], title string, content ggui.Widget) *CollapsibleWidget {
	c := &CollapsibleWidget{open: open, title: ggui.Text(title).NoWrap(), content: content}
	c.Role = ggui.RoleDisclosure
	c.SetName(title)
	c.AutoKey()
	return c
}

// Disabled greys the header out and ignores input while v is true.
func (c *CollapsibleWidget) Disabled(v bool) *CollapsibleWidget { c.SetInert(v); return c }

// BindDisabled follows r for Disabled without a rebuild.
func (c *CollapsibleWidget) BindDisabled(r ggui.Readable[bool]) *CollapsibleWidget {
	c.BindInert(r)
	return c
}

func (c *CollapsibleWidget) toggle() { c.open.Set(!ggui.Untrack(c.open.Get)) }

// Describe implements ggui.Describer: a disclosure reports whether its
// content is showing, so the expand and collapse actions mean something.
func (c *CollapsibleWidget) Describe() ggui.Node {
	open := ggui.Untrack(c.open.Get)
	return ggui.Node{
		Role:     ggui.RoleDisclosure,
		Name:     c.SemanticName(),
		Expanded: ggui.Expandable(open),
		Disabled: c.IsInert(),
		Actions:  ggui.ActionPress | ggui.ActionFocus | pick(open, ggui.ActionCollapse, ggui.ActionExpand),
	}
}

// Act implements ggui.Actor: expanding and collapsing say which way to go,
// where pressing only says to change.
func (c *CollapsibleWidget) Act(a ggui.Action) bool {
	if c.IsInert() {
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
	t := uitheme.From(env)
	c.theme = t
	c.env = env
	c.motion = env.Motion(t.MotionFast)
	open := c.open.Get()
	c.progress = c.reveal.Toggle(open, ggui.FrameTime(), c.motion)
	c.pad = t.FieldPad
	c.title.Color(pick(c.IsInert(), t.MutedFg, t.Fg))
	c.titleSize = c.title.Layout(ggui.Loose(ggui.Sz(max(cs.MaxW-c.pad.Left-c.pad.Right-t.ControlSize-t.ControlGap, 0), cs.MaxH)), env)
	c.headerH = c.titleSize.H + c.pad.Top + c.pad.Bottom
	body := ggui.Constraints{MinW: cs.MinW, MaxW: cs.MaxW, MaxH: max(cs.MaxH-c.headerH, 0)}
	c.bodySize = ggui.Size{}
	if c.progress > 0 || open {
		c.bodySize = c.content.Layout(body, env)
	}
	return cs.Constrain(ggui.Sz(max(c.titleSize.W+c.pad.Left+c.pad.Right+t.ControlSize+t.ControlGap, c.bodySize.W), c.headerH+c.bodySize.H*c.progress))
}

// Paint implements Widget.
func (c *CollapsibleWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	open := ggui.Untrack(c.open.Get)
	header := ggui.Rct(r.Origin, ggui.Sz(r.Size.W, c.headerH))
	c.Hit(dst, header, c, ggui.CursorShapePointer)
	if c.Hovered && !c.IsInert() {
		dst.FillRoundRect(header, t.Radius, t.Muted)
	}
	// The chevron turns from pointing right (0) to pointing down (1) with
	// the body it reveals.
	v := c.progress
	cx, cy := r.Origin.X+c.pad.Left+t.ControlSize*0.4, r.Origin.Y+c.headerH/2
	paintIcon(dst, c.env, icons.ChevronRight, ggui.Rct(ggui.Pt(cx-8, cy-8), ggui.Sz(16, 16)), t.MutedFg, v*math.Pi/2)
	dst.Paint(c.title, ggui.Rct(ggui.Pt(r.Origin.X+c.pad.Left+t.ControlSize+t.ControlGap, r.Origin.Y+c.pad.Top), c.titleSize))
	c.FocusRing(dst, header, t.Radius, t.Ring)
	paintDisclosure(dst, c.env, c.content, ggui.Pt(r.Origin.X, r.Origin.Y+c.headerH), r.Size.W, c.bodySize, c.progress, open)
}

// HandleKey implements KeyHandler: Space or Enter toggles.
func (c *CollapsibleWidget) HandleKey(ev ggui.KeyEvent) { c.Keyboard(ev, c.toggle) }

// HandlePointer implements PointerHandler.
func (c *CollapsibleWidget) HandlePointer(ev ggui.PointerEvent) bool { return c.Pointer(ev, c.toggle) }
