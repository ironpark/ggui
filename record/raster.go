package record

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

// The frame and the cursor are drawn on the CPU, into image.RGBA, whose
// pixels are premultiplied: they are drawn once (the frame again only
// when the theme changes), they need no renderer, so their geometry is
// tested without one, and they come out the same on every machine.

// roundRectCov returns how much of the pixel centred on (px, py) the
// rectangle from (x0, y0) to (x1, y1), its corners rounded by r, covers:
// 1 inside, 0 outside, a ramp one pixel wide across the edge.
func roundRectCov(px, py, x0, y0, x1, y1, r float64) float32 {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hw, hh := (x1-x0)/2, (y1-y0)/2
	r = max(min(r, hw, hh), 0)
	qx := math.Abs(px-cx) - hw + r
	qy := math.Abs(py-cy) - hh + r
	d := math.Hypot(max(qx, 0), max(qy, 0)) + min(max(qx, qy), 0) - r
	return float32(clamp01(0.5 - d))
}

// circleCov returns how much of the pixel centred on (px, py) a circle
// covers.
func circleCov(px, py, cx, cy, r float64) float32 {
	return float32(clamp01(r - math.Hypot(px-cx, py-cy) + 0.5))
}

func clamp01(v float64) float64 { return min(max(v, 0), 1) }

// blend draws c, not premultiplied, over the pixel at i of dst with
// coverage a.
func blend(dst []uint8, i int, c color.NRGBA, a float32) {
	a *= float32(c.A) / 255
	if a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	k := 1 - a
	dst[i] = uint8(float32(c.R)*a + float32(dst[i])*k + 0.5)
	dst[i+1] = uint8(float32(c.G)*a + float32(dst[i+1])*k + 0.5)
	dst[i+2] = uint8(float32(c.B)*a + float32(dst[i+2])*k + 0.5)
	dst[i+3] = uint8(255*a + float32(dst[i+3])*k + 0.5)
}

// over draws the premultiplied pixel s over the one at i of dst.
func over(dst []uint8, i int, s []uint8) {
	sa := uint32(s[3])
	if sa == 0 {
		return
	}
	if sa == 255 {
		copy(dst[i:i+4], s[:4])
		return
	}
	k := 255 - sa
	for j := range 4 {
		dst[i+j] = uint8(uint32(s[j]) + (uint32(dst[i+j])*k+127)/255)
	}
}

// drawOver draws src over dst with src's top-left corner at at, clipped
// to dst.
func drawOver(dst, src *image.RGBA, at image.Point) {
	r := src.Rect.Add(at.Sub(src.Rect.Min)).Intersect(dst.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			so := src.PixOffset(x-at.X+src.Rect.Min.X, y-at.Y+src.Rect.Min.Y)
			over(dst.Pix, dst.PixOffset(x, y), src.Pix[so:so+4])
		}
	}
}

// gaussBlur blurs the w×h mask in place, about as a Gaussian of
// standard deviation sigma pixels does, by three box blurs each way;
// outside the mask is 0.
func gaussBlur(m []float32, w, h int, sigma float64) {
	if sigma < 0.5 {
		return
	}
	// The widths of three boxes whose sum of variances is sigma².
	ideal := math.Sqrt(12*sigma*sigma/3 + 1)
	wl := int(ideal)
	if wl%2 == 0 {
		wl--
	}
	wu := wl + 2
	mi := math.Round((12*sigma*sigma - 3*float64(wl*wl) - 12*float64(wl) - 9) / (-4*float64(wl) - 4))
	tmp := make([]float32, max(w, h))
	for i := range 3 {
		r := (wu - 1) / 2
		if float64(i) < mi {
			r = (wl - 1) / 2
		}
		if r < 1 {
			continue
		}
		for y := range h {
			boxLine(m[y*w:(y+1)*w], 1, w, r, tmp)
		}
		for x := range w {
			boxLine(m[x:], w, h, r, tmp)
		}
	}
}

