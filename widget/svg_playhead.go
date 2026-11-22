package widget

import (
	"math"
	"time"

	"github.com/ironpark/ggfx/svg"
	"github.com/ironpark/ggui/gem"
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
func (p *svgPlayhead) advance(dt time.Duration, tl svg.Timeline, loop int, alternate bool) (finished bool) {
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

	if span <= 0 {
		p.pos = 0
		p.playing = false
		return false
	}

	// Infinite looping over [0, span] (loop < 0).
	if loop < 0 {
		cycle := span
		if alternate {
			cycle = 2 * span
		}
		rem := dt % cycle
		for rem > 0 {
			if p.dir > 0 {
				toEnd := span - p.pos
				if rem < toEnd {
					p.pos += rem
					rem = 0
				} else {
					rem -= toEnd
					if alternate {
						p.pos = span
						p.dir = -1
					} else {
						p.pos = 0
					}
				}
			} else {
				toStart := p.pos
				if rem < toStart {
					p.pos -= rem
					rem = 0
				} else {
					rem -= toStart
					if alternate {
						p.pos = 0
						p.dir = 1
					} else {
						p.pos = span
					}
				}
			}
		}
		return false
	}

	// Finite run: loop > 0 passes, or loop == 0 -> 1 authored finite pass.
	totalPasses := loop
	if totalPasses == 0 {
		totalPasses = 1
	}
	rem := dt
	for rem > 0 {
		// Fast-forward full passes if starting from an endpoint.
		if (p.pos == 0 && p.dir > 0) || (p.pos == span && p.dir < 0) {
			full := int(rem / span)
			left := totalPasses - p.pass
			if full >= left {
				// Consume all remaining passes and land on the final endpoint.
				finalDir := p.dir
				if alternate && (left-1)%2 == 1 {
					finalDir = -finalDir
				}
				p.pass = totalPasses
				if finalDir > 0 {
					p.pos = span
				} else {
					p.pos = 0
				}
				p.playing = false
				return true
			}
			if full > 0 {
				p.pass += full
				rem -= time.Duration(full) * span
				if alternate && full%2 == 1 {
					p.dir = -p.dir
					if p.dir > 0 {
						p.pos = 0
					} else {
						p.pos = span
					}
				}
				if rem == 0 {
					return false
				}
			}
		}
		if p.dir > 0 {
			toEnd := span - p.pos
			if rem < toEnd {
				p.pos += rem
				rem = 0
			} else {
				rem -= toEnd
				p.pass++
				if p.pass >= totalPasses {
					p.pos = span
					p.playing = false
					return true
				}
				if alternate {
					p.pos = span
					p.dir = -1
				} else {
					p.pos = 0
				}
			}
		} else {
			toStart := p.pos
			if rem < toStart {
				p.pos -= rem
				rem = 0
			} else {
				rem -= toStart
				p.pass++
				if p.pass >= totalPasses {
					p.pos = 0
					p.playing = false
					return true
				}
				if alternate {
					p.pos = 0
					p.dir = 1
				} else {
					p.pos = span
				}
			}
		}
	}
	return false
}

// evalTime returns the document timestamp corresponding to p's current position.
func (p *svgPlayhead) evalTime(tl svg.Timeline) time.Duration {
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
