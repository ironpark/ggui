package ggui

import (
	"image"
	"image/color"
	"math"
	"reflect"
	"strings"

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

	// Root-only frame state.
	logical    Size // the window in logical pixels, for Size
	pointer    Point
	hasPointer bool
	overlays   []func(*Canvas)
	trace      []traceEntry // every Paint call, when the inspector is on
	tracing    bool
	depth      int
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

// Adopter is a handler that can take over from the handler that occupied
// the same Rect in the previous frame. When a rebuild replaces a widget,
// the new one is handed the old one as it registers its region, so hover,
// press, caret or an animation in flight carry across the rebuild instead
// of resetting. prev is the previous PointerHandler or KeyHandler; check
// its type and copy what applies.
type Adopter interface {
	Adopt(prev any)
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
	child := &Canvas{parent: c, clip: r, clipped: true, scale: c.scale}
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
	if c == nil {
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

// adopt hands h's new handlers the ones that held the same Rect last frame.
func (c *Canvas) adopt(h *hitRegion) {
	if len(c.prev) == 0 {
		return
	}
	var old *hitRegion
	for i := len(c.prev) - 1; i >= 0; i-- {
		if c.prev[i].rect == h.rect {
			old = &c.prev[i]
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
	pointer PointerHandler
	key     KeyHandler
	cursor  ebiten.CursorShapeType
}

// merge folds o into r when they share a Rect and o only adds what r lacks.
func (r *hitRegion) merge(o hitRegion) bool {
	if r.rect != o.rect ||
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
	return true
}

// HitPointer registers r as a region that receives pointer events. Regions
// painted later sit on top of earlier ones, so a container registers itself
// before painting its children.
func (c *Canvas) HitPointer(r Rect, h PointerHandler) {
	c.add(hitRegion{rect: r, pointer: h})
}

// HitKey registers r as a region that receives keyboard events while focused.
func (c *Canvas) HitKey(r Rect, h KeyHandler) {
	c.add(hitRegion{rect: r, key: h})
}

// HitCursor asks for the mouse cursor to take shape while it is over r.
func (c *Canvas) HitCursor(r Rect, shape ebiten.CursorShapeType) {
	c.add(hitRegion{rect: r, cursor: shape})
}
