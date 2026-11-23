package ggui

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"sync/atomic"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/vector"
	"github.com/ironpark/ggui/geom"
	"github.com/ironpark/ggui/internal/framestats"
	"github.com/ironpark/ggui/internal/render"
)

// Path is a reusable vector outline in logical pixels. Mutating a path invalidates
// its device-scale cache. Like widgets, paths belong to the UI thread.
type Path struct {
	// gradientMask is the part of maskStore the coverage takes; maskBytes
	// counts maskStore's bytes for the PathMasks gauge, apart from the Path
	// so the cleanup that counts them gone does not keep it alive.
	gradientMask *ggfx.Image
	maskStore    *ggfx.Image
	maskBytes    *atomic.Int64
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

// RoundRect adds r with its corners rounded by radius as a closed subpath,
// running clockwise on screen from where the top-left corner meets the top
// edge. The
// radius is clamped to half the shorter side, so a large one makes a
// pill; zero or less leaves the corners square.
func (p *Path) RoundRect(r Rect, radius float64) {
	x0, y0 := r.Origin.X, r.Origin.Y
	x1, y1 := x0+r.Size.W, y0+r.Size.H
	rad := max(min(radius, r.Size.W/2, r.Size.H/2), 0)
	if rad == 0 {
		p.MoveTo(x0, y0)
		p.LineTo(x1, y0)
		p.LineTo(x1, y1)
		p.LineTo(x0, y1)
		p.Close()
		return
	}
	const k = 0.5522847498307936 // 4/3·(√2−1), a quarter circle's handle
	h := rad * (1 - k)
	p.MoveTo(x0+rad, y0)
	p.LineTo(x1-rad, y0)
	p.CubicTo(x1-h, y0, x1, y0+h, x1, y0+rad)
	p.LineTo(x1, y1-rad)
	p.CubicTo(x1, y1-h, x1-h, y1, x1-rad, y1)
	p.LineTo(x0+rad, y1)
	p.CubicTo(x0+h, y1, x0, y1-h, x0, y1-rad)
	p.LineTo(x0, y0+rad)
	p.CubicTo(x0, y0+h, x0+h, y0, x0+rad, y0)
	p.Close()
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
	c.drewPath(path, 0)
	vector.FillPath(c.Image, path.device(c.Scale()), nil, pathOptions(col))
}

// StrokePath strokes an outline with round joins and a logical pixel width.
func (c *Canvas) StrokePath(path *Path, width float64, col color.Color) {
	if c == nil || c.Image == nil || path == nil || col == nil || width <= 0 {
		return
	}
	c.drewPath(path, width/2)
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
	whole := device.Bounds()
	coverage := whole.Intersect(c.physical(bounds))
	w, h := coverage.Dx(), coverage.Dy()
	if w <= 0 || h <= 0 {
		return nil, image.Point{}, false
	}
	key := gradientKey{bounds: bounds, coverage: coverage, scale: scale}
	if p.gradientRev != p.revision || p.gradientKey != key || p.gradientMask == nil {
		// Where bounds cut the path short on the right or below, the mask
		// is the coverage exactly, so the path is cut there too; else it
		// may run past the coverage, on maskSide's grid, clear beyond it.
		cut := whole.Max.X > coverage.Max.X || whole.Max.Y > coverage.Max.Y
		p.gradientMask = p.maskFor(w, h, cut)
		p.gradientMask.Clear()
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

// maskFormat is the pixel format of a path's gradient mask.
const maskFormat = ggfx.PixelFormatAlpha

// maskFor returns the top-left part of the mask the path keeps for a w×h
// coverage: w×h when exact, else its sides rounded up by maskSide, so that
// a coverage changing size by a little every frame, as an animated one
// does, takes the same part rather than a sub-image of its own each frame.
// The mask is kept while the coverage fits it and takes at least a quarter
// of it; past that a new one is made, its sides rounded up a little so the
// next frame's growth fits too.
func (p *Path) maskFor(w, h int, exact bool) *ggfx.Image {
	if s := p.maskStore; s != nil {
		if b := s.Bounds(); w > b.Dx() || h > b.Dy() || 4*w*h < b.Dx()*b.Dy() {
			p.freeMask()
		}
	}
	if p.maskStore == nil {
		// A large mask is a texture of its own size, as a large image is.
		// It holds white coverage only, so it is alpha-only, a byte a pixel.
		p.maskStore = render.NewBlankOf(maskSide(w), maskSide(h), maskFormat)
		if p.maskBytes == nil {
			// A Path the app drops takes its mask with it: ggfx frees the
			// texture once it is collected, and this counts its bytes gone.
			p.maskBytes = new(atomic.Int64)
			runtime.AddCleanup(p, func(n *atomic.Int64) {
				framestats.AddTexture(framestats.PathMasks, -n.Swap(0))
			}, p.maskBytes)
		}
		b := p.maskStore.Bounds()
		p.maskBytes.Store(framestats.ImageBytesOf(b.Dx(), b.Dy(), maskFormat))
		framestats.KeptOf(framestats.PathMasks, b, maskFormat)
	}
	if !exact {
		w, h = maskSide(w), maskSide(h)
	}
	if g := p.gradientMask; g != nil && g.Bounds() == image.Rect(0, 0, w, h) {
		return g
	}
	return maskCorner(p.maskStore, w, h)
}

// maskCorner is the w×h part of img at its corner, img itself when that is
// all of it.
func maskCorner(img *ggfx.Image, w, h int) *ggfx.Image {
	if b := img.Bounds(); b.Dx() == w && b.Dy() == h {
		return img
	}
	return img.SubImage(image.Rect(0, 0, w, h)).(*ggfx.Image)
}

// maskSide rounds a coverage mask's side up: to 16 pixels for one on the
// atlas, to render.LayerGrid for a texture of its own.
func maskSide(n int) int {
	grid := 16
	if n > render.LargeImage {
		grid = render.LayerGrid
	}
	return (n + grid - 1) &^ (grid - 1)
}

// freeMask frees the path's coverage mask.
func (p *Path) freeMask() {
	if p.maskStore == nil {
		return
	}
	p.maskStore.Deallocate()
	p.maskStore, p.gradientMask = nil, nil
	framestats.AddTexture(framestats.PathMasks, -p.maskBytes.Swap(0))
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
