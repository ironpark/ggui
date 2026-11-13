package ggui

import (
	"runtime"
	"slices"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/pkg/kinetics"
	"github.com/ironpark/ggwd"
)

// PointerKind says what a PointerEvent reports.
type PointerKind int

const (
	PointerDown   PointerKind = iota // a button went down inside the region
	PointerUp                        // a button came up inside the region
	PointerTap                       // down and up happened inside the same region
	PointerMove                      // the cursor is inside the region this frame
	PointerEnter                     // the region became the hovered one
	PointerExit                      // the region stopped being the hovered one
	PointerScroll                    // the wheel moved over the region
	PointerDrag                      // a button that went down inside the region is still held, wherever the cursor is now
)

// PointerEvent is a mouse or touch event delivered to a hit region.
type PointerEvent struct {
	Kind           PointerKind
	Pos            Point // where the handler painted: window coordinates, unless under a Transform
	Button         MouseButton
	Scroll         Point // wheel delta, for PointerScroll
	ScrollPixels   bool  // Scroll is in logical pixels (a trackpad's, or touch panning), not wheel units
	ScrollMomentum bool  // inertial scrolling; return false when no further movement is possible
	// ScrollStep is set for the notches of a wheel the platform said how
	// far to scroll for: a step, which a scrolling view eases over rather
	// than jumps, where a trackpad's scroll or a pan is followed as it
	// comes. ScrollBy says how far.
	ScrollStep bool
	// scrollBy is how far the platform scrolls for Scroll's wheel units,
	// in logical pixels, or zero where it did not say; see ScrollBy.
	scrollBy Point
	// ScrollPhase is where a trackpad's scroll is in its gesture, on a
	// platform that says: ScrollPhaseEnded is the fingers lifting, and may
	// come with no Scroll at all. A wheel's is ScrollPhaseNone.
	ScrollPhase ScrollPhase
	// Mods are the modifiers held, for a press, release, tap or wheel: a
	// Shift-click that extends a selection, a ⌘-click that toggles one.
	Mods Mods
}

// PointerHandler receives pointer events whose position fell inside the
// region it was registered with. Returning true consumes the event; false
// lets the region beneath see it, so a tap target does not block scrolling.
//
// A region that consumed PointerDown captures the pointer: until the button
// comes up it receives PointerDrag every frame and then PointerUp, even when
// the cursor has left it. That is what a slider knob or a text selection
// needs.
type PointerHandler interface {
	HandlePointer(ev PointerEvent) bool
}

// TouchDragCapturer lets a pressed control retain touch drags instead of
// allowing an enclosing scroll region to take over the gesture.
type TouchDragCapturer interface {
	CaptureTouchDrag() bool
}

// Mods are the modifier keys held during a KeyEvent. Cmd reports the
// platform's command modifier: Meta (⌘) on macOS, Ctrl elsewhere.
type Mods = ggwd.Mods

func runtimeIsDarwin() bool { return runtime.GOOS == "darwin" }

// KeyKind says what a KeyEvent reports.
type KeyKind int

const (
	KeyPress   KeyKind = iota // Key was just pressed
	KeyText                   // Text was typed
	KeyFocus                  // the region gained keyboard focus
	KeyBlur                   // the region lost keyboard focus
	KeyRelease                // Key was let go of, for a KeyReleaseHandler alone
)

// KeyEvent is a keyboard event delivered to the focused region.
type KeyEvent struct {
	Kind KeyKind
	Key  KeyboardKey // for KeyPress
	Text string      // for KeyText
	Mods Mods        // for KeyPress
}

// KeyHandler receives keyboard events while its region has focus. A region
// gains focus when a button goes down inside it and loses it when the button
// goes down elsewhere or the region disappears. A held key repeats: after a
// short delay it delivers KeyPress again every few frames.
type KeyHandler interface {
	HandleKey(ev KeyEvent)
}

// KeyReleaseHandler is a KeyHandler that is also told, while it has focus,
// of each key let go of, as a KeyRelease: a terminal reports releases to a
// program that asks for them. Only the focused handler is told, and only of
// keys released while it has focus; a held key repeats as KeyPress.
type KeyReleaseHandler interface {
	KeyHandler
	HandleKeyRelease(ev KeyEvent)
}

// TickHandler is a KeyHandler that also needs to run once per frame while it
// has focus, before that frame's keys are delivered: a text editor drives the
// platform IME this way. Returning true reports that the IME consumed this
// frame's input, and the KeyPress and KeyText events are then withheld.
type TickHandler interface {
	KeyHandler
	HandleTick() (consumed bool)
}

// KeyConsumer is a KeyHandler that can say which key presses it acts on,
// before they are delivered. A bare-key shortcut runs only when the
// focused handler does not consume the key, and an Escape the focused
// handler does not consume closes the dialog or popup it is in. A handler
// that does not implement it consumes nothing that way.
type KeyConsumer interface {
	ConsumesKey(ev KeyEvent) bool
}

// TabStopper is a KeyHandler that may stay out of the Tab order while
// remaining focusable by a click, an arrow key or Canvas.RequestFocus: the
// rows of a table, list or tree, of which one is the stop Tab lands on and
// the arrows reach the rest.
type TabStopper interface {
	TabStop() bool
}

// ChordClaimer is a KeyHandler that keeps chords from the runtime's own
// bindings while it has the focus. A chord it claims skips everything that
// would otherwise take it before the handler sees it -- Tab's focus
// traversal, App.OnKey hooks, shortcuts and menu actions -- and arrives as a
// KeyPress. A text editor keeps ⌘Z, ⌘C and the rest, so the app's Undo
// shortcut or Edit menu undoes the app only when no field is being edited;
// a terminal keeps Tab for the shell's completion and Ctrl+C for the
// program, so focus leaves it by the pointer or from code.
type ChordClaimer interface {
	ClaimsChord(ev KeyEvent) bool
}

// Revealer is a container that can scroll to show a Rect: Scroll is one.
// Focus moved by the keyboard into a region asks every Revealer painted to
// reveal it, in window coordinates; one that does not contain r ignores
// the call.
type Revealer interface {
	Reveal(r Rect)
}

// WheelLine is how far a unit of a wheel's Scroll moves a scrolling view,
// in logical pixels, where the platform did not say: for an event made by
// hand or by a Probe. A window's own say how far in ScrollBy.
const WheelLine = 20

