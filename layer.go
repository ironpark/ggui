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
	layer := c.derive()
	layer.Image = img
	paint(layer)
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
	if img := f.layers[i]; img != nil {
		if img.Bounds().Max == size {
			img.Clear()
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
