package ggui

import (
	"sync"
	"time"
)

// clock is the raw time source; SetClock replaces it. Animations do not
// read it directly: they read Now, the instant the current frame froze.
var clock = time.Now

// maxFrameStep caps how far one frame may move the frame clock. Ebiten
// stops calling Update and Draw while the window is minimized or hidden,
// so the frame that comes back sees the whole pause as its delta, and
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
}

var frame frameClock

// begin moves the clock forward by the time that has passed since the last
// frame, at most maxFrameStep, and returns the instant this frame reads.
// App calls it twice: once in Update, which Ebiten runs at a fixed TPS,
// and once in Draw, which it runs at the display's rate, so motion eased
// during Paint is as smooth as the screen allows.
func (f *frameClock) begin(raw time.Time) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
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
// Probe.Advance or SetClock.
func Now() time.Time { return frame.instant() }

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
