package ggui

// ForWidget is a keyed, reactive list: it watches a Reader of items, keeps one
// child per key across changes, and hands each child its item as a Signal so
// the child can react to updates on its own. Build one with For.
type ForWidget[T any, K comparable] struct {
	flow
	entries map[K]*forEntry[T]
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
// Children lay out like a Column; Gap and Horizontal adjust that.
//
//	ggui.For(todos, func(t Todo) int { return t.ID }, func(t *ggui.Signal[Todo]) ggui.Widget {
//		return todoRow(t)
//	})
func For[T any, K comparable](items Reader[[]T], key func(T) K, build func(*Signal[T]) Widget) *ForWidget[T, K] {
	f := &ForWidget[T, K]{entries: map[K]*forEntry[T]{}}
	// The outer effect reads nothing, so it only ever runs once and is
	// disposed with its owner; the entries belong to it. The inner effect
	// follows items and is the only thing that re-runs.
	Effect(func() {
		owner := currentOwner()
		OnCleanup(func() {
			for _, e := range f.entries {
				e.dispose()
			}
			clear(f.entries)
		})
		Effect(func() {
			list := items.Get()
			seen := make(map[K]bool, len(list))
			children := make([]Widget, 0, len(list))
			for _, it := range list {
				k := key(it)
				if seen[k] {
					panic("ggui: For saw the same key twice")
				}
				seen[k] = true
				e := f.entries[k]
				if e == nil {
					e = &forEntry[T]{item: State(it)}
					withOwner(owner, func() {
						e.dispose = Root(func() { e.widget = build(e.item) })
					})
					f.entries[k] = e
				} else {
					e.item.Set(it)
				}
				children = append(children, e.widget)
			}
			for k, e := range f.entries {
				if !seen[k] {
					e.dispose()
					delete(f.entries, k)
				}
			}
			f.children = children
		})
	})
	return f
}

// Gap sets the space between consecutive children.
func (f *ForWidget[T, K]) Gap(v float64) *ForWidget[T, K] { f.gap = v; return f }

// Horizontal lays the children out like a Row instead of a Column.
func (f *ForWidget[T, K]) Horizontal() *ForWidget[T, K] { f.horizontal = true; return f }

// Layout implements Widget.
func (f *ForWidget[T, K]) Layout(c Constraints, env Env) Size { return f.layout(c, env) }

// Paint implements Widget.
func (f *ForWidget[T, K]) Paint(dst *Canvas, r Rect) { f.paint(dst, r) }
