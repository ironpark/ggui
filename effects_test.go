package ggui

import (
	"image"
	"image/color"
	"math"
	"testing"
	"time"

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

// topLeft places w at the probe's origin at its own size.
func topLeft(w Widget) Widget { return Align(w).At(0, 0) }

func nearPt(a, b Point) bool { return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9 }

func TestTransformRoutesInputThroughTheInverse(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(40, 20)}
	p := NewProbe(topLeft(Transform(tg).Translate(100, 50)), Sz(300, 200))
	defer p.Close()
	p.Click(Pt(10, 10))
	if len(tg.taps()) != 0 {
		t.Fatal("a click where the child was laid out reached it after it moved")
	}
	p.Click(Pt(110, 60))
	taps := tg.taps()
	if len(taps) != 1 || !nearPt(taps[0], Pt(10, 10)) {
		t.Fatalf("taps %v; want one at the child's own (10, 10)", taps)
	}
	if tg.rect != Rct(Pt(0, 0), Sz(40, 20)) {
		t.Fatalf("child painted at %v; layout should not move", tg.rect)
	}
}

func TestTransformedPressActionTapsInTheChildsCoordinates(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(40, 20)}
	p := NewProbe(topLeft(Transform(tg).Translate(100, 50)), Sz(300, 200))
	defer p.Close()
	node, ok := p.Semantics().Find(RoleButton, "target")
	if !ok || node.Rect != Rct(Pt(100, 50), Sz(40, 20)) {
		t.Fatalf("node %v %v; want it at the moved Rect", ok, node.Rect)
	}
	p.Perform(node.ID, Action{Kind: ActionPress})
	taps := tg.taps()
	if len(taps) != 1 || !nearPt(taps[0], Pt(20, 10)) {
		t.Fatalf("taps %v; want one at the child's own centre", taps)
	}
}

func TestTransformRotationHitsTheTurnedShape(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(100, 20)}
	// A bar 100 wide turned a quarter about its centre (50, 10) stands
	// from (40, -40) to (60, 60).
	p := NewProbe(topLeft(Transform(tg).Rotate(math.Pi/2)), Sz(200, 200))
	defer p.Close()
	p.Click(Pt(90, 10))
	if len(tg.taps()) != 0 {
		t.Fatal("the end of the bar before turning still takes a click")
	}
	p.Click(Pt(50, 55))
	taps := tg.taps()
	if len(taps) != 1 || math.Abs(taps[0].X-95) > 1e-6 || math.Abs(taps[0].Y-10) > 1e-6 {
		t.Fatalf("taps %v; want one at the bar's own (95, 10)", taps)
	}
	f, ok := p.Semantics().Find(RoleButton, "target")
	if !ok {
		t.Fatal("the turned target is missing from the semantics")
	}
	if math.Abs(f.Rect.Size.W-20) > 1e-6 || math.Abs(f.Rect.Size.H-100) > 1e-6 {
		t.Fatalf("semantic bounds %v; want the turned 20x100", f.Rect)
	}
}

func TestTransformBindingsAndZeroScale(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(20, 20)}
	dx := State(Pt(0.0, 0.0))
	scale := State(1.0)
	w := Transform(tg).BindOffset(dx).BindScale(scale).Origin(0, 0)
	p := NewProbe(topLeft(w), Sz(200, 200))
	defer p.Close()
	p.Click(Pt(5, 5))
	if len(tg.taps()) != 1 {
		t.Fatal("untransformed child missed")
	}
	dx.Set(Pt(50.0, 0.0))
	scale.Set(2)
	p.Frame()
	p.Click(Pt(85, 35))
	taps := tg.taps()
	if len(taps) != 2 || !nearPt(taps[1], Pt(17.5, 17.5)) {
		t.Fatalf("taps %v; want the second at (17.5, 17.5) after scaling and moving", taps)
	}
	scale.Set(0)
	p.Frame()
	p.Click(Pt(50, 0))
	if len(tg.taps()) != 2 {
		t.Fatal("a child scaled to nothing took a click")
	}
}

func TestTransformNestsAndPaintsThroughALayer(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(10, 10)}
	inner := Transform(tg).Translate(10, 0)
	outer := Transform(inner).Scale(2).Origin(0, 0)
	p := NewProbe(topLeft(outer), Sz(100, 100))
	defer p.Close()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	p.Draw(img)
	if p.canvas.frameState.layers.Depth() != 0 || layerCount(&p.canvas) != 2 {
		t.Fatalf("depth %d with %d layers; want two nested layers, all returned", p.canvas.frameState.layers.Depth(), layerCount(&p.canvas))
	}
	p.Click(Pt(25, 5))
	taps := tg.taps()
	if len(taps) != 1 || !nearPt(taps[0], Pt(2.5, 2.5)) {
		t.Fatalf("taps %v; want one at (2.5, 2.5) through both transforms", taps)
	}
}

