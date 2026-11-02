package ggui

import "testing"

func TestClipTakesNoAllocationOnceItsCanvasesAreMade(t *testing.T) {
	c := &Canvas{}
	f := c.fs()
	paint := func() {
		f.resetCanvases()
		for i := range 8 {
			c.Clip(Rct(Pt(float64(i), 0), Sz(10, 10))).Clip(Rct(Point{}, Sz(5, 5)))
		}
	}
	paint()
	if n := testing.AllocsPerRun(100, paint); n != 0 {
		t.Fatalf("a paint's Clips took %v allocations", n)
	}
	if len(f.canvases) != 16 {
		t.Fatalf("%d canvases kept, want the 16 a paint takes", len(f.canvases))
	}
}

func TestAProbesClipsAreHandedOutAgainEachFrame(t *testing.T) {
	p := ProbeBuilder(func() Widget { return twoClips{} }, Sz(20, 20))
	for range 5 {
		p.Frame()
	}
	if n := len(p.canvas.fs().canvases); n != 2 {
		t.Fatalf("%d canvases after 5 frames of 2 Clips each", n)
	}
}

// twoClips paints through two nested Clips.
type twoClips struct{}

func (twoClips) Layout(cs Constraints, _ Env) Size { return cs.Max() }
func (twoClips) Paint(dst *Canvas, r Rect)         { dst.Clip(r).Clip(r) }
