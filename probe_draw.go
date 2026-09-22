package ggui

import "github.com/ironpark/ggfx"

// Draw runs a probe frame into an image, including overlays and retained
// animation state. The caller clears the image and sizes the probe in logical
// pixels with Resize; the image is painted at one device pixel per logical pixel.
// Call from an Ebitengine Draw callback, where GPU drawing is supported.
func (p *Probe) Draw(image *ggfx.Image) Size {
	p.canvas.Image = image
	defer func() { p.canvas.Image = nil }()
	return p.Frame()
}
