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
	open    *ggui.Signal[bool]
	title   *ggui.TextWidget
	content ggui.Widget
	body    ggui.Widget // content behind Presence, so it animates out

	hovered bool
	focus   focusState
	chevron ggui.Motion // 0 pointing right, 1 pointing down

	theme     ggui.Theme
	pad       ggui.EdgeInsets
	titleSize ggui.Size
	headerH   float64
	bodySize  ggui.Size
}

// Collapsible creates a section whose content shows while open is true.
// The header toggles it on click, Space or Enter; the content fades and
// slides in and out.
func Collapsible(open *ggui.Signal[bool], title string, content ggui.Widget) *CollapsibleWidget {
	c := &CollapsibleWidget{open: open, title: ggui.Text(title).NoWrap(), content: content}
	c.body = ggui.Presence(open, ggui.Transition(content).Fade().Slide(0, -6).Duration(knobDuration))
	return c
}

func (c *CollapsibleWidget) toggle() { c.open.Set(!c.open.Peek()) }

// Layout implements Widget.
func (c *CollapsibleWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	c.theme = t
	c.pad = ggui.Insets(t.Space*0.75, t.Space)
	c.title.Color(t.Fg)
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
	dst.HitPointer(header, c)
	dst.HitKey(header, c)
	dst.HitCursor(header, ebiten.CursorShapePointer)
	if c.hovered {
		dst.FillRoundRect(header, t.Radius, t.Surface)
	}
	now := time.Now()
	c.chevron.MoveTo(pick(c.open.Peek(), 1.0, 0.0), now, knobDuration)
	v := c.chevron.Value(now)
	// The chevron turns from pointing right to pointing down.
	cx, cy := r.Origin.X+c.pad.Left+controlSize*0.4, r.Origin.Y+c.headerH/2
	rot := func(x, y float64) ggui.Point {
		a := v * math.Pi / 2
		return ggui.Pt(cx+x*math.Cos(a)-y*math.Sin(a), cy+x*math.Sin(a)+y*math.Cos(a))
	}
	tip := rot(2, 0)
	dst.StrokeLine(rot(-2, -4), tip, 1.5, t.Muted)
	dst.StrokeLine(tip, rot(-2, 4), 1.5, t.Muted)
	dst.Paint(c.title, ggui.Rct(ggui.Pt(r.Origin.X+c.pad.Left+controlSize+controlGap, r.Origin.Y+c.pad.Top), c.titleSize))
	c.focus.paintRing(dst, header, t.Radius, t)
	dst.Paint(c.body, ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+c.headerH), c.bodySize))
}

// HandleKey implements KeyHandler: Space or Enter toggles.
func (c *CollapsibleWidget) HandleKey(ev ggui.KeyEvent) {
	c.focus.handle(ev)
	if activates(ev) {
		c.toggle()
	}
}

// Adopt implements ggui.Adopter.
func (c *CollapsibleWidget) Adopt(prev any) {
	if p, ok := prev.(*CollapsibleWidget); ok {
		c.hovered, c.focus, c.chevron = p.hovered, p.focus, p.chevron
	}
}

// HandlePointer implements PointerHandler.
func (c *CollapsibleWidget) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		c.hovered = true
	case ggui.PointerExit:
		c.hovered = false
	case ggui.PointerTap:
		if ev.Button == ebiten.MouseButtonLeft {
			c.toggle()
		}
	case ggui.PointerScroll:
		return false
	}
	return true
}
