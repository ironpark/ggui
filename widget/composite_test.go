package widget

import (
	"image/color"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
)

// The tests here are of the widgets that paint their child through one of
// the Canvas's effects: Transform, Opacity, the blurs and the shape clips,
// and of Box's shadows and gradients; and of widgets that take input
// painted under them.

// tapTarget is a fixed-size widget, described as a button, that records
// the pointer events it is given and takes presses, releases and taps.
type tapTarget struct {
	size   ggui.Size
	events []ggui.PointerEvent
	rect   ggui.Rect
}

func (t *tapTarget) Layout(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(t.size) }
func (t *tapTarget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t.rect = r
	dst.HitPointer(r, t)
	dst.Describe(r, t)
}
func (t *tapTarget) Semantics() (ggui.Role, string) { return ggui.RoleButton, "target" }
func (t *tapTarget) HandlePointer(ev ggui.PointerEvent) bool {
	t.events = append(t.events, ev)
	return ev.Kind == ggui.PointerDown || ev.Kind == ggui.PointerTap || ev.Kind == ggui.PointerUp
}

// taps returns where the target was tapped, in its own coordinates.
func (t *tapTarget) taps() []ggui.Point {
	var out []ggui.Point
	for _, ev := range t.events {
		if ev.Kind == ggui.PointerTap {
			out = append(out, ev.Pos)
		}
	}
	return out
}

func samePt(a, b ggui.Point) bool { return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9 }

// topLeft places w at the probe's origin at its own size.
func topLeft(w ggui.Widget) ggui.Widget { return Align(w).At(0, 0) }

func TestTransformRoutesInputThroughTheInverse(t *testing.T) {
	t.Parallel()
	tg := &tapTarget{size: ggui.Sz(40, 20)}
	p := ggui.NewProbe(topLeft(Transform(tg).Translate(100, 50)), ggui.Sz(300, 200))
	defer p.Close()
	p.Click(ggui.Pt(10, 10))
	if len(tg.taps()) != 0 {
		t.Fatal("a click where the child was laid out reached it after it moved")
	}
	p.Click(ggui.Pt(110, 60))
	taps := tg.taps()
	if len(taps) != 1 || !samePt(taps[0], ggui.Pt(10, 10)) {
		t.Fatalf("taps %v; want one at the child's own (10, 10)", taps)
	}
	if tg.rect != ggui.Rct(ggui.Pt(0, 0), ggui.Sz(40, 20)) {
		t.Fatalf("child painted at %v; layout should not move", tg.rect)
	}
}

func TestTransformedPressActionTapsInTheChildsCoordinates(t *testing.T) {
	t.Parallel()
	tg := &tapTarget{size: ggui.Sz(40, 20)}
	p := ggui.NewProbe(topLeft(Transform(tg).Translate(100, 50)), ggui.Sz(300, 200))
	defer p.Close()
	node, ok := p.Semantics().Find(ggui.RoleButton, "target")
	if !ok || node.Rect != ggui.Rct(ggui.Pt(100, 50), ggui.Sz(40, 20)) {
		t.Fatalf("node %v %v; want it at the moved Rect", ok, node.Rect)
	}
	p.Perform(node.ID, ggui.Action{Kind: ggui.ActionPress})
	taps := tg.taps()
	if len(taps) != 1 || !samePt(taps[0], ggui.Pt(20, 10)) {
		t.Fatalf("taps %v; want one at the child's own centre", taps)
	}
}

func TestTransformRotationHitsTheTurnedShape(t *testing.T) {
	t.Parallel()
	tg := &tapTarget{size: ggui.Sz(100, 20)}
	// A bar 100 wide turned a quarter about its centre (50, 10) stands
	// from (40, -40) to (60, 60).
	p := ggui.NewProbe(topLeft(Transform(tg).Rotate(math.Pi/2)), ggui.Sz(200, 200))
	defer p.Close()
	p.Click(ggui.Pt(90, 10))
	if len(tg.taps()) != 0 {
		t.Fatal("the end of the bar before turning still takes a click")
	}
	p.Click(ggui.Pt(50, 55))
	taps := tg.taps()
	if len(taps) != 1 || math.Abs(taps[0].X-95) > 1e-6 || math.Abs(taps[0].Y-10) > 1e-6 {
		t.Fatalf("taps %v; want one at the bar's own (95, 10)", taps)
	}
	f, ok := p.Semantics().Find(ggui.RoleButton, "target")
	if !ok {
		t.Fatal("the turned target is missing from the semantics")
	}
	if math.Abs(f.Rect.Size.W-20) > 1e-6 || math.Abs(f.Rect.Size.H-100) > 1e-6 {
		t.Fatalf("semantic bounds %v; want the turned 20x100", f.Rect)
	}
}

