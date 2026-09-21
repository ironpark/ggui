package ggui

import (
	"cmp"
	"math"

	"github.com/ironpark/ggui/geom"
	"github.com/ironpark/ggui/internal/fn"
)

// The geometry types live in the geom package, which sits below both this
// one and a11y, so that a bridge measures in the same types layout does.
// They are those types under these names.
type (
	// Number is any built-in numeric type. The geometry constructors take
	// one so that pixel counts from Ebitengine (int) and layout math
	// (float64) can be mixed without a cast at every call site.
	Number = geom.Number

	// Point is a position in logical (device-independent) pixels.
	Point = geom.Point

	// Size is a width/height pair in logical pixels.
	Size = geom.Size

	// Rect is an axis-aligned rectangle anchored at Origin. Paint receives
	// one: the origin its parent chose and the size its own Layout returned.
	Rect = geom.Rect
)

// Pt returns the Point at (x, y), whatever numeric type they are.
func Pt[T Number](x, y T) Point { return geom.Pt(x, y) }

// Sz returns the Size w by h, whatever numeric type they are.
func Sz[T Number](w, h T) Size { return geom.Sz(w, h) }

// Rct returns the Rect at origin with size.
func Rct(origin Point, size Size) Rect { return geom.Rct(origin, size) }

// Unbounded is the maximum a Scroll gives its child along the scroll axis:
// take whatever you need. Widgets that would fill the space they are given
// fall back to their content size on an unbounded axis.
var Unbounded = math.Inf(1)

// bounded returns v, or fallback when v is Unbounded.
func bounded(v, fallback float64) float64 { return fn.Bounded(v, fallback) }

// Constraints bound the size a widget may choose during layout, the same way
// Flutter's BoxConstraints flow down the tree while sizes flow back up.
type Constraints struct {
	MinW, MinH float64
	MaxW, MaxH float64
}

// Tight returns constraints that permit exactly one size.
func Tight(s Size) Constraints {
	return Constraints{MinW: s.W, MinH: s.H, MaxW: s.W, MaxH: s.H}
}

// Loose returns constraints that permit anything up to s.
func Loose(s Size) Constraints {
	return Constraints{MaxW: s.W, MaxH: s.H}
}

// Max returns the largest size c permits.
func (c Constraints) Max() Size { return Size{W: c.MaxW, H: c.MaxH} }

// Loosen returns c with its minimums dropped: anything up to c.Max() passes.
func (c Constraints) Loosen() Constraints { return Loose(c.Max()) }

// Constrain clamps s so that it satisfies c.
func (c Constraints) Constrain(s Size) Size {
	return Size{
		W: clamp(s.W, c.MinW, c.MaxW),
		H: clamp(s.H, c.MinH, c.MaxH),
	}
}

// clamp keeps v within [lo, hi]; lo wins when the bounds contradict.
func clamp[T cmp.Ordered](v, lo, hi T) T { return fn.Clamp(v, lo, hi) }
