package ggui

import (
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
	c.Layer(nil, func(l *Canvas) { first = l.Image })
	c.Layer(nil, func(l *Canvas) { second = l.Image })
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
	c.Layer(nil, func(l *Canvas) {
		outer = l.Image
		l.Layer(nil, func(l *Canvas) { inner = l.Image })
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
			l.Layer(nil, func(l *Canvas) { open(l, n-1) })
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
	c.Layer(nil, func(l *Canvas) { got = l.Image })
	c.Image = large
	c.Layer(nil, func(l *Canvas) { got = l.Image })
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
	clipped.Layer(nil, func(l *Canvas) {
		if !l.inert || !l.clipped || l.clip != clipped.clip || l.root() != root || l.frame != nil {
			t.Fatalf("layer canvas lost the caller's state: inert %v clipped %v clip %v", l.inert, l.clipped, l.clip)
		}
	})
	root.freeLayers()
}

func TestLayerWithoutAnImagePaintsIntoTheCaller(t *testing.T) {
	c := &Canvas{}
	var got *Canvas
	c.Layer(nil, func(l *Canvas) { got = l })
	if got != c || layerCount(c) != 0 {
		t.Fatalf("paint got %p, pooled %d; want the caller and nothing pooled", got, layerCount(c))
	}
	var nilCanvas *Canvas
	called := false
	nilCanvas.Layer(nil, func(l *Canvas) { called = l == nil })
	if !called {
		t.Fatal("a nil Canvas did not paint with itself")
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
