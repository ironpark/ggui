package ggui

import (
	"image"
	"math"
	"reflect"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/render"
)

// Canvas is what a Widget paints into: the target image plus the frame's list
// of interactive regions. Widgets that react to input register the Rect they
// painted with HitPointer or HitKey; the runtime dispatches the next frame's
// events to those regions, topmost (last painted) first. A nil Canvas paints
// nothing and collects nothing, which is what layout tests want.
type Canvas struct {
	Image   *ggfx.Image
	scale   float64
	parent  *Canvas // set on a Clip
	clip    Rect
	clipped bool
	inert   bool        // registers no hit regions: a widget on its way out
	scope   *focusScope // the focus trap regions are registered under, if any
	group   any         // what regions painted now belong to, for EachKeyed's eviction
	space   *paintSpace // inside a Transform or a shape clip; see effects.go
	// enclosing is what the widget painting now set for those inside it to
	// find; see Enclose.
	enclosing any

	// window is set on a Canvas that draws into the window's own image,
	// its screen or its backbuffer. derive and Clip keep it; a layer's
	// Canvas draws into an image of its own and clears it.
	window bool

	// frameState is the state there is one of per frame rather than one
	// per Canvas, the regions registered among it. The root makes it on
	// first use and every Canvas derived from the root points to the same
	// one, so a field added there is shared by construction, and reaching
	// it, which every Paint does, takes no walk up the chain. Reach it with
	// fs, which makes it.
	*frameState
}

// frameState is everything a frame accumulates once: what was painted, what
// it retained, and the accessibility tree built alongside. Every Canvas in a
// Clip chain reaches the same one through root, so a field added here is
// shared by construction and needs no clearing when a child Canvas is made.
type frameState struct {
	logical  Size // the window in logical pixels, for Size
	overlays []overlay
	// clock is the frame clock of the world painting, for the length of a
	// paint, so that Paint finds it without asking which goroutine runs.
	// Outside a paint it is nil, and Paint asks.
	clock *frameClock

	// The images Layer lends, by depth: as large as the areas the layers
	// at that depth took this frame, kept for the next frame's, and freed
	// at the end of one that took them no more.
	layers render.LayerPool[*ggfx.Image]

	// backdrop records that a BackdropBlur read the window's own image
	// this frame, or wanted to: the window keeps a backbuffer for it where
	// its screen cannot be read.
	backdrop bool

	inputView    // where the pointer and focus were as the paint began
	paintedInput // what the paint registers for input
	retained     // what Retain kept, this frame and last
	adoptIndex   // where last frame's regions are, for adopt
	canvasPool   // the Canvases Clip hands out
	traceState   // every widget painted, for the inspector; see inspector.go
	semState     // the accessibility tree; see semantics.go
	motionState  // what a frame painted in part repaints; see damage.go
}

// paintedInput is what a paint registers for input to take when it ends:
// the hit regions, with the last frame's for Adopter handoff, and the
// requests, observers and shortcuts widgets made as they painted.
type paintedInput struct {
	hits           []hitRegion
	prev           []hitRegion // last frame's regions, for Adopter handoff
	focusRequest   KeyHandler
	focusClaim     *FocusRef // a Focus waiting for the next key region painted
	revealRequests []Rect
	inputObservers []inputObserver
	shortcuts      []paintedShortcut
}

// inputView is where input stood as the paint began, which the Canvas's
// focus and pointer queries answer from: the paint cannot see input move.
type inputView struct {
	pointer    Point
	hasPointer bool
	// pointerRead records that a widget read the pointer in the last
	// whole paint, or in a part painted since: what such a widget draws
	// follows the pointer with no event reaching it, so a mouse move paints
	// it again. pointerAt are the bounds those widgets drew in, in the
	// window, which the move paints in part, as it would a widget that read
	// the time; pointerLost, that one read it where that cannot be told,
	// under a Transform or outside any widget's Paint, and the move repaints
	// the whole window.
	pointerRead  bool
	pointerLost  bool
	pointerAt    []Rect
	focusBounds  Rect       // focused input region at the start of this paint
	focusKey     KeyHandler // its handler
	focusPainted bool       // the focused handler registered since PaintFocus began
	focusVisible bool       // focus came there by keyboard
	pressed      bool       // a pointer button is held
	pressPointer PointerHandler

	// found is what the paint learns of its regions as it registers them,
	// for input to start from: see regionIndex.
	found regionIndex
}

