package record

import (
	"image"
	"image/color"
	"slices"
)

// The palette is chosen once for a whole recording, from a histogram of
// every pixel that changed: a GIF frame can carry a palette of its own,
// but one shared palette keeps the colors of what stays on screen from
// shifting between frames, and costs nothing per frame.

// histogram counts colors: each exactly, up to maxExact of them, which UI
// seldom passes, and past that in bins of histBits a channel, each bin
// summing the colors counted in it so that its color is their mean.
type histogram struct {
	exact map[uint32]uint32 // count by color, until there are too many
	bins  *bins
}

// maxExact is how many colors a histogram counts one by one.
const maxExact = 1 << 17

// histBits is the precision a color is binned at, per channel.
const histBits = 6

const histSize = 1 << (3 * histBits)

type bins struct {
	n   [histSize]uint32
	sum [histSize][3]uint64
}

func (b *bins) add(r, g, bl uint8, n uint32) {
	const s = 8 - histBits
	i := int(r>>s)<<(2*histBits) | int(g>>s)<<histBits | int(bl>>s)
	if b.n[i] > 1<<31 {
		return
	}
	b.n[i] += n
	sum := &b.sum[i]
	sum[0] += uint64(r) * uint64(n)
	sum[1] += uint64(g) * uint64(n)
	sum[2] += uint64(bl) * uint64(n)
}

// add counts the pixels of img in rect.
func (h *histogram) add(img *image.RGBA, rect image.Rectangle) {
	if h.exact == nil && h.bins == nil {
		h.exact = map[uint32]uint32{}
	}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		p := img.PixOffset(rect.Min.X, y)
		for range rect.Dx() {
			r, g, b := img.Pix[p], img.Pix[p+1], img.Pix[p+2]
			p += 4
			if h.bins != nil {
				h.bins.add(r, g, b, 1)
				continue
			}
			k := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
			if n := h.exact[k]; n < 1<<31 {
				h.exact[k] = n + 1
			}
			if len(h.exact) > maxExact {
				h.bins = new(bins)
				for k, n := range h.exact {
					h.bins.add(uint8(k>>16), uint8(k>>8), uint8(k), n)
				}
				h.exact = nil
			}
		}
	}
}

// entries returns the colors counted, binned or not.
func (h *histogram) entries() []entry {
	var out []entry
	if h.bins == nil {
		for k, n := range h.exact {
			out = append(out, entry{n: float64(n), c: [3]float64{float64(k >> 16), float64(k >> 8 & 0xff), float64(k & 0xff)}})
		}
		// Map order is random; the palette must not be.
		slices.SortFunc(out, func(a, b entry) int { return cmpColor(a.c, b.c) })
		return out
	}
	for i := range h.bins.n {
		if n := h.bins.n[i]; n > 0 {
			f := float64(n)
			s := h.bins.sum[i]
			out = append(out, entry{n: f, c: [3]float64{float64(s[0]) / f, float64(s[1]) / f, float64(s[2]) / f}})
		}
	}
	return out
}

