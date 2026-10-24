package ggui

import (
	"image"
	"image/color"

	"github.com/ironpark/ggfx"
)

// ClipPath is Clip to the inside of path, antialiased, for a subtree: paint
// draws into a Layer covering the path's bounds, which is composited through
// the path filled as FillPath fills it. Input is clipped to the same shape,
// so a pointer outside the path does not reach what paint registers there.
// A nil path draws nothing.
func (c *Canvas) ClipPath(path *Path, paint func(dst *Canvas)) {
	if path == nil {
		return
	}
	polys := path.polygons()
	c.clipShape(path.Bounds(), func(p Point) bool { return polygonsContain(polys, p) }, func(mask *Canvas) {
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
	img := f.borrowLayer(c.root().Image)
	defer func() { f.layerDepth-- }()
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
