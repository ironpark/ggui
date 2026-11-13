package ui

import (
	"math"
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
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
	theme                     *uitheme.Theme
	gap, padY, fadeLen        float64 // between cards, above and below them, and a faded end's length
	paintCardsFn              func(*ggui.Canvas)
}

// AttachmentGroup lays cards out a space and a half apart with half a space of
// vertical breathing room, and fades them out toward an end there is more beyond.
// Left/Right, Home/End and PageUp/PageDown scroll while the group is focused.
func AttachmentGroup(cards ...*AttachmentWidget) *AttachmentGroupWidget {
	g := &AttachmentGroupWidget{children: cards}
	g.paintCardsFn = g.paintCards
	g.Role = ggui.RoleGroup
	g.SetName("Attachments")
	g.AutoKey()
	return g
}

// Name names the scrollable strip for keyboard users and assistive technology.
func (g *AttachmentGroupWidget) Name(s string) *AttachmentGroupWidget { g.SetName(s); return g }

// Position returns the current scroll offset in logical pixels.
func (g *AttachmentGroupWidget) Position() float64 { return g.offset }

func (g *AttachmentGroupWidget) limit() float64     { return max(0, g.content-g.viewport) }
func (g *AttachmentGroupWidget) scrollTo(v float64) { g.offset = max(0, min(v, g.limit())) }

// Layout implements Widget.
func (g *AttachmentGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	g.theme = uitheme.Of(env)
	g.gap, g.padY, g.fadeLen = g.theme.Space*1.5, g.theme.Space/2, g.theme.Space*2
	g.sizes, g.starts = g.sizes[:0], g.starts[:0]
	width, height := 0.0, 0.0
	for i, card := range g.children {
		if i > 0 {
			width += g.gap
		}
		g.starts = append(g.starts, width)
		// A card cannot exceed the viewport; its title truncates before its actions.
		s := card.Layout(ggui.Loose(ggui.Sz(c.MaxW, max(0, c.MaxH-2*g.padY))), env)
		g.sizes = append(g.sizes, s)
		width += s.W
		height = max(height, s.H)
	}
	size := c.Constrain(ggui.Sz(width, height+2*g.padY))
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

// Paint implements Widget.
func (g *AttachmentGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	g.rect = r
	// Hit registration adopts offset before using it, so keyed rebuilds do not
	// flash the first card or leave the first frame's hit regions behind.
	dst.HitPointer(r, g)
	dst.HitKey(r, g)
	g.scrollTo(g.offset)
	// A wheel's snap waits for the wheel to stop, which needs no frame
	// until then.
	if !g.settle.IsZero() {
		if ggui.FrameTime().Before(g.settle) {
			ggui.WakeAt(g.settle)
		} else {
			g.scrollTo(g.nearest(g.offset))
			g.settle = time.Time{}
		}
	}
	dst.DescribeNode(r, g, func(dst *ggui.Canvas) {
		// The cards fade out toward an end there is more beyond, by as much
		// as there is, up to fadeLen: a mask, so the surface behind the
		// strip shows through whatever it is.
		dst.Clip(r).FadeEdges(r, true, min(g.offset, g.fadeLen), min(g.limit()-g.offset, g.fadeLen), g.paintCardsFn)
	})
	g.FocusRing(dst, r, g.theme.Radius, g.theme.Ring)
}

// paintCards paints the strip's cards where the scroll offset puts them.
func (g *AttachmentGroupWidget) paintCards(dst *ggui.Canvas) {
	for i, card := range g.children {
		dst.Paint(card, ggui.Rct(g.rect.Origin.Add(ggui.Pt(g.starts[i]-g.offset, g.padY)), g.sizes[i]))
	}
}

// Describe implements ggui.Describer.
func (g *AttachmentGroupWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleGroup, Name: g.SemanticName(), Actions: ggui.ActionFocus | ggui.ActionScrollIntoView}
}
func (g *AttachmentGroupWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind != ggui.PointerScroll {
		return false
	}
	by := ev.ScrollBy()
	delta := by.X
	if delta == 0 && !ev.ScrollPixels {
		delta = by.Y
	}
	if delta == 0 || g.limit() == 0 {
		return false
	}
	// Touch already supplies logical pixels. Keep finger tracking and inertia
	// continuous; a wheel's delayed snap would fight both.
	settle := ggui.FrameTime().Add(120 * time.Millisecond)
	if ev.ScrollPixels {
		settle = time.Time{}
	}
	before := g.offset
	g.scrollTo(before - delta)
	g.settle = settle
	return !ev.ScrollMomentum || g.offset != before
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
	case ggui.KeyArrowLeft:
		g.step(-1)
	case ggui.KeyArrowRight:
		g.step(1)
	case ggui.KeyHome:
		g.scrollTo(0)
	case ggui.KeyEnd:
		g.scrollTo(g.limit())
	case ggui.KeyPageUp:
		g.scrollTo(g.nearest(max(0, g.offset-g.viewport)))
	case ggui.KeyPageDown:
		g.scrollTo(g.nearest(min(g.limit(), g.offset+g.viewport)))
	}
	g.settle = time.Time{}
}
func (g *AttachmentGroupWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	if ev.Kind != ggui.KeyPress {
		return false
	}
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyHome, ggui.KeyEnd, ggui.KeyPageUp, ggui.KeyPageDown:
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
