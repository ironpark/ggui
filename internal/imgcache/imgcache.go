// Package imgcache keeps the images an app loads off its UI thread: each
// loaded once however many widgets show it, decoded, scaled and turned
// into RGBA pixels on a goroutine, uploaded on the UI thread, and kept
// while nothing shows it only within a budget of bytes, the least recently
// shown going first.
//
// It is generic in the texture an image is uploaded to, so that it can be
// tested and measured with no GPU behind it.
package imgcache

import (
	"context"
	"errors"
	"image"
	"sync"

	"golang.org/x/image/draw"
)

// Status is where an entry's image is: loading, ready to show, or failed.
type Status uint8

const (
	Pending Status = iota
	Ready
	Failed
)

// Key is what an entry is decoded for: a source, and the box of pixels it
// is scaled to.
//
// Without Cover the image is shrunk, keeping its aspect ratio, until it
// fits inside W×H, and a side of 0 sets no limit. With Cover it is shrunk
// only as far as it still covers W×H, and a side of 0 asks for nothing.
// An image already within the box keeps its size: it is never enlarged.
type Key struct {
	Src   string
	W, H  int
	Cover bool
}

// Max is the Key of src no larger than side pixels on its larger side, 0
// for its full size.
func Max(src string, side int) Key { return Key{Src: src, W: side, H: side} }

// Loader produces the image for src. It runs on a goroutine of its own and
// should give up when ctx is cancelled.
type Loader func(ctx context.Context, src string) (image.Image, error)

// Waiter is a holder waiting for an entry to load: how to reach its UI
// thread, and what to tell it there.
type Waiter struct {
	Post   func(func())
	Notify func()
}

// Entry is one image in the cache, from the moment it is asked for.
type Entry[T any] struct {
	key     Key
	status  Status
	decoded image.Image // handed over by the loader, until it is uploaded
	source  image.Point // the loaded image's size, before it was scaled
	tex     T
	err     error
	bytes   int64
	refs    int
	cancel  context.CancelFunc
	waiters []Waiter
	// Its place among the idle entries while nothing holds it: newer is
	// toward the front, the most recently released.
	newer, older *Entry[T]
	idle         bool
	dead         bool // evicted or abandoned; a holder must ask again
}

// Cache holds images by Key, with those nothing holds kept in least
// recently used order within a byte budget.
type Cache[T any] struct {
	mu      sync.Mutex
	limit   int64
	idleSum int64 // bytes of the idle entries, the ones the limit applies to
	entries map[Key]*Entry[T]
	front   *Entry[T] // the idle entry released last
	back    *Entry[T] // and first, which goes first
	upload  func(image.Image) (T, int64)
	free    func(T)
}

// New returns a cache that keeps up to limit bytes of images nothing
// holds. upload turns a decoded image into a texture on the UI thread and
// says how many bytes it costs; the image it is given is always an
// *image.RGBA holding exactly its own pixels, so that it has only to copy
// them. free lets go of a texture the cache drops.
func New[T any](limit int64, upload func(image.Image) (T, int64), free func(T)) *Cache[T] {
	return &Cache[T]{limit: max(limit, 0), entries: map[Key]*Entry[T]{}, upload: upload, free: free}
}

// SetLimit changes the budget, dropping what no longer fits.
func (c *Cache[T]) SetLimit(bytes int64) {
	c.mu.Lock()
	c.limit = max(bytes, 0)
	victims := c.evictLocked()
	c.mu.Unlock()
	c.freeAll(victims)
}

// Has reports whether the cache has an entry for k.
func (c *Cache[T]) Has(k Key) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.entries[k]
	return ok
}

