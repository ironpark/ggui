package ggui

import (
	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/runtime"

	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggui/internal/property"
)

// Threading: signals, effects, layout and paint all run on the UI thread,
// the one Ebitengine calls Update and Draw on. A goroutine that has a
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
var ErrCycle = errors.New("ggui: effects did not settle after " + itoa(maxFlushPasses) + " passes; an Effect is writing a StateValue it reads")

// cycleError is ErrCycle with the effects that would not settle.
type cycleError struct{ msg string }

func (e *cycleError) Error() string { return e.msg }
func (e *cycleError) Unwrap() error { return ErrCycle }

// cycle describes the effects a flush just gave up on. It is built only on
// that path, so a settled frame pays nothing for it.
func cycle() error {
	stuck, total := effects.unsettled()
	var b strings.Builder
	b.WriteString(ErrCycle.Error())
	fmt.Fprintf(&b, "\n  %d of %d effects never settled", len(stuck), total)
	named := 0
	for _, e := range stuck {
		where := e.origin
		if where == "" {
			continue
		}
		named++
		b.WriteString("\n    - ")
		b.WriteString(where)
		if e.cell != nil {
			b.WriteString(" (a derived value)")
		}
	}
	if named == 0 {
		b.WriteString("\n  Build with -tags ggui_debug to see where each was created.")
	}
	return &cycleError{msg: b.String()}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// Dialogs is the host's file dialogs: the platform's under an App, a
// runtime.StubFilePicker under a Probe, or what App.SetDialogs installed.
func (l *frameLoop) Dialogs() runtime.FilePicker { return l.dialogs }

// frameLoop is the part of a frame loop App and Probe share: the root owner
// the tree is built under, the posted work, and the record of the last
// layout, which lets a still frame skip layout.
type frameLoop struct {
	build   Builder
	setup   []func()
	root    Widget
	owner   *effect
	dispose func()
	closed  bool

	postMu sync.Mutex
	posted []func()
	frame  []func() // OnFrame handlers, run at the start of every frame

	laidSize Size
	laidGen  uint64
	rootSize Size

	notices []Announcement // queued by Announce, drained by the bridge

	dialogs runtime.FilePicker // the host's file dialogs; see Host.Dialogs

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

// running is the loop whose frame is executing: what UIThread hands out. It
// is written on the UI goroutine at the start of a frame and read there too,
// and is a pointer swap only so that a test holding two probes cannot tear
// it. One app runs per process; a Probe is a test's, and the last one to
// start a frame is the one a component built in that frame belongs to.
var running atomic.Pointer[frameLoop]

// UIThread returns the current owner's dispatcher. Capture it during app or
// component setup, then call it from a worker to deliver immutable results.
// App closure drops queued work; use Resource for component-scoped cancellation.
func UIThread() func(func()) {
	checkUIThread("UIThread")
	owner := currentOwner()
	if owner == nil || owner.loop == nil {
		panic("ggui: UIThread requires an app or mounted component owner")
	}
	return owner.loop.post
}

// start runs the setup functions and the builder under a fresh root owner.
// Everything they create lives until close.
func (r *frameLoop) start() {
	running.Store(r)
	r.dispose = Root(func() {
		currentOwner().loop = r
		r.owner = currentOwner()
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
	running.CompareAndSwap(r, nil)
	if r.dispose != nil {
		r.dispose()
	}
	r.root = nil
}

// post queues fn to run on the UI thread before the next frame's input.
func (r *frameLoop) post(fn func()) {
	r.postMu.Lock()
	if !r.closed {
		r.posted = append(r.posted, fn)
	}
	r.postMu.Unlock()
}

// runFrame runs the OnFrame handlers, in registration order.
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
	running.Store(r)
	anims.step(now)
	if !effects.flush() {
		return cycle()
	}
	return nil
}

// needsLayout reports whether the tree must be laid out again for a
// viewport of the given logical size, and records that it will be: when
// the viewport changed size, or a StateValue was written or Invalidate called
// since the last layout. A rebuilt root is covered, since only a StateValue
// write rebuilds it. Hover and press live outside signals and only change
// how a widget paints, so a still frame costs no layout.
func (r *frameLoop) needsLayout(logical Size) bool {
	gen := layoutGen.Load()
	if logical == r.laidSize && gen == r.laidGen {
		return false
	}
	r.laidSize, r.laidGen = logical, gen
	return true
}

// settle completes structural work, including mounts discovered by layout,
// before running user effects. Both App and Probe use this exact ordering.
func (r *frameLoop) settle(size Size) error {
	running.Store(r)
	for range maxFlushPasses {
		if r.closed {
			return nil
		}
		if !effects.flush() {
			return cycle()
		}
		before := stateGen
		if r.root != nil && r.needsLayout(size) {
			withOwner(r.owner, func() { defer property.EnterLayout()(); r.rootSize = r.root.Layout(Tight(size), rootEnv()) })
		}
		if effects.dirtyGen != effects.settledGen || stateGen != before {
			continue
		}
		if !effects.flushUsers(r) {
			return nil
		}
	}
	return cycle()
}
