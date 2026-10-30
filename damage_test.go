package ggui

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggfx"
)

// breathing reads the time as it paints, and draws past its Rect.
func breathing(reach float64) Widget {
	return FromFuncs(func(c Constraints, _ Env) Size { return c.Constrain(Sz(10, 10)) }, func(dst *Canvas, r Rect) {
		Now()
		dst.FillRect(grow(r, reach), color.White)
	})
}

func TestATimedWidgetLeavesWhereItDrewAsMotion(t *testing.T) {
	t.Parallel()
	still := Box().Size(100, 10).Fill(color.Black)
	p := NewProbe(topLeft(Column(still, Padding(breathing(3), 20))), Sz(200, 200))
	defer p.Close()
	img := ggfx.NewImage(200, 200)
	defer img.Deallocate()
	p.Draw(img)
	f := p.canvas.fs()
	if len(f.motion) != 1 || f.motionLost {
		t.Fatalf("motion %v, lost %v; want the one breathing widget", f.motion, f.motionLost)
	}
	// At (20, 30), 10 square, drawing 3 beyond it, with culling's pixel.
	if m := f.motion[0]; m.Origin.X > 17 || m.Origin.Y > 27 || m.Size.W < 16 || m.Size.H < 16 || m.Size.W > 20 {
		t.Fatalf("motion %v; want about (17, 27) 16×16", m)
	}
}

func TestAContainerOfATimedWidgetIsNotItselfMotion(t *testing.T) {
	t.Parallel()
	p := NewProbe(topLeft(Box(Padding(breathing(0), 40)).Size(300, 300).Fill(color.Black)), Sz(400, 400))
	defer p.Close()
	p.Frame()
	if m := p.canvas.fs().motion; len(m) != 1 || m[0].Size.W > 20 {
		t.Fatalf("motion %v; want the 10×10 widget alone, not its 300×300 box", m)
	}
}

func TestMotionUnderATransformCannotBePlaced(t *testing.T) {
	t.Parallel()
	moved := FromFuncs(func(c Constraints, env Env) Size { return c.Constrain(Sz(50, 50)) }, func(dst *Canvas, r Rect) {
		dst.Transform(Affine{}.Translate(5, 0), func(dst *Canvas) { dst.Paint(breathing(0), r) })
	})
	p := NewProbe(topLeft(moved), Sz(100, 100))
	defer p.Close()
	p.Frame()
	if f := p.canvas.fs(); !f.motionLost {
		t.Fatalf("motion %v under a transform was placed", f.motion)
	}
}

func TestMotionRegionsMergeWhereTheyMeet(t *testing.T) {
	window := Rct(Point{}, Sz(1000, 1000))
	got := motionRegions([]Rect{
		Rct(Pt(10, 10), Sz(10, 10)), Rct(Pt(22, 10), Sz(10, 10)), // close: one
		Rct(Pt(500, 500), Sz(10, 10)), // apart
		Rct(Pt(-50, -50), Sz(10, 10)), // off the window
	}, window)
	if len(got) != 2 {
		t.Fatalf("regions %v; want the two near ones as one, the far one, and none off the window", got)
	}
	var many []Rect
	for i := range mostMotionRegions + 1 {
		many = append(many, Rct(Pt(float64(i)*100, 0), Sz(10, 10)))
	}
	if got := motionRegions(many, window); len(got) != 1 {
		t.Fatalf("%d regions for %d apart; want one around them all", len(got), len(many))
	}
}

func TestAPartialFrameSkipsWhatItsRegionMisses(t *testing.T) {
	f := &frameState{partial: true, damage: Rct(Pt(100, 100), Sz(10, 10))}
	c := &Canvas{}
	if !f.skips(c, Rct(Pt(300, 300), Sz(20, 20))) {
		t.Fatal("a widget far from the region was painted")
	}
	if f.skips(c, Rct(Pt(100+10+damageReach-5, 100), Sz(20, 20))) {
		t.Fatal("a widget whose shadow may reach the region was skipped")
	}
	if f.skips(&Canvas{space: &paintSpace{}}, Rct(Pt(300, 300), Sz(20, 20))) {
		t.Fatal("a widget under a transform, which may be anywhere, was skipped")
	}
	f.partial = false
	if f.skips(c, Rct(Pt(300, 300), Sz(20, 20))) {
		t.Fatal("a whole frame skipped a widget")
	}
}
