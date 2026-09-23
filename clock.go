package ggui

import (
	"sync"
	"time"
)

// clock is the raw time source; SetClock replaces it. Animations do not
// read it directly: they read Now, the instant the current frame froze.
var clock = time.Now

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
		return clock()
	}
	return f.now
}

var frame frameClock

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

// reset forgets the frame clock, so the next frame starts afresh from
// whatever source SetClock left behind.
func (f *frameClock) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now, f.real = time.Time{}, time.Time{}
}

// instant returns the current frame's time, or the raw clock while no
// frame has begun.
func (f *frameClock) instant() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.read = f.read || f.hidden == 0
	if f.now.IsZero() {
		return clock()
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
func Now() time.Time { return frame.instant() }

// FrameTime is Now without the promise to paint again: reading it does not
// keep the app animating. Use it for the instant handed to a Motion, which
// asks for frames itself while it moves, or together with WakeAt.
func FrameTime() time.Time { return frame.peek() }

// WakeAt asks for a frame at t, for a look that changes at a known time
// such as a caret blink or a tooltip delay. Nothing is painted before then
// unless something else asks. The earliest request in a frame wins.
func WakeAt(t time.Time) {
	if t.IsZero() {
		return
	}
	frame.mu.Lock()
	if frame.wake.IsZero() || t.Before(frame.wake) {
		frame.wake = t
	}
	frame.mu.Unlock()
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
// returns. It is for tests:
//
//	restore := ggui.SetClock(func() time.Time { return now })
//	defer restore()
func SetClock(fn func() time.Time) (restore func()) {
	prev := clock
	clock = fn
	frame.reset()
	return func() { clock = prev; frame.reset() }
}
