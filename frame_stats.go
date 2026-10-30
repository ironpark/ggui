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
}

var framesRun, frameNanos atomic.Int64

func countFrame(d time.Duration) {
	framesRun.Add(1)
	frameNanos.Add(int64(d))
}

// ReadFrameStats reports the frames run so far. It is safe to call from
// any goroutine.
func ReadFrameStats() FrameStats {
	return FrameStats{Frames: framesRun.Load(), Busy: time.Duration(frameNanos.Load())}
}
