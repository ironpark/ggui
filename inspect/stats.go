package inspect

import "github.com/ironpark/ggui/internal/framestats"

// FrameStats counts the frames the app's windows have run since the
// program started, by why, and the time they took; see the fields.
type FrameStats = framestats.Stats

// MotionReport is what moved in the last frame a window painted, and why
// frames that could have been painted in part were painted whole; see the
// fields.
type MotionReport = framestats.Report

// ReadFrameStats reports the frames run so far. It is safe to call from
// any goroutine.
func ReadFrameStats() FrameStats { return framestats.Read() }

// ReadMotionReport reports what moves and what keeps frames whole. It is
// safe to call from any goroutine.
func ReadMotionReport() MotionReport { return framestats.ReadReport() }
