package widget

import (
	"math"
	"time"

	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/svgx"
)

// svgPlayhead tracks playback state for either an SVGController (shared by all
// widgets attached to it) or a standalone SVGWidget (retained via svgSlot).
type svgPlayhead struct {
	init      bool
	playing   bool
	dir       int           // +1 forward, -1 reverse (0 treated as +1)
	pos       time.Duration // current playhead position
	pass      int           // completed passes in the current run
	doneEpoch uint64        // monotonically incremented each time a finite run completes
	anchor    time.Time     // last quantized frame timestamp while playing

	// Shared-controller offscreen coordination: if one widget on a shared
	// controller paints offscreen earlier in the same frame than a visible
	// widget, the visible widget restores savedAnchor when clearedAt == now.
	visAt       time.Time
	savedAnchor time.Time
	clearedAt   time.Time
}

// markDone records a finite playback completion, advancing doneEpoch so every
// attached widget can fire its OnDone callback once.
func (p *svgPlayhead) markDone() {
	p.doneEpoch++
}

// markOffscreen clears the clock anchor unless another visible widget on the
// same playhead already painted in this frame.
func (p *svgPlayhead) markOffscreen(now time.Time) {
	if p.visAt == now {
		return
	}
	if !p.anchor.IsZero() {
		p.savedAnchor = p.anchor
		p.clearedAt = now
		p.anchor = time.Time{}
	}
}

// markVisible restores a clock anchor cleared earlier in the same frame by an
// offscreen sibling sharing this playhead, and marks this frame visible.
func (p *svgPlayhead) markVisible(now time.Time) {
	if p.anchor.IsZero() && p.clearedAt == now && !p.savedAnchor.IsZero() {
		p.anchor = p.savedAnchor
	}
	p.visAt = now
	p.savedAnchor = time.Time{}
	p.clearedAt = time.Time{}
}

// advance steps p forward or backward by dt >= 0 according to tl, loop and
// alternate. It reports true when a finite run reaches its end.
func (p *svgPlayhead) advance(dt time.Duration, tl svgx.Timeline, loop int, alternate bool) (finished bool) {
	if dt <= 0 {
		return false
	}
	if p.dir == 0 {
		p.dir = 1
	}
	span := tl.Span

	// Authored infinite timeline (loop == 0 && tl.Infinite): play indefinitely
	// when moving forward, or stop at 0 when reversing.
	if loop == 0 && tl.Infinite {
		return p.advanceAuthored(dt, tl)
	}

	if span <= 0 {
		p.pos = 0
		p.playing = false
		return false
	}

	run := svgRun{span: span, alternate: alternate}
	// Infinite looping over [0, span] (loop < 0).
	if loop < 0 {
		cycle := span
		if alternate {
			cycle = 2 * span
		}
		for rem := dt % cycle; rem > 0; {
			var reached bool
			if rem, reached = p.stepPass(rem, span); reached {
				p.turn(run)
			}
		}
		return false
	}

	// Finite run: loop > 0 passes, or loop == 0 -> 1 authored finite pass.
	totalPasses := loop
	if totalPasses == 0 {
		totalPasses = 1
	}
	return p.runPasses(dt, run, totalPasses)
}

// svgRun is how a playhead plays a finite timeline: over span, and every
// other pass backwards when alternate.
type svgRun struct {
	span      time.Duration
	alternate bool
}

// advanceAuthored steps p by dt over an authored infinite timeline: on and
// on, round its loop, when moving forward, or back to a stop at 0, when it
// reports true.
func (p *svgPlayhead) advanceAuthored(dt time.Duration, tl svgx.Timeline) bool {
	if p.dir > 0 {
		p.pos += dt
		if tl.Loop > 0 && p.pos >= tl.LoopStart {
			p.pos = tl.LoopStart + (p.pos-tl.LoopStart)%tl.Loop
		}
		return false
	}
	if dt >= p.pos {
		p.pos = 0
		p.playing = false
		return true
	}
	p.pos -= dt
	return false
}

// runPasses steps p by rem through a finite run of passes passes, and
// reports true when the run reaches its end.
func (p *svgPlayhead) runPasses(rem time.Duration, run svgRun, passes int) bool {
	for rem > 0 {
		// Fast-forward full passes if starting from an endpoint.
		if (p.pos == 0 && p.dir > 0) || (p.pos == run.span && p.dir < 0) {
			var finished bool
			if rem, finished = p.skipPasses(rem, run, passes); finished || rem == 0 {
				return finished
			}
		}
		var reached bool
		if rem, reached = p.stepPass(rem, run.span); !reached {
			continue
		}
		p.pass++
		if p.pass >= passes {
			p.pos = gem.Pick(p.dir > 0, run.span, 0)
			p.playing = false
			return true
		}
		p.turn(run)
	}
	return false
}

// skipPasses fast-forwards p, at the endpoint a pass starts from, past the
// whole passes rem covers, and returns what is left of rem. When those are
// all the passes left, it lands on the run's final endpoint and reports
// true.
func (p *svgPlayhead) skipPasses(rem time.Duration, run svgRun, passes int) (time.Duration, bool) {
	full := int(rem / run.span)
	left := passes - p.pass
	if full >= left {
		// Consume all remaining passes and land on the final endpoint.
		finalDir := p.dir
		if run.alternate && (left-1)%2 == 1 {
			finalDir = -finalDir
		}
		p.pass = passes
		p.pos = gem.Pick(finalDir > 0, run.span, 0)
		p.playing = false
		return 0, true
	}
	if full > 0 {
		p.pass += full
		rem -= time.Duration(full) * run.span
		if run.alternate && full%2 == 1 {
			p.turn(run)
		}
	}
	return rem, false
}

// stepPass moves p by up to rem toward the end of its pass, in its
// direction, and returns what is left of rem and whether it reached that
// end, where it leaves pos for the caller to set.
func (p *svgPlayhead) stepPass(rem, span time.Duration) (time.Duration, bool) {
	if p.dir > 0 {
		toEnd := span - p.pos
		if rem < toEnd {
			p.pos += rem
			return 0, false
		}
		return rem - toEnd, true
	}
	toStart := p.pos
	if rem < toStart {
		p.pos -= rem
		return 0, false
	}
	return rem - toStart, true
}

// turn starts the next pass at the end p reached: back the way it came
// when the run alternates, else over again from the start.
func (p *svgPlayhead) turn(run svgRun) {
	if run.alternate {
		p.dir = -p.dir
	}
	p.pos = gem.Pick(p.dir > 0, 0, run.span)
}

// evalTime returns the document timestamp corresponding to p's current position.
func (p *svgPlayhead) evalTime(tl svgx.Timeline) time.Duration {
	t := max(p.pos, 0)
	if tl.Infinite && tl.Loop > 0 && t >= tl.LoopStart {
		t = tl.LoopStart + (t-tl.LoopStart)%tl.Loop
	}
	return t
}

func progressToTime(p float64, span time.Duration) time.Duration {
	if math.IsNaN(p) || span <= 0 {
		return 0
	}
	p = gem.Clamp(p, 0, 1)
	return time.Duration(math.Round(p * float64(span)))
}