// ScrollBy returns how far a PointerScroll moves a scrolling view, in
// logical pixels with Scroll's signs: Scroll itself when it is in pixels,
// and for a wheel what the platform and its settings scroll for its
// notches, which is not the same from one to the next: a notch is 100
// pixels on Windows as it ships, 53 under X, and what the page would have
// scrolled in a browser. A view that scrolls moves by this; what steps by
// the notch, a zoom or a pager, counts Scroll.
func (ev PointerEvent) ScrollBy() Point {
	switch {
	case ev.ScrollPixels:
		return ev.Scroll
	case ev.scrollBy != (Point{}):
		return ev.scrollBy
	}
	return Pt(ev.Scroll.X*WheelLine, ev.Scroll.Y*WheelLine)
}

// scrolled reports whether the frame scrolled anything.
func (f *frameInput) scrolled() bool { return f.wheel != (Point{}) || f.wheelPx != (Point{}) }

// frameInput is everything the runtime read from the platform this frame.
type frameInput struct {
	touch   bool
	pos     Point
	down    []MouseButton
	up      []MouseButton
	wheel   Point         // a wheel's notches
	wheelBy Point         // how far the platform scrolls for them, in logical pixels; zero where it did not say
	wheelPx Point         // a precise scroll's logical pixels, a trackpad's
	phase   ScrollPhase   // of the frame's scrolls, the lift outranking the rest
	keys    []KeyboardKey // just pressed, plus repeats of held keys
	// released are the keys let go of, for a KeyReleaseHandler.
	released []KeyboardKey
	text     string
	mods     Mods
	drop     []DroppedFile // files dropped onto the window this frame
	drag     bool          // files are being dragged over the window
	dragAt   Point         // where, when drag is set
}

// A hitRegion is a part of the window a widget registered with the Canvas
// for input in the frame painted last: its handlers, what it is called,
// and the space it was painted in. Input reads the regions of the last
// paint, top down, and keeps copies of the ones it follows across frames.
type hitRegion struct {
	rect    Rect
	full    Rect // rect before clipping, for scrolling it into view
	reach   Rect // set by HitReach: where the pointer lands on it besides rect
	id      any  // from an Identified handler, else nil
	pointer PointerHandler
	key     KeyHandler
	cursor  CursorShape
	arrow   bool        // asks for the default cursor over whatever lies under it
	scope   *focusScope // the focus trap the region was painted in, if any
	role    Role        // from a Semantic handler
	label   string
	group   any        // set by inGroup: the EachKeyed entry that painted it
	drag    dragSource // a DragSource beneath its child; see dragdrop.go

	space  *paintSpace // the space it was painted in, nil for the window's
	local  Rect        // rect in that space, clipped there; see contains
	near   Rect        // reach in that space
	origin Point       // full's corner in that space, before placeRegion moved it
}

