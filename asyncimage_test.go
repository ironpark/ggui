package ggui

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ironpark/ggfx"

	"github.com/ironpark/ggui/internal/goid"
)

// measured lays w out loosely and records the size it asked for, which a
// probe's root, laid out tight, does not tell.
type measured struct {
	w    Widget
	size Size
}

func (m *measured) Layout(c Constraints, env Env) Size {
	m.size = m.w.Layout(c.Loosen(), env)
	return c.Constrain(m.size)
}
func (m *measured) Paint(dst *Canvas, r Rect) { dst.Paint(m.w, Rct(r.Origin, m.size)) }

// measure is a probe around w and a function running a frame and returning
// w's size.
func measure(w Widget, size Size) (*Probe, func() Size) {
	m := &measured{w: w}
	p := NewProbe(m, size)
	return p, func() Size { p.Frame(); return m.size }
}

// pumpUntil runs frames until done reports true, failing after a second.
func pumpUntil(t *testing.T, p *Probe, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the image")
		}
		p.Frame()
		time.Sleep(time.Millisecond)
	}
	p.Frame()
}

// sized returns a loader of w×h images that counts its calls.
func sized(w, h int, calls *atomic.Int32) ImageLoader {
	return func(ctx context.Context, src string) (image.Image, error) {
		calls.Add(1)
		return image.NewRGBA(image.Rect(0, 0, w, h)), nil
	}
}

func TestAsyncImageDecodesOffTheUIThread(t *testing.T) {
	t.Parallel()
	ui := goid.ID()
	release := make(chan struct{})
	var loaderGoroutine atomic.Int64
	a := AsyncImage(t.Name()).Loader(func(ctx context.Context, src string) (image.Image, error) {
		loaderGoroutine.Store(goid.ID())
		<-release
		return image.NewRGBA(image.Rect(0, 0, 40, 20)), nil
	}).Placeholder(Box().Size(7, 7))
	a.cache = newImageCache(1 << 20)
	p, size := measure(a, Sz(300, 300))
	defer p.Close()
	if got := size(); got != Sz(7, 7) || a.Status() != Pending {
		t.Fatalf("before ready: size %v status %v, want the placeholder", got, a.Status())
	}
	close(release)
	pumpUntil(t, p, func() bool { return a.Status() == Ready })
	if got := size(); got != Sz(40, 20) {
		t.Fatalf("ready: size %v, want 40x20", got)
	}
	if id := loaderGoroutine.Load(); id == 0 || id == ui {
		t.Fatalf("loader ran on goroutine %d, the UI goroutine is %d", id, ui)
	}
}

func TestAsyncImageSameSrcDecodesOnce(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	cache := newImageCache(1 << 20)
	a := AsyncImage("same").Loader(sized(10, 10, &calls))
	b := AsyncImage("same").Loader(sized(10, 10, &calls))
	a.cache, b.cache = cache, cache
	p := NewProbe(Column(a, b), Sz(300, 300))
	defer p.Close()
	pumpUntil(t, p, func() bool { return a.Status() == Ready && b.Status() == Ready })
	if calls.Load() != 1 {
		t.Fatalf("loaded %d times, want once", calls.Load())
	}
	// A third, shown later, finds it ready.
	c := AsyncImage("same").Loader(sized(10, 10, &calls))
	c.cache = cache
	q := NewProbe(c, Sz(100, 100))
	defer q.Close()
	q.Frame()
	if c.Status() != Ready || calls.Load() != 1 {
		t.Fatalf("a cached src: status %v after %d loads", c.Status(), calls.Load())
	}
}

func TestAsyncImageMaxSizeDownscales(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	a := AsyncImage(t.Name()).Loader(sized(400, 100, &calls)).MaxSize(80)
	a.cache = newImageCache(1 << 20)
	p, size := measure(a, Sz(300, 300))
	defer p.Close()
	pumpUntil(t, p, func() bool { return a.Status() == Ready })
	if got := size(); got != Sz(80, 20) {
		t.Fatalf("size %v, want 80x20", got)
	}
}