// boxLine averages each of the n values of line, stride apart, with the
// r on either side of it.
func boxLine(line []float32, stride, n, r int, tmp []float32) {
	for i := range n {
		tmp[i] = line[i*stride]
	}
	inv := 1 / float32(2*r+1)
	var sum float32
	for i := 0; i < r && i < n; i++ {
		sum += tmp[i]
	}
	for i := range n {
		if j := i + r; j < n {
			sum += tmp[j]
		}
		if j := i - r - 1; j >= 0 {
			sum -= tmp[j]
		}
		line[i*stride] = max(sum*inv, 0)
	}
}

// pt is a point in device pixels.
type pt struct{ x, y float64 }

// fillPolys fills each polygon into dst, a coverage mask, at full
// opacity, antialiased; where they overlap they unite.
func fillPolys(dst *image.Alpha, polys ...[]pt) {
	b := dst.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	for _, p := range polys {
		if len(p) < 3 {
			continue
		}
		z.Reset(b.Dx(), b.Dy())
		z.DrawOp = draw.Over
		z.MoveTo(float32(p[0].x), float32(p[0].y))
		for _, q := range p[1:] {
			z.LineTo(float32(q.x), float32(q.y))
		}
		z.ClosePath()
		z.Draw(dst, b, image.Opaque, image.Point{})
	}
}

// roundRectPoly returns the outline of a rounded rectangle as a polygon,
// each corner a quarter circle of r.
func roundRectPoly(x0, y0, x1, y1, r float64) []pt {
	r = max(min(r, (x1-x0)/2, (y1-y0)/2), 0)
	var out []pt
	corner := func(cx, cy, from float64) {
		const steps = 8
		for i := 0; i <= steps; i++ {
			a := from + float64(i)/steps*math.Pi/2
			out = append(out, pt{cx + r*math.Cos(a), cy + r*math.Sin(a)})
		}
	}
	corner(x1-r, y0+r, -math.Pi/2)
	corner(x1-r, y1-r, 0)
	corner(x0+r, y1-r, math.Pi/2)
	corner(x0+r, y0+r, math.Pi)
	return out
}

// capsulePoly returns the outline of a line from a to b with round ends,
// r from it.
func capsulePoly(a, b pt, r float64) []pt {
	ang := math.Atan2(b.y-a.y, b.x-a.x)
	var out []pt
	const steps = 12
	for i := 0; i <= steps; i++ {
		t := ang - math.Pi/2 + float64(i)/steps*math.Pi
		out = append(out, pt{b.x + r*math.Cos(t), b.y + r*math.Sin(t)})
	}
	for i := 0; i <= steps; i++ {
		t := ang + math.Pi/2 + float64(i)/steps*math.Pi
		out = append(out, pt{a.x + r*math.Cos(t), a.y + r*math.Sin(t)})
	}
	return out
}

// offsetPoly moves each edge of the simple polygon p outward by d, the
// corners mitred (a miter longer than 3d is cut to it).
func offsetPoly(p []pt, d float64) []pt {
	n := len(p)
	area := 0.0
	for i := range n {
		q, r := p[i], p[(i+1)%n]
		area += q.x*r.y - r.x*q.y
	}
	s := 1.0
	if area < 0 {
		s = -1
	}
	normal := func(a, b pt) pt {
		dx, dy := b.x-a.x, b.y-a.y
		l := math.Hypot(dx, dy)
		return pt{s * dy / l, -s * dx / l}
	}
	out := make([]pt, n)
	for i := range n {
		prev, cur, next := p[(i+n-1)%n], p[i], p[(i+1)%n]
		n1, n2 := normal(prev, cur), normal(cur, next)
		k := 1 + n1.x*n2.x + n1.y*n2.y
		m := pt{n1.x + n2.x, n1.y + n2.y}
		f := d / max(k, 2.0/9) // a miter of at most 3d
		out[i] = pt{cur.x + m.x*f, cur.y + m.y*f}
	}
	return out
}
