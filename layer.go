package ggui

import (
	"image"

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
	c.layer(image.Rectangle{}, paint, func(img *ggfx.Image) {
		op := &ggfx.DrawImageOptions{}
		if o.Scale != 0 && o.Scale != 1 {
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
	var window *ggfx.Image
	if c != nil {
		window = c.root().Image
	}
	if c == nil || c.Image == nil || window == nil {
		paint(c)
		return
	}
	f := c.fs()
	img := f.borrowLayer(window)
	defer func() { f.layerDepth-- }()
	if !area.Empty() {
		img = img.SubImage(area).(*ggfx.Image)
	}
	img.Clear()
	layer := c.derive()
	layer.Image = img
	paint(layer)
	draw(img)
}

// borrowLayer returns the image for the next Layer depth, covering the
// window image's coordinates. The caller clears what it paints into.
func (f *frameState) borrowLayer(window *ggfx.Image) *ggfx.Image {
	// Painting keeps the window's coordinates, so the layer covers them
	// from the origin, which is the whole window when it starts there.
	size := window.Bounds().Max
	i := f.layerDepth
	f.layerDepth++
	f.layerPeak = max(f.layerPeak, f.layerDepth)
	if i == len(f.layers) {
		f.layers = append(f.layers, nil)
	}
	if img := f.layers[i]; img != nil {
		if img.Bounds().Max == size {
			return img
		}
		img.Deallocate()
	}
	f.layers[i] = ggfx.NewImage(size.X, size.Y)
	return f.layers[i]
}

// trimLayers keeps the first keep layer images, frees the rest, and starts
// counting the next frame's peak.
func (f *frameState) trimLayers(keep int) {
	for _, img := range f.layers[keep:] {
		img.Deallocate()
	}
	clear(f.layers[keep:])
	f.layers = f.layers[:keep]
	f.layerPeak = 0
}

// trimLayers ends a frame for the layer images: it keeps as many as the frame
// had open at once and frees the rest.
func (c *Canvas) trimLayers() {
	if f := c.frame; f != nil {
		f.trimLayers(f.layerPeak)
	}
}

// freeLayers frees every layer image, for an App or Probe that is closing.
func (c *Canvas) freeLayers() {
	if f := c.frame; f != nil {
		f.trimLayers(0)
	}
}
