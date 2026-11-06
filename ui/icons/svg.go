package icons

import (
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"math"
	"slices"
	"strings"
	"sync"
	"weak"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/svg"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/framestats"
)

// SVG is a parsed monochrome SVG. Its alpha coverage is tinted at draw time,
// including currentColor strokes and fills. Gradients and multicolor artwork
// should use ggui.Image instead. SVGs are intended to be trusted local assets.
// A shared SVG retains at most eight device-resolution rasterizations, and
// every SVG's together at most 32 MB, the least recently drawn let go first.
type SVG struct {
	mu    sync.Mutex
	doc   *svg.Document
	cache []*raster // least recently drawn first
}

// raster is an SVG drawn at a size, on the GPU. It is on its SVG's cache
// and on rasters, the list of every SVG's by when each was last drawn.
type raster struct {
	size  int
	image *ggfx.Image
	owner weak.Pointer[SVG] // weak, so the list keeps no SVG alive
	bytes int64

	// Under rasters.mu: the list's links, and whether the raster has been
	// taken off the list to be freed. Whoever takes it off frees it.
	prev, next *raster
	gone       bool
}

// rasterBudget is the bytes of rasters every SVG together keeps.
const rasterBudget = 32 << 20

// rasters is every SVG's rasters, by when each was last drawn. Its lock is
// taken under an SVG's, never the other way round, and no SVG's lock is
// taken while another's is held.
var rasters rasterList

type rasterList struct {
	mu         sync.Mutex
	head, tail *raster // most and least recently drawn
	bytes      int64
	budget     int64
}

func (l *rasterList) unlink(r *raster) {
	if r.prev != nil {
		r.prev.next = r.next
	} else {
		l.head = r.next
	}
	if r.next != nil {
		r.next.prev = r.prev
	} else {
		l.tail = r.prev
	}
	r.prev, r.next = nil, nil
}

func (l *rasterList) pushFront(r *raster) {
	r.next = l.head
	if l.head != nil {
		l.head.prev = r
	} else {
		l.tail = r
	}
	l.head = r
}

// add puts r on the list, as drawn now.
func (l *rasterList) add(r *raster) {
	l.mu.Lock()
	l.pushFront(r)
	l.bytes += r.bytes
	l.mu.Unlock()
	framestats.AddTexture(framestats.Icons, r.bytes)
}

// touch marks r drawn now, reporting false when it is gone, to be freed.
func (l *rasterList) touch(r *raster) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.gone {
		return false
	}
	if l.head != r {
		l.unlink(r)
		l.pushFront(r)
	}
	return true
}

// take takes r off the list, reporting whether the caller is to free it:
// false when the list took it off first.
func (l *rasterList) take(r *raster) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.gone {
		return false
	}
	l.unlink(r)
	r.gone = true
	l.bytes -= r.bytes
	return true
}

// overBudget takes off the least recently drawn rasters past the budget,
// for the caller to free, sparing the one most recently drawn.
func (l *rasterList) overBudget() []*raster {
	l.mu.Lock()
	defer l.mu.Unlock()
	budget := l.budget
	if budget == 0 {
		budget = rasterBudget
	}
	var out []*raster
	for l.bytes > budget && l.tail != nil && l.tail != l.head {
		r := l.tail
		l.unlink(r)
		r.gone = true
		l.bytes -= r.bytes
		out = append(out, r)
	}
	return out
}

// free frees r, taken off the list, under its SVG's lock when it has one so
// that no Draw is drawing it.
func (r *raster) free() {
	if s := r.owner.Value(); s != nil {
		s.mu.Lock()
		if i := slices.Index(s.cache, r); i >= 0 {
			s.cache = slices.Delete(s.cache, i, i+1)
		}
		r.image.Deallocate()
		s.mu.Unlock()
	} else {
		r.image.Deallocate()
	}
	framestats.AddTexture(framestats.Icons, -r.bytes)
}

// scratch is the CPU images rasterize draws into before upload.
var scratch = sync.Pool{New: func() any { return new(image.RGBA) }}

// scratchRGBA returns a clear size×size image from scratch.
func scratchRGBA(size int) *image.RGBA {
	img := scratch.Get().(*image.RGBA)
	n := 4 * size * size
	if cap(img.Pix) < n {
		img.Pix = make([]byte, n)
	} else {
		img.Pix = img.Pix[:n]
		clear(img.Pix)
	}
	img.Stride, img.Rect = 4*size, image.Rect(0, 0, size, size)
	return img
}

