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
	"github.com/ironpark/ggui/internal/render"
	"github.com/ironpark/ggui/pkg/gem"
)

// SVG is a parsed monochrome SVG. Its alpha coverage is tinted at draw time,
// including currentColor strokes and fills. Gradients and multicolor artwork
// should use ggui.Image instead. SVGs are intended to be trusted local assets.
// A shared SVG retains at most eight device-resolution rasterizations, and
// every SVG's together at most 32 MB, the least recently drawn let go first.
type SVG struct {
	doc   *svg.Document
	cache []*raster // under rasters.mu, least recently drawn first
}

// raster is an SVG drawn at a size, on the GPU. It is on its SVG's cache
// and on rasters, the list of every SVG's by when each was last drawn.
type raster struct {
	size       int
	image      *ggfx.Image
	owner      weak.Pointer[SVG] // weak, so the list keeps no SVG alive
	prev, next *raster
}

// rasterFormat is a raster's pixel format: an icon is white coverage, so
// alpha-only, a byte a pixel.
const rasterFormat = ggfx.PixelFormatAlpha

func (r *raster) bytes() int64 { return framestats.ImageBytesOf(r.size, r.size, rasterFormat) }

func (r *raster) bounds() image.Rectangle { return image.Rect(0, 0, r.size, r.size) }

// rasterBudget is the bytes of rasters every SVG together keeps.
const rasterBudget = 32 << 20

// rasters is every SVG's rasters, by when each was last drawn. Its lock
// guards the list and every SVG's cache. Draw is on the UI thread only, so
// it is uncontended.
var rasters rasterList

type rasterList struct {
	mu         sync.Mutex
	head, tail *raster // most and least recently drawn
	bytes      int64
	budget     int64 // rasterBudget when 0; tests lower it
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

// add puts r, new, on the list as drawn now.
func (l *rasterList) add(r *raster) {
	l.pushFront(r)
	l.bytes += r.bytes()
	framestats.KeptOf(framestats.Icons, r.bounds(), rasterFormat)
}

// touch marks r drawn now.
func (l *rasterList) touch(r *raster) {
	if l.head != r {
		l.unlink(r)
		l.pushFront(r)
	}
}

// free takes r off the list and its SVG's cache, and deallocates it.
func (l *rasterList) free(r *raster) {
	l.unlink(r)
	l.bytes -= r.bytes()
	if s := r.owner.Value(); s != nil {
		if i := slices.Index(s.cache, r); i >= 0 {
			s.cache = slices.Delete(s.cache, i, i+1)
		}
	}
	r.image.Deallocate()
	framestats.FreedOf(framestats.Icons, r.bounds(), rasterFormat)
}

// trim frees the least recently drawn rasters past the budget, sparing the
// one most recently drawn.
func (l *rasterList) trim() {
	budget := l.budget
	if budget == 0 {
		budget = rasterBudget
	}
	for l.bytes > budget && l.tail != nil && l.tail != l.head {
		l.free(l.tail)
	}
}

// scratch is the CPU images upload draws into.
var scratch = gem.Pool[*image.RGBA]{New: func() *image.RGBA { return new(image.RGBA) }}

// mostScratch is the largest buffer, in bytes, scratch keeps: an icon 256
// pixels square. A larger one is let go, not pinned for every small one after.
const mostScratch = 256 << 10

// scratchRGBA returns a clear size×size image from scratch.
func scratchRGBA(size int) *image.RGBA {
	img := scratch.Get()
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

// releaseScratch returns img, from scratchRGBA, to scratch unless it is past
// mostScratch.
func releaseScratch(img *image.RGBA) {
	if cap(img.Pix) <= mostScratch {
		scratch.Put(img)
	}
}

// upload makes the GPU image of a raster of size, by way of scratch: only
// its alpha, which the image keeps, so the mask is not whitened first.
func (s *SVG) upload(size int) *ggfx.Image {
	img := scratchRGBA(size)
	defer releaseScratch(img)
	s.drawCoverage(img)
	return render.NewTextureOf(img, rasterFormat)
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
	s.drawCoverage(img)
	// Store a white coverage mask so recoloring never requires rasterization.
	for i := 0; i < len(img.Pix); i += 4 {
		a := img.Pix[i+3]
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = a, a, a
	}
}

// drawCoverage draws the icon over img, clear and square, as the document
// colors it: only its alpha is the icon.
func (s *SVG) drawCoverage(img *image.RGBA) {
	size := img.Rect.Dx()
	x, y, w, h := s.doc.ViewBox()
	scale := float64(size) / max(w, h)
	var g ggfx.GeoM
	g.Translate(-x, -y)
	g.Scale(scale, scale)
	g.Translate((float64(size)-w*scale)/2, (float64(size)-h*scale)/2)
	s.doc.Draw(img, g)
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
	rasters.mu.Lock()
	defer rasters.mu.Unlock()
	for i, c := range s.cache {
		if c.size == size {
			s.cache = append(slices.Delete(s.cache, i, i+1), c)
			rasters.touch(c)
			dst.DrawImage(c.image, at, opts)
			return
		}
	}
	if len(s.cache) == 8 {
		rasters.free(s.cache[0])
	}
	c := &raster{size: size, image: s.upload(size), owner: weak.Make(s)}
	s.cache = append(s.cache, c)
	rasters.add(c)
	dst.DrawImage(c.image, at, opts)
	rasters.trim()
}