func TestTransformBindingsAndZeroScale(t *testing.T) {
	t.Parallel()
	tg := &tapTarget{size: ggui.Sz(20, 20)}
	dx := ggui.State(ggui.Pt(0.0, 0.0))
	scale := ggui.State(1.0)
	w := Transform(tg).BindOffset(dx).BindScale(scale).Origin(0, 0)
	p := ggui.NewProbe(topLeft(w), ggui.Sz(200, 200))
	defer p.Close()
	p.Click(ggui.Pt(5, 5))
	if len(tg.taps()) != 1 {
		t.Fatal("untransformed child missed")
	}
	dx.Set(ggui.Pt(50.0, 0.0))
	scale.Set(2)
	p.Frame()
	p.Click(ggui.Pt(85, 35))
	taps := tg.taps()
	if len(taps) != 2 || !samePt(taps[1], ggui.Pt(17.5, 17.5)) {
		t.Fatalf("taps %v; want the second at (17.5, 17.5) after scaling and moving", taps)
	}
	scale.Set(0)
	p.Frame()
	p.Click(ggui.Pt(50, 0))
	if len(tg.taps()) != 2 {
		t.Fatal("a child scaled to nothing took a click")
	}
}

func TestTransformNestsAndPaintsThroughALayer(t *testing.T) {
	t.Parallel()
	tg := &tapTarget{size: ggui.Sz(10, 10)}
	inner := Transform(tg).Translate(10, 0)
	outer := Transform(inner).Scale(2).Origin(0, 0)
	p := ggui.NewProbe(topLeft(outer), ggui.Sz(100, 100))
	defer p.Close()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	p.Draw(img)
	if p.Layers() != 2 {
		t.Fatalf("%d layers; want two nested layers", p.Layers())
	}
	p.Click(ggui.Pt(25, 5))
	taps := tg.taps()
	if len(taps) != 1 || !samePt(taps[0], ggui.Pt(2.5, 2.5)) {
		t.Fatalf("taps %v; want one at (2.5, 2.5) through both transforms", taps)
	}
}

func TestOpacityFadesWithoutChangingLayoutOrInput(t *testing.T) {
	t.Parallel()
	tg := &tapTarget{size: ggui.Sz(30, 30)}
	level := ggui.State(2.0)
	w := Opacity(tg, 0.5).BindOpacity(level)
	p := ggui.NewProbe(topLeft(w), ggui.Sz(100, 100))
	defer p.Close()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	p.Draw(img)
	if w.shown != 1 || p.Layers() != 0 {
		t.Fatalf("opacity %v with %d layers; want 1, clamped, and no layer", w.shown, p.Layers())
	}
	level.Set(0.25)
	p.Draw(img)
	if w.shown != 0.25 || p.Layers() != 1 {
		t.Fatalf("opacity %v with %d layers; want 0.25 through one layer", w.shown, p.Layers())
	}
	w.Opacity(0)
	p.Draw(img)
	if p.Layers() != 0 {
		t.Fatalf("an invisible child kept %d layers", p.Layers())
	}
	p.Click(ggui.Pt(10, 10))
	if len(tg.taps()) != 1 {
		t.Fatal("an invisible child stopped taking input")
	}
}

func TestClipOvalKeepsInputInsideTheOval(t *testing.T) {
	t.Parallel()
	tg := &tapTarget{size: ggui.Sz(100, 100)}
	p := ggui.NewProbe(topLeft(ClipOval(tg)), ggui.Sz(200, 200))
	defer p.Close()
	img := ggfx.NewImage(200, 200)
	defer img.Deallocate()
	p.Draw(img)
	p.Click(ggui.Pt(5, 5))
	if len(tg.taps()) != 0 {
		t.Fatal("a corner outside the oval reached the child")
	}
	p.Click(ggui.Pt(50, 50))
	p.Click(ggui.Pt(98, 50))
	if len(tg.taps()) != 2 {
		t.Fatalf("taps %v; want the centre and the right edge", tg.taps())
	}
}

