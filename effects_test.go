package ggui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/framestats"
)

// target is a fixed-size widget that records the pointer events it is given.
type target struct {
	size   Size
	events []PointerEvent
	rect   Rect
}

func (t *target) Layout(c Constraints, _ Env) Size { return c.Constrain(t.size) }
func (t *target) Paint(dst *Canvas, r Rect) {
	t.rect = r
	dst.HitPointer(r, t)
	dst.Describe(r, t)
}
func (t *target) Semantics() (Role, string) { return RoleButton, "target" }
func (t *target) HandlePointer(ev PointerEvent) bool {
	t.events = append(t.events, ev)
	return ev.Kind == PointerDown || ev.Kind == PointerTap || ev.Kind == PointerUp
}

func (t *target) taps() []Point {
	var out []Point
	for _, ev := range t.events {
		if ev.Kind == PointerTap {
			out = append(out, ev.Pos)
		}
	}
	return out
}

func nearPt(a, b Point) bool { return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9 }

func TestPathContainsAndBounds(t *testing.T) {
	t.Parallel()
	var p Path
	if !p.Bounds().Empty() || p.Contains(Pt(0, 0)) {
		t.Fatal("an empty path has area")
	}
	// A square with a square hole wound the other way.
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.LineTo(10, 10)
	p.LineTo(0, 10)
	p.Close()
	p.MoveTo(3, 3)
	p.LineTo(3, 7)
	p.LineTo(7, 7)
	p.LineTo(7, 3)
	p.Close()
	if !p.Contains(Pt(1, 1)) || p.Contains(Pt(5, 5)) || p.Contains(Pt(11, 5)) {
		t.Fatal("nonzero rule: want the ring filled and the hole empty")
	}
	if b := p.Bounds(); b != Rct(Pt(0, 0), Sz(10, 10)) {
		t.Fatalf("bounds %v", b)
	}
	oval := ovalPath(Rct(Pt(0, 0), Sz(20, 10)))
	if !oval.Contains(Pt(10, 5)) || oval.Contains(Pt(1, 1)) || !oval.Contains(Pt(19.5, 5)) {
		t.Fatal("oval outline is off")
	}
	var q Path
	q.MoveTo(0, 0)
	q.QuadTo(10, 20, 20, 0)
	if !q.Contains(Pt(10, 5)) || q.Contains(Pt(10, 11)) {
		t.Fatal("a quadratic curve is not followed")
	}
	before := q.outline.Polygons()
	q.Reset()
	if len(before) != 1 || len(q.outline.Polygons()) != 0 {
		t.Fatal("Reset did not drop the outline, or changed one handed out")
	}
}

func TestGradientStopsAndPlacement(t *testing.T) {
	t.Parallel()
	red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	s := Stops(red, blue, red)
	if len(s) != 3 || s[0].Offset != 0 || s[1].Offset != 0.5 || s[2].Offset != 1 {
		t.Fatalf("Stops %v", s)
	}
	if one := Stops(red); len(one) != 1 || one[0].Offset != 0 {
		t.Fatalf("a single stop %v", one)
	}
	g := LinearGradient(0, GradientStop{1.5, blue}, GradientStop{-1, red})
	if g.stops[0].Color != red || g.stops[0].Offset != 0 || g.stops[1].Offset != 1 {
		t.Fatalf("stops not clamped and sorted: %v", g.stops)
	}
	c := &Canvas{scale: 2}
	r := Rct(Pt(10, 10), Sz(100, 50))
	d := g.device(c, r)
	if !nearPt(d.Start, Pt(20, 70)) || !nearPt(d.End, Pt(220, 70)) {
		t.Fatalf("horizontal line %v -> %v; want across the middle in device pixels", d.Start, d.End)
	}
	diag := LinearGradient(math.Atan2(50, 100), s...).device(&Canvas{}, Rct(Pt(0, 0), Sz(100, 50)))
	if !nearPt(diag.Start, Pt(0, 0)) && math.Hypot(diag.Start.X, diag.Start.Y) > 1e-6 {
		t.Fatalf("a diagonal gradient should start at the corner, starts at %v", diag.Start)
	}
	rad := RadialGradient(s...).Center(0, 0).Radius(1).device(c, r)
	if !nearPt(rad.Start, Pt(20, 20)) || !nearPt(rad.End, Pt(100, 100)) {
		t.Fatalf("radial centre %v radius %v", rad.Start, rad.End)
	}
	sweep := SweepGradient(s...).StartAngle(1).device(c, r)
	if sweep.End.X != 1 || !nearPt(sweep.Start, Pt(120, 70)) {
		t.Fatalf("sweep %v", sweep)
	}
	if !(Gradient{}).IsZero() || g.IsZero() {
		t.Fatal("IsZero")
	}
}

