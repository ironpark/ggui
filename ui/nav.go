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
// wrapping past the ends when wrap is set and stopping at them otherwise,
// and Home and End go to the first and last enabled item. A step from -1,
// nothing yet, starts at the near end. ok is false for any other key or
// event. With nothing enabled a step stays at cur and Home and End give -1.
func navIndex(ev ggui.KeyEvent, axis navAxis, wrap bool, cur, n int, enabled func(int) bool) (i int, ok bool) {
	if ev.Kind != ggui.KeyPress {
		return cur, false
	}
	switch navMoveOf(ev.Key, axis) {
	case navPrev:
		return stepIndex(cur, -1, n, wrap, enabled), true
	case navNext:
		return stepIndex(cur, 1, n, wrap, enabled), true
	case navFirst:
		return stepIndex(-1, 1, n, true, enabled), true
	case navLast:
		return stepIndex(-1, -1, n, true, enabled), true
	}
	return cur, false
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
