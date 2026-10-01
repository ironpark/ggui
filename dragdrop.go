package ggui

import (
	"slices"

	"github.com/ironpark/ggui/internal/property"
)

// Dragging inside the app: a DragSource is something the user can pick up
// and a DropZone somewhere it can be put down, with a value of any type
// carried between them. Files dragged in from the desktop are another
// matter, handled by PointerWidget.OnDrop.
//
// A source does not take the press. The widget it wraps keeps its clicks,
// and only when the pointer moves a few pixels with the button down does the
// press become a drag: the widget gets a PointerUp without a tap, and from
// then on the drag follows the pointer until the button comes up over a
// zone that accepts it, anywhere else, or Escape cancels it.

// dragThreshold is how far, in logical pixels, a press moves before it
// becomes a drag.
const dragThreshold = 4

// dragSource is the side of a DragSourceWidget input talks to.
type dragSource interface {
	dragValue() any
	dragMoved(at, grab Point)
	dragEnded(dropped bool)
}

// dropTarget is the side of a DropZoneWidget input talks to.
type dropTarget interface {
	acceptsDrag(v any) bool
	dragOver(over bool)
	dropped(v any, at Point)
}

// appDrag is the drag in progress, or about to be: from the press until it
// moves far enough, only candidate and from are set.
type appDrag struct {
	candidate *hitRegion // the source under the press
	from      Point      // where the press was
	source    dragSource // set once it is a drag
	value     any
	target    *hitRegion // the zone it is over, if one accepts it
}

// pressDrag notes the drag source under a press, if there is one.
func (in *inputState) pressDrag(at Point) {
	r := in.topmost(func(r *hitRegion) bool { return r.drag != nil && r.rect.Contains(at) })
	in.drag = nil
	if r != nil {
		in.drag = &appDrag{candidate: keep(r), from: at}
	}
}

// updateAppDrag moves the drag in progress: it starts one when a press on a
// source has moved far enough, tracks the zone under the pointer, and on
// release drops onto it. It runs after the pointer buttons were dispatched,
// so a release ends the drag in the frame it happens.
func (in *inputState) updateAppDrag(f frameInput) {
	d := in.drag
	if d == nil {
		return
	}
	released := slices.Contains(f.up, MouseButtonLeft)
	if d.source == nil {
		dx, dy := f.pos.X-d.from.X, f.pos.Y-d.from.Y
		if released || dx*dx+dy*dy < dragThreshold*dragThreshold {
			if released {
				in.drag = nil
			}
			return
		}
		src := d.candidate.drag
		d.source, d.value = src, src.dragValue()
		// The widget under the press lets go of it, without a tap.
		if p := in.pressed; p != nil {
			if cur := in.findPointer(p); cur != nil {
				cur.pointer.HandlePointer(PointerEvent{Kind: PointerUp, Pos: f.pos, Button: in.pressedBtn, Mods: f.mods})
			}
			in.pressed = nil
		}
	}
	grab := Pt(d.from.X-d.candidate.full.Origin.X, d.from.Y-d.candidate.full.Origin.Y)
	d.source.dragMoved(f.pos, grab)
	now := in.topmost(func(r *hitRegion) bool {
		t, ok := r.pointer.(dropTarget)
		return ok && r.rect.Contains(f.pos) && t.acceptsDrag(d.value)
	})
	if d.target != nil && !in.continues(now, d.target) {
		d.target.pointer.(dropTarget).dragOver(false)
		d.target = nil
	}
	if now != nil && d.target == nil {
		now.pointer.(dropTarget).dragOver(true)
	}
	if now != nil {
		d.target = keep(now)
	}
	if !released {
		return
	}
	dropped := false
	if now != nil {
		t := now.pointer.(dropTarget)
		t.dragOver(false)
		t.dropped(d.value, Pt(f.pos.X-now.full.Origin.X, f.pos.Y-now.full.Origin.Y))
		dropped = true
	}
	in.drag = nil
	d.source.dragEnded(dropped)
}

// cancelAppDrag ends a drag in progress without a drop, as Escape does, and
// reports whether there was one.
func (in *inputState) cancelAppDrag() bool {
	d := in.drag
	in.drag = nil
	if d == nil || d.source == nil {
		return false
	}
	if d.target != nil {
		d.target.pointer.(dropTarget).dragOver(false)
	}
	d.source.dragEnded(false)
	return true
}

// DragSourceWidget lets the user drag its child to a DropZone, carrying a
// value of type T. Build one with DragSource.
type DragSourceWidget[T any] struct {
	props    property.Owner
	child    Widget
	value    T
	preview  Widget
	onStart  func()
	onEnd    func(dropped bool)
	disabled bool

	dragging bool
	at, grab Point // the pointer, and where on the child it took hold
	size     Size
}

// DragSource makes child draggable with value. The child keeps its clicks;
// moving the pointer a few pixels with the button down starts the drag,
// which shows a faded copy of the child under the pointer.
func DragSource[T any](child Widget, value T) *DragSourceWidget[T] {
	return &DragSourceWidget[T]{child: child, value: value}
}

// Preview draws w under the pointer while dragging instead of the child.
func (d *DragSourceWidget[T]) Preview(w Widget) *DragSourceWidget[T] {
	defer property.Watch(&d.props, &d.preview)()
	d.preview = w
	return d
}

// Disabled stops the child from being dragged while v is true.
func (d *DragSourceWidget[T]) Disabled(v bool) *DragSourceWidget[T] {
	defer property.Watch(&d.props, &d.disabled)()
	d.disabled = v
	return d
}

// OnDragStart runs when a drag begins.
func (d *DragSourceWidget[T]) OnDragStart(fn func()) *DragSourceWidget[T] { d.onStart = fn; return d }

