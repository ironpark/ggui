package ggui

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/imgcache"
	"github.com/ironpark/ggui/internal/reactive/goid"
)

func TestImageLayoutKeepsAspect(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(200, 100)
	cases := []struct {
		name string
		w    *ImageWidget
		c    Constraints
		want Size
	}{
		{"natural", Image(img), Loose(Sz(Unbounded, Unbounded)), Sz(200, 100)},
		{"shrinks to width", Image(img), Loose(Sz(100, Unbounded)), Sz(100, 50)},
		{"shrinks to height", Image(img), Loose(Sz(Unbounded, 25)), Sz(50, 25)},
		{"width follows", Image(img).Height(50), Loose(Sz(Unbounded, Unbounded)), Sz(100, 50)},
		{"height follows", Image(img).Width(50), Loose(Sz(Unbounded, Unbounded)), Sz(50, 25)},
		{"fixed", Image(img).Size(30, 30), Loose(Sz(Unbounded, Unbounded)), Sz(30, 30)},
	}
	for _, tc := range cases {
		if got := tc.w.Layout(tc.c, Env{}); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestImagePlacement(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(200, 100)
	box := Rct(Pt(0, 0), Sz(100, 100))
	cases := []struct {
		fit  ImageFit
		want Rect
	}{
		{FitContain, Rct(Pt(0, 25), Sz(100, 50))},
		{FitCover, Rct(Pt(-50, 0), Sz(200, 100))},
		{FitFill, box},
		{FitNone, Rct(Pt(-50, 0), Sz(200, 100))},
	}
	for _, tc := range cases {
		if got := tc.fit.Place(Image(img).natural(), box); got != tc.want {
			t.Errorf("fit %d: %+v, want %+v", tc.fit, got, tc.want)
		}
	}
}

// encodedImages returns a tiny image in each format DecodeImage reads.
func encodedImages(t testing.TB) map[string][]byte {
	t.Helper()
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := range src.Pix {
		src.Pix[i] = byte(i * 17)
	}
	pal := image.NewPaletted(image.Rect(0, 0, 3, 2), color.Palette{color.Black, color.White})
	pal.SetColorIndex(1, 1, 1)
	var p, j, g bytes.Buffer
	if err := png.Encode(&p, src); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&j, src, nil); err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(&g, pal, nil); err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{"png": p.Bytes(), "jpeg": j.Bytes(), "gif": g.Bytes()}
}

// largestFuzzImage bounds the pixels a fuzzed header may ask for: the
// standard decoders allocate what the header says before reading the
// data, so a few bytes could otherwise ask for gigabytes.
const largestFuzzImage = 1 << 20

func FuzzDecodeImage(f *testing.F) {
	for _, data := range encodedImages(f) {
		f.Add(data)
		f.Add(data[:len(data)/2])
	}
	f.Add([]byte{})
	f.Add([]byte("GIF89a"))
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add([]byte{0xff, 0xd8, 0xff})
	f.Add(emptyGIF)
	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, _, cfgErr := image.DecodeConfig(bytes.NewReader(data))
		if cfgErr == nil && cfg.Width*cfg.Height > largestFuzzImage {
			return
		}
		want, _, stdErr := image.Decode(bytes.NewReader(data))
		if stdErr == nil && want.Bounds().Empty() {
			if _, err := DecodeImage(data); err == nil {
				t.Fatal("DecodeImage accepted an image with no pixels")
			}
			return
		}
		img, err := DecodeImage(data)
		if (err == nil) != (stdErr == nil) {
			t.Fatalf("DecodeImage error %v, but image.Decode error %v", err, stdErr)
		}
		if err != nil {
			if img != nil {
				t.Fatalf("DecodeImage returned an image with the error %v", err)
			}
			if !strings.HasPrefix(err.Error(), "ggui: decode image: ") {
				t.Fatalf("error %q does not say what failed", err)
			}
			return
		}
		if got, wantB := img.Bounds(), want.Bounds(); got.Dx() != wantB.Dx() || got.Dy() != wantB.Dy() {
			t.Fatalf("decoded image is %v, want the %v image.Decode reads", got, wantB)
		}
	})
}

// emptyGIF is a well-formed GIF whose one frame is 0x0: image.Decode
// accepts it as an image with no pixels.
var emptyGIF = []byte("GIF89a\x00\x00\x00\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff" +
	"\x2c\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x01\x2c\x00\x3b")

func TestDecodeImageRejectsAnEmptyImage(t *testing.T) {
	t.Parallel()
	if _, err := DecodeImage(emptyGIF); err == nil {
		t.Fatal("DecodeImage of a 0x0 GIF succeeded, want an error: there is nothing to draw")
	}
}

func TestDecodeImageReadsEveryRegisteredFormat(t *testing.T) {
	t.Parallel()
	for name, data := range encodedImages(t) {
		img, err := DecodeImage(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if b := img.Bounds(); b.Dx() != 3 || b.Dy() != 2 {
			t.Errorf("%s decoded to %v, want 3x2", name, b)
		}
	}
	_, err := DecodeImage([]byte("not an image"))
	if !errors.Is(err, image.ErrFormat) {
		t.Fatalf("DecodeImage of text = %v, want an error wrapping image.ErrFormat", err)
	}
}

func TestLoadImageFileReadsTheFileAndReportsWhatFailed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "icon.png")
	if err := os.WriteFile(path, encodedImages(t)["png"], 0o600); err != nil {
		t.Fatal(err)
	}
	img, err := LoadImageFile(path)
	if err != nil || img.Bounds().Dx() != 3 {
		t.Fatalf("LoadImageFile = %v, %v; want the 3x2 image written", img, err)
	}
	if _, err := LoadImageFile(filepath.Join(dir, "missing.png")); !errors.Is(err, fs.ErrNotExist) || !strings.HasPrefix(err.Error(), "ggui: load image: ") {
		t.Fatalf("a missing file gave %v, want a load error wrapping fs.ErrNotExist", err)
	}
	junk := filepath.Join(dir, "junk.png")
	if err := os.WriteFile(junk, []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadImageFile(junk); err == nil || !strings.HasPrefix(err.Error(), "ggui: decode image: ") {
		t.Fatalf("a file that is not an image gave %v, want a decode error", err)
	}
}

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
	if a.cache.Has(imgcache.Key{Src: "slow"}) {
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
	if n := cache.Len(); n != 0 {
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
