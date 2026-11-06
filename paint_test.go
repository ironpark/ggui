package ggui

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/framestats"
)

func TestPathScaleCacheInvalidation(t *testing.T) {
	t.Parallel()
	var p Path
	p.MoveTo(2, 3)
	p.LineTo(12, 3)
	p.LineTo(12, 13)
	p.Close()
	a := p.device(2)
	if a.Bounds().Min.X != 4 || a.Bounds().Max.Y != 26 {
		t.Fatalf("scaled bounds %v", a.Bounds())
	}
	if p.device(2) != a {
		t.Fatal("scale cache not reused")
	}
	p.Reset()
	p.MoveTo(0, 0)
	p.QuadTo(5, 10, 10, 0)
	p.CubicTo(10, 5, 20, 5, 20, 0)
	p.Close()
	if p.device(2).Bounds().Max.X != 40 {
		t.Fatal("mutation did not invalidate scale cache")
	}
	var canvas *Canvas
	canvas.FillPath(&p, color.White)
	canvas.StrokePath(&p, 2, color.White)
	canvas.FillPathGradient(&p, Rct(Point{}, Sz(20, 20)), color.White, color.Black)
}

// An arc follows its circle, from the angle it starts at around to the one
// it ends at.
func TestPathArcFollowsTheCircle(t *testing.T) {
	t.Parallel()
	var p Path
	p.Arc(50, 50, 20, -math.Pi/2, math.Pi/2) // the right half, top to bottom
	b := p.Bounds()
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.05 }
	if !near(b.Origin.X, 50) || !near(b.Origin.Y, 30) || !near(b.Size.W, 20) || !near(b.Size.H, 40) {
		t.Fatalf("half arc bounds %v, want the right half of the circle", b)
	}
	for _, poly := range p.outline.Polygons() {
		for _, pt := range poly {
			if d := math.Hypot(pt.X-50, pt.Y-50); !near(d, 20) {
				t.Fatalf("point %v is %.3f from the centre, want 20", pt, d)
			}
		}
	}
	p.Reset()
	p.Arc(0, 0, 10, 0, 7) // past a turn: the whole circle
	if b := p.Bounds(); !near(b.Size.W, 20) || !near(b.Size.H, 20) {
		t.Fatalf("full arc bounds %v", b)
	}
}

func layerCount(c *Canvas) int {
	if c.frameState == nil {
		return 0
	}
	return c.frameState.layers.Len()
}

func TestLayerSiblingsShareOneImage(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 80)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	var first, second *ggfx.Image
	c.Layer(LayerOptions{}, func(l *Canvas) { first = l.Image })
	c.Layer(LayerOptions{}, func(l *Canvas) { second = l.Image })
	if first == nil || first != second {
		t.Fatalf("siblings borrowed %p and %p; want one image", first, second)
	}
	if first == img || layerCount(c) != 1 {
		t.Fatalf("layer is the window image or the pool has %d images; want a separate one", layerCount(c))
	}
	c.freeLayers()
}

func TestLayerNestedGetsItsOwnImage(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 80)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	var outer, inner *ggfx.Image
	c.Layer(LayerOptions{}, func(l *Canvas) {
		outer = l.Image
		l.Layer(LayerOptions{}, func(l *Canvas) { inner = l.Image })
	})
	if outer == inner || layerCount(c) != 2 {
		t.Fatalf("nested layers borrowed %p and %p with %d pooled; want two images", outer, inner, layerCount(c))
	}
	if c.frameState.layers.Depth() != 0 {
		t.Fatalf("depth %d after the layers returned; want 0", c.frameState.layers.Depth())
	}
	c.freeLayers()
}