// merge folds o into r when they share a Rect and o only adds what r lacks.
func (r *hitRegion) merge(o hitRegion) bool {
	if r.rect != o.rect || r.reach != o.reach || r.space != o.space || (o.id != nil && r.id != nil && o.id != r.id) ||
		o.drag != nil || r.drag != nil ||
		(o.pointer != nil && r.pointer != nil) ||
		(o.key != nil && r.key != nil) ||
		(o.cursor != 0 && (r.cursor != 0 || r.arrow)) || (o.arrow && r.cursor != 0) {
		return false
	}
	r.arrow = r.arrow || o.arrow
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

// contains reports whether the window point p lands on r: in its Rect, or
// in the reach a control gave it around that.
func (r *hitRegion) contains(p Point) bool {
	if !r.rect.Contains(p) && !r.reach.Contains(p) {
		return false
	}
	if r.space == nil {
		return true
	}
	q, ok := r.space.reach(p)
	return ok && (r.local.Contains(q) || r.near.Contains(q))
}

// at returns the transform for the child painted in r.
func (t *TransformWidget) at(r Rect) Affine {
	return t.shown.About(Pt(r.Origin.X+r.Size.W*t.originX, r.Origin.Y+r.Size.H*t.originY))
}

// offset returns the window point p from the corner of the Rect r was
// registered for, in the coordinates it was painted in: where on a drag
// source the pointer took hold, or where in a drop zone it let go.
func (r *hitRegion) offset(p Point) Point {
	if r.space == nil {
		return p.Sub(r.full.Origin)
	}
	return r.at(p).Sub(r.origin)
}

// pointerAt is ev as r's handler sees it.
func (r *hitRegion) pointerAt(ev PointerEvent) PointerEvent {
	if r.space == nil {
		return ev
	}
	if ev.Scroll != (Point{}) {
		// A delta is a direction, so it turns and scales but does not move:
		// a scroll view turned on its side scrolls along its own axis, and
		// a pan under a zoom follows the finger.
		ev.Scroll = r.at(ev.Scroll).Sub(r.at(Point{}))
	}
	ev.Pos = r.at(ev.Pos)
	return ev
}

// keep returns a copy of r that outlives the regions buffer.
func keep(r *hitRegion) *hitRegion {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}

// regionSlot keeps a region the input follows every frame, as keep does,
// in one of two buffers it alternates between: keeping one allocates
// nothing, and the copy kept before stays valid while the next is made,
// as a hover's exit needs the region it leaves. A pointer it returned is
// good until the second keep after it, so only the field it fills may hold
// one past the call.
type regionSlot struct {
	buf [2]hitRegion
	cur uint8
}

func (s *regionSlot) keep(r *hitRegion) *hitRegion {
	s.cur ^= 1
	if r == nil {
		// Let go of the handlers kept here, all but the copy just
		// replaced, which its caller may still read.
		s.buf[s.cur] = hitRegion{}
		return nil
	}
	s.buf[s.cur] = *r
	return &s.buf[s.cur]
}

// handle delivers the window-space ev to r's PointerHandler in the
// coordinates r was painted in and reports whether it consumed it.
func (r *hitRegion) handle(ev PointerEvent) bool {
	return r.pointer.HandlePointer(r.pointerAt(ev))
}

// inputState routes frameInput to the regions painted last frame. A pressed
// or focused region is matched across frames by its handler when the widget
// survived, or by Rect when a rebuild replaced it, so a widget that moves
// while dragged keeps the drag and a tap completes across a rebuild.
type inputState struct {
	// touched are the regions, in the window's coordinates, pointer events
	// were handed to since takeTouched; touchedUnplaced, that one was under
	// a Transform or a shape clip, where it cannot be placed.
	touched         []Rect
	touchedUnplaced bool
	// lastMove is where the last frame's move was, and moved whether this
	// frame's is elsewhere; movedOnce, that there was one.
	lastMove         Point
	moved, movedOnce bool

	regions   []hitRegion
	observers []inputObserver
	// ix is what is known of regions without scanning them again.
	ix regionIndex

	// These are copies: regions is rebuilt every frame in a reused buffer,
	// so a pointer into it would soon describe a different region.
	hovered    *hitRegion
	pressed    *hitRegion
	pressedBtn MouseButton
	focused    *hitRegion
	// The slots hovered and focused are kept in: they are kept again
	// every frame, which a slot does without allocating.
	hoverSlot, focusSlot regionSlot
	focusVisible         bool        // focused moved there with the keyboard
	cursor               CursorShape // what the hovered region asked for
	dragTarget           *hitRegion  // the region files are being dragged over
	drag                 *appDrag    // a drag inside the app; see dragdrop.go
	touchPan             kinetics.Pan
	touchScroll          *hitRegion // what took the pan
	touchMotion          touchMotion

	shortcuts []func(KeyEvent) bool // App.OnKey handlers, tried before the focused widget
	chords    []*ShortcutHandle     // App.Shortcut handlers
	painted   []paintedShortcut     // Canvas.Shortcut handlers, from the last paint
	drops     []func(DropEvent)     // Host.OnDrop handlers, for drops no region took

	// A focus trap: while regions of a scope exist, Tab cycles within them
	// and an unconsumed Escape goes to the scope. trapReturn is where focus
	// was when the scope appeared, restored when it goes.
	trap       any
	trapReturn *hitRegion
}

// dispatch turns one frame's raw input into handler calls, in the order the
// frame needs them: what is under the pointer is settled before a press can
// use it, presses before releases, and the keyboard last, because moving
// focus with Tab changes who the remaining keys belong to.
func (in *inputState) dispatch(f frameInput) {
	defer in.markBusyGroups()
	in.updateTrap()
	in.notifyObservers(f)
	in.updateHover(f)
	in.updateCursor(f.pos)
	in.updateDrag(f)
	in.panTouch(&f)
	in.dispatchPointer(f)
	in.updateAppDrag(f)
	if len(f.drop) > 0 {
		in.dispatchDrop(f)
	}
	if i := slices.Index(f.keys, KeyEscape); i >= 0 && in.cancelAppDrag() {
		f.keys = slices.Delete(slices.Clone(f.keys), i, i+1)
	}
	f.keys = in.consumeBindings(f.keys, f.mods)
	in.dispatchKeys(f)
}

// dispatchOver offers f to the overlay before the widgets, unless a widget
// holds a press, so that a drag that began in the app finishes there.
func (in *inputState) dispatchOver(o Overlay, f frameInput) {
	if o == nil || in.pressed != nil || !o.Input(overlayInput(f)) {
		in.dispatch(f)
	}
}

// cursorOver is the cursor at p: the overlay's when it wants one and no
// widget holds a press, else what the widgets asked for.
func (in *inputState) cursorOver(o Overlay, p Point) CursorShape {
	if o != nil && in.pressed == nil {
		if shape, ok := o.Cursor(p); ok {
			return shape
		}
	}
	return in.cursor
}

// markBusyGroups records the groups holding focus or a pointer capture after
// this dispatch, so EachKeyed.Retain leaves them be.
func (in *inputState) markBusyGroups() {
	busy := activeWorld().busy
	clear(busy)
	for _, r := range []*hitRegion{in.focused, in.pressed, in.touchMotion.target} {
		if r != nil && r.group != nil {
			busy[r.group] = true
		}
	}
}

// notifyObservers tells the regions watching for input that some arrived
// inside them, or reached the widget that has focus. A trap in front of an
// observer hides the input from it.
func (in *inputState) notifyObservers(f frameInput) {
	if len(in.observers) == 0 {
		return
	}
	scope := in.activeScope()
	for _, observer := range in.observers {
		if scope != nil && observer.scope != scope {
			continue
		}
		pointer := (len(f.down) > 0 || f.scrolled()) && observer.rect.Contains(f.pos)
		keyboard := (len(f.keys) > 0 || f.text != "") && in.focused != nil && !observer.rect.Intersect(in.focused.rect).Empty()
		if pointer || keyboard {
			observer.notify()
		}
	}
}

// updateHover moves hover to the topmost region that claims PointerMove,
// sending the exit before the enter so the two never overlap.
func (in *inputState) updateHover(f frameInput) {
	move := PointerEvent{Kind: PointerMove, Pos: f.pos, Mods: f.mods}
	in.moved = !in.movedOnce || f.pos != in.lastMove
	in.lastMove, in.movedOnce = f.pos, true
	now := in.send(move)
	if !in.continues(now, in.hovered) {
		if in.hovered != nil {
			// The exit goes to the handler holding the region now: one
			// rebuilt since it was entered has taken over its hover.
			left := in.hovered
			if cur := in.findPointer(left); cur != nil {
				left = cur
			}
			in.deliver(left, PointerEvent{Kind: PointerExit, Pos: f.pos})
		}
		if now != nil {
			in.deliver(now, PointerEvent{Kind: PointerEnter, Pos: f.pos})
		}
	} else if now != nil && now.pointer != nil && !sameAny(now.pointer, in.hovered.pointer) {
		// Rebuilt under the pointer, the region is a new handler, with a
		// hover state of its own that never heard the enter: a row of a
		// list redrawn for a change elsewhere would show itself left. The
		// one it replaced is gone, and hears no exit.
		in.deliver(now, PointerEvent{Kind: PointerEnter, Pos: f.pos})
	}
	in.hovered = in.hoverSlot.keep(now)
}

// updateDrag moves the drag target to the topmost region taking DragOver,
// sending the exit to the old one first. With no drag it only sends the
// exit, which is how a drop or an abandoned drag ends the target's hover.
func (in *inputState) updateDrag(f frameInput) {
	var now *hitRegion
	if f.drag {
		over := DragEvent{Kind: DragOver, Pos: f.dragAt}
		now = in.topmost(func(r *hitRegion) bool {
			h, ok := r.pointer.(DragHandler)
			return ok && r.contains(over.Pos) && h.HandleDrag(DragEvent{Kind: over.Kind, Pos: r.at(over.Pos)})
		})
	}
	if in.dragTarget != nil && !in.continues(now, in.dragTarget) {
		in.dragTarget.pointer.(DragHandler).HandleDrag(DragEvent{Kind: DragExit, Pos: in.dragTarget.at(f.dragAt)})
	}
	in.dragTarget = keep(now)
}

// updateCursor picks the shape of the topmost region at pos asking for one.
// A region that asked for the default cursor counts, so that a button over
// a text field shows the arrow and not the field's beam.
func (in *inputState) updateCursor(pos Point) {
	in.cursor = CursorShapeDefault
	if !in.index().cursors {
		return
	}
	if r := in.topmost(func(r *hitRegion) bool { return (r.cursor != 0 || r.arrow) && r.contains(pos) }); r != nil {
		in.cursor = r.cursor
	}
}

// dispatchPointer sends this frame's presses, drag, releases and wheel. The
// region that took a press keeps the drag and the release wherever the
// cursor goes, and gets a tap only if the release landed back inside it.
func (in *inputState) dispatchPointer(f frameInput) {
	for _, b := range f.down {
		ev := PointerEvent{Kind: PointerDown, Pos: f.pos, Button: b, Mods: f.mods}
		if b == MouseButtonLeft {
			in.pressDrag(f.pos)
		}
		in.pressed, in.pressedBtn = keep(in.send(ev)), b
		in.setFocus(in.findKey(f.pos))
	}
	if in.pressed != nil && len(f.down) == 0 && len(f.up) == 0 {
		if cur := in.findPointer(in.pressed); cur != nil {
			in.deliver(cur, PointerEvent{Kind: PointerDrag, Pos: f.pos, Button: in.pressedBtn})
		}
	}
	for _, b := range f.up {
		ev := PointerEvent{Kind: PointerUp, Pos: f.pos, Button: b, Mods: f.mods}
		p := in.pressed
		if p == nil || b != in.pressedBtn {
			in.send(ev)
			continue
		}
		if cur := in.findPointer(p); cur != nil {
			in.deliver(cur, ev)
			if cur.contains(f.pos) {
				in.deliver(cur, PointerEvent{Kind: PointerTap, Pos: f.pos, Button: b, Mods: f.mods})
			}
		}
		in.pressed = nil
	}
	// A wheel's notches and a trackpad's pixels go as scrolls of their own;
	// a gesture's lift, which moves nothing, goes as the trackpad's.
	lifted := f.phase == ScrollPhaseEnded || f.phase == ScrollPhaseMomentumEnded
	momentum := f.phase == ScrollPhaseMomentum || f.phase == ScrollPhaseMomentumEnded
	if f.wheel != (Point{}) {
		in.sendScroll(PointerEvent{Kind: PointerScroll, Pos: f.pos, Scroll: f.wheel, scrollBy: f.wheelBy, ScrollStep: f.wheelBy != (Point{}), Mods: f.mods, ScrollMomentum: momentum, ScrollPhase: f.phase})
	}
	if f.wheelPx != (Point{}) || lifted && f.wheel == (Point{}) {
		in.sendScroll(PointerEvent{Kind: PointerScroll, Pos: f.pos, Scroll: f.wheelPx, ScrollPixels: true, Mods: f.mods, ScrollMomentum: momentum, ScrollPhase: f.phase})
	}
}

// sendScroll sends a wheel's or a trackpad's scroll. One that moves along
// both axes goes as two, an axis each, so that each finds the view that
// scrolls its way: a diagonal swipe over a vertical list inside a
// horizontal one moves both, where a single event would stop at the first
// to use either part of it.
func (in *inputState) sendScroll(ev PointerEvent) {
	if ev.Scroll.X == 0 || ev.Scroll.Y == 0 {
		in.send(ev)
		return
	}
	x, y := ev, ev
	x.Scroll.Y, x.scrollBy.Y = 0, 0
	y.Scroll.X, y.scrollBy.X = 0, 0
	in.send(x)
	in.send(y)
}

// dispatchKeys hands the remaining keys and text to the focused widget, then
// runs what the widget did not consume through afterFocused. A widget whose
// tick handled the input this frame keeps the keys to itself.
func (in *inputState) dispatchKeys(f frameInput) {
	if in.focused == nil {
		in.afterFocused(f, nil)
		return
	}
	cur := in.findKeyRegion(in.focused)
	if cur == nil {
		in.setFocus(nil)
		in.afterFocused(f, nil)
		return
	}
	if prev := in.focused.key; !sameAny(prev, cur.key) {
		// The tree was rebuilt: the region is the same, the widget new.
		// An Adopter already took the old one's state over during paint;
		// anything else is told it has focus now.
		if _, ok := cur.key.(Adopter); !ok {
			delivered.Add(1)
			cur.key.HandleKey(KeyEvent{Kind: KeyFocus})
		}
	}
	in.focused = in.focusSlot.keep(cur)
	if th, ok := cur.key.(TickHandler); ok && th.HandleTick() {
		releaseKeys(cur.key, f)
		return
	}
	for _, k := range f.keys {
		cur.key.HandleKey(KeyEvent{Kind: KeyPress, Key: k, Mods: f.mods})
	}
	if f.text != "" {
		cur.key.HandleKey(KeyEvent{Kind: KeyText, Text: f.text})
	}
	releaseKeys(cur.key, f)
	in.afterFocused(f, cur.key)
}

// releaseKeys tells h of the keys let go of this frame, after the frame's
// presses, when it asks to be told. They go to it even when its tick took
// the presses, so a key it saw pressed is not left held.
func releaseKeys(h KeyHandler, f frameInput) {
	if rh, ok := h.(KeyReleaseHandler); ok {
		for _, k := range f.released {
			rh.HandleKeyRelease(KeyEvent{Kind: KeyRelease, Key: k, Mods: f.mods})
		}
	}
}

// activeScope returns the topmost focus scope painted this frame.
func (in *inputState) activeScope() *focusScope {
	if i := in.index().scopeAt; i >= 0 {
		return in.regions[i].scope
	}
	return nil
}

// updateTrap notices a focus scope appearing or going. When one appears,
// focus that is outside it moves to its first key region and where focus
// was is remembered; when it goes, focus that went with it returns there.
func (in *inputState) updateTrap() {
	var owner any
	if s := in.activeScope(); s != nil {
		owner = s.owner
	}
	if owner == in.trap {
		return
	}
	if owner == nil {
		in.trap = nil
		if in.focused == nil || in.findKeyRegion(in.focused) == nil {
			if r := in.trapReturn; r != nil {
				in.focus(in.findKeyRegion(r), false)
			}
		}
		in.trapReturn = nil
		return
	}
	if in.trap == nil {
		in.trapReturn = keep(in.focused)
	}
	in.trap = owner
	inside := func(r *hitRegion) bool { return r.scope != nil && r.scope.owner == owner }
	if in.focused != nil {
		if cur := in.findKeyRegion(in.focused); cur != nil && inside(cur) {
			return
		}
	}
	for i := range in.regions {
		if r := &in.regions[i]; r.key != nil && inside(r) {
			in.focus(r, false)
			return
		}
	}
}

// topmost returns the last region painted that match accepts, or nil. Later
// regions sit on top of earlier ones, so the scan runs backwards.
func (in *inputState) topmost(match func(*hitRegion) bool) *hitRegion {
	for i := len(in.regions) - 1; i >= 0; i-- {
		if r := &in.regions[i]; match(r) {
			return r
		}
	}
	return nil
}

// regionIndex is what input knows of a list of regions without scanning
// it: the topmost region in a focus scope, whether any asks for a cursor,
// and where the handlers input follows were found. A paint fills it as it
// registers the regions; for regions set otherwise, as a test sets them,
// one scan fills it the first time it is asked. It holds for the buffer
// and length it was made for alone, so a later paint into the same buffer
// must hand input a new one with the regions.
type regionIndex struct {
	made    bool
	base    *hitRegion // regions[0] of the list it describes, nil if empty
	n       int
	scopeAt int  // the topmost region with a focus scope, or -1
	cursors bool // a region asks for a cursor
	// The last key and pointer handlers looked up, and where.
	key, pointer handlerHint
}

// handlerHint is where the topmost region with handler h is, or -1 when
// none has it; a zero hint knows of no handler.
type handlerHint struct {
	h  any
	at int
}

// describes reports whether x is the index of rs.
func (x *regionIndex) describes(rs []hitRegion) bool {
	if !x.made || x.n != len(rs) {
		return false
	}
	return len(rs) == 0 || x.base == &rs[0]
}

// setRegions hands input the regions a paint registered, and what the
// paint learned of them as it did.
func (in *inputState) setRegions(rs []hitRegion, found regionIndex) {
	in.regions, in.ix = rs, found
	in.ix.made, in.ix.n, in.ix.base = true, len(rs), nil
	if len(rs) > 0 {
		in.ix.base = &rs[0]
	}
}

// index returns the index of the regions, scanning them once for one
// made where they were not painted.
func (in *inputState) index() *regionIndex {
	x := &in.ix
	if x.describes(in.regions) {
		return x
	}
	*x = regionIndex{made: true, n: len(in.regions), scopeAt: -1}
	if len(in.regions) > 0 {
		x.base = &in.regions[0]
	}
	for i := range in.regions {
		r := &in.regions[i]
		if r.scope != nil {
			x.scopeAt = i
		}
		if r.cursor != 0 {
			x.cursors = true
		}
	}
	return x
}

// regionOf returns the topmost region whose key handler, or pointer
// handler when key is false, is h, or nil. The answer is remembered for
// the next lookup of h over the same regions: a focused or pressed widget
// is looked up every frame, and the regions change only when painted.
func (in *inputState) regionOf(h any, key bool) *hitRegion {
	if h == nil {
		return nil
	}
	x := in.index()
	hint := &x.pointer
	if key {
		hint = &x.key
	}
	if hint.h != nil && sameAny(hint.h, h) {
		if hint.at < 0 {
			return nil
		}
		if r := &in.regions[hint.at]; sameAny(handlerOf(r, key), h) {
			return r
		}
	}
	at := -1
	for i := len(in.regions) - 1; i >= 0; i-- {
		if r := handlerOf(&in.regions[i], key); r != nil && sameAny(r, h) {
			at = i
			break
		}
	}
	*hint = handlerHint{h: h, at: at}
	if at < 0 {
		return nil
	}
	return &in.regions[at]
}

// handlerOf is r's key handler when key is set, else its pointer handler.
func handlerOf(r *hitRegion, key bool) any {
	if key {
		if r.key == nil {
			return nil
		}
		return r.key
	}
	if r.pointer == nil {
		return nil
	}
	return r.pointer
}

// deliver hands ev to r as handle does, noting where: a widget a pointer
// event reached may look otherwise with no signal written, as a button
// pressed or hovered does, and the frame paints its region again.
//
// A move is handed to the regions under the pointer every frame, moved or
// not, and passes through those that do not claim it: only one claimed
// after the pointer moved can have changed.
func (in *inputState) deliver(r *hitRegion, ev PointerEvent) bool {
	took := r.handle(ev)
	if ev.Kind == PointerMove && (!took || !in.moved) {
		return took
	}
	if r.space != nil {
		in.touchedUnplaced = true
	} else {
		in.touched = append(in.touched, r.rect)
	}
	return took
}

// delivered counts the events handed to widgets, which may change how
// they look with no signal written: a frame in which it moves is painted
// whole.
var delivered atomic.Uint64

// send offers ev to regions under it from the top down until one consumes it
// and returns that region, or nil.
func (in *inputState) send(ev PointerEvent) *hitRegion {
	return in.topmost(func(r *hitRegion) bool {
		return r.pointer != nil && r.contains(ev.Pos) && in.deliver(r, ev)
	})
}

// findKey returns the topmost region with a KeyHandler under p.
func (in *inputState) findKey(p Point) *hitRegion {
	return in.topmost(func(r *hitRegion) bool { return r.key != nil && r.contains(p) })
}

// findPointer returns the region that continues p this frame by its
// pointer handler; see continuing.
func (in *inputState) findPointer(p *hitRegion) *hitRegion { return in.continuing(p, false) }

// findKeyRegion is findPointer for a key region.
func (in *inputState) findKeyRegion(p *hitRegion) *hitRegion { return in.continuing(p, true) }

// continuing returns the region that continues p this frame, matched by
// its key handler when key is set and by its pointer handler otherwise:
// the one with the same handler, wherever it moved to, or else the topmost
// region with such a handler and p's ID, or with none and painted exactly
// where p was, which is a rebuilt widget.
func (in *inputState) continuing(p *hitRegion, key bool) *hitRegion {
	if r := in.regionOf(handlerOf(p, key), key); r != nil {
		return r
	}
	if p.id != nil {
		return in.topmost(func(r *hitRegion) bool { return r.handles(key) && r.id == p.id })
	}
	return in.topmost(func(r *hitRegion) bool { return r.handles(key) && r.id == nil && r.rect == p.rect })
}

// handles reports whether r has a key handler, when key is set, or else a
// pointer handler.
func (r *hitRegion) handles(key bool) bool {
	if key {
		return r.key != nil
	}
	return r.pointer != nil
}

// refreshFocused moves the focused region to where its widget painted this
// frame, so the next paint's FocusWithin sees it there: a region focused
// while scrolled out of view was kept with no area. Only the same handler
// counts; one rebuilt in its place is left for dispatchKeys to tell it has
// focus.
func (in *inputState) refreshFocused() {
	if in.focused == nil {
		return
	}
	if r := in.regionOf(in.focused.key, true); r != nil {
		in.focused = in.focusSlot.keep(r)
	}
}

func (in *inputState) setFocus(r *hitRegion) { in.focus(r, false) }

// focus moves keyboard focus to r. A move made with the keyboard is
// reported with Key set to KeyTab, so a control can show a focus ring only
// then, as browsers do with :focus-visible.
func (in *inputState) focus(r *hitRegion, keyboard bool) {
	if r == nil && in.focused == nil {
		return
	}
	if r != nil && in.focused != nil && (sameAny(r.key, in.focused.key) || in.continues(r, in.focused)) {
		// The same widget, perhaps in another region, such as a dropdown's
		// list under its field: focus stays, without a blur, and becomes
		// visible if the keyboard moved it there.
		in.focused = in.focusSlot.keep(r)
		in.focusVisible = in.focusVisible || keyboard
		return
	}
	if in.focused != nil && in.focused.key != nil {
		delivered.Add(1)
		in.focused.key.HandleKey(KeyEvent{Kind: KeyBlur})
	}
	if r != nil && r.key != nil {
		ev := KeyEvent{Kind: KeyFocus}
		if keyboard {
			ev.Key = KeyTab
			in.reveal(r.full)
		}
		r.key.HandleKey(ev)
	}
	in.focused = in.focusSlot.keep(r)
	in.focusVisible = keyboard
}

// reveal asks every Revealer painted to scroll target into view; each
// decides whether target is inside its content.
func (in *inputState) reveal(target Rect) {
	for i := range in.regions {
		if v, ok := in.regions[i].pointer.(Revealer); ok {
			v.Reveal(target)
		}
	}
}

// moveFocus steps focus through the key regions in paint order, wrapping
// at the ends: Tab is dir 1, Shift+Tab is -1. While a focus scope is
// showing, only its regions take part.
func (in *inputState) moveFocus(dir int) {
	var keyed []*hitRegion
	cur := -1
	scope := in.activeScope()
	// The focused region may have moved since it was recorded, as after a
	// Reveal, so find it the way a frame does, handler before Rect.
	var focused *hitRegion
	if in.focused != nil {
		focused = in.findKeyRegion(in.focused)
	}
	for i := range in.regions {
		r := &in.regions[i]
		if r.key == nil || (scope != nil && (r.scope == nil || r.scope.owner != scope.owner)) {
			continue
		}
		if r == focused || focused != nil && sameAny(r.key, focused.key) {
			// The focused widget is one stop, at its first region, however
			// many it painted: a combobox's input also takes the keyboard
			// over its list.
			if cur < 0 {
				cur = len(keyed)
				keyed = append(keyed, r)
			}
			continue
		}
		if t, ok := r.key.(TabStopper); ok && !t.TabStop() {
			continue
		}
		keyed = append(keyed, r)
	}
	if len(keyed) == 0 {
		return
	}
	next := 0
	if dir < 0 {
		next = len(keyed) - 1
	}
	if cur >= 0 {
		next = (cur + dir + len(keyed)) % len(keyed)
	}
	in.focus(keyed[next], true)
}

// releasePress takes the press away from the widget under it when a drag
// or a pan takes over: it gets a PointerUp without a tap.
func (in *inputState) releasePress(pos Point, mods Mods) {
	if in.pressed == nil {
		return
	}
	if cur := in.findPointer(in.pressed); cur != nil {
		in.deliver(cur, PointerEvent{Kind: PointerUp, Pos: pos, Button: in.pressedBtn, Mods: mods})
	}
	in.pressed = nil
}

// continues reports whether now, a region painted this frame, is the one
// prev was recorded from, matched as findPointer and findKeyRegion match: by
// handler, then id, and by Rect only for a rebuilt widget. Two regions
// sharing a Rect, as in a Stack, stay two.
func (in *inputState) continues(now, prev *hitRegion) bool {
	if now == nil || prev == nil {
		return now == prev
	}
	if (prev.pointer != nil && sameAny(now.pointer, prev.pointer)) || (prev.key != nil && sameAny(now.key, prev.key)) {
		return true
	}
	if prev.pointer != nil && in.findPointer(prev) == now {
		return true
	}
	return prev.key != nil && in.findKeyRegion(prev) == now
}

// PointerWidget makes its child react to the pointer. Build one with Pointer
// or Tap. It takes no space of its own: the child's Rect is the hit region.
type PointerWidget struct {
	props       property.Owner
	child       Widget
	cursor      CursorShape
	onDown      func(PointerEvent)
	onUp        func(PointerEvent)
	onTap       func()
	onMove      func(PointerEvent)
	onDrag      func(PointerEvent)
	onEnter     func()
	onExit      func()
	onScroll    func(Point)
	onDrop      func(DropEvent)
	onDropHover func(bool)
	dropHovered bool
}

// Pointer wraps child in a hit region. Attach callbacks with the On methods;
// the widget consumes only the events it has a callback for.
func Pointer(child Widget) *PointerWidget { return &PointerWidget{child: child} }

// Tap is Pointer with an OnTap callback.
func Tap(child Widget, fn func()) *PointerWidget { return Pointer(child).OnTap(fn) }

// OnTap fires when a button goes down and comes up inside the child.
func (p *PointerWidget) OnTap(fn func()) *PointerWidget { p.onTap = fn; return p }

// OnDown fires when a button goes down inside the child.
func (p *PointerWidget) OnDown(fn func(PointerEvent)) *PointerWidget { p.onDown = fn; return p }

// OnUp fires when a button comes up inside the child.
func (p *PointerWidget) OnUp(fn func(PointerEvent)) *PointerWidget { p.onUp = fn; return p }

// OnMove fires every frame the cursor is inside the child.
func (p *PointerWidget) OnMove(fn func(PointerEvent)) *PointerWidget { p.onMove = fn; return p }

// OnDrag fires every frame a button pressed inside the child is still held,
// with the cursor's current position, inside or out. OnUp ends it.
func (p *PointerWidget) OnDrag(fn func(PointerEvent)) *PointerWidget { p.onDrag = fn; return p }

// Cursor sets the mouse cursor shown while the pointer is over the child,
// such as CursorShapePointer for something clickable.
func (p *PointerWidget) Cursor(shape CursorShape) *PointerWidget {
	defer property.Watch(&p.props, &p.cursor)()
	p.cursor = shape
	return p
}

// OnEnter fires when the cursor enters the child.
func (p *PointerWidget) OnEnter(fn func()) *PointerWidget { p.onEnter = fn; return p }

// OnExit fires when the cursor leaves the child.
func (p *PointerWidget) OnExit(fn func()) *PointerWidget { p.onExit = fn; return p }

// OnHover fires with true on enter and false on exit; hover.Set fits it.
func (p *PointerWidget) OnHover(fn func(bool)) *PointerWidget {
	return p.OnEnter(func() { fn(true) }).OnExit(func() { fn(false) })
}

// OnScroll fires with the wheel delta when the wheel moves over the child.
func (p *PointerWidget) OnScroll(fn func(delta Point)) *PointerWidget { p.onScroll = fn; return p }

// OnDrop fires when files are dropped onto the child from the desktop,
// which makes the child a drop zone: a drop over it goes here rather than
// to the handlers registered with Host.OnDrop.
func (p *PointerWidget) OnDrop(fn func(DropEvent)) *PointerWidget { p.onDrop = fn; return p }

// OnDropHover fires with true when files are dragged over the child and
// false when they leave it or are dropped, so the zone can highlight while
// a drop would land on it; hover.Set fits it. Only a platform that reports
// the drag delivers it, which is macOS today; see DragHandler.
func (p *PointerWidget) OnDropHover(fn func(bool)) *PointerWidget { p.onDropHover = fn; return p }

// HandleDrag implements DragHandler. A drop zone claims the drag whether or
// not it listens to it, so that the zone which lights up is the one the
// drop will reach.
func (p *PointerWidget) HandleDrag(ev DragEvent) bool {
	if p.onDrop == nil {
		return false
	}
	if p.onDropHover == nil {
		return true
	}
	switch ev.Kind {
	case DragOver:
		if !p.dropHovered {
			p.dropHovered = true
			p.onDropHover(true)
		}
	case DragExit:
		if p.dropHovered {
			p.dropHovered = false
			p.onDropHover(false)
		}
	}
	return true
}

// HandleDrop implements DropHandler.
func (p *PointerWidget) HandleDrop(ev DropEvent) bool {
	if p.onDrop == nil {
		return false
	}
	p.onDrop(ev)
	return true
}

// HandlePointer implements PointerHandler.
func (p *PointerWidget) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerDown:
		if p.onDown != nil {
			p.onDown(ev)
		}
		return p.onDown != nil || p.onTap != nil || p.onDrag != nil
	case PointerUp:
		if p.onUp != nil {
			p.onUp(ev)
		}
		return p.onUp != nil || p.onTap != nil || p.onDrag != nil
	case PointerDrag:
		if p.onDrag != nil {
			p.onDrag(ev)
		}
		return p.onDrag != nil
	case PointerTap:
		if p.onTap != nil {
			p.onTap()
		}
		return p.onTap != nil
	case PointerMove:
		if p.onMove != nil {
			p.onMove(ev)
		}
		return p.onMove != nil || p.onEnter != nil || p.onExit != nil
	case PointerEnter:
		if p.onEnter != nil {
			p.onEnter()
		}
		return true
	case PointerExit:
		if p.onExit != nil {
			p.onExit()
		}
		return true
	case PointerScroll:
		if p.onScroll != nil {
			p.onScroll(ev.Scroll)
		}
		return p.onScroll != nil
	}
	return false
}

