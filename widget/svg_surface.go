package widget

import (
	"image"
	"image/color"
	"runtime"
	"slices"
	"sync"
	"time"
	"weak"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/svg"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
)

const (
	// svgMaxSide caps the rasterized device-pixel dimension on either axis,
	// preserving aspect ratio.
	svgMaxSide = 1024

	// svgCacheBudget is the unified VRAM budget across shared static SVG
	// rasters and per-widget dynamic SVG surfaces.
	svgCacheBudget = 64 << 20

	// svgMaxDocVariants is the maximum number of cached static rasters kept
	// for a single svg.Document across sizes and colors.
	svgMaxDocVariants = 8

	// svgMostScratch is the largest pooled CPU RGBA buffer kept between
	// rasters (1024×1024×4 = 4 MiB, matching svgMaxSide).
	svgMostScratch = svgMaxSide * svgMaxSide * 4
)

// svgScratch pools CPU RGBA images so steady-state SVG animation allocates
// zero pixel buffers.
var svgScratch = gem.Pool[*image.RGBA]{New: func() *image.RGBA { return new(image.RGBA) }}

func svgScratchRGBA(w, h int) *image.RGBA {
	img := svgScratch.Get()
	n := 4 * w * h
	if cap(img.Pix) < n {
		img.Pix = make([]byte, n)
	} else {
		img.Pix = img.Pix[:n]
		clear(img.Pix)
	}
	img.Stride = 4 * w
	img.Rect = image.Rect(0, 0, w, h)
	return img
}

func releaseSVGScratch(img *image.RGBA) {
	if cap(img.Pix) <= svgMostScratch {
		svgScratch.Put(img)
	}
}

// svgKey identifies the exact rasterized frame held in a private svgSurface.
type svgKey struct {
	doc   *svg.Document
	w, h  int32
	t     time.Duration
	color uint32
	nodes uint64
}

// staticKey identifies a shared static raster in svgTextures.
type staticKey struct {
	doc   weak.Pointer[svg.Document]
	w, h  int32
	color uint32
}

// svgLRUEntry is one GPU texture tracked under the unified svgTextures budget,
// either a shared static raster or a private dynamic surface's texture.
type svgLRUEntry struct {
	isStatic   bool
	sKey       staticKey
	owner      *svgSurfaceHandle
	img        *ggfx.Image
	bytes      int64
	prev, next *svgLRUEntry
}

// svgTextures is the process-wide LRU registry tracking all widget.SVG GPU
// textures (shared static + per-widget dynamic) under a unified 64 MiB budget.
var svgTextures svgTexturePool

type svgTexturePool struct {
	mu          sync.Mutex
	head, tail  *svgLRUEntry // most and least recently used
	bytes       int64
	budget      int64 // svgCacheBudget when <= 0; tests may lower it
	statics     map[staticKey]*svgLRUEntry
	byDoc       map[weak.Pointer[svg.Document]][]*svgLRUEntry
	docCleanups map[weak.Pointer[svg.Document]]struct{}
}

func (p *svgTexturePool) unlinkLocked(e *svgLRUEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		p.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		p.tail = e.prev
	}
	e.prev, e.next = nil, nil
}

func (p *svgTexturePool) pushFrontLocked(e *svgLRUEntry) {
	e.next = p.head
	if p.head != nil {
		p.head.prev = e
	} else {
		p.tail = e
	}
	p.head = e
}

func (p *svgTexturePool) touchLocked(e *svgLRUEntry) {
	if p.head != e {
		p.unlinkLocked(e)
		p.pushFrontLocked(e)
	}
	if e.isStatic {
		if list := p.byDoc[e.sKey.doc]; len(list) > 1 {
			if i := slices.Index(list, e); i >= 0 && i < len(list)-1 {
				p.byDoc[e.sKey.doc] = append(slices.Delete(list, i, i+1), e)
			}
		}
	}
}

func (p *svgTexturePool) addLocked(e *svgLRUEntry) {
	p.pushFrontLocked(e)
	p.bytes += e.bytes
	ggui.AccountTexture(ggui.TextureImages, e.bytes)
}

