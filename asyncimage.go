package ggui

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"runtime"
	"sync"

	"github.com/ironpark/ggfx"
	"golang.org/x/image/draw"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
)

// ImageLoader produces the image for src. It runs on a goroutine of its
// own, never the UI thread, and should give up when ctx is cancelled: the
// widget asking for it moved on to another src, or went away.
type ImageLoader func(ctx context.Context, src string) (image.Image, error)

// LoadImage is the default ImageLoader: it reads the PNG, JPEG or GIF file
// at the path src.
func LoadImage(ctx context.Context, src string) (image.Image, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("ggui: load image: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ggui: decode image: %w", err)
	}
	return img, nil
}

// AsyncImageWidget shows an image that is loaded and decoded off the UI
// thread. Build one with AsyncImage.
type AsyncImageWidget struct {
	props       property.Owner
	src         property.Value[string]
	loader      ImageLoader
	maxSize     int
	placeholder Widget
	fallback    Widget
	inner       *ImageWidget
	cache       *imageCache
	h           *imageHandle
	status      AsyncStatus
	err         error
}

// AsyncImage shows the image at src, which the Loader turns into pixels --
// by default src is a file path. The file is read and decoded on a
// goroutine, the Placeholder shows until it is ready, and the decoded
// image is kept in a process-wide cache bounded in bytes (see
// SetImageCacheLimit), so the same src shown twice, or again later, is
// decoded once. The cache is keyed by src and MaxSize, not by loader: give
// each loader srcs of its own.
//
// Once loaded it lays out and paints as Image does, with the same Fit,
// Size, Width, Height, Pixelated and Alt.
//
//	ggui.AsyncImage(photo.Path).MaxSize(512).Size(96, 96).Fit(ggui.FitCover).
//		Placeholder(ui.Skeleton(96, 96))
func AsyncImage(src string) *AsyncImageWidget {
	a := &AsyncImageWidget{loader: LoadImage, inner: Image(nil), cache: defaultImageCache}
	a.src.Set(src)
	a.h = &imageHandle{}
	// A widget dropped by a rebuild lets go of its image, so the cache may
	// evict it: through its owner when it was built under one, else when
	// it is collected.
	h := a.h
	if reactive.CurrentOwner() != nil {
		OnCleanup(h.release)
	}
	runtime.AddCleanup(a, func(h *imageHandle) { h.release() }, h)
	return a
}

// Src changes the image shown; the load of the previous one is cancelled
// if nothing else is showing it.
func (a *AsyncImageWidget) Src(src string) *AsyncImageWidget {
	if a.src.Set(src) {
		a.props.Changed()
	}
	return a
}

// BindSrc follows r for Src without a rebuild.
func (a *AsyncImageWidget) BindSrc(r Readable[string]) *AsyncImageWidget {
	if a.src.Bind(r, "BindSrc") {
		a.props.Changed()
	}
	return a
}

// Loader replaces how a src becomes an image: from an archive, the
// network, a thumbnailer. nil restores LoadImage.
func (a *AsyncImageWidget) Loader(fn ImageLoader) *AsyncImageWidget {
	if fn == nil {
		fn = LoadImage
	}
	a.loader = fn
	return a
}

// MaxSize downscales an image whose larger side exceeds px pixels to that
// size before it is uploaded, keeping its aspect ratio: a thumbnail of a
// photo costs a thumbnail's memory. 0, the default, keeps the full size.
func (a *AsyncImageWidget) MaxSize(px int) *AsyncImageWidget {
	defer property.Watch(&a.props, &a.maxSize)()
	a.maxSize = max(px, 0)
	return a
}

// Placeholder shows w until the image is ready, and when it fails without
// a Fallback. With a fixed Size the placeholder is given that size.
func (a *AsyncImageWidget) Placeholder(w Widget) *AsyncImageWidget {
	defer property.Watch(&a.props, &a.placeholder)()
	a.placeholder = w
	return a
}

// Fallback shows w in place of an image that failed to load.
func (a *AsyncImageWidget) Fallback(w Widget) *AsyncImageWidget {
	defer property.Watch(&a.props, &a.fallback)()
	a.fallback = w
	return a
}

// Fit sets how the image is placed when the box has another shape.
func (a *AsyncImageWidget) Fit(f ImageFit) *AsyncImageWidget { a.inner.Fit(f); return a }

// Size fixes both dimensions; see ImageWidget.Size.
func (a *AsyncImageWidget) Size(width, height float64) *AsyncImageWidget {
	a.inner.Size(width, height)
	return a
}

// Width fixes the width; the height follows the aspect ratio.
func (a *AsyncImageWidget) Width(v float64) *AsyncImageWidget { a.inner.Width(v); return a }

// Height fixes the height; the width follows the aspect ratio.
func (a *AsyncImageWidget) Height(v float64) *AsyncImageWidget { a.inner.Height(v); return a }

// Pixelated scales the image by its nearest pixel instead of smoothing it.
func (a *AsyncImageWidget) Pixelated(v bool) *AsyncImageWidget { a.inner.Pixelated(v); return a }

