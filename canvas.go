package ggui

import (
	"github.com/ironpark/ggui/inspect"
	"image"
	"image/color"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Canvas is what a Widget paints into: the target image plus the frame's list
// of interactive regions. Widgets that react to input register the Rect they
// painted with HitPointer or HitKey; the runtime dispatches the next frame's
// events to those regions, topmost (last painted) first. A nil Canvas paints
// nothing and collects nothing, which is what layout tests want.
type Canvas struct {
	Image          *ebiten.Image
	focusRequest   KeyHandler
	inputObservers []inputObserver

	scale   float64
	hits    []hitRegion
	prev    []hitRegion // last frame's regions, for Adopter handoff
	parent  *Canvas     // set on a Clip; hit regions go to the root
	clip    Rect
	clipped bool
	inert   bool        // registers no hit regions: a widget on its way out
	scope   *focusScope // the focus trap regions are registered under, if any
	group   any         // what regions painted now belong to, for EachKeyed's eviction

	// frame is the state there is one of per frame rather than one per
	// Canvas: it is reached through root, and only the root holds it. A
	// Clip or an Inert child leaves this nil, which is what makes "root
	// only" a property of the type rather than a rule to remember.
	frame *frameState
}

// frameState is everything a frame accumulates once: what was painted, what
// it retained, and the accessibility tree built alongside. Every Canvas in a
// Clip chain reaches the same one through root, so a field added here is
// shared by construction and needs no clearing when a child Canvas is made.
type frameState struct {
	logical      Size // the window in logical pixels, for Size
	pointer      Point
	hasPointer   bool
	focusBounds  Rect // focused input region at the start of this paint
	overlays     []overlay
	trace        []inspect.Node // every Paint call, when the inspector is on
	traceWidgets []Widget       // the widget behind each trace node
	tracing      bool
	depth        int
	traceParent  int // index plus one of the widget currently painting
	traceRoots   int
	keeps        map[retainKey]any // Retain this frame
	prevKeeps    map[retainKey]any // Retain last frame, read by Retained

	// Where last frame's regions are, for adopt. Built on the frame's
	// first handoff and not at all in a frame with none, which is most
	// of them: only a handful of widgets adopt.
	prevByID    map[any]int  // id to its place in prev, the last one painted
	prevByRect  map[Rect]int // the same for a region with no id
	prevIndexed bool

	// The frame's accessibility tree, kept apart from hits: input scans
	// hits on every pointer event, and most of what a screen reader reads
	// takes no input at all. semParent and semLast are indices into sem
	// plus one, so a zero value means none.
	sem       []semNode
	semIndex  map[any]int // handler to index plus one, for Describe and SemanticRef
	semParent int         // the node being painted into
	semLast   int         // the node most recently recorded
}

// fs returns the frame state, creating it on the root's first use. A Canvas
// with no parent is a root even before a frame has started, which is what a
// Probe and the zero Canvas in App rely on.
func (c *Canvas) fs() *frameState {
	r := c.root()
	if r.frame == nil {
		r.frame = &frameState{}
	}
	return r.frame
}

// overlay is one deferred paint and the semantics node it belongs under.
type overlay struct {
	fn    func(*Canvas)
	owner SemRef
}

// Slot names one value a widget retains on the Canvas between frames,
// typed like an EnvKey. Make one per animated value or timer with
// NewSlot, as a package variable.
type Slot[T any] struct {
	id   *byte
	name string
}

// NewSlot creates a distinct Slot; name is for messages only.
func NewSlot[T any](name string) Slot[T] { return Slot[T]{id: new(byte), name: name} }

// Anchor is what retained state is attached to: the widget's ID when it
// has one, so the state follows it wherever it moves, or else the Rect it
// painted.
type Anchor struct {
	Rect Rect
	ID   any
}

// retainKey identifies a Retain slot: the anchor and the slot.
type retainKey struct {
	rect Rect
	id   any
	slot *byte
}

func (a Anchor) key(slot *byte) retainKey {
	if a.ID != nil {
		return retainKey{id: a.ID, slot: slot}
	}
	return retainKey{rect: a.Rect, slot: slot}
}

// Retain stores v for the next frame under at and s, and Retained returns
// what was stored under the same pair last frame. It is how a widget
// rebuilt every frame keeps state that has no signal: a ui.Tooltip's hover
// timer, a Transition's start time. A slot nobody retains again is
// dropped, so state goes away with the widget.
func (c *Canvas) Retain[T any](at Anchor, s Slot[T], v T) {
	if c == nil {
		return
	}
	f := c.fs()
	if f.keeps == nil {
		f.keeps = make(map[retainKey]any)
	}
	f.keeps[at.key(s.id)] = v
}

// Retained returns what Retain stored under at and s last frame.
func (c *Canvas) Retained[T any](at Anchor, s Slot[T]) (T, bool) {
	if c == nil {
		var zero T
		return zero, false
	}
	v, ok := c.fs().prevKeeps[at.key(s.id)].(T)
	return v, ok
}

// Ease moves a Motion retained under at and s toward target over d and
// returns where it is now, so a control rebuilt every frame keeps
// animating; a switch knob and a tab underline use it.
func (c *Canvas) Ease(at Anchor, s Slot[*Motion], target float64, d time.Duration) float64 {
	now := Now()
	m, ok := c.Retained(at, s)
	if !ok {
		m = new(Motion)
	}
	m.MoveTo(target, now, d)
	c.Retain(at, s, m)
	return m.Value(now)
}

// nextFrame moves this frame's retained values to last frame's place and
// clears the current slots, ready for a paint.
func (c *Canvas) nextFrame() {
	c.inputObservers = c.inputObservers[:0]
	f := c.fs()
	f.prevKeeps, f.keeps = f.keeps, f.prevKeeps
	clear(f.keeps)
	// prev is a different slice now, so what adopt knew about it is stale.
	// The maps keep their storage for the next frame that needs them.
	f.prevIndexed = false
	clear(f.prevByID)
	clear(f.prevByRect)
	f.traceParent, f.traceRoots = 0, 0
	rotateEnvMemo()
}

// Inert returns a Canvas that draws where c does but registers no hit
// regions, for a widget that is leaving and must not take input.
func (c *Canvas) Inert() *Canvas {
	if c == nil {
		return nil
	}
	// Only the root holds frameState, so the copy's parent link is all it
	// takes to leave the frame's overlays, trace, retained slots and
	// semantics where they are: there is nothing here to clear.
	child := *c
	child.parent, child.inert, child.frame = c, true, nil
	return &child
}

// frameTrace is every widget painted this frame, in paint order; the
// inspector reads it. frameSem is the same for the accessibility tree.
// inspectComparable keeps a widget id only when it can be compared, so an
// inspect.Node stays usable as a map key however a widget was identified.
func inspectComparable(v any) any {
	if v != nil && reflect.ValueOf(v).Comparable() {
		return v
	}
	return nil
}

// root returns the Canvas a Clip chain started from.
func (c *Canvas) root() *Canvas {
	for c.parent != nil {
		c = c.parent
	}
	return c
}

// Paint paints w into r. Containers paint their children through it rather
// than calling w.Paint directly, so the inspector can show every widget's
// Rect. A nil Canvas paints w with a nil Canvas.
func (c *Canvas) Paint(w Widget, r Rect) {
	if c == nil {
		w.Paint(nil, r)
		return
	}
	// The trace exists only for the inspector, so an app with no panel and
	// no OnInspect handler skips the frameState walk on every painted
	// widget for one test of a package flag.
	if inspectorEnabled {
		f := c.fs()
		if f.tracing {
			parent := f.traceParent
			path := "/" + strconv.Itoa(f.traceRoots)
			if parent > 0 {
				p := &f.trace[parent-1]
				path = p.Path + "/" + strconv.Itoa(p.Children)
				p.Children++
			} else {
				f.traceRoots++
			}
			n := inspect.Node{
				Rect: r, Depth: f.depth, Name: widgetName(w), Kind: inspectKind(w),
				ID: inspectComparable(idOf(w)), Instance: inspectComparable(w),
				Path: path, Clip: c.clip, Clipped: c.clipped,
			}
			f.trace = append(f.trace, n)
			f.traceWidgets = append(f.traceWidgets, w)
			f.traceParent = len(f.trace)
			f.depth++
			defer func() { f.depth--; f.traceParent = parent }()
		}
	}
	w.Paint(c, r)
}

var widgetNames sync.Map // reflect.Type -> string; names never change

// widgetName is a widget's type for the inspector: Box for *ggui.BoxWidget.
func widgetName(w Widget) string {
	t := reflect.TypeOf(w)
	if name, ok := widgetNames.Load(t); ok {
		return name.(string)
	}
	original := t
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := t.Name()
	if i := strings.Index(name, "["); i >= 0 {
		name = name[:i]
	}
	name = strings.TrimSuffix(name, "Widget")
	widgetNames.Store(original, name)
	return name
}

// Pointer returns where the mouse cursor was when this frame began, in
// logical pixels, and whether it is known. Widgets that react to hovering
// without a hit region, such as ui.Tooltip, read it in Paint.
func (c *Canvas) Pointer() (Point, bool) {
	if c == nil {
		return Point{}, false
	}
	f := c.fs()
	return f.pointer, f.hasPointer
}

// FocusWithin reports whether the focused input region overlaps r. Containers
// use it to pause motion while a descendant (including a custom control) has
// the keyboard. Like Pointer, it reads the state at the start of the paint.
func (c *Canvas) FocusWithin(r Rect) bool {
	return c != nil && !c.fs().focusBounds.Intersect(r).Empty()
}

// Overlay schedules fn to paint after the whole tree has, on the root
// Canvas, unclipped and above everything: tooltips, popups and menus go
// there. Hit regions fn registers sit on top of the tree's.
//
// An overlay paints long after its widget did, so what it describes would
// otherwise stand beside the whole tree rather than inside it. Pass the
// owner a SemanticRef gave to attach it where it belongs: a dropdown's
// option list under the combobox that opened it.
func (c *Canvas) Overlay(fn func(dst *Canvas), owner ...SemRef) {
	if c == nil {
		return
	}
	var under SemRef
	if len(owner) > 0 {
		under = owner[0]
	}
	f := c.fs()
	f.overlays = append(f.overlays, overlay{fn: fn, owner: under})
}

// Size returns the logical size of the window being painted, or zero for
// a Canvas that was given none.
func (c *Canvas) Size() Size {
	if c == nil {
		return Size{}
	}
	root := c.root()
	if f := c.fs(); f.logical != (Size{}) || root.Image == nil {
		return f.logical
	}
	b := root.Image.Bounds()
	return Sz(c.dp(float64(b.Dx())), c.dp(float64(b.Dy())))
}

// paintOverlays runs the overlays queued this frame, including ones they
// queue themselves, and clears the queue.
func (c *Canvas) paintOverlays() {
	f := c.fs()
	for i := 0; i < len(f.overlays); i++ {
		o := f.overlays[i]
		c.scoped(o.owner, o.fn)
	}
	f.overlays = f.overlays[:0]
}

// Adopter is a handler that can take over from the handler that held the
// same region in the previous frame: the one with the same ID, or else the
// one at the same Rect. When a rebuild replaces a widget, the new one is
// handed the old one as it registers its region, so hover, press, caret or
// an animation in flight carry across the rebuild instead of resetting.
// prev is the previous PointerHandler or KeyHandler; check its type and
// copy what applies.
type Adopter interface {
	Adopt(prev any)
}

// Identified is a handler with an identity of its own. A region it
// registers is matched to last frame's by that ID before its Rect, so a
// widget that is rebuilt and moved in the same frame keeps its state, and
// an unrelated widget painted where it was does not take it.
type Identified interface {
	HitID() any
}

func idOf(h any) any {
	if i, ok := h.(Identified); ok {
		return i.HitID()
	}
	return nil
}

// Scale is the number of Image pixels per logical pixel: the monitor's device
// scale factor on a HiDPI screen, 1 elsewhere, and 1 for a canvas that was
// never given one. Layout and Rects are in logical pixels; anything drawn on
// Image must be scaled by it. FillRect, Geo and Px do that; text scales its
// face size instead.
func (c *Canvas) Scale() float64 {
	if c == nil || c.scale == 0 {
		return 1
	}
	return c.scale
}

// px and dp convert a length between logical and Image pixels. Every
// conversion in the framework goes through this pair.
func (c *Canvas) px(v float64) float64 { return v * c.Scale() }
func (c *Canvas) dp(v float64) float64 { return v / c.Scale() }

// Px converts a logical length or coordinate to Image pixels.
func (c *Canvas) Px(v float64) float32 { return float32(c.px(v)) }

// Geo returns the transform that maps a widget's own logical coordinates,
// with its origin at, onto Image pixels. Use it in DrawImageOptions.
func (c *Canvas) Geo(at Point) ebiten.GeoM {
	var g ebiten.GeoM
	s := c.Scale()
	g.Scale(s, s)
	g.Translate(c.px(at.X), c.px(at.Y))
	return g
}

// visiblePaintBounds tests drawing only; callers must still collect input,
// semantics and traces. Keep one physical pixel for antialiasing and round to
// float32 like the vector API. Unusual geometry takes the original draw path.
func (c *Canvas) visiblePaintBounds(r Rect, extra float64) bool {
	if c == nil || c.Image == nil {
		return false
	}
	bounds := c.Image.Bounds()
	if bounds.Empty() {
		return false
	}
	x, y, w, h := c.Px(r.Origin.X), c.Px(r.Origin.Y), c.Px(r.Size.W), c.Px(r.Size.H)
	pad := c.px(extra) + 1
	// Avoid culling huge coordinates where float32 intermediate rounding
	// can exceed the one-pixel guard.
	for _, v := range [...]float64{float64(x), float64(y), float64(w), float64(h), pad} {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1<<20 {
			return true
		}
	}
	if w < 0 || h < 0 || pad < 1 {
		return true
	}
	return float64(x+w)+pad > float64(bounds.Min.X) && float64(y+h)+pad > float64(bounds.Min.Y) &&
		float64(x)-pad < float64(bounds.Max.X) && float64(y)-pad < float64(bounds.Max.Y)
}

// FillRect fills the logical Rect r with col.
//
// A rectangle is drawn without anti-aliasing, which its axis-aligned edges
// have no use for. That matters more than it sounds: ebiten draws an
// anti-aliased shape by rendering it eight times into an offscreen stencil
// buffer the size of the shape and compositing the result, which breaks the
// batch both ways. A plain rectangle is instead one batched image, and most
// of the large fills in a frame are rectangles: table rows, sidebars, text
// selection, a modal's scrim.
func (c *Canvas) FillRect(r Rect, col color.Color) {
	if c == nil || c.Image == nil || col == nil {
		return
	}
	if !c.visiblePaintBounds(r, 0) {
		return
	}
	w, h := c.Px(r.Size.W), c.Px(r.Size.H)
	// Without anti-aliasing a shape thinner than a pixel covers no pixel
	// centre and disappears. A hairline divider asked for is drawn.
	if r.Size.W > 0 {
		w = max(w, 1)
	}
	if r.Size.H > 0 {
		h = max(h, 1)
	}
	vector.FillRect(c.Image, c.Px(r.Origin.X), c.Px(r.Origin.Y), w, h, col, false)
}

// FillCircle fills a circle of logical radius around center with col.
func (c *Canvas) FillCircle(center Point, radius float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || radius <= 0 {
		return
	}
	r := Rct(center.Add(Pt(-radius, -radius)), Sz(2*radius, 2*radius))
	if !c.visiblePaintBounds(r, 0) {
		return
	}
	// A circle is a rounded rectangle whose corner is its own half size.
	c.shadeRoundRect(r, radius, 0, col)
}

// StrokeLine draws a line of logical width w from a to b in col. The ends
// are cut square, so a line drawn in pieces joins up.
func (c *Canvas) StrokeLine(a, b Point, w float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || w <= 0 || c.Image.Bounds().Empty() {
		return
	}
	if !c.visiblePaintBounds(Rct(Pt(min(a.X, b.X), min(a.Y, b.Y)), Sz(math.Abs(b.X-a.X), math.Abs(b.Y-a.Y))), w/2) {
		return
	}
	// A line along an axis is a rectangle, and a rectangle draws as one
	// batched image rather than a shaded quad of its own. Rules, table
	// borders and the straight runs of a dashed outline are all this.
	switch {
	case a.Y == b.Y:
		c.FillRect(Rct(Pt(min(a.X, b.X), a.Y-w/2), Sz(math.Abs(b.X-a.X), w)), col)
	case a.X == b.X:
		c.FillRect(Rct(Pt(a.X-w/2, min(a.Y, b.Y)), Sz(w, math.Abs(b.Y-a.Y))), col)
	default:
		c.shadeSegment(a, b, w, col)
	}
}

func pathOptions(col color.Color) *vector.DrawPathOptions {
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(col)
	return op
}

// FillRoundRect fills the logical Rect r with col, with corners rounded by
// radius. A zero radius is FillRect.
func (c *Canvas) FillRoundRect(r Rect, radius float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil {
		return
	}
	if !c.visiblePaintBounds(r, 0) {
		return
	}
	if radius <= 0 {
		c.FillRect(r, col)
		return
	}
	c.shadeRoundRect(r, radius, 0, col)
}

// StrokeRoundRect draws a line of logical width w in col just inside r,
// with corners rounded by radius.
func (c *Canvas) StrokeRoundRect(r Rect, radius, w float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || w <= 0 || c.Image.Bounds().Empty() {
		return
	}
	if w <= r.Size.W && w <= r.Size.H && !c.visiblePaintBounds(r, w) {
		return
	}
	// A border as thick as the box it outlines leaves no hole to draw
	// around, so it is that box filled.
	if w >= r.Size.W || w >= r.Size.H {
		c.FillRoundRect(r, radius, col)
		return
	}
	// The border straddles a rectangle inset by half its width, which is
	// what keeps it just inside r.
	inset := Rct(r.Origin.Add(Pt(w/2, w/2)), Sz(r.Size.W-w, r.Size.H-w))
	c.shadeRoundRect(inset, max(radius-w/2, 0), w/2, col)
}

// physical returns the Image pixels r covers, rounded outwards.
func (c *Canvas) physical(r Rect) image.Rectangle {
	return image.Rect(
		int(math.Floor(c.px(r.Origin.X))), int(math.Floor(c.px(r.Origin.Y))),
		int(math.Ceil(c.px(r.Origin.X+r.Size.W))), int(math.Ceil(c.px(r.Origin.Y+r.Size.H))),
	)
}

// Clip returns a Canvas that draws only inside r and registers hit regions
// only where they overlap r. Coordinates are unchanged, and clips nest.
func (c *Canvas) Clip(r Rect) *Canvas {
	if c == nil {
		return nil
	}
	child := &Canvas{parent: c, clip: r, clipped: true, scale: c.scale, inert: c.inert, scope: c.scope, group: c.group}
	if c.clipped {
		child.clip = c.clip.Intersect(r)
	}
	if c.Image != nil {
		child.Image = c.Image.SubImage(c.physical(child.clip)).(*ebiten.Image)
	}
	return child
}

// add records a hit region, trimmed to the clip, on the root Canvas. A
// region registered at the same Rect as the previous one is merged into it,
// so Pointer(Focus(w)) or a widget that calls HitPointer and HitKey for the
// same Rect is one region with both handlers.
func (c *Canvas) add(h hitRegion) {
	if c == nil || c.inert {
		return
	}
	h.full = h.rect
	if c.clipped {
		h.rect = h.rect.Intersect(c.clip)
		if h.rect.Empty() {
			if h.key == nil {
				return
			}
			// A key region outside the clip stays registered with no area,
			// so Tab can reach it and Reveal can scroll it into view.
			h.rect = Rect{Origin: h.full.Origin}
		}
	}
	root := c
	for root.parent != nil {
		root = root.parent
	}
	root.adopt(&h)
	if n := len(root.hits); n > 0 && root.hits[n-1].merge(h) {
		return
	}
	root.hits = append(root.hits, h)
}

// adopt hands h's new handlers the ones that held the same region last
// frame: by ID when h has one, else by Rect.
func (c *Canvas) adopt(h *hitRegion) {
	if len(c.prev) == 0 {
		return
	}
	pointer, adoptsPointer := h.pointer.(Adopter)
	key, adoptsKey := h.key.(Adopter)
	if !adoptsPointer && !adoptsKey {
		return
	}
	old := c.lastFrame(h)
	if old == nil {
		return
	}
	if adoptsPointer && old.pointer != nil && !sameAny(old.pointer, h.pointer) {
		pointer.Adopt(old.pointer)
	}
	if adoptsKey && old.key != nil && !sameAny(old.key, h.key) && !sameAny(h.key, h.pointer) {
		key.Adopt(old.key)
	}
}

// lastFrame finds the region h takes over from: the one with the same ID,
// or, when h has none, the one that held the same Rect. Where two match,
// the one painted last wins, as it is the one on top.
//
// An ID the language cannot compare, which Interactive.SetKey accepts, matches
// nothing rather than bringing the process down.
func (c *Canvas) lastFrame(h *hitRegion) *hitRegion {
	f := c.fs()
	if !f.prevIndexed {
		c.indexPrev()
	}
	if h.id != nil {
		if !comparableID(h.id) {
			return nil
		}
		if i, ok := f.prevByID[h.id]; ok {
			return &c.prev[i]
		}
		return nil
	}
	if i, ok := f.prevByRect[h.rect]; ok {
		return &c.prev[i]
	}
	return nil
}

// indexPrev records where each of last frame's regions is. Walking forward
// leaves the last of any duplicates in the map, which is the one a scan
// backwards from the end would have stopped at.
func (c *Canvas) indexPrev() {
	f := c.fs()
	f.prevIndexed = true
	if f.prevByID == nil {
		f.prevByID = make(map[any]int, len(c.prev))
		f.prevByRect = make(map[Rect]int, len(c.prev))
	}
	for i := range c.prev {
		switch p := &c.prev[i]; {
		case p.id == nil:
			f.prevByRect[p.rect] = i
		case comparableID(p.id):
			f.prevByID[p.id] = i
		}
	}
}

// comparableID reports whether id may be used as a map key. A map operation
// on a key the language cannot compare panics where == only would have if
// the dynamic types matched, so both sides of the index check first.
func comparableID(id any) bool { return reflect.TypeOf(id).Comparable() }

// sameAny compares two handlers, treating uncomparable ones as different.
func sameAny(a, b any) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}