func (p *svgTexturePool) freeLocked(e *svgLRUEntry) {
	p.unlinkLocked(e)
	p.bytes -= e.bytes
	if e.isStatic {
		delete(p.statics, e.sKey)
		if list := p.byDoc[e.sKey.doc]; len(list) > 0 {
			if i := slices.Index(list, e); i >= 0 {
				list = slices.Delete(list, i, i+1)
			}
			if len(list) == 0 {
				delete(p.byDoc, e.sKey.doc)
			} else {
				p.byDoc[e.sKey.doc] = list
			}
		}
	} else if e.owner != nil && e.owner.entry == e {
		e.owner.entry = nil
		e.owner.hasKey = false
	}
	if e.img != nil {
		e.img.Deallocate()
		e.img = nil
	}
	ggui.AccountTexture(ggui.TextureImages, -e.bytes)
}

func (p *svgTexturePool) trimLocked(keep *svgLRUEntry) {
	budget := p.budget
	if budget <= 0 {
		budget = svgCacheBudget
	}
	if p.bytes <= budget {
		return
	}
	// Evict dead static documents first.
	for e := p.tail; e != nil && p.bytes > budget; {
		prev := e.prev
		if e != keep && e.isStatic && e.sKey.doc.Value() == nil {
			p.freeLocked(e)
		}
		e = prev
	}
	// Evict least-recently-used entries until within budget, sparing keep.
	for e := p.tail; e != nil && p.bytes > budget; {
		prev := e.prev
		if e != keep {
			p.freeLocked(e)
		}
		e = prev
	}
}

func (p *svgTexturePool) sweepDoc(wp weak.Pointer[svg.Document]) {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := p.byDoc[wp]
	for len(list) > 0 {
		p.freeLocked(list[0])
		list = p.byDoc[wp]
	}
	delete(p.docCleanups, wp)
}

// drain frees every tracked SVG texture; used by tests to isolate VRAM checks.
func (p *svgTexturePool) drain() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for p.tail != nil {
		p.freeLocked(p.tail)
	}
	clear(p.statics)
	clear(p.byDoc)
}

// getStatic returns a cached GPU texture for a static SVG render, rasterizing
// and inserting it on a miss. The bool reports whether it was a cache hit.
func (p *svgTexturePool) getStatic(doc *svg.Document, w, h int, colKey uint32, col color.Color, geoM ggfx.GeoM) (*ggfx.Image, bool) {
	wp := weak.Make(doc)
	sk := staticKey{doc: wp, w: int32(w), h: int32(h), color: colKey}

	p.mu.Lock()
	if e := p.statics[sk]; e != nil {
		p.touchLocked(e)
		img := e.img
		p.mu.Unlock()
		runtime.KeepAlive(doc)
		return img, true
	}
	p.mu.Unlock()

	buf := svgScratchRGBA(w, h)
	defer releaseSVGScratch(buf)
	var opts *svg.DrawOptions
	if col != nil {
		opts = &svg.DrawOptions{CurrentColor: col}
	}
	doc.DrawWith(buf, geoM, opts)
	img := ggui.UploadTexture(buf, ggfx.PixelFormatRGBA)

	p.mu.Lock()
	if e := p.statics[sk]; e != nil {
		p.touchLocked(e)
		cached := e.img
		p.mu.Unlock()
		img.Deallocate()
		runtime.KeepAlive(doc)
		return cached, true
	}
	if p.statics == nil {
		p.statics = make(map[staticKey]*svgLRUEntry)
		p.byDoc = make(map[weak.Pointer[svg.Document]][]*svgLRUEntry)
		p.docCleanups = make(map[weak.Pointer[svg.Document]]struct{})
	}
	for list := p.byDoc[wp]; len(list) >= svgMaxDocVariants; list = p.byDoc[wp] {
		p.freeLocked(list[0])
	}
	if _, has := p.docCleanups[wp]; !has {
		p.docCleanups[wp] = struct{}{}
		runtime.AddCleanup(doc, func(dead weak.Pointer[svg.Document]) {
			svgTextures.sweepDoc(dead)
		}, wp)
	}
	e := &svgLRUEntry{
		isStatic: true,
		sKey:     sk,
		img:      img,
		bytes:    ggui.ImageBytes(w, h, ggfx.PixelFormatRGBA),
	}
	p.statics[sk] = e
	p.byDoc[wp] = append(p.byDoc[wp], e)
	p.addLocked(e)
	p.trimLocked(e)
	p.mu.Unlock()
	runtime.KeepAlive(doc)
	return img, false
}

