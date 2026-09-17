package ggui

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Threading: signals, effects, layout and paint all run on the UI thread,
// the one Ebitengine calls Update and Draw on. A goroutine that has a
// result hands it back with App.Post (Probe.Post in tests), and the posted
// function runs on the UI thread before the next frame's input. The mutexes
// inside Signal and the effect set are cheap protection against a stray
// read from another goroutine, not a license to write from one: Set and
// Update from a goroutine race with the frame, so post them instead.

// ErrCycle is the error App.Run returns, and Probe panics with, when the
// effects of a frame never settle: an Effect writes a Signal that, through
// other effects and memos, marks the same Effect dirty again. Break the
// loop with Untrack or Peek on the read that should not subscribe.
var ErrCycle = errors.New("ggui: effects did not settle after " + itoa(maxFlushPasses) + " passes; an Effect is writing a Signal it reads")

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

// frameLoop is the part of a frame loop App and Probe share: the root owner
// the tree is built under, the posted work, and the record of the last
// layout, which lets a still frame skip layout.
type frameLoop struct {
	build   Builder
	setup   []func()
	root    Widget
	dispose func()
	closed  bool

	postMu sync.Mutex
	posted []func()

	laidSize Size
	laidGen  uint64
	rootSize Size

	notices []Announcement // queued by Announce, drained by the bridge

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
	r.sem.Store(buildSemTree(c, focused))
}

// announce queues something to say out loud. It shares the post lock, as
// it is called from the same places and just as rarely.
func (r *frameLoop) announce(a Announcement) {
	if a.Text == "" {
		return
	}
	r.postMu.Lock()
	r.notices = append(r.notices, a)
	r.postMu.Unlock()
}

// takeAnnouncements empties the queue. Nothing calls it yet; the platform
// bridge will, from whichever thread it speaks on.
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
	return &SemTree{focused: -1}
}

// start runs the setup functions and the builder under a fresh root owner.
// Everything they create lives until close.
func (r *frameLoop) start() {
	r.dispose = Root(func() {
		for _, fn := range r.setup {
			fn()
		}
		// The tree is rebuilt through an Effect, so every signal read
		// during build rebuilds the tree when it changes.
		Effect(func() { r.root = r.build() })
	})
	r.setup = nil
}

// close disposes the root owner, and with it every effect, memo and
// component the app created, then drops the tree.
func (r *frameLoop) close() {
	if r.closed {
		return
	}
	r.closed = true
	if r.dispose != nil {
		r.dispose()
	}
	r.root = nil
	r.postMu.Lock()
	r.posted, r.notices = nil, nil
	r.postMu.Unlock()
}

// post queues fn to run on the UI thread before the next frame's input.
func (r *frameLoop) post(fn func()) {
	r.postMu.Lock()
	r.posted = append(r.posted, fn)
	r.postMu.Unlock()
}

// runPosted runs what post queued, in order, including what those
// functions post themselves.
func (r *frameLoop) runPosted() {
	for {
		r.postMu.Lock()
		list := r.posted
		r.posted = nil
		r.postMu.Unlock()
		if len(list) == 0 {
			return
		}
		for _, fn := range list {
			fn()
		}
	}
}

// tick is the shared per-frame step after input: animations advance, then
// effects run until quiet. It returns ErrCycle when they never are.
func (r *frameLoop) tick(now time.Time) error {
	anims.step(now)
	if !effects.flush() {
		return ErrCycle
	}
	return nil
}

// needsLayout reports whether the tree must be laid out again for a
// viewport of the given logical size, and records that it will be: when
// the viewport changed size, or a Signal was written or Invalidate called
// since the last layout. A rebuilt root is covered, since only a Signal
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
