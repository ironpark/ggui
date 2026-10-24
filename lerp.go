package ggui

import (
	"image/color"
	"time"
)

// A lerp moves between two values: it returns a at t = 0, b at t = 1, and
// what lies between for the rest. TweenWith takes one, so a tween can move
// any type; the ones here cover the values widgets are configured with.
// t may leave [0, 1] under an easing that overshoots, such as EaseOutBack;
// these extrapolate along the same line, and LerpColor clamps.

// LerpColor blends a into b in premultiplied sRGB, as CSS transitions do,
// so a fade to or from transparent does not darken through a gray fringe.
// A nil color is transparent. It returns a and b themselves at the ends,
// so a theme token tweened to stays that token.
func LerpColor(a, b color.Color, t float64) color.Color {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return ColorVector.Join(lerp4(ColorVector.Split(a), ColorVector.Split(b), t))
}

// LerpFloat is the straight line from a to b.
func LerpFloat(a, b, t float64) float64 { return a + (b-a)*t }

// LerpPoint moves along the straight line from a to b.
func LerpPoint(a, b Point, t float64) Point { return PointVector.Lerp(a, b, t) }

// LerpSize grows or shrinks a toward b.
func LerpSize(a, b Size, t float64) Size { return SizeVector.Lerp(a, b, t) }

// LerpRect moves and resizes a toward b.
func LerpRect(a, b Rect, t float64) Rect { return RectVector.Lerp(a, b, t) }

// LerpInsets moves each side of a toward b's.
func LerpInsets(a, b EdgeInsets, t float64) EdgeInsets { return InsetsVector.Lerp(a, b, t) }

func lerp4(a, b [4]float64, t float64) (c [4]float64) {
	for i := range c {
		c[i] = LerpFloat(a[i], b[i], t)
	}
	return c
}

// Vector splits a value into at most four numbers and joins them back, so a
// Spring can move each number on its own. Unused numbers stay zero.
type Vector[T any] struct {
	Split func(T) [4]float64
	Join  func([4]float64) T
}

// Lerp blends a into b number by number: the lerp a Vector implies.
func (v Vector[T]) Lerp(a, b T, t float64) T { return v.Join(lerp4(v.Split(a), v.Split(b), t)) }

var (
	// ColorVector splits a color into premultiplied red, green, blue and
	// alpha from 0 to 255. Join clamps, so a spring overshooting a color
	// stays a valid one.
	ColorVector = Vector[color.Color]{Split: splitColor, Join: joinColor}

	// PointVector splits a Point into X and Y.
	PointVector = Vector[Point]{
		Split: func(p Point) [4]float64 { return [4]float64{p.X, p.Y} },
		Join:  func(c [4]float64) Point { return Pt(c[0], c[1]) },
	}

	// SizeVector splits a Size into W and H.
	SizeVector = Vector[Size]{
		Split: func(s Size) [4]float64 { return [4]float64{s.W, s.H} },
		Join:  func(c [4]float64) Size { return Sz(c[0], c[1]) },
	}

	// RectVector splits a Rect into its origin and size.
	RectVector = Vector[Rect]{
		Split: func(r Rect) [4]float64 { return [4]float64{r.Origin.X, r.Origin.Y, r.Size.W, r.Size.H} },
		Join:  func(c [4]float64) Rect { return Rct(Pt(c[0], c[1]), Sz(c[2], c[3])) },
	}

	// InsetsVector splits EdgeInsets into top, right, bottom and left.
	InsetsVector = Vector[EdgeInsets]{
		Split: func(e EdgeInsets) [4]float64 { return [4]float64{e.Top, e.Right, e.Bottom, e.Left} },
		Join:  func(c [4]float64) EdgeInsets { return EdgeInsets{Top: c[0], Right: c[1], Bottom: c[2], Left: c[3]} },
	}
)

func splitColor(c color.Color) [4]float64 {
	if c == nil {
		return [4]float64{}
	}
	r, g, b, a := c.RGBA()
	const k = 255.0 / 0xffff
	return [4]float64{float64(r) * k, float64(g) * k, float64(b) * k, float64(a) * k}
}

func joinColor(c [4]float64) color.Color {
	a := clamp(c[3], 0, 255)
	ch := func(v float64) uint16 { return uint16(clamp(v, 0, a)*0xffff/255 + 0.5) }
	return color.RGBA64{R: ch(c[0]), G: ch(c[1]), B: ch(c[2]), A: uint16(a*0xffff/255 + 0.5)}
}

// TweenColor creates a Tweened color that blends with LerpColor.
//
//	fill := ggui.TweenColor(idle, 150*time.Millisecond)
//	fill.Set(hover)
func TweenColor(c color.Color, d time.Duration) *Tweened[color.Color] {
	return TweenWith(c, d, LerpColor)
}

// TweenOfWith creates a Tweened that follows src, as TweenOf does, for any
// type lerp can move. It needs an owner, like Watch.
func TweenOfWith[T any](src Readable[T], d time.Duration, lerp func(a, b T, t float64) T) *Tweened[T] {
	t := TweenWith(Untrack(src.Get), d, lerp)
	Watch(src, t.Set)
	return t
}

// SpringOfWith creates a Sprung that follows src, as SpringOf does, for any
// type vec splits. It needs an owner, like Watch.
func SpringOfWith[T any](src Readable[T], vec Vector[T]) *Sprung[T] {
	s := SpringWith(Untrack(src.Get), vec)
	Watch(src, s.Set)
	return s
}
