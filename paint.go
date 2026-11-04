package ggui

import (
	"cmp"
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/vector"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/render"
	"github.com/ironpark/ggui/pkg/geom"
)

// Path is a reusable vector outline in logical pixels. Mutating a path invalidates
// its device-scale cache. Like widgets, paths belong to the UI thread.
type Path struct {
	gradientMask *ggfx.Image
	gradientKey  gradientKey
	gradientRev  uint64
	path         vector.Path
	scaled       vector.Path
	scale        float64
	scaledRev    uint64
	// revision counts mutations. Each cache records the revision it was built
	// from, so a new mutator only has to bump this one field to invalidate them
	// all rather than remember a flag per cache.
	revision uint64

	// outline is the path as its points, for Contains and Bounds.
	outline geom.Outline
}

// gradientKey is everything besides path geometry that the coverage mask depends on.
type gradientKey struct {
	bounds   Rect
	coverage image.Rectangle
	scale    float64
}

func (p *Path) MoveTo(x, y float64) {
	p.path.MoveTo(float32(x), float32(y))
	p.outline.MoveTo(Pt(x, y))
	p.revision++
}
func (p *Path) LineTo(x, y float64) {
	p.path.LineTo(float32(x), float32(y))
	p.outline.LineTo(Pt(x, y))
	p.revision++
}

// QuadTo appends a quadratic Bezier segment.
func (p *Path) QuadTo(x1, y1, x2, y2 float64) {
	p.path.QuadTo(float32(x1), float32(y1), float32(x2), float32(y2))
	p.outline.QuadTo(Pt(x1, y1), Pt(x2, y2))
	p.revision++
}
func (p *Path) CubicTo(x1, y1, x2, y2, x3, y3 float64) {
	p.path.CubicTo(float32(x1), float32(y1), float32(x2), float32(y2), float32(x3), float32(y3))
	p.outline.CubicTo(Pt(x1, y1), Pt(x2, y2), Pt(x3, y3))
	p.revision++
}

// Arc starts a subpath on the circle about (cx, cy) of radius r, from angle
// start to end in radians, measured from +x and turning clockwise on
// screen as the angle grows. A sweep of 2π or more is the whole circle.
func (p *Path) Arc(cx, cy, r, start, end float64) {
	sweep := max(min(end-start, 2*math.Pi), -2*math.Pi)
	at := func(a float64) (float64, float64) { return cx + r*math.Cos(a), cy + r*math.Sin(a) }
	p.MoveTo(at(start))
	// A cubic per quarter turn or less stays within 0.03% of the circle.
	n := max(1, int(math.Ceil(math.Abs(sweep)/(math.Pi/2)-1e-9)))
	step := sweep / float64(n)
	k := 4.0 / 3 * math.Tan(step/4) * r
	for i := range n {
		a0, a1 := start+step*float64(i), start+step*float64(i+1)
		x0, y0 := at(a0)
		x1, y1 := at(a1)
		p.CubicTo(x0-k*math.Sin(a0), y0+k*math.Cos(a0), x1+k*math.Sin(a1), y1-k*math.Cos(a1), x1, y1)
	}
}

func (p *Path) Close() { p.path.Close(); p.outline.Close(); p.revision++ }
func (p *Path) Reset() { p.path.Reset(); p.outline.Reset(); p.revision++ }

// Contains reports whether pt is inside the path as FillPath fills it: by
// the nonzero rule, with every subpath closed. Curves are followed to within
// a sixteenth of their length.
func (p *Path) Contains(pt Point) bool {
	if p == nil {
		return false
	}
	return p.outline.Contains(pt)
}

// Bounds returns the smallest Rect holding the path's outline, or the zero
// Rect for an empty path.
func (p *Path) Bounds() Rect {
	if p == nil {
		return Rect{}
	}
	return p.outline.Bounds()
}