type hitRegion struct {
	rect    Rect
	full    Rect // rect before clipping, for scrolling it into view
	id      any  // from an Identified handler, else nil
	pointer PointerHandler
	key     KeyHandler
	cursor  CursorShape
	scope   *focusScope // the focus trap the region was painted in, if any
	role    Role        // from a Semantic handler
	label   string
	group   any // set by inGroup: the EachKeyed entry that painted it
}

// inGroup paints through fn with every region it registers marked as
// belonging to g, so input can report whether g holds focus or a capture.
func (c *Canvas) inGroup(g any, fn func()) {
	if c == nil {
		fn()
		return
	}
	prev := c.group
	c.group = g
	defer func() { c.group = prev }()
	fn()
}

// focusScope is a focus trap for one frame: while regions carrying it
// exist, Tab cycles within them and an unconsumed Escape calls escape.
// Frames compare scopes by owner.
type focusScope struct {
	owner  any
	escape func()
}

// FocusTrap paints through fn as a focus scope, which is what a dialog or
// popup is: while any region fn registered exists, Tab and Shift+Tab move
// only among those regions, focus is moved inside when the scope appears
// and returned to where it was when the scope goes, and an Escape the
// focused widget did not consume calls onEscape. owner identifies the
// scope from frame to frame.
func (c *Canvas) FocusTrap(owner any, onEscape func(), fn func(dst *Canvas)) {
	if c == nil {
		fn(nil)
		return
	}
	prev := c.scope
	c.scope = &focusScope{owner: owner, escape: onEscape}
	defer func() { c.scope = prev }()
	fn(c)
}