// take records where in stands, for a paint about to begin.
func (v *inputView) take(in *inputState) {
	v.focusBounds, v.focusKey, v.focusPainted = Rect{}, nil, false
	if in.focused != nil {
		v.focusBounds, v.focusKey = in.focused.rect, in.focused.key
	}
	v.focusVisible = in.focused != nil && in.focusVisible
	v.pressed, v.pressPointer = in.pressed != nil, nil
	if in.pressed != nil {
		v.pressPointer = in.pressed.pointer
	}
	v.pointerRead, v.pointerLost, v.pointerAt = false, false, v.pointerAt[:0]
	v.found = regionIndex{scopeAt: -1}
	if v.focusKey != nil {
		v.found.key = handlerHint{h: v.focusKey, at: -1}
	}
	if v.pressPointer != nil {
		v.found.pointer = handlerHint{h: v.pressPointer, at: -1}
	}
}

// note records that the region at i of the paint's regions is r, which
// was just registered there or merged into: the last registered of a
// handler or a scope is the topmost.
func (v *inputView) note(r *hitRegion, i int) {
	x := &v.found
	if r.scope != nil {
		x.scopeAt = i
	}
	if r.cursor != 0 {
		x.cursors = true
	}
	if r.key != nil && x.key.h != nil && sameAny(r.key, x.key.h) {
		x.key.at = i
	}
	if r.pointer != nil && x.pointer.h != nil && sameAny(r.pointer, x.pointer.h) {
		x.pointer.at = i
	}
}

// retained is what Retain stored this frame, and last frame's, which
// Retained reads.
//
// A frame painted in part paints some widgets and leaves the rest as they
// were, so it keeps what the frame before stored and stores over it: keeps
// is then both. read and stored are the slots the widgets it painted read
// and stored, and a slot read but not stored again is dropped as the frame
// ends, as a whole frame drops it, while one no widget it painted touched
// stays.
type retained struct {
	keeps        map[retainKey]any
	prevKeeps    map[retainKey]any
	read, stored map[retainKey]struct{}
}

// next makes this frame's values last frame's.
func (r *retained) next() {
	r.prevKeeps, r.keeps = r.keeps, r.prevKeeps
	clear(r.keeps)
}

// endPartial drops, at the end of a frame painted in part, the slots read
// in it and not stored again.
func (r *retained) endPartial() {
	for k := range r.read {
		if _, ok := r.stored[k]; !ok {
			delete(r.keeps, k)
		}
	}
	clear(r.read)
	clear(r.stored)
}

// adoptIndex is where last frame's regions are, for adopt. It is built on
// a frame's first handoff and not at all in a frame with none, which is
// most of them: only a handful of widgets adopt.
type adoptIndex struct {
	prevByID    map[any]int  // id to its place in prev, the last one painted
	prevByRect  map[Rect]int // the same for a region with no id
	prevHandler map[any]bool // the handlers of prev that are pointers
	prevIndexed bool
}

// reset forgets the index, which a new prev makes stale. The maps keep
// their storage for the next frame that needs them.
func (x *adoptIndex) reset() {
	x.prevIndexed = false
	clear(x.prevByID)
	clear(x.prevByRect)
	clear(x.prevHandler)
}

// canvasPool holds the Canvases Clip hands out, made once and handed out
// again from the next paint on: canvasIdx are those taken this one.
type canvasPool struct {
	canvases  []*Canvas
	canvasIdx int
}

// resetCanvases takes back the Canvases Clip handed out, at the start and
// the end of a paint: nothing a widget painted keeps one past its frame,
// as with ClipReusing. Zeroed, they hold no image or parent until taken.
func (f *canvasPool) resetCanvases() {
	for i := 0; i < f.canvasIdx; i++ {
		*f.canvases[i] = Canvas{}
	}
	f.canvasIdx = 0
}

// fs returns the frame state, creating it on the root's first use. A Canvas
// with no parent is a root even before a frame has started, which is what a
// Probe and the zero Canvas in App rely on. One made by hand with a parent
// finds its root's the first time and keeps it.
func (c *Canvas) fs() *frameState {
	if c.frameState != nil {
		return c.frameState
	}
	r := c.root()
	if r.frameState == nil {
		r.frameState = &frameState{layers: render.LayerPool[*ggfx.Image]{Gauge: gaugeLayers}}
	}
	c.frameState = r.frameState
	return c.frameState
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
	k := at.key(s.id)
	f.keeps[k] = v
	if f.partial {
		if f.stored == nil {
			f.stored = make(map[retainKey]struct{})
		}
		f.stored[k] = struct{}{}
	}
}