func TestAsyncImageSrcChangeCancelsStaleLoad(t *testing.T) {
	t.Parallel()
	cancelled := make(chan string, 4)
	loader := func(ctx context.Context, src string) (image.Image, error) {
		if src == "slow" {
			<-ctx.Done()
			cancelled <- src
			return nil, ctx.Err()
		}
		return image.NewRGBA(image.Rect(0, 0, 12, 34)), nil
	}
	src := State("slow")
	a := AsyncImage("").BindSrc(src).Loader(loader)
	a.cache = newImageCache(1 << 20)
	p, size := measure(a, Sz(300, 300))
	defer p.Close()
	p.Frame()
	src.Set("fast")
	pumpUntil(t, p, func() bool { return a.Status() == Ready })
	select {
	case got := <-cancelled:
		if got != "slow" {
			t.Fatalf("cancelled %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("the stale load was not cancelled")
	}
	if got := size(); got != Sz(12, 34) {
		t.Fatalf("size %v, want the new image's", got)
	}
	a.cache.mu.Lock()
	_, stale := a.cache.entries[imageKey{src: "slow"}]
	a.cache.mu.Unlock()
	if stale {
		t.Fatal("the abandoned load is still cached")
	}
}

func TestAsyncImageErrorShowsFallback(t *testing.T) {
	t.Parallel()
	a := AsyncImage(t.Name()).Loader(func(context.Context, string) (image.Image, error) {
		return nil, errors.New("nope")
	}).Placeholder(Box().Size(5, 5)).Fallback(Box().Size(9, 3))
	a.cache = newImageCache(1 << 20)
	p, size := measure(a, Sz(300, 300))
	defer p.Close()
	pumpUntil(t, p, func() bool { return a.Status() == Failed })
	if got := size(); got != Sz(9, 3) || a.Err() == nil {
		t.Fatalf("failed: size %v err %v", got, a.Err())
	}
}

func TestAsyncImageLRUEvictsWhatIsNotShown(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	cache := newImageCache(2 * 400) // two idle 10x10 images
	var mu sync.Mutex
	var freed []*ggfx.Image
	cache.free = func(img *ggfx.Image) {
		mu.Lock()
		freed = append(freed, img)
		mu.Unlock()
		img.Deallocate()
	}
	src := State("a")
	a := AsyncImage("").BindSrc(src).Loader(sized(10, 10, &calls))
	a.cache = cache
	p := NewProbe(a, Sz(300, 300))
	defer p.Close()
	var shown []*ggfx.Image
	for _, s := range []string{"a", "b", "c", "d"} {
		src.Set(s)
		pumpUntil(t, p, func() bool { return a.Status() == Ready && a.h.entry != nil && a.h.entry.key.src == s })
		shown = append(shown, a.inner.img)
	}
	cache.mu.Lock()
	_, hasA := cache.entries[imageKey{src: "a"}]
	_, hasB := cache.entries[imageKey{src: "b"}]
	_, hasC := cache.entries[imageKey{src: "c"}]
	idle := cache.idleSum
	cache.mu.Unlock()
	if hasA || !hasB || !hasC || idle != 800 {
		t.Fatalf("cache a=%v b=%v c=%v idle=%d, want a evicted and b, c kept", hasA, hasB, hasC, idle)
	}
	mu.Lock()
	if len(freed) != 1 || freed[0] != shown[0] {
		t.Fatalf("freed %d images, want only a's", len(freed))
	}
	mu.Unlock()

	// Back to b: no load, it comes from the cache.
	before := calls.Load()
	src.Set("b")
	p.Frame()
	if a.Status() != Ready || calls.Load() != before {
		t.Fatalf("a cached image loaded again (%d -> %d)", before, calls.Load())
	}

	// A limit below what is shown evicts every idle image but not the one
	// on screen.
	cache.setLimit(0)
	p.Frame()
	if a.Status() != Ready || a.inner.img != shown[1] {
		t.Fatal("the image on screen was evicted")
	}
	mu.Lock()
	for _, img := range freed {
		if img == shown[1] {
			t.Fatal("the image on screen was deallocated")
		}
	}
	if len(freed) != 3 {
		t.Fatalf("freed %d images, want a, c and d", len(freed))
	}
	mu.Unlock()
}

func TestAsyncImageReleasesWhenItsOwnerGoes(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	cache := newImageCache(0)
	var a *AsyncImageWidget
	p := ProbeBuilder(func() Widget {
		a = AsyncImage("x").Loader(sized(4, 4, &calls))
		a.cache = cache
		return a
	}, Sz(100, 100))
	p.Frame()
	pumpUntil(t, p, func() bool { return a.Status() == Ready })
	p.Close()
	cache.mu.Lock()
	n := len(cache.entries)
	cache.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d entries left after the probe closed with a zero limit", n)
	}
}

func TestAsyncImageDefaultLoaderReadsFiles(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "x.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 6, 3))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	a := AsyncImage(path)
	a.cache = newImageCache(1 << 20)
	p, size := measure(a, Sz(100, 100))
	defer p.Close()
	pumpUntil(t, p, func() bool { return a.Status() != Pending })
	if got := size(); a.Status() != Ready || got != Sz(6, 3) {
		t.Fatalf("status %v size %v err %v", a.Status(), got, a.Err())
	}
}
