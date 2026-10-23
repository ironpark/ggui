package ggui

import (
	"image"
	"math"
	"testing"

	"github.com/ironpark/ggfx"
)

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
	if c.fs().layerDepth != 0 || layerCount(c.root()) != 0 {
		t.Fatal("a transform with nothing to show borrowed a layer")
	}
	c.Transform(Affine{}.Translate(30, 0), func(dst *Canvas) { tg.Paint(dst, Rct(Pt(0, 0), Sz(10, 10))) })
	if c.fs().layerDepth != 0 || layerCount(c.root()) != 1 {
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