func cmpColor(a, b [3]float64) int {
	for c := range 3 {
		if a[c] != b[c] {
			if a[c] < b[c] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// entry is a color counted, or a bin: how many pixels it counted and
// their mean, c, in sRGB until palette works in Oklab, and rgb, the
// sRGB.
type entry struct {
	n   float64
	c   [3]float64
	rgb [3]float64
}

// The palette is chosen and colors mapped to it in Oklab, in which a
// distance is about as visible wherever it is: in sRGB the steps of a
// dark gradient, or of a shadow, are closer together to the eye than
// those of a light one, and a palette chosen there spends too few colors
// on them.
func dist(a, b [3]float64) float64 {
	d0, d1, d2 := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return d0*d0 + d1*d1 + d2*d2
}

// anchorShare is the share of the pixels counted a color needs to keep
// a palette entry of its own: the flat colors of a UI, its backgrounds,
// a frame's title bar and buttons, are kept exactly, rather than
// averaged with the antialiasing and gradients around them.
const anchorShare = 1.0 / 2000

// palette chooses at most k colors for what h counted. A recording with
// no more than k colors keeps them all, exactly. Otherwise the colors
// that cover anchorShare of the pixels or more, up to a quarter of k, are
// kept exactly, and the rest are chosen in Oklab: a median cut that
// splits, again and again, the box of colors whose split lowers the
// squared error most, where it lowers it most, and then a few rounds of
// k-means to settle the colors on the pixels they stand for, the exact
// ones staying put.
func (h *histogram) palette(k int) color.Palette {
	entries := h.entries()
	if len(entries) == 0 {
		return color.Palette{color.RGBA{A: 0xff}}
	}
	if len(entries) <= k {
		p := make(color.Palette, len(entries))
		for i, e := range entries {
			p[i] = color.RGBA{R: round8(e.c[0]), G: round8(e.c[1]), B: round8(e.c[2]), A: 0xff}
		}
		return p
	}
	var total float64
	for i := range entries {
		e := &entries[i]
		total += e.n
		e.rgb = e.c
		e.c = oklab(e.c[0], e.c[1], e.c[2])
	}
	byCount := slices.Clone(entries)
	slices.SortStableFunc(byCount, func(a, b entry) int {
		switch {
		case a.n > b.n:
			return -1
		case a.n < b.n:
			return 1
		}
		return cmpColor(a.rgb, b.rgb)
	})
	var anchors []entry
	for _, e := range byCount {
		if e.n < total*anchorShare || len(anchors) >= k/4 {
			break
		}
		anchors = append(anchors, e)
	}
	boxes := []box{newBox(entries)}
	for len(boxes) < k-len(anchors) {
		best, gain := -1, 0.0
		for i := range boxes {
			if g := boxes[i].gain(); g > gain {
				best, gain = i, g
			}
		}
		if best < 0 {
			break
		}
		a, b := boxes[best].split()
		boxes[best] = a
		boxes = append(boxes, b)
	}
	centers := make([][3]float64, 0, len(anchors)+len(boxes))
	for _, a := range anchors {
		centers = append(centers, a.c)
	}
	for _, b := range boxes {
		centers = append(centers, b.mean())
	}
	refine(entries, centers, len(anchors), 4)

	p := make(color.Palette, 0, len(centers))
	seen := map[color.RGBA]bool{}
	for i, c := range centers {
		rgb := fromOklab(c)
		if i < len(anchors) {
			rgb = anchors[i].rgb
		}
		rgba := color.RGBA{R: round8(rgb[0]), G: round8(rgb[1]), B: round8(rgb[2]), A: 0xff}
		if !seen[rgba] {
			seen[rgba] = true
			p = append(p, rgba)
		}
	}
	return p
}

func round8(v float64) uint8 { return clamp8(float32(v)) }

// box is a set of entries the median cut splits: the best split along
// its widest axis is found when it is made.
type box struct {
	e          []entry
	n          float64
	sum        [3]float64
	cut        int     // where the best split falls, in e sorted along axis
	axis       int     // the axis e is sorted along
	splitError float64 // the squared error of the two halves of that split
	err        float64 // the squared error of the box whole
}

func newBox(e []entry) box {
	b := box{e: e}
	var sq float64
	for _, x := range e {
		b.n += x.n
		for c := range 3 {
			b.sum[c] += x.n * x.c[c]
			sq += x.n * x.c[c] * x.c[c]
		}
	}
	b.err = sq - sqNorm(b.sum)/b.n
	b.cut = -1
	if len(e) < 2 {
		return b
	}
	// The axis along which the colors spread most.
	var lo, hi [3]float64
	lo, hi = e[0].c, e[0].c
	for _, x := range e {
		for c := range 3 {
			lo[c] = min(lo[c], x.c[c])
			hi[c] = max(hi[c], x.c[c])
		}
	}
	for c := range 3 {
		if hi[c]-lo[c] > hi[b.axis]-lo[b.axis] {
			b.axis = c
		}
	}
	slices.SortFunc(b.e, func(x, y entry) int {
		switch {
		case x.c[b.axis] < y.c[b.axis]:
			return -1
		case x.c[b.axis] > y.c[b.axis]:
			return 1
		}
		return 0
	})
	// The cut that leaves the least squared error on its two sides.
	var n float64
	var sum [3]float64
	best := -1.0
	for i := range len(b.e) - 1 {
		x := b.e[i]
		n += x.n
		for c := range 3 {
			sum[c] += x.n * x.c[c]
		}
		var rest [3]float64
		for c := range 3 {
			rest[c] = b.sum[c] - sum[c]
		}
		e := sq - sqNorm(sum)/n - sqNorm(rest)/(b.n-n)
		if best < 0 || e < best {
			best, b.cut = e, i+1
		}
	}
	b.splitError = best
	return b
}

// sqNorm is the squared length of a sum of colors.
func sqNorm(s [3]float64) float64 {
	return s[0]*s[0] + s[1]*s[1] + s[2]*s[2]
}

// gain is how much splitting the box lowers the error, 0 when it cannot
// be split.
func (b *box) gain() float64 {
	if b.cut < 0 {
		return 0
	}
	return b.err - b.splitError
}

func (b *box) split() (box, box) {
	return newBox(b.e[:b.cut:b.cut]), newBox(b.e[b.cut:])
}

func (b *box) mean() [3]float64 {
	return [3]float64{b.sum[0] / b.n, b.sum[1] / b.n, b.sum[2] / b.n}
}

// refine moves each center to the mean of the entries nearest it, rounds
// times: Lloyd's k-means, seeded by the median cut. The first fixed
// centers stay where they are.
func refine(entries []entry, centers [][3]float64, fixed, rounds int) {
	n := make([]float64, len(centers))
	sum := make([][3]float64, len(centers))
	for range rounds {
		clear(n)
		clear(sum)
		for _, e := range entries {
			i := nearest(centers, e.c)
			n[i] += e.n
			for c := range 3 {
				sum[i][c] += e.n * e.c[c]
			}
		}
		for i := fixed; i < len(centers); i++ {
			if n[i] > 0 {
				centers[i] = [3]float64{sum[i][0] / n[i], sum[i][1] / n[i], sum[i][2] / n[i]}
			}
		}
	}
}

func nearest(centers [][3]float64, c [3]float64) int {
	best, bd := 0, -1.0
	for i, x := range centers {
		if d := dist(x, c); bd < 0 || d < bd {
			best, bd = i, d
		}
	}
	return best
}

// mapper maps colors to the nearest of a palette's, remembering each
// color it has mapped, and dithers when asked.
type mapper struct {
	pal    [][3]float64 // in Oklab
	seen   map[uint32]uint8
	last   uint32 // the color mapped last, +1, and its index: a run of one color looks up once
	lastI  uint8
	dither float64 // the amplitude of the ordered dither, in levels of 255
}

// maxSeen bounds the colors a mapper remembers; past it, it starts over.
const maxSeen = 1 << 20

func newMapper(p color.Palette, dither float64) *mapper {
	m := &mapper{seen: map[uint32]uint8{}, dither: dither * 32}
	for _, c := range p {
		r, g, b, _ := c.RGBA()
		m.pal = append(m.pal, oklab(float64(r>>8), float64(g>>8), float64(b>>8)))
	}
	return m
}

// bayer8 is the 8×8 ordered dither matrix.
var bayer8 = [64]uint8{
	0, 32, 8, 40, 2, 34, 10, 42,
	48, 16, 56, 24, 50, 18, 58, 26,
	12, 44, 4, 36, 14, 46, 6, 38,
	60, 28, 52, 20, 62, 30, 54, 22,
	3, 35, 11, 43, 1, 33, 9, 41,
	51, 19, 59, 27, 49, 17, 57, 25,
	15, 47, 7, 39, 13, 45, 5, 37,
	63, 31, 55, 23, 61, 29, 53, 21,
}

// index returns the palette index for the color r, g, b at x, y. The
// dither depends on the position alone, so a pixel that has not changed
// maps as it did in the frame before.
func (m *mapper) index(r, g, b uint8, x, y int) uint8 {
	if m.dither > 0 {
		d := int((float64(bayer8[(y&7)*8+x&7])+0.5)/64*m.dither - m.dither/2)
		r, g, b = add8(r, d), add8(g, d), add8(b, d)
	}
	key := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	if key+1 == m.last {
		return m.lastI
	}
	i, ok := m.seen[key]
	if !ok {
		if len(m.seen) >= maxSeen {
			clear(m.seen)
		}
		i = uint8(nearest(m.pal, oklab(float64(r), float64(g), float64(b))))
		m.seen[key] = i
	}
	m.last, m.lastI = key+1, i
	return i
}

func add8(v uint8, d int) uint8 {
	return uint8(min(max(int(v)+d, 0), 255))
}
