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
	ready(t, c, &m, Max("x", 1), sized(3, 2, &calls), &u)
	if calls.Load() != 2 {
		t.Fatalf("%d loads, want a second for another Max", calls.Load())
	}
	if _, tm, _, _ := m.Get(c, Max("x", 1)); tm.w != 1 {
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

// Fit gives the size FitSize says, keeping the aspect, whether it scales
// in one step or, from many times larger, in two.
func TestFitKeepsTheAspect(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		w, h, bw, bh int
		cover        bool
		ww, wh       int
	}{
		{100, 50, 10, 10, false, 10, 5},
		{50, 100, 10, 10, false, 5, 10},
		{8, 4, 10, 10, false, 8, 4}, // already small
		{1000, 1, 10, 10, false, 10, 1},
		{6000, 4000, 512, 512, false, 512, 341}, // in two steps
		{4000, 6000, 512, 512, false, 341, 512},
		{6000, 4000, 512, 512, true, 768, 512},
		{6000, 4000, 1024, 128, true, 1024, 683},
		{3000, 2000, 1000, 1000, false, 1000, 666}, // three times: in one
	} {
		src := image.NewRGBA(image.Rect(0, 0, tt.w, tt.h))
		for i := range src.Pix {
			src.Pix[i] = 0x80
		}
		img := Fit(src, tt.bw, tt.bh, tt.cover)
		b := img.Bounds()
		if b.Dx() != tt.ww || b.Dy() != tt.wh {
			t.Errorf("Fit(%dx%d, %dx%d, cover %v) = %dx%d, want %dx%d",
				tt.w, tt.h, tt.bw, tt.bh, tt.cover, b.Dx(), b.Dy(), tt.ww, tt.wh)
		}
		if w, h := FitSize(tt.w, tt.h, tt.bw, tt.bh, tt.cover); b.Dx() != w || b.Dy() != h {
			t.Errorf("Fit and FitSize disagree: %v, %dx%d", b, w, h)
		}
		if r, g, bl, a := img.At(b.Dx()/2, b.Dy()/2).RGBA(); r>>8 != 0x80 || g>>8 != 0x80 || bl>>8 != 0x80 || a>>8 != 0x80 {
			t.Errorf("Fit(%dx%d) changed a flat image to %x %x %x %x", tt.w, tt.h, r, g, bl, a)
		}
	}
}

func TestFitSize(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		iw, ih, w, h int
		cover        bool
		ww, wh       int
	}{
		{4000, 3000, 512, 512, false, 512, 384},  // inside a square: the larger side
		{4000, 3000, 512, 512, true, 683, 512},   // covering it: the smaller side, rounded up
		{4000, 3000, 512, 0, false, 512, 384},    // only a width to fit
		{3000, 4000, 0, 512, false, 384, 512},    // only a height
		{4000, 3000, 1024, 128, false, 170, 128}, // a wide box: the height limits
		{4000, 3000, 1024, 128, true, 1024, 768}, // and covering, the width
		{4000, 3000, 0, 0, false, 4000, 3000},    // no box
		{4000, 3000, 0, 0, true, 4000, 3000},
		{300, 200, 512, 512, true, 300, 200}, // never enlarged
		{300, 200, 512, 512, false, 300, 200},
	} {
		if w, h := FitSize(tt.iw, tt.ih, tt.w, tt.h, tt.cover); w != tt.ww || h != tt.wh {
			t.Errorf("FitSize(%dx%d, %dx%d, cover %v) = %dx%d, want %dx%d",
				tt.iw, tt.ih, tt.w, tt.h, tt.cover, w, h, tt.ww, tt.wh)
		}
	}
}

