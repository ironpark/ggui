package ggui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

func TestCanvasScaleConvertsToImagePixels(t *testing.T) {
	c := &Canvas{scale: 2}
	if got := c.Px(7.5); got != 15 {
		t.Fatalf("Px(7.5) = %v at scale 2, want 15", got)
	}
	if got := c.physical(Rct(Pt(1.25, 2), Sz(3, 4.5))); got != image.Rect(2, 4, 9, 13) {
		t.Fatalf("physical = %v, want (2,4)-(9,13) rounded outwards", got)
	}
	g := c.Geo(Pt(10, 20))
	x, y := g.Apply(1, 1)
	if x != 22 || y != 42 {
		t.Fatalf("Geo maps (1,1) to (%v,%v), want (22,42)", x, y)
	}
	var zero Canvas
	if zero.Scale() != 1 || (*Canvas)(nil).Scale() != 1 {
		t.Fatal("zero and nil canvases must scale by 1")
	}
}

func TestClipKeepsScale(t *testing.T) {
	c := &Canvas{scale: 2}
	if got := c.Clip(Rct(Pt(0, 0), Sz(10, 10))).Scale(); got != 2 {
		t.Fatalf("clipped canvas scale = %v, want 2", got)
	}
}

func TestTextRasterizesAtScaledSize(t *testing.T) {
	w := Text("x").Size(14)
	face, ok := w.faceAt(2).(*text.GoTextFace)
	if !ok || face.Size != 28 {
		t.Fatalf("face at scale 2 = %+v, want a GoTextFace of size 28", face)
	}
	if w.faceAt(1).(*text.GoTextFace).Size != 14 {
		t.Fatal("layout face must stay at the logical size")
	}
}

func TestFillRectAndTextTolerateNilTargets(t *testing.T) {
	(*Canvas)(nil).FillRect(Rect{}, nil)
	(&Canvas{}).FillRect(Rct(Pt(0, 0), Sz(1, 1)), nil)
	w := Text("x")
	w.Layout(Loose(Sz(100, 100)))
	w.Paint(nil, Rct(Pt(0, 0), Sz(10, 10)))
	w.Paint(&Canvas{}, Rct(Pt(0, 0), Sz(10, 10)))
}