// Retained returns what Retain stored under at and s last frame.
func (c *Canvas) Retained[T any](at Anchor, s Slot[T]) (T, bool) {
	if c == nil {
		var zero T
		return zero, false
	}
	f := c.fs()
	k := at.key(s.id)
	keeps := f.prevKeeps
	if f.partial {
		// The last frame's values are what keeps holds; see retained. Read
		// from those the whole frame before that left, a Transition that
		// appeared in the last whole frame would start again at every frame
		// painted in part, and never end.
		keeps = f.keeps
		if f.read == nil {
			f.read = make(map[retainKey]struct{})
		}
		f.read[k] = struct{}{}
	}
	v, ok := keeps[k].(T)
	return v, ok
}

// Ease moves a Motion retained under at and s toward target over d and
// returns where it is now, so a control rebuilt every frame keeps
// animating; a switch knob and a tab underline use it.
func (c *Canvas) Ease(at Anchor, s Slot[*Motion], target float64, d time.Duration) float64 {
	now := FrameTime()
	if c == nil {
		// Nothing keeps the motion: a new one would be at target at once.
		return target
	}
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
	f := c.fs()
	f.inputObservers = f.inputObservers[:0]
	f.shortcuts = f.shortcuts[:0]
	f.retained.next()
	// prev is a different slice now, so what adopt knew about it is stale.
	f.adoptIndex.reset()
}

// Inert returns a Canvas that draws where c does but registers no hit
// regions, for a widget that is leaving and must not take input. Like a
// Clip's, it is the frame's: a widget keeps none past its frame.
func (c *Canvas) Inert() *Canvas {
	if c == nil {
		return nil
	}
	child := c.derive()
	child.inert = true
	return child
}

// derive returns a copy of c that paints through it. The copy shares the
// root's frameState, so the frame's overlays, trace, retained slots and
// semantics stay where they are: there is nothing here to clear. It is
// the frame's, as a Clip is, so no widget keeps it past its frame.
func (c *Canvas) derive() *Canvas {
	f := c.fs()
	child := f.takeCanvas()
	*child = *c
	child.parent, child.frameState = c, f
	return child
}

// into returns a Canvas like c that draws into img, an image of its own
// such as a layer's.
func (c *Canvas) into(img *ggfx.Image) *Canvas {
	child := c.derive()
	child.Image, child.window = img, false
	return child
}

// readable reports whether what c has drawn can be read back, which a
// window's screen cannot be where its driver cannot sample it (Metal's it
// can).
func (c *Canvas) readable() bool { return c.Image != nil && c.Image.CanBeSource() }

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
	if inspectorEnabled.Load() {
		if f := c.fs(); f.tracing {
			defer f.traceLeave(f.traceEnter(c, w, r))
		}
	}
	fs := c.fs()
	if fs.skips(c, r) {
		return
	}
	clock := fs.clock
	if clock == nil {
		clock = &activeWorld().frame
	}
	// A widget wholly outside the clip still paints, for its semantics, but
	// an animation inside it must not keep the app painting. A zero-size
	// host, such as a toaster that paints through overlays, is never hidden.
	if c.clipped && !r.Empty() && r.Intersect(c.clip).Empty() {
		clock.hide()
		defer clock.unhide()
	}
	fs.openScope(c, r, clock.marks.Load())
	w.Paint(c, r)
	fs.closeScope(w, clock.marks.Load())
}

// Pointer returns where the mouse cursor was when this frame began, in
// logical pixels, and whether it is known. Widgets that react to hovering
// without a hit region, such as ui.Tooltip, read it in Paint. A mouse move
// paints such a widget again where it drew last, and what lies around it
// only when it draws beyond that or opens an Overlay: what it draws as the
// pointer moves belongs in what it drew before, its own Rect say.
func (c *Canvas) Pointer() (Point, bool) {
	if c == nil {
		return Point{}, false
	}
	f := c.fs()
	f.pointerRead = true
	if n := len(f.scopes); n > 0 {
		// The widget painting now leaves its bounds for a move to paint
		// again, as closeScope does a widget's that read the time.
		f.scopes[n-1].pointer = true
	} else {
		f.pointerLost = true
	}
	return c.space.local(f.pointer), f.hasPointer
}

