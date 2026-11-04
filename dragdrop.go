package ggui

import (
	"image/color"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
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
	dragMoved(v any, at Point) // the drag is over the target, at a point of its own
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
	r := in.topmost(func(r *hitRegion) bool { return r.drag != nil && r.contains(at) })
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
		dx := f.pos.Sub(d.from)
		if released || dx.X*dx.X+dx.Y*dx.Y < dragThreshold*dragThreshold {
			if released {
				in.drag = nil
			}
			return
		}
		src := d.candidate.drag
		d.source, d.value = src, src.dragValue()
		in.releasePress(f.pos, f.mods)
	}
	d.source.dragMoved(d.candidate.at(f.pos), d.candidate.offset(d.from))
	now := in.topmost(func(r *hitRegion) bool {
		t, ok := r.pointer.(dropTarget)
		return ok && r.contains(f.pos) && t.acceptsDrag(d.value)
	})
	if !in.continues(now, d.target) {
		if d.target != nil {
			d.target.pointer.(dropTarget).dragOver(false)
		}
		if now != nil {
			now.pointer.(dropTarget).dragOver(true)
		}
	}
	d.target = keep(now)
	if now != nil && !released {
		now.pointer.(dropTarget).dragMoved(d.value, now.offset(f.pos))
	}
	if !released {
		return
	}
	dropped := false
	if now != nil {
		t := now.pointer.(dropTarget)
		t.dragOver(false)
		t.dropped(d.value, now.offset(f.pos))
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
	defer d.props.Layout().EndLayout()
	d.size = d.child.Layout(c, env)
	// The preview is only painted mid-drag, so only then is it laid out.
	if d.preview != nil && d.dragging {
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
		d.props.Changed() // lay out the preview
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
	onOver  func(Dropped[T])
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

// OnOver runs as a drag the zone accepts moves over it, with where it would
// land: to show where in the zone the value would go, as a gap opening
// between tabs or rows. OnHover with false says the drag has left.
func (z *DropZoneWidget[T]) OnOver(fn func(Dropped[T])) *DropZoneWidget[T] { z.onOver = fn; return z }

// Over reports whether a drag the zone accepts is over it.
func (z *DropZoneWidget[T]) Over() bool { return z.over }

// Layout implements Widget.
func (z *DropZoneWidget[T]) Layout(c Constraints, env Env) Size {
	defer z.props.Layout().EndLayout()
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

func (z *DropZoneWidget[T]) dragMoved(v any, at Point) {
	if z.onOver != nil {
		z.onOver(Dropped[T]{Value: v.(T), Pos: at, Size: z.size})
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

// Files dropped onto the window from the desktop. The platform reports a
// drop once, at the cursor's position, with a file system holding what was
// dropped; ggui routes it like a pointer event, to the topmost region under
// the cursor that accepts drops, and otherwise to the handlers registered
// with Host.OnDrop.
//
// While files are still being dragged, the zone under the cursor hears of
// it through DragHandler, so that it can highlight. ggfx reports only
// the drop, so ggui follows the drag on the platform's own drag session
// where it can: macOS today. Elsewhere a zone learns of a drag when it
// lands, and its drop handler still runs.

// DroppedFile is one file or directory that was dropped onto the window.
type DroppedFile struct {
	// Name is the file's base name, as it appears in the file system the
	// platform handed over.
	Name string
	// Path is the absolute path in the real file system, or empty where the
	// platform provides none, which is the case in a browser.
	Path string
	// Dir reports whether a directory was dropped rather than a file.
	Dir bool

	fsys fs.FS
}

// Open reads the dropped file, wherever the platform keeps it. It is the
// one way to read a drop that has no Path.
func (f DroppedFile) Open() (fs.File, error) {
	if f.fsys == nil {
		return nil, &fs.PathError{Op: "open", Path: f.Name, Err: fs.ErrNotExist}
	}
	return f.fsys.Open(f.Name)
}

// DropEvent is a drop of files onto the window.
type DropEvent struct {
	Pos   Point // where the handler painted: window coordinates, unless under a Transform
	Files []DroppedFile
}

// Paths returns the real file system path of every dropped file that has
// one, in the order they were dropped.
func (ev DropEvent) Paths() []string {
	var paths []string
	for _, f := range ev.Files {
		if f.Path != "" {
			paths = append(paths, f.Path)
		}
	}
	return paths
}

// DropHandler is a PointerHandler that also accepts dropped files. A drop
// goes to the topmost region under the cursor whose handler implements it
// and returns true; a region that returns false lets the drop fall through
// to what is beneath, and then to the host's handlers.
type DropHandler interface {
	HandleDrop(DropEvent) bool
}

// DragKind is what a DragEvent reports: files over the zone, or gone.
type DragKind uint8

const (
	// DragOver is sent every frame files are dragged over the zone.
	DragOver DragKind = iota
	// DragExit is sent once when they leave it, or are dropped or let go.
	DragExit
)

// DragEvent is files being dragged over the window, before any drop.
type DragEvent struct {
	Kind DragKind
	Pos  Point // where the handler painted: window coordinates, unless under a Transform
}

// DragHandler is a DropHandler that also follows the drag before the drop.
// Each frame files are over the window, the topmost region under the cursor
// whose handler returns true from HandleDrag for a DragOver is the drag's
// target; it receives a DragExit when the cursor moves off it or the drag
// ends. A handler that returns false for DragOver is passed over. A drop
// zone should claim the drag whether or not it shows it, as PointerWidget
// does, so that the zone which highlights is the one the drop reaches.
type DragHandler interface {
	HandleDrag(DragEvent) bool
}

// absPather is what a dropped entry implements where the platform knows
// the file's real path; ggfx names it AbsPather.
type absPather interface {
	AbsPath() string
}

// droppedFiles reads the root of the file system a drop handed over into
// the files a DropEvent carries.
func droppedFiles(fsys fs.FS) []DroppedFile {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	files := make([]DroppedFile, 0, len(entries))
	for _, e := range entries {
		f := DroppedFile{Name: e.Name(), Dir: e.IsDir(), fsys: fsys}
		if p, ok := e.(absPather); ok {
			f.Path = p.AbsPath()
		}
		files = append(files, f)
	}
	return files
}

// droppedPaths makes the files a drop of real paths would carry, for a
// Probe. Each file reads from its own directory, so the paths need not
// share one.
func droppedPaths(paths []string) []DroppedFile {
	files := make([]DroppedFile, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		info, err := os.Stat(abs)
		files = append(files, DroppedFile{
			Name: filepath.Base(abs),
			Path: abs,
			Dir:  err == nil && info.IsDir(),
			fsys: os.DirFS(filepath.Dir(abs)),
		})
	}
	return files
}

// dispatchDrop routes a frame's drop: to the topmost accepting region under
// the cursor, else to every handler the host registered.
func (in *inputState) dispatchDrop(f frameInput) {
	ev := DropEvent{Pos: f.pos, Files: f.drop}
	taken := in.topmost(func(r *hitRegion) bool {
		h, ok := r.pointer.(DropHandler)
		return ok && r.contains(ev.Pos) && h.HandleDrop(DropEvent{Pos: r.at(ev.Pos), Files: ev.Files})
	})
	if taken != nil {
		return
	}
	for _, fn := range in.drops {
		fn(ev)
	}
}

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
	defer l.props.Layout().EndLayout()
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
		size := f.rects[d].Size
		y := clamp(st.at.Y-st.grab.Y, r.Origin.Y, max(r.Origin.Y, r.Origin.Y+r.Size.H-size.H))
		lifted = Rct(Pt(r.Origin.X+f.rects[d].Origin.X, y), size)
		middle := y - r.Origin.Y + size.H/2
		for j := range n {
			if j != d && f.rects[j].Origin.Y+f.rects[j].Size.H/2 < middle {
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
			rc := Rct(r.Origin.Add(f.rects[i].Origin).Add(Pt(0, dy)), f.rects[i].Size)
			// Only where the row shows: one scrolled out of a Scroll's
			// window lies under whatever is drawn there instead.
			if keyed && rc.Contains(focus) && (!dst.clipped || dst.clip.Contains(focus)) {
				keyed = false
				row := child.(*reorderRow[T, K])
				dst.Shortcut(reorderUp, func() { row.moveBy(-1) })
				dst.Shortcut(reorderDown, func() { row.moveBy(1) })
			}
			if st.reveal != nil && *st.reveal == k {
				dst.RequestReveal(Rct(r.Origin.Add(f.rects[i].Origin), f.rects[i].Size))
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
	st.painted[k] = l.list.rects[i].Origin.Y + dy
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
		at[k] = f.rects[i].Origin.Y
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
