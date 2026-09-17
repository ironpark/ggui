package ggui

import "math"

// ForWidget is a keyed, reactive list: it watches a Reader of items, keeps one
// child per key across changes, and hands each child its item as a Signal so
// the child can react to updates on its own. Build one with For.
type ForWidget[T any, K comparable] struct {
	flow
	key     func(T) K
	build   func(*Signal[T]) Widget
	owner   *effect
	items   []T
	keys    []K
	entries map[K]*forEntry[T]
	stale   bool // items changed since children was last filled
	cache   *CachedWidget
	extent  float64 // fixed main-axis size per item; 0 lays every child out

	// The virtual path: the range of items laid out this frame and their
	// widgets; sizes and offsets are indexed the same way.
	first, last int
	visible     []Widget
}

type forEntry[T any] struct {
	item    *Signal[T]
	widget  Widget
	dispose func()
}

// For builds one child per item and reuses it while the item's key stays in
// the list, so state inside a child (a Component's signals, a Scroll's
// offset) survives reordering and updates. Each child gets its item as a
// *Signal[T] that For writes whenever the item changes; read it reactively.
// Children lay out like a Column; Gap and Horizontal adjust that. A child is
// built the first time it is laid out, so with ItemExtent inside a Scroll
// only the items in view exist at all.
//
//	ggui.For(todos, func(t Todo) int { return t.ID }, func(t *ggui.Signal[Todo]) ggui.Widget {
//		return todoRow(t)
//	})
func For[T any, K comparable](items Reader[[]T], key func(T) K, build func(*Signal[T]) Widget) *ForWidget[T, K] {
	f := &ForWidget[T, K]{key: key, build: build, entries: map[K]*forEntry[T]{}}
	// The outer effect reads nothing, so it only ever runs once and is
	// disposed with its owner; the entries belong to it. The inner effect
	// follows items and is the only thing that re-runs.
	Effect(func() {
		f.owner = currentOwner()
		OnCleanup(func() {
			for _, e := range f.entries {
				e.dispose()
			}
			clear(f.entries)
		})
		Effect(func() {
			list := items.Get()
			seen := make(map[K]bool, len(list))
			keys := make([]K, len(list))
			for i, it := range list {
				k := key(it)
				if seen[k] {
					panic("ggui: For saw the same key twice")
				}
				seen[k] = true
				keys[i] = k
				if e := f.entries[k]; e != nil {
					e.item.Set(it)
				}
			}
			for k, e := range f.entries {
				if !seen[k] {
					e.dispose()
					delete(f.entries, k)
				}
			}
			f.items, f.keys, f.stale = list, keys, true
			f.cache.invalidate()
		})
	})
	return f
}

// Gap sets the space between consecutive children.
func (f *ForWidget[T, K]) Gap(v float64) *ForWidget[T, K] { f.gap = v; return f }

// Space sets the gap to n times the theme's Space, resolved at layout.
func (f *ForWidget[T, K]) Space(n float64) *ForWidget[T, K] { f.space = n; return f }

// Align places children across the list's axis.
func (f *ForWidget[T, K]) Align(a CrossAlign) *ForWidget[T, K] { f.align = a; return f }

// Horizontal lays the children out like a Row instead of a Column.
func (f *ForWidget[T, K]) Horizontal() *ForWidget[T, K] { f.horizontal = true; return f }

// ItemExtent fixes every child's height (width, with Horizontal) to v. The
// list's size then follows from the count alone, and inside a Scroll only
// the children in view are built, laid out and painted: a list of tens of
// thousands of rows costs what the visible ones do.
func (f *ForWidget[T, K]) ItemExtent(v float64) *ForWidget[T, K] { f.extent = v; return f }

// Len returns the number of items the list currently holds.
func (f *ForWidget[T, K]) Len() int { return len(f.items) }

// entry returns the child for item i, building it on first use.
func (f *ForWidget[T, K]) entry(i int) *forEntry[T] {
	k := f.keys[i]
	e := f.entries[k]
	if e == nil {
		e = &forEntry[T]{item: State(f.items[i])}
		withOwner(f.owner, func() {
			e.dispose = Root(func() { e.widget = f.build(e.item) })
		})
		f.entries[k] = e
	}
	return e
}

// Layout implements Widget.
func (f *ForWidget[T, K]) Layout(c Constraints, env Env) Size {
	f.cache, _ = env.Get(cacheOwner)
	n := len(f.items)
	if f.extent <= 0 {
		if f.stale {
			f.children = resize(f.children, n)
			for i := range n {
				f.children[i] = f.entry(i).widget
			}
			f.stale = false
		}
		return f.layout(c, env)
	}

	// Fixed extents: size from the count, lay out only what a Scroll shows.
	gap := f.gapFor(env)
	pitch := f.extent + gap
	total := max(float64(n)*pitch-gap, 0)
	f.first, f.last = 0, n
	if vp, ok := ScrollViewport(env); ok && vp.Horizontal == f.horizontal && n > 0 {
		f.first = clamp(int(math.Floor(vp.Offset/pitch)), 0, n)
		f.last = clamp(int(math.Ceil((vp.Offset+vp.Extent)/pitch)), f.first, n)
	}
	shown := f.last - f.first
	f.visible = resize(f.visible, shown)
	f.sizes = resize(f.sizes, shown)
	f.offsets = resize(f.offsets, shown)
	crossMax := f.cross(c.Max())
	crossMin := f.stretched(crossMax, 0)
	var crossUsed float64
	for j := range shown {
		w := f.entry(f.first + j).widget
		f.visible[j] = w
		f.sizes[j] = w.Layout(f.constraints(f.extent, f.extent, crossMin, crossMax), env)
		crossUsed = max(crossUsed, f.cross(f.sizes[j]))
	}
	result := c.Constrain(f.size(total, f.stretched(crossMax, crossUsed)))
	for j, s := range f.sizes {
		main := float64(f.first+j) * pitch
		off := (f.cross(result) - f.cross(s)) * f.crossFraction()
		if f.horizontal {
			f.offsets[j] = Pt(main, off)
		} else {
			f.offsets[j] = Pt(off, main)
		}
	}
	return result
}

// Paint implements Widget.
func (f *ForWidget[T, K]) Paint(dst *Canvas, r Rect) {
	if f.extent <= 0 {
		f.paint(dst, r)
		return
	}
	for j, w := range f.visible {
		dst.Paint(w, Rct(r.Origin.Add(f.offsets[j]), f.sizes[j]))
	}
}