func TestBlurPaintsTheSubtreeOnceAndReturnsItsImages(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(80, 80)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	for _, sigma := range []float64{0, 2, 30} {
		painted := 0
		c.Blur(Rct(Pt(10, 10), Sz(20, 20)), sigma, func(dst *Canvas) {
			painted++
			dst.FillRect(Rct(Pt(10, 10), Sz(20, 20)), color.White)
		})
		c.BackdropBlur(Rct(Pt(10, 10), Sz(20, 20)), 4, sigma)
		c.BackdropBlur(Rct(Pt(10, 10), Sz(20, 20)), 0, sigma)
		if painted != 1 || c.fs().layers.Depth() != 0 {
			t.Fatalf("sigma %v: painted %d times, %d layers open", sigma, painted, c.fs().layers.Depth())
		}
	}
	// The passes take turns with the image blurred, so a blur borrows
	// two at most: the layer or backdrop, and one scratch.
	if n := layerCount(c); n != 2 {
		t.Fatalf("blurs borrowed %d layers; want 2", n)
	}
	c.freeLayers()
}

func TestBackdropBlurNeverReadsTheScreen(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(80, 80)
	defer img.Deallocate()
	// The zero Image, which cannot be a source, stands in for a screen its
	// driver cannot sample.
	c := &Canvas{Image: &ggfx.Image{}, window: true}
	f := c.fs()
	// Reading a screen its driver cannot sample panics; on it the blur must
	// not touch it, only ask for a backbuffer.
	c.BackdropBlur(Rct(Pt(10, 10), Sz(20, 20)), 4, 3)
	if !f.backdrop || layerCount(c) != 0 {
		t.Fatalf("backdrop wanted %v, %d layers borrowed", f.backdrop, layerCount(c))
	}
	// Inside a layer the image read is the layer's, which is allowed.
	f.backdrop = false
	c.Image = img
	c.Layer(LayerOptions{}, func(dst *Canvas) {
		dst.BackdropBlur(Rct(Pt(10, 10), Sz(20, 20)), 4, 3)
	})
	if f.backdrop {
		t.Fatal("a blur inside a layer asked for a backbuffer")
	}
	// On the backbuffer it reads the window's image and keeps asking.
	c.Clip(Rct(Pt(0, 0), Sz(40, 40))).BackdropBlur(Rct(Pt(10, 10), Sz(20, 20)), 4, 3)
	if !f.backdrop {
		t.Fatal("a blur on the backbuffer let it go")
	}
	c.freeLayers()
}

// TestBackdropBlurReadsAReadableScreen paints a blur on a screen its driver
// can sample, an offscreen image standing in for it: the first frame blurs
// it in place, and the window never makes a backbuffer. (What it reads is
// checked on a real Metal screen in ggfx's rendertest.)
func TestBackdropBlurReadsAReadableScreen(t *testing.T) {
	t.Parallel()
	screen := ggfx.NewImage(40, 40)
	defer screen.Deallocate()
	w := &Window{}
	target, _ := w.paintTarget(screen, false)
	if target != screen {
		t.Fatal("a readable screen painted off screen")
	}
	w.aim(screen)
	c := &w.canvas
	if !c.readable() {
		t.Fatal("the Canvas on a readable screen cannot read it")
	}
	// The blur reads the screen this frame: it borrows its scratch layers.
	c.BackdropBlur(Rct(Pt(10, 10), Sz(20, 20)), 4, 3)
	if layerCount(c) == 0 {
		t.Fatal("the blur on a readable screen went without")
	}
	w.presentBackdrop(screen, target)
	// The blur wanted it, and the next frame still goes without a backbuffer.
	if target, _ := w.paintTarget(screen, true); target != screen || w.backbuf != nil || w.backdropTTL != 0 {
		t.Fatal("a readable screen made a backbuffer")
	}
	c.freeLayers()
}

