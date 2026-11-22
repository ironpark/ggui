package ggui

import (
	"image"
	"slices"
	"time"

	"github.com/ironpark/ggui/internal/platform/textinput"
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

// IME is a text widget's session with the platform's input method, through
// which typing reaches it on every platform: composed, as Korean and
// Japanese are, with the composition shown in place, or a keystroke at a
// time. NewIME starts one. The widget calls Update from HandleTick while it
// has the focus, and Confirm before it edits its text itself: a session's
// next commit applies to the text it was last told about.
type IME interface {
	// Update runs the input method for a frame, with the caret where the
	// widget last drew it, in logical pixels, for the candidate window. It
	// reports whether the input method took the frame's keys, which
	// HandleTick returns.
	Update(caret image.Rectangle) (handled bool, err error)
	// Confirm ends the session, committing what was being composed.
	Confirm()
	// Cancel ends the session, dropping what was being composed.
	Cancel()
	// Rebase points the session at new text around the caret, written
	// outside the input method.
	Rebase(before, after string)
	// Typed is the text of a KeyText event. An input method delivers
	// typing through IMEHandler.Commit and ignores it; with none, as under
	// a Probe, Typed commits it.
	Typed(text string)
}

// IMEHandler is how an IME edits the widget it serves. Every function but
// End is required.
type IMEHandler struct {
	// Session starts a session: the caret, in logical pixels, and the text
	// before and after it, which the input method reads for prediction
	// and Replace replaces.
	Session func() (caret image.Rectangle, before, after string)
	// Composition shows text being composed at the caret, with its own
	// caret at a byte offset into it; "" clears it.
	Composition func(text string, caret int)
	// Commit puts text in place of the selection.
	Commit func(text string)
	// Replace puts before, text and after in place of the text the
	// session started with, with the caret after text: an input method
	// rewrote what it was told about, as an autocorrect does.
	Replace func(before, text, after string)
	// End is the user ending input from the platform's side, as by
	// dismissing a virtual keyboard: the widget should give up the focus,
	// since the next session would show the keyboard again.
	End func()
}

// NewIME starts the IME h serves: the one SetIME installed for the tree
// being built, else the platform's input method. That works only while an
// App runs; outside one, as under a Probe, there is no input method:
// Update does nothing and Typed commits what was typed.
func NewIME(h IMEHandler) IME {
	for w := range ownerWorld().chain() {
		if w.ime != nil {
			return w.ime(h)
		}
	}
	return newPlatformIME(h)
}

// SetIME makes fn start the IME of every text widget built from now on, in
// place of the platform's input method, and returns a function that
// restores the one before. Like SetClock, it sets that of the App or Probe
// whose frames run on this goroutine, or, before any has, the one every
// App and Probe made on the goroutine, and every widget built outside
// them, reads. It is for tests, whose fake composes and commits by hand
// through the handler it is given:
//
//	restore := ggui.SetIME(func(h ggui.IMEHandler) ggui.IME { return &fakeIME{h: h} })
//	defer restore()
func SetIME(fn func(IMEHandler) IME) (restore func()) {
	w := activeWorld()
	prev := w.ime
	w.ime = fn
	return func() { w.ime = prev }
}

// platformIME drives textinput.Composer, but only inside a running app:
// the package panics when used before ggfx has chosen its backend.
type platformIME struct {
	textinput.Composer
	h IMEHandler
}

// newPlatformIME builds the platform's IME driver for h, after
// examples/textinput in the Ebitengine repository.
func newPlatformIME(h IMEHandler) *platformIME {
	c := &platformIME{h: h}
	c.OnNewSession = func() *textinput.SessionOptions {
		caret, before, after := h.Session()
		return &textinput.SessionOptions{CaretBounds: caret, TextBeforeCaret: before, TextAfterCaret: after}
	}
	c.OnComposition = func(comp *textinput.Composition) {
		start, _ := comp.SelectionRangeInBytes()
		h.Composition(comp.Text(), start)
	}
	c.OnCommit = func(commit *textinput.Commit) {
		if before, after := commit.IsSurroundingTextReplaced(); before || after {
			nb, na := commit.SurroundingText()
			h.Replace(nb, commit.Text(), na)
			return
		}
		h.Commit(commit.Text())
	}
	c.OnEndByUser = h.End
	return c
}

func (c *platformIME) Rebase(before, after string) { textinput.Rebase(&c.Composer, before, after) }

func (c *platformIME) Update(caret image.Rectangle) (bool, error) {
	textinput.UpdateCaret(caret)
	if !appRunning.Load() {
		return false, nil
	}
	return c.Composer.Update()
}

// Typed commits text typed outside a running app, as under a Probe, where
// there is no platform IME and typed text arrives as KeyText instead, as
// it would without one. Inside one the IME delivers it.
func (c *platformIME) Typed(text string) {
	if !appRunning.Load() {
		c.h.Commit(text)
	}
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
	shaped  bool // the region asks for cursor, the default arrow included
	// within marks a region registered with HitWithin, whose handler is
	// kept beside the list, in inputState.withins: see withinOf.
	within bool
	// parent is the index, plus one, in the same list of the region this
	// one is inside, 0 for none: see parentFor. It is always below the
	// region's own index plus one, so a scan from the end of the list
	// reaches a region before any it is inside. A copy kept past its frame
	// has it cleared, as the list it indexes is gone. It sits beside shaped,
	// in what would be padding.
	parent int32
	scope  *focusScope // the focus trap the region was painted in, if any
	role   Role        // from a Semantic handler
	label  string
	group  any       // set by Group: the EachKeyed entry that painted it
	drag   Draggable // a DragSource beneath its child; see drag.go

	space  *paintSpace // the space it was painted in, nil for the window's
	local  Rect        // rect in that space, clipped there; see contains
	near   Rect        // reach in that space
	origin Point       // full's corner in that space, before placeRegion moved it
}

// merge folds o into r when they share a Rect and o only adds what r lacks.
func (r *hitRegion) merge(o hitRegion) bool {
	if r.rect != o.rect || r.reach != o.reach || r.space != o.space || (o.id != nil && r.id != nil && !gem.Same(o.id, r.id)) ||
		o.drag != nil || r.drag != nil ||
		(o.pointer != nil && r.pointer != nil) ||
		(o.key != nil && r.key != nil) ||
		(o.shaped && r.shaped) ||
		(o.within && r.within) {
		return false
	}
	if o.pointer != nil {
		r.pointer = o.pointer
	}
	if o.key != nil {
		r.key = o.key
	}
	if o.shaped {
		r.cursor, r.shaped = o.cursor, true
	}
	r.within = r.within || o.within
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

// keep returns a copy of r that outlives the regions buffer. The copy
// knows no parent: the list its link indexes is rebuilt by the next paint.
func keep(r *hitRegion) *hitRegion {
	if r == nil {
		return nil
	}
	c := *r
	c.unlink()
	return &c
}

// unlink clears what r knows of the list it was registered in, for a copy
// that outlives it: following a kept copy's link then finds nothing rather
// than a region of another frame.
func (r *hitRegion) unlink() { r.parent = 0 }

// parentOf returns the region r is inside, or nil. r must point into
// in.regions: a kept copy has no parent, the list it indexed being gone.
func (in *inputState) parentOf(r *hitRegion) *hitRegion {
	if r == nil || r.parent == 0 || int(r.parent) > len(in.regions) {
		return nil
	}
	return &in.regions[r.parent-1]
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
	s.buf[s.cur].unlink()
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
	if gem.Same(owner, in.trap) {
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
	inside := func(r *hitRegion) bool { return r.scope != nil && gem.Same(r.scope.owner, owner) }
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
	within  bool // a region was registered with HitWithin
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

// setRegions hands input the regions a paint registered, the handlers of
// those it registered with HitWithin, and what the paint learned of them as
// it did.
func (in *inputState) setRegions(rs []hitRegion, withins []withinEntry, found regionIndex) {
	in.regions, in.withins, in.ix = rs, withins, found
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
		if r.shaped {
			x.cursors = true
		}
		if r.within {
			x.within = true
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
	if hint.h != nil && gem.Same(hint.h, h) {
		if hint.at < 0 {
			return nil
		}
		if r := &in.regions[hint.at]; gem.Same(handlerOf(r, key), h) {
			return r
		}
	}
	at := -1
	for i := len(in.regions) - 1; i >= 0; i-- {
		if r := handlerOf(&in.regions[i], key); r != nil && gem.Same(r, h) {
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
		return in.topmost(func(r *hitRegion) bool { return r.handles(key) && gem.Same(r.id, p.id) })
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
	if r != nil && in.focused != nil && (gem.Same(r.key, in.focused.key) || in.continues(r, in.focused)) {
		// The same widget, perhaps in another region, such as a dropdown's
		// list under its field: focus stays, without a blur, and becomes
		// visible if the keyboard moved it there.
		in.focused = in.focusSlot.keep(r)
		in.focusVisible = in.focusVisible || keyboard
		in.updateFocusWithin(r)
		return
	}
	if in.focused != nil && in.focused.key != nil {
		RepaintAll()
		in.focused.key.HandleKey(KeyEvent{Kind: KeyBlur})
	}
	if r != nil && r.key != nil {
		RepaintAll()
		ev := KeyEvent{Kind: KeyFocus}
		if keyboard {
			ev.Key = KeyTab
			in.reveal(r.full)
		}
		r.key.HandleKey(ev)
	}
	in.focused = in.focusSlot.keep(r)
	in.focusVisible = keyboard
	in.updateFocusWithin(r)
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
		if r.key == nil || (scope != nil && (r.scope == nil || !gem.Same(r.scope.owner, scope.owner))) {
			continue
		}
		if r == focused || focused != nil && gem.Same(r.key, focused.key) {
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
	if (prev.pointer != nil && gem.Same(now.pointer, prev.pointer)) || (prev.key != nil && gem.Same(now.key, prev.key)) {
		return true
	}
	if prev.pointer != nil && in.findPointer(prev) == now {
		return true
	}
	return prev.key != nil && in.findKeyRegion(prev) == now
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
		if gem.Same(r.key, h) {
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
		in.touchMotion = touchMotion{Last: now, Pos: f.pos}
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
			in.deliver(cur, ev)
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
	if scope := in.activeScope(); scope != nil && (cur.scope == nil || !gem.Same(scope.owner, cur.scope.owner)) {
		m.stop()
		return
	}
	travel, next, finished := kinetics.Fling(m.Velocity, dt)
	ev := PointerEvent{Kind: PointerScroll, Pos: m.origin, Scroll: travel, ScrollPixels: true, ScrollMomentum: true}
	m.Last, m.Velocity = now, next
	if moved := in.deliver(cur, ev); !moved || finished {
		m.stop()
	}
}
