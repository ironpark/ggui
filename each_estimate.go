package ggui

import (
	"slices"
	"sort"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
)

// eachEstimate is what an EachWidget keeps to virtualize rows of any size:
// the size each row had when last laid out, the estimate standing in for
// the rest, and the row the window started on, which it keeps in place.
type eachEstimate[K comparable] struct {
	measured map[K]float64 // main-axis size, by key
	starts   []float64     // where each row starts, and the end of the last plus a gap

	anchored bool
	anchor   K       // the row at the window's start
	anchorAt float64 // where it started
}

// EstimatedExtent virtualizes rows of any size inside a Scroll: only the
// rows in view, and some way either side, are built, laid out and
// painted. A row is v long, along the list's axis, until it has been laid
// out, and as long as it was after. As the size of rows above the window
// comes to be known, or rows come in above it, the Scroll moves by as much,
// so what is in view stays where it is. ItemExtent, for rows all of one
// size, takes precedence. StickyHeaders keep to their rows.
//
//	ggui.EachKeyed(messages, Message.ID, bubble).EstimatedExtent(120)
func (f *EachWidget[T, K]) EstimatedExtent(v float64) *EachWidget[T, K] {
	defer property.Watch(&f.props, &f.estimate)()
	f.estimate = v
	return f
}

// estimated reports whether rows of any size are virtualized.
func (f *EachWidget[T, K]) estimated() bool { return f.extent <= 0 && f.estimate > 0 }

// virtual reports whether only the rows in view are laid out.
func (f *EachWidget[T, K]) virtual() bool { return f.extent > 0 || f.estimate > 0 }

// place works out where each row starts from what is known of their sizes.
func (e *eachEstimate[K]) place(keys []K, estimate, gap float64) {
	e.starts = resize(e.starts, len(keys)+1)
	at := 0.0
	for i, k := range keys {
		e.starts[i] = at
		size, ok := e.measured[k]
		if !ok {
			size = estimate
		}
		at += size + gap
	}
	e.starts[len(keys)] = at
}

// rowAt returns the row that reaches past v: the first whose end, with the
// gap after it, is beyond v.
func (e *eachEstimate[K]) rowAt(v float64) int {
	n := len(e.starts) - 1
	return max(sort.Search(n, func(i int) bool { return e.starts[i+1] > v }), 0)
}

// moved returns how far the row the window last started on has moved
// since, which is 0 when it is gone or there was none.
func (e *eachEstimate[K]) moved(keys []K) float64 {
	if !e.anchored {
		return 0
	}
	i := slices.Index(keys, e.anchor)
	if i < 0 {
		return 0
	}
	return e.starts[i] - e.anchorAt
}

// prune forgets the sizes of rows no longer in the list.
func (e *eachEstimate[K]) prune(keys []K) {
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

// layoutEstimated lays out the rows in or near a Scroll's window, each at
// its own size, and places the rest from their sizes as last known.
func (f *EachWidget[T, K]) layoutEstimated(c Constraints, env Env) Size {
	est := &f.est
	if est.measured == nil {
		est.measured = map[K]float64{}
	}
	if f.stale {
		est.prune(f.keys)
		f.stale = false
	}
	n := len(f.items)
	gap := f.gapFor(env)
	est.place(f.keys, f.estimate, gap)

	vp, ok := viewportAlong(env, f.horizontal)
	port := scrollPortAlong(env, f.horizontal)
	windowed := ok && n > 0
	// The window, in the list's own coordinates, where it will be once the
	// Scroll keeps the rows in view in place; and as far again either side,
	// so that a wheel's step finds its rows laid out, as does a window a
	// frame off where the list starts in it.
	lo := vp.Offset - f.lead + est.moved(f.keys)
	reach := vp.Extent / 2
	f.first, f.last = 0, n
	if windowed {
		f.first = est.rowAt(max(lo-reach, 0))
	}

	crossMax := f.cross(c.Max())
	crossMin := f.stretched(crossMax, 0)
	var crossUsed float64
	f.frame++
	f.visible, f.virtualRects = f.visible[:0], f.virtualRects[:0]
	at := est.starts[f.first]
	i := f.first
	for ; i < n && (!windowed || at < lo+vp.Extent+reach); i++ {
		e := f.entry(i)
		e.seen = f.frame
		s := e.widget.Layout(f.constraints(0, Unbounded, crossMin, crossMax), env)
		est.measured[f.keys[i]] = f.main(s)
		f.visible = append(f.visible, e.widget)
		f.virtualRects = append(f.virtualRects, Rect{Size: s})
		crossUsed = max(crossUsed, f.cross(s))
		at += f.main(s) + gap
	}
	f.rects = f.virtualRects
	f.last = i
	est.place(f.keys, f.estimate, gap)

	total := max(est.starts[n]-gap, 0)
	result := c.Constrain(f.size(total, f.stretched(crossMax, crossUsed)))
	f.evict()
	for j := range f.rects {
		f.placeRow(j, est.starts[f.first+j], result)
	}

	d := est.moved(f.keys)
	est.anchored = false
	if !windowed || port == nil {
		return result
	}
	if d != 0 {
		port.keepInView(d)
	}
	// The window's start once the Scroll has moved, inside the list: the
	// row there is the one the next layout keeps in place.
	if start := vp.Offset - f.lead + d; start >= 0 && start < total {
		a := est.rowAt(start)
		est.anchored, est.anchor, est.anchorAt = true, f.keys[a], est.starts[a]
	}
	return result
}

// paintLead notes, on a virtual path, where the list starts in its
// Scroll's content, which the next layout's window is reckoned from, and
// lays out again when that moved: under a header, or padded.
func (f *EachWidget[T, K]) paintLead(r Rect) {
	p := f.pin.port
	if p == nil {
		return
	}
	lead := pick(f.horizontal, r.Origin.X-p.rect.Origin.X, r.Origin.Y-p.rect.Origin.Y) + p.visualOffset()
	if lead != f.lead {
		f.lead = lead
		f.cache.invalidate()
		reactive.RequestLayout()
	}
}