func TestLayerFrameEndKeepsOnlyWhatTheFrameUsed(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 80)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	var open func(l *Canvas, n int)
	open = func(l *Canvas, n int) {
		if n > 0 {
			l.Layer(LayerOptions{}, func(l *Canvas) { open(l, n-1) })
		}
	}
	for _, step := range []struct{ depth, want int }{{2, 2}, {1, 1}, {0, 0}, {1, 1}} {
		open(c, step.depth)
		c.trimLayers()
		if got := layerCount(c); got != step.want {
			t.Fatalf("after a frame %d deep, %d images pooled; want %d", step.depth, got, step.want)
		}
	}
	c.freeLayers()
	if layerCount(c) != 0 {
		t.Fatal("freeLayers left images pooled")
	}
}

func TestLayerFollowsTheWindowSize(t *testing.T) {
	t.Parallel()
	small, large := ggfx.NewImage(40, 30), ggfx.NewImage(90, 60)
	defer small.Deallocate()
	defer large.Deallocate()
	c := &Canvas{Image: small}
	var got *ggfx.Image
	c.Layer(LayerOptions{}, func(l *Canvas) { got = l.Image })
	c.Image = large
	c.Layer(LayerOptions{}, func(l *Canvas) { got = l.Image })
	if got.Bounds().Max != large.Bounds().Max || layerCount(c) != 1 {
		t.Fatalf("after a resize the layer covers %v with %d pooled; want %v and 1", got.Bounds().Max, layerCount(c), large.Bounds().Max)
	}
	c.freeLayers()
}

func TestLayerCanvasIsTheCallersButForTheImage(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 80)
	defer img.Deallocate()
	root := &Canvas{Image: img}
	clipped := root.Clip(Rct(Pt(10, 10), Sz(20, 20))).Inert()
	clipped.Layer(LayerOptions{}, func(l *Canvas) {
		if !l.inert || !l.clipped || l.clip != clipped.clip || l.root() != root || l.frameState != root.fs() {
			t.Fatalf("layer canvas lost the caller's state: inert %v clipped %v clip %v, its frame state shared %v", l.inert, l.clipped, l.clip, l.frameState == root.fs())
		}
	})
	root.freeLayers()
}

func TestLayerWithoutAnImagePaintsIntoTheCaller(t *testing.T) {
	t.Parallel()
	c := &Canvas{}
	var got *Canvas
	c.Layer(LayerOptions{}, func(l *Canvas) { got = l })
	if got != c || layerCount(c) != 0 {
		t.Fatalf("paint got %p, pooled %d; want the caller and nothing pooled", got, layerCount(c))
	}
	var nilCanvas *Canvas
	called := false
	nilCanvas.Layer(LayerOptions{}, func(l *Canvas) { called = l == nil })
	if !called {
		t.Fatal("a nil Canvas did not paint with itself")
	}
}

func TestClipRoundRectPaintsIntoALayerCoveringOnlyTheRect(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 80)
	defer img.Deallocate()
	c := &Canvas{Image: img, scale: 2}
	r := Rct(Pt(10, 5), Sz(20, 30))
	var got *Canvas
	c.ClipRoundRect(r, 10, func(l *Canvas) { got = l })
	if got.Image == img || got.Image.Bounds() != image.Rect(20, 10, 60, 70) {
		t.Fatalf("round clip painted into %v; want a layer over the rect's pixels", got.Image.Bounds())
	}
	if !got.clipped || got.clip != r || layerCount(c) != 1 || c.frameState.layers.Depth() != 0 {
		t.Fatalf("round clip canvas clip %v (%v) with %d pooled; want %v and one returned layer", got.clip, got.clipped, layerCount(c), r)
	}
	c.freeLayers()
}

func TestClipRoundRectWithoutARadiusOrImageIsClip(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 80)
	defer img.Deallocate()
	r := Rct(Pt(10, 5), Sz(20, 30))
	for _, tc := range []struct {
		c      *Canvas
		radius float64
	}{{&Canvas{Image: img}, 0}, {&Canvas{}, 0}, {&Canvas{}, 10}} {
		var got *Canvas
		tc.c.ClipRoundRect(r, tc.radius, func(l *Canvas) { got = l })
		if !got.clipped || got.clip != r || layerCount(tc.c) != 0 {
			t.Fatalf("radius %v, image %v: clip %v, %d pooled; want a plain clip to %v", tc.radius, tc.c.Image != nil, got.clip, layerCount(tc.c), r)
		}
	}
}

