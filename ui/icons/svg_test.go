package icons

import (
	"image"
	"image/color"
	"slices"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/framestats"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestSVGCurrentColorAndViewBox(t *testing.T) {
	t.Parallel()
	a, err := Parse([]byte(`<svg viewBox="10 20 20 10" xmlns="http://www.w3.org/2000/svg"><rect x="10" y="20" width="20" height="10" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	img := a.rasterize(40)
	if got := img.RGBAAt(20, 20); got.A != 255 || got.R != 255 {
		t.Fatalf("center coverage %v", got)
	}
	if img.RGBAAt(20, 2).A != 0 || img.RGBAAt(20, 37).A != 0 {
		t.Fatal("non-square SVG was stretched or viewBox origin ignored")
	}
	for _, source := range []string{`<svg>`, `<svg viewBox="0 0 0 24"/>`, `<svg viewBox="0 0 -24 24"/>`} {
		if _, err := Parse([]byte(source)); err == nil {
			t.Fatalf("invalid SVG accepted: %s", source)
		}
	}
}
func TestSetResolutionPrecedenceAndFallback(t *testing.T) {
	t.Parallel()
	a, b, c := &SVG{}, &SVG{}, &SVG{}
	theme := uitheme.Default().Set(SetKey, Set(Map{Check: b, Close: b}))
	env := theme.Apply(ggui.Env{}).With(SetKey, Set(Map{Check: a}))
	fallback := Map{Check: c, Close: c, Search: c}
	for role, want := range map[Role]*SVG{Check: a, Close: b, Search: c, Role("missing"): nil} {
		if got := Resolve(env, fallback, role); got != want {
			t.Fatalf("%s resolved to %p, want %p", role, got, want)
		}
	}
}

func TestSVGImageIsAWhiteMask(t *testing.T) {
	t.Parallel()
	a, err := Parse([]byte(`<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><circle cx="12" cy="12" r="6" stroke="currentColor" fill="none" stroke-width="2"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	img := a.Image(36)
	if img.Bounds().Dx() != 36 || img.Bounds().Dy() != 36 {
		t.Fatalf("image is %v, want 36 square", img.Bounds())
	}
	if got := img.RGBAAt(18, 9); got.A == 0 || got.R != got.A || got.G != got.A || got.B != got.A {
		t.Fatalf("on the ring %v, want white of its coverage", got)
	}
	if img.RGBAAt(18, 18).A != 0 || img.RGBAAt(1, 1).A != 0 {
		t.Fatal("inside or outside the ring is not clear")
	}
	if (*SVG)(nil).Image(36) != nil || a.Image(0) != nil {
		t.Fatal("an image from no icon, or of no size")
	}
}

// An icon whose size animates misses the cache on every frame.
func BenchmarkSVGDrawAnimatedSize(b *testing.B) {
	asset, err := Parse([]byte(squareSVG))
	if err != nil {
		b.Fatal(err)
	}
	img := ggfx.NewImage(256, 256)
	defer img.Deallocate()
	c := &ggui.Canvas{Image: img}
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		s := 100 + float64(i%40)
		i++
		asset.Draw(c, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(s, s)), color.White, 0)
	}
}

// drain frees every raster of the SVGs given.
func drain(svgs ...*SVG) {
	for _, s := range svgs {
		s.mu.Lock()
		cache := slices.Clone(s.cache)
		s.mu.Unlock()
		for _, r := range cache {
			if rasters.take(r) {
				r.free()
			}
		}
	}
}

// Not parallel: other tests rasterize through the same pool, and one
// could take a and grow it before b is taken.
func TestScratchIsClearAndSized(t *testing.T) {
	a := scratchRGBA(20)
	for i := range a.Pix {
		a.Pix[i] = 0xff
	}
	scratch.Put(a)
	b := scratchRGBA(10)
	if b.Bounds() != image.Rect(0, 0, 10, 10) || b.Stride != 40 || len(b.Pix) != 400 {
		t.Fatalf("scratch %v stride %d len %d", b.Bounds(), b.Stride, len(b.Pix))
	}
	for _, v := range b.Pix {
		if v != 0 {
			t.Fatal("a reused scratch image is not clear")
		}
	}
	if b == a && cap(b.Pix) != 1600 {
		t.Fatal("the reused scratch did not keep its buffer")
	}
}

// An SVG keeps eight sizes; every SVG's rasters together keep to the
// budget, the least recently drawn freed first; the Icons gauge holds the
// bytes of the rasters kept. Not parallel: the list and gauge are shared.
func TestSVGRasterBudgetAndGauge(t *testing.T) {
	img := ggfx.NewImage(64, 64)
	defer img.Deallocate()
	c := &ggui.Canvas{Image: img}
	base := framestats.ReadTextures().Icons
	svgs := make([]*SVG, 4)
	for i := range svgs {
		s, err := Parse([]byte(squareSVG))
		if err != nil {
			t.Fatal(err)
		}
		svgs[i] = s
	}
	defer drain(svgs...)
	draw := func(s *SVG, size float64) { s.Draw(c, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(size, size)), color.White, 0) }
	gauge := func() int64 { return framestats.ReadTextures().Icons - base }

	var want int64
	for size := 10; size < 20; size++ {
		draw(svgs[0], float64(size))
	}
	for size := 12; size < 20; size++ {
		want += framestats.ImageBytes(size, size)
	}
	if len(svgs[0].cache) != 8 || gauge() != want {
		t.Fatalf("ten sizes: %d kept, gauge %d want %d", len(svgs[0].cache), gauge(), want)
	}
	drain(svgs[0])
	if gauge() != 0 {
		t.Fatalf("drained: gauge %d", gauge())
	}

	one := framestats.ImageBytes(32, 32)
	rasters.mu.Lock()
	rasters.budget = rasters.bytes + 3*one
	rasters.mu.Unlock()
	defer func() { rasters.mu.Lock(); rasters.budget = 0; rasters.mu.Unlock() }()
	draw(svgs[0], 32)
	draw(svgs[1], 32)
	draw(svgs[2], 32)
	draw(svgs[0], 32) // drawn again: svgs[1] is the least recent now
	draw(svgs[3], 32)
	if len(svgs[1].cache) != 0 {
		t.Fatal("past the budget, the least recently drawn raster was kept")
	}
	for _, i := range []int{0, 2, 3} {
		if len(svgs[i].cache) != 1 {
			t.Fatalf("svg %d lost its raster", i)
		}
	}
	if gauge() != 3*one {
		t.Fatalf("gauge %d, want three rasters' %d", gauge(), 3*one)
	}
	drain(svgs...)
	if gauge() != 0 {
		t.Fatalf("drained: gauge %d", gauge())
	}
}
