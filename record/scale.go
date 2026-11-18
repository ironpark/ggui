package record

import (
	"image"
	"math"
)

// scaler shrinks frames of one size to another with an area
// filter: each pixel out is the average of the source area it covers,
// partly covered pixels in proportion. It is what keeps thin strokes and
// text legible when a frame shrinks by a fraction, where sampling would
// drop or double whole columns. It reuses its buffers frame to frame.
type scaler struct {
	sw, sh, dw, dh int
	xs, ys         []taps
	row            []float32 // a source row filtered across, dw×4
	acc            []float32 // the output row being summed, dw×4
	dst            *image.RGBA
}

// taps are the source pixels one output pixel covers, from first, with
// their weights, which sum to 1.
type taps struct {
	first int
	w     []float32
}

// newScaler returns a scaler from sw×sh to width dw, the height in
// proportion, or nil when dw keeps the size, as zero or dw ≥ sw does.
func newScaler(sw, sh, dw int) *scaler {
	if dw <= 0 || dw >= sw {
		return nil
	}
	dh := max(int(math.Round(float64(sh)*float64(dw)/float64(sw))), 1)
	return &scaler{
		sw: sw, sh: sh, dw: dw, dh: dh,
		xs:  areaTaps(sw, dw),
		ys:  areaTaps(sh, dh),
		row: make([]float32, dw*4),
		acc: make([]float32, dw*4),
		dst: image.NewRGBA(image.Rect(0, 0, dw, dh)),
	}
}

// areaTaps returns, for each of the n pixels out, the source pixels of
// the src it covers and how much of each.
func areaTaps(src, n int) []taps {
	out := make([]taps, n)
	k := float64(src) / float64(n) // source pixels per pixel out
	for i := range out {
		lo, hi := float64(i)*k, float64(i+1)*k
		first := int(lo)
		last := min(int(math.Ceil(hi)), src) - 1
		w := make([]float32, last-first+1)
		for j := first; j <= last; j++ {
			cover := math.Min(hi, float64(j+1)) - math.Max(lo, float64(j))
			w[j-first] = float32(cover / k)
		}
		out[i] = taps{first: first, w: w}
	}
	return out
}

// scale returns src scaled; the image is the scaler's, overwritten by the
// next call.
func (s *scaler) scale(src *image.RGBA) *image.RGBA {
	for oy, ty := range s.ys {
		clear(s.acc)
		for j, wy := range ty.w {
			y := ty.first + j
			line := src.Pix[y*src.Stride : y*src.Stride+s.sw*4]
			for ox, tx := range s.xs {
				var r, g, b, a float32
				p := tx.first * 4
				for _, wx := range tx.w {
					r += wx * float32(line[p])
					g += wx * float32(line[p+1])
					b += wx * float32(line[p+2])
					a += wx * float32(line[p+3])
					p += 4
				}
				s.row[ox*4], s.row[ox*4+1], s.row[ox*4+2], s.row[ox*4+3] = r, g, b, a
			}
			for i, v := range s.row {
				s.acc[i] += wy * v
			}
		}
		out := s.dst.Pix[oy*s.dst.Stride:]
		for ox := range s.dw {
			for j := range 4 {
				out[ox*4+j] = clamp8(s.acc[ox*4+j])
			}
		}
	}
	return s.dst
}

func clamp8(v float32) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	}
	return uint8(v + 0.5)
}
