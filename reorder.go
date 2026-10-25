package ggui

import (
	"image/color"
	"maps"
	"slices"
	"time"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
)

const (
	reorderDuration  = 200 * time.Millisecond
	reorderEdge      = 48.0  // how near a Scroll's edge a drag starts scrolling it
	reorderMaxSpeed  = 900.0 // logical pixels per second, at the very edge
	reorderMaxFrameS = 0.05
)

var (
	reorderUp   = MustChord("alt+up")
	reorderDown = MustChord("alt+down")
)

// ReorderableWidget is a keyed list the user rearranges by dragging rows,
// with Alt+Up and Alt+Down, or through the move actions an assistive
// technology offers. Build one with Reorderable.
type ReorderableWidget[T any, K comparable] struct {
	props     property.Owner
	list      *EachWidget[T, K]
	items     Binding[[]T]
	onReorder func(from, to int)
	handles   bool
	disabled  bool
	id        any

	st   *reorderState[K]
	port *ScrollWidget
	env  Env
}

// reorderState is a list's drag and its row animations; a rebuilt list
// adopts it, so a drag carries on.
type reorderState[K comparable] struct {
	dragging bool
	key      K     // the row being dragged
	at, grab Point // the pointer, and where on the row it took hold
	to       int   // where the row would drop, counted as Move counts
	last     time.Time

	shift   map[K]*Motion // how far each row is moved out of the dragged row's way
	painted map[K]float64 // where each row was drawn at the last paint, list-local
	settle  map[K]float64 // where rows were drawn before a move, to ease from
	reveal  *K            // a row moved by the keyboard, to keep in view
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
//	ggui.Reorderable(tasks, func(t Task) int { return t.ID }, func(it ggui.EachItem[Task]) ggui.Widget {
//		return ggui.Row(ggui.ReorderHandle(ui.Icon(icons.Grip)), ggui.Text(it.Item.Title)).Gap(8)
//	}).Handles().Gap(4)
//
// Rows lay out vertically and are all laid out, as EachKeyed's are
// without ItemExtent.
func Reorderable[T any, K comparable](items Binding[[]T], key func(T) K, build func(EachItem[T]) Widget) *ReorderableWidget[T, K] {
	property.Require(items, "Reorderable")
	l := &ReorderableWidget[T, K]{items: items, id: reactive.AutoID(), st: &reorderState[K]{
		shift: map[K]*Motion{}, painted: map[K]float64{},
	}}
	l.list = EachKeyed(items, key, func(it EachItem[T]) Widget {
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
	}
}

// HandlePointer implements PointerHandler. The list registers only to be
// adopted; its rows take the input.
func (l *ReorderableWidget[T, K]) HandlePointer(PointerEvent) bool { return false }

// Layout implements Widget.
func (l *ReorderableWidget[T, K]) Layout(c Constraints, env Env) Size {
	defer l.props.Layout()()
	l.env, l.port = env, scrollPortOf(env)
	return l.list.Layout(c, env)
}

func (l *ReorderableWidget[T, K]) gap() float64 { return l.list.gapFor(l.env) }

// index returns the row keyed k's index, or -1.
func (l *ReorderableWidget[T, K]) index(k K) int { return slices.Index(l.list.keys, k) }

// Paint implements Widget.
func (l *ReorderableWidget[T, K]) Paint(dst *Canvas, r Rect) {
	f, st := l.list, l.st
	dst.HitPointer(r, l)
	if len(f.items) == 0 && f.empty != nil {
		dst.Paint(f.empty, r)
		return
	}
	f.placeKey(r) // a ScrollController's ScrollToKey reaches these rows too
	n := len(f.children)
	now := FrameTime()
	dur := l.env.Motion(reorderDuration)
	d := -1
	if st.dragging {
		if d = l.index(st.key); d < 0 || d >= n {
			st.dragging, d = false, -1 // the row went away mid-drag
		}
	}
	// Where the lifted row is, and the slot it would drop into: the number
	// of other rows whose middle is above its middle.
	var lifted Rect
	slot, span := 0, 0.0
	if d >= 0 {
		size := f.sizes[d]
		y := clamp(st.at.Y-st.grab.Y, r.Origin.Y, max(r.Origin.Y, r.Origin.Y+r.Size.H-size.H))
		lifted = Rct(Pt(r.Origin.X+f.offsets[d].X, y), size)
		middle := y - r.Origin.Y + size.H/2
		for j := range n {
			if j != d && f.offsets[j].Y+f.sizes[j].H/2 < middle {
				slot++
			}
		}
		st.to = pick(slot <= d, slot, slot+1)
		span = size.H + l.gap()
	}
	if st.settle != nil {
		// The rows were just moved: each eases from where it was drawn.
		for i, k := range f.keys[:n] {
			if was, ok := st.settle[k]; ok {
				m := new(Motion)
				m.MoveTo(was-f.offsets[i].Y, now, 0)
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
	focus, keyed := Point{}, false
	if dst != nil && !l.disabled {
		if fb := dst.fs().focusBounds; !fb.Empty() {
			focus, keyed = dst.space.local(fb.Center()), true
		}
	}
	dst.Node(r, Node{Role: RoleList, Min: 1, Max: float64(n)}, func(dst *Canvas) {
		for i, child := range f.children {
			k := f.keys[i]
			if i == d {
				continue
			}
			dy := 0.0
			if moving {
				dy = l.shiftRow(k, i, d, slot, span, now, dur)
			}
			rc := Rct(r.Origin.Add(f.offsets[i]).Add(Pt(0, dy)), f.sizes[i])
			// Only where the row shows: one scrolled out of a Scroll's
			// window lies under whatever is drawn there instead.
			if keyed && rc.Contains(focus) && (!dst.clipped || dst.clip.Contains(focus)) {
				keyed = false
				row := child.(*reorderRow[T, K])
				dst.Shortcut(reorderUp, func() { row.moveBy(-1) })
				dst.Shortcut(reorderDown, func() { row.moveBy(1) })
			}
			if st.reveal != nil && *st.reveal == k {
				dst.RequestReveal(Rct(r.Origin.Add(f.offsets[i]), f.sizes[i]))
				st.reveal = nil
			}
			dst.inGroup(f.childEntries[i], func() { l.paintRow(dst, child, rc, i, n) })
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
	dst.Overlay(func(o *Canvas) {
		// Catch the drop anywhere, so a release away from the list still
		// puts the row down; Escape alone cancels.
		o.HitPointer(Rct(Pt(-1e6, -1e6), Sz(2e6, 2e6)), reorderCatcher{token})
		o.Shadow(lifted, 6, ShadowStyle{Offset: Pt(0, 4), Blur: 12, Color: color.NRGBA{A: 72}})
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
		m = new(Motion)
		m.MoveTo(0, now, 0) // from its place, which a row at rest is in
		st.shift[k] = m
	}
	m.MoveTo(target, now, dur)
	dy := m.Value(now)
	st.painted[k] = l.list.offsets[i].Y + dy
	if d < 0 && dy == 0 && m.to == 0 {
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
		at[k] = f.offsets[i].Y
	}
	return at
}

// paintRow paints one row as a list item that knows its place and offers
// the move actions.
func (l *ReorderableWidget[T, K]) paintRow(dst *Canvas, w Widget, rc Rect, i, n int) {
	item := Node{Role: RoleListItem, Min: 1, Now: float64(i + 1), Max: float64(n)}
	if !l.disabled {
		item.Actions = ActionIncrement | ActionDecrement
		item.Description = "Alt+Up or Alt+Down moves it"
	}
	dst.scoped(dst.addSem(rc, item, w), func(dst *Canvas) { dst.Paint(w, rc) })
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
	activeWorld().frame.animate() // keep scrolling while the row is held still
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
	l.commit(i, pick(delta < 0, j, j+1))
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
	l.items.Set(Move(Peek(l.items), from, to))
	if l.onReorder != nil {
		l.onReorder(from, to)
	}
}

// reorderRow wraps a row so its subtree can find it through the Env, as a
// ReorderHandle does, and so the row itself can be dragged.
type reorderRow[T any, K comparable] struct {
	list  *ReorderableWidget[T, K]
	key   K
	child Widget
	rect  Rect
}

// reorderTarget is the row a ReorderHandle drags.
type reorderTarget interface {
	dragSource
	moveBy(delta int) bool
	enabled() bool
	rowRect() Rect
}

var reorderRowKey = NewEnvKey[reorderTarget]("reorder row")

func (w *reorderRow[T, K]) Layout(c Constraints, env Env) Size {
	return w.child.Layout(c, env.With(reorderRowKey, reorderTarget(w)))
}

func (w *reorderRow[T, K]) Baseline() (float64, bool) { return baselineOf(w.child) }

func (w *reorderRow[T, K]) Paint(dst *Canvas, r Rect) {
	w.rect = r
	if w.enabled() && !w.list.handles {
		dst.add(dst.region(r, nil, hitRegion{drag: w}))
	}
	dst.Paint(w.child, r)
}

// Act implements Actor: an assistive technology moves the row down with
// increment and up with decrement.
func (w *reorderRow[T, K]) Act(a Action) bool { return reorderAct(w, a) }

// reorderAct moves t down for increment and up for decrement.
func reorderAct(t reorderTarget, a Action) bool {
	switch a.Kind {
	case ActionIncrement:
		return t.moveBy(1)
	case ActionDecrement:
		return t.moveBy(-1)
	}
	return false
}

func (w *reorderRow[T, K]) moveBy(delta int) bool { return w.list.move(w.key, delta) }
func (w *reorderRow[T, K]) enabled() bool         { return !w.list.disabled }
func (w *reorderRow[T, K]) rowRect() Rect         { return w.rect }

func (w *reorderRow[T, K]) dragValue() any { return reorderToken{w.list.st} }

func (w *reorderRow[T, K]) dragMoved(at, grab Point) {
	st := w.list.st
	if !st.dragging {
		st.dragging, st.key, st.grab = true, w.key, grab
		st.last = FrameTime()
		st.to = w.list.index(w.key)
	}
	st.at = at
}

func (w *reorderRow[T, K]) dragEnded(dropped bool) {
	l := w.list
	st := l.st
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

func (c reorderCatcher) HandlePointer(PointerEvent) bool { return false }
func (c reorderCatcher) acceptsDrag(v any) bool          { return v == c.token }
func (c reorderCatcher) dragOver(bool)                   {}
func (c reorderCatcher) dragMoved(any, Point)            {}
func (c reorderCatcher) dropped(any, Point)              {}

// ReorderHandleWidget is the grip a row of a Reorderable is dragged by.
// Build one with ReorderHandle.
type ReorderHandleWidget struct {
	Interactive
	child Widget
	row   reorderTarget
	ring  color.Color
	rect  Rect
}

// ReorderHandle makes child the grip its Reorderable row is dragged by,
// which with Handles is the only way to drag it. The handle is a button
// in the Tab order, named "Reorder" unless Name says otherwise, that Alt+Up
// and Alt+Down move. Outside a Reorderable it is child alone.
func ReorderHandle(child Widget) *ReorderHandleWidget {
	h := &ReorderHandleWidget{child: child}
	h.Role = RoleButton
	h.AutoKey()
	return h
}

// Name sets the accessible name.
func (h *ReorderHandleWidget) Name(v string) *ReorderHandleWidget { h.SetName(v); return h }

// BindName follows a non-nil accessible name reader.
func (h *ReorderHandleWidget) BindName(r Readable[string]) *ReorderHandleWidget {
	h.Interactive.BindName(r)
	return h
}

// Layout implements Widget.
func (h *ReorderHandleWidget) Layout(c Constraints, env Env) Size {
	h.Sync()
	h.row, _ = env.Get(reorderRowKey)
	h.ring = env.Text().Color
	return h.child.Layout(c, env)
}

// Paint implements Widget.
func (h *ReorderHandleWidget) Paint(dst *Canvas, r Rect) {
	h.rect = r
	if h.row == nil {
		dst.Paint(h.child, r)
		return
	}
	if h.row.enabled() && !h.IsInert() {
		dst.add(dst.region(r, nil, hitRegion{drag: reorderHandleSource{h}}))
	}
	h.Hit(dst, r, h, CursorShapeMove)
	dst.Paint(h.child, r)
	h.FocusRing(dst, r, 4, h.ring)
}

// HandlePointer implements PointerHandler.
func (h *ReorderHandleWidget) HandlePointer(ev PointerEvent) bool { return h.Pointer(ev, nil) }

// HandleKey implements KeyHandler.
func (h *ReorderHandleWidget) HandleKey(ev KeyEvent) { h.Keyboard(ev, nil) }

// ConsumesKey implements KeyConsumer: the handle presses nothing.
func (h *ReorderHandleWidget) ConsumesKey(KeyEvent) bool { return false }

// Describe implements Describer.
func (h *ReorderHandleWidget) Describe() Node {
	n := Node{Role: RoleButton, Name: h.name(), Disabled: h.IsInert() || h.row == nil || !h.row.enabled()}
	if !n.Disabled {
		n.Actions = ActionFocus | ActionIncrement | ActionDecrement
		n.Description = "Alt+Up or Alt+Down moves the row"
	}
	return n
}

// Act implements Actor: increment moves the row down, decrement up.
func (h *ReorderHandleWidget) Act(a Action) bool {
	if h.row == nil || h.IsInert() {
		return false
	}
	return reorderAct(h.row, a)
}

// Semantics implements Semantic.
func (h *ReorderHandleWidget) Semantics() (Role, string) { return RoleButton, h.name() }

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

func (s reorderHandleSource) dragValue() any { return s.h.row.dragValue() }
func (s reorderHandleSource) dragMoved(at, grab Point) {
	s.h.row.dragMoved(at, grab.Add(s.h.rect.Origin.Sub(s.h.row.rowRect().Origin)))
}
func (s reorderHandleSource) dragEnded(dropped bool) { s.h.row.dragEnded(dropped) }
