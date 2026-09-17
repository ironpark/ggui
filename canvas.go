package ggui

import (
	"image"
	"image/color"
	"math"
	"reflect"
	"strings"
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
	Image *ebiten.Image

	scale   float64
	hits    []hitRegion
	prev    []hitRegion // last frame's regions, for Adopter handoff
	parent  *Canvas     // set on a Clip; hit regions go to the root
	clip    Rect
	clipped bool
	inert   bool // registers no hit regions: a widget on its way out

	// Root-only frame state.
	logical    Size // the window in logical pixels, for Size
	pointer    Point
	hasPointer bool
	overlays   []func(*Canvas)
	trace      []traceEntry // every Paint call, when the inspector is on
	tracing    bool
	depth      int
	keeps      map[retainKey]any // Retain this frame
	prevKeeps  map[retainKey]any // Retain last frame, read by Retained
}

// Slot names one value a widget retains on the Canvas between frames,
// typed like an Env Key. Make one per animated value or timer with
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
// rebuilt every frame keeps state that has no signal: a Tooltip's hover
// timer, a Transition's start time. A slot nobody retains again is
// dropped, so state goes away with the widget.
func (c *Canvas) Retain[T any](at Anchor, s Slot[T], v T) {
	if c == nil {
		return
	}
	root := c.root()
	if root.keeps == nil {
		root.keeps = make(map[retainKey]any)
	}
	root.keeps[at.key(s.id)] = v
}

// Retained returns what Retain stored under at and s last frame.
func (c *Canvas) Retained[T any](at Anchor, s Slot[T]) (T, bool) {
	if c == nil {
		var zero T
		return zero, false
	}
	v, ok := c.root().prevKeeps[at.key(s.id)].(T)
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
	c.prevKeeps, c.keeps = c.keeps, c.prevKeeps
	clear(c.keeps)
	rotateEnvMemo()
}

// Inert returns a Canvas that draws where c does but registers no hit
// regions, for a widget that is leaving and must not take input.
func (c *Canvas) Inert() *Canvas {
	if c == nil {
		return nil
	}
	child := *c
	child.parent, child.inert = c, true
	child.overlays, child.trace, child.keeps, child.prevKeeps = nil, nil, nil, nil
	return &child
}

// traceEntry is one widget's Rect as painted, for the inspector.
type traceEntry struct {
	rect  Rect
	depth int
	name  string
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
	root := c.root()
	if root.tracing {
		root.trace = append(root.trace, traceEntry{rect: r, depth: root.depth, name: widgetName(w)})
		root.depth++
		defer func() { root.depth-- }()
	}
	w.Paint(c, r)
}

// widgetName is a widget's type for the inspector: Box for *ggui.BoxWidget.
func widgetName(w Widget) string {
	t := reflect.TypeOf(w)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := t.Name()
	if i := strings.Index(name, "["); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSuffix(name, "Widget")
}

// Pointer returns where the mouse cursor was when this frame began, in
// logical pixels, and whether it is known. Widgets that react to hovering
// without a hit region, such as Tooltip, read it in Paint.
func (c *Canvas) Pointer() (Point, bool) {
	if c == nil {
		return Point{}, false
	}
	root := c.root()
	return root.pointer, root.hasPointer
}

// Overlay schedules fn to paint after the whole tree has, on the root
// Canvas, unclipped and above everything: tooltips, popups and menus go
// there. Hit regions fn registers sit on top of the tree's.
func (c *Canvas) Overlay(fn func(dst *Canvas)) {
	if c == nil {
		return
	}
	root := c.root()
	root.overlays = append(root.overlays, fn)
}

// Size returns the logical size of the window being painted, or zero for
// a Canvas that was given none.
func (c *Canvas) Size() Size {
	if c == nil {
		return Size{}
	}
	root := c.root()
	if root.logical != (Size{}) || root.Image == nil {
		return root.logical
	}
	b := root.Image.Bounds()
	return Sz(c.dp(float64(b.Dx())), c.dp(float64(b.Dy())))
}

