package chart

import (
	"cmp"
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func pick[T any](cond bool, a, b T) T { return gem.Pick(cond, a, b) }

func clamp[T cmp.Ordered](v, lo, hi T) T { return gem.Clamp(v, lo, hi) }

func bounded(v, fallback float64) float64 { return gem.Bounded(v, fallback) }

func fade(c color.Color, alpha float64) color.Color { return uitheme.Fade(c, alpha) }

func colorOr(value, fallback color.Color) color.Color {
	if value != nil {
		return value
	}
	return fallback
}

// paintShadows draws the layers of a shadow under r, for a surface that is
// painted rather than a Box.
func paintShadows(dst *ggui.Canvas, r ggui.Rect, radius float64, layers ...ggui.ShadowStyle) {
	for _, s := range layers {
		if s.Color != nil {
			dst.Shadow(r, radius, s)
		}
	}
}

// stepIndex moves cur by dir through n items, wrapping around; from -1 it
// starts at the near end.
func stepIndex(cur, dir, n int) int {
	if n <= 0 {
		return cur
	}
	if cur < 0 {
		return pick(dir < 0, n-1, 0)
	}
	return (cur + dir + n) % n
}

// pointerMotion reports whether the pointer actually moved. PointerMove is
// synthesized every frame, so hover competing with a keyboard highlight has to
// ignore a pointer that is standing still.
type pointerMotion struct {
	last ggui.Point
	seen bool
}

// moved reports whether the pointer is somewhere new; a first sighting counts.
func (m *pointerMotion) moved(p ggui.Point) bool {
	was, seen := m.last, m.seen
	m.last, m.seen = p, true
	return !seen || was != p
}