func TestClipPathFollowsItsShape(t *testing.T) {
	t.Parallel()
	tg := &tapTarget{size: ggui.Sz(100, 100)}
	triangle := func(r ggui.Rect) *ggui.Path {
		path := &ggui.Path{}
		path.MoveTo(r.Origin.X, r.Origin.Y)
		path.LineTo(r.Origin.X+r.Size.W, r.Origin.Y)
		path.LineTo(r.Origin.X, r.Origin.Y+r.Size.H)
		path.Close()
		return path
	}
	p := ggui.NewProbe(topLeft(ClipPath(tg, triangle)), ggui.Sz(200, 200))
	defer p.Close()
	p.Click(ggui.Pt(90, 90))
	if len(tg.taps()) != 0 {
		t.Fatal("a point past the diagonal reached the child")
	}
	p.Click(ggui.Pt(10, 10))
	if len(tg.taps()) != 1 {
		t.Fatal("a point inside the triangle missed")
	}
	var c *ggui.Canvas
	c.ClipPath(triangle(ggui.Rct(ggui.Pt(0, 0), ggui.Sz(1, 1))), func(*ggui.Canvas) {})
	c.ClipOval(ggui.Rct(ggui.Pt(0, 0), ggui.Sz(1, 1)), func(*ggui.Canvas) {})
}

// clipProbe is a widget of size that records the clip it was painted under.
func clipProbe(size ggui.Size, clip *ggui.Rect, clipped *bool) ggui.Widget {
	return ggui.FromFuncs(
		func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(size) },
		func(dst *ggui.Canvas, _ ggui.Rect) { *clip, *clipped = dst.ClipBounds() },
	)
}

func TestClipRoundRectClipsToItsRect(t *testing.T) {
	t.Parallel()
	var clip ggui.Rect
	var clipped bool
	p := ggui.NewProbe(topLeft(Padding(ClipRoundRect(clipProbe(ggui.Sz(60, 40), &clip, &clipped), 100), 10)), ggui.Sz(200, 200))
	defer p.Close()
	img := ggfx.NewImage(200, 200)
	defer img.Deallocate()
	p.Draw(img)
	if !clipped || clip != ggui.Rct(ggui.Pt(10, 10), ggui.Sz(60, 40)) {
		t.Fatalf("child clipped to %v (%v), want its own rect", clip, clipped)
	}
	var c *ggui.Canvas
	c.Paint(ClipRoundRect(Box(), 4), ggui.Rct(ggui.Pt(0, 0), ggui.Sz(1, 1)))
}

func TestBoxGradientResolvesThemeColors(t *testing.T) {
	t.Parallel()
	box := Box().Size(40, 20).Gradient(ggui.LinearGradient(0, ggui.Stops(testColor{}, color.White)...)).Radius(4)
	env := ggui.Env{}.With(testColorKey, color.Color(color.RGBA{G: 200, A: 255}))
	box.Layout(ggui.Loose(ggui.Sz(100, 100)), env)
	want := ggui.LinearGradient(0, ggui.Stops(color.RGBA{G: 200, A: 255}, color.White)...)
	if got := box.deco.shownGradient; !reflect.DeepEqual(got, want) {
		t.Fatalf("painted gradient %+v; want the Env's color in its first stop, %+v", got, want)
	}
	img := ggfx.NewImage(50, 50)
	defer img.Deallocate()
	box.Paint(&ggui.Canvas{Image: img}, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(40, 20)))
}

func TestBlurWidgetsPassLayoutAndInputThrough(t *testing.T) {
	t.Parallel()
	tg := &tapTarget{size: ggui.Sz(30, 30)}
	sigma := ggui.State(3.0)
	blur := Blur(tg, 0).BindSigma(sigma)
	glass := BackdropBlur(nil, 8).Radius(6)
	p := ggui.NewProbe(topLeft(Column(blur, Box(glass).Size(40, 10))), ggui.Sz(100, 100))
	defer p.Close()
	img := ggfx.NewImage(100, 100)
	defer img.Deallocate()
	p.Draw(img)
	if blur.shown != 3 || glass.shown != 8 {
		t.Fatalf("sigmas %v and %v", blur.shown, glass.shown)
	}
	p.Click(ggui.Pt(10, 10))
	if len(tg.taps()) != 1 {
		t.Fatal("a blurred child stopped taking input")
	}
	if tg.rect.Size != ggui.Sz(30, 30) {
		t.Fatalf("blurred child laid out at %v", tg.rect)
	}
}

