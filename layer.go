package ggui

import (
	"image"
	"slices"

	"github.com/ironpark/ggfx"
)

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

// borrowLayer returns an image for the next Layer depth that covers area
// of the window's coordinates, the whole window when area is empty. The
// caller clears what it paints into.
//
// An image is only as large as an area asks for, on a grid of
// layerGrid pixels so that one moving a little keeps it, rather than the
// window's size: a fade along a tab bar 60 pixels tall took a layer of the
// whole window, 80 MB on a 6K display. Each depth keeps the images its
// areas took this frame, so layers taken in turn at the same depth over
// different areas, as each pane's tab bar does, do not take and give back
// an image each every frame.
func (f *frameState) borrowLayer(window *ggfx.Image, area image.Rectangle) *ggfx.Image {
	whole := window.Bounds()
	if area.Empty() {
		area = whole
	}
	i := f.layerDepth
	f.layerDepth++
	f.layerPeak = max(f.layerPeak, f.layerDepth)
	for len(f.layers) <= i {
		f.layers = append(f.layers, nil)
	}
	pool := f.layers[i]
	// The smallest that holds the area.
	best := -1
	for j, l := range pool {
		if area.In(l.img.Bounds()) && (best < 0 || pixels(l.img.Bounds()) < pixels(pool[best].img.Bounds())) {
			best = j
		}
	}
	if best >= 0 {
		pool[best].used = true
		return pool[best].img
	}
	b := image.Rect(area.Min.X&^(layerGrid-1), area.Min.Y&^(layerGrid-1),
		(area.Max.X+layerGrid-1)&^(layerGrid-1), (area.Max.Y+layerGrid-1)&^(layerGrid-1)).Intersect(whole)
	if b.Empty() {
		b = area
	}
	// What the new image holds, it replaces.
	pool = slices.DeleteFunc(pool, func(l layerImage) bool {
		if l.img.Bounds().In(b) {
			l.img.Deallocate()
			return true
		}
		return false
	})
	// Unmanaged, the layer is a texture of its own size: on the atlas it
	// would take a page rounded up to powers of two, 4096×2048 for a
	// 3252×1534 window, 32 MB rather than 20.
	img := ggfx.NewImageWithOptions(b, &ggfx.NewImageOptions{Unmanaged: true})
	f.layers[i] = append(pool, layerImage{img: img, used: true})
	return img
}

// layerGrid is the grid in pixels a layer image's bounds are rounded out
// to.
const layerGrid = 64

// layerImage is an image a layer depth keeps, and whether a layer took it
// this frame.
type layerImage struct {
	img  *ggfx.Image
	used bool
}

func pixels(r image.Rectangle) int { return r.Dx() * r.Dy() }

// trimLayers ends a frame for the layer images: it frees those no layer
// took this frame, and the depths past the deepest the frame opened.
func (f *frameState) trimLayers() {
	for i, pool := range f.layers {
		f.layers[i] = slices.DeleteFunc(pool, func(l layerImage) bool {
			if i >= f.layerPeak || !l.used {
				l.img.Deallocate()
				return true
			}
			return false
		})
		for j := range f.layers[i] {
			f.layers[i][j].used = false
		}
	}
	for len(f.layers) > 0 && len(f.layers[len(f.layers)-1]) == 0 {
		f.layers = f.layers[:len(f.layers)-1]
	}
	f.layerPeak = 0
}

// freeLayersAll frees every layer image.
func (f *frameState) freeLayersAll() {
	for _, pool := range f.layers {
		for _, l := range pool {
			l.img.Deallocate()
		}
	}
	f.layers = f.layers[:0]
	f.layerPeak = 0
}

// trimLayers ends a frame for the layer images: it keeps as many as the frame
// had open at once and frees the rest.
func (c *Canvas) trimLayers() {
	if f := c.frame; f != nil {
		f.trimLayers()
	}
}

// freeLayers frees every layer image, for an App or Probe that is closing.
func (c *Canvas) freeLayers() {
	if f := c.frame; f != nil {
		f.freeLayersAll()
	}
}
