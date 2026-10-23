package ggui

import (
	"image/color"
	"math"
	"testing"

	"github.com/ironpark/ggfx"
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

func TestAffineComposesAndInverts(t *testing.T) {
	t.Parallel()
	if !(Affine{}).IsIdentity() || !nearPt(Affine{}.Apply(Pt(3, 4)), Pt(3, 4)) {
		t.Fatal("zero Affine is not the identity")
	}
	turn := Affine{}.Rotate(math.Pi / 2)
	if got := turn.Apply(Pt(1, 0)); !nearPt(got, Pt(0, 1)) {
		t.Fatalf("a quarter turn takes (1, 0) to %v; want (0, 1), clockwise on screen", got)
	}
	m := Affine{}.Scale(2, 3).Translate(10, 0)
	if got := m.Apply(Pt(1, 1)); !nearPt(got, Pt(12, 3)) {
		t.Fatalf("scale then move gives %v; want (12, 3)", got)
	}
	inv, ok := m.Rotate(0.3).Skew(0.2, 0).Invert()
	if !ok {
		t.Fatal("an invertible transform has no inverse")
	}
	p := Pt(5, -7)
	if got := inv.Apply(m.Rotate(0.3).Skew(0.2, 0).Apply(p)); math.Abs(got.X-p.X) > 1e-9 || math.Abs(got.Y-p.Y) > 1e-9 {
		t.Fatalf("inverse round trip gives %v; want %v", got, p)
	}
	if _, ok := (Affine{}).Scale(0, 1).Invert(); ok {
		t.Fatal("a zero scale has an inverse")
	}
	about := Affine{}.Scale(2, 2).About(Pt(10, 10))
	if got := about.Apply(Pt(10, 10)); !nearPt(got, Pt(10, 10)) {
		t.Fatalf("About moved its own centre to %v", got)
	}
	b := turn.Bounds(Rct(Pt(0, 0), Sz(10, 4)))
	if !nearPt(b.Origin, Pt(-4, 0)) || math.Abs(b.Size.W-4) > 1e-9 || math.Abs(b.Size.H-10) > 1e-9 {
		t.Fatalf("turned bounds %v; want (-4,0) 4x10", b)
	}
}

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
	if p.canvas.frame.layerDepth != 0 || layerCount(&p.canvas) != 2 {
		t.Fatalf("depth %d with %d layers; want two nested layers, all returned", p.canvas.frame.layerDepth, layerCount(&p.canvas))
	}
	p.Click(Pt(25, 5))
	taps := tg.taps()
	if len(taps) != 1 || !nearPt(taps[0], Pt(2.5, 2.5)) {
		t.Fatalf("taps %v; want one at (2.5, 2.5) through both transforms", taps)
	}
}

func TestLayerTransformLeavesHitRegions(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(50, 50)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	tg := &target{}
	c.Layer(LayerOptions{Transform: Affine{}.Translate(20, 0)}, func(l *Canvas) { tg.Paint(l, Rct(Pt(0, 0), Sz(10, 10))) })
	if len(c.hits) != 1 || c.hits[0].rect != Rct(Pt(0, 0), Sz(10, 10)) || c.hits[0].space != nil {
		t.Fatalf("hits %+v; a Layer's transform must not move them", c.hits)
	}
	c.freeLayers()
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
	if p.canvas.frame.layerDepth != 0 {
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
	before := q.polygons()
	q.Reset()
	if len(before) != 1 || len(q.polygons()) != 0 {
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
	if got := box.gradient[1].stops[0].Color; got != (color.RGBA{G: 200, A: 255}) {
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
	none.FillGradientRoundRect(Rct(Pt(0, 0), Sz(1, 1)), 1, box.gradient[1])
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
		if painted != 1 || c.fs().layerDepth != 0 {
			t.Fatalf("sigma %v: painted %d times, %d layers open", sigma, painted, c.fs().layerDepth)
		}
	}
	if layerCount(c) == 0 {
		t.Fatal("a blur borrowed no layer")
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
