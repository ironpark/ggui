package ggui

import (
	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/runtime"

	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggui/internal/property"
)

// Threading: signals, effects, layout and paint all run on the UI thread,
// the one ggfx delivers the window's events on. A goroutine that has a
// result hands it back with App.Post (Probe.Post in tests), and the posted
// function runs on the UI thread before the next frame's input. The mutexes
// inside StateValue and the effect set are cheap protection against a stray
// read from another goroutine, not a license to write from one: Set and
// Update from a goroutine race with the frame, so post them instead.

// ErrCycle is what App.Run returns, and Probe panics with, when the effects
// of a frame never settle: an Effect writes a StateValue that, through other
// effects and memos, marks the same Effect dirty again. Break the loop with
// Untrack on the read that should not subscribe.
//
// The value carries which effects the cycle runs through, so match it with
// errors.Is(err, ggui.ErrCycle) rather than ==, and print the error itself
// for the detail. Build with -tags ggui_debug and each one is named by the
// file and line that created it.
var ErrCycle = errors.New("ggui: effects did not settle after " + strconv.Itoa(reactive.MaxFlushPasses) + " passes; an Effect is writing a StateValue it reads")

// cycleError is ErrCycle with the effects that would not settle.
type cycleError struct{ msg string }

func (e *cycleError) Error() string { return e.msg }
func (e *cycleError) Unwrap() error { return ErrCycle }

// cycle describes the effects a flush just gave up on. It is built only on
// that path, so a settled frame pays nothing for it.
func cycle(rt *reactive.Runtime) error {
	stuck, total := rt.Unsettled()
	var b strings.Builder
	b.WriteString(ErrCycle.Error())
	fmt.Fprintf(&b, "\n  %d of %d effects never settled", len(stuck), total)
	named := 0
	for _, e := range stuck {
		where := e.Origin()
		if where == "" {
			continue
		}
		named++
		b.WriteString("\n    - ")
		b.WriteString(where)
		if e.Derived() {
			b.WriteString(" (a derived value)")
		}
	}
	if named == 0 {
		b.WriteString("\n  Build with -tags ggui_debug to see where each was created.")
	}
	return &cycleError{msg: b.String()}
}

// Dialogs is the host's file and message dialogs: the platform's under an App, a
// runtime.StubFilePicker under a Probe, or what App.SetDialogs installed.
func (l *frameLoop) Dialogs() runtime.Dialogs { return l.dialogs }

// OnCloseRequest registers fn to decide whether the window closes when the
// user asks to close it: its close button, or the platform's quit command.
// fn runs on the UI thread at the start of the next frame. Return true to
// let the window close, or false to keep it open, for instance to ask
// "Discard unsaved changes?" and call Close once the user confirms; Close
// does not ask again. The handlers run in order and the first false stops
// the rest, so two of them never both put up a question. A browser tab
// does not report the request, so there the window closes unasked.
func (l *frameLoop) OnCloseRequest(fn func() bool) {
	l.closeRequests = append(l.closeRequests, fn)
}

// closeAllowed runs the close handlers and reports whether they all agree.
func (l *frameLoop) closeAllowed() bool {
	for _, fn := range l.closeRequests {
		if !fn() {
			return false
		}
	}
	return true
}

// Clipboard is the host's clipboard: the system's under an App, a
// runtime.MemoryClipboard of its own under a Probe, or what
// App.SetClipboard installed.
func (l *frameLoop) Clipboard() runtime.Clipboard { return l.clipboard }

// frameLoop is the part of a frame loop App and Probe share: the root owner
// the tree is built under, the posted work, and the record of the last
// layout, which lets a still frame skip layout.
type frameLoop struct {
	host Host // the Window or Probe this loop is, for UseHost
	// rt is the reactive runtime the tree is built in and flushed by: a
	// Probe's own, so that its effects run in its frames alone, or the
	// App's, which every window of it shares. Nil stands for the running
	// goroutine's Base.
	rt      *reactive.Runtime
	build   Builder
	setup   []func()
	root    Widget
	owner   *reactive.Computation
	dispose func()
	closed  bool

	postMu sync.Mutex
	posted []func()
	wake   func()   // asks the host for a frame; nil under a Probe
	frame  []func() // OnFrame handlers, run at the start of every frame

	closeRequests []func() bool // OnCloseRequest handlers, in order

	laidSize Size
	laidGen  uint64
	rootSize Size

	notices []Announcement // queued by Announce, drained by the bridge

	dialogs   runtime.Dialogs   // the host's dialogs; see Host.Dialogs
	clipboard runtime.Clipboard // the host's clipboard; see Host.Clipboard

	// The last frame's finished accessibility tree. It is published at the
	// end of a frame and read from anywhere, including a thread that is not
	// this one, which is why it is a pointer swap and not a buffer.
	sem atomic.Pointer[SemTree]
}

// publishSemantics freezes what the frame just painted into a tree and
// makes it the one readers see. focused is the region holding keyboard
// focus, which only the UI goroutine may look at, mirrored into the tree
// so that everyone else can.
func (r *frameLoop) publishSemantics(c *Canvas, focused *hitRegion) {
	r.sem.Store(buildSemTree(c, focused, r.sem.Load()))
}