// Layout implements Widget.
func (p *PointerWidget) Layout(c Constraints, env Env) Size {
	defer p.props.Layout().EndLayout()
	return p.child.Layout(c, env)
}

// Baseline implements Baseliner: the child's.
func (p *PointerWidget) Baseline() (float64, bool) { return baselineOf(p.child) }

// Paint implements Widget.
func (p *PointerWidget) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, p)
	if p.cursor != 0 {
		dst.HitCursor(r, p.cursor)
	}
	dst.Paint(p.child, r)
}

// FocusWidget gives its child keyboard focus when clicked. Build one with
// Focus.
type FocusWidget struct {
	child   Widget
	onKey   func(KeyboardKey)
	onText  func(string)
	onFocus func(bool)
}

// Focus wraps child in a focusable region. Clicking inside focuses it; keys
// typed while focused reach the On callbacks.
func Focus(child Widget) *FocusWidget { return &FocusWidget{child: child} }

// OnKey fires once per key press while focused.
func (f *FocusWidget) OnKey(fn func(KeyboardKey)) *FocusWidget { f.onKey = fn; return f }

// OnText fires with the characters typed this frame while focused.
func (f *FocusWidget) OnText(fn func(string)) *FocusWidget { f.onText = fn; return f }

// OnFocus fires with true on focus and false on blur; focused.Set fits it.
func (f *FocusWidget) OnFocus(fn func(bool)) *FocusWidget { f.onFocus = fn; return f }

