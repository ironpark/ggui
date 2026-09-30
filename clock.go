package ggui

import (
	"sync"
	"time"
)

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
	f.read = f.read || f.hidden == 0
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
	f.read = f.read || f.hidden == 0
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
