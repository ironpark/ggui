package geom

import "github.com/ironpark/ggui/pkg/layout"

// Regions are sets of rectangles, as a frame repaints them: grown by what
// is drawn around them, merged where they meet, and folded into one when
// there are too many to keep apart. An empty rectangle is no region and
// drops out of every union.

// Grow returns r with d added on every side.
func Grow(r Rect, d float64) Rect {
	return layout.Rct(layout.Pt(r.Origin.X-d, r.Origin.Y-d), layout.Sz(r.Size.W+2*d, r.Size.H+2*d))
}

// Cover is the Rect around a and b, either of which may be empty and is
// then left out.
func Cover(a, b Rect) Rect {
	switch {
	case a.Empty():
		return b
	case b.Empty():
		return a
	}
	return a.Union(b)
}

// CoverAll is the Rect around every Rect of sets that is not empty.
func CoverAll(sets ...[]Rect) Rect {
	var all Rect
	for _, rs := range sets {
		for _, r := range rs {
			all = Cover(all, r)
		}
	}
	return all
}

// Overlaps reports whether a and b share some area.
func Overlaps(a, b Rect) bool { return !a.Intersect(b).Empty() }

// MergeRegions returns rs as a frame paints them: each grown by slack and cut to
// window, those that come within slack of one another merged, and, when
// more than most are left, the one Rect around them all. rs is not
// changed.
func MergeRegions(rs []Rect, window Rect, slack float64, most int) []Rect {
	out := make([]Rect, 0, len(rs))
	for _, r := range rs {
		if r = Grow(r, slack).Intersect(window); !r.Empty() {
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
			reach := Grow(out[i], slack)
			for j := i + 1; j < len(out); {
				if !Overlaps(reach, out[j]) {
					j++
					continue
				}
				out[i] = Cover(out[i], out[j])
				out[j] = out[len(out)-1]
				out = out[:len(out)-1]
				merged = true
				reach, j = Grow(out[i], slack), i+1
			}
		}
	}
	if len(out) > most {
		out = append(out[:0], CoverAll(out))
	}
	return out
}