// HandleKey implements KeyHandler.
func (f *FocusWidget) HandleKey(ev KeyEvent) {
	switch ev.Kind {
	case KeyPress:
		if f.onKey != nil {
			f.onKey(ev.Key)
		}
	case KeyText:
		if f.onText != nil {
			f.onText(ev.Text)
		}
	case KeyFocus:
		if f.onFocus != nil {
			f.onFocus(true)
		}
	case KeyBlur:
		if f.onFocus != nil {
			f.onFocus(false)
		}
	}
}

// Layout implements Widget.
func (f *FocusWidget) Layout(c Constraints, env Env) Size { return f.child.Layout(c, env) }

// Baseline implements Baseliner: the child's.
func (f *FocusWidget) Baseline() (float64, bool) { return baselineOf(f.child) }

// Paint implements Widget.
func (f *FocusWidget) Paint(dst *Canvas, r Rect) {
	dst.HitKey(r, f)
	dst.Paint(f.child, r)
}

// applyRevealRequests runs the Canvas.RequestReveal calls this paint made.
func (in *inputState) applyRevealRequests(f *frameState) {
	for _, r := range f.revealRequests {
		in.reveal(r)
	}
	f.revealRequests = f.revealRequests[:0]
}

func (in *inputState) applyFocusRequest(f *frameState) {
	h := f.focusRequest
	f.focusRequest = nil
	if h == nil {
		return
	}
	for i := range in.regions {
		r := &in.regions[i]
		if sameAny(r.key, h) {
			if scope := in.activeScope(); scope != nil && r.scope != scope {
				return
			}
			in.focus(r, true)
			return
		}
	}
}

