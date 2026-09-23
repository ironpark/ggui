package ggui

import (
	"image"
	"testing"
	"time"

	"github.com/ironpark/ggfx"
)

func layerCount(c *Canvas) int {
	if c.frame == nil {
		return 0
	}
	return len(c.frame.layers)
}

func TestLayerSiblingsShareOneImage(t *testing.T) {
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
	if c.frame.layerDepth != 0 {
		t.Fatalf("depth %d after the layers returned; want 0", c.frame.layerDepth)
	}
	c.freeLayers()
}

func TestLayerFrameEndKeepsOnlyWhatTheFrameUsed(t *testing.T) {
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
	img := ggfx.NewImage(100, 80)
	defer img.Deallocate()
	root := &Canvas{Image: img}
	clipped := root.Clip(Rct(Pt(10, 10), Sz(20, 20))).Inert()
	clipped.Layer(LayerOptions{}, func(l *Canvas) {
		if !l.inert || !l.clipped || l.clip != clipped.clip || l.root() != root || l.frame != nil {
			t.Fatalf("layer canvas lost the caller's state: inert %v clipped %v clip %v", l.inert, l.clipped, l.clip)
		}
	})
	root.freeLayers()
}

func TestLayerWithoutAnImagePaintsIntoTheCaller(t *testing.T) {
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
	img := ggfx.NewImage(100, 80)
	defer img.Deallocate()
	c := &Canvas{Image: img, scale: 2}
	r := Rct(Pt(10, 5), Sz(20, 30))
	var got *Canvas
	c.ClipRoundRect(r, 10, func(l *Canvas) { got = l })
	if got.Image == img || got.Image.Bounds() != image.Rect(20, 10, 60, 70) {
		t.Fatalf("round clip painted into %v; want a layer over the rect's pixels", got.Image.Bounds())
	}
	if !got.clipped || got.clip != r || layerCount(c) != 1 || c.frame.layerDepth != 0 {
		t.Fatalf("round clip canvas clip %v (%v) with %d pooled; want %v and one returned layer", got.clip, got.clipped, layerCount(c), r)
	}
	c.freeLayers()
}

func TestClipRoundRectWithoutARadiusOrImageIsClip(t *testing.T) {
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

// A shader that fails to compile panics on its first use, which for these
// is the first rounded image or clip painted.
func TestRoundedShadersCompile(t *testing.T) {
	sharedRoundRectMask()
	sharedRoundRectImage()
}