// Alt describes the image for a screen reader; see ImageWidget.Alt.
func (a *AsyncImageWidget) Alt(s string) *AsyncImageWidget { a.inner.Alt(s); return a }

// Status reports whether the image is still loading, shown, or failed, as
// of the last layout.
func (a *AsyncImageWidget) Status() AsyncStatus { return a.status }

// Err is why the image failed to load, when Status is Failed.
func (a *AsyncImageWidget) Err() error { return a.err }

// sync makes the handle hold the entry for the current src and reads its
// state. It runs on the UI thread, in Layout.
func (a *AsyncImageWidget) sync() {
	key := imageKey{src: a.src.Get(), max: a.maxSize}
	h := a.h
	if key.src == "" {
		h.release()
		a.status, a.err, a.inner.img = Failed, errors.New("ggui: AsyncImage has no src"), nil
		return
	}
	if !h.holds(a.cache, key) {
		post := loopPost(reactive.CurrentOwner())
		if l := runningLoop(); post == nil && l != nil {
			post = l.post
		}
		notify := func() { a.props.Changed() }
		h.acquire(a.cache, key, a.loader, post, notify)
	}
	a.status, a.inner.img, a.err = h.state()
}

// Layout implements Widget.
func (a *AsyncImageWidget) Layout(c Constraints, env Env) Size {
	defer a.props.Layout()()
	a.inner.props.Read()
	a.sync()
	if a.status == Ready {
		return a.inner.Layout(c, env)
	}
	w, h := a.inner.width, a.inner.height
	if p := a.shown(); p != nil {
		if w > 0 && h > 0 {
			c = Tight(c.Constrain(Sz(w, h)))
		}
		return p.Layout(c, env)
	}
	return c.Constrain(Sz(w, h))
}

// shown is the widget painted in place of the image, or nil.
func (a *AsyncImageWidget) shown() Widget {
	if a.status == Failed && a.fallback != nil {
		return a.fallback
	}
	return a.placeholder
}

// Paint implements Widget.
func (a *AsyncImageWidget) Paint(dst *Canvas, r Rect) {
	if a.status == Ready {
		a.inner.Paint(dst, r)
		return
	}
	if a.inner.alt != "" {
		dst.Leaf(r, Node{Role: RoleImage, Name: a.inner.alt})
	}
	if p := a.shown(); p != nil {
		dst.Paint(p, r)
	}
}

// SetImageCacheLimit bounds the memory the AsyncImage cache keeps for
// images nothing shows, in bytes of decoded RGBA pixels; the least
// recently shown go first, and are deallocated. Images on screen are never
// evicted, whatever they cost. The default is 256 MiB; 0 keeps nothing that
// is not shown. Call it on the UI thread.
func SetImageCacheLimit(bytes int64) { defaultImageCache.setLimit(bytes) }

var defaultImageCache = newImageCache(256 << 20)

// imageKey is what a cache entry is decoded for.
type imageKey struct {
	src string
	max int
}

// imageEntry is one image in the cache, from the moment it is asked for.
type imageEntry struct {
	key     imageKey
	status  AsyncStatus
	decoded image.Image // handed over by the loader, until it is uploaded
	img     *ggfx.Image
	err     error
	bytes   int64
	refs    int
	cancel  context.CancelFunc
	waiters []imageWaiter
	idle    *list.Element // its place in the LRU while nothing shows it
	dead    bool          // evicted or abandoned; a holder must ask again
}

// imageWaiter is a widget waiting for an entry: how to reach its UI thread
// and what to tell it there.
type imageWaiter struct {
	post   func(func())
	notify func()
}

// imageCache holds decoded images by key, with those nothing shows kept in
// least-recently-used order within a byte budget.
type imageCache struct {
	mu      sync.Mutex
	limit   int64
	idleSum int64 // bytes of the idle entries, the ones the limit applies to
	entries map[imageKey]*imageEntry
	idle    *list.List // front is the most recently released
	free    func(*ggfx.Image)
	loads   int // loads started, for tests
}

func newImageCache(limit int64) *imageCache {
	return &imageCache{limit: limit, entries: map[imageKey]*imageEntry{}, idle: list.New(),
		free: (*ggfx.Image).Deallocate}
}

func (c *imageCache) setLimit(bytes int64) {
	c.mu.Lock()
	c.limit = max(bytes, 0)
	victims := c.evictLocked()
	c.mu.Unlock()
	c.freeAll(victims)
}

// acquire returns the entry for key with one more holder, starting its
// load when there is none.
func (c *imageCache) acquire(key imageKey, load ImageLoader, w imageWaiter) *imageEntry {
	c.mu.Lock()
	e := c.entries[key]
	if e == nil {
		ctx, cancel := context.WithCancel(context.Background())
		e = &imageEntry{key: key, status: Pending, cancel: cancel}
		c.entries[key] = e
		c.loads++
		go c.run(ctx, e, load)
	}
	if e.idle != nil {
		c.idle.Remove(e.idle)
		c.idleSum -= e.bytes
		e.idle = nil
	}
	e.refs++
	if e.status == Pending && e.decoded == nil && w.post != nil {
		e.waiters = append(e.waiters, w)
	}
	c.mu.Unlock()
	c.upload(e)
	return e
}

