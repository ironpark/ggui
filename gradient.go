package ggui

import (
	"cmp"
	"image/color"
	"math"
	"slices"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/render"
)

// GradientStop is one colour of a Gradient, at Offset along it: 0 at its
// start, 1 at its end.
type GradientStop struct {
	Offset float64
	Color  color.Color
}

// Stops spreads colors evenly along a gradient, the first at 0 and the last
// at 1.
func Stops(colors ...color.Color) []GradientStop {
	out := make([]GradientStop, len(colors))
	for i, c := range colors {
		out[i] = GradientStop{Color: c}
		if len(colors) > 1 {
			out[i].Offset = float64(i) / float64(len(colors)-1)
		}
	}
	return out
}

type gradientKind int

const (
	gradientLinear gradientKind = render.Linear
	gradientRadial gradientKind = render.Radial
	gradientSweep  gradientKind = render.Sweep
)

// Gradient is a fill that blends colours across the Rect it fills: along a
// line at an angle, outwards from a centre, or around one. It is placed
// relative to that Rect, so one Gradient fits a Box of any size. Colours
// blend premultiplied and hold their end colours beyond the first and last
// stop. Up to 16 stops are drawn; more are dropped. The zero Gradient
// fills nothing.
//
//	ggui.LinearGradient(math.Pi/4, ggui.Stops(from, to)...)
//	ggui.RadialGradient(ggui.Stops(glow, transparent)...).Center(0.5, 0.3)
type Gradient struct {
	kind   gradientKind
	stops  []GradientStop
	angle  float64
	cx, cy float64
	radius float64
}

// LinearGradient blends along a line through the Rect's centre at angle
// radians, clockwise from pointing right: 0 runs left to right, π/2 top to
// bottom. The line is as long as it takes for the first and last stops to
// fall on the Rect's far corners, as CSS's does.
func LinearGradient(angle float64, stops ...GradientStop) Gradient {
	g := newGradient(gradientLinear, stops)
	g.angle = angle
	return g
}

// RadialGradient blends outwards in a circle from the Rect's centre, the
// last stop at half its shorter side until Radius says otherwise.
func RadialGradient(stops ...GradientStop) Gradient {
	return newGradient(gradientRadial, stops)
}

// SweepGradient blends clockwise around the Rect's centre, a full turn from
// pointing right until StartAngle says otherwise: a colour wheel, or a
// conic progress ring.
func SweepGradient(stops ...GradientStop) Gradient {
	return newGradient(gradientSweep, stops)
}

func newGradient(kind gradientKind, stops []GradientStop) Gradient {
	s := slices.Clone(stops)
	for i := range s {
		s[i].Offset = clamp(s[i].Offset, 0, 1)
	}
	slices.SortStableFunc(s, func(a, b GradientStop) int { return cmp.Compare(a.Offset, b.Offset) })
	return Gradient{kind: kind, stops: s, cx: 0.5, cy: 0.5, radius: 0.5}
}

// Center places a radial or sweep gradient's centre at fractions of the
// Rect: (0, 0) is its top-left corner, (0.5, 0.5) its middle, the default.
func (g Gradient) Center(x, y float64) Gradient {
	g.cx, g.cy = x, y
	return g
}

// Radius sets how far a radial gradient reaches, as a fraction of the
// Rect's shorter side; the default is 0.5.
func (g Gradient) Radius(r float64) Gradient {
	g.radius = r
	return g
}

// StartAngle turns a sweep gradient so that it begins rad radians clockwise
// from pointing right.
func (g Gradient) StartAngle(rad float64) Gradient {
	g.angle = rad
	return g
}

// IsZero reports whether g has no stops and so fills nothing.
func (g Gradient) IsZero() bool { return len(g.stops) == 0 }

// themed reports whether a stop is an EnvColor.
func (g Gradient) themed() bool {
	return slices.ContainsFunc(g.stops, func(s GradientStop) bool { return IsEnvColor(s.Color) })
}

// resolve returns g with its EnvColor stops resolved in env.
func (g Gradient) resolve(env Env) Gradient {
	if !g.themed() {
		return g
	}
	g.stops = slices.Clone(g.stops)
	for i, s := range g.stops {
		g.stops[i].Color = ResolveColor(s.Color, env)
	}
	return g
}

// device returns g placed over the logical Rect r on c, in Image pixels.
func (g Gradient) device(c *Canvas, r Rect) render.Gradient {
	out := render.Gradient{Kind: int(g.kind)}
	for _, s := range g.stops {
		if s.Color != nil {
			out.Stops = append(out.Stops, render.Stop{Offset: s.Offset, Color: render.Premul(s.Color)})
		}
	}
	if g.kind == gradientLinear {
		sin, cos := math.Sincos(g.angle)
		half := math.Abs(r.Size.W/2*cos) + math.Abs(r.Size.H/2*sin)
		mid := r.Center()
		out.Start = Pt(c.px(mid.X-cos*half), c.px(mid.Y-sin*half))
		out.End = Pt(c.px(mid.X+cos*half), c.px(mid.Y+sin*half))
		return out
	}
	out.Start = Pt(c.px(r.Origin.X+r.Size.W*g.cx), c.px(r.Origin.Y+r.Size.H*g.cy))
	if g.kind == gradientRadial {
		radius := c.px(g.radius * min(r.Size.W, r.Size.H))
		out.End = Pt(radius, radius)
	} else {
		out.End = Pt(g.angle, 0)
	}
	return out
}

// FillGradientRect fills the logical Rect r with g placed over r.
func (c *Canvas) FillGradientRect(r Rect, g Gradient) { c.FillGradientRoundRect(r, 0, g) }

// FillGradientRoundRect fills r with g, with corners rounded by radius,
// antialiased as FillRoundRect is.
func (c *Canvas) FillGradientRoundRect(r Rect, radius float64, g Gradient) {
	if c == nil || c.Image == nil || g.IsZero() || r.Empty() || !c.visiblePaintBounds(r, 0) {
		return
	}
	d := g.device(c, r)
	centre := r.Center()
	render.FillGradientRoundRect(c.Image, &d, Pt(c.px(centre.X), c.px(centre.Y)),
		Sz(c.px(r.Size.W/2), c.px(r.Size.H/2)), c.px(max(radius, 0)))
}

// FillGradientPath fills path with g placed over bounds, which is usually
// the Rect the path was drawn for; the path is cut to bounds. Like
// FillPathGradient it fills through a coverage mask kept on the path, rebuilt
// only when the path, bounds or scale change.
func (c *Canvas) FillGradientPath(path *Path, bounds Rect, g Gradient) {
	if c == nil || c.Image == nil || path == nil || g.IsZero() || bounds.Empty() {
		return
	}
	mask, at, ok := path.coverage(c, bounds)
	if !ok {
		return
	}
	d := g.device(c, bounds)
	render.FillGradientMask(c.Image, mask, &d, at)
}

// Gradient fills the box with g, over its Fill and under its border, with
// the same rounded corners. A zero Gradient removes it. EnvColor stops are
// resolved at layout, as Fill's color is.
func (b *BoxWidget) Gradient(g Gradient) *BoxWidget {
	if property.Equal(g, b.gradient) {
		return b
	}
	if g.themed() || b.gradient.themed() {
		b.props.Changed()
	}
	b.gradient, b.shownGradient = g, g
	return b
}