// ovalPath is the ellipse inscribed in r, as four cubic arcs.
func ovalPath(r Rect) *Path {
	const k = 0.5522847498307936 // 4/3·(√2−1), a quarter circle's handle
	cx, cy := r.Origin.X+r.Size.W/2, r.Origin.Y+r.Size.H/2
	rx, ry := r.Size.W/2, r.Size.H/2
	p := &Path{}
	p.MoveTo(cx+rx, cy)
	p.CubicTo(cx+rx, cy+k*ry, cx+k*rx, cy+ry, cx, cy+ry)
	p.CubicTo(cx-k*rx, cy+ry, cx-rx, cy+k*ry, cx-rx, cy)
	p.CubicTo(cx-rx, cy-k*ry, cx-k*rx, cy-ry, cx, cy-ry)
	p.CubicTo(cx+k*rx, cy-ry, cx+rx, cy-k*ry, cx+rx, cy)
	p.Close()
	return p
}
func (p *Path) device(scale float64) *vector.Path {
	if scale == 1 {
		return &p.path
	}
	if p.scaledRev != p.revision || p.scale != scale {
		p.scaled.Reset()
		op := &vector.AddPathOptions{}
		op.GeoM.Scale(scale, scale)
		p.scaled.AddPath(&p.path, op)
		p.scale = scale
		p.scaledRev = p.revision
	}
	return &p.scaled
}

// FillPath fills a closed outline, honoring the canvas clip and device scale.
func (c *Canvas) FillPath(path *Path, col color.Color) {
	if c == nil || c.Image == nil || path == nil || col == nil {
		return
	}
	vector.FillPath(c.Image, path.device(c.Scale()), nil, pathOptions(col))
}

// StrokePath strokes an outline with round joins and a logical pixel width.
func (c *Canvas) StrokePath(path *Path, width float64, col color.Color) {
	if c == nil || c.Image == nil || path == nil || col == nil || width <= 0 {
		return
	}
	vector.StrokePath(c.Image, path.device(c.Scale()), &vector.StrokeOptions{Width: c.Px(width), LineJoin: vector.LineJoinRound}, pathOptions(col))
}

// FillPathGradient fills a path with a vertical premultiplied color gradient
// from top to bottom of bounds: FillGradientPath with a LinearGradient at
// π/2. A reusable GPU coverage mask avoids translucent fan-triangulation
// artifacts. The mask is rebuilt only when geometry, bounds or device scale
// changes.
func (c *Canvas) FillPathGradient(path *Path, bounds Rect, top, bottom color.Color) {
	if top == nil || bottom == nil {
		return
	}
	c.FillGradientPath(path, bounds, LinearGradient(math.Pi/2, Stops(top, bottom)...))
}

// coverage returns the path's coverage mask within bounds on c and where
// its first pixel goes, building it if the path, bounds or scale changed.
func (p *Path) coverage(c *Canvas, bounds Rect) (*ggfx.Image, image.Point, bool) {
	scale := c.Scale()
	device := p.device(scale)
	coverage := device.Bounds().Intersect(c.physical(bounds))
	w, h := coverage.Dx(), coverage.Dy()
	if w <= 0 || h <= 0 {
		return nil, image.Point{}, false
	}
	key := gradientKey{bounds: bounds, coverage: coverage, scale: scale}
	if p.gradientRev != p.revision || p.gradientKey != key || p.gradientMask == nil {
		if p.gradientMask == nil || p.gradientMask.Bounds().Dx() != w || p.gradientMask.Bounds().Dy() != h {
			if p.gradientMask != nil {
				p.gradientMask.Deallocate()
			}
			p.gradientMask = ggfx.NewImage(w, h)
		} else {
			p.gradientMask.Clear()
		}
		var local vector.Path
		op := &vector.AddPathOptions{}
		op.GeoM.Translate(-float64(coverage.Min.X), -float64(coverage.Min.Y))
		local.AddPath(device, op)
		vector.FillPath(p.gradientMask, &local, nil, pathOptions(color.White))
		p.gradientKey = key
		p.gradientRev = p.revision
	}
	return p.gradientMask, coverage.Min, true
}

// ClipPath is Clip to the inside of path, antialiased, for a subtree: paint
// draws into a Layer covering the path's bounds, which is composited through
// the path filled as FillPath fills it. Input is clipped to the same shape,
// so a pointer outside the path does not reach what paint registers there.
// A nil path draws nothing.
func (c *Canvas) ClipPath(path *Path, paint func(dst *Canvas)) {
	if path == nil {
		return
	}
	polys := path.outline.Polygons()
	c.clipShape(path.Bounds(), func(p Point) bool { return geom.PolygonsContain(polys, p) }, func(mask *Canvas) {
		mask.FillPath(path, color.White)
	}, paint)
}

