package ui

import (
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// AttachmentGroupWidget is a horizontal attachment strip. Wheel movement snaps
// to a card after the gesture settles; Tab reveals offscreen actions. The group
// itself is focusable so even a row of noninteractive files can be scrolled.
type AttachmentGroupWidget struct {
	ggui.Interactive
	children                  []*AttachmentWidget
	sizes                     []ggui.Size
	starts                    []float64
	offset, content, viewport float64
	rect                      ggui.Rect
	settle                    time.Time
	theme                     ggui.Theme
}

// AttachmentGroup lays cards out with a 12px gap and 4px vertical breathing room.
// Left/Right, Home/End and PageUp/PageDown scroll while the group is focused.
func AttachmentGroup(cards ...*AttachmentWidget) *AttachmentGroupWidget {
	g := &AttachmentGroupWidget{children: cards}
	g.Role, g.Name = ggui.RoleGroup, "Attachments"
	g.AutoKey()
	return g
}

// Named names the scrollable strip for keyboard users and assistive technology.
func (g *AttachmentGroupWidget) Named(s string) *AttachmentGroupWidget { g.Name = s; return g }

// Position returns the current scroll offset in logical pixels.
func (g *AttachmentGroupWidget) Position() float64  { return g.offset }
func (g *AttachmentGroupWidget) limit() float64     { return max(0, g.content-g.viewport) }
func (g *AttachmentGroupWidget) scrollTo(v float64) { g.offset = max(0, min(v, g.limit())) }

func (g *AttachmentGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	g.theme = env.Theme()
	g.sizes, g.starts = g.sizes[:0], g.starts[:0]
	width, height := 0.0, 0.0
	for i, card := range g.children {
		if i > 0 {
			width += 12
		}
		g.starts = append(g.starts, width)
		// A card cannot exceed the viewport; its title truncates before its actions.
		s := card.Layout(ggui.Loose(ggui.Sz(c.MaxW, max(0, c.MaxH-8))), env)
		g.sizes = append(g.sizes, s)
		width += s.W
		height = max(height, s.H)
	}
	size := c.Constrain(ggui.Sz(width, height+8))
	g.content, g.viewport = width, size.W
	g.scrollTo(g.offset)
	return size
}
func (g *AttachmentGroupWidget) nearest(v float64) float64 {
	best := g.limit()
	for _, start := range g.starts {
		point := min(start, g.limit())
		if math.Abs(point-v) < math.Abs(best-v) {
			best = point
		}
	}
	return best
}
func (g *AttachmentGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	g.rect = r
	// Hit registration adopts offset before using it, so keyed rebuilds do not
	// flash the first card or leave the first frame's hit regions behind.
	dst.HitPointer(r, g)
	dst.HitKey(r, g)
	g.scrollTo(g.offset)
	if !g.settle.IsZero() && !ggui.Now().Before(g.settle) {
		g.scrollTo(g.nearest(g.offset))
		g.settle = time.Time{}
	}
	dst.DescribeNode(r, g, func(dst *ggui.Canvas) {
		clipped := dst.Clip(r)
		for i, card := range g.children {
			clipped.Paint(card, ggui.Rct(r.Origin.Add(ggui.Pt(g.starts[i]-g.offset, 4.0)), g.sizes[i]))
		}
		// Scroll-aware edge fades stay visual: they register no hit regions.
		const fadeWidth = 12
		for i := 0; i < fadeWidth; i++ {
			alpha := .85 * (1 - float64(i)/fadeWidth)
			if g.offset > 0 {
				clipped.FillRect(ggui.Rct(r.Origin.Add(ggui.Pt(float64(i), 0.0)), ggui.Sz(1.0, r.Size.H)), fade(g.theme.Card, alpha))
			}
			if g.offset < g.limit() {
				clipped.FillRect(ggui.Rct(r.Origin.Add(ggui.Pt(r.Size.W-float64(i)-1, 0.0)), ggui.Sz(1.0, r.Size.H)), fade(g.theme.Card, alpha))
			}
		}
	})
	g.FocusRing(dst, r, g.theme.Radius, g.theme.Ring)
}
func (g *AttachmentGroupWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleGroup, Name: g.Name, Actions: ggui.ActionFocus | ggui.ActionScrollIntoView}
}
func (g *AttachmentGroupWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind != ggui.PointerScroll {
		return false
	}
	delta := ev.Scroll.X
	if delta == 0 {
		delta = ev.Scroll.Y
	}
	if delta == 0 || g.limit() == 0 {
		return false
	}
	g.scrollTo(g.offset - delta*20)
	g.settle = ggui.Now().Add(120 * time.Millisecond)
	return true // contain wheel movement at either end of a scrollable strip
}
func (g *AttachmentGroupWidget) step(direction int) {
	target := pick(direction < 0, 0.0, g.limit())
	if direction > 0 {
		for _, s := range g.starts {
			if s > g.offset+.5 {
				target = s
				break
			}
		}
	} else {
		for _, s := range g.starts {
			if s < g.offset-.5 {
				target = s
			}
		}
	}
	g.scrollTo(target)
	g.settle = time.Time{}
}
func (g *AttachmentGroupWidget) HandleKey(ev ggui.KeyEvent) {
	g.Keyboard(ev, nil)
	if ev.Kind != ggui.KeyPress {
		return
	}
	switch ev.Key {
	case ebiten.KeyArrowLeft:
		g.step(-1)
	case ebiten.KeyArrowRight:
		g.step(1)
	case ebiten.KeyHome:
		g.scrollTo(0)
	case ebiten.KeyEnd:
		g.scrollTo(g.limit())
	case ebiten.KeyPageUp:
		g.scrollTo(g.nearest(max(0, g.offset-g.viewport)))
	case ebiten.KeyPageDown:
		g.scrollTo(g.nearest(min(g.limit(), g.offset+g.viewport)))
	}
	g.settle = time.Time{}
}
func (g *AttachmentGroupWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	if ev.Kind != ggui.KeyPress {
		return false
	}
	switch ev.Key {
	case ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyHome, ebiten.KeyEnd, ebiten.KeyPageUp, ebiten.KeyPageDown:
		return true
	}
	return false
}

// Reveal keeps a focused trigger or action fully visible. It deliberately does
// not snap: aligning a wide card must not hide the action the user just reached.
func (g *AttachmentGroupWidget) Reveal(target ggui.Rect) {
	left, right := target.Origin.X-g.rect.Origin.X, target.Origin.X+target.Size.W-g.rect.Origin.X
	if target.Origin.Y+target.Size.H <= g.rect.Origin.Y || target.Origin.Y >= g.rect.Origin.Y+g.rect.Size.H {
		return
	}
	if left < 0 {
		g.scrollTo(g.offset + left)
	} else if right > g.viewport {
		g.scrollTo(g.offset + right - g.viewport)
	}
	g.settle = time.Time{}
}

// Adopt preserves scroll position and pending snapping across keyed rebuilds.
func (g *AttachmentGroupWidget) Adopt(prev any) {
	g.Interactive.Adopt(prev)
	if p, ok := prev.(*AttachmentGroupWidget); ok {
		g.offset, g.settle = p.offset, p.settle
	}
}