// What upload is given is RGBA pixels however the loader decoded them, so
// that the UI thread only copies them; and the holder learns the size the
// image was loaded at.
func TestUploadIsGivenRGBA(t *testing.T) {
	t.Parallel()
	var got []image.Image
	var mu sync.Mutex
	c := New(1<<20, func(img image.Image) (*tex, int64) {
		mu.Lock()
		got = append(got, img)
		mu.Unlock()
		b := img.Bounds()
		return &tex{w: b.Dx(), h: b.Dy()}, int64(b.Dx() * b.Dy() * 4)
	}, func(*tex) {})
	var u ui
	for i, load := range []Loader{
		func(context.Context, string) (image.Image, error) {
			return image.NewYCbCr(image.Rect(0, 0, 40, 30), image.YCbCrSubsampleRatio420), nil
		},
		func(context.Context, string) (image.Image, error) {
			return image.NewNRGBA(image.Rect(0, 0, 40, 30)), nil
		},
		func(context.Context, string) (image.Image, error) {
			// A part of a larger image.
			return image.NewRGBA(image.Rect(0, 0, 80, 60)).SubImage(image.Rect(10, 10, 50, 40)), nil
		},
	} {
		for _, k := range []Key{{Src: fmt.Sprint(i)}, Max(fmt.Sprint(i), 20)} {
			var h Handle[*tex]
			ready(t, c, &h, k, load, &u)
			if s := h.Source(); s != image.Pt(40, 30) {
				t.Errorf("%v: source %v, want 40x30", k, s)
			}
			h.Release()
		}
	}
	if len(got) != 6 {
		t.Fatalf("%d uploads, want 6", len(got))
	}
	for i, img := range got {
		m, ok := img.(*image.RGBA)
		if !ok {
			t.Errorf("upload %d was given %T", i, img)
			continue
		}
		if b := m.Bounds(); len(m.Pix) != 4*b.Dx()*b.Dy() {
			t.Errorf("upload %d: %d bytes of pixels for %v", i, len(m.Pix), b)
		}
	}
}

// AcquireReady holds a ready copy of a src at another size: one pick
// accepts over a larger one, and none once they are evicted.
func TestAcquireReadyHoldsASrcAtAnotherSize(t *testing.T) {
	t.Parallel()
	c, _ := newCache(1 << 20)
	var calls atomic.Int32
	var u ui
	var a, b, other, h Handle[*tex]
	ready(t, c, &a, Max("x", 4), sized(8, 8, &calls), &u)
	ready(t, c, &b, Key{Src: "x"}, sized(8, 8, &calls), &u)
	ready(t, c, &other, Key{Src: "y"}, sized(8, 8, &calls), &u)
	b.Release() // idle, still ready
	none := func(Key) bool { return false }
	if k, ok := h.AcquireReady(c, "x", none); !ok || k != (Key{Src: "x"}) {
		t.Fatalf("AcquireReady(x) = %v, %v, want the larger copy", k, ok)
	}
	if k, ok := h.AcquireReady(c, "x", func(k Key) bool { return k.W == 4 }); !ok || k != Max("x", 4) {
		t.Fatalf("AcquireReady(x, picking 4) = %v, %v", k, ok)
	}
	if st, _, _, ok := h.Get(c, Max("x", 4)); !ok || st != Ready {
		t.Fatalf("held %v, %v", st, ok)
	}
	if _, ok := h.AcquireReady(c, "z", none); ok {
		t.Fatal("AcquireReady found a src never loaded")
	}
	h.Release()
	a.Release()
	c.SetLimit(0)
	if k, ok := h.AcquireReady(c, "x", none); ok {
		t.Fatalf("after eviction AcquireReady(x) = %v", k)
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

// A photo's pixels scaled to a few times smaller, and to many: the
// default AsyncImage decodes a 6000×4000 JPEG shown at a thumbnail's size.
func BenchmarkFit(b *testing.B) {
	for _, bb := range []struct {
		name       string
		w, h, side int
	}{
		{"1024x768to256", 1024, 768, 256},
		{"6000x4000to512", 6000, 4000, 512},
	} {
		b.Run(bb.name, func(b *testing.B) {
			img := image.NewYCbCr(image.Rect(0, 0, bb.w, bb.h), image.YCbCrSubsampleRatio420)
			b.ReportAllocs()
			for b.Loop() {
				Fit(img, bb.side, bb.side, false)
			}
		})
	}
}

// A JPEG's pixels made RGBA, as the loader's goroutine does before upload.
func BenchmarkRGBAFromYCbCr(b *testing.B) {
	img := image.NewYCbCr(image.Rect(0, 0, 1024, 768), image.YCbCrSubsampleRatio420)
	b.ReportAllocs()
	for b.Loop() {
		RGBA(img)
	}
}