// merge folds o into r when they share a Rect and o only adds what r lacks.
func (r *hitRegion) merge(o hitRegion) bool {
	if r.rect != o.rect || (o.id != nil && r.id != nil && o.id != r.id) ||
		(o.pointer != nil && r.pointer != nil) ||
		(o.key != nil && r.key != nil) ||
		(o.cursor != 0 && r.cursor != 0) {
		return false
	}
	if o.pointer != nil {
		r.pointer = o.pointer
	}
	if o.key != nil {
		r.key = o.key
	}
	if o.cursor != 0 {
		r.cursor = o.cursor
	}
	if o.id != nil {
		r.id = o.id
	}
	if o.role != "" {
		r.role, r.label = o.role, o.label
	}
	return true
}

// HitPointer registers r as a region that receives pointer events. Regions
// painted later sit on top of earlier ones, so a container registers itself
// before painting its children.
func (c *Canvas) HitPointer(r Rect, h PointerHandler) {
	c.add(c.region(r, h, hitRegion{pointer: h}))
}

// HitKey registers r as a region that receives keyboard events while focused.
func (c *Canvas) HitKey(r Rect, h KeyHandler) {
	c.add(c.region(r, h, hitRegion{key: h}))
}

// region fills in what every registration shares: the Rect, the handler's
// identity and semantics, and the focus scope.
func (c *Canvas) region(r Rect, h any, reg hitRegion) hitRegion {
	reg.rect, reg.id = r, idOf(h)
	if c != nil {
		reg.scope, reg.group = c.scope, c.group
	}
	if s, ok := h.(Semantic); ok {
		reg.role, reg.label = s.Semantics()
	}
	return reg
}

// HitCursor asks for the mouse cursor to take shape while it is over r.
func (c *Canvas) HitCursor(r Rect, shape CursorShape) {
	c.add(c.region(r, nil, hitRegion{cursor: shape}))
}

// RequestFocus asks the frame to focus a painted, enabled key handler. Composite
// controls use it after navigation or validation. Requests for absent controls
// are ignored; normal focus trapping still applies.
func (c *Canvas) RequestFocus(h KeyHandler) {
	if c != nil && !c.inert {
		c.root().focusRequest = h
	}
}

type inputObserver struct {
	rect   Rect
	scope  *focusScope
	notify func()
}

// ObserveInput is notified before a pointer press, wheel or keyboard input in
// r, including input handled by descendants. It does not consume the event.
// This lets a streaming viewport pause before a reader interacts with content.
func (c *Canvas) ObserveInput(r Rect, notify func()) {
	if c == nil || c.inert || notify == nil {
		return
	}
	if c.clipped {
		r = r.Intersect(c.clip)
	}
	if !r.Empty() {
		c.root().inputObservers = append(c.root().inputObservers, inputObserver{r, c.scope, notify})
	}
}
