package ggui

import (
	"sync/atomic"
	"time"
)

// FrameStats counts the frames the app's windows have run since the
// program started, and the time they took: input, layout, paint and the
// draw handed to the GPU, not the GPU's own. Read twice, they give the
// frame rate, which for an app at rest should be near nothing.
type FrameStats struct {
	Frames int64
	Busy   time.Duration

	// Of the frames, those a frame before asked for, by why: an animation
	// running, a widget that read the frame's time to move with it, a
	// callback waiting for the next frame, work posted to the window, or a
	// drag or touch going on. The rest came from outside: an event, an
	// Invalidate, a new frame the window or the platform asked for.
	ForAnimation, ForTime, ForCallback, ForPosted, ForInput int64
}

const (
	frameForAnimation = iota
	frameForTime
	frameForCallback
	frameForPosted
	frameForInput
	frameReasons
)

var (
	framesRun, frameNanos atomic.Int64
	framesFor             [frameReasons]atomic.Int64
)

func countFrame(d time.Duration) {
	framesRun.Add(1)
	frameNanos.Add(int64(d))
}

// ReadFrameStats reports the frames run so far. It is safe to call from
// any goroutine.
func ReadFrameStats() FrameStats {
	return FrameStats{
		Frames: framesRun.Load(), Busy: time.Duration(frameNanos.Load()),
		ForAnimation: framesFor[frameForAnimation].Load(), ForTime: framesFor[frameForTime].Load(),
		ForCallback: framesFor[frameForCallback].Load(), ForPosted: framesFor[frameForPosted].Load(),
		ForInput: framesFor[frameForInput].Load(),
	}
}