func TestShadowDoesNotChangeLayoutOrHits(t *testing.T) {
	t.Parallel()
	b := Box().Size(50, 30).Shadow(ggui.ShadowStyle{Blur: 20, Spread: 5, Color: color.Black})
	if got := b.Layout(ggui.Loose(ggui.Sz(100, 100)), ggui.Env{}); got != ggui.Sz(50, 30) {
		t.Fatal(got)
	}
	c := &ggui.Canvas{}
	b.Paint(c, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(50, 30)))
	if c.HitMark() != (ggui.HitMark{}) {
		t.Fatal("shadow added hits")
	}
	(*ggui.Canvas)(nil).Shadow(ggui.Rect{}, 0, ggui.ShadowStyle{Color: color.Black})
	b.Shadow()
	if len(b.deco.shadows) != 0 {
		t.Fatal("shadow not cleared")
	}
}

// overlayTapTarget is a target that paints itself through an Overlay, as a
// popup or tooltip places its content beside the Rect it was painted in.
type overlayTapTarget struct{ tapTarget }

func (o *overlayTapTarget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Overlay(func(dst *ggui.Canvas) { o.tapTarget.Paint(dst, r) })
}

func TestTransformCarriesOverlaysWithIt(t *testing.T) {
	t.Parallel()
	tg := &overlayTapTarget{tapTarget{size: ggui.Sz(40, 40)}}
	p := ggui.NewProbe(topLeft(Transform(tg).Translate(100, 0)), ggui.Sz(200, 200))
	defer p.Close()
	p.Click(ggui.Pt(10, 10))
	if len(tg.taps()) != 0 {
		t.Fatal("an overlay under a Transform stayed where its anchor was laid out")
	}
	p.Click(ggui.Pt(110, 10))
	if taps := tg.taps(); len(taps) != 1 || !samePt(taps[0], ggui.Pt(10, 10)) {
		t.Fatalf("taps %v; want one at the overlay's own (10, 10), beside the moved anchor", taps)
	}
	gone := &overlayTapTarget{tapTarget{size: ggui.Sz(40, 40)}}
	q := ggui.NewProbe(topLeft(Transform(gone).Scale(0)), ggui.Sz(200, 200))
	defer q.Close()
	q.Click(ggui.Pt(20, 20))
	if len(gone.taps()) != 0 {
		t.Fatal("an overlay scheduled under a zero scale took a click")
	}
}

func TestTransformTurnsScrollDeltas(t *testing.T) {
	t.Parallel()
	tg := &tapTarget{size: ggui.Sz(40, 40)}
	p := ggui.NewProbe(topLeft(Transform(tg).Rotate(math.Pi/2).Scale(2)), ggui.Sz(200, 200))
	defer p.Close()
	p.Scroll(ggui.Pt(20, 20), ggui.Pt(0, 10))
	var got []ggui.Point
	for _, ev := range tg.events {
		if ev.Kind == ggui.PointerScroll {
			got = append(got, ev.Scroll)
		}
	}
	// Down the screen is along the turned child's x, at half the length.
	if len(got) != 1 || !samePt(ggui.Pt(math.Round(got[0].X*1e9)/1e9, math.Round(got[0].Y*1e9)/1e9), ggui.Pt(5, 0)) {
		t.Fatalf("scroll deltas %v; want (5, 0) in the child's coordinates", got)
	}
}

