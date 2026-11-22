package ggui

import (
	"slices"

	"github.com/ironpark/ggui/pkg/gem"
	"github.com/ironpark/ggui/pkg/kinetics"
	"github.com/ironpark/ggwd"
)

// PointerKind says what a PointerEvent reports.
type PointerKind int

const (
	PointerDown   PointerKind = iota // a button went down inside the region
	PointerUp                        // a button came up inside the region
	PointerTap                       // down and up happened inside the same region
	PointerMove                      // the cursor is over the region this frame, or over a region painted inside it
	PointerEnter                     // the cursor came over the region, or over a region painted inside it
	PointerExit                      // the cursor left the region and everything painted inside it
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
	// Inner is set on a PointerMove or PointerEnter that reaches a region
	// because the pointer is over a region painted inside it, which claimed
	// the move, rather than over the region itself. A control that should
	// look hovered only where a click would land on it ignores an enter
	// with Inner set and clears its look on a move with it.
	Inner bool
}

// PointerHandler receives pointer events whose position fell inside the
// region it was registered with. Returning true consumes the event; false
// lets the region beneath see it, so a tap target does not block scrolling.
//
// A PointerMove is claimed rather than consumed. The topmost region that
// returns true for it is the one the pointer is over; regions beneath it
// that it is not painted inside hear nothing, and each region it is painted
// inside is offered the move in turn, with Inner set, and is hovered with it
// if it returns true as well. A hovered region hears PointerEnter once as
// that begins and PointerExit once as it ends, however the pointer moves
// among the regions inside it: a row stays hovered over its own buttons, as
// an element does under CSS's :hover. A region is inside another when the
// widget that registered it was painted, through any number of widgets, by
// the one that registered the other.
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

// DragCapturer is a PointerHandler that keeps the drags of a press it took,
// instead of letting a drag source around it pick its child up: a slider's
// knob in a row that drags, a selection in a text field in a tab. Input
// asks the handler holding the press as the pointer moves far enough to
// start a drag; while CaptureDrag reports true the press stays the
// handler's and no drag starts. TouchDragCapturer is the same question for
// a touch's pan.
type DragCapturer interface {
	CaptureDrag() bool
}

// PointerWithinHandler is the handler of a HitWithin region that is offered
// each press and each scroll of the wheel headed for the region or for one
// painted inside it, before any of those hears of it, and after the within
// handlers of the regions it is inside itself. Returning false lets the
// event go on. Returning true takes it: nothing inside hears it, and a
// press taken belongs to the region, whose PointerHandler, if it has one,
// receives the PointerDrag, PointerUp and PointerTap that follow.
type PointerWithinHandler interface {
	HandlePointerWithin(ev PointerEvent) bool
}

// KeyWithinHandler is the handler of a HitWithin region that is offered key
// presses while focus is in the region or inside it, where a shortcut would
// run: a press with a modifier other than Shift before the focused widget
// and the window's shortcuts, unless the widget claims the chord; a bare key
// after the focused widget, when it did not consume it, before a focus
// trap's Escape and the bare-key shortcuts. The handler nearest the focus is
// offered a press first, and returning true takes it from the rest. Tab,
// which moves focus, is not offered.
type KeyWithinHandler interface {
	HandleKeyWithin(ev KeyEvent) bool
}

// FocusWithinHandler is the handler of a HitWithin region that is told when
// keyboard focus comes to a region painted inside its own, or to its own,
// and when it has left them all. It is not told as focus moves from one
// region inside to another.
type FocusWithinHandler interface {
	HandleFocusWithin(within bool)
}