// FocusWithin reports whether the focused input region overlaps r. Containers
// use it to pause motion while a descendant (including a custom control) has
// the keyboard. Like Pointer, it reads the state at the start of the paint.
// Overlap is all it knows: a widget floating over a focused one finds focus
// within it too. PaintFocus tells the focused widget by who painted it.
func (c *Canvas) FocusWithin(r Rect) bool {
	return c != nil && !c.fs().focusBounds.Intersect(c.toWindow(r)).Empty()
}

// HasFocus reports whether h is the focused key handler, as of the start of
// the paint.
func (c *Canvas) HasFocus(h KeyHandler) bool {
	return c != nil && h != nil && c.fs().focusKey != nil && sameAny(c.fs().focusKey, h)
}

// FocusVisible reports whether focus, wherever it is, came there by the
// keyboard, as the focus ring shows.
func (c *Canvas) FocusVisible() bool { return c != nil && c.fs().focusVisible }

// PaintFocus paints w into r, as Paint does, and reports whether the focused
// key handler is one w registered. A wrapper that reacts to its child's
// focus, as a Tooltip does, asks this rather than FocusWithin, which a
// focused widget under the wrapper, or around it, satisfies as well.
func (c *Canvas) PaintFocus(w Widget, r Rect) bool {
	if c == nil {
		c.Paint(w, r)
		return false
	}
	f := c.fs()
	outer := f.focusPainted
	f.focusPainted = false
	c.Paint(w, r)
	held := f.focusPainted
	f.focusPainted = outer || held
	return held
}

// FocusVisibleWithin is FocusWithin for focus moved there with the
// keyboard, as the focus ring shows: what a tooltip opens on, since a click
// that focuses a button is no request for its tip.
func (c *Canvas) FocusVisibleWithin(r Rect) bool {
	return c.FocusWithin(r) && c.fs().focusVisible
}