// OnDragEnd runs when the drag ends, reporting whether a zone took it.
func (d *DragSourceWidget[T]) OnDragEnd(fn func(dropped bool)) *DragSourceWidget[T] {
	d.onEnd = fn
	return d
}

// Dragging reports whether the child is being dragged, so it can paint
// itself as the place the drag came from.
func (d *DragSourceWidget[T]) Dragging() bool { return d.dragging }

// Layout implements Widget.
func (d *DragSourceWidget[T]) Layout(c Constraints, env Env) Size {
	defer d.props.Layout()()
	d.size = d.child.Layout(c, env)
	if d.preview != nil {
		d.preview.Layout(Tight(d.size), env)
	}
	return d.size
}

// Paint implements Widget. The source registers beneath its child, so the
// child's own regions stay on top and keep their clicks.
func (d *DragSourceWidget[T]) Paint(dst *Canvas, r Rect) {
	if !d.disabled {
		dst.add(dst.region(r, nil, hitRegion{drag: d}))
	}
	dst.Paint(d.child, r)
	if !d.dragging {
		return
	}
	ghost := pick(d.preview != nil, d.preview, d.child)
	at := Rct(Pt(d.at.X-d.grab.X, d.at.Y-d.grab.Y), d.size)
	dst.Overlay(func(dst *Canvas) {
		dst.Inert().Layer(LayerOptions{Fade: 0.3}, func(layer *Canvas) { layer.Paint(ghost, at) })
	})
}

func (d *DragSourceWidget[T]) dragValue() any { return d.value }

func (d *DragSourceWidget[T]) dragMoved(at, grab Point) {
	if !d.dragging {
		d.dragging = true
		if d.onStart != nil {
			d.onStart()
		}
	}
	d.at, d.grab = at, grab
}

func (d *DragSourceWidget[T]) dragEnded(dropped bool) {
	d.dragging = false
	if d.onEnd != nil {
		d.onEnd(dropped)
	}
}

// Adopt implements Adopter: a source rebuilt mid-drag carries on with it.
func (d *DragSourceWidget[T]) Adopt(prev any) {
	if p, ok := prev.(*DragSourceWidget[T]); ok && p.dragging {
		d.dragging, d.at, d.grab = true, p.at, p.grab
	}
}

// Dropped is a value put down on a DropZone: the value, where it landed in
// the zone's own coordinates, and the zone's size, so a list row can tell
// whether it went above or below its middle.
type Dropped[T any] struct {
	Value T
	Pos   Point
	Size  Size
}

// Before reports whether the drop landed in the top half of the zone: for
// a vertical list, whether the value goes before the row or after it.
func (d Dropped[T]) Before() bool { return d.Pos.Y < d.Size.H/2 }

// DropZoneWidget takes values of type T dragged from a DragSource. Build
// one with DropZone.
type DropZoneWidget[T any] struct {
	props   property.Owner
	child   Widget
	onDrop  func(Dropped[T])
	accept  func(T) bool
	onHover func(over bool)
	over    bool
	size    Size
}

// DropZone calls onDrop with a T dropped on child. A value of another type
// passes over it, to a zone beneath that takes it.
func DropZone[T any](child Widget, onDrop func(Dropped[T])) *DropZoneWidget[T] {
	return &DropZoneWidget[T]{child: child, onDrop: onDrop}
}

// Accept narrows the values the zone takes to those fn returns true for.
func (z *DropZoneWidget[T]) Accept(fn func(T) bool) *DropZoneWidget[T] { z.accept = fn; return z }

// OnHover runs when a drag the zone accepts comes over it and when it
// leaves, to highlight where the drop would go.
func (z *DropZoneWidget[T]) OnHover(fn func(over bool)) *DropZoneWidget[T] { z.onHover = fn; return z }

// Over reports whether a drag the zone accepts is over it.
func (z *DropZoneWidget[T]) Over() bool { return z.over }

// Layout implements Widget.
func (z *DropZoneWidget[T]) Layout(c Constraints, env Env) Size {
	defer z.props.Layout()()
	z.size = z.child.Layout(c, env)
	return z.size
}

// Paint implements Widget.
func (z *DropZoneWidget[T]) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, z)
	dst.Paint(z.child, r)
}

// HandlePointer implements PointerHandler. The zone takes no pointer input
// of its own; it registers only to be found under a drag.
func (z *DropZoneWidget[T]) HandlePointer(PointerEvent) bool { return false }

func (z *DropZoneWidget[T]) acceptsDrag(v any) bool {
	t, ok := v.(T)
	return ok && (z.accept == nil || z.accept(t))
}

func (z *DropZoneWidget[T]) dragOver(over bool) {
	if z.over == over {
		return
	}
	z.over = over
	if z.onHover != nil {
		z.onHover(over)
	}
}

func (z *DropZoneWidget[T]) dropped(v any, at Point) {
	if z.onDrop != nil {
		z.onDrop(Dropped[T]{Value: v.(T), Pos: at, Size: z.size})
	}
}

// Adopt implements Adopter: a zone rebuilt under a drag stays highlighted.
func (z *DropZoneWidget[T]) Adopt(prev any) {
	if p, ok := prev.(*DropZoneWidget[T]); ok {
		z.over = p.over
	}
}

// Move returns items with the element at from moved to index to, counted
// in the slice before the move, as a list reordered by dragging needs. It
// returns items unchanged when either index is out of range.
func Move[T any](items []T, from, to int) []T {
	if from < 0 || from >= len(items) || to < 0 || to > len(items) || from == to || from+1 == to {
		return items
	}
	out := slices.Clone(items)
	v := out[from]
	out = slices.Delete(out, from, from+1)
	if to > from {
		to--
	}
	return slices.Insert(out, to, v)
}
