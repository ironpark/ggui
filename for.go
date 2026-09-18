package ggui

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"
)

// EachWidget is a keyed, reactive list: it watches a Readable of items, keeps one
// child per key across changes, and hands each child its item as a StateValue so
// the child can react to updates on its own. Build one with EachKeyed.
type EachWidget[T any, K comparable] struct {
	flow
	mount        *ComponentWidget
	mounted      bool
	emptyBuild   func() Widget
	empty        Widget
	emptyDispose Cleanup
	key          func(int, T) K
	build        func(EachItem[T]) Widget
	retain       int // offscreen rows kept mounted with ItemExtent; 0 keeps all
	frame        uint64
	owner        *effect
	items        []T
	keys         []K
	entries      map[K]*forEntry[T]
	stale        bool // items changed since children was last filled
	cache        *CachedWidget
	extent       float64 // fixed main-axis size per item; 0 lays every child out

	// Transition wraps every row; removed rows then leave through it.
	transition func(Widget) *TransitionWidget
	leaving    []*forEntry[T]
	leaveKeys  []K
	reduced    bool

	// Entries in the same paint order as children, including leaving rows.
	childEntries []*forEntry[T]

	// The virtual path: the range of items laid out this frame and their
	// widgets; sizes and offsets are indexed the same way.
	first, last int
	visible     []Widget
}

type forEntry[T any] struct {
	item     *StateValue[T]
	position *StateValue[int]
	widget   Widget
	dispose  func()
	seen     uint64 // the last frame the entry was laid out

	// While leaving: the row's last index, and when it was removed.
	index int
	since time.Time
}

// transition returns the row's Transition, when EachKeyed wraps rows in one.
func (e *forEntry[T]) transition() *TransitionWidget {
	t, _ := e.widget.(*TransitionWidget)
	return t
}

// EachItem exposes a row's current value and position without rebuilding it.
type EachItem[T any] struct {
	Value Readable[T]
	Index Readable[int]
}

// EachKeyed reuses rows by a unique key and preserves their local state while
// values and indices change. Factories run untracked once per row instance.
// Rows lay out vertically unless Horizontal is set. ItemExtent virtualizes
// fixed-size rows inside Scroll; Else mounts an empty-list branch.
func EachKeyed[T any, K comparable](items Readable[[]T], key func(T) K, build func(EachItem[T]) Widget) *EachWidget[T, K] {
	return each(items, func(_ int, value T) K { return key(value) }, build)
}

// Each reuses rows by position. Duplicate values are allowed.
func Each[T any](items Readable[[]T], build func(EachItem[T]) Widget) *EachWidget[T, int] {
	return each(items, func(index int, _ T) int { return index }, build)
}

func each[T any, K comparable](items Readable[[]T], key func(int, T) K, build func(EachItem[T]) Widget) *EachWidget[T, K] {
	f := &EachWidget[T, K]{key: key, build: build, entries: map[K]*forEntry[T]{}}
	f.mount = Component(func() Widget {
		f.mounted = true
		f.owner = currentOwner()
		OnCleanup(func() {
			for _, e := range f.entries {
				e.dispose()
			}
			for _, e := range f.leaving {
				e.dispose()
			}
			if f.emptyDispose != nil {
				f.emptyDispose()
			}
			clear(f.entries)
			f.leaving = nil
			f.leaveKeys = nil
		})
		observe(func() {
			list := items.Get()
			seen := make(map[K]bool, len(list))
			keys := make([]K, len(list))
			// Validate before mutating any row, so duplicate keys cannot partially apply.
			for i, value := range list {
				k := key(i, value)
				if seen[k] {
					panic("ggui: EachKeyed saw the same key twice")
				}
				seen[k], keys[i] = true, k
			}
			for k, e := range f.entries {
				if !seen[k] {
					f.remove(k, e)
				}
			}
			for i := 0; i < len(f.leaving); i++ {
				if !seen[f.leaveKeys[i]] {
					continue
				}
				f.entries[f.leaveKeys[i]] = f.leaving[i]
				f.leaving[i].transition().drive(1, false)
				f.leaving = slices.Delete(f.leaving, i, i+1)
				f.leaveKeys = slices.Delete(f.leaveKeys, i, i+1)
				i--
			}
			for i, k := range keys {
				if e := f.entries[k]; e != nil {
					e.item.Set(list[i])
					e.position.Set(i)
				}
			}
			if len(list) != 0 && f.emptyDispose != nil {
				f.emptyDispose()
				f.emptyDispose, f.empty = nil, nil
			}
			if len(list) == 0 && f.emptyBuild != nil && f.emptyDispose == nil {
				withOwner(f.owner, func() { f.emptyDispose = rootWith(new(int), "", func() { f.empty = f.emptyBuild() }) })
			}
			f.items, f.keys, f.stale = list, keys, true
			f.cache.invalidate()
		})
		return nil
	})
	return f
}