// ClipOval is ClipPath to the ellipse inscribed in r: a round avatar of
// widgets, or a circular reveal.
func (c *Canvas) ClipOval(r Rect, paint func(dst *Canvas)) {
	if r.Empty() {
		return
	}
	centre := r.Center()
	rx, ry := r.Size.W/2, r.Size.H/2
	inside := func(p Point) bool {
		dx, dy := (p.X-centre.X)/rx, (p.Y-centre.Y)/ry
		return dx*dx+dy*dy <= 1
	}
	c.clipShape(r, inside, func(mask *Canvas) {
		if rx == ry {
			mask.FillCircle(centre, rx, color.White)
			return
		}
		mask.FillPath(ovalPath(r), color.White)
	}, paint)
}

// clipShape clips paint to bounds as a Rect and, within it, to the shape
// inside tests and drawMask fills in white on a Canvas like c's.
func (c *Canvas) clipShape(bounds Rect, inside func(Point) bool, drawMask func(*Canvas), paint func(*Canvas)) {
	clip := c.Clip(bounds)
	if clip == nil {
		paint(nil)
		return
	}
	shaped := clip.enter(Affine{}, Affine{}, inside)
	if shaped.Image == nil || shaped.Image.Bounds().Empty() {
		paint(shaped)
		return
	}
	shaped.layer(shaped.Image.Bounds(), paint, func(img *ggfx.Image) {
		shaped.scratch(img.Bounds(), func(mask *ggfx.Image) {
			drawMask(shaped.into(mask))
			// The mask keeps the layer where it is opaque.
			op := &ggfx.DrawImageOptions{Blend: ggfx.BlendSourceIn}
			op.GeoM.Translate(float64(img.Bounds().Min.X), float64(img.Bounds().Min.Y))
			mask.DrawImage(img, op)
			op = &ggfx.DrawImageOptions{}
			op.GeoM.Translate(float64(mask.Bounds().Min.X), float64(mask.Bounds().Min.Y))
			shaped.Image.DrawImage(mask, op)
		})
	})
}

// scratch lends fn the part of a cleared layer image within area, the whole
// window when empty: a Layer's, or one more for an effect that needs it.
func (c *Canvas) scratch(area image.Rectangle, fn func(img *ggfx.Image)) {
	f := c.fs()
	img := f.layers.Borrow(c.root().Image.Bounds(), area, newLayerImage)
	defer f.layers.Return()
	if !area.Empty() {
		img = img.SubImage(area).(*ggfx.Image)
	}
	img.Clear()
	fn(img)
}

// ClipPathWidget clips its child to a path built for the Rect it paints in,
// input included. Build one with ClipPath or ClipOval.
type ClipPathWidget struct {
	child Widget
	shape func(r Rect) *Path
	oval  bool
}

// ClipPath clips child to the path shape returns for the Rect the child
// paints in, in the same coordinates:
//
//	ggui.ClipPath(photo, func(r ggui.Rect) *ggui.Path { return star(r) })
func ClipPath(child Widget, shape func(r Rect) *Path) *ClipPathWidget {
	return &ClipPathWidget{child: child, shape: shape}
}

// ClipOval clips child to the ellipse inscribed in its Rect, a circle when
// the child is square.
func ClipOval(child Widget) *ClipPathWidget { return &ClipPathWidget{child: child, oval: true} }

// Baseline implements Baseliner: the child's.
func (w *ClipPathWidget) Baseline() (float64, bool) { return baselineOf(w.child) }

// Layout implements Widget.
func (w *ClipPathWidget) Layout(c Constraints, env Env) Size { return w.child.Layout(c, env) }

// Paint implements Widget.
func (w *ClipPathWidget) Paint(dst *Canvas, r Rect) {
	paint := func(dst *Canvas) { dst.Paint(w.child, r) }
	if w.oval {
		dst.ClipOval(r, paint)
		return
	}
	dst.ClipPath(w.shape(r), paint)
}

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

