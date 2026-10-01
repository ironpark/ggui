package ui

import "github.com/ironpark/ggui"

// rowCursor is a row's key, or none.
type rowCursor[K comparable] struct {
	key K
	ok  bool
}

func at[K comparable](k K) rowCursor[K] { return rowCursor[K]{k, true} }

// rovingRows is the keyboard state of a widget whose keyed rows share one
// Tab stop, as WAI-ARIA grids and trees do: Tab lands on one row and the
// arrows move between the rest.
type rovingRows[T any, K comparable] struct {
	active rowCursor[K] // the row that last had focus
	focus  rowCursor[K] // a row to focus when it next paints

	// The Tab stop, worked out when a Tab asks for it and kept until what
	// it depends on changes, so a Tab past a long list scans it once.
	stop     rowCursor[K]
	stopFrom stopInputs[T, K]
}

type stopInputs[T any, K comparable] struct {
	first            *T
	n                int
	active, selected rowCursor[K]
}

// isStop reports whether the row keyed k is the one Tab lands on: the row
// that last had focus, else the selected one, else the first.
func (v *rovingRows[T, K]) isStop(k K, rows []T, key func(T) K, selected rowCursor[K]) bool {
	in := stopInputs[T, K]{n: len(rows), active: v.active, selected: selected}
	if len(rows) > 0 {
		in.first = &rows[0]
	}
	if in != v.stopFrom {
		v.stopFrom, v.stop = in, rowCursor[K]{}
		for i, item := range rows {
			rk := key(item)
			if v.active.ok && rk == v.active.key {
				v.stop = v.active
				break
			}
			if i == 0 || (selected.ok && rk == selected.key) {
				v.stop = at(rk)
			}
		}
	}
	return v.stop.ok && v.stop.key == k
}

// ask makes the row keyed k the active one and focuses it when it next
// paints, which may be after a scroll brings it into view.
func (v *rovingRows[T, K]) ask(k K) { v.active, v.focus = at(k), at(k) }

// claim reports whether the row keyed k was asked to take the focus, and
// forgets the request when it was.
func (v *rovingRows[T, K]) claim(k K) bool {
	if !v.focus.ok || v.focus.key != k {
		return false
	}
	v.focus = rowCursor[K]{}
	return true
}

// revealRow scrolls offset, the position of a view viewH tall over rows
// rowH tall each, so that row i is in view.
func revealRow(offset *ggui.StateValue[float64], i int, rowH, viewH float64) {
	if viewH <= 0 {
		return
	}
	top, off := float64(i)*rowH, ggui.Untrack(offset.Get)
	switch {
	case top < off:
		offset.Set(top)
	case top+rowH > off+viewH:
		offset.Set(top + rowH - viewH)
	}
}