// Mods are the modifier keys held during a KeyEvent. Cmd reports the
// platform's command modifier: Meta (⌘) on macOS, Ctrl elsewhere.
type Mods = ggwd.Mods

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
	pressed    *hitRegion
	pressedBtn MouseButton
	focused    *hitRegion
	// The slot focused is kept in: it is kept again every frame, which a
	// slot does without allocating.
	focusSlot regionSlot
	// hover is the hover path, the hot region first and the regions around
	// it that claimed the move after it; hoverAt is the buffer this frame's
	// is found in, as indices into regions. See updateHover.
	hover   regionPath
	hoverAt []int32
	// withins are the handlers of the HitWithin regions of regions, by
	// index; see withinOf. offered are the regions whose within handlers
	// the event being sent was offered; see offerWithin.
	withins []withinEntry
	offered []int32
	// focusWithin is the focus path: the regions with a FocusWithinHandler
	// on the focused region's chain, the innermost first; focusAt is the
	// buffer this frame's is found in. See updateFocusWithin.
	focusWithin withinPath
	focusAt     []int32

	focusVisible bool        // focused moved there with the keyboard
	cursor       CursorShape // what the hovered region asked for
	dragTarget   *hitRegion  // the region files are being dragged over
	drag         *appDrag    // a drag inside the app; see drag.go
	// latched is, per axis, x then y, the region that took the trackpad
	// gesture under way, kept in latch; lifted, that the fingers of the
	// last gesture lifted. See sendGesture.
	latch       [2]regionSlot
	latched     [2]*hitRegion
	lifted      bool
	touchPan    kinetics.Pan
	touchScroll *hitRegion // what took the pan
	touchMotion touchMotion

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
// holds a press or an in-app drag is active, so that a drag that began in the
// app finishes there.
func (in *inputState) dispatchOver(o Overlay, f frameInput) {
	if o == nil || in.pressed != nil || in.drag != nil || !o.Input(overlayInput(f)) {
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
		if r != nil && r.group != nil && comparableID(r.group) {
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

// updateCursor picks the shape of the topmost region at pos asking for
// one. A region that asks for the default arrow is asking too: under the
// pointer it keeps the shape of a region beneath it, a text field's I-beam
// under a menu say, from showing through.
func (in *inputState) updateCursor(pos Point) {
	in.cursor = CursorShapeDefault
	if !in.index().cursors {
		return
	}
	if r := in.topmost(func(r *hitRegion) bool { return r.shaped && r.contains(pos) }); r != nil {
		in.cursor = r.cursor
	}
}

// dispatchPointer sends this frame's presses, drag, releases and wheel. The
// region that took a press keeps the drag and the release wherever the
// cursor goes, and gets a tap only if the release landed back inside it.
func (in *inputState) dispatchPointer(f frameInput) {
	if len(f.down) > 0 {
		in.unlatch() // a press ends a gesture
	}
	for _, b := range f.down {
		ev := PointerEvent{Kind: PointerDown, Pos: f.pos, Button: b, Mods: f.mods}
		pressed := in.send(ev)
		if in.pressed == nil {
			if b == MouseButtonLeft {
				in.pressDrag(f.pos, pressed)
			}
			in.pressed, in.pressedBtn = keep(pressed), b
		}
		in.setFocus(in.findKey(f.pos))
	}
	if in.pressed != nil && len(f.down) == 0 && len(f.up) == 0 {
		if cur := in.findPointer(in.pressed); cur != nil {
			in.deliver(cur, PointerEvent{Kind: PointerDrag, Pos: f.pos, Button: in.pressedBtn, Mods: f.mods})
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
	in.gesture(f.phase)
	if f.wheel != (Point{}) {
		in.sendScroll(PointerEvent{Kind: PointerScroll, Pos: f.pos, Scroll: f.wheel, scrollBy: f.wheelBy, ScrollStep: f.wheelBy != (Point{}), Mods: f.mods, ScrollMomentum: momentum, ScrollPhase: f.phase})
	}
	if f.wheelPx != (Point{}) || lifted && f.wheel == (Point{}) {
		in.sendScroll(PointerEvent{Kind: PointerScroll, Pos: f.pos, Scroll: f.wheelPx, ScrollPixels: true, Mods: f.mods, ScrollMomentum: momentum, ScrollPhase: f.phase})
	}
	if f.phase == ScrollPhaseMomentumEnded {
		in.unlatch()
	}
}

// sendScroll sends a wheel's or a trackpad's scroll. One that moves along
// both axes goes as two, an axis each, so that each finds the view that
// scrolls its way: a diagonal swipe over a vertical list inside a
// horizontal one moves both, where a single event would stop at the first
// to use either part of it.
//
// A trackpad's gesture stays, an axis each, with the view that took it as
// it began: see sendGesture.
func (in *inputState) sendScroll(ev PointerEvent) {
	if ev.Scroll.X == 0 || ev.Scroll.Y == 0 {
		in.sendGesture(ev)
		return
	}
	x, y := ev, ev
	x.Scroll.Y, x.scrollBy.Y = 0, 0
	y.Scroll.X, y.scrollBy.X = 0, 0
	in.sendGesture(x)
	in.sendGesture(y)
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
	if prev := in.focused.key; !gem.Same(prev, cur.key) {
		// The tree was rebuilt: the region is the same, the widget new.
		// An Adopter already took the old one's state over during paint;
		// anything else is told it has focus now.
		if _, ok := cur.key.(Adopter); !ok {
			RepaintAll()
			cur.key.HandleKey(KeyEvent{Kind: KeyFocus})
		}
	}
	in.focused = in.focusSlot.keep(cur)
	// A rebuild may have painted the focused widget under other regions.
	in.updateFocusWithin(cur)
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

// deliver hands ev to r as handle does, noting where: a widget a pointer
// event reached may look otherwise with no signal written, as a button
// pressed or hovered does, and the frame paints its region again.
//
// A move is handed to the regions under the pointer every frame, moved or
// not, and passes through those that do not claim it: only one claimed
// after the pointer moved can have changed.
func (in *inputState) deliver(r *hitRegion, ev PointerEvent) bool {
	took := r.handle(ev)
	if (ev.Kind == PointerMove || ev.Kind == PointerDrag) && (!took || !in.moved) {
		return took
	}
	in.touch(r)
	return took
}

// touch notes that an event reached r, for the frame to paint it again.
func (in *inputState) touch(r *hitRegion) {
	if r.space != nil {
		in.touchedUnplaced = true
	} else {
		in.touched = append(in.touched, r.rect)
	}
}

// RepaintAll has the frame that follows the input being handled paint the
// whole window, rather than only the regions the input reached. A handler
// calls it when what it did changes how widgets look with no signal
// written, and beyond the region the event was handed to: a click that
// clears a selection running through paragraphs it did not land in. Input
// calls it itself for every focus and blur it hands out. It counts the
// events handed to widgets in the active world, and a frame in which its
// world's count moved is painted whole: a window's frame is not painted
// whole for what a probe or an App on another goroutine handled meanwhile.
// Outside an input handler it has no effect.
func RepaintAll() { activeWorld().delivered++ }

// send offers ev to regions under it from the top down until one consumes it
// and returns that region, or nil. A press or a wheel is offered first to
// the within handlers around each region it reaches; see sendAt.
func (in *inputState) send(ev PointerEvent) *hitRegion {
	if i := in.sendAt(ev); i >= 0 {
		return &in.regions[i]
	}
	return nil
}

// takeTouched returns the regions pointer events reached since the last
// call, and false when one cannot be placed in the window.
func (in *inputState) takeTouched() ([]Rect, bool) {
	t, ok := in.touched, !in.touchedUnplaced
	in.touched, in.touchedUnplaced = in.touched[:0], false
	return t, ok
}