func TestTransitionFadeHoldsALayerOnlyWhileAnimating(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(120, 80)
	defer img.Deallocate()
	fading := func() *Probe {
		p := NewProbe(Transition(Text("fading")), Sz(120, 80))
		p.RenderTo(img)
		p.Frame()
		if n := layerCount(&p.canvas); n != 1 {
			t.Fatalf("mid-fade the window pools %d images; want 1", n)
		}
		return p
	}
	t.Run("fade ends", func(t *testing.T) {
		p := fading()
		defer p.Close()
		p.Advance(time.Second)
		p.Frame()
		if n := layerCount(&p.canvas); n != 0 {
			t.Fatalf("after the fade the window pools %d images; want 0", n)
		}
	})
	t.Run("closed mid-fade", func(t *testing.T) {
		p := fading()
		p.Close()
		if n := layerCount(&p.canvas); n != 0 {
			t.Fatalf("a closed probe pools %d images; want 0", n)
		}
	})
}

func TestLayerUnderAClipCoversTheClipAlone(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 80)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	clip := c.Clip(Rct(Pt(20, 10), Sz(40, 30)))
	var faded, scaled image.Rectangle
	clip.Layer(LayerOptions{Fade: 0.5}, func(l *Canvas) { faded = l.Image.Bounds() })
	clip.Layer(LayerOptions{Scale: 0.9}, func(l *Canvas) { scaled = l.Image.Bounds() })
	if faded != image.Rect(20, 10, 60, 40) {
		t.Errorf("faded layer under the clip covers %v; want the clip", faded)
	}
	// A scaled layer can draw what was painted outside the clip into it.
	if scaled != img.Bounds() {
		t.Errorf("scaled layer under the clip covers %v; want the window", scaled)
	}
	c.freeLayers()
}

func TestALayerOverAStripTakesAnImageTheSizeOfTheStrip(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(1000, 800)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	bar := func(y float64) *ggfx.Image {
		var got *ggfx.Image
		c.Clip(Rct(Pt(0, y), Sz(1000, 30))).Layer(LayerOptions{}, func(l *Canvas) { got = l.Image })
		return got
	}
	for range 2 {
		top, bottom := bar(0), bar(500)
		if b := top.Bounds(); b.Dy() > 64 || b.Dx() > 1000 {
			t.Fatalf("a layer over a 30-pixel strip covers %v", b)
		}
		if layerCount(c) != 2 || top.Bounds().Overlaps(bottom.Bounds()) {
			t.Fatalf("two strips at one depth: %v and %v with %d pooled; want one image each", top.Bounds(), bottom.Bounds(), layerCount(c))
		}
		// The next frame takes the same images again.
		c.trimLayers()
		if layerCount(c) != 2 {
			t.Fatalf("the frame's end kept %d of the strips' images; want both", layerCount(c))
		}
	}
	c.freeLayers()
}

// A gradient path whose bounds grow and shrink each frame, as one animated
// does, rebuilds its coverage mask every frame.
func BenchmarkPathGradientResize(b *testing.B) {
	img := ggfx.NewImage(800, 800)
	defer img.Deallocate()
	c := &Canvas{Image: img, scale: 2}
	var p Path
	p.MoveTo(0, 0)
	p.LineTo(300, 0)
	p.LineTo(300, 300)
	p.Close()
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		s := 200 + float64(i%40)*2
		i++
		c.FillPathGradient(&p, Rct(Point{}, Sz(s, s)), color.White, color.Black)
	}
}