// announce queues something to say out loud. It shares the post lock, as
// it is called from the same places and just as rarely.
func (r *frameLoop) announce(a Announcement) {
	if a.Text == "" {
		return
	}
	r.postMu.Lock()
	if !r.closed {
		r.notices = append(r.notices, a)
	}
	r.postMu.Unlock()
}

// takeAnnouncements empties the queue. App.Draw hands what it returns to
// the accessibility bridge once a frame, after the tree is published.
func (r *frameLoop) takeAnnouncements() []Announcement {
	r.postMu.Lock()
	out := r.notices
	r.notices = nil
	r.postMu.Unlock()
	return out
}

// semantics returns the last published tree, or an empty one before the
// first frame.
func (r *frameLoop) semantics() *SemTree {
	if t := r.sem.Load(); t != nil {
		return t
	}
	return a11y.Build(nil, -1)
}

// runningLoop is the loop whose frame the running goroutine executes, or
// executed last: what UseHost falls back to and whose clock Now reads. It
// is kept per goroutine, so that probes on goroutines of their own, as
// t.Parallel runs them, each see their own. One app runs per process; a
// Probe is a test's, and the last one to start a frame on a goroutine is
// the one a component built there in that frame belongs to.
func runningLoop() *frameLoop {
	l, _ := reactive.RunningLoop().(*frameLoop)
	return l
}

// setRunning makes l the running goroutine's loop.
func setRunning(l *frameLoop) { reactive.SetRunningLoop(l) }

// UIThread returns the current owner's dispatcher. Capture it during app or
// component setup, then call it from a worker to deliver immutable results.
// App closure drops queued work; use Resource for component-scoped cancellation.
func UIThread() func(func()) {
	reactive.CheckUIThread("UIThread")
	owner := reactive.CurrentOwner()
	post := loopPost(owner)
	if post == nil {
		panic("ggui: UIThread requires an app or mounted component owner")
	}
	return post
}

// loopPost is the owner's frame loop dispatcher, or nil when it has none.
// An effect holds its loop as an opaque token, so this is the one place
// that turns it back into the concrete loop.
func loopPost(owner *reactive.Computation) func(func()) {
	if l := loopOf(owner); l != nil {
		return l.post
	}
	return nil
}

// loopOf is the frame loop owner belongs to, or nil when it has none.
func loopOf(owner *reactive.Computation) *frameLoop {
	if owner == nil {
		return nil
	}
	l, _ := owner.Loop().(*frameLoop)
	return l
}

// runtime is the reactive runtime this loop flushes: the goroutine's Base
// for a loop made with none, or no loop at all.
func (r *frameLoop) runtime() *reactive.Runtime {
	if r == nil || r.rt == nil {
		return reactive.Base()
	}
	return r.rt
}

// world is the world of the loop's runtime.
func (r *frameLoop) world() *world { return worldOf(r.runtime()) }

// start runs the setup functions and the builder under a fresh root owner.
// Everything they create lives until close.
func (r *frameLoop) start() {
	setRunning(r)
	r.dispose = r.runtime().Root(func() {
		reactive.CurrentOwner().SetLoop(r)
		r.owner = reactive.CurrentOwner()
		for _, fn := range r.setup {
			fn()
		}
		// Root setup runs once. Reactive blocks own subsequent updates.
		r.root = r.build()
	})
	r.setup = nil
}

// close disposes the root owner, and with it every effect, memo and
// component the app created, then drops the tree.
func (r *frameLoop) close() {
	r.postMu.Lock()
	if r.closed {
		r.postMu.Unlock()
		return
	}
	r.closed = true
	r.posted, r.notices = nil, nil
	r.postMu.Unlock()
	reactive.ClearRunningLoop(r)
	if r.dispose != nil {
		r.dispose()
	}
	r.root = nil
}

// post queues fn to run on the UI thread before the next frame's input.
func (r *frameLoop) post(fn func()) {
	r.postMu.Lock()
	queued := !r.closed
	if queued {
		r.posted = append(r.posted, fn)
	}
	r.postMu.Unlock()
	// An idle window draws no frames, so work posted from a worker or a
	// browser callback would otherwise wait for the next input.
	if queued && r.wake != nil {
		r.wake()
	}
}

// runFrame runs the OnFrame handlers, in registration order.
// hasPosted reports whether work is waiting for the next frame.
func (r *frameLoop) hasPosted() bool {
	r.postMu.Lock()
	defer r.postMu.Unlock()
	return len(r.posted) > 0
}

func (r *frameLoop) runFrame() {
	for _, fn := range r.frame {
		fn()
	}
}

// runPosted runs the work queued at the start of this frame, in order.
// Work posted by a callback waits for the next frame so input and rendering
// cannot be starved by a callback that posts itself again.
func (r *frameLoop) runPosted() {
	r.postMu.Lock()
	list := r.posted
	r.posted = nil
	r.postMu.Unlock()
	for _, fn := range list {
		if r.closed {
			return
		}
		fn()
	}
}