// TestWindowKeepsABackbufferWhileBackdropsPaint is not parallel: it reads
// the backbuffer gauge, which every window adds to.
func TestWindowKeepsABackbufferWhileBackdropsPaint(t *testing.T) {
	before := framestats.ReadTextures().Backbuffer
	screen := ggfx.NewImage(40, 30)
	defer screen.Deallocate()
	w := &Window{}
	target := func() *ggfx.Image { img, _ := w.paintTarget(screen, false); return img }
	if target() != screen {
		t.Fatal("a window with no backdrop painted off screen")
	}
	// A frame on a screen that cannot be read, the zero Image standing in
	// for it, that wanted a blur turns the backbuffer on.
	w.canvas.fs().backdrop = true
	unreadable := &ggfx.Image{}
	w.presentBackdrop(unreadable, unreadable)
	back, kept := w.paintTarget(screen, false)
	if back == screen || back.Bounds() != screen.Bounds() || kept {
		t.Fatalf("backbuffer %v, kept %v, for screen %v; want a new one", back.Bounds(), kept, screen.Bounds())
	}
	if _, kept := w.paintTarget(screen, false); !kept {
		t.Fatal("the backbuffer did not keep the last frame")
	}
	if got, want := framestats.ReadTextures().Backbuffer-before, framestats.ImageBytes(40, 30); got != want {
		t.Fatalf("backbuffer gauged at %d bytes, want %d", got, want)
	}
	// Frames without one count it down and free it.
	w.canvas.fs().backdrop = false
	for range backdropFrames {
		w.presentBackdrop(screen, target())
	}
	if w.backbuf != nil || target() != screen {
		t.Fatal("the backbuffer outlived its backdrops")
	}
	if got := framestats.ReadTextures().Backbuffer; got != before {
		t.Fatalf("backbuffer gauged at %d bytes once freed, want %d", got, before)
	}
}

func TestShadowGeometryScaleClipAndSpread(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(200, 200)
	defer img.Deallocate()
	c := &Canvas{Image: img, scale: 2}
	s := ShadowStyle{Offset: Pt(3, 4), Blur: 5, Spread: 2, Color: color.Black}
	r := Rct(Pt(20, 20), Sz(40, 30))
	g, ok := c.shadowGeometry(r, 8, s)
	if !ok || g.Bounds != image.Rect(32, 34, 140, 122) || g.Radius != 20 || g.Feather != 10 {
		t.Fatalf("scaled geometry: %+v", g)
	}
	clipped := c.Clip(Rct(Pt(25, 25), Sz(20, 20)))
	g, ok = clipped.shadowGeometry(r, 8, s)
	if !ok || g.Bounds != image.Rect(50, 50, 90, 90) || g.Centre != Pt(86, 78) {
		t.Fatalf("clipped geometry: %+v", g)
	}
	for _, bad := range []ShadowStyle{{Spread: -30}, {Blur: math.NaN()}, {Offset: Pt(math.Inf(1), 0)}} {
		if _, ok := c.shadowGeometry(r, 8, bad); ok {
			t.Fatal("invalid geometry accepted", bad)
		}
	}
}