// PointerPressed reports whether a pointer button is held down, as of the
// start of the paint: a tooltip closes on a press.
func (c *Canvas) PointerPressed() bool { return c != nil && c.fs().pressed }

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
	// An overlay is placed in the coordinates of the widget scheduling it,
	// so under a Transform it paints through the same transform: a popup
	// opens beside its moved anchor, not where the anchor was laid out.
	if m := c.space.affine(); !m.IsIdentity() {
		inner := fn
		fn = func(dst *Canvas) { dst.Transform(m, inner) }
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
	f.overlaid = f.overlaid || len(f.overlays) > 0
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
// with its origin at, onto Image pixels, for drawing on Image with ggfx
// directly. DrawImage does this itself.
func (c *Canvas) Geo(at Point) ggfx.GeoM {
	var g ggfx.GeoM
	s := c.Scale()
	g.Scale(s, s)
	g.Translate(c.px(at.X), c.px(at.Y))
	return g
}

// physical returns the Image pixels r covers, rounded outwards.
func (c *Canvas) physical(r Rect) image.Rectangle {
	return image.Rect(
		int(math.Floor(c.px(r.Origin.X))), int(math.Floor(c.px(r.Origin.Y))),
		int(math.Ceil(c.px(r.Origin.X+r.Size.W))), int(math.Ceil(c.px(r.Origin.Y+r.Size.H))),
	)
}

// logicalRect is r, in physical pixels, in logical ones: physical's
// inverse, for a part of the image that holds r.
func (c *Canvas) logicalRect(r image.Rectangle) Rect {
	return Rct(Pt(c.dp(float64(r.Min.X)), c.dp(float64(r.Min.Y))), Sz(c.dp(float64(r.Dx())), c.dp(float64(r.Dy()))))
}

// Clip returns a Canvas that draws only inside r and registers hit regions
// only where they overlap r. Coordinates are unchanged, and clips nest.
// The Canvas is the frame's: it is handed out again in a later frame, so
// a widget keeps none past its Paint or the overlays the frame paints.
func (c *Canvas) Clip(r Rect) *Canvas {
	if c == nil {
		return nil
	}
	return c.clipInto(c.fs().takeCanvas(), r)
}

// takeCanvas hands out a Canvas of the frame's, made once and handed out
// again from the next paint on.
func (f *canvasPool) takeCanvas() *Canvas {
	var child *Canvas
	if f.canvasIdx < len(f.canvases) {
		child = f.canvases[f.canvasIdx]
	} else {
		child = new(Canvas)
		f.canvases = append(f.canvases, child)
	}
	f.canvasIdx++
	return child
}

// Enclose paints fn with v as what encloses what fn paints: a widget
// painted inside finds v with Enclosing, as a context menu inside another
// finds the other.
func (c *Canvas) Enclose(v any, fn func()) {
	if c == nil {
		fn()
		return
	}
	prev := c.enclosing
	c.enclosing = v
	defer func() { c.enclosing = prev }()
	fn()
}

// TakesInput reports whether the hit regions registered through c take
// input: not on an Inert Canvas, nor in a frame painted over the last one
// in part, whose regions stay the last whole frame's.
func (c *Canvas) TakesInput() bool { return c != nil && !c.inert }

// Enclosing is what the nearest Enclose around this paint set, or nil.
func (c *Canvas) Enclosing() any {
	if c == nil {
		return nil
	}
	return c.enclosing
}

// Visible is the part of r this Canvas draws in: r within its clip.
func (c *Canvas) Visible(r Rect) Rect {
	if c != nil && c.clipped {
		return r.Intersect(c.clip)
	}
	return r
}

// ClipReusing is Clip made in reuse, a Canvas the caller keeps from frame
// to frame, for a widget that clips each time it paints: a row of a long
// list clipping its content would otherwise make a Canvas a row a frame.
// What was painted through reuse before is let go, so nothing a widget
// painted may keep reuse past its frame.
func (c *Canvas) ClipReusing(reuse *Canvas, r Rect) *Canvas {
	if c == nil {
		return nil
	}
	return c.clipInto(reuse, r)
}

// clipInto makes child the Clip of c to r.
func (c *Canvas) clipInto(child *Canvas, r Rect) *Canvas {
	*child = Canvas{parent: c, frameState: c.fs(), clip: r, clipped: true, scale: c.scale, inert: c.inert, scope: c.scope, group: c.group, space: c.space, enclosing: c.enclosing, window: c.window}
	if c.clipped {
		child.clip = c.clip.Intersect(r)
	}
	if c.Image != nil {
		child.Image = c.Image.SubImage(c.physical(child.clip)).(*ggfx.Image)
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
		// A reach is cut as the Rect is, and goes with it: a control
		// scrolled out of a list is not hit through the list's edge.
		if h.reach = h.reach.Intersect(c.clip); h.reach.Empty() || h.rect.Empty() {
			h.reach = Rect{}
		}
	}
	if c.space != nil && !c.space.placeRegion(&h) {
		return
	}
	f := c.fs()
	f.adopt(&h)
	if h.key != nil && f.focusKey != nil && sameAny(h.key, f.focusKey) {
		f.focusPainted = true
	}
	if h.key != nil && f.focusClaim != nil {
		f.focusRequest, f.focusClaim = h.key, nil
	}
	i := len(f.hits)
	if i > 0 && f.hits[i-1].merge(h) {
		i--
	} else {
		f.hits = append(f.hits, h)
	}
	f.note(&f.hits[i], i)
}

// adopt hands h's new handlers the ones that held the same region last
// frame: by ID when h has one, else by Rect.
func (f *frameState) adopt(h *hitRegion) {
	if len(f.prev) == 0 {
		return
	}
	pointer, adoptsPointer := h.pointer.(Adopter)
	key, adoptsKey := h.key.(Adopter)
	if !adoptsPointer && !adoptsKey {
		return
	}
	old := f.lastFrame(h)
	if old == nil {
		return
	}
	// A handler that held a region last frame is the widget it was, not one
	// rebuilt in another's place, wherever it is now and whatever else held
	// this Rect: a scroll and the scroll inside it are clipped to one
	// viewport, and the outer must not take the inner's offset for its own.
	if adoptsPointer && old.pointer != nil && !sameAny(old.pointer, h.pointer) && !f.heldLastFrame(h.pointer) {
		pointer.Adopt(old.pointer)
	}
	if adoptsKey && old.key != nil && !sameAny(old.key, h.key) && !sameAny(h.key, h.pointer) && !f.heldLastFrame(h.key) {
		key.Adopt(old.key)
	}
}

// heldLastFrame reports whether handler, a pointer, was a handler of one
// of last frame's regions. One of another kind is not looked for.
func (f *frameState) heldLastFrame(handler any) bool { return f.prevHandler[handler] }

// lastFrame finds the region h takes over from: the one with the same ID,
// or, when h has none, the one that held the same Rect. Where two match,
// the one painted last wins, as it is the one on top.
//
// An ID the language cannot compare, which Interactive.SetKey accepts, matches
// nothing rather than bringing the process down.
func (f *frameState) lastFrame(h *hitRegion) *hitRegion {
	if !f.prevIndexed {
		f.indexPrev()
	}
	if h.id != nil {
		if !comparableID(h.id) {
			return nil
		}
		if i, ok := f.prevByID[h.id]; ok {
			return &f.prev[i]
		}
		return nil
	}
	if i, ok := f.prevByRect[h.rect]; ok {
		return &f.prev[i]
	}
	return nil
}

// indexPrev records where each of last frame's regions is. Walking forward
// leaves the last of any duplicates in the map, which is the one a scan
// backwards from the end would have stopped at.
func (f *frameState) indexPrev() {
	f.prevIndexed = true
	if f.prevByID == nil {
		f.prevByID = make(map[any]int, len(f.prev))
		f.prevByRect = make(map[Rect]int, len(f.prev))
		f.prevHandler = make(map[any]bool, len(f.prev))
	}
	for i := range f.prev {
		for _, handler := range [2]any{f.prev[i].pointer, f.prev[i].key} {
			if handler != nil && reflect.TypeOf(handler).Kind() == reflect.Pointer {
				f.prevHandler[handler] = true
			}
		}
		switch p := &f.prev[i]; {
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
// Handlers are pointers, and IDs mostly strings and numbers, which compare
// as they are; input compares every region's handler this way, so only
// another kind pays for the recover that guards a comparison which panics.
func sameAny(a, b any) bool {
	t := reflect.TypeOf(a)
	if t != reflect.TypeOf(b) {
		return false
	}
	if t == nil {
		return true
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.Chan, reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return a == b
	}
	return sameAnyGuarded(a, b)
}

// sameAnyGuarded is a == b, false where the comparison panics: a slice, a
// map or a func, or a struct or array holding one, even in an interface.
func sameAnyGuarded(a, b any) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
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
// CursorShapeDefault is a request like any other: it puts the arrow over r
// whatever a region painted before, under it, asked for.
func (c *Canvas) HitCursor(r Rect, shape CursorShape) {
	c.add(c.region(r, nil, hitRegion{cursor: shape, arrow: shape == CursorShapeDefault}))
}

// RequestFocus asks the frame to focus a painted, enabled key handler. Composite
// controls use it after navigation or validation. Requests for absent controls
// are ignored; normal focus trapping still applies.
func (c *Canvas) RequestFocus(h KeyHandler) {
	if c != nil && !c.inert {
		c.fs().focusRequest = h
	}
}

// RequestReveal asks every Scroll around r to move the least it must to show
// it, as moving focus there with the keyboard would, without moving focus:
// a list whose arrow keys a parent handles keeps its selection in view.
// It takes effect once, after this paint; ask again only when the target
// changes, or the user can no longer scroll away from it.
func (c *Canvas) RequestReveal(r Rect) {
	if c != nil && !c.inert {
		f := c.fs()
		f.revealRequests = append(f.revealRequests, c.toWindow(r))
	}
}

// paintedShortcut is a chord a widget listens for while it is painted.
type paintedShortcut struct {
	chord Chord
	fn    func()
	scope *focusScope
}

// Shortcut runs fn on chord for as long as the caller is painted: a
// widget registers it from Paint every frame, as it does its hit regions,
// so the shortcut goes when the widget does, when it is inert, and while a
// focus trap it is not inside, such as a dialog over it, is open. It runs
// where Window.Shortcut's would: a chord with a modifier before the
// focused widget, a bare key only if the focused widget left it.
func (c *Canvas) Shortcut(chord Chord, fn func()) {
	if c == nil || c.inert || fn == nil {
		return
	}
	f := c.fs()
	f.shortcuts = append(f.shortcuts, paintedShortcut{chord, fn, c.scope})
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
	if c.space != nil {
		r = c.space.window(r, false)
	}
	if !r.Empty() {
		f := c.fs()
		f.inputObservers = append(f.inputObservers, inputObserver{r, c.scope, notify})
	}
}