// takeTouched returns the regions pointer events reached since the last
// call, and false when one cannot be placed in the window.
func (in *inputState) takeTouched() ([]Rect, bool) {
	t, ok := in.touched, !in.touchedUnplaced
	in.touched, in.touchedUnplaced = in.touched[:0], false
	return t, ok
}

// touchInput maps a primary finger to the single pointer used by widgets.
// Once it lifts, wait for all fingers to lift before starting another press.
type touchInput struct {
	id              ggwd.TouchID
	active, waiting bool
	pos             Point
}

func (t *touchInput) apply(f *frameInput, ids []ggwd.TouchID, position func(ggwd.TouchID) Point) {
	if !t.active && !t.waiting && len(ids) == 0 {
		return
	}
	// Ignore mouse edges while handling touch, including the release frame.
	f.down, f.up = nil, nil
	if t.waiting {
		f.pos = t.pos
		t.waiting = len(ids) != 0
		return
	}
	f.touch = true
	if !t.active {
		t.id, t.active = ids[0], true
		f.down = []MouseButton{MouseButtonLeft}
	}
	if slices.Contains(ids, t.id) {
		t.pos = position(t.id)
	} else {
		// ggfx no longer exposes the position of a released touch.
		// Keep its last position instead of releasing at the mouse cursor.
		f.up = []MouseButton{MouseButtonLeft}
		t.active, t.waiting = false, len(ids) != 0
	}
	f.pos = t.pos
}