// release drops one holder. An entry nothing holds is cancelled while it
// loads, forgotten when it failed and kept, within the budget, when it is
// ready.
func (c *imageCache) release(e *imageEntry) {
	c.mu.Lock()
	e.refs--
	var victims []*ggfx.Image
	if e.refs <= 0 && !e.dead {
		e.refs = 0
		switch {
		case e.status == Ready:
			e.idle = c.idle.PushFront(e)
			c.idleSum += e.bytes
			victims = c.evictLocked()
		default:
			e.dead = true
			e.cancel()
			delete(c.entries, e.key)
		}
	}
	c.mu.Unlock()
	c.freeAll(victims)
}

// run loads and prepares an entry's image on a goroutine of its own.
func (c *imageCache) run(ctx context.Context, e *imageEntry, load ImageLoader) {
	img, err := load(ctx, e.key.src)
	if err == nil && (img == nil || img.Bounds().Empty()) {
		err = errors.New("ggui: load image: the image has no pixels")
	}
	if err == nil && e.key.max > 0 {
		img = downscale(img, e.key.max)
	}
	if err == nil {
		err = ctx.Err()
	}
	c.mu.Lock()
	if e.dead {
		c.mu.Unlock()
		return
	}
	if err != nil {
		e.status, e.err = Failed, err
	} else {
		e.decoded = img
	}
	waiters := e.waiters
	e.waiters = nil
	c.mu.Unlock()
	for _, w := range waiters {
		w.post(func() {
			c.upload(e)
			w.notify()
		})
	}
}

// upload turns a decoded image into a ggfx one, on the UI thread.
func (c *imageCache) upload(e *imageEntry) {
	c.mu.Lock()
	if e.dead || e.decoded == nil {
		c.mu.Unlock()
		return
	}
	src := e.decoded
	e.decoded = nil
	c.mu.Unlock()
	img := ggfx.NewImageFromImage(src)
	b := img.Bounds()
	c.mu.Lock()
	if e.dead {
		c.mu.Unlock()
		c.free(img)
		return
	}
	e.img, e.status = img, Ready
	e.bytes = int64(b.Dx()) * int64(b.Dy()) * 4
	if e.refs == 0 && e.idle == nil {
		e.idle = c.idle.PushFront(e)
		c.idleSum += e.bytes
	}
	victims := c.evictLocked()
	c.mu.Unlock()
	c.freeAll(victims)
}

// evictLocked drops idle entries, oldest first, until the idle ones fit
// the limit, and returns their images to free once the lock is released.
func (c *imageCache) evictLocked() []*ggfx.Image {
	var victims []*ggfx.Image
	for c.idleSum > c.limit {
		back := c.idle.Back()
		if back == nil {
			break
		}
		e := back.Value.(*imageEntry)
		c.idle.Remove(back)
		c.idleSum -= e.bytes
		e.idle, e.dead = nil, true
		delete(c.entries, e.key)
		if e.img != nil {
			victims = append(victims, e.img)
			e.img = nil
		}
	}
	return victims
}

func (c *imageCache) freeAll(imgs []*ggfx.Image) {
	for _, img := range imgs {
		c.free(img)
	}
}

// imageHandle is a widget's hold on one cache entry. It is apart from the
// widget so that collecting the widget can release it.
type imageHandle struct {
	mu    sync.Mutex
	cache *imageCache
	entry *imageEntry
}

func (h *imageHandle) holds(c *imageCache, key imageKey) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.entry == nil || h.cache != c || h.entry.key != key {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !h.entry.dead
}

// acquire holds the entry for key in place of the one held before. The new
// one is taken first, so that going back to an idle image cannot evict it
// to make room for the one being let go.
func (h *imageHandle) acquire(c *imageCache, key imageKey, load ImageLoader, post func(func()), notify func()) {
	e := c.acquire(key, load, imageWaiter{post: post, notify: notify})
	h.mu.Lock()
	oldCache, old := h.cache, h.entry
	h.cache, h.entry = c, e
	h.mu.Unlock()
	if old != nil {
		oldCache.release(old)
	}
}

func (h *imageHandle) release() {
	h.mu.Lock()
	c, e := h.cache, h.entry
	h.cache, h.entry = nil, nil
	h.mu.Unlock()
	if e != nil {
		c.release(e)
	}
}

// state is what the held entry has to show.
func (h *imageHandle) state() (AsyncStatus, *ggfx.Image, error) {
	h.mu.Lock()
	c, e := h.cache, h.entry
	h.mu.Unlock()
	if e == nil {
		return Pending, nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return e.status, e.img, e.err
}

// downscale shrinks img so that its larger side is at most side pixels,
// keeping the aspect ratio; a smaller image is returned as it is.
func downscale(img image.Image, side int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= side && h <= side {
		return img
	}
	if w >= h {
		w, h = side, max(1, h*side/w)
	} else {
		w, h = max(1, w*side/h), side
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}
