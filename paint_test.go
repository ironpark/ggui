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

// A rounded rectangle fills its Rect except outside the arcs at its
// corners, and its radius stops at half the shorter side.
func TestPathRoundRect(t *testing.T) {
	t.Parallel()
	var p Path
	r := Rct(Pt(10, 20), Sz(100, 40))
	p.RoundRect(r, 10)
	if b := p.Bounds(); b != r {
		t.Fatalf("bounds %v, want %v", b, r)
	}
	for _, pt := range []Point{Pt(60, 40), Pt(11, 40), Pt(60, 21), Pt(14, 24)} {
		if !p.Contains(pt) {
			t.Fatalf("%v should be inside", pt)
		}
	}
	if p.Contains(Pt(11, 21)) || p.Contains(Pt(109, 59)) {
		t.Fatal("a corner outside the arc is inside")
	}

	p.Reset()
	p.RoundRect(r, 100) // a pill: the radius is 20
	if !p.Contains(Pt(30, 40)) || p.Contains(Pt(13, 23)) || !p.Contains(Pt(11, 40)) {
		t.Fatal("pill corners")
	}

	p.Reset()
	p.RoundRect(r, 0)
	if !p.Contains(Pt(10.5, 20.5)) {
		t.Fatal("square corner is not filled")
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

// A gradient path's mask, alpha-only, is kept while the coverage fits it and
// takes a quarter of it at least, made anew past that, and the PathMasks
// gauge holds its bytes, one a pixel. Not parallel: the gauge is the process's.
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
			t.Fatalf("coverage of %v cutting the path: mask part %v", side, b)
		}
		if f := p.maskStore.Format(); f != ggfx.PixelFormatAlpha {
			t.Fatalf("coverage of %v: mask of format %v, want alpha", side, f)
		}
		// An alpha mask is a byte a pixel.
		s := p.maskStore.Bounds()
		if got, want := framestats.ReadTextures().PathMasks-base, int64(s.Dx()*s.Dy()); got != want {
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
	// A path the bounds do not cut takes the mask's part rounded up, the
	// same part for coverages a little apart.
	whole := triangle(590)
	c.FillPathGradient(whole, Rct(Point{}, Sz(600, 600)), color.White, color.Black)
	part := whole.gradientMask
	if b := part.Bounds(); b != image.Rect(0, 0, 640, 640) {
		t.Fatalf("an uncut coverage of 590: mask part %v, want rounded up to 64", b)
	}
	whole.Reset()
	whole.MoveTo(0, 0)
	whole.LineTo(600, 0)
	whole.LineTo(600, 600)
	whole.Close()
	c.FillPathGradient(whole, Rct(Point{}, Sz(600, 600)), color.White, color.Black)
	if whole.gradientMask != part {
		t.Fatalf("an uncut coverage of 600 took part %v, not the one of 590", whole.gradientMask.Bounds())
	}
	whole.freeMask()
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
	// Paths that earlier tests dropped leave the gauge as their cleanups
	// run, which is some time after a collection: wait for it to rest, or
	// one leaving during the test would be taken for this one's.
	base := framestats.ReadTextures().PathMasks
	for still := 0; still < 20; still++ {
		runtime.GC()
		time.Sleep(time.Millisecond)
		if now := framestats.ReadTextures().PathMasks; now != base {
			base, still = now, 0
		}
	}
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

func TestImagePlacement(t *testing.T) {
	t.Parallel()
	natural := Sz(200, 100)
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
		if got := tc.fit.Place(natural, box); got != tc.want {
			t.Errorf("fit %d: %+v, want %+v", tc.fit, got, tc.want)
		}
	}
}

func TestFillRectPixelGridSnapping(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(400, 400)
	defer img.Deallocate()

	for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2} {
		c := &Canvas{Image: img, scale: scale}

		// Adjacent blocks and 1px dividers tiled along X and Y share exact
		// snapped pixel boundaries with no seams and no overlaps.
		edges := []float64{-10, -1, 0, 1, 11, 12, 25.5, 39, 40, 100}
		var prevXEnd, prevYEnd float32
		for i := 0; i+1 < len(edges); i++ {
			r := Rct(Pt(edges[i], edges[i]), Sz(edges[i+1]-edges[i], edges[i+1]-edges[i]))
			x, y, w, h, ok := c.snapFillRect(r)
			if !ok {
				t.Fatalf("scale %g rect %v: snapFillRect failed", scale, r)
			}
			for _, v := range []float32{x, y, w, h} {
				if float64(v) != math.Trunc(float64(v)) {
					t.Fatalf("scale %g rect %v: non-integer snapped value %v", scale, r, v)
				}
			}
			if w < 1 || h < 1 {
				t.Fatalf("scale %g rect %v: snapped size (%v,%v) < 1", scale, r, w, h)
			}
			if i > 0 {
				if x != prevXEnd || y != prevYEnd {
					t.Fatalf("scale %g seam between segments %d and %d: prev end (%v,%v), next origin (%v,%v)",
						scale, i-1, i, prevXEnd, prevYEnd, x, y)
				}
			}
			prevXEnd, prevYEnd = x+w, y+h
			c.FillRect(r, color.White)
		}

		// Sub-pixel positive sizes remain at least 1 physical pixel thick.
		_, _, hw, hh, ok := c.snapFillRect(Rct(Pt(10.3, 20.7), Sz(0.2, 0.1)))
		if !ok || hw != 1 || hh != 1 {
			t.Fatalf("scale %g hairline: ok=%v size=(%v,%v), want (1,1)", scale, ok, hw, hh)
		}
	}

	// A 1px rect centered on 0 stays 1px wide rather than expanding to 2px.
	c1 := &Canvas{Image: img, scale: 1}
	if x, _, w, _, ok := c1.snapFillRect(Rct(Pt(-0.5, 0), Sz(1, 10))); !ok || x != 0 || w != 1 {
		t.Fatalf("rect centered on 0: ok=%v x=%v w=%v, want x=0 w=1", ok, x, w)
	}

	// Zero, negative, NaN and Inf dimensions are rejected cleanly.
	for _, bad := range []Rect{
		Rct(Pt(0, 0), Sz(0, 10)),
		Rct(Pt(0, 0), Sz(10, 0)),
		Rct(Pt(0, 0), Sz(-5, 10)),
		Rct(Pt(0, 0), Sz(10, -5)),
		Rct(Pt(math.NaN(), 0), Sz(10, 10)),
		Rct(Pt(0, 0), Sz(math.NaN(), 10)),
		Rct(Pt(0, 0), Sz(math.Inf(1), 10)),
	} {
		if _, _, _, _, ok := c1.snapFillRect(bad); ok {
			t.Fatalf("expected snapFillRect(%v) to fail", bad)
		}
		c1.FillRect(bad, color.White)
	}
}

