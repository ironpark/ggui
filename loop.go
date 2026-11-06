package ggui

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/runtime"
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

// frameLoop is the part of a frame loop App and Probe share: the root owner
// the tree is built under, the posted work, and the record of the last
// layout, which lets a still frame skip layout.
type frameLoop struct {
	// contained are the window regions changes made in reactive.Contained
	// since the last frame show in; see damage.go.
	contained []Rect
	// freeWriters are who wrote outside Contained while watchingFree,
	// since the last frame; see watchFree.
	freeWriters  []uintptr
	watchingFree bool
	host         Host // the Window or Probe this loop is, for UseHost
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
	// semBuild is the scratch publishSemantics builds each tree with, so a
	// frame that changed a row copies one page of nodes and nothing else.
	semBuild a11y.Builder
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

// publishSemantics freezes what the frame just painted into a tree and
// makes it the one readers see. focused is the region holding keyboard
// focus, which only the UI goroutine may look at, mirrored into the tree
// so that everyone else can.
func (l *frameLoop) publishSemantics(c *Canvas, focused *hitRegion) {
	l.sem.Store(buildSemTree(c, focused, l.sem.Load(), &l.semBuild))
}

// announce queues something to say out loud. It shares the post lock, as
// it is called from the same places and just as rarely.
func (l *frameLoop) announce(a Announcement) {
	if a.Text == "" {
		return
	}
	l.postMu.Lock()
	if !l.closed {
		l.notices = append(l.notices, a)
	}
	l.postMu.Unlock()
}

// takeAnnouncements empties the queue. App.Draw hands what it returns to
// the accessibility bridge once a frame, after the tree is published.
func (l *frameLoop) takeAnnouncements() []Announcement {
	l.postMu.Lock()
	out := l.notices
	l.notices = nil
	l.postMu.Unlock()
	return out
}

// semantics returns the last published tree, or an empty one before the
// first frame.
func (l *frameLoop) semantics() *SemTree {
	if t := l.sem.Load(); t != nil {
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
func (l *frameLoop) runtime() *reactive.Runtime {
	if l == nil || l.rt == nil {
		return reactive.Base()
	}
	return l.rt
}

// world is the world of the loop's runtime.
func (l *frameLoop) world() *world { return worldOf(l.runtime()) }

// start runs the setup functions and the builder under a fresh root owner.
// Everything they create lives until close.
func (l *frameLoop) start() {
	setRunning(l)
	l.dispose = l.runtime().Root(func() {
		reactive.CurrentOwner().SetLoop(l)
		l.owner = reactive.CurrentOwner()
		for _, fn := range l.setup {
			fn()
		}
		// Root setup runs once. Reactive blocks own subsequent updates.
		l.root = remembering(l.build)
	})
	l.setup = nil
}

// close disposes the root owner, and with it every effect, memo and
// component the app created, then drops the tree.
func (l *frameLoop) close() {
	l.postMu.Lock()
	if l.closed {
		l.postMu.Unlock()
		return
	}
	l.closed = true
	l.posted, l.notices = nil, nil
	l.postMu.Unlock()
	reactive.ClearRunningLoop(l)
	if l.dispose != nil {
		l.dispose()
	}
	l.root = nil
}

// post queues fn to run on the UI thread before the next frame's input.
func (l *frameLoop) post(fn func()) {
	l.postMu.Lock()
	queued := !l.closed
	if queued {
		l.posted = append(l.posted, fn)
	}
	l.postMu.Unlock()
	// An idle window draws no frames, so work posted from a worker or a
	// browser callback would otherwise wait for the next input.
	if queued && l.wake != nil {
		l.wake()
	}
}

// hasPosted reports whether work is waiting for the next frame.
func (l *frameLoop) hasPosted() bool {
	l.postMu.Lock()
	defer l.postMu.Unlock()
	return len(l.posted) > 0
}

// runFrame runs the OnFrame handlers, in registration order.
func (l *frameLoop) runFrame() {
	for _, fn := range l.frame {
		fn()
	}
}

// runPosted runs the work queued at the start of this frame, in order.
// Work posted by a callback waits for the next frame so input and rendering
// cannot be starved by a callback that posts itself again.
func (l *frameLoop) runPosted() {
	l.postMu.Lock()
	list := l.posted
	l.posted = nil
	l.postMu.Unlock()
	for _, fn := range list {
		if l.closed {
			return
		}
		before := reactive.StateGen()
		fn()
		if reactive.StateGen() != before {
			noteDirtier(dirtySignal, reflect.ValueOf(fn).Pointer())
		}
	}
}

// tick is the shared per-frame step after input: animations advance, then
// effects run until quiet. It returns ErrCycle when they never are.
func (l *frameLoop) tick(now time.Time) error {
	setRunning(l)
	checkStores()
	// Animations made outside every tree, such as one a test built before
	// its probe, belong to a Base runtime's world, which this loop's flushes
	// cover too.
	for _, rt := range l.runtime().Related() {
		if rt != nil {
			worldOf(rt).anims.step(now)
		}
	}
	if rt := l.runtime(); !rt.Flush() {
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
func (l *frameLoop) needsLayout(logical Size) bool {
	gen := layoutGen()
	if logical == l.laidSize && gen == l.laidGen {
		return false
	}
	l.laidSize, l.laidGen = logical, gen
	return true
}

// paintTree paints the laid-out tree and the overlays it queued. A widget
// that configures a child it paints inline, such as a Text stamped in
// several colors, changes only what it draws now: as during layout, the
// write dirties the child's cached measurement but schedules no layout.
// Scheduling one would lay the whole tree out again on every frame.
func (l *frameLoop) paintTree(c *Canvas) {
	defer property.EnterLayout()()
	c.Paint(l.root, Rect{Size: l.rootSize})
	c.paintOverlays()
}

// paintPass is what one paint of a frame takes from the host running it,
// which a Window and a Probe each hold.
type paintPass struct {
	canvas  *Canvas
	in      *inputState
	overlay Overlay
	size    Size // the logical viewport
	tracing bool // the inspector or a sink reads the paint's trace
	semOff  bool // nothing reads the accessibility tree
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
func (l *frameLoop) paint(p paintPass) (bool, error) {
	c := p.canvas
	f := l.beginPaint(c)
	defer endPaint(f)
	// Last frame's regions stay readable while this frame paints, for
	// Adopter handoff, and input keeps routing to them until the paint is
	// done. The buffer of the frame before takes the new regions: input
	// keeps copies of the regions it follows, never a pointer into it.
	spare := f.prev
	f.prev, f.hits = f.hits, spare[:0]
	// A frame that ends before painting leaves the buffers as they were,
	// so the one input routes to is never painted into.
	unswap := func() { f.hits, f.prev = f.prev, spare }
	f.logical = p.size
	f.traceState.reset(p.tracing)
	f.inputView.take(p.in)
	c.nextFrame()
	c.resetSemantics()
	f.semOff = p.semOff
	if err := l.settle(p.size); err != nil {
		unswap()
		return false, err
	}
	if l.closed || l.root == nil {
		unswap()
		return false, nil
	}
	if p.settled != nil {
		p.settled()
	}
	l.paintTree(c)
	p.in.setRegions(f.hits, f.found)
	p.in.observers = f.inputObservers
	p.in.painted = f.shortcuts
	p.in.refreshFocused()
	// A widget that changed its cursor over what the pointer just did, as
	// a link underlined when a key is held, shows it this frame.
	if f.hasPointer {
		p.in.updateCursor(f.pointer)
	}
	p.in.applyFocusRequest(f)
	p.in.applyRevealRequests(f)
	if !p.semOff {
		l.publishSemantics(c, p.in.focused)
	}
	if p.painted != nil {
		p.painted()
	}
	if p.overlay != nil {
		p.overlay.Paint(c)
	}
	return true, nil
}

// beginPaint starts a paint of the tree through c, the root Canvas, whole
// or in part: the Canvases Clip handed out are taken back, the world's
// clock is the one Paint finds, the motion is noted afresh, and no
// BackdropBlur has read the image yet. Defer endPaint with what it returns.
func (l *frameLoop) beginPaint(c *Canvas) *frameState {
	f := c.fs()
	f.resetCanvases()
	f.clock = &l.world().frame
	f.startMotion()
	f.backdrop = false
	return f
}

// endPaint ends the paint beginPaint began: no Canvas Clip handed out
// outlives it, and outside a paint, Paint asks for the clock.
func endPaint(f *frameState) {
	f.resetCanvases()
	f.clock = nil
}

// settle completes structural work, including mounts discovered by layout,
// before running user effects. Both App and Probe use this exact ordering.
func (l *frameLoop) settle(size Size) error {
	setRunning(l)
	rt := l.runtime()
	for range reactive.MaxFlushPasses {
		if l.closed {
			return nil
		}
		if !rt.Flush() {
			return cycle(rt)
		}
		before := reactive.StateGen()
		if l.root != nil && l.needsLayout(size) {
			reactive.WithOwner(l.owner, func() { defer property.EnterLayout()(); l.rootSize = l.root.Layout(Tight(size), rootEnv()) })
		}
		if !rt.Settled() || reactive.StateGen() != before {
			continue
		}
		if !rt.FlushUsers(l) {
			return nil
		}
	}
	return cycle(rt)
}

// A world is what the frame loops of one reactive runtime share beside the
// graph itself: the animations their frames step, the clock those frames
// read, and the groups their input holds busy. It is kept on the runtime,
// so the App's windows share the App's world, each Probe has one of its
// own, and what is made outside every tree lands in the world of its
// goroutine's Base runtime. A probe's animations step in its frames alone
// and Advance moves its clock without moving anyone else's.
//
// A world also holds the settings SetClock, SetDefaultFont and
// SetEmojiFont make, and a test's IME. One it has not set is its parent's:
// the world of the Base its runtime was made beside. Set on the goroutine
// before any App or Probe runs, a setting reaches every one made there;
// set during their frames, it is theirs alone.
type world struct {
	parent *world // nil for a Base's own

	anims animator
	frame frameClock
	busy  map[any]bool // see inputState.markBusyGroups

	env     *StateValue[Env] // see UseEnv; nil until first read
	envMemo envMemo

	font        *Font                      // see SetDefaultFont; nil inherits
	mono        *Font                      // see SetDefaultMonoFont; nil inherits
	nativeFonts bool                       // an App's: defaults to the platform's fonts
	emoji       *fontsys.EmojiChoice       // see SetEmojiFont; nil inherits
	fontGen     uint64                     // counts this world's font and emoji settings
	ime         func(*TextInputWidget) ime // a test's IME; nil inherits
}

// worldOf returns rt's world, making it on first use. A runtime is used by
// one goroutine at a time, so making it needs no lock.
func worldOf(rt *reactive.Runtime) *world {
	if w, ok := rt.Host.(*world); ok {
		return w
	}
	w := &world{busy: map[any]bool{}}
	if p := rt.Parent(); p != nil {
		w.parent = worldOf(p)
		w.frame.parent = &w.parent.frame
	}
	rt.Host = w
	return w
}

// chain yields w and then its parents, nearest first: where a setting w
// has not made is looked up.
func (w *world) chain() iter.Seq[*world] {
	return func(yield func(*world) bool) {
		for ; w != nil && yield(w); w = w.parent {
		}
	}
}

// activeWorld is the world of the loop whose frame the running goroutine
// runs, or ran last: what Now, WakeAt and a painting widget reach. Before
// any frame on the goroutine, it is its Base runtime's.
func activeWorld() *world { return runningLoop().world() }

// ownerWorld is the world of the tree being built now, for something that
// keeps its world from creation on, as an animation does: the current
// owner's loop's, else the active one.
func ownerWorld() *world {
	if l := loopOf(reactive.CurrentOwner()); l != nil {
		return l.world()
	}
	return activeWorld()
}

// maxFrameStep caps how far one frame may move the frame clock. A frame
// comes only when something asks for one, and none while the window is
// minimized, so the frame after a pause sees the whole pause as its delta, and
// without a cap every animation running at the time would teleport to its
// end. The cap turns a pause into a pause instead of a jump, and keeps a
// stiff spring stable after a frame that took too long.
const maxFrameStep = 100 * time.Millisecond

// frameClock is the time a frame's widgets see. It is frozen for the frame
// so that every animation painted in it agrees on the instant, whether it
// is stepped by the runtime or eased inside Paint, and it advances by at
// most maxFrameStep per frame so it never jumps.
type frameClock struct {
	mu   sync.Mutex
	now  time.Time // the frozen instant Now returns
	real time.Time // the raw reading now was last advanced from

	// read reports whether something painted this frame depends on time,
	// so the next frame must run without waiting for input: Now was called,
	// or a Motion reported that it is still moving.
	read bool

	// hidden counts the paints in progress of widgets wholly outside their
	// clip. Time read there does not mark the frame: nothing it moves can
	// show, and the frame that scrolls it into view resumes it.
	hidden int

	// marks counts the times something painted where it can be seen read
	// the time or asked to be painted again at one. Canvas.Paint compares
	// it across a widget's paint to learn that the widget moves with time,
	// and where: the place a frame that only time drives redraws.
	marks atomic.Uint64

	// wake is the earliest instant a widget asked to be painted again at,
	// or zero. It is for looks that change at a known time, like a caret
	// blink or a tooltip delay, which need no frame until then.
	wake time.Time

	// pinned is the raw time this clock advances from, or zero; see raw.
	// Probe.Advance sets it.
	pinned time.Time

	// source is the raw time source SetClock installed, or nil to take the
	// parent's: the clock of the world this clock's world inherits from.
	source func() time.Time
	parent *frameClock
}

// raw is the time the frame clock advances from: the time a Probe's
// Advance pinned, else the source SetClock installed here or on a parent,
// else time.Now. Animations do not read it directly: they read Now, the
// instant the current frame froze. A parent is read without its lock, as
// it belongs to the same goroutine.
func (f *frameClock) raw() time.Time {
	if !f.pinned.IsZero() {
		return f.pinned
	}
	for c := f; c != nil; c = c.parent {
		if c.source != nil {
			return c.source()
		}
	}
	return time.Now()
}

// timeRead reports whether the frame depends on time.
func (f *frameClock) timeRead() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read
}

// animate marks the frame as depending on time.
func (f *frameClock) animate() {
	f.mu.Lock()
	if f.hidden == 0 {
		f.read = true
		f.marks.Add(1)
	}
	f.mu.Unlock()
}

// hide and unhide bracket the paint of a widget no one can see.
func (f *frameClock) hide() {
	f.mu.Lock()
	f.hidden++
	f.mu.Unlock()
}

func (f *frameClock) unhide() {
	f.mu.Lock()
	f.hidden--
	f.mu.Unlock()
}

// takeWake returns the earliest wake-up asked for this frame, or zero.
func (f *frameClock) takeWake() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := f.wake
	f.wake = time.Time{}
	return w
}

// peek returns the frame's instant without marking the frame as depending
// on time, for callers that report their own motion.
func (f *frameClock) peek() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.now.IsZero() {
		return f.raw()
	}
	return f.now
}

// begin moves the clock forward by the time that has passed since the last
// frame, at most maxFrameStep, and returns the instant this frame reads.
// App calls it once per frame, before input.
func (f *frameClock) begin(raw time.Time) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.read = false
	f.wake = time.Time{}
	switch {
	case f.now.IsZero():
		f.now = raw
	case raw.After(f.real):
		f.now = f.now.Add(min(raw.Sub(f.real), maxFrameStep))
	}
	f.real = raw
	return f.now
}

// set puts the clock at raw exactly, with no cap, for Probe: a headless
// frame has no pause to guard against, and a test wants the step it asked
// for however large.
func (f *frameClock) set(raw time.Time) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now, f.real = raw, raw
	return raw
}

// setSource installs the raw time source, nil to take the parent's, and
// returns the one before. The clock starts afresh from it.
func (f *frameClock) setSource(fn func() time.Time) (prev func() time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prev, f.source = f.source, fn
	f.now, f.real = time.Time{}, time.Time{}
	return prev
}

// instant returns the current frame's time, or the raw clock while no
// frame has begun.
func (f *frameClock) instant() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hidden == 0 {
		f.read = true
		f.marks.Add(1)
	}
	if f.now.IsZero() {
		return f.raw()
	}
	return f.now
}

