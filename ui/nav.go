package ui

import "github.com/ironpark/ggui"

// navAxis is which arrow keys move the keyboard through a list-like
// widget: Up and Down, Left and Right, or both.
type navAxis uint8

const (
	navVertical   navAxis = 1 << iota // Up and Down
	navHorizontal                     // Left and Right
	navBoth       = navVertical | navHorizontal
)

// navMove is where a navigation key asks to go.
type navMove uint8

const (
	navNone  navMove = iota
	navPrev          // Up or Left: the item before
	navNext          // Down or Right: the item after
	navFirst         // Home
	navLast          // End
)

// navMoveOf is the move key makes along axis, navNone for any other key.
func navMoveOf(key ggui.KeyboardKey, axis navAxis) navMove {
	switch {
	case key == ggui.KeyHome:
		return navFirst
	case key == ggui.KeyEnd:
		return navLast
	case key == ggui.KeyArrowUp && axis&navVertical != 0, key == ggui.KeyArrowLeft && axis&navHorizontal != 0:
		return navPrev
	case key == ggui.KeyArrowDown && axis&navVertical != 0, key == ggui.KeyArrowRight && axis&navHorizontal != 0:
		return navNext
	}
	return navNone
}

// navConsumes reports whether ev is a press of a key that moves along
// axis: what a list-like widget's ConsumesKey keeps from the focus keys.
func navConsumes(ev ggui.KeyEvent, axis navAxis) bool {
	return ev.Kind == ggui.KeyPress && navMoveOf(ev.Key, axis) != navNone
}

// navIndex is the item a key press moves the keyboard to from cur, one of
// n items, skipping those enabled rejects (nil rejects none), as WAI-ARIA
// has composite widgets do: the arrows step to the next enabled item,
// wrapping past the ends, and Home and End go to the first and last
// enabled item. A step from -1,
// nothing yet, starts at the near end. ok is false for any other key or
// event. With nothing enabled a step stays at cur and Home and End give -1.
func navIndex(ev ggui.KeyEvent, axis navAxis, cur, n int, enabled func(int) bool) (i int, ok bool) {
	if ev.Kind != ggui.KeyPress {
		return cur, false
	}
	switch navMoveOf(ev.Key, axis) {
	case navPrev:
		return stepIndex(cur, -1, n, enabled), true
	case navNext:
		return stepIndex(cur, 1, n, enabled), true
	case navFirst:
		return stepIndex(-1, 1, n, enabled), true
	case navLast:
		return stepIndex(-1, -1, n, enabled), true
	}
	return cur, false
}

// navIndexBounded is navIndex for a list that does not wrap: an arrow at
// the last enabled item, or the first, stays there.
func navIndexBounded(ev ggui.KeyEvent, axis navAxis, cur, n int, enabled func(int) bool) (i int, ok bool) {
	if ev.Kind != ggui.KeyPress {
		return cur, false
	}
	move := navMoveOf(ev.Key, axis)
	if cur < 0 || cur >= n || move != navPrev && move != navNext {
		return navIndex(ev, axis, cur, n, enabled)
	}
	dir := pick(move == navPrev, -1, 1)
	for i := cur + dir; i >= 0 && i < n; i += dir {
		if enabled == nil || enabled(i) {
			return i, true
		}
	}
	return cur, true
}

// navTarget is the row a navigation key takes the keyboard to from row i
// of n, unclamped, for a list that clamps and skips rows itself as it
// moves: a ListBox, Tree or Table. ok is false for any other key.
func navTarget(key ggui.KeyboardKey, axis navAxis, i, n int) (int, bool) {
	switch navMoveOf(key, axis) {
	case navPrev:
		return i - 1, true
	case navNext:
		return i + 1, true
	case navFirst:
		return 0, true
	case navLast:
		return n - 1, true
	}
	return 0, false
}

// directionalKey maps physical horizontal movement to logical item order.
func directionalKey(ev ggui.KeyEvent, rtl bool) ggui.KeyEvent {
	if rtl {
		switch ev.Key {
		case ggui.KeyArrowLeft:
			ev.Key = ggui.KeyArrowRight
		case ggui.KeyArrowRight:
			ev.Key = ggui.KeyArrowLeft
		}
	}
	return ev
}

// mirrorHorizontal mirrors a local rectangle without changing its hit identity.
func mirrorHorizontal(r, parent ggui.Rect, rtl bool) ggui.Rect {
	if rtl {
		r.Origin.X = parent.Origin.X + parent.Size.W - (r.Origin.X - parent.Origin.X) - r.Size.W
	}
	return r
}
