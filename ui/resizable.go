package ui

import (
	"math"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ResizableWidget splits its space into two clipped panes with a draggable divider.
// Nest splitters for more panes. The binding is the first pane's fraction of
// available space, excluding the divider.
//
// The divider is a line of the theme's border width where the panes meet,
// grabbed across 8 logical pixels over their edges, so the line of a split
// nested in one pane runs up to it.
//
// Each pane is a ggui.Contain: its size is the split's to decide, so a
// change of size inside one, a disclosure easing open, is painted within
// that pane alone.
type ResizableWidget struct {
	props property.Owner
	ggui.Interactive
	laidFraction                        float64
	fraction                            ggui.Binding[float64]
	first, second                       ggui.Widget
	withHandle                          bool
	vertical                            bool
	minFirst, minSecond                 float64
	onChange                            func(float64)
	theme                               *uitheme.Theme
	env                                 ggui.Env
	rect, firstRect, secondRect, handle ggui.Rect
	available, lo, hi, dragOffset       float64
}

// Resizable creates a horizontal split. Arrow keys move 5%, as
// react-resizable-panels' do (10% with Shift);
// Home/End move to the allowed limits. It fills finite constraints and uses
// 320x200 logical pixels as its fallback under an unbounded parent.
func Resizable(fraction ggui.Binding[float64], first, second ggui.Widget) *ResizableWidget {
	r := &ResizableWidget{fraction: fraction, first: ggui.Contain(first), second: ggui.Contain(second)}
	r.Role = ggui.RoleSeparator
	r.AutoKey()
	if r.HitID() == nil {
		r.Key(r)
	}
	return r
}

// Vertical stacks the panes above and below each other.
func (r *ResizableWidget) Vertical() *ResizableWidget { r.vertical = true; return r }

// WithHandle displays a small grip at the center of the divider: a rounded
// bar in the border color. The divider is grabbed along its whole length
// with or without one.
func (r *ResizableWidget) WithHandle() *ResizableWidget {
	defer property.Watch(&r.props, &r.withHandle)()
	r.withHandle = true
	return r
}

// Hairline lays the divider out as its line alone, which every split now
// does: it is kept for the code that asked for it when the divider took 8
// logical pixels between the panes.
func (r *ResizableWidget) Hairline() *ResizableWidget { return r }

// MinSizes sets the minimum extent of each pane in logical pixels. If both
// cannot fit, available space is distributed in proportion to these minima.
func (r *ResizableWidget) MinSizes(first, second float64) *ResizableWidget {
	defer property.Watch(&r.props, &r.minFirst)()
	defer property.Watch(&r.props, &r.minSecond)()
	r.minFirst = max(0, first)
	r.minSecond = max(0, second)
	return r
}

// Name names the divider for tests and the inspector.
func (r *ResizableWidget) Name(s string) *ResizableWidget { r.SetName(s); return r }

// Disabled prevents dragging and keyboard resizing.
func (r *ResizableWidget) Disabled(v bool) *ResizableWidget { r.SetInert(v); return r }

// OnChange reports the new fraction after a drag or keyboard resize.
func (r *ResizableWidget) OnChange(fn func(float64)) *ResizableWidget { r.onChange = fn; return r }
func (r *ResizableWidget) value() float64 {
	v := ggui.Untrack(r.fraction.Get)
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
	defer r.props.Layout().EndLayout()
	r.Sync()
	r.env, r.theme = env, uitheme.Of(env)
	size := c.Constrain(ggui.Sz(bounded(c.MaxW, 320), bounded(c.MaxH, 200)))
	main := size.W
	if r.vertical {
		main = size.H
	}
	handle := min(r.theme.BorderWidth, main)
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
	// Grabbed across more than the line, centred on it, within the split.
	r.handle = grabArea(r.handle, r.vertical, size)
	if r.mirrored() {
		parent := ggui.Rct(ggui.Pt(0, 0), size)
		r.firstRect = mirrorHorizontal(r.firstRect, parent, true)
		r.secondRect = mirrorHorizontal(r.secondRect, parent, true)
		r.handle = mirrorHorizontal(r.handle, parent, true)
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
		dst.Paint(w, at) // a Contain, clipped to at
	}
	handle := r.handle
	handle.Origin = handle.Origin.Add(rect.Origin)
	r.Hit(dst, handle, r, pick(r.vertical, ggui.CursorShapeNSResize, ggui.CursorShapeEWResize))
	// The line is the gap between the panes, wherever the grab area was
	// clamped to.
	t := r.theme
	width := min(t.BorderWidth, pick(r.vertical, rect.Size.H, rect.Size.W))
	line := ggui.Rct(ggui.Pt(rect.Origin.X+r.firstRect.Size.W, rect.Origin.Y), ggui.Sz(width, rect.Size.H))
	if r.vertical {
		line = ggui.Rct(ggui.Pt(rect.Origin.X, rect.Origin.Y+r.firstRect.Size.H), ggui.Sz(rect.Size.W, width))
	}
	line = mirrorHorizontal(line, rect, r.mirrored())
	if r.Focused && r.FocusVisible && !r.IsInert() {
		// shadcn's ring-1: the ring color for the border's width on
		// either side of the line.
		ring := ggui.Rct(line.Origin.Add(ggui.Pt(-t.BorderWidth, 0)), ggui.Sz(line.Size.W+2*t.BorderWidth, line.Size.H))
		if r.vertical {
			ring = ggui.Rct(line.Origin.Add(ggui.Pt(0, -t.BorderWidth)), ggui.Sz(line.Size.W, line.Size.H+2*t.BorderWidth))
		}
		dst.FillRect(ring, t.Ring)
	}
	dst.FillRect(line, t.Border)
	if r.withHandle {
		center := line.Origin.Add(ggui.Pt(line.Size.W/2, line.Size.H/2))
		w, h := resizableGripW, min(resizableGripH, handle.Size.H)
		if r.vertical {
			w, h = min(resizableGripH, handle.Size.W), resizableGripW
		}
		dst.FillRoundRect(ggui.Rct(ggui.Pt(center.X-w/2, center.Y-h/2), ggui.Sz(w, h)), resizableGripW/2, t.Border)
	}
}

// The grip WithHandle draws: shadcn's h-6 w-1, turned for a vertical split.
const (
	resizableGripW = 4.0
	resizableGripH = 24.0
)

// ConsumesKey implements ggui.KeyConsumer.
func (r *ResizableWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	return navConsumes(ev, pick(r.vertical, navVertical, navHorizontal))
}

// HandleKey implements ggui.KeyHandler.
func (r *ResizableWidget) HandleKey(ev ggui.KeyEvent) {
	r.Keyboard(ev, nil)
	if r.IsInert() || !r.ConsumesKey(ev) {
		return
	}
	// react-resizable-panels moves a separator 5 of the group's 100 for an
	// arrow key; Shift, which it does not read, is twice that here.
	step := .05
	if ev.Mods.Shift {
		step = .1
	}
	ev = directionalKey(ev, r.mirrored())
	switch ev.Key {
	case ggui.KeyHome:
		r.set(r.lo)
	case ggui.KeyEnd:
		r.set(r.hi)
	case ggui.KeyArrowLeft, ggui.KeyArrowUp:
		r.set(r.value() - step)
	default:
		r.set(r.value() + step)
	}
}

// CaptureTouchDrag keeps divider drags from panning an enclosing scroller.
func (r *ResizableWidget) CaptureTouchDrag() bool { return !r.IsInert() }

// HandlePointer implements ggui.PointerHandler; captures drag past the divider.
func (r *ResizableWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if r.IsInert() {
		return false
	}
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		r.dragOffset = r.pointerAxis(ev.Pos) - r.available*r.value()
	case ggui.PointerDrag:
		if r.Pressed && r.available > 0 {
			r.set((r.pointerAxis(ev.Pos) - r.dragOffset) / r.available)
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

// BindDisabled follows r for Disabled without rebuilding the control.
func (r *ResizableWidget) BindDisabled(when ggui.Readable[bool]) *ResizableWidget {
	r.BindInert(when)
	return r
}

func (r *ResizableWidget) name() string {
	return pick(r.SemanticName() != "", r.SemanticName(), "Resize panels")
}

// Semantics implements ggui.Semantic, including the built-in fallback name.
func (r *ResizableWidget) Semantics() (ggui.Role, string) { return r.Role, r.name() }

// grabArea widens the divider's line to the 8 logical pixels it is grabbed
// across, centred on it and kept within size.
func grabArea(line ggui.Rect, vertical bool, size ggui.Size) ggui.Rect {
	const grab = 8.0
	if vertical {
		y := clamp(line.Origin.Y+line.Size.H/2-grab/2, 0, max(0, size.H-grab))
		return ggui.Rct(ggui.Pt(0, y), ggui.Sz(size.W, min(grab, size.H)))
	}
	x := clamp(line.Origin.X+line.Size.W/2-grab/2, 0, max(0, size.W-grab))
	return ggui.Rct(ggui.Pt(x, 0), ggui.Sz(min(grab, size.W), size.H))
}

// mirrored reports whether the panes run right to left: a horizontal split
// under a right-to-left direction.
func (r *ResizableWidget) mirrored() bool {
	return !r.vertical && r.env.Direction() == ggui.DirRTL
}

func (r *ResizableWidget) pointerAxis(p ggui.Point) float64 {
	v := r.axis(p) - r.axis(r.rect.Origin)
	if r.mirrored() {
		return r.rect.Size.W - v
	}
	return v
}
