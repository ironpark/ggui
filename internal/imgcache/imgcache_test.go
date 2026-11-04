package imgcache

import (
	"context"
	"errors"
	"fmt"
	"image"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tex stands in for a GPU texture: its size, and whether it was freed.
type tex struct {
	w, h  int
	freed bool
}

// ui is a UI thread for the tests: what the cache posts runs when pump is
// called, on the test's goroutine.
type ui struct {
	mu   sync.Mutex
	jobs []func()
}

func (u *ui) post(fn func()) { u.mu.Lock(); u.jobs = append(u.jobs, fn); u.mu.Unlock() }

func (u *ui) pump() {
	u.mu.Lock()
	jobs := u.jobs
	u.jobs = nil
	u.mu.Unlock()
	for _, fn := range jobs {
		fn()
	}
}

func newCache(limit int64) (*Cache[*tex], *[]*tex) {
	var freed []*tex
	var mu sync.Mutex
	c := New(limit, func(img image.Image) (*tex, int64) {
		b := img.Bounds()
		return &tex{w: b.Dx(), h: b.Dy()}, int64(b.Dx() * b.Dy() * 4)
	}, func(t *tex) {
		mu.Lock()
		t.freed = true
		freed = append(freed, t)
		mu.Unlock()
	})
	return c, &freed
}

// sized loads w×h images, counting the loads.
func sized(w, h int, calls *atomic.Int32) Loader {
	return func(context.Context, string) (image.Image, error) {
		calls.Add(1)
		return image.NewRGBA(image.Rect(0, 0, w, h)), nil
	}
}

// ready acquires k on h and pumps u until it is loaded.
func ready(t testing.TB, c *Cache[*tex], h *Handle[*tex], k Key, load Loader, u *ui) *tex {
	t.Helper()
	notified := make(chan struct{}, 1)
	h.Acquire(c, k, load, Waiter{Post: u.post, Notify: func() { notified <- struct{}{} }})
	deadline := time.After(time.Second)
	for {
		if st, tx, _, _ := h.Get(c, k); st != Pending {
			return tx
		}
		u.pump()
		select {
		case <-notified:
		case <-deadline:
			t.Fatalf("%v never loaded", k)
		case <-time.After(time.Millisecond):
		}
	}
}

func TestTwoHoldersShareOneLoad(t *testing.T) {
	t.Parallel()
	c, _ := newCache(1 << 20)
	var calls atomic.Int32
	var u ui
	var a, b Handle[*tex]
	ta := ready(t, c, &a, Key{Src: "x"}, sized(3, 2, &calls), &u)
	tb := ready(t, c, &b, Key{Src: "x"}, sized(3, 2, &calls), &u)
	if ta != tb || calls.Load() != 1 {
		t.Fatalf("two holders: same texture %v, %d loads", ta == tb, calls.Load())
	}
	if ta.w != 3 || ta.h != 2 {
		t.Fatalf("texture %dx%d, want 3x2", ta.w, ta.h)
	}
	// A different Max is a different image.
	var m Handle[*tex]
	ready(t, c, &m, Key{Src: "x", Max: 1}, sized(3, 2, &calls), &u)
	if calls.Load() != 2 {
		t.Fatalf("%d loads, want a second for another Max", calls.Load())
	}
	if _, tm, _, _ := m.Get(c, Key{Src: "x", Max: 1}); tm.w != 1 {
		t.Fatalf("Max 1 gave a texture %d wide", tm.w)
	}
}

func TestReleaseWhileLoadingCancelsAndForgets(t *testing.T) {
	t.Parallel()
	c, _ := newCache(1 << 20)
	cancelled := make(chan struct{})
	var h Handle[*tex]
	h.Acquire(c, Key{Src: "slow"}, func(ctx context.Context, _ string) (image.Image, error) {
		<-ctx.Done()
		close(cancelled)
		return nil, ctx.Err()
	}, Waiter{})
	h.Release()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("the load was not cancelled")
	}
	if c.Has(Key{Src: "slow"}) || c.Len() != 0 {
		t.Fatal("the abandoned load is still cached")
	}
}

func TestFailedLoadIsForgottenOnRelease(t *testing.T) {
	t.Parallel()
	c, _ := newCache(1 << 20)
	var u ui
	var h Handle[*tex]
	ready(t, c, &h, Key{Src: "bad"}, func(context.Context, string) (image.Image, error) {
		return nil, errors.New("nope")
	}, &u)
	if st, _, err, _ := h.Get(c, Key{Src: "bad"}); st != Failed || err == nil {
		t.Fatalf("state %v, %v; want Failed with an error", st, err)
	}
	h.Release()
	if c.Len() != 0 {
		t.Fatal("a failed image is kept, so it can never be tried again")
	}
	// An image with no pixels fails too.
	ready(t, c, &h, Key{Src: "empty"}, func(context.Context, string) (image.Image, error) {
		return image.NewRGBA(image.Rectangle{}), nil
	}, &u)
	if st, _, _, _ := h.Get(c, Key{Src: "empty"}); st != Failed {
		t.Fatalf("an empty image is %v, want Failed", st)
	}
}

