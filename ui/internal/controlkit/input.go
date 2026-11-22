package controlkit

import (
	"maps"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
)

// WithKey returns a copy of the set m with k in it when on and out of it
// otherwise, leaving m as it was for whoever else holds it.
func WithKey[K comparable](m map[K]bool, k K, on bool) map[K]bool {
	next := make(map[K]bool, len(m)+1)
	maps.Copy(next, m)
	if on {
		next[k] = true
	} else {
		delete(next, k)
	}
	return next
}

// StepIndex moves cur by dir through n items, wrapping around and skipping
// items enabled rejects (nil rejects none); from -1 it starts at the near
// end. It returns cur when nothing is enabled.
func StepIndex(cur, dir, n int, enabled func(int) bool) int {
	i := cur
	for range n {
		if i < 0 {
			i = gem.Pick(dir < 0, n-1, 0)
		} else {
			i = (i + dir + n) % n
		}
		if enabled == nil || enabled(i) {
			return i
		}
	}
	return cur
}

// PointerMotion reports whether the pointer actually moved. PointerMove is
// synthesized every frame, so hover competing with a keyboard highlight has to
// ignore a pointer that is standing still.
type PointerMotion struct {
	last ggui.Point
	seen bool
}

// Moved reports whether the pointer is somewhere new; a first sighting counts.
func (m *PointerMotion) Moved(p ggui.Point) bool {
	was, seen := m.last, m.seen
	m.last, m.seen = p, true
	return !seen || was != p
}

// Drifted is Moved without the first sighting, so a menu opened from the
// keyboard is not stolen by a pointer that has not actually moved.
func (m *PointerMotion) Drifted(p ggui.Point) bool {
	seen := m.seen
	return m.Moved(p) && seen
}

// Beyond reports whether ev is a move a header hears only because the
// pointer is over a region painted inside its own but outside at, the
// header's Rect: a disclosure's body, painted after the header registered
// its region and so inside it. A header passes such a move, so that it is
// hovered over itself and the controls in it, not over the body it
// reveals.
func Beyond(ev ggui.PointerEvent, at ggui.Rect) bool {
	return ev.Kind == ggui.PointerMove && ev.Inner && !at.Contains(ev.Pos)
}

// DirectionalKey maps physical horizontal movement to logical item order.
func DirectionalKey(ev ggui.KeyEvent, rtl bool) ggui.KeyEvent {
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
