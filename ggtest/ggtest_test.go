package ggtest_test

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ggtest"
	"github.com/ironpark/ggui/widget"
)

func TestMain(m *testing.M) { ggtest.Main(m) }

// What a probe paints is read back where it painted it, at the scale it
// painted at.
func TestRenderReadsWhatAProbePainted(t *testing.T) {
	red := color.RGBA{R: 0xff, A: 0xff}
	tree := widget.Row(
		widget.Box().Size(20, 10).Fill(red),
		widget.Box().Size(20, 10),
	)
	p := ggui.NewProbe(tree, ggui.Sz(40, 10))
	defer p.Close()
	img := ggtest.Render(p, ggui.Sz(40, 10), 2)
	if b := img.Bounds(); b.Dx() != 80 || b.Dy() != 20 {
		t.Fatalf("rendered %v", b)
	}
	if got := img.RGBAAt(20, 10); !near(got, red) {
		t.Fatalf("the red box reads %v", got)
	}
	if got := img.RGBAAt(60, 10); got.A > 8 {
		t.Fatalf("the empty box reads %v", got)
	}
}

func near(a, b color.RGBA) bool {
	d := func(x, y uint8) bool { return max(x, y)-min(x, y) < 4 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}