func TestIdleImagesAreEvictedOldestFirst(t *testing.T) {
	t.Parallel()
	c, freed := newCache(2 * 400) // two idle 10x10 images
	var calls atomic.Int32
	var u ui
	var h Handle[*tex]
	var shown []*tex
	for _, s := range []string{"a", "b", "c", "d"} {
		shown = append(shown, ready(t, c, &h, Key{Src: s}, sized(10, 10, &calls), &u))
	}
	if c.Has(Key{Src: "a"}) || !c.Has(Key{Src: "b"}) || !c.Has(Key{Src: "c"}) || c.idleSum != 800 {
		t.Fatalf("a=%v b=%v c=%v idle=%d, want a evicted and b, c kept",
			c.Has(Key{Src: "a"}), c.Has(Key{Src: "b"}), c.Has(Key{Src: "c"}), c.idleSum)
	}
	if len(*freed) != 1 || (*freed)[0] != shown[0] {
		t.Fatalf("freed %d textures, want only a's", len(*freed))
	}
	// Back to b: no load.
	before := calls.Load()
	if got := ready(t, c, &h, Key{Src: "b"}, sized(10, 10, &calls), &u); got != shown[1] || calls.Load() != before {
		t.Fatal("a cached image was loaded again")
	}
	// A zero limit keeps nothing idle, but never what is held.
	c.SetLimit(0)
	if shown[1].freed || c.Len() != 1 || c.idleSum != 0 {
		t.Fatalf("after SetLimit(0): held freed %v, %d entries, %d idle bytes", shown[1].freed, c.Len(), c.idleSum)
	}
	h.Release()
	if !shown[1].freed || c.Len() != 0 {
		t.Fatal("released with a zero limit, the image is kept")
	}
}

func TestGetFindsOnlyWhatIsHeld(t *testing.T) {
	t.Parallel()
	c, _ := newCache(1 << 20)
	var calls atomic.Int32
	var u ui
	var h Handle[*tex]
	k := Key{Src: "x"}
	ready(t, c, &h, k, sized(2, 2, &calls), &u)
	if _, _, _, ok := h.Get(c, k); !ok {
		t.Fatal("Get misses what was acquired")
	}
	if _, _, _, ok := h.Get(c, Key{Src: "y"}); ok {
		t.Fatal("Get finds a key never acquired")
	}
	if h.entry == nil || h.entry.key != k {
		t.Fatal("the handle holds another entry")
	}
	h.Release()
	if _, _, _, ok := h.Get(c, k); ok {
		t.Fatal("a released handle still holds")
	}
	if h.entry != nil {
		t.Fatal("a released handle has an entry")
	}
}

func TestDownscaleKeepsTheAspect(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ w, h, side, ww, wh int }{
		{100, 50, 10, 10, 5},
		{50, 100, 10, 5, 10},
		{8, 4, 10, 8, 4}, // already small
		{1000, 1, 10, 10, 1},
	} {
		b := Downscale(image.NewRGBA(image.Rect(0, 0, tt.w, tt.h)), tt.side).Bounds()
		if b.Dx() != tt.ww || b.Dy() != tt.wh {
			t.Errorf("Downscale(%dx%d, %d) = %dx%d, want %dx%d", tt.w, tt.h, tt.side, b.Dx(), b.Dy(), tt.ww, tt.wh)
		}
	}
}

// Many holders taking and letting go of images at once, from goroutines,
// as widgets coming and going on several windows would: run with -race.
func TestConcurrentHolders(t *testing.T) {
	t.Parallel()
	c, _ := newCache(10 * 64)
	var calls atomic.Int32
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			var u ui
			var h Handle[*tex]
			for i := range 50 {
				ready(t, c, &h, Key{Src: fmt.Sprint((g + i) % 20)}, sized(4, 4, &calls), &u)
			}
			h.Release()
		})
	}
	wg.Wait()
	if c.idleSum > 10*64 {
		t.Fatalf("%d idle bytes over a limit of %d", c.idleSum, 10*64)
	}
}

// A widget showing an image it showed before, the cache's common case: no
// load, only the hold moving.
func BenchmarkAcquireCached(b *testing.B) {
	c, _ := newCache(1 << 30)
	var calls atomic.Int32
	var u ui
	var h Handle[*tex]
	keys := []Key{{Src: "a"}, {Src: "b"}}
	for _, k := range keys {
		ready(b, c, &h, k, sized(8, 8, &calls), &u)
	}
	load := sized(8, 8, &calls)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		h.Acquire(c, keys[i&1], load, Waiter{})
		i++
	}
}

// A layout of a widget showing a loaded image: what it holds, found
// again.
func BenchmarkLayoutHeld(b *testing.B) {
	c, _ := newCache(1 << 30)
	var calls atomic.Int32
	var u ui
	var h Handle[*tex]
	k := Key{Src: "a"}
	ready(b, c, &h, k, sized(8, 8, &calls), &u)
	b.ReportAllocs()
	for b.Loop() {
		h.Get(c, k)
	}
}

// Images scrolling past a budget that holds a few: each new one loads,
// uploads and evicts the oldest.
func BenchmarkChurn(b *testing.B) {
	c, _ := newCache(8 * 16 * 16 * 4)
	var calls atomic.Int32
	load := sized(16, 16, &calls)
	var u ui
	var h Handle[*tex]
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		k := Key{Src: fmt.Sprint(i)}
		h.Acquire(c, k, load, Waiter{Post: u.post, Notify: func() {}})
		for {
			if st, _, _, _ := h.Get(c, k); st != Pending {
				break
			}
			u.pump()
		}
		i++
	}
}

func BenchmarkDownscale(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	b.ReportAllocs()
	for b.Loop() {
		Downscale(img, 256)
	}
}
