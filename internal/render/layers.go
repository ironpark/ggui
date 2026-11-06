package render

import (
	"image"
	"slices"

	"github.com/ironpark/ggui/internal/framestats"
)

// Texture is what a LayerPool lends: an image with bounds, which it frees
// when no layer wants it any more. *ggfx.Image is one.
type Texture interface {
	Bounds() image.Rectangle
	Deallocate()
}

// LayerPool lends the images layers paint into, by depth: a layer opened
// inside another takes one of the depth below.
//
// An image is only as large as an area asks for, on a grid of LayerGrid
// pixels so that one moving a little keeps it, rather than the window's
// size: a fade along a tab bar 60 pixels tall took a layer of the whole
// window, 80 MB on a 6K display. Each depth keeps the images its areas
// took this frame, so layers taken in turn at the same depth over
// different areas, as each pane's tab bar does, do not take and give back
// an image each every frame.
//
// The zero LayerPool is ready to use.
type LayerPool[T Texture] struct {
	// Gauge, when set, is told the bytes of RGBA pixels of each image the
	// pool makes, and their negation as it frees one.
	Gauge func(bytes int64)

	depths [][]pooled[T]
	depth  int // layers open now
	peak   int // the most open at once this frame
}

// pooled is an image a depth keeps, and whether a layer took it this frame.
type pooled[T Texture] struct {
	img  T
	used bool
}

// LayerGrid is the grid in pixels a layer image's bounds are rounded out
// to.
const LayerGrid = 64

// Borrow returns an image for the next depth that covers area of whole,
// all of whole when area is empty, making one with alloc when the depth
// keeps none that does. The caller clears what it paints into, and calls
// Return when its layer closes.
func (p *LayerPool[T]) Borrow(whole, area image.Rectangle, alloc func(image.Rectangle) T) T {
	if area.Empty() {
		area = whole
	}
	i := p.depth
	p.depth++
	p.peak = max(p.peak, p.depth)
	if len(p.depths) == i { // depths open one at a time
		p.depths = append(p.depths, nil)
	}
	pool := p.depths[i]
	// The smallest that holds the area.
	best, bestPx := -1, 0
	for j, l := range pool {
		b := l.img.Bounds()
		if px := b.Dx() * b.Dy(); area.In(b) && (best < 0 || px < bestPx) {
			best, bestPx = j, px
		}
	}
	if best >= 0 {
		pool[best].used = true
		return pool[best].img
	}
	b := image.Rect(area.Min.X&^(LayerGrid-1), area.Min.Y&^(LayerGrid-1),
		(area.Max.X+LayerGrid-1)&^(LayerGrid-1), (area.Max.Y+LayerGrid-1)&^(LayerGrid-1)).Intersect(whole)
	if b.Empty() {
		b = area
	}
	// What the new image holds, it replaces.
	pool = slices.DeleteFunc(pool, func(l pooled[T]) bool {
		if l.img.Bounds().In(b) {
			p.free(l.img)
			return true
		}
		return false
	})
	img := alloc(b)
	if p.Gauge != nil {
		p.Gauge(textureBytes(img))
	}
	p.depths[i] = append(pool, pooled[T]{img: img, used: true})
	return img
}

// Return closes the deepest layer open.
func (p *LayerPool[T]) Return() { p.depth-- }

// Depth is the count of layers open.
func (p *LayerPool[T]) Depth() int { return p.depth }

// Len is the count of images kept, at every depth.
func (p *LayerPool[T]) Len() int {
	n := 0
	for _, pool := range p.depths {
		n += len(pool)
	}
	return n
}

// EndFrame frees the images no layer took this frame, and the depths past
// the deepest the frame opened.
func (p *LayerPool[T]) EndFrame() {
	for i, pool := range p.depths {
		p.depths[i] = slices.DeleteFunc(pool, func(l pooled[T]) bool {
			if i >= p.peak || !l.used {
				p.free(l.img)
				return true
			}
			return false
		})
		for j := range p.depths[i] {
			p.depths[i][j].used = false
		}
	}
	for len(p.depths) > 0 && len(p.depths[len(p.depths)-1]) == 0 {
		p.depths = p.depths[:len(p.depths)-1]
	}
	p.peak = 0
}

// FreeAll frees every image kept.
func (p *LayerPool[T]) FreeAll() {
	for _, pool := range p.depths {
		for _, l := range pool {
			p.free(l.img)
		}
	}
	p.depths = p.depths[:0]
	p.peak = 0
}

// free frees img, a pool's image.
func (p *LayerPool[T]) free(img T) {
	if p.Gauge != nil {
		p.Gauge(-textureBytes(img))
	}
	img.Deallocate()
}

// textureBytes is the bytes of RGBA pixels t takes.
func textureBytes(t Texture) int64 {
	b := t.Bounds()
	return framestats.ImageBytes(b.Dx(), b.Dy())
}