// Else mounts an empty-list branch. Configure it before the first layout.
func (f *EachWidget[T, K]) Else(build func() Widget) *EachWidget[T, K] {
	if f.mounted {
		panic("ggui: Each configured after mount")
	}
	f.emptyBuild = build
	return f
}

// remove takes the row for k out of the list: disposed at once, or kept
// while it plays its Transition backwards.
func (f *EachWidget[T, K]) remove(k K, e *forEntry[T]) {
	delete(f.entries, k)
	if f.transition == nil || f.extent > 0 || f.reduced || e.transition() == nil {
		e.dispose()
		return
	}
	e.index = slices.Index(f.keys, k)
	e.since = Now()
	f.leaving = append(f.leaving, e)
	f.leaveKeys = append(f.leaveKeys, k)
}

// Transition wraps every row in the Transition wrap returns, so a row
// plays its enter animation when it appears and the same animation
// backwards when its item is removed, inert to input meanwhile, the way
// Presence does for one child. It applies without ItemExtent; a
// virtualized list removes rows at once.
//
//	ggui.EachKeyed(todos, key, row).Transition(func(w ggui.Widget) *ggui.TransitionWidget {
//		return ggui.Transition(w).Fade().Slide(-16, 0)
//	})
func (f *EachWidget[T, K]) Transition(wrap func(Widget) *TransitionWidget) *EachWidget[T, K] {
	f.checkConfig()
	f.transition = wrap
	return f
}

// Gap sets the space between consecutive children.
func (f *EachWidget[T, K]) Gap(v float64) *EachWidget[T, K] { f.checkConfig(); f.gap = v; return f }

// Space sets the gap to n times the theme's Space, resolved at layout.
func (f *EachWidget[T, K]) Space(n float64) *EachWidget[T, K] { f.checkConfig(); f.space = n; return f }

// Align places children across the list's axis.
func (f *EachWidget[T, K]) Align(a CrossAlign) *EachWidget[T, K] {
	f.checkConfig()
	f.align = a
	return f
}

// Horizontal lays the children out like a Row instead of a Column.
func (f *EachWidget[T, K]) Horizontal() *EachWidget[T, K] {
	f.checkConfig()
	f.horizontal = true
	return f
}

// ItemExtent fixes every child's height (width, with Horizontal) to v. The
// list's size then follows from the count alone, and inside a Scroll only
// the children in view are built, laid out and painted: a list of tens of
// thousands of rows costs what the visible ones do.
func (f *EachWidget[T, K]) ItemExtent(v float64) *EachWidget[T, K] {
	f.checkConfig()
	f.extent = v
	return f
}

// Retain keeps at most n rows that are out of view mounted, with
// ItemExtent inside a Scroll; the rest are disposed and rebuilt, with fresh
// local state, when they scroll back in. Without it every row once built
// stays. A row holding focus or a pointer capture is never evicted.
func (f *EachWidget[T, K]) Retain(n int) *EachWidget[T, K] { f.checkConfig(); f.retain = n; return f }

// Len returns the number of items the list currently holds.
func (f *EachWidget[T, K]) Len() int { return len(f.items) }

// entry returns the child for item i, building it on first use.
func (f *EachWidget[T, K]) entry(i int) *forEntry[T] {
	k := f.keys[i]
	e := f.entries[k]
	if e == nil {
		e = &forEntry[T]{item: State(f.items[i]), position: State(i)}
		withOwner(f.owner, func() {
			e.dispose = rootWith(e, fmt.Sprint(k), func() {
				e.widget = f.build(EachItem[T]{Value: e.item, Index: e.position})
				if f.transition != nil {
					t := f.transition(e.widget)
					t.id = forRowKey[K]{k} // the row keeps its animation when it moves
					e.widget = t
				}
			})
		})
		f.entries[k] = e
	}
	return e
}