func TestTransformLayerCoversOnlyWhatShows(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	c := (&Canvas{Image: img}).Clip(Rct(Pt(0, 0), Sz(50, 50)))
	inv, _ := Affine{}.Translate(30, 0).Invert()
	area, ok := c.transformArea(inv)
	if !ok || area != image.Rect(0, 0, 22, 52) {
		t.Fatalf("area %v %v; want the clip moved back, a pixel or two to spare, cut to the window", area, ok)
	}
	inv, _ = Affine{}.Translate(500, 0).Invert()
	if _, ok := c.transformArea(inv); ok {
		t.Fatal("a layer moved wholly out of the clip still shows")
	}
	tg := &target{}
	c.Transform(Affine{}.Translate(500, 0), func(dst *Canvas) { tg.Paint(dst, Rct(Pt(0, 0), Sz(10, 10))) })
	if c.fs().layers.Depth() != 0 || layerCount(c.root()) != 0 {
		t.Fatal("a transform with nothing to show borrowed a layer")
	}
	c.Transform(Affine{}.Translate(30, 0), func(dst *Canvas) { tg.Paint(dst, Rct(Pt(0, 0), Sz(10, 10))) })
	if c.fs().layers.Depth() != 0 || layerCount(c.root()) != 1 {
		t.Fatal("a visible transform did not paint through one returned layer")
	}
	c.root().freeLayers()
}

// These tests check that the parity tracks compose: text under paint
// effects, animation values driving paint effects, and selectable text
// inside the scroll track's sticky headers.

// FadeEdges' ramp is white coverage, alpha-only.
func TestFadeRampIsAlpha(t *testing.T) {
	if f := fadeRamp().Format(); f != ggfx.PixelFormatAlpha {
		t.Fatalf("fade ramp of format %v, want alpha", f)
	}
}

// A Canvas fills a rectangle, a path and a path's vertical ramp with a
// gradient, and a nil one draws nothing without failing.
func TestCanvasFillsGradientsAndToleratesNone(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(50, 50)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	c.FillGradientRect(Rct(Pt(0, 0), Sz(10, 10)), SweepGradient(Stops(color.White, color.Black)...))
	var path Path
	path.MoveTo(0, 0)
	path.LineTo(20, 0)
	path.LineTo(0, 20)
	path.Close()
	c.FillGradientPath(&path, Rct(Pt(0, 0), Sz(20, 20)), RadialGradient(Stops(color.White, color.Black)...))
	c.FillPathGradient(&path, Rct(Pt(0, 0), Sz(20, 20)), color.White, color.Black)
	var none *Canvas
	none.FillGradientRoundRect(Rct(Pt(0, 0), Sz(1, 1)), 1, LinearGradient(0, Stops(color.White, color.Black)...))
}

// A region painted under a Canvas transform takes the pointer where the
// transform moved it, in its own coordinates, and not where it was laid
// out.
func TestInputFollowsACanvasTransform(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(40, 20)}
	moved := FromFuncs(tg.Layout, func(dst *Canvas, r Rect) {
		dst.Transform(Affine{}.Translate(100, 50), func(dst *Canvas) { tg.Paint(dst, r) })
	})
	p := NewProbe(stack(moved), Sz(300, 200))
	defer p.Close()
	p.Click(Pt(10, 10))
	if len(tg.taps()) != 0 {
		t.Fatal("a click where the target was laid out reached it after it moved")
	}
	p.Click(Pt(110, 60))
	if taps := tg.taps(); len(taps) != 1 || !nearPt(taps[0], Pt(10, 10)) {
		t.Fatalf("taps %v; want one at the target's own (10, 10)", taps)
	}
}

// Transforms nested on a Canvas paint through a layer each, and have given
// every one back once they return.
func TestNestedCanvasTransformsReturnTheirLayers(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	tg := &target{size: Sz(10, 10)}
	c.Transform(Affine{}.Scale(2, 2), func(dst *Canvas) {
		dst.Transform(Affine{}.Translate(10, 0), func(dst *Canvas) { tg.Paint(dst, Rct(Pt(0, 0), Sz(10, 10))) })
	})
	if c.fs().layers.Depth() != 0 || layerCount(c) != 2 {
		t.Fatalf("depth %d with %d layers; want two nested layers, all returned", c.fs().layers.Depth(), layerCount(c))
	}
	c.freeLayers()
}

// An oval clip paints through layers and has given them back once it
// returns.
func TestClipOvalReturnsItsLayer(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	c.ClipOval(Rct(Pt(0, 0), Sz(100, 100)), func(dst *Canvas) { dst.FillRect(Rct(Pt(0, 0), Sz(100, 100)), color.White) })
	if c.fs().layers.Depth() != 0 || layerCount(c) == 0 {
		t.Fatalf("depth %d with %d layers; want layers, all returned", c.fs().layers.Depth(), layerCount(c))
	}
	c.freeLayers()
}

