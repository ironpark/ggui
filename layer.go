package ggui

import "github.com/ironpark/ggfx"

// Layer paints into an offscreen image the size of the window, then draws
// that image onto c with op: the way to apply an opacity or a transform to a
// subtree as a whole, which a Transition's fade and scale do. paint gets a
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
func (c *Canvas) Layer(op *ggfx.DrawImageOptions, paint func(layer *Canvas)) {
	if c == nil || c.Image == nil || c.root().Image == nil {
		paint(c)
		return
	}
	f := c.fs()
	img := f.borrowLayer(c.root().Image)
	defer func() { f.layerDepth-- }()
	layer := *c
	layer.parent, layer.frame, layer.Image = c, nil, img
	paint(&layer)
	c.Image.DrawImage(img, op)
}

// borrowLayer returns the cleared image for the next Layer depth, covering
// the window image's coordinates.
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
	img := f.layers[i]
	if img != nil && img.Bounds().Max == size {
		img.Clear()
		return img
	}
	if img != nil {
		img.Deallocate()
	}
	img = ggfx.NewImage(size.X, size.Y)
	f.layers[i] = img
	return img
}

// trimLayers ends a frame for the layer images: it keeps as many as the frame
// had open at once and frees the rest.
func (c *Canvas) trimLayers() {
	if c.frame == nil {
		return
	}
	f := c.frame
	for i, img := range f.layers[f.layerPeak:] {
		img.Deallocate()
		f.layers[f.layerPeak+i] = nil
	}
	f.layers = f.layers[:f.layerPeak]
	f.layerPeak = 0
}

// freeLayers frees every layer image, for an App or Probe that is closing.
func (c *Canvas) freeLayers() {
	if c.frame != nil {
		c.frame.layerPeak = 0
	}
	c.trimLayers()
}
