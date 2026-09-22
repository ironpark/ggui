package ggui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestVisiblePaintBounds(t *testing.T) {
	img := ebiten.NewImage(100, 100)
	defer img.Deallocate()
	c := (&Canvas{Image: img, scale: 2}).Clip(Rct(Pt(10, 10), Sz(20, 20)))
	for _, tc := range []struct {
		name  string
		r     Rect
		extra float64
		want  bool
	}{
		{"inside", Rct(Pt(12, 12), Sz(4, 4)), 0, true},
		{"outside", Rct(Pt(35, 12), Sz(4, 4)), 0, false},
		{"stroke reaches clip", Rct(Pt(31, 12), Sz(4, 4)), 2, true},
		{"antialias edge", Rct(Pt(30.25, 12), Sz(4, 4)), 0, true},
		{"negative dimensions fallback", Rct(Pt(40, 12), Sz(-30, 4)), 0, true},
		{"nonfinite fallback", Rct(Pt(math.NaN(), 12), Sz(4, 4)), 0, true},
		{"large float32 fallback", Rct(Pt(1<<24, 12), Sz(4, 4)), 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.visiblePaintBounds(tc.r, tc.extra); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	empty := c.Clip(Rct(Pt(80, 80), Sz(1, 1)))
	if empty.visiblePaintBounds(Rct(Point{}, Sz(100, 100)), 0) {
		t.Fatal("empty clip draws")
	}
}

func TestEmptyClipRetainsTextSemanticsAndTrace(t *testing.T) {
	img := ebiten.NewImage(100, 100)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	inspectorEnabled = true
	c.fs().tracing = true
	txt := Text("offscreen text")
	size := txt.Layout(Tight(Sz(100, 20)), rootEnv())
	clipped := c.Clip(Rct(Pt(200, 200), Sz(10, 10)))
	clipped.Paint(txt, Rct(Pt(200, 200), size))
	if len(c.fs().sem) != 1 || c.fs().sem[0].node.Name != "offscreen text" {
		t.Fatal("drawing cull lost semantics")
	}
	// The trace is collected only once something asked for frames.
	if len(c.fs().trace) != 1 {
		t.Fatal("drawing cull lost inspection")
	}
}

func BenchmarkOffscreenPaintBounds(b *testing.B) {
	img := ebiten.NewImage(100, 100)
	defer img.Deallocate()
	c := &Canvas{Image: img}
	r := Rct(Pt(0, 500), Sz(120, 40))
	b.ReportAllocs()
	for b.Loop() {
		c.visiblePaintBounds(r, 0)
	}
}