// Layout implements Widget.
func (f *EachWidget[T, K]) Layout(c Constraints, env Env) Size {
	f.cache, _ = env.Get(cacheOwner)
	f.mount.Layout(c, env)
	if len(f.items) == 0 && len(f.leaving) == 0 && f.empty != nil {
		return f.empty.Layout(c, env)
	}
	f.reduced = env.ReducedMotion()
	n := len(f.items)
	if f.extent <= 0 {
		if f.stale || len(f.leaving) > 0 {
			clear(f.childEntries)
			f.childEntries = resize(f.childEntries, n)
			f.children = resize(f.children, n)
			for i := range n {
				e := f.entry(i)
				f.children[i], f.childEntries[i] = e.widget, e
			}
			f.stale = false
			f.placeLeaving()
		}
		if n == 0 && len(f.leaving) == 0 && f.empty != nil {
			return f.empty.Layout(c, env)
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
	f.frame++
	f.visible = resize(f.visible, shown)
	f.sizes = resize(f.sizes, shown)
	f.offsets = resize(f.offsets, shown)
	crossMax := f.cross(c.Max())
	crossMin := f.stretched(crossMax, 0)
	var crossUsed float64
	for j := range shown {
		e := f.entry(f.first + j)
		e.seen = f.frame
		w := e.widget
		f.visible[j] = w
		f.sizes[j] = w.Layout(f.constraints(f.extent, f.extent, crossMin, crossMax), env)
		crossUsed = max(crossUsed, f.cross(f.sizes[j]))
	}
	result := c.Constrain(f.size(total, f.stretched(crossMax, crossUsed)))
	f.evict()
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

// forRowKey identifies a row's Transition by its item key.
type forRowKey[K comparable] struct{ k K }

// placeLeaving puts the rows on their way out back among the children at
// their old places, drives their Transitions by the time since removal,
// and drops the ones that finished. The list lays out every frame while
// any is leaving.
func (f *EachWidget[T, K]) placeLeaving() {
	now := Now()
	for i := 0; i < len(f.leaving); i++ {
		e := f.leaving[i]
		t := e.transition()
		p := 1.0
		if t.duration > 0 {
			p = 1 - float64(now.Sub(e.since))/float64(t.duration)
		}
		if p <= 0 {
			e.dispose()
			f.leaving = slices.Delete(f.leaving, i, i+1)
			f.leaveKeys = slices.Delete(f.leaveKeys, i, i+1)
			i--
			continue
		}
		t.drive(p, true)
		at := min(max(e.index, 0), len(f.children))
		f.children = slices.Insert(f.children, at, e.widget)
		f.childEntries = slices.Insert(f.childEntries, at, e)
	}
	if len(f.leaving) > 0 {
		requestLayout()
		f.cache.invalidate()
	}
}

// evict disposes offscreen entries beyond Retain, oldest first.
func (f *EachWidget[T, K]) evict() {
	if f.retain <= 0 {
		return
	}
	var out []K
	for k, e := range f.entries {
		if e.seen != f.frame && !busy[e] {
			out = append(out, k)
		}
	}
	if len(out) <= f.retain {
		return
	}
	slices.SortFunc(out, func(a, b K) int { return cmp.Compare(f.entries[a].seen, f.entries[b].seen) })
	for _, k := range out[:len(out)-f.retain] {
		f.entries[k].dispose()
		delete(f.entries, k)
	}
}

// Paint implements Widget. The list describes itself as a list of Len
// items and each row as the item it is, so "item 3 of 200" can be said of
// a virtualized list where only a dozen rows exist at all.
func (f *EachWidget[T, K]) Paint(dst *Canvas, r Rect) {
	if len(f.items) == 0 && len(f.leaving) == 0 && f.empty != nil {
		dst.Paint(f.empty, r)
		return
	}
	n := len(f.items)
	dst.Node(r, Node{Role: RoleList, Min: 1, Max: float64(n)}, func(dst *Canvas) {
		if f.extent <= 0 {
			for i, child := range f.children {
				rc := Rct(r.Origin.Add(f.offsets[i]), f.sizes[i])
				dst.inGroup(f.childEntries[i], func() { f.paintRow(dst, child, rc, i, n) })
			}
			return
		}
		for j, w := range f.visible {
			e := f.entries[f.keys[f.first+j]]
			rc := Rct(r.Origin.Add(f.offsets[j]), f.sizes[j])
			dst.inGroup(e, func() { f.paintRow(dst, w, rc, f.first+j, n) })
		}
	})
}

// paintRow paints one row inside a list item that knows its place.
func (f *EachWidget[T, K]) paintRow(dst *Canvas, w Widget, rc Rect, i, n int) {
	item := Node{Role: RoleListItem, Min: 1, Now: float64(i + 1), Max: float64(n)}
	dst.Node(rc, item, func(dst *Canvas) { dst.Paint(w, rc) })
}

func (f *EachWidget[T, K]) checkConfig() {
	if f.mounted {
		panic("ggui: Each configured after mount")
	}
}