// Len is the count of entries, held or idle.
func (c *Cache[T]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// acquire returns the entry for k with one more holder, starting its load
// when there is none.
func (c *Cache[T]) acquire(k Key, load Loader, w Waiter) *Entry[T] {
	c.mu.Lock()
	e := c.entries[k]
	if e == nil {
		ctx, cancel := context.WithCancel(context.Background())
		e = &Entry[T]{key: k, status: Pending, cancel: cancel}
		c.entries[k] = e
		go c.run(ctx, e, load)
	}
	if e.idle {
		c.unidle(e)
	}
	e.refs++
	if e.status == Pending && e.decoded == nil && w.Post != nil {
		e.waiters = append(e.waiters, w)
	}
	decoded := e.decoded != nil
	c.mu.Unlock()
	if decoded { // loaded while nothing waited on its UI thread
		c.uploadEntry(e)
	}
	return e
}

// forget takes e out of the cache.
func (c *Cache[T]) forget(e *Entry[T]) { delete(c.entries, e.key) }

// release drops one holder. An entry nothing holds is cancelled while it
// loads, forgotten when it failed and kept, within the budget, when it is
// ready.
func (c *Cache[T]) release(e *Entry[T]) {
	c.mu.Lock()
	e.refs--
	var victims []T
	if e.refs <= 0 && !e.dead {
		e.refs = 0
		if e.status == Ready {
			c.toIdle(e)
			victims = c.evictLocked()
		} else {
			e.dead = true
			e.cancel()
			c.forget(e)
		}
	}
	c.mu.Unlock()
	c.freeAll(victims)
}

// run loads and prepares an entry's image on a goroutine of its own.
func (c *Cache[T]) run(ctx context.Context, e *Entry[T], load Loader) {
	img, err := load(ctx, e.key.Src)
	if err == nil && (img == nil || img.Bounds().Empty()) {
		err = errors.New("ggui: load image: the image has no pixels")
	}
	var source image.Point
	if err == nil {
		source = img.Bounds().Size()
		img = RGBA(Fit(img, e.key.W, e.key.H, e.key.Cover))
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
		e.decoded, e.source = img, source
	}
	waiters := e.waiters
	e.waiters = nil
	c.mu.Unlock()
	for _, w := range waiters {
		w.Post(func() {
			c.uploadEntry(e)
			w.Notify()
		})
	}
}

// uploadEntry turns a decoded image into a texture, on the UI thread.
func (c *Cache[T]) uploadEntry(e *Entry[T]) {
	c.mu.Lock()
	if e.dead || e.decoded == nil {
		c.mu.Unlock()
		return
	}
	src := e.decoded
	e.decoded = nil
	c.mu.Unlock()
	tex, bytes := c.upload(src)
	c.mu.Lock()
	if e.dead {
		c.mu.Unlock()
		c.free(tex)
		return
	}
	e.tex, e.status, e.bytes = tex, Ready, bytes
	if e.refs == 0 && !e.idle {
		c.toIdle(e)
	}
	victims := c.evictLocked()
	c.mu.Unlock()
	c.freeAll(victims)
}

// evictLocked drops idle entries, oldest first, until the idle ones fit
// the limit, and returns their textures to free once the lock is released.
func (c *Cache[T]) evictLocked() []T {
	var victims []T
	for c.idleSum > c.limit && c.back != nil {
		e := c.back
		c.unidle(e)
		e.dead = true
		c.forget(e)
		// Only a Ready entry is ever idle, so it has a texture.
		var zero T
		victims = append(victims, e.tex)
		e.tex = zero
	}
	return victims
}

// toIdle puts e at the front of the idle entries.
func (c *Cache[T]) toIdle(e *Entry[T]) {
	e.idle, e.newer, e.older = true, nil, c.front
	if c.front != nil {
		c.front.newer = e
	} else {
		c.back = e
	}
	c.front = e
	c.idleSum += e.bytes
}

// unidle takes e out of the idle entries.
func (c *Cache[T]) unidle(e *Entry[T]) {
	if e.newer != nil {
		e.newer.older = e.older
	} else {
		c.front = e.older
	}
	if e.older != nil {
		e.older.newer = e.newer
	} else {
		c.back = e.newer
	}
	e.idle, e.newer, e.older = false, nil, nil
	c.idleSum -= e.bytes
}

func (c *Cache[T]) freeAll(texs []T) {
	for _, t := range texs {
		c.free(t)
	}
}

// Handle is one holder's hold on an entry. It is apart from the holder so
// that collecting the holder can release it.
type Handle[T any] struct {
	mu    sync.Mutex
	cache *Cache[T]
	entry *Entry[T]
}

// Get returns what the entry h holds for k of c has to show, and false
// when it holds none: nothing, another key or cache, or an entry evicted
// since, which the holder must Acquire again.
func (h *Handle[T]) Get(c *Cache[T], k Key) (st Status, tex T, err error, ok bool) {
	h.mu.Lock()
	e := h.entry
	if e == nil || h.cache != c || e.key != k {
		h.mu.Unlock()
		return Pending, tex, nil, false
	}
	h.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if e.dead {
		return Pending, tex, nil, false
	}
	return e.status, e.tex, e.err, true
}

// Source is the size the held entry's image was loaded at, before it was
// scaled to its Key; zero until it has loaded.
func (h *Handle[T]) Source() image.Point {
	h.mu.Lock()
	c, e := h.cache, h.entry
	h.mu.Unlock()
	if e == nil {
		return image.Point{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return e.source
}

// Acquire holds the entry of c for k in place of the one held before,
// starting its load with load when there is none. The new one is taken
// first, so that going back to an idle image cannot evict it to make room
// for the one being let go. w is told on its UI thread when the load ends.
// It returns what the entry has to show now, as Get does.
func (h *Handle[T]) Acquire(c *Cache[T], k Key, load Loader, w Waiter) (Status, T, error) {
	e := c.acquire(k, load, w)
	h.hold(c, e)
	c.mu.Lock()
	defer c.mu.Unlock()
	return e.status, e.tex, e.err
}

// AcquireReady holds, in place of the entry held before, an entry of c for
// src whose image is ready to show, for a holder with nothing to show while
// the size it wants loads: one that pick accepts when there is one, else
// the one of the most bytes. It returns the entry's Key, and false, holding
// what it held, when c has none ready. pick runs under the cache's lock.
//
// It looks through every entry, which is fine for what a holder does once
// when it is first given a src.
func (h *Handle[T]) AcquireReady(c *Cache[T], src string, pick func(Key) bool) (Key, bool) {
	c.mu.Lock()
	var e *Entry[T]
	for k, o := range c.entries {
		if k.Src != src || o.status != Ready || o.dead {
			continue
		}
		if pick(k) {
			e = o
			break
		}
		if e == nil || o.bytes > e.bytes {
			e = o
		}
	}
	if e == nil {
		c.mu.Unlock()
		return Key{}, false
	}
	if e.idle {
		c.unidle(e)
	}
	e.refs++
	c.mu.Unlock()
	h.hold(c, e)
	return e.key, true
}

// hold puts e, which the caller has taken a hold on, in place of what h
// held, and lets go of that.
func (h *Handle[T]) hold(c *Cache[T], e *Entry[T]) {
	h.mu.Lock()
	oldCache, old := h.cache, h.entry
	h.cache, h.entry = c, e
	h.mu.Unlock()
	if old != nil {
		oldCache.release(old)
	}
}

// Release lets go of the entry h holds.
func (h *Handle[T]) Release() {
	h.mu.Lock()
	c, e := h.cache, h.entry
	h.cache, h.entry = nil, nil
	h.mu.Unlock()
	if e != nil {
		c.release(e)
	}
}

// Fit scales img down to the box w×h as a Key with those sides and cover
// says, keeping its aspect ratio; an image already within it is returned as
// it is. A scaled image is an *image.RGBA.
//
// CatmullRom costs in proportion to the pixels it reads, so an image many
// times larger than the box is first brought to twice its size by the
// cheap ApproxBiLinear, which costs in proportion to the pixels it writes,
// and CatmullRom smooths that last step: a 6000×4000 photo fitted to 512
// takes a fifteenth of the time.
func Fit(img image.Image, w, h int, cover bool) image.Image {
	b := img.Bounds()
	dw, dh := FitSize(b.Dx(), b.Dy(), w, h, cover)
	if dw == b.Dx() && dh == b.Dy() {
		return img
	}
	if b.Dx() > 4*dw && b.Dy() > 4*dh {
		mid := image.NewRGBA(image.Rect(0, 0, 2*dw, 2*dh))
		draw.ApproxBiLinear.Scale(mid, mid.Bounds(), img, b, draw.Src, nil)
		img, b = mid, mid.Bounds()
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// FitSize is the size Fit scales an iw×ih image to.
func FitSize(iw, ih, w, h int, cover bool) (int, int) {
	// The image is scaled by w/iw, to the box's width, or by h/ih, to its
	// height: for a box to fit inside, the smaller of the two; to cover,
	// the larger. A side of 0 does not take part.
	var byWidth bool
	switch {
	case w > 0 && h > 0:
		wide := int64(w)*int64(ih) <= int64(h)*int64(iw) // w/iw <= h/ih
		byWidth = wide != cover
		if int64(w)*int64(ih) == int64(h)*int64(iw) {
			byWidth = true
		}
	case w > 0:
		byWidth = true
	case h > 0:
		byWidth = false
	default:
		return iw, ih
	}
	// Covering rounds the other side up, so that it still covers.
	scale := func(other, side, of int) int {
		n := int64(other) * int64(side)
		q := n / int64(of)
		if cover && n%int64(of) != 0 {
			q++
		}
		return max(1, int(q))
	}
	if byWidth {
		if w >= iw {
			return iw, ih
		}
		return w, scale(ih, w, iw)
	}
	if h >= ih {
		return iw, ih
	}
	return scale(iw, h, ih), h
}

// RGBA returns img as an *image.RGBA of exactly its pixels, converting it
// when it is another kind -- a JPEG's YCbCr, a PNG's NRGBA, a GIF's
// palette -- or a part of a larger one.
func RGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	if m, ok := img.(*image.RGBA); ok && len(m.Pix) == 4*b.Dx()*b.Dy() {
		return m
	}
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}
