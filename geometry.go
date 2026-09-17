package ggui

import "cmp"

// Number is any built-in numeric type. The geometry constructors take one so
// that pixel counts from Ebitengine (int) and layout math (float64) can be
// mixed without a cast at every call site.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// Point is a position in logical (device-independent) pixels.
type Point struct {
	X, Y float64
}

// Pt returns the Point at (x, y), whatever numeric type they are.
func Pt[T Number](x, y T) Point { return Point{X: float64(x), Y: float64(y)} }

// Add returns the component-wise sum of p and q.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Size is a width/height pair in logical pixels.
type Size struct {
	W, H float64
}

// Sz returns the Size w by h, whatever numeric type they are.
func Sz[T Number](w, h T) Size { return Size{W: float64(w), H: float64(h)} }

// Rect is an axis-aligned rectangle anchored at Origin. Paint receives one:
// the origin its parent chose and the size its own Layout returned.
type Rect struct {
	Origin Point
	Size   Size
}

// Rct returns the Rect at origin with size.
func Rct(origin Point, size Size) Rect { return Rect{Origin: origin, Size: size} }

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
func clamp[T cmp.Ordered](v, lo, hi T) T {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
