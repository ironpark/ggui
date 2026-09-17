package ui

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// ResizableWidget splits its space into two clipped panes with a draggable divider.
// Nest splitters for more panes. The binding is the first pane's fraction of
// available space, excluding the handle.
type ResizableWidget struct {
	ggui.Interactive
	laidFraction                        float64
	fraction                            ggui.Binding[float64]
	first, second                       ggui.Widget
	vertical                            bool
	minFirst, minSecond                 float64
	onChange                            func(float64)
	theme                               ggui.Theme
	env                                 ggui.Env
	rect, firstRect, secondRect, handle ggui.Rect
	available, lo, hi, dragOffset       float64
}

// Resizable creates a horizontal split. Arrow keys move 1% (10% with Shift);
// Home/End move to the allowed limits. It fills finite constraints and uses
// 320x200 logical pixels as its fallback under an unbounded parent.
func Resizable(fraction ggui.Binding[float64], first, second ggui.Widget) *ResizableWidget {
	r := &ResizableWidget{fraction: fraction, first: first, second: second}
	r.Role, r.Name = ggui.RoleSeparator, "Resize panels"
	r.AutoKey()
	if r.HitID() == nil {
		r.Key(r)
	}
	return r
}

// Vertical stacks the panes above and below each other.
func (r *ResizableWidget) Vertical() *ResizableWidget { r.vertical = true; return r }

// MinSizes sets the minimum extent of each pane in logical pixels. If both
// cannot fit, available space is distributed in proportion to these minima.
func (r *ResizableWidget) MinSizes(first, second float64) *ResizableWidget {
	r.minFirst = max(0, first)
	r.minSecond = max(0, second)
	return r
}

// Label names the divider for tests and the inspector.
func (r *ResizableWidget) Label(s string) *ResizableWidget { r.Name = s; return r }

// Disabled prevents dragging and keyboard resizing.
func (r *ResizableWidget) Disabled(v bool) *ResizableWidget { r.Inert = v; return r }

// OnChange reports the new fraction after a drag or keyboard resize.
func (r *ResizableWidget) OnChange(fn func(float64)) *ResizableWidget { r.onChange = fn; return r }
func (r *ResizableWidget) value() float64 {
	v := r.fraction.Peek()
	if math.IsNaN(v) {
		v = .5
	}
	return clamp(v, r.lo, r.hi)
}
func (r *ResizableWidget) set(v float64) {
	setChanged(r.fraction, clamp(v, r.lo, r.hi), r.onChange)
	ggui.Invalidate(r.env)
}
func (r *ResizableWidget) axis(p ggui.Point) float64 {
	if r.vertical {
		return p.Y
	}
	return p.X
}

// Layout implements ggui.Widget.
func (r *ResizableWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	r.env, r.theme = env, env.Theme()
	size := c.Constrain(ggui.Sz(bounded(c.MaxW, 320), bounded(c.MaxH, 200)))
	main := size.W
	if r.vertical {
		main = size.H
	}
	handle := min(8.0, main)
	r.available = max(0, main-handle)
	r.lo, r.hi = 0, 1
	if r.available > 0 {
		if total := r.minFirst + r.minSecond; total > r.available {
			r.lo = r.minFirst / total
			r.hi = r.lo
		} else {
			r.lo = r.minFirst / r.available
			r.hi = 1 - r.minSecond/r.available
		}
	}
	r.laidFraction = r.value()
	first := r.available * r.laidFraction
	second := r.available - first
	if r.vertical {
		r.firstRect = ggui.Rct(ggui.Pt(0, 0), ggui.Sz(size.W, first))
		r.handle = ggui.Rct(ggui.Pt(0.0, first), ggui.Sz(size.W, handle))
		r.secondRect = ggui.Rct(ggui.Pt(0.0, first+handle), ggui.Sz(size.W, second))
	} else {
		r.firstRect = ggui.Rct(ggui.Pt(0, 0), ggui.Sz(first, size.H))
		r.handle = ggui.Rct(ggui.Pt(first, 0.0), ggui.Sz(handle, size.H))
		r.secondRect = ggui.Rct(ggui.Pt(first+handle, 0.0), ggui.Sz(second, size.H))
	}
	r.first.Layout(ggui.Tight(r.firstRect.Size), env)
	r.second.Layout(ggui.Tight(r.secondRect.Size), env)
	return size
}

// Paint implements ggui.Widget.
func (r *ResizableWidget) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	r.rect = rect
	if r.value() != r.laidFraction {
		ggui.Invalidate(r.env)
	}
	for i, w := range []ggui.Widget{r.first, r.second} {
		at := r.firstRect
		if i == 1 {
			at = r.secondRect
		}
		at.Origin = at.Origin.Add(rect.Origin)
		dst.Clip(at).Paint(w, at)
	}
	handle := r.handle
	handle.Origin = handle.Origin.Add(rect.Origin)
	r.Hit(dst, handle, r, pick(r.vertical, ebiten.CursorShapeNSResize, ebiten.CursorShapeEWResize))
	col := pick(r.Hovered || r.Pressed, r.theme.Accent, r.theme.Border)
	dst.FillRoundRect(handle, 2, col)
	r.FocusRing(dst, handle, 2, r.theme.Accent)
}

// ConsumesKey implements ggui.KeyConsumer.
func (r *ResizableWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	if ev.Kind != ggui.KeyPress {
		return false
	}
	if ev.Key == ebiten.KeyHome || ev.Key == ebiten.KeyEnd {
		return true
	}
	if r.vertical {
		return ev.Key == ebiten.KeyArrowUp || ev.Key == ebiten.KeyArrowDown
	}
	return ev.Key == ebiten.KeyArrowLeft || ev.Key == ebiten.KeyArrowRight
}

// HandleKey implements ggui.KeyHandler.
func (r *ResizableWidget) HandleKey(ev ggui.KeyEvent) {
	r.Keyboard(ev, nil)
	if r.Inert || !r.ConsumesKey(ev) {
		return
	}
	step := .01
	if ev.Mods.Shift {
		step = .1
	}
	switch ev.Key {
	case ebiten.KeyHome:
		r.set(r.lo)
	case ebiten.KeyEnd:
		r.set(r.hi)
	case ebiten.KeyArrowLeft, ebiten.KeyArrowUp:
		r.set(r.value() - step)
	default:
		r.set(r.value() + step)
	}
}

// HandlePointer implements ggui.PointerHandler; captures drag past the divider.
func (r *ResizableWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if r.Inert {
		return false
	}
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ebiten.MouseButtonLeft {
			return false
		}
		r.dragOffset = r.axis(ev.Pos) - r.axis(r.rect.Origin) - r.available*r.value()
	case ggui.PointerDrag:
		if r.Pressed && r.available > 0 {
			r.set((r.axis(ev.Pos) - r.axis(r.rect.Origin) - r.dragOffset) / r.available)
		}
	}
	return r.Pointer(ev, nil)
}

// Adopt keeps an in-progress drag across a rebuild.
func (r *ResizableWidget) Adopt(prev any) {
	r.Interactive.Adopt(prev)
	if p, ok := prev.(*ResizableWidget); ok {
		r.dragOffset = p.dragOffset
	}
}