func TestOpacityFadesWithoutChangingLayoutOrInput(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(30, 30)}
	level := State(2.0)
	w := Opacity(tg, 0.5).BindOpacity(level)
	p := NewProbe(topLeft(w), Sz(100, 100))
	defer p.Close()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	p.Draw(img)
	if w.shown != 1 || layerCount(&p.canvas) != 0 {
		t.Fatalf("opacity %v with %d layers; want 1, clamped, and no layer", w.shown, layerCount(&p.canvas))
	}
	level.Set(0.25)
	p.Draw(img)
	if w.shown != 0.25 || layerCount(&p.canvas) != 1 {
		t.Fatalf("opacity %v with %d layers; want 0.25 through one layer", w.shown, layerCount(&p.canvas))
	}
	w.Opacity(0)
	p.Draw(img)
	if layerCount(&p.canvas) != 0 {
		t.Fatalf("an invisible child kept %d layers", layerCount(&p.canvas))
	}
	p.Click(Pt(10, 10))
	if len(tg.taps()) != 1 {
		t.Fatal("an invisible child stopped taking input")
	}
}

func TestClipOvalKeepsInputInsideTheOval(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(100, 100)}
	p := NewProbe(topLeft(ClipOval(tg)), Sz(200, 200))
	defer p.Close()
	img := ggfx.NewImage(200, 200)
	defer img.Deallocate()
	p.Draw(img)
	p.Click(Pt(5, 5))
	if len(tg.taps()) != 0 {
		t.Fatal("a corner outside the oval reached the child")
	}
	p.Click(Pt(50, 50))
	p.Click(Pt(98, 50))
	if len(tg.taps()) != 2 {
		t.Fatalf("taps %v; want the centre and the right edge", tg.taps())
	}
	if p.canvas.frameState.layers.Depth() != 0 {
		t.Fatal("a layer was not returned")
	}
}

func TestClipPathFollowsItsShape(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(100, 100)}
	triangle := func(r Rect) *Path {
		path := &Path{}
		path.MoveTo(r.Origin.X, r.Origin.Y)
		path.LineTo(r.Origin.X+r.Size.W, r.Origin.Y)
		path.LineTo(r.Origin.X, r.Origin.Y+r.Size.H)
		path.Close()
		return path
	}
	p := NewProbe(topLeft(ClipPath(tg, triangle)), Sz(200, 200))
	defer p.Close()
	p.Click(Pt(90, 90))
	if len(tg.taps()) != 0 {
		t.Fatal("a point past the diagonal reached the child")
	}
	p.Click(Pt(10, 10))
	if len(tg.taps()) != 1 {
		t.Fatal("a point inside the triangle missed")
	}
	var c *Canvas
	c.ClipPath(triangle(Rct(Pt(0, 0), Sz(1, 1))), func(*Canvas) {})
	c.ClipOval(Rct(Pt(0, 0), Sz(1, 1)), func(*Canvas) {})
}

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