func TestDragAndDropOffsetsUnderATransform(t *testing.T) {
	t.Parallel()
	var got []Dropped[int]
	source := DragSource(Box().Size(100, 40), 7)
	zone := DropZone(Box().Size(100, 40), func(d Dropped[int]) { got = append(got, d) })
	p := ggui.NewProbe(topLeft(Column(Transform(source).Scale(2).Origin(0, 0), Transform(zone).Translate(50, 0))), ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	p.Press(ggui.Pt(30, 10))
	p.Move(ggui.Pt(30, 40))
	p.Move(ggui.Pt(120, 60))
	if !source.Dragging() || source.grab != ggui.Pt(15, 5) {
		t.Fatalf("dragging %v, grab %v; want the hold at (15, 5) on the source's own coordinates", source.Dragging(), source.grab)
	}
	p.Release(ggui.Pt(120, 60))
	if len(got) != 1 || got[0].Pos != ggui.Pt(70, 20) {
		t.Fatalf("dropped %+v; want one at (70, 20) in the moved zone's coordinates", got)
	}
}

func TestBoxGradientEqualAssignmentIsANoOp(t *testing.T) {
	t.Parallel()
	box := Box().Gradient(ggui.LinearGradient(0, ggui.Stops(testColor{}, testColor{})...))
	before := box.props.Version()
	box.Gradient(ggui.LinearGradient(0, ggui.Stops(testColor{}, testColor{})...))
	if box.props.Version() != before {
		t.Fatal("setting an equal themed gradient asked for a layout")
	}
	box.Gradient(ggui.Gradient{})
	if box.props.Version() == before {
		t.Fatal("removing a themed gradient did not ask for a layout")
	}
}

func TestRichTextLinkTapsUnderATransform(t *testing.T) {
	t.Parallel()
	taps := 0
	text := RichText(Span("Read the "), Span("guide").OnTap(func() { taps++ }), Span(" first.")).Selectable()
	p := ggui.NewProbe(topLeft(Transform(text).Translate(120, 40)), ggui.Sz(500, 200))
	defer p.Close()
	p.Frame()
	link, ok := p.FindRole(ggui.RoleLink, "guide")
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
	p := ggui.NewProbe(topLeft(Transform(w).Scale(2).Origin(0, 0)), ggui.Sz(600, 100))
	defer p.Close()
	p.Frame()
	local := textAt(&w.rich.w, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(300, 50)), 4) // late in "hello", where an unmapped 2x point lands in "world"
	at := ggui.Pt(local.X*2, local.Y*2)
	p.Click(at)
	p.Click(at)
	if got := w.rich.w.SelectedText(); got != "hello" {
		t.Fatalf("double click under a 2x scale selected %q, want hello", got)
	}
}

func TestTimelineDrivesATransformOffset(t *testing.T) {
	t.Parallel()
	tl := ggui.Timeline(time.Second)
	tg := &tapTarget{size: ggui.Sz(20, 20)}
	w := Transform(tg).BindOffset(ggui.Interpolate(tl, ggui.Pt(0, 0), ggui.Pt(100, 0), ggui.LerpPoint)).Origin(0, 0)
	p := ggui.NewProbe(topLeft(w), ggui.Sz(300, 100))
	defer p.Close()
	p.Click(ggui.Pt(5, 5))
	if len(tg.taps()) != 1 {
		t.Fatal("the child at rest missed its click")
	}
	tl.Seek(0.5)
	p.Frame()
	p.Click(ggui.Pt(55, 5))
	if taps := tg.taps(); len(taps) != 2 || !samePt(taps[1], ggui.Pt(5, 5)) {
		t.Fatalf("taps %v; want the second at (5, 5) after the timeline moved the child 50", taps)
	}
}

func TestSelectableTextInAPinnedStickyHeader(t *testing.T) {
	t.Parallel()
	head := Text("alpha beta").Selectable()
	var body ggui.Rect
	a := Sticky(head, probe(200, 300, &body))
	offset := ggui.State(0.0)
	p := ggui.NewProbe(Scroll(Column(a)).BindOffset(offset), ggui.Sz(200, 100))
	defer p.Close()
	p.Frame()
	offset.Set(120)
	p.Frame()
	if !a.Pinned() {
		t.Fatal("header did not pin")
	}
	at := textAt(&head.rich.w, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(200, 30)), 7) // inside "beta"
	p.Click(at)
	p.Click(at)
	if got := head.rich.w.SelectedText(); got != "beta" {
		t.Fatalf("double click on the pinned header selected %q, want beta", got)
	}
}

// probeFrames returns a Probe painting w into an image of its own, a few
// frames in, so that what each later frame allocates is the frame's own.
func probeFrames(w ggui.Widget) *ggui.Probe {
	p := ggui.NewProbe(w, ggui.Sz(64, 64))
	p.RenderTo(ggfx.NewImage(64, 64))
	for range 4 {
		p.Frame()
	}
	return p
}

// A rotating Transform paints through a layer each frame, under a new
// matrix: what a spinner costs a frame.
func BenchmarkRotatingTransformFrame(b *testing.B) {
	angle := ggui.State(0.0)
	p := probeFrames(Transform(Box().Size(32, 32).Fill(color.White)).BindRotate(angle))
	defer p.Close()
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		angle.Set(float64(i%64) * 0.1)
		p.Frame()
	}
}

// A half-transparent subtree paints through a layer each frame.
func BenchmarkOpacityLayerFrame(b *testing.B) {
	p := probeFrames(Opacity(Box().Size(32, 32).Fill(color.White), 0.5))
	defer p.Close()
	b.ReportAllocs()
	for b.Loop() {
		p.Frame()
	}
}
