// Package virtual is the arithmetic of a list that lays out only the rows
// in view: which rows a window over the list reaches, where each row
// starts when only some have been measured, and how far the row a window
// started on has moved since, so that what is in view can stay in place.
// It knows rows by index and key and sizes along the list's axis only;
// building, laying out and painting them is its caller's.
package virtual

import (
	"math"
	"slices"
	"sort"
	"unsafe"

	"github.com/ironpark/ggui/pkg/gem"
)

// Fixed returns the rows [first, last) of n, each pitch apart (a row's
// size and the gap after it), that a window of extent starting at at
// reaches.
func Fixed(n int, pitch, at, extent float64) (first, last int) {
	if n <= 0 || pitch <= 0 {
		return 0, max(n, 0)
	}
	first = gem.Clamp(int(math.Floor(at/pitch)), 0, n)
	last = gem.Clamp(int(math.Ceil((at+extent)/pitch)), first, n)
	return first, last
}

// Estimate places rows of any size: each at the size it had when last
// measured, and the rest at an estimate, until they are. It keeps the row
// a window last started on, its anchor, to tell how far that row has moved
// as the sizes above it came to be known. The zero Estimate is ready to
// use.
type Estimate[K comparable] struct {
	measured map[K]float64 // main-axis size, by key
	starts   []float64     // where each row starts, and the end of the last plus a gap
	estimate float64       // as last placed
	gap      float64

	anchored bool
	anchor   K       // the row at the window's start
	anchorI  int     // its index when anchored, where Moved looks first
	anchorAt float64 // where it started

	// What the last Place placed, and what has changed since: placed is
	// the keys it was given, stale whether a size changed for a row whose
	// index is not known, and from the first index MeasureAt changed, or
	// past the end for none. A Place with nothing changed keeps the starts
	// it has, and one after MeasureAt works them out from there on, so a
	// frame that only scrolled costs nothing however long the list.
	placed []K
	stale  bool
	from   int
}

// Measure records the size of the row k, and reports whether it differs
// from the size the last Place gave it, so that the rows need placing
// again. MeasureAt is the same for a row whose index is known, and lets
// the next Place work out only the rows from there on.
func (e *Estimate[K]) Measure(k K, size float64) (changed bool) {
	changed = e.measure(k, size)
	e.stale = e.stale || changed
	return changed
}

// MeasureAt is Measure for the row at index i of the keys the next Place
// is given.
func (e *Estimate[K]) MeasureAt(i int, k K, size float64) (changed bool) {
	if changed = e.measure(k, size); changed {
		e.from = min(e.from, i)
	}
	return changed
}

func (e *Estimate[K]) measure(k K, size float64) (changed bool) {
	if e.measured == nil {
		e.measured = map[K]float64{}
	}
	was, ok := e.measured[k]
	if !ok {
		was = e.estimate
	}
	e.measured[k] = size
	return size != was
}

// Place works out where each row of keys starts, at the sizes measured and
// estimate for the rest, with gap after each.
//
// Given the slice the last Place was, unchanged in length, the same
// estimate and gap, it takes the keys to be the same and works out again
// only what was measured differently since: a caller that changes its keys
// in place, rather than making a new slice, calls Forget first.
func (e *Estimate[K]) Place(keys []K, estimate, gap float64) {
	from := 0
	if e.same(keys, estimate, gap) && !e.stale {
		from = min(e.from, len(keys))
	}
	e.estimate, e.gap = estimate, gap
	e.placed, e.stale, e.from = keys, false, math.MaxInt
	if from == len(keys) && len(e.starts) == len(keys)+1 {
		return
	}
	// The rows before from keep their starts, and row from starts where
	// it did: only sizes from there on changed.
	at := 0.0
	if from > 0 {
		at = e.starts[from]
	} else {
		e.starts = slices.Grow(e.starts[:0], len(keys)+1)[:len(keys)+1]
	}
	for i := from; i < len(keys); i++ {
		e.starts[i] = at
		size, ok := e.measured[keys[i]]
		if !ok {
			size = estimate
		}
		at += size + gap
	}
	e.starts[len(keys)] = at
}

// same reports whether keys, estimate and gap are what the last Place was
// given.
func (e *Estimate[K]) same(keys []K, estimate, gap float64) bool {
	return e.starts != nil && len(keys) == len(e.placed) && len(e.starts) == len(keys)+1 &&
		unsafe.SliceData(keys) == unsafe.SliceData(e.placed) && estimate == e.estimate && gap == e.gap
}

// Forget makes the next Place work out every row's start afresh, for a
// caller that changed the keys it gave the last one in place.
func (e *Estimate[K]) Forget() { e.placed = nil }

// Start returns where row i starts as last placed, and, at i equal to the
// count of rows, the end of the last plus its gap. It is false for an i
// out of that range.
func (e *Estimate[K]) Start(i int) (float64, bool) {
	if i < 0 || i >= len(e.starts) {
		return 0, false
	}
	return e.starts[i], true
}

// Total is the length of the rows as last placed, without the gap after
// the last.
func (e *Estimate[K]) Total() float64 {
	if len(e.starts) == 0 {
		return 0
	}
	return max(e.starts[len(e.starts)-1]-e.gap, 0)
}

// RowAt returns the row that reaches past v: the first whose end, with the
// gap after it, is beyond v.
func (e *Estimate[K]) RowAt(v float64) int {
	n := len(e.starts) - 1
	return max(sort.Search(n, func(i int) bool { return e.starts[i+1] > v }), 0)
}

// Moved returns how far the anchor has moved since it was set, as keys
// are placed now: 0 when there is none, or it is no longer among them.
func (e *Estimate[K]) Moved(keys []K) float64 {
	if !e.anchored {
		return 0
	}
	i := e.anchorI
	if i >= len(keys) || keys[i] != e.anchor {
		if i = slices.Index(keys, e.anchor); i < 0 {
			return 0
		}
		e.anchorI = i
	}
	return e.starts[i] - e.anchorAt
}

// TakeMoved returns Moved and forgets the anchor.
func (e *Estimate[K]) TakeMoved(keys []K) float64 {
	d := e.Moved(keys)
	e.anchored = false
	return d
}

// Anchor makes the row at v, of keys as last placed, the one Moved
// measures from.
func (e *Estimate[K]) Anchor(keys []K, v float64) {
	a := e.RowAt(v)
	if a >= len(keys) {
		e.anchored = false
		return
	}
	e.anchored, e.anchor, e.anchorI, e.anchorAt = true, keys[a], a, e.starts[a]
}

// Prune forgets the sizes of rows no longer among keys.
func (e *Estimate[K]) Prune(keys []K) {
	if len(e.measured) <= len(keys) {
		return
	}
	keep := make(map[K]float64, len(keys))
	for _, k := range keys {
		if v, ok := e.measured[k]; ok {
			keep[k] = v
		}
	}
	e.measured = keep
}