// svgSurfaceHandle holds the GPU texture and svg.Renderer owned by one
// svgSurface. It is separated from svgSurface so runtime.AddCleanup on
// svgSurface can release it idempotently without keeping svgSurface alive.
type svgSurfaceHandle struct {
	doc      *svg.Document
	renderer *svg.Renderer
	entry    *svgLRUEntry
	w, h     int32
	key      svgKey
	hasKey   bool
	released bool
}

func (h *svgSurfaceHandle) release() {
	svgTextures.mu.Lock()
	if h.released {
		svgTextures.mu.Unlock()
		return
	}
	h.released = true
	if h.entry != nil {
		svgTextures.freeLocked(h.entry)
	}
	r := h.renderer
	h.renderer = nil
	svgTextures.mu.Unlock()
	if r != nil {
		r.Release()
	}
}

func (h *svgSurfaceHandle) releaseTexture() {
	svgTextures.mu.Lock()
	if h.entry != nil {
		svgTextures.freeLocked(h.entry)
	}
	h.hasKey = false
	svgTextures.mu.Unlock()
}

// svgSurface is a per-widget reusable GPU surface and svg.Renderer for
// animated or programmatically overridden SVGs. It updates its unmanaged
// texture in place via WritePixels when the size is unchanged.
type svgSurface struct {
	h           *svgSurfaceHandle
	rasterCount uint64
	onRaster    func(*image.RGBA) // optional test hook called with each rasterized buffer
}

func newSVGSurface(doc *svg.Document) *svgSurface {
	h := &svgSurfaceHandle{
		doc:      doc,
		renderer: doc.NewRenderer(),
	}
	s := &svgSurface{h: h}
	runtime.AddCleanup(s, func(handle *svgSurfaceHandle) {
		handle.release()
	}, h)
	return s
}

func (s *svgSurface) releaseTexture() {
	if s != nil && s.h != nil {
		s.h.releaseTexture()
	}
}

// render returns the surface's GPU image for key, re-rasterizing and uploading
// pixels only on a key mismatch or after an LRU eviction.
func (s *svgSurface) render(key svgKey, geoM ggfx.GeoM, opts *svg.DrawOptions) *ggfx.Image {
	h := s.h
	svgTextures.mu.Lock()
	if !h.released && h.entry != nil && h.hasKey && h.key == key {
		svgTextures.touchLocked(h.entry)
		img := h.entry.img
		svgTextures.mu.Unlock()
		runtime.KeepAlive(s)
		return img
	}
	svgTextures.mu.Unlock()

	w, ht := int(key.w), int(key.h)
	buf := svgScratchRGBA(w, ht)
	defer releaseSVGScratch(buf)
	s.rasterCount++
	h.renderer.Draw(buf, geoM, opts)
	if s.onRaster != nil {
		s.onRaster(buf)
	}

	svgTextures.mu.Lock()
	if h.entry != nil && h.w == key.w && h.h == key.h {
		img := h.entry.img
		img.WritePixels(buf.Pix)
		svgTextures.touchLocked(h.entry)
		h.key = key
		h.hasKey = true
		svgTextures.mu.Unlock()
		runtime.KeepAlive(s)
		return img
	}
	if h.entry != nil {
		svgTextures.freeLocked(h.entry)
	}
	// Mutable animated textures are unmanaged to avoid atlas churn on WritePixels.
	img := ggfx.NewImageWithOptions(image.Rect(0, 0, w, ht), &ggfx.NewImageOptions{
		Unmanaged: true,
		Format:    ggfx.PixelFormatRGBA,
	})
	img.WritePixels(buf.Pix)
	e := &svgLRUEntry{
		owner: h,
		img:   img,
		bytes: ggui.ImageBytes(w, ht, ggfx.PixelFormatRGBA),
	}
	h.entry = e
	h.w, h.h = key.w, key.h
	h.key = key
	h.hasKey = true
	svgTextures.addLocked(e)
	svgTextures.trimLocked(e)
	svgTextures.mu.Unlock()
	runtime.KeepAlive(s)
	return img
}

// packRGBA8 packs a resolved color into a comparable 32-bit premultiplied RGBA8 key.
func packRGBA8(c color.Color) uint32 {
	if c == nil {
		return 0x000000ff
	}
	r, g, b, a := c.RGBA()
	return (r>>8)<<24 | (g>>8)<<16 | (b>>8)<<8 | (a >> 8)
}
