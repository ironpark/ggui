package ggui

import (
	"image"
	"image/color"
	"math"

	"github.com/ironpark/ggfx"
)

// FadeEdges paints paint into a layer over r and draws it with its ends
// along an axis faded out: the first lead logical pixels from clear to
// opaque, and the last trail from opaque to clear, across x when
// horizontal and across y otherwise. What is under r shows through the
// faded ends, as it would through the content's own transparency.
func (c *Canvas) FadeEdges(r Rect, horizontal bool, lead, trail float64, paint func(dst *Canvas)) {
	if lead <= 0 && trail <= 0 || c == nil || c.Image == nil || c.Image.Bounds().Empty() {
		paint(c)
		return
	}
	c.layer(c.Image.Bounds(), paint, func(img *ggfx.Image) {
		ramp := fadeRamp()
		// A ramp of fadeRampLength pixels, clear to opaque, stretched over
		// an end and turned to face into r.
		end := func(from, length float64, flip bool) {
			if length <= 0 {
				return
			}
			op := &ggfx.DrawImageOptions{Blend: ggfx.BlendDestinationIn, Filter: ggfx.FilterLinear}
			along, across := c.px(length)/fadeRampLength, c.px(pick(horizontal, r.Size.H, r.Size.W))
			if flip {
				along = -along
			}
			op.GeoM.Scale(along, across)
			if horizontal {
				op.GeoM.Translate(c.px(from), c.px(r.Origin.Y))
			} else {
				// The ramp runs across x; turned a quarter clockwise, it
				// runs down y, from r's right edge back over its width.
				op.GeoM.Rotate(math.Pi / 2)
				op.GeoM.Translate(c.px(r.Origin.X+r.Size.W), c.px(from))
			}
			img.DrawImage(ramp, op)
		}
		start, stop := pick(horizontal, r.Origin.X, r.Origin.Y), pick(horizontal, r.Origin.X+r.Size.W, r.Origin.Y+r.Size.H)
		end(start, min(lead, stop-start), false)
		end(stop, min(trail, stop-start), true)
		op := &ggfx.DrawImageOptions{}
		op.GeoM.Translate(float64(img.Bounds().Min.X), float64(img.Bounds().Min.Y))
		c.Image.DrawImage(img, op)
	})
}

const fadeRampLength = 64

// rampImage is the ramp FadeEdges masks with: a row of white from clear to
// opaque, made once.
var rampImage *ggfx.Image

func fadeRamp() *ggfx.Image {
	if rampImage == nil {
		src := image.NewRGBA(image.Rect(0, 0, fadeRampLength, 1))
		for x := range fadeRampLength {
			a := uint8((float64(x) + .5) / fadeRampLength * 255)
			src.SetRGBA(x, 0, color.RGBA{a, a, a, a})
		}
		rampImage = ggfx.NewImageFromImage(src)
	}
	return rampImage
}