func triangle(side float64) *Path {
	p := &Path{}
	p.MoveTo(0, 0)
	p.LineTo(side, 0)
	p.LineTo(side, side)
	p.Close()
	return p
}

// A gradient path's mask is kept while the coverage fits it and takes a
// quarter of it at least, made anew past that, and the PathMasks gauge
// holds its bytes. Not parallel: the gauge is the process's.
func TestPathMaskReusesShrinksAndGauges(t *testing.T) {
	img := ggfx.NewImage(800, 800)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	base := framestats.ReadTextures().PathMasks
	p := triangle(700)
	fill := func(side float64) *ggfx.Image {
		t.Helper()
		c.FillPathGradient(p, Rct(Point{}, Sz(side, side)), color.White, color.Black)
		if b := p.gradientMask.Bounds(); b != image.Rect(0, 0, int(side), int(side)) {
			t.Fatalf("coverage of %v: mask part %v", side, b)
		}
		s := p.maskStore.Bounds()
		if got, want := framestats.ReadTextures().PathMasks-base, framestats.ImageBytes(s.Dx(), s.Dy()); got != want {
			t.Fatalf("coverage of %v: gauge %d, mask %v of %d bytes", side, got, s, want)
		}
		return p.maskStore
	}
	first := fill(200)
	if s := first.Bounds(); s.Dx() != 208 || s.Dy() != 208 {
		t.Fatalf("mask %v, want the coverage rounded up to 16", s)
	}
	if fill(190) != first || fill(204) != first || fill(120) != first {
		t.Fatal("a coverage within the mask took a new one")
	}
	grown := fill(300)
	if grown == first {
		t.Fatal("a larger coverage kept the mask")
	}
	if fill(100) == grown {
		t.Fatal("a coverage under a quarter of the mask kept it")
	}
	if s := fill(600).Bounds(); s.Dx() != 640 {
		t.Fatalf("a large mask %v, want rounded up to 64", s)
	}
	p.freeMask()
	if got := framestats.ReadTextures().PathMasks - base; got != 0 {
		t.Fatalf("after freeing, the gauge is %d off", got)
	}
}

// A Path the app drops counts its mask gone once it is collected.
func TestDroppedPathMaskLeavesTheGauge(t *testing.T) {
	img := ggfx.NewImage(200, 200)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	runtime.GC()
	base := framestats.ReadTextures().PathMasks
	func() {
		c.FillPathGradient(triangle(100), Rct(Point{}, Sz(100, 100)), color.White, color.Black)
	}()
	if framestats.ReadTextures().PathMasks == base {
		t.Fatal("the mask was not counted")
	}
	deadline := time.Now().Add(5 * time.Second)
	for framestats.ReadTextures().PathMasks != base {
		if time.Now().After(deadline) {
			t.Fatalf("a dropped path's mask still counted: %d", framestats.ReadTextures().PathMasks-base)
		}
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
}

// Layers count their images' bytes while kept, and nothing once freed.
func TestLayerGaugeBalances(t *testing.T) {
	img := ggfx.NewImage(300, 200)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	base := framestats.ReadTextures().Layers
	c.Layer(LayerOptions{}, func(l *Canvas) {
		l.Layer(LayerOptions{}, func(*Canvas) {})
	})
	if got := framestats.ReadTextures().Layers - base; got != 2*framestats.ImageBytes(300, 200) {
		t.Fatalf("two nested layers over the window: gauge %d", got)
	}
	c.trimLayers()
	c.trimLayers() // a frame with no layers frees them
	if got := framestats.ReadTextures().Layers - base; got != 0 {
		t.Fatalf("after an idle frame the gauge is %d off", got)
	}
	c.Layer(LayerOptions{}, func(*Canvas) {})
	c.freeLayers()
	if got := framestats.ReadTextures().Layers - base; got != 0 {
		t.Fatalf("after freeing the gauge is %d off", got)
	}
}
