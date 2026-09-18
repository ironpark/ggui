package ggui

import (
	"image/color"
	"testing"
)

func TestPathScaleCacheInvalidation(t *testing.T) {
	var p Path
	p.MoveTo(2, 3)
	p.LineTo(12, 3)
	p.LineTo(12, 13)
	p.Close()
	a := p.device(2)
	if a.Bounds().Min.X != 4 || a.Bounds().Max.Y != 26 {
		t.Fatalf("scaled bounds %v", a.Bounds())
	}
	if p.device(2) != a {
		t.Fatal("scale cache not reused")
	}
	p.Reset()
	p.MoveTo(0, 0)
	p.QuadTo(5, 10, 10, 0)
	p.CubicTo(10, 5, 20, 5, 20, 0)
	p.Close()
	if p.device(2).Bounds().Max.X != 40 {
		t.Fatal("mutation did not invalidate scale cache")
	}
	var canvas *Canvas
	canvas.FillPath(&p, color.White)
	canvas.StrokePath(&p, 2, color.White)
	canvas.FillPathGradient(&p, Rct(Point{}, Sz(20, 20)), color.White, color.Black)
}