// paintOverlays runs the overlays queued this frame, including ones they
// queue themselves, and clears the queue.
func (c *Canvas) paintOverlays() {
	for i := 0; i < len(c.overlays); i++ {
		c.overlays[i](c)
	}
	c.overlays = c.overlays[:0]
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

// FillRect fills the logical Rect r with col.
func (c *Canvas) FillRect(r Rect, col color.Color) {
	if c == nil || c.Image == nil || col == nil {
		return
	}
	vector.FillRect(c.Image, c.Px(r.Origin.X), c.Px(r.Origin.Y), c.Px(r.Size.W), c.Px(r.Size.H), col, true)
}

// FillCircle fills a circle of logical radius around center with col.
func (c *Canvas) FillCircle(center Point, radius float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || radius <= 0 {
		return
	}
	vector.FillCircle(c.Image, c.Px(center.X), c.Px(center.Y), c.Px(radius), col, true)
}

// StrokeLine draws a line of logical width w from a to b in col.
func (c *Canvas) StrokeLine(a, b Point, w float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || w <= 0 {
		return
	}
	vector.StrokeLine(c.Image, c.Px(a.X), c.Px(a.Y), c.Px(b.X), c.Px(b.Y), c.Px(w), col, true)
}

// roundRect traces r with corners of the given logical radius, in Image
// pixels. A zero radius traces a plain rectangle.
func (c *Canvas) roundRect(r Rect, radius float64) *vector.Path {
	x, y, w, h := c.Px(r.Origin.X), c.Px(r.Origin.Y), c.Px(r.Size.W), c.Px(r.Size.H)
	rad := min(c.Px(radius), w/2, h/2)
	var p vector.Path
	p.MoveTo(x+rad, y)
	p.LineTo(x+w-rad, y)
	p.ArcTo(x+w, y, x+w, y+rad, rad)
	p.LineTo(x+w, y+h-rad)
	p.ArcTo(x+w, y+h, x+w-rad, y+h, rad)
	p.LineTo(x+rad, y+h)
	p.ArcTo(x, y+h, x, y+h-rad, rad)
	p.LineTo(x, y+rad)
	p.ArcTo(x, y, x+rad, y, rad)
	p.Close()
	return &p
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
	if radius <= 0 {
		c.FillRect(r, col)
		return
	}
	vector.FillPath(c.Image, c.roundRect(r, radius), &vector.FillOptions{}, pathOptions(col))
}

// StrokeRoundRect draws a line of logical width w in col just inside r,
// with corners rounded by radius.
func (c *Canvas) StrokeRoundRect(r Rect, radius, w float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || w <= 0 {
		return
	}
	inset := Rct(r.Origin.Add(Pt(w/2, w/2)), Sz(r.Size.W-w, r.Size.H-w))
	vector.StrokePath(c.Image, c.roundRect(inset, max(radius-w/2, 0)), &vector.StrokeOptions{Width: c.Px(w)}, pathOptions(col))
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
	child := &Canvas{parent: c, clip: r, clipped: true, scale: c.scale, inert: c.inert}
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
	if c.clipped {
		h.rect = h.rect.Intersect(c.clip)
		if h.rect.Empty() {
			return
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
	var old *hitRegion
	for i := len(c.prev) - 1; i >= 0; i-- {
		p := &c.prev[i]
		if h.id != nil && p.id == h.id || h.id == nil && p.id == nil && p.rect == h.rect {
			old = p
			break
		}
	}
	if old == nil {
		return
	}
	if a, ok := h.pointer.(Adopter); ok && old.pointer != nil && !sameAny(old.pointer, h.pointer) {
		a.Adopt(old.pointer)
	}
	if a, ok := h.key.(Adopter); ok && old.key != nil && !sameAny(old.key, h.key) && !sameAny(h.key, h.pointer) {
		a.Adopt(old.key)
	}
}

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
	id      any // from an Identified handler, else nil
	pointer PointerHandler
	key     KeyHandler
	cursor  ebiten.CursorShapeType
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
	return true
}

// HitPointer registers r as a region that receives pointer events. Regions
// painted later sit on top of earlier ones, so a container registers itself
// before painting its children.
func (c *Canvas) HitPointer(r Rect, h PointerHandler) {
	c.add(hitRegion{rect: r, id: idOf(h), pointer: h})
}

// HitKey registers r as a region that receives keyboard events while focused.
func (c *Canvas) HitKey(r Rect, h KeyHandler) {
	c.add(hitRegion{rect: r, id: idOf(h), key: h})
}

// HitCursor asks for the mouse cursor to take shape while it is over r.
func (c *Canvas) HitCursor(r Rect, shape ebiten.CursorShapeType) {
	c.add(hitRegion{rect: r, cursor: shape})
}