// panTouch lets a scroll region take over a touch once kinetics.Pan says
// it pans.
// Hit testing stays at the gesture origin; a captured scroller continues
// receiving movement even when the finger leaves its viewport.
func (in *inputState) panTouch(f *frameInput) {
	now := activeWorld().frame.raw()
	if len(f.down) != 0 || f.scrolled() || len(f.keys) != 0 {
		in.touchMotion.stop()
	}
	in.stepTouchMomentum(now)
	if !f.touch {
		return
	}
	if len(f.down) != 0 {
		in.touchPan.Down(f.pos)
		in.touchScroll = nil
		in.touchMotion = touchMotion{Tracker: kinetics.Tracker{Last: now, Pos: f.pos}}
		return
	}
	// Assert before re-resolving: the scan only pays off for the rare control
	// that captures drags, and it runs on every frame of a held finger.
	if in.pressed != nil {
		if _, ok := in.pressed.pointer.(TouchDragCapturer); ok {
			if cur := in.findPointer(in.pressed); cur != nil {
				if capturer, ok := cur.pointer.(TouchDragCapturer); ok && capturer.CaptureTouchDrag() {
					return
				}
			}
		}
	}
	in.touchMotion.Sample(now, f.pos, maxFrameStep)
	delta, ok := in.touchPan.Move(f.pos)
	if !ok {
		return
	}
	ev := PointerEvent{Kind: PointerScroll, Pos: in.touchPan.Start, Scroll: delta, ScrollPixels: true}
	if in.touchPan.Panning {
		if cur := in.findPointer(in.touchScroll); cur != nil && delta != (Point{}) {
			cur.pointer.HandlePointer(ev)
		}
	} else if delta != (Point{}) {
		if target := in.send(ev); target != nil {
			in.touchScroll, in.touchPan.Panning = keep(target), true
			in.releasePress(f.pos, f.mods)
		}
	}
	if in.touchPan.Panning {
		// The original target has already been released. Never generate a tap.
		ended := len(f.up) != 0
		f.down, f.up = nil, nil
		if ended {
			in.touchMotion.Boost()
			in.touchMotion.target = in.touchScroll
			in.touchMotion.origin = in.touchPan.Start
			in.touchMotion.Last = now
			in.touchPan.Panning, in.touchScroll = false, nil
		}
	}
}