// FadeEdges paints through a layer of r, not of all its canvas holds: a list
// fading out at its foot, painted on the window's canvas, took a layer of
// the whole window.
func TestFadeEdgesLayerCoversR(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(1000, 800)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	r := Rct(Pt(10, 700), Sz(200, 60))
	var in image.Rectangle
	c.FadeEdges(r, false, 0, 20, func(dst *Canvas) { in = dst.Image.Bounds() })
	if in != c.physical(r) {
		t.Fatalf("painted into %v; want r, %v", in, c.physical(r))
	}
	c.freeLayers()
	// Outside what c holds, it paints into an invisible canvas (nil Image)
	// through no layer so nothing escapes unfaded onto c.Image.
	var onto *ggfx.Image
	c.FadeEdges(Rct(Pt(2000, 0), Sz(10, 10)), false, 0, 20, func(dst *Canvas) { onto = dst.Image })
	if onto != nil || layerCount(c) != 0 {
		t.Fatalf("outside the canvas: painted into image %v, %d layers; want nil and 0", onto != nil, layerCount(c))
	}
}

func TestEffectsEmptyAreaSingularTransformAndPartialBackdrop(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	c := &Canvas{Image: img}

	// Blur with empty area paints with nil Image and borrows no layer.
	var blurImg *ggfx.Image = img
	c.Blur(Rct(Pt(500, 500), Sz(10, 10)), 2, func(dst *Canvas) { blurImg = dst.Image })
	if blurImg != nil || layerCount(c) != 0 {
		t.Fatalf("offscreen Blur: img=%v layers=%d, want nil and 0", blurImg != nil, layerCount(c))
	}

	// Singular Transform paints with nil Image, inert, and borrows no layer.
	var singImg *ggfx.Image = img
	var singInert bool
	c.Transform(Affine{}.Scale(0, 0), func(dst *Canvas) {
		singImg = dst.Image
		singInert = dst.inert
	})
	if singImg != nil || !singInert || layerCount(c) != 0 {
		t.Fatalf("singular Transform: img=%v inert=%v layers=%d", singImg != nil, singInert, layerCount(c))
	}

	// Transform records transformed bounds in parent scope, and unplaced local
	// child bounds do not pollute the placed parent scope.
	f := c.fs()
	f.startMotion()
	child := FromFuncs(func(Constraints, Env) Size { return Sz(10, 10) }, func(dst *Canvas, r Rect) {
		dst.FillRect(r, color.White)
	})
	host := FromFuncs(func(Constraints, Env) Size { return Sz(10, 10) }, func(dst *Canvas, _ Rect) {
		dst.Transform(Affine{}.Translate(40, 50), func(tDst *Canvas) {
			tDst.Paint(child, Rct(Pt(0, 0), Sz(10, 10)))
		})
	})
	f.openScope(c, Rct(Pt(40, 50), Sz(10, 10)), 0)
	host.Paint(c, Rct(Pt(40, 50), Sz(10, 10)))
	bounds := f.scopes[len(f.scopes)-1].bounds
	f.closeScope(host, 0)
	if bounds.Origin.X < 30 || bounds.Origin.Y < 40 {
		t.Fatalf("parent scope bounds polluted by local coords: %+v", bounds)
	}
	c.freeLayers()

	// Partial frame without backdrop must not decrement backdropTTL.
	screen := ggfx.NewImage(40, 30)
	defer screen.Deallocate()
	w := &Window{}
	w.canvas.fs().backdrop = true
	unreadable := &ggfx.Image{}
	w.presentBackdrop(unreadable, unreadable)
	ttl := w.backdropTTL
	w.canvas.fs().backdrop = false
	back, _ := w.paintTarget(screen, false)
	w.finishBackdrop(screen, back, true)
	if w.backdropTTL != ttl {
		t.Fatalf("partial frame decremented backdropTTL from %d to %d", ttl, w.backdropTTL)
	}
	w.freeBackbuf()
}
