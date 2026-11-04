// Package imgcache keeps the images an app loads off its UI thread: each
// loaded once however many widgets show it, decoded and scaled on a
// goroutine, uploaded on the UI thread, and kept while nothing shows it
// only within a budget of bytes, the least recently shown going first.
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

// Key is what an entry is decoded for: a source, and the most pixels its
// larger side may have, 0 for no limit.
type Key struct {
	Src string
	Max int
}

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
// says how many bytes it costs; free lets go of a texture the cache drops.
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
			delete(c.entries, e.key)
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
	if err == nil && e.key.Max > 0 {
		img = Downscale(img, e.key.Max)
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
		delete(c.entries, e.key)
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

// Holds reports whether h holds a live entry of c for k.
func (h *Handle[T]) Holds(c *Cache[T], k Key) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.entry == nil || h.cache != c || h.entry.key != k {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !h.entry.dead
}

// Acquire holds the entry of c for k in place of the one held before,
// starting its load with load when there is none. The new one is taken
// first, so that going back to an idle image cannot evict it to make room
// for the one being let go. w is told on its UI thread when the load ends.
func (h *Handle[T]) Acquire(c *Cache[T], k Key, load Loader, w Waiter) {
	e := c.acquire(k, load, w)
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

// State is what the held entry has to show: Pending with nothing held.
func (h *Handle[T]) State() (Status, T, error) {
	h.mu.Lock()
	c, e := h.cache, h.entry
	h.mu.Unlock()
	var zero T
	if e == nil {
		return Pending, zero, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return e.status, e.tex, e.err
}

// Downscale shrinks img so that its larger side is at most side pixels,
// keeping the aspect ratio; a smaller image is returned as it is.
func Downscale(img image.Image, side int) image.Image {
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
