// Package framestats counts, for the whole process, the frames ggui's
// windows run and why, and what kept frames from being painted in part:
// what moved, why a frame that could have been painted in part was painted
// whole, and the code that marked frames dirty. Everything here is safe to
// call from any goroutine. ggui/inspect reports it.
package framestats

import (
	"maps"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Reason is why a frame before asked for the next.
type Reason uint8

const (
	Animation Reason = iota // an animation running
	Time                    // a widget read the frame's time to move with it
	Callback                // a callback waiting for the next frame
	Posted                  // work posted to the window
	Input                   // a drag or touch going on
	reasons
)

var (
	frames, nanos atomic.Int64
	asked         [reasons]atomic.Int64
	partial       atomic.Int64
	skipped       atomic.Int64
	contained     atomic.Int64
)

// Frame counts a frame run, which took d.
func Frame(d time.Duration) {
	frames.Add(1)
	nanos.Add(int64(d))
}

// For counts a frame the one before asked for, for r.
func For(r Reason) { asked[r].Add(1) }

// CountPartial counts a frame painted in part.
func CountPartial() { partial.Add(1) }

// CountSkipped counts a frame that painted and presented nothing.
func CountSkipped() { skipped.Add(1) }

// CountContained counts a frame painted within the viewports of scrolls
// that moved.
func CountContained() { contained.Add(1) }

// Stats counts the frames the app's windows have run since the program
// started, and the time they took: input, layout, paint and the draw
// handed to the GPU, not the GPU's own. Read twice, they give the frame
// rate, which for an app at rest should be near nothing.
type Stats struct {
	Frames int64
	Busy   time.Duration

	// Of the frames, those a frame before asked for, by why: an animation
	// running, a widget that read the frame's time to move with it, a
	// callback waiting for the next frame, work posted to the window, or a
	// drag or touch going on. The rest came from outside: an event, an
	// Invalidate, a new frame the window or the platform asked for.
	ForAnimation, ForTime, ForCallback, ForPosted, ForInput int64

	// Partial counts the frames painted in part, where only what moves
	// with time changed. Skipped counts those that painted nothing, as
	// nothing had changed, and presented nothing. Contained counts those
	// painted within the viewports of scrolls that moved, as nothing else
	// changed.
	Partial, Skipped, Contained int64
}

// Read reports the frames run so far.
func Read() Stats {
	return Stats{
		Frames: frames.Load(), Busy: time.Duration(nanos.Load()),
		ForAnimation: asked[Animation].Load(), ForTime: asked[Time].Load(),
		ForCallback: asked[Callback].Load(), ForPosted: asked[Posted].Load(),
		ForInput: asked[Input].Load(),
		Partial:  partial.Load(), Skipped: skipped.Load(), Contained: contained.Load(),
	}
}

// What moved and why frames were painted whole.
var (
	mu       sync.Mutex
	movers   []string
	refusals = map[string]int64{}
	// dirtiers counts, by code address, what marked frames dirty. Most are
	// a handful of sites, so the map stays small; past mostDirtiers it
	// takes no new ones.
	dirtiers = map[dirtier]int64{}
)

type dirtier struct {
	why string
	pc  uintptr
}

const mostDirtiers = 256

// SetMovers records what moved in the frame just painted, by widget type.
func SetMovers(m []string) {
	mu.Lock()
	movers = append(movers[:0], m...)
	mu.Unlock()
}

// Refuse counts a frame that could have been painted in part but was
// painted whole, by why: "" when nothing had moved is not counted.
func Refuse(why string) {
	if why == "" {
		return
	}
	mu.Lock()
	refusals[why]++
	mu.Unlock()
}

// Dirtied counts pc, a function's entry or a call site, as having marked a
// frame dirty for why.
func Dirtied(why string, pc uintptr) {
	k := dirtier{why, pc}
	mu.Lock()
	if _, ok := dirtiers[k]; ok || len(dirtiers) < mostDirtiers {
		dirtiers[k]++
	}
	mu.Unlock()
}

// Report is what moved in the last frame a window painted, by widget type,
// and how many frames since the program started could have been painted
// in part but were painted whole, by why: "dirty" when something else
// asked for them, "state changed", "overlay", "unplaced" when what moved
// was under a transform, "inspected", "resized", "not kept" when the last
// frame was not, "too much changed".
//
// Dirtiers counts, by name, the posted work that wrote a signal
// ("dirty(signal) pkg.func") and the callers of Repaint
// ("dirty(repaint) pkg.func"), whether or not the frame could have been
// painted in part.
type Report struct {
	Movers   []string
	Refusals map[string]int64
	Dirtiers map[string]int64
}

// ReadReport reports what moves and what keeps frames whole.
func ReadReport() Report {
	mu.Lock()
	defer mu.Unlock()
	r := Report{Movers: slices.Clone(movers), Refusals: maps.Clone(refusals), Dirtiers: make(map[string]int64, len(dirtiers))}
	for k, v := range dirtiers {
		name := "?"
		if f := runtime.FuncForPC(k.pc); f != nil {
			name = f.Name()
		}
		r.Dirtiers[k.why+" "+name] += v
	}
	return r
}