// LayerOptions says how Layer draws the finished layer. The zero value draws
// it as it was painted.
type LayerOptions struct {
	// Fade is how far the layer fades out: 0 draws it opaque, 1 not at all.
	Fade float64
	// Scale sizes the layer about the logical point About. 0 and 1 leave it
	// at its size.
	Scale float64
	About Point
}

// Layer paints into an offscreen image the size of the window, then draws
// that image onto c as o says: the way to apply an opacity or a transform to
// a subtree as a whole, which a Transition's fade and scale do. paint gets a
// Canvas like c in every respect but the image it draws into, so hit regions,
// clipping, focus traps and inertness are what they would be without the
// layer. Layers nest.
//
// The image is the window's, not the caller's: it goes back when paint
// returns, the next Layer reuses it, and one no Layer needed during a frame
// is freed when the frame ends. A widget holds no texture of its own, so
// nothing is left for the garbage collector when it goes away.
//
// Without an image to draw into, Layer calls paint with c.
func (c *Canvas) Layer(o LayerOptions, paint func(layer *Canvas)) {
	scaled := o.Scale != 0 && o.Scale != 1
	// Unscaled, nothing painted outside the clip can show, so the layer
	// is cleared and composited over the clip alone.
	var area image.Rectangle
	if !scaled && c != nil && c.Image != nil {
		area = c.Image.Bounds()
	}
	c.layer(area, paint, func(img *ggfx.Image) {
		op := &ggfx.DrawImageOptions{}
		// A source is drawn from its corner, a destination's coordinates
		// are the window's.
		op.GeoM.Translate(float64(img.Bounds().Min.X), float64(img.Bounds().Min.Y))
		if scaled {
			op.Filter = ggfx.FilterLinear
			cx, cy := c.px(o.About.X), c.px(o.About.Y)
			op.GeoM.Translate(-cx, -cy)
			op.GeoM.Scale(o.Scale, o.Scale)
			op.GeoM.Translate(cx, cy)
		}
		if o.Fade != 0 {
			op.ColorScale.ScaleAlpha(float32(1 - o.Fade))
		}
		c.Image.DrawImage(img, op)
	})
}

// ClipRoundRect is Clip with the corners of r rounded by radius, antialiased,
// for a subtree: a card whose content runs to its rounded edge, say. paint
// draws into a Layer covering r alone, which is composited through the
// rounded shape. Hit regions are clipped to r as a rectangle. A zero radius
// is Clip. A single image is cheaper drawn with ImageOptions.Radius.
func (c *Canvas) ClipRoundRect(r Rect, radius float64, paint func(dst *Canvas)) {
	clip := c.Clip(r)
	if radius <= 0 || clip == nil || clip.Image == nil || clip.Image.Bounds().Empty() {
		paint(clip)
		return
	}
	clip.layer(clip.Image.Bounds(), paint, func(img *ggfx.Image) { clip.maskRoundRect(img, r, radius) })
}

// layer paints into the part of a borrowed layer image within area, the
// whole window when empty, and hands that part to draw. Without an image it
// paints into c.
func (c *Canvas) layer(area image.Rectangle, paint func(*Canvas), draw func(*ggfx.Image)) {
	if c == nil || c.Image == nil || c.root().Image == nil {
		paint(c)
		return
	}
	c.scratch(area, func(img *ggfx.Image) {
		paint(c.into(img))
		draw(img)
	})
}

// newLayerImage makes an image for a layer. Unmanaged, the layer is a
// texture of its own size: on the atlas it would take a page rounded up to
// powers of two, 4096×2048 for a 3252×1534 window, 32 MB rather than 20.
func newLayerImage(b image.Rectangle) *ggfx.Image {
	return ggfx.NewImageWithOptions(b, &ggfx.NewImageOptions{Unmanaged: true})
}

// trimLayers ends a frame for the layer images: it keeps as many as the frame
// had open at once and frees the rest.
func (c *Canvas) trimLayers() {
	if f := c.frame; f != nil {
		f.layers.EndFrame()
	}
}

// freeLayers frees every layer image, for an App or Probe that is closing.
func (c *Canvas) freeLayers() {
	if f := c.frame; f != nil {
		f.layers.FreeAll()
	}
}