// touchMotion is a touch's velocity, tracked while the finger is down,
// and the fling that carries on from it after release: at target, from
// origin. Finger-down movement stays one-to-one; only released motion is
// decelerated.
type touchMotion struct {
	kinetics.Tracker
	origin Point
	target *hitRegion
}

func (m *touchMotion) stop() { m.target, m.Velocity = nil, Point{} }

// stepTouchMomentum advances a fling to now, scrolling its target by what
// it travels since the last step.
func (in *inputState) stepTouchMomentum(now time.Time) {
	m := &in.touchMotion
	if m.target == nil {
		return
	}
	dt := now.Sub(m.Last).Seconds()
	if dt <= 0 {
		return
	}
	// Background tabs should not resume an old fling or jump forward.
	if dt > maxFrameStep.Seconds() || !kinetics.Flinging(m.Velocity) {
		m.stop()
		return
	}
	cur := in.findPointer(m.target)
	if cur == nil {
		m.stop()
		return
	}
	if scope := in.activeScope(); scope != nil && (cur.scope == nil || !sameAny(scope.owner, cur.scope.owner)) {
		m.stop()
		return
	}
	travel, next, finished := kinetics.Fling(m.Velocity, dt)
	ev := PointerEvent{Kind: PointerScroll, Pos: m.origin, Scroll: travel, ScrollPixels: true, ScrollMomentum: true}
	m.Last, m.Velocity = now, next
	if moved := cur.pointer.HandlePointer(ev); !moved || finished {
		m.stop()
	}
}
