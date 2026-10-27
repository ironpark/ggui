package ggui

import (
	"image"
	"image/color"
	"math"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/vector"
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

	// outline is the path as its points, for Contains and Bounds, and
	// flat that outline in straight lines as of flatRev.
	outline []pathOp
	flat    [][]Point
	flatRev uint64
}

// gradientKey is everything besides path geometry that the coverage mask depends on.
type gradientKey struct {
	bounds   Rect
	coverage image.Rectangle
	scale    float64
}

func (p *Path) MoveTo(x, y float64) {
	p.path.MoveTo(float32(x), float32(y))
	p.record(pathMove, Pt(x, y))
}
func (p *Path) LineTo(x, y float64) {
	p.path.LineTo(float32(x), float32(y))
	p.record(pathLine, Pt(x, y))
}

// QuadTo appends a quadratic Bezier segment.
func (p *Path) QuadTo(x1, y1, x2, y2 float64) {
	p.path.QuadTo(float32(x1), float32(y1), float32(x2), float32(y2))
	p.record(pathQuad, Pt(x1, y1), Pt(x2, y2))
}
func (p *Path) CubicTo(x1, y1, x2, y2, x3, y3 float64) {
	p.path.CubicTo(float32(x1), float32(y1), float32(x2), float32(y2), float32(x3), float32(y3))
	p.record(pathCubic, Pt(x1, y1), Pt(x2, y2), Pt(x3, y3))
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

func (p *Path) Close() { p.path.Close(); p.record(pathClose) }
func (p *Path) Reset() { p.path.Reset(); p.outline = p.outline[:0]; p.revision++ }
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
