package widget

import (
	"image/color"
	"maps"
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/property"
)

// Dragging inside the app: a DragSource is something the user can pick up
// and a DropZone somewhere it can be put down, with a value of any type
// carried between them. Files dragged in from the desktop are another
// matter, handled by PointerWidget.OnDrop. The session that follows the
// pointer from one to the other is in drag.go.

// DragSourceWidget lets the user drag its child to a DropZone, carrying a
// value of type T. Build one with DragSource.
type DragSourceWidget[T any] struct {
	props    property.Owner
	child    ggui.Widget
	value    T
	preview  ggui.Widget
	onStart  func()
	onEnd    func(dropped bool)
	disabled bool

	dragging    bool
	at, grab    ggui.Point // the pointer, and where on the child it took hold
	size        ggui.Size
	env         ggui.Env
	previewLaid bool
}

// DragSource makes child draggable with value. The child keeps its clicks;
// moving the pointer a few pixels with the button down starts the drag,
// which shows a faded copy of the child under the pointer.
func DragSource[T any](child ggui.Widget, value T) *DragSourceWidget[T] {
	return &DragSourceWidget[T]{child: child, value: value}
}

// Preview draws w under the pointer while dragging instead of the child.
func (d *DragSourceWidget[T]) Preview(w ggui.Widget) *DragSourceWidget[T] {
	defer property.Watch(&d.props, &d.preview)()
	d.preview = w
	d.previewLaid = false
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
func (d *DragSourceWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer d.props.Layout().EndLayout()
	d.env = env
	d.size = d.child.Layout(c, env)
	d.previewLaid = false
	// The preview is only painted mid-drag, so only then is it laid out.
	if d.preview != nil && d.dragging {
		d.preview.Layout(ggui.Tight(d.size), env)
		d.previewLaid = true
	}
	return d.size
}

// Paint implements Widget. The source registers beneath its child, so the
// child's own regions stay on top and keep their clicks.
func (d *DragSourceWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if !d.disabled {
		dst.HitDrag(r, d)
	}
	dst.Paint(d.child, r)
	if !d.dragging {
		return
	}
	if d.preview != nil && !d.previewLaid {
		d.preview.Layout(ggui.Tight(d.size), d.env)
		d.previewLaid = true
	}
	ghost := gem.Pick(d.preview != nil, d.preview, d.child)
	at := ggui.Rct(ggui.Pt(d.at.X-d.grab.X, d.at.Y-d.grab.Y), d.size)
	dst.Overlay(func(dst *ggui.Canvas) {
		dst.Inert().Layer(ggui.LayerOptions{Fade: 0.3}, func(layer *ggui.Canvas) { layer.Paint(ghost, at) })
	})
}

// DragValue implements Draggable: the value the source was made with.
func (d *DragSourceWidget[T]) DragValue() any { return d.value }

// DragMoved implements Draggable: the first move starts the drag.
func (d *DragSourceWidget[T]) DragMoved(at, grab ggui.Point) {
	if !d.dragging {
		d.dragging = true
		d.props.Changed() // lay out the preview
		if d.onStart != nil {
			d.onStart()
		}
	}
	d.at, d.grab = at, grab
}

// DragEnded implements Draggable.
func (d *DragSourceWidget[T]) DragEnded(dropped bool) {
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
	Pos   ggui.Point
	Size  ggui.Size
}

// Before reports whether the drop landed in the top half of the zone: for
// a vertical list, whether the value goes before the row or after it.
func (d Dropped[T]) Before() bool { return d.Pos.Y < d.Size.H/2 }

// DropZoneWidget takes values of type T dragged from a DragSource. Build
// one with DropZone.
type DropZoneWidget[T any] struct {
	props   property.Owner
	child   ggui.Widget
	onDrop  func(Dropped[T])
	accept  func(T) bool
	onHover func(over bool)
	onOver  func(Dropped[T])
	over    bool
	size    ggui.Size
}

// DropZone calls onDrop with a T dropped on child. A value of another type
// passes over it, to a zone beneath that takes it.
func DropZone[T any](child ggui.Widget, onDrop func(Dropped[T])) *DropZoneWidget[T] {
	return &DropZoneWidget[T]{child: child, onDrop: onDrop}
}

// Accept narrows the values the zone takes to those fn returns true for.
func (z *DropZoneWidget[T]) Accept(fn func(T) bool) *DropZoneWidget[T] { z.accept = fn; return z }

// OnHover runs when a drag the zone accepts comes over it and when it
// leaves, to highlight where the drop would go.
func (z *DropZoneWidget[T]) OnHover(fn func(over bool)) *DropZoneWidget[T] { z.onHover = fn; return z }

// OnOver runs as a drag the zone accepts moves over it, with where it would
// land: to show where in the zone the value would go, as a gap opening
// between tabs or rows. OnHover with false says the drag has left.
func (z *DropZoneWidget[T]) OnOver(fn func(Dropped[T])) *DropZoneWidget[T] { z.onOver = fn; return z }

// Over reports whether a drag the zone accepts is over it.
func (z *DropZoneWidget[T]) Over() bool { return z.over }

// Layout implements Widget.
func (z *DropZoneWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer z.props.Layout().EndLayout()
	z.size = z.child.Layout(c, env)
	return z.size
}

// Paint implements Widget.
func (z *DropZoneWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.HitPointer(r, z)
	dst.Paint(z.child, r)
}

// HandlePointer implements PointerHandler. The zone takes no pointer input
// of its own; it registers only to be found under a drag.
func (z *DropZoneWidget[T]) HandlePointer(ggui.PointerEvent) bool { return false }

// AcceptsDrop implements DropTarget: a T that Accept, if set, takes.
func (z *DropZoneWidget[T]) AcceptsDrop(v any) bool {
	t, ok := v.(T)
	return ok && (z.accept == nil || z.accept(t))
}

// DropHover implements DropTarget.
func (z *DropZoneWidget[T]) DropHover(over bool) {
	if z.over == over {
		return
	}
	z.over = over
	if z.onHover != nil {
		z.onHover(over)
	}
}

// DropOver implements DropTarget. A value that is not a T is ignored.
func (z *DropZoneWidget[T]) DropOver(v any, at ggui.Point) {
	if t, ok := v.(T); ok && z.onOver != nil {
		z.onOver(Dropped[T]{Value: t, Pos: at, Size: z.size})
	}
}

// Drop implements DropTarget. A value that is not a T is ignored.
func (z *DropZoneWidget[T]) Drop(v any, at ggui.Point) {
	if t, ok := v.(T); ok && z.onDrop != nil {
		z.onDrop(Dropped[T]{Value: t, Pos: at, Size: z.size})
	}
}

// Adopt implements Adopter: a zone rebuilt under a drag stays highlighted.
func (z *DropZoneWidget[T]) Adopt(prev any) {
	if p, ok := prev.(*DropZoneWidget[T]); ok {
		z.over = p.over
	}
}

// MoveItem returns items with the element at from moved to index to, counted
// in the slice before the move, as a list reordered by dragging needs. It
// returns items unchanged when either index is out of range.
func MoveItem[T any](items []T, from, to int) []T {
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

const (
	reorderDuration  = 200 * time.Millisecond
	reorderEdge      = 48.0  // how near a Scroll's edge a drag starts scrolling it
	reorderMaxSpeed  = 900.0 // logical pixels per second, at the very edge
	reorderMaxFrameS = 0.05
)

var (
	reorderUp   = ggui.MustChord("alt+up")
	reorderDown = ggui.MustChord("alt+down")
)

// ReorderableWidget is a keyed list the user rearranges by dragging rows,
// with Alt+Up and Alt+Down, or through the move actions an assistive
// technology offers. Build one with Reorderable.
type ReorderableWidget[T any, K comparable] struct {
	props     property.Owner
	list      *EachWidget[T, K]
	items     ggui.Binding[[]T]
	onReorder func(from, to int)
	handles   bool
	disabled  bool
	id        any

	st   *reorderState[T, K]
	port *ScrollWidget
	env  ggui.Env
}

// reorderState is a list's drag and its row animations; a rebuilt list
// adopts it, so a drag carries on.
type reorderState[T any, K comparable] struct {
	owner    *ReorderableWidget[T, K] // the live mounted instance; updated in Adopt and Paint
	dragging bool
	key      K          // the row being dragged
	at, grab ggui.Point // the pointer, and where on the row it took hold
	to       int        // where the row would drop, counted as Move counts
	last     time.Time

	shift   map[K]*ggui.Motion // how far each row is moved out of the dragged row's way
	painted map[K]float64      // where each row was drawn at the last paint, list-local
	settle  map[K]float64      // where rows were drawn before a move, to ease from
	reveal  *K                 // a row moved by the keyboard, to keep in view
}

// reorderToken is the value a list's drag carries, accepted by that list alone.
type reorderToken struct{ owner any }

// Reorderable lists items like EachKeyed, one row per key built by build,
// and lets the user move rows: a press that drags a row lifts it under the
// pointer while the others ease aside to open the gap it will drop into,
// a Scroll around the list scrolls while the row is held near its edge,
// and Escape puts it back. Alt+Up and Alt+Down move the row holding
// focus. Every move writes items with Move and then calls OnReorder.
//
//	widget.Reorderable(tasks, func(t Task) int { return t.ID }, func(it widget.EachItem[Task]) ggui.Widget {
//		return widget.Row(widget.ReorderHandle(ui.Icon(icons.Grip)), widget.Text(it.Item.Title)).Gap(8)
//	}).Handles().Gap(4)
//
// Rows lay out vertically and are all laid out, as EachKeyed's are
// without ItemExtent.
func Reorderable[T any, K comparable](items ggui.Binding[[]T], key func(T) K, build func(EachItem[T]) ggui.Widget) *ReorderableWidget[T, K] {
	property.Require(items, "Reorderable")
	l := &ReorderableWidget[T, K]{items: items, id: reactive.AutoID(), st: &reorderState[T, K]{
		shift: map[K]*ggui.Motion{}, painted: map[K]float64{},
	}}
	l.st.owner = l
	l.list = EachKeyed(items, key, func(it EachItem[T]) ggui.Widget {
		return &reorderRow[T, K]{list: l, key: key(it.Item), child: build(it)}
	}).Unlisted()
	return l
}

// Gap sets the space between consecutive rows.
func (l *ReorderableWidget[T, K]) Gap(v float64) *ReorderableWidget[T, K] { l.list.Gap(v); return l }

// Space sets the gap to n times the environment spacing unit.
func (l *ReorderableWidget[T, K]) Space(n float64) *ReorderableWidget[T, K] {
	l.list.Space(n)
	return l
}

// Align places rows across the list.
func (l *ReorderableWidget[T, K]) Align(a CrossAlign) *ReorderableWidget[T, K] {
	l.list.Align(a)
	return l
}

// Stretch makes every row as wide as the list: Align(AlignStretch).
func (l *ReorderableWidget[T, K]) Stretch() *ReorderableWidget[T, K] { return l.Align(AlignStretch) }

// Handles makes rows draggable only by a ReorderHandle inside them, so the
// rest of a row can take presses and drags of its own. The keyboard and
// assistive technology move rows either way.
func (l *ReorderableWidget[T, K]) Handles() *ReorderableWidget[T, K] {
	defer property.Watch(&l.props, &l.handles)()
	l.handles = true
	return l
}

// Disabled stops rows from being moved while v is true.
func (l *ReorderableWidget[T, K]) Disabled(v bool) *ReorderableWidget[T, K] {
	defer property.Watch(&l.props, &l.disabled)()
	l.disabled = v
	return l
}

// OnReorder runs after a move has been written to items, with the indexes
// Move takes: from where the row was, and to, its place counted before the
// move, so items[to] was the row it now sits before.
func (l *ReorderableWidget[T, K]) OnReorder(fn func(from, to int)) *ReorderableWidget[T, K] {
	l.onReorder = fn
	return l
}

// Key gives the list an identity, so a rebuilt one that also moved keeps
// its drag. Without one, the constructor's place in the tree does.
func (l *ReorderableWidget[T, K]) Key(k any) *ReorderableWidget[T, K] { l.id = k; return l }

// Len returns the number of rows.
func (l *ReorderableWidget[T, K]) Len() int { return l.list.Len() }

// Dragging reports whether a row is being dragged.
func (l *ReorderableWidget[T, K]) Dragging() bool { return l.st.dragging }

// HitID implements Identified.
func (l *ReorderableWidget[T, K]) HitID() any { return l.id }

// Adopt implements Adopter: a rebuilt list keeps its drag and animations.
func (l *ReorderableWidget[T, K]) Adopt(prev any) {
	if p, ok := prev.(*ReorderableWidget[T, K]); ok && p != l {
		l.st = p.st
		l.st.owner = l
	}
}

// HandlePointer implements PointerHandler. The list registers only to be
// adopted; its rows take the input.
func (l *ReorderableWidget[T, K]) HandlePointer(ggui.PointerEvent) bool { return false }

// Layout implements Widget.
func (l *ReorderableWidget[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer l.props.Layout().EndLayout()
	l.env, l.port = env, scrollPortOf(env)
	return l.list.Layout(c, env)
}

// Baseline implements Baseliner: the first row's that has one, as a
// Column's, so a heading beside the list lines up with its first row.
func (l *ReorderableWidget[T, K]) Baseline() (float64, bool) { return l.list.Baseline() }

func (l *ReorderableWidget[T, K]) gap() float64 { return l.list.gapFor(l.env) }

// index returns the row keyed k's index, or -1.
func (l *ReorderableWidget[T, K]) index(k K) int { return slices.Index(l.list.keys, k) }

// Paint implements Widget.
func (l *ReorderableWidget[T, K]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.HitPointer(r, l)
	f, st := l.list, l.st
	st.owner = l
	if len(f.items) == 0 && f.empty != nil {
		dst.Paint(f.empty, r)
		return
	}
	f.placeKey(r) // a ScrollController's ScrollToKey reaches these rows too
	n := len(f.children)
	now := ggui.FrameTime()
	dur := l.env.Motion(reorderDuration)
	d := -1
	if st.dragging {
		if d = l.index(st.key); d < 0 || d >= n {
			st.dragging, d = false, -1 // the row went away mid-drag
		}
	}
	// Where the lifted row is, and the slot it would drop into: the number
	// of other rows whose middle is above its middle.
	var lifted ggui.Rect
	slot, span := 0, 0.0
	if d >= 0 {
		size := f.rects[d].Size
		y := gem.Clamp(st.at.Y-st.grab.Y, r.Origin.Y, max(r.Origin.Y, r.Origin.Y+r.Size.H-size.H))
		lifted = ggui.Rct(ggui.Pt(r.Origin.X+f.rects[d].Origin.X, y), size)
		middle := y - r.Origin.Y + size.H/2
		for j := range n {
			if j != d && f.rects[j].Origin.Y+f.rects[j].Size.H/2 < middle {
				slot++
			}
		}
		st.to = gem.Pick(slot <= d, slot, slot+1)
		span = size.H + l.gap()
	}
	if st.settle != nil {
		// The rows were just moved: each eases from where it was drawn.
		for i, k := range f.keys[:n] {
			if was, ok := st.settle[k]; ok {
				m := new(ggui.Motion)
				m.MoveTo(was-f.rects[i].Origin.Y, now, 0)
				st.shift[k] = m
			}
		}
		st.settle = nil
	}
	// Rows move only around a drag and while they settle after a move; a
	// list at rest skips their motions and leaves painted empty for drawn.
	moving := d >= 0 || len(st.shift) > 0
	clear(st.painted)
	// The row holding focus takes the move keys: the one its middle is in,
	// as rows easing past each other overlap.
	focus, keyed := ggui.Point{}, false
	if fb, ok := dst.FocusBounds(); ok && !l.disabled {
		focus, keyed = fb.Center(), true
	}
	clip, clipped := dst.ClipBounds()
	dst.Node(r, listNode(n), func(dst *ggui.Canvas) {
		for i, child := range f.children {
			k := f.keys[i]
			if i == d {
				continue
			}
			dy := 0.0
			if moving {
				dy = l.shiftRow(k, i, d, slot, span, now, dur)
			}
			rc := ggui.Rct(r.Origin.Add(f.rects[i].Origin).Add(ggui.Pt(0, dy)), f.rects[i].Size)
			// Only where the row shows: one scrolled out of a Scroll's
			// window lies under whatever is drawn there instead.
			if keyed && rc.Contains(focus) && (!clipped || clip.Contains(focus)) {
				keyed = false
				row := child.(*reorderRow[T, K])
				dst.Shortcut(reorderUp, func() { row.moveBy(-1) })
				dst.Shortcut(reorderDown, func() { row.moveBy(1) })
			}
			if st.reveal != nil && *st.reveal == k {
				dst.RequestReveal(ggui.Rct(r.Origin.Add(f.rects[i].Origin), f.rects[i].Size))
				st.reveal = nil
			}
			dst.Group(f.childEntries[i], func() { l.paintRow(dst, child, rc, i, n) })
		}
	})
	for k := range st.shift {
		if _, ok := st.painted[k]; !ok && (d < 0 || k != st.key) {
			delete(st.shift, k) // a row that is gone
		}
	}
	if d < 0 {
		return
	}
	st.painted[st.key] = lifted.Origin.Y - r.Origin.Y
	row := f.children[d]
	token := reorderToken{st}
	dst.Overlay(func(o *ggui.Canvas) {
		// Catch the drop anywhere, so a release away from the list still
		// puts the row down; Escape alone cancels.
		o.HitPointer(ggui.Rct(ggui.Pt(-1e6, -1e6), ggui.Sz(2e6, 2e6)), reorderCatcher{token})
		o.Shadow(lifted, 6, ggui.ShadowStyle{Offset: ggui.Pt(0, 4), Blur: 12, Color: color.NRGBA{A: 72}})
		o.Inert().Paint(row, lifted)
	})
	l.autoScroll(now)
}

// shiftRow eases row i, keyed k, out of the way of the row lifted from d
// toward slot and returns how far it is moved. A row back in its place
// with no drag under way drops its motion.
func (l *ReorderableWidget[T, K]) shiftRow(k K, i, d, slot int, span float64, now time.Time, dur time.Duration) float64 {
	st := l.st
	target := 0.0
	switch {
	case d >= 0 && i < d && i >= slot:
		target = span
	case d >= 0 && i > d && i-1 < slot:
		target = -span
	}
	m := st.shift[k]
	if m == nil {
		m = new(ggui.Motion)
		m.MoveTo(0, now, 0) // from its place, which a row at rest is in
		st.shift[k] = m
	}
	m.MoveTo(target, now, dur)
	dy := m.Value(now)
	st.painted[k] = l.list.rects[i].Origin.Y + dy
	if d < 0 && dy == 0 && m.Target() == 0 {
		delete(st.shift, k)
	}
	return dy
}

// drawn returns where each row was drawn at the last paint, list-local:
// painted, or the rows' own places when they were at rest.
func (l *ReorderableWidget[T, K]) drawn() map[K]float64 {
	if len(l.st.painted) > 0 {
		return maps.Clone(l.st.painted)
	}
	f := l.list
	at := make(map[K]float64, len(f.children))
	for i, k := range f.keys[:len(f.children)] {
		at[k] = f.rects[i].Origin.Y
	}
	return at
}

// paintRow paints one row as a list item that knows its place and offers
// the move actions, which reach the row.
func (l *ReorderableWidget[T, K]) paintRow(dst *ggui.Canvas, w ggui.Widget, rc ggui.Rect, i, n int) {
	row := w.(*reorderRow[T, K])
	row.index, row.count = i, n
	dst.DescribeNode(rc, row, func(dst *ggui.Canvas) { dst.Paint(w, rc) })
}

// autoScroll scrolls the enclosing Scroll while the dragged row is held
// near one of its edges, faster the nearer it is.
func (l *ReorderableWidget[T, K]) autoScroll(now time.Time) {
	st, p := l.st, l.port
	dt := min(now.Sub(st.last).Seconds(), reorderMaxFrameS)
	st.last = now
	if p == nil || p.horizontal {
		return
	}
	win := p.rect
	edge := min(reorderEdge, win.Size.H/4)
	if edge <= 0 {
		return
	}
	v := 0.0
	switch y := st.at.Y; {
	case y < win.Origin.Y+edge:
		v = -min((win.Origin.Y+edge-y)/edge, 1)
	case y > win.Origin.Y+win.Size.H-edge:
		v = min((y-(win.Origin.Y+win.Size.H-edge))/edge, 1)
	}
	if v == 0 || (v < 0 && p.position() <= 0) || (v > 0 && p.position() >= p.maxOffset()) {
		return
	}
	ggui.Now() // keep scrolling while the row is held still
	if dt > 0 {
		p.scrollTo(p.position() + v*reorderMaxSpeed*dt)
	}
}

// move moves the row keyed k by delta places, as the keyboard does.
func (l *ReorderableWidget[T, K]) move(k K, delta int) bool {
	i := l.index(k)
	j := i + delta
	if l.disabled || l.st.dragging || i < 0 || j < 0 || j >= len(l.list.keys) {
		return false
	}
	l.commit(i, gem.Pick(delta < 0, j, j+1))
	l.st.reveal = &k
	return true
}

// commit writes the move and has every row ease from where it was drawn.
func (l *ReorderableWidget[T, K]) commit(from, to int) {
	st := l.st
	st.settle = l.drawn()
	if from == to || from+1 == to {
		return
	}
	l.items.Set(MoveItem(ggui.Peek(l.items), from, to))
	if l.onReorder != nil {
		l.onReorder(from, to)
	}
}

// reorderRow wraps a row so its subtree can find it through the Env, as a
// ReorderHandle does, and so the row itself can be dragged.
type reorderRow[T any, K comparable] struct {
	list  *ReorderableWidget[T, K]
	key   K
	child ggui.Widget
	rect  ggui.Rect

	index, count int // its place in the list, as last painted
}

// reorderTarget is the row a ReorderHandle drags.
type reorderTarget interface {
	ggui.Draggable
	moveBy(delta int) bool
	enabled() bool
	rowRect() ggui.Rect
}

var reorderRowKey = ggui.NewEnvKey[reorderTarget]("reorder row")

func (w *reorderRow[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return w.child.Layout(c, env.With(reorderRowKey, reorderTarget(w)))
}

func (w *reorderRow[T, K]) Baseline() (float64, bool) { return ggui.BaselineOf(w.child) }

func (w *reorderRow[T, K]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	w.rect = r
	if w.enabled() && !w.list.handles {
		dst.HitDrag(r, w)
	}
	dst.Paint(w.child, r)
}

// Describe implements Describer: a list item that knows its place and,
// unless the list is disabled, offers the move actions.
func (w *reorderRow[T, K]) Describe() ggui.Node {
	item := listItem(w.index, w.count)
	if !w.list.disabled {
		item.Actions = ggui.ActionIncrement | ggui.ActionDecrement
		item.Description = "Alt+Up or Alt+Down moves it"
	}
	return item
}

// Act implements Actor: an assistive technology moves the row down with
// increment and up with decrement.
func (w *reorderRow[T, K]) Act(a ggui.Action) bool { return reorderAct(w, a) }

// reorderAct moves t down for increment and up for decrement.
func reorderAct(t reorderTarget, a ggui.Action) bool {
	switch a.Kind {
	case ggui.ActionIncrement:
		return t.moveBy(1)
	case ggui.ActionDecrement:
		return t.moveBy(-1)
	}
	return false
}

func (w *reorderRow[T, K]) moveBy(delta int) bool { return w.list.move(w.key, delta) }
func (w *reorderRow[T, K]) enabled() bool         { return !w.list.disabled }
func (w *reorderRow[T, K]) rowRect() ggui.Rect    { return w.rect }

// DragValue implements Draggable: a token only the row's own list takes.
func (w *reorderRow[T, K]) DragValue() any { return reorderToken{w.list.st} }

// DragMoved implements Draggable: the first move lifts the row.
func (w *reorderRow[T, K]) DragMoved(at, grab ggui.Point) {
	st := w.list.st
	l := st.owner
	if l == nil {
		l = w.list
	}
	if !st.dragging {
		st.dragging, st.key, st.grab = true, w.key, grab
		st.last = ggui.FrameTime()
		st.to = l.index(w.key)
	}
	st.at = at
}

// DragEnded implements Draggable: a drop commits the move.
func (w *reorderRow[T, K]) DragEnded(dropped bool) {
	st := w.list.st
	l := st.owner
	if l == nil {
		l = w.list
	}
	if !st.dragging {
		return
	}
	st.dragging = false
	from := l.index(st.key)
	if !dropped || from < 0 {
		st.settle = l.drawn()
		return
	}
	l.commit(from, st.to)
}

// reorderCatcher is the drop zone a list's drag ends on.
type reorderCatcher struct{ token reorderToken }

func (c reorderCatcher) HandlePointer(ggui.PointerEvent) bool { return false }
func (c reorderCatcher) AcceptsDrop(v any) bool               { return v == c.token }
func (c reorderCatcher) DropHover(bool)                       {}
func (c reorderCatcher) DropOver(any, ggui.Point)             {}
func (c reorderCatcher) Drop(any, ggui.Point)                 {}

// ReorderHandleWidget is the grip a row of a Reorderable is dragged by.
// Build one with ReorderHandle.
type ReorderHandleWidget struct {
	ggui.Interactive
	child ggui.Widget
	row   reorderTarget
	ring  color.Color
	rect  ggui.Rect
}

// ReorderHandle makes child the grip its Reorderable row is dragged by,
// which with Handles is the only way to drag it. The handle is a button
// in the Tab order, named "Reorder" unless Name says otherwise, that Alt+Up
// and Alt+Down move. Outside a Reorderable it is child alone.
func ReorderHandle(child ggui.Widget) *ReorderHandleWidget {
	h := &ReorderHandleWidget{child: child}
	h.Role = ggui.RoleButton
	h.AutoKey()
	return h
}

// Name sets the accessible name.
func (h *ReorderHandleWidget) Name(v string) *ReorderHandleWidget { h.SetName(v); return h }

// BindName follows a non-nil accessible name reader.
func (h *ReorderHandleWidget) BindName(r ggui.Readable[string]) *ReorderHandleWidget {
	h.Interactive.BindName(r)
	return h
}

// Layout implements Widget.
func (h *ReorderHandleWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	h.Sync()
	h.row, _ = env.Get(reorderRowKey)
	h.ring = env.Text().Color
	return h.child.Layout(c, env)
}

// Paint implements Widget.
func (h *ReorderHandleWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	h.rect = r
	if h.row == nil {
		dst.Paint(h.child, r)
		return
	}
	if h.row.enabled() && !h.IsInert() {
		dst.HitDrag(r, reorderHandleSource{h})
	}
	h.Hit(dst, r, h, ggui.CursorShapeMove)
	dst.Paint(h.child, r)
	h.FocusRing(dst, r, 4, h.ring)
}

// HandlePointer implements PointerHandler.
func (h *ReorderHandleWidget) HandlePointer(ev ggui.PointerEvent) bool { return h.Pointer(ev, nil) }

// HandleKey implements KeyHandler.
func (h *ReorderHandleWidget) HandleKey(ev ggui.KeyEvent) { h.Keyboard(ev, nil) }

// ConsumesKey implements KeyConsumer: the handle presses nothing.
func (h *ReorderHandleWidget) ConsumesKey(ggui.KeyEvent) bool { return false }

// Describe implements Describer.
func (h *ReorderHandleWidget) Describe() ggui.Node {
	n := ggui.Node{Role: ggui.RoleButton, Name: h.name(), Disabled: h.IsInert() || h.row == nil || !h.row.enabled()}
	if !n.Disabled {
		n.Actions = ggui.ActionFocus | ggui.ActionIncrement | ggui.ActionDecrement
		n.Description = "Alt+Up or Alt+Down moves the row"
	}
	return n
}

// Act implements Actor: increment moves the row down, decrement up.
func (h *ReorderHandleWidget) Act(a ggui.Action) bool {
	if h.row == nil || h.IsInert() {
		return false
	}
	return reorderAct(h.row, a)
}

// Semantics implements Semantic.
func (h *ReorderHandleWidget) Semantics() (ggui.Role, string) { return ggui.RoleButton, h.name() }

// name is the accessible name, "Reorder" unless Name says otherwise.
func (h *ReorderHandleWidget) name() string {
	if name := h.SemanticName(); name != "" {
		return name
	}
	return "Reorder"
}

// reorderHandleSource drags the handle's row, holding it where the
// pointer took the handle.
type reorderHandleSource struct{ h *ReorderHandleWidget }

func (s reorderHandleSource) DragValue() any { return s.h.row.DragValue() }
func (s reorderHandleSource) DragMoved(at, grab ggui.Point) {
	s.h.row.DragMoved(at, grab.Add(s.h.rect.Origin.Sub(s.h.row.rowRect().Origin)))
}
func (s reorderHandleSource) DragEnded(dropped bool) { s.h.row.DragEnded(dropped) }