// largeRaster is the side past which a raster is a texture of its own
// size: on the atlas it would take a page rounded up to powers of two.
const largeRaster = 512

// upload makes the GPU image of a raster of size, by way of scratch.
func (s *SVG) upload(size int) *ggfx.Image {
	img := scratchRGBA(size)
	s.draw(img)
	defer scratch.Put(img)
	if size > largeRaster {
		return ggfx.NewImageFromImageWithOptions(img, &ggfx.NewImageFromImageOptions{Unmanaged: true})
	}
	return ggfx.NewImageFromImage(img)
}

// Parse compiles SVG paths once without creating GPU resources.
func Parse(data []byte) (*SVG, error) {
	source := strings.ReplaceAll(string(data), "currentColor", "#ffffff")
	doc, err := svg.Parse(strings.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("icons: parse SVG: %w", err)
	}
	x, y, w, h := doc.ViewBox()
	if w <= 0 || h <= 0 || math.IsNaN(w+h+x+y) || math.IsInf(w+h+x+y, 0) {
		return nil, fmt.Errorf("icons: SVG needs a finite positive viewBox")
	}
	return &SVG{doc: doc}, nil
}

// Load reads an SVG from an embedded or other filesystem.
func Load(files fs.FS, path string) (*SVG, error) {
	data, err := fs.ReadFile(files, path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func (s *SVG) rasterize(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s.draw(img)
	return img
}

// draw draws the icon as a white mask over img, clear and square.
func (s *SVG) draw(img *image.RGBA) {
	size := img.Rect.Dx()
	x, y, w, h := s.doc.ViewBox()
	scale := float64(size) / max(w, h)
	var g ggfx.GeoM
	g.Translate(-x, -y)
	g.Scale(scale, scale)
	g.Translate((float64(size)-w*scale)/2, (float64(size)-h*scale)/2)
	s.doc.Draw(img, g)
	// Store a white coverage mask so recoloring never requires rasterization.
	for i := 0; i < len(img.Pix); i += 4 {
		a := img.Pix[i+3]
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = a, a, a
	}
}

// Image is the icon drawn size pixels square, in white over clear: a mask
// whose alpha is the icon, as a template image wants, such as a tray's.
func (s *SVG) Image(size int) *image.RGBA {
	if s == nil || size < 1 {
		return nil
	}
	return s.rasterize(size)
}

// Draw paints the icon centered in r, preserving its aspect ratio. Rotation is
// clockwise in radians. Call on the UI thread, as with other Canvas methods.
func (s *SVG) Draw(dst *ggui.Canvas, r ggui.Rect, col color.Color, rotation float64) {
	if s == nil || dst == nil || dst.Image == nil || col == nil {
		return
	}
	side := min(r.Size.W, r.Size.H)
	if side <= 0 || math.IsNaN(side) || math.IsInf(side, 0) {
		return
	}
	size := int(min(1024, math.Ceil(side*dst.Scale())))
	if size < 1 {
		return
	}
	at := ggui.FitContain.Place(ggui.Sz(1, 1), r)
	opts := ggui.ImageOptions{Fit: ggui.FitFill, Tint: col, Rotation: rotation}
	s.mu.Lock()
	var hit *raster
	for i, c := range s.cache {
		if c.size == size {
			s.cache = append(slices.Delete(s.cache, i, i+1), c)
			hit = c
			break
		}
	}
	if hit != nil && rasters.touch(hit) {
		dst.DrawImage(hit.image, at, opts)
		s.mu.Unlock()
		return
	}
	// A raster the list took off to free is drawn anew; the list frees it.
	var evicted *raster
	if hit != nil {
		s.cache = s.cache[:len(s.cache)-1]
	} else if len(s.cache) == 8 {
		if rasters.take(s.cache[0]) {
			evicted = s.cache[0]
		}
		s.cache = slices.Delete(s.cache, 0, 1)
	}
	if evicted != nil {
		evicted.image.Deallocate()
		framestats.AddTexture(framestats.Icons, -evicted.bytes)
	}
	c := &raster{size: size, image: s.upload(size), owner: weak.Make(s), bytes: framestats.ImageBytes(size, size)}
	s.cache = append(s.cache, c)
	rasters.add(c)
	dst.DrawImage(c.image, at, opts)
	s.mu.Unlock()
	// Past the budget, rasters are freed with no SVG's lock held, as each
	// takes its own.
	for _, r := range rasters.overBudget() {
		r.free()
	}
}