func TestBoxGradientResolvesThemeColors(t *testing.T) {
	t.Parallel()
	box := Box().Size(40, 20).Gradient(LinearGradient(0, Stops(testColor{}, color.White)...)).Radius(4)
	env := Env{}.With(testColorKey, color.Color(color.RGBA{G: 200, A: 255}))
	box.Layout(Loose(Sz(100, 100)), env)
	if got := box.shownGradient.stops[0].Color; got != (color.RGBA{G: 200, A: 255}) {
		t.Fatalf("painted stop %v; want the Env's color", got)
	}
	img := ggfx.NewImage(50, 50)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	box.Paint(c, Rct(Pt(0, 0), Sz(40, 20)))
	c.FillGradientRect(Rct(Pt(0, 0), Sz(10, 10)), SweepGradient(Stops(color.White, color.Black)...))
	var path Path
	path.MoveTo(0, 0)
	path.LineTo(20, 0)
	path.LineTo(0, 20)
	path.Close()
	c.FillGradientPath(&path, Rct(Pt(0, 0), Sz(20, 20)), RadialGradient(Stops(color.White, color.Black)...))
	c.FillPathGradient(&path, Rct(Pt(0, 0), Sz(20, 20)), color.White, color.Black)
	var none *Canvas
	none.FillGradientRoundRect(Rct(Pt(0, 0), Sz(1, 1)), 1, box.shownGradient)
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

func TestBlurWidgetsPassLayoutAndInputThrough(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(30, 30)}
	sigma := State(3.0)
	blur := Blur(tg, 0).BindSigma(sigma)
	glass := BackdropBlur(nil, 8).Radius(6)
	p := NewProbe(topLeft(Column(blur, Box(glass).Size(40, 10))), Sz(100, 100))
	defer p.Close()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	p.Draw(img)
	if blur.shown != 3 || glass.shown != 8 {
		t.Fatalf("sigmas %v and %v", blur.shown, glass.shown)
	}
	p.Click(Pt(10, 10))
	if len(tg.taps()) != 1 {
		t.Fatal("a blurred child stopped taking input")
	}
	if tg.rect.Size != Sz(30, 30) {
		t.Fatalf("blurred child laid out at %v", tg.rect)
	}
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
func TestShadowDoesNotChangeLayoutOrHits(t *testing.T) {
	t.Parallel()
	b := Box().Size(50, 30).Shadow(ShadowStyle{Blur: 20, Spread: 5, Color: color.Black})
	if got := b.Layout(Loose(Sz(100, 100)), Env{}); got != Sz(50, 30) {
		t.Fatal(got)
	}
	c := &Canvas{}
	b.Paint(c, Rct(Pt(0, 0), Sz(50, 30)))
	if len(c.fs().hits) != 0 {
		t.Fatal("shadow added hits")
	}
	(*Canvas)(nil).Shadow(Rect{}, 0, ShadowStyle{Color: color.Black})
	b.Shadow()
	if len(b.shadows) != 0 {
		t.Fatal("shadow not cleared")
	}
}

// overlayTarget is a target that paints itself through an Overlay, as a
// popup or tooltip places its content beside the Rect it was painted in.
type overlayTarget struct{ target }

func (o *overlayTarget) Paint(dst *Canvas, r Rect) {
	dst.Overlay(func(dst *Canvas) { o.target.Paint(dst, r) })
}

func TestTransformCarriesOverlaysWithIt(t *testing.T) {
	t.Parallel()
	tg := &overlayTarget{target{size: Sz(40, 40)}}
	p := NewProbe(topLeft(Transform(tg).Translate(100, 0)), Sz(200, 200))
	defer p.Close()
	p.Click(Pt(10, 10))
	if len(tg.taps()) != 0 {
		t.Fatal("an overlay under a Transform stayed where its anchor was laid out")
	}
	p.Click(Pt(110, 10))
	if taps := tg.taps(); len(taps) != 1 || !nearPt(taps[0], Pt(10, 10)) {
		t.Fatalf("taps %v; want one at the overlay's own (10, 10), beside the moved anchor", taps)
	}
	gone := &overlayTarget{target{size: Sz(40, 40)}}
	q := NewProbe(topLeft(Transform(gone).Scale(0)), Sz(200, 200))
	defer q.Close()
	q.Click(Pt(20, 20))
	if len(gone.taps()) != 0 {
		t.Fatal("an overlay scheduled under a zero scale took a click")
	}
}

func TestTransformTurnsScrollDeltas(t *testing.T) {
	t.Parallel()
	tg := &target{size: Sz(40, 40)}
	p := NewProbe(topLeft(Transform(tg).Rotate(math.Pi/2).Scale(2)), Sz(200, 200))
	defer p.Close()
	p.Scroll(Pt(20, 20), Pt(0, 10))
	var got []Point
	for _, ev := range tg.events {
		if ev.Kind == PointerScroll {
			got = append(got, ev.Scroll)
		}
	}
	// Down the screen is along the turned child's x, at half the length.
	if len(got) != 1 || !nearPt(Pt(math.Round(got[0].X*1e9)/1e9, math.Round(got[0].Y*1e9)/1e9), Pt(5, 0)) {
		t.Fatalf("scroll deltas %v; want (5, 0) in the child's coordinates", got)
	}
}

func TestDragAndDropOffsetsUnderATransform(t *testing.T) {
	t.Parallel()
	var got []Dropped[int]
	source := DragSource(Box().Size(100, 40), 7)
	zone := DropZone(Box().Size(100, 40), func(d Dropped[int]) { got = append(got, d) })
	p := NewProbe(topLeft(Column(Transform(source).Scale(2).Origin(0, 0), Transform(zone).Translate(50, 0))), Sz(300, 200))
	defer p.Close()
	p.Frame()
	p.Press(Pt(30, 10))
	p.Move(Pt(30, 40))
	p.Move(Pt(120, 60))
	if !source.Dragging() || source.grab != Pt(15, 5) {
		t.Fatalf("dragging %v, grab %v; want the hold at (15, 5) on the source's own coordinates", source.Dragging(), source.grab)
	}
	p.Release(Pt(120, 60))
	if len(got) != 1 || got[0].Pos != Pt(70, 20) {
		t.Fatalf("dropped %+v; want one at (70, 20) in the moved zone's coordinates", got)
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

func TestBoxGradientEqualAssignmentIsANoOp(t *testing.T) {
	t.Parallel()
	box := Box().Gradient(LinearGradient(0, Stops(testColor{}, testColor{})...))
	before := box.props.Version()
	box.Gradient(LinearGradient(0, Stops(testColor{}, testColor{})...))
	if box.props.Version() != before {
		t.Fatal("setting an equal themed gradient asked for a layout")
	}
	box.Gradient(Gradient{})
	if box.props.Version() == before {
		t.Fatal("removing a themed gradient did not ask for a layout")
	}
}

// These tests check that the parity tracks compose: text under paint
// effects, animation values driving paint effects, and selectable text
// inside the scroll track's sticky headers.

func TestRichTextLinkTapsUnderATransform(t *testing.T) {
	t.Parallel()
	taps := 0
	text := RichText(Span("Read the "), Span("guide").OnTap(func() { taps++ }), Span(" first.")).Selectable()
	p := NewProbe(topLeft(Transform(text).Translate(120, 40)), Sz(500, 200))
	defer p.Close()
	p.Frame()
	link, ok := p.FindRole(RoleLink, "guide")
	if !ok {
		t.Fatal("no link")
	}
	if link.Center().X < 120 || link.Center().Y < 40 {
		t.Fatalf("link reported at %v; want its moved on-screen box", link)
	}
	p.Click(link.Center())
	if taps != 1 {
		t.Fatalf("taps %d; want the moved link to take the click", taps)
	}
	if got := text.SelectedText(); got != "" {
		t.Fatalf("a link tap selected %q", got)
	}
}

func TestSelectableTextSelectsUnderAScale(t *testing.T) {
	t.Parallel()
	w := Text("hello world").Selectable()
	p := NewProbe(topLeft(Transform(w).Scale(2).Origin(0, 0)), Sz(600, 100))
	defer p.Close()
	p.Frame()
	local := textAt(w.rich, Rct(Pt(0, 0), Sz(300, 50)), 4) // late in "hello", where an unmapped 2x point lands in "world"
	at := Pt(local.X*2, local.Y*2)
	p.Click(at)
	p.Click(at)
	if got := w.rich.SelectedText(); got != "hello" {
		t.Fatalf("double click under a 2x scale selected %q, want hello", got)
	}
}

func TestTimelineDrivesATransformOffset(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	tl := Timeline(time.Second)
	tg := &target{size: Sz(20, 20)}
	w := Transform(tg).BindOffset(Interpolate(tl, Pt(0, 0), Pt(100, 0), LerpPoint)).Origin(0, 0)
	p := NewProbe(topLeft(w), Sz(300, 100))
	defer p.Close()
	p.Click(Pt(5, 5))
	if len(tg.taps()) != 1 {
		t.Fatal("the child at rest missed its click")
	}
	tl.Seek(0.5)
	p.Frame()
	p.Click(Pt(55, 5))
	if taps := tg.taps(); len(taps) != 2 || !nearPt(taps[1], Pt(5, 5)) {
		t.Fatalf("taps %v; want the second at (5, 5) after the timeline moved the child 50", taps)
	}
}

func TestSelectableTextInAPinnedStickyHeader(t *testing.T) {
	t.Parallel()
	head := Text("alpha beta").Selectable()
	var body Rect
	a := Sticky(head, probe(200, 300, &body))
	offset := State(0.0)
	p := NewProbe(Scroll(Column(a)).BindOffset(offset), Sz(200, 100))
	defer p.Close()
	p.Frame()
	offset.Set(120)
	p.Frame()
	if !a.Pinned() {
		t.Fatal("header did not pin")
	}
	at := textAt(head.rich, Rct(Pt(0, 0), Sz(200, 30)), 7) // inside "beta"
	p.Click(at)
	p.Click(at)
	if got := head.rich.SelectedText(); got != "beta" {
		t.Fatalf("double click on the pinned header selected %q, want beta", got)
	}
}
