package ggui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/ironpark/ggfx"
)

func TestShadowShaderCompiles(t *testing.T) {
	if sharedShadow() == nil {
		t.Fatal("missing shader")
	}
	if sharedShadow() != sharedShadow() {
		t.Fatal("shader not shared")
	}
}
func TestShadowGeometryScaleClipAndSpread(t *testing.T) {
	img := ggfx.NewImage(200, 200)
	defer img.Deallocate()
	c := &Canvas{Image: img, scale: 2}
	s := ShadowStyle{Offset: Pt(3, 4), Blur: 5, Spread: 2, Color: color.Black}
	r := Rct(Pt(20, 20), Sz(40, 30))
	g, ok := c.shadowGeometry(r, 8, s)
	if !ok || g.bounds != image.Rect(32, 34, 140, 122) || g.radius != 20 || g.feather != 10 {
		t.Fatalf("scaled geometry: %+v", g)
	}
	clipped := c.Clip(Rct(Pt(25, 25), Sz(20, 20)))
	g, ok = clipped.shadowGeometry(r, 8, s)
	if !ok || g.bounds != image.Rect(50, 50, 90, 90) || g.center != [2]float32{36, 28} {
		t.Fatalf("clipped geometry: %+v", g)
	}
	for _, bad := range []ShadowStyle{{Spread: -30}, {Blur: math.NaN()}, {Offset: Pt(math.Inf(1), 0)}} {
		if _, ok := c.shadowGeometry(r, 8, bad); ok {
			t.Fatal("invalid geometry accepted", bad)
		}
	}
}
func TestShadowDoesNotChangeLayoutOrHits(t *testing.T) {
	b := Box().Size(50, 30).Shadow(ShadowStyle{Blur: 20, Spread: 5, Color: color.Black})
	if got := b.Layout(Loose(Sz(100, 100)), Env{}); got != Sz(50, 30) {
		t.Fatal(got)
	}
	c := &Canvas{}
	b.Paint(c, Rct(Pt(0, 0), Sz(50, 30)))
	if len(c.hits) != 0 {
		t.Fatal("shadow added hits")
	}
	(*Canvas)(nil).Shadow(Rect{}, 0, ShadowStyle{Color: color.Black})
	b.Shadow()
	if len(b.shadows) != 0 {
		t.Fatal("shadow not cleared")
	}
}
