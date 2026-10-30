package ggui

import (
	"cmp"
	"math"

	"github.com/ironpark/ggui/internal/fn"
	"github.com/ironpark/ggui/pkg/layout"
)

// Geometry and constraints are owned by the renderer-independent layout
// package. These aliases keep ggui's public geometry API unchanged.
type (
	// Number is any built-in numeric type. The geometry constructors take
	// one so that pixel counts from ggfx (int) and layout math
	// (float64) can be mixed without a cast at every call site.
	Number = layout.Number

	// Point is a position in logical (device-independent) pixels.
	Point = layout.Point

	// Size is a width/height pair in logical pixels.
	Size = layout.Size

	// Rect is an axis-aligned rectangle anchored at Origin. Paint receives
	// one: the origin its parent chose and the size its own Layout returned.
	Rect = layout.Rect
)

// Pt returns the Point at (x, y), whatever numeric type they are.
func Pt[T Number](x, y T) Point { return layout.Pt(x, y) }

// Sz returns the Size w by h, whatever numeric type they are.
func Sz[T Number](w, h T) Size { return layout.Sz(w, h) }

// Rct returns the Rect at origin with size.
func Rct(origin Point, size Size) Rect { return layout.Rct(origin, size) }

// Unbounded is the maximum a Scroll gives its child along the scroll axis:
// take whatever you need. Widgets that would fill the space they are given
// fall back to their content size on an unbounded axis.
var Unbounded = math.Inf(1)

// bounded returns v, or fallback when v is Unbounded.
func bounded(v, fallback float64) float64 { return fn.Bounded(v, fallback) }

// Constraints bound the size a widget may choose during layout.
type Constraints = layout.Constraints

// Tight returns constraints that permit exactly one size.
func Tight(s Size) Constraints { return layout.Tight(s) }

// Loose returns constraints that permit anything up to s.
func Loose(s Size) Constraints { return layout.Loose(s) }

// clamp keeps v within [lo, hi]; lo wins when the bounds contradict.
func clamp[T cmp.Ordered](v, lo, hi T) T { return fn.Clamp(v, lo, hi) }