func TestStrokeLineAxisAlignedUniformThickness(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(400, 400)
	defer img.Deallocate()

	for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2} {
		c := &Canvas{Image: img, scale: scale}
		for _, logicalW := range []float64{0.5, 1, 1.5, 2} {
			wantThick := float32(max(snapPx(logicalW*scale), 1))
			for step := -4; step <= 40; step++ {
				pos := float64(step) * 0.25

				// Horizontal line at pos.
				hx, hy, hw, hh, ok := c.snapAxisLine(2, 50, pos, logicalW, true)
				if !ok || hh != wantThick {
					t.Fatalf("scale %g w=%g y=%g: ok=%v thick=%v, want %v", scale, logicalW, pos, ok, hh, wantThick)
				}
				// Reversed endpoints match exactly.
				rx, ry, rw, rh, rok := c.snapAxisLine(50, 2, pos, logicalW, true)
				if !rok || rx != hx || ry != hy || rw != hw || rh != hh {
					t.Fatalf("scale %g reversed horizontal mismatch: (%v,%v,%v,%v) vs (%v,%v,%v,%v)",
						scale, rx, ry, rw, rh, hx, hy, hw, hh)
				}
				// Center stays within 0.5 physical pixels of cross*scale.
				centerErr := math.Abs((float64(hy) + float64(hh)/2) - pos*scale)
				if centerErr > 0.5+1e-9 {
					t.Fatalf("scale %g w=%g y=%g: center drift %v > 0.5px", scale, logicalW, pos, centerErr)
				}

				// Vertical line at pos.
				vx, vy, vw, vh, vok := c.snapAxisLine(2, 50, pos, logicalW, false)
				if !vok || vw != wantThick {
					t.Fatalf("scale %g w=%g x=%g: ok=%v thick=%v, want %v", scale, logicalW, pos, vok, vw, wantThick)
				}
				vCenterErr := math.Abs((float64(vx) + float64(vw)/2) - pos*scale)
				if vCenterErr > 0.5+1e-9 {
					t.Fatalf("scale %g w=%g x=%g: vertical center drift %v > 0.5px", scale, logicalW, pos, vCenterErr)
				}
				if float64(vy) != math.Trunc(float64(vy)) || float64(vh) != math.Trunc(float64(vh)) {
					t.Fatalf("scale %g vertical endpoints not integer: y=%v h=%v", scale, vy, vh)
				}
			}

			// Collinear segments sharing a midpoint join without seam or overlap.
			ax, ay, aw, ah, ok1 := c.snapAxisLine(0, 13, 10, logicalW, true)
			bx, by, _, bh, ok2 := c.snapAxisLine(13, 26, 10, logicalW, true)
			if !ok1 || !ok2 || ay != by || ah != bh || ax+aw != bx {
				t.Fatalf("scale %g w=%g collinear join failed: a=(%v,%v,%v,%v) b=(%v,%v,_,%v)",
					scale, logicalW, ax, ay, aw, ah, bx, by, bh)
			}

			c.StrokeLine(Pt(0, 10), Pt(50, 10), logicalW, color.White)
			c.StrokeLine(Pt(10, 0), Pt(10, 50), logicalW, color.White)
		}
	}

	// Degenerate or non-finite lines are ignored cleanly.
	c := &Canvas{Image: img, scale: 1.5}
	if _, _, _, _, ok := c.snapAxisLine(5, 5, 10, 1, true); ok {
		t.Fatal("zero-length line should not draw")
	}
	if _, _, _, _, ok := c.snapAxisLine(0, 10, math.NaN(), 1, true); ok {
		t.Fatal("NaN cross coordinate should not draw")
	}
	c.StrokeLine(Pt(5, 5), Pt(5, 5), 1, color.White)
	c.StrokeLine(Pt(0, 5), Pt(10, 5), math.NaN(), color.White)
}
