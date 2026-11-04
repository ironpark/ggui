// Package region is how a frame picks the parts of a window it paints
// again: rectangles grown by what is drawn around them, merged where they
// meet, and folded into one when there are too many to keep apart. An
// empty rectangle is no region and drops out of every union.
package region

import "github.com/ironpark/ggui/pkg/layout"

type Rect = layout.Rect

// CoverAll is the Rect around every Rect of sets that is not empty.
func CoverAll(sets ...[]Rect) Rect {
	var all Rect
	for _, rs := range sets {
		for _, r := range rs {
			all = all.Cover(r)
		}
	}
	return all
}

// Merge returns rs as a frame paints them: each grown by slack and cut to
// window, those that come within slack of one another merged, and, when
// more than most are left, the one Rect around them all. rs is not
// changed.
func Merge(rs []Rect, window Rect, slack float64, most int) []Rect {
	out := make([]Rect, 0, len(rs))
	for _, r := range rs {
		if r = r.Grow(slack).Intersect(window); !r.Empty() {
			out = append(out, r)
		}
	}
	// Merging only grows regions, so any order of merges ends at the same
	// regions: each takes in all it reaches, looking again at those it
	// passed whenever it grew, and a pass that merged any is followed by
	// another, for a region that grew into one before it.
	for merged := true; merged; {
		merged = false
		for i := 0; i < len(out); i++ {
			reach := out[i].Grow(slack)
			for j := i + 1; j < len(out); {
				if !reach.Overlaps(out[j]) {
					j++
					continue
				}
				out[i] = out[i].Cover(out[j])
				out[j] = out[len(out)-1]
				out = out[:len(out)-1]
				merged = true
				reach, j = out[i].Grow(slack), i+1
			}
		}
	}
	if len(out) > most {
		out = append(out[:0], CoverAll(out))
	}
	return out
}
