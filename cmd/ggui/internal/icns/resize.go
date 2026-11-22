package icns

import (
	"image"
	"image/draw"
	"math"
)

// Resizer scales one source image to multiple square sizes, precomputing its
// premultiplied linear-light pixel buffer once.
type Resizer struct {
	w, h int
	lin  []float64
}

// NewResizer prepares a Resizer for src, converting it to premultiplied linear
// RGB and alpha once.
func NewResizer(src image.Image) *Resizer {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	rgba, ok := src.(*image.NRGBA)
	if !ok || b.Min != (image.Point{}) || rgba.Stride != w*4 {
		rgba = image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)
	}
	// Premultiplied linear RGB and alpha, one float per channel.
	lin := make([]float64, w*h*4)
	for i := 0; i < w*h; i++ {
		px := rgba.Pix[i*4 : i*4+4]
		a := float64(px[3]) / 255
		lin[i*4+0] = toLinear[px[0]] * a
		lin[i*4+1] = toLinear[px[1]] * a
		lin[i*4+2] = toLinear[px[2]] * a
		lin[i*4+3] = a
	}
	return &Resizer{w: w, h: h, lin: lin}
}

// Resize scales r's source image to size×size by area averaging in linear light.
func (r *Resizer) Resize(size int) *image.NRGBA {
	w, h, lin := r.w, r.h, r.lin

	// Separable: rows first, then columns.
	xw := weights(w, size)
	yw := weights(h, size)
	tmp := make([]float64, size*h*4)
	for y := range h {
		for x := range size {
			var acc [4]float64
			for _, c := range xw[x] {
				s := (y*w + c.i) * 4
				for k := range acc {
					acc[k] += lin[s+k] * c.w
				}
			}
			copy(tmp[(y*size+x)*4:], acc[:])
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			var acc [4]float64
			for _, c := range yw[y] {
				s := (c.i*size + x) * 4
				for k := range acc {
					acc[k] += tmp[s+k] * c.w
				}
			}
			a := acc[3]
			o := dst.PixOffset(x, y)
			if a <= 0 {
				dst.Pix[o+0], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = 0, 0, 0, 0
				continue
			}
			dst.Pix[o+0] = fromLinear(acc[0] / a)
			dst.Pix[o+1] = fromLinear(acc[1] / a)
			dst.Pix[o+2] = fromLinear(acc[2] / a)
			dst.Pix[o+3] = clamp8(a * 255)
		}
	}
	return dst
}

// Resize scales src to size×size by area averaging: each destination
// pixel is the mean of the source area it covers, partial pixels at its
// edges weighted by how much of them it covers.
//
// For shrinking that is the filter to want. A 1024-pixel icon becomes a
// 16-pixel one by a factor of 64, and a filter with a fixed small support,
// bilinear or even Lanczos, would sample a few source pixels per output
// pixel and alias thin strokes into noise; averaging the whole footprint
// cannot skip any. It is done in linear light, so a white stroke on black
// keeps its weight instead of darkening as gamma-encoded averaging would
// make it, and on premultiplied alpha, so a transparent pixel's color,
// which is meaningless, does not bleed into the edge of the shape beside
// it.
func Resize(src image.Image, size int) *image.NRGBA {
	return NewResizer(src).Resize(size)
}

type contrib struct {
	i int
	w float64
}

// weights returns, for each of n destination pixels along an axis of m
// source pixels, the source pixels it covers and how much of each, the
// weights summing to one. A destination larger than the source, which
// Encode never asks for, comes out as nearest-neighbor.
func weights(m, n int) [][]contrib {
	out := make([][]contrib, n)
	scale := float64(m) / float64(n)
	for d := range n {
		lo, hi := float64(d)*scale, float64(d+1)*scale
		var cs []contrib
		for s := int(math.Floor(lo)); s < int(math.Ceil(hi)) && s < m; s++ {
			cover := math.Min(hi, float64(s+1)) - math.Max(lo, float64(s))
			if cover > 0 {
				cs = append(cs, contrib{s, cover / scale})
			}
		}
		out[d] = cs
	}
	return out
}

// toLinear maps an sRGB byte to linear light.
var toLinear = func() (t [256]float64) {
	for i := range t {
		c := float64(i) / 255
		if c <= 0.04045 {
			t[i] = c / 12.92
		} else {
			t[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return t
}()

// fromLinear maps linear light back to an sRGB byte.
func fromLinear(v float64) uint8 {
	if v <= 0.0031308 {
		return clamp8(v * 12.92 * 255)
	}
	return clamp8((1.055*math.Pow(v, 1/2.4) - 0.055) * 255)
}

func clamp8(v float64) uint8 {
	v = math.Round(v)
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return uint8(v)
}