// Now returns the instant the current frame reads: sampled once per frame
// and frozen, so two widgets animating together stay in step, and never
// advancing more than maxFrameStep at a time, so nothing jumps when the
// window comes back from being hidden. Widgets that ease a value should
// read Now rather than time.Now, which also lets tests step them with
// Probe.Advance or SetClock. A read while painting a widget wholly outside
// its clip, such as a card scrolled out of view, does not ask for another
// frame.
func Now() time.Time { return activeWorld().frame.instant() }

// FrameTime is Now without the promise to paint again: reading it does not
// keep the app animating. Use it for the instant handed to a Motion, which
// asks for frames itself while it moves, or together with WakeAt.
func FrameTime() time.Time { return activeWorld().frame.peek() }

// Paced is Now for a look that loops on with no end, a spinner or a pulse:
// it asks for the next frame at the next multiple of every, rather than at
// every refresh of the display. A loop needs no more than 20 or 30 steps a
// second, and each step paints a frame, which the display would otherwise
// ask for 120 times a second while one shows.
func Paced(every time.Duration) time.Time {
	now := FrameTime()
	if every <= 0 {
		return Now()
	}
	WakeAt(now.Truncate(every).Add(every))
	return now
}

// WakeAt asks for a frame at t, for a look that changes at a known time
// such as a caret blink or a tooltip delay. Nothing is painted before then
// unless something else asks. The earliest request in a frame wins.
func WakeAt(t time.Time) {
	if t.IsZero() {
		return
	}
	f := &activeWorld().frame
	f.mu.Lock()
	if f.wake.IsZero() || t.Before(f.wake) {
		f.wake = t
	}
	if f.hidden == 0 {
		f.marks.Add(1)
	}
	f.mu.Unlock()
}

// blinkWake returns when a caret that toggles every period since start next
// changes, for WakeAt.
func blinkWake(start, now time.Time, period time.Duration) time.Time {
	n := now.Sub(start)/period + 1
	return start.Add(n * period)
}

// SetClock replaces the raw time source behind Now and every animation,
// and returns a function that restores the previous one. The frame clock
// starts again from the new source, so the next frame reads whatever it
// returns. A Probe that Advance has moved keeps the clock Advance gave it.
//
// Like SetEnv, it sets the clock of the App or Probe whose frames run on
// this goroutine, or, before any has, the clock every App and Probe made
// on the goroutine reads until it is given its own, so tests that set
// their own clocks may run in parallel. It is for tests:
//
//	restore := ggui.SetClock(func() time.Time { return now })
//	defer restore()
func SetClock(fn func() time.Time) (restore func()) {
	f := &activeWorld().frame
	prev := f.setSource(fn)
	return func() { f.setSource(prev) }
}