// tick is the shared per-frame step after input: animations advance, then
// effects run until quiet. It returns ErrCycle when they never are.
func (r *frameLoop) tick(now time.Time) error {
	setRunning(r)
	// Animations made outside every tree, such as one a test built before
	// its probe, belong to a Base runtime's world, which this loop's flushes
	// cover too.
	for _, rt := range r.runtime().Related() {
		if rt != nil {
			worldOf(rt).anims.step(now)
		}
	}
	if rt := r.runtime(); !rt.Flush() {
		return cycle(rt)
	}
	return nil
}

// layoutGen changes whenever something that can move a tree laid out on
// this goroutine changes: a StateValue write or Invalidate here, or a font
// the active world measures with; see fontGen. Both parts only grow, so
// their sum changes whenever either does.
func layoutGen() uint64 { return reactive.LayoutGen() + fontGen() }

// needsLayout reports whether the tree must be laid out again for a
// viewport of the given logical size, and records that it will be: when
// the viewport changed size, or a StateValue was written or Invalidate called
// since the last layout. A rebuilt root is covered, since only a StateValue
// write rebuilds it. Hover and press live outside signals and only change
// how a widget paints, so a still frame costs no layout.
func (r *frameLoop) needsLayout(logical Size) bool {
	gen := layoutGen()
	if logical == r.laidSize && gen == r.laidGen {
		return false
	}
	r.laidSize, r.laidGen = logical, gen
	return true
}

// paintTree paints the laid-out tree and the overlays it queued. A widget
// that configures a child it paints inline, such as a Text stamped in
// several colors, changes only what it draws now: as during layout, the
// write dirties the child's cached measurement but schedules no layout.
// Scheduling one would lay the whole tree out again on every frame.
func (r *frameLoop) paintTree(c *Canvas) {
	defer property.EnterLayout()()
	c.Paint(r.root, Rect{Size: r.rootSize})
	c.paintOverlays()
}

// paintPass is what one paint of a frame takes from the host running it,
// which a Window and a Probe each hold.
type paintPass struct {
	canvas  *Canvas
	in      *inputState
	overlay Overlay
	size    Size        // the logical viewport
	hits    []hitRegion // a spare buffer for the regions the paint registers
	tracing bool        // the inspector or a sink reads the paint's trace
	semOff  bool        // nothing reads the accessibility tree
	// settled runs after the settle, before the paint; it may be nil.
	settled func()
	// painted runs once the tree is painted and the accessibility tree
	// published, before the overlay paints over it, for what the host
	// does with the frame. It may be nil.
	painted func()
}

// paint is the paint half of a frame, the same for a Window and a Probe:
// settle effects and layout at the viewport size, paint the tree and its
// overlays, hand the regions to input, publish the accessibility tree,
// then paint the overlay. It reports false, painting nothing, when the loop
// closed or had no root by the time it settled.
func (r *frameLoop) paint(p paintPass) (bool, error) {
	c := p.canvas
	// Last frame's regions stay readable while this frame paints, for
	// Adopter handoff, and input keeps routing to them until the paint is
	// done.
	c.prev, c.hits = c.hits, p.hits
	f := c.fs()
	f.logical = p.size
	clear(f.trace)
	clear(f.traceWidgets)
	f.tracing = p.tracing
	f.trace, f.traceWidgets = f.trace[:0], f.traceWidgets[:0]
	f.focusBounds = Rect{}
	if p.in.focused != nil {
		f.focusBounds = p.in.focused.rect
	}
	c.nextFrame()
	c.resetSemantics()
	f.semOff = p.semOff
	if err := r.settle(p.size); err != nil {
		return false, err
	}
	if r.closed || r.root == nil {
		return false, nil
	}
	if p.settled != nil {
		p.settled()
	}
	r.paintTree(c)
	p.in.regions = c.hits
	p.in.observers = c.inputObservers
	p.in.painted = c.shortcuts
	p.in.applyFocusRequest(c)
	if !p.semOff {
		r.publishSemantics(c, p.in.focused)
	}
	if p.painted != nil {
		p.painted()
	}
	if p.overlay != nil {
		p.overlay.Paint(c)
	}
	return true, nil
}

// settle completes structural work, including mounts discovered by layout,
// before running user effects. Both App and Probe use this exact ordering.
func (r *frameLoop) settle(size Size) error {
	setRunning(r)
	rt := r.runtime()
	for range reactive.MaxFlushPasses {
		if r.closed {
			return nil
		}
		if !rt.Flush() {
			return cycle(rt)
		}
		before := reactive.StateGen()
		if r.root != nil && r.needsLayout(size) {
			reactive.WithOwner(r.owner, func() { defer property.EnterLayout()(); r.rootSize = r.root.Layout(Tight(size), rootEnv()) })
		}
		if !rt.Settled() || reactive.StateGen() != before {
			continue
		}
		if !rt.FlushUsers(r) {
			return nil
		}
	}
	return cycle(rt)
}
