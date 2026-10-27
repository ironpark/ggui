package ggui

import (
	"image/color"
	"math"
	"testing"
)

func TestPathScaleCacheInvalidation(t *testing.T) {
	t.Parallel()
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

// An arc follows its circle, from the angle it starts at around to the one
// it ends at.
func TestPathArcFollowsTheCircle(t *testing.T) {
	t.Parallel()
	var p Path
	p.Arc(50, 50, 20, -math.Pi/2, math.Pi/2) // the right half, top to bottom
	b := p.Bounds()
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.05 }
	if !near(b.Origin.X, 50) || !near(b.Origin.Y, 30) || !near(b.Size.W, 20) || !near(b.Size.H, 40) {
		t.Fatalf("half arc bounds %v, want the right half of the circle", b)
	}
	for _, poly := range p.polygons() {
		for _, pt := range poly {
			if d := math.Hypot(pt.X-50, pt.Y-50); !near(d, 20) {
				t.Fatalf("point %v is %.3f from the centre, want 20", pt, d)
			}
		}
	}
	p.Reset()
	p.Arc(0, 0, 10, 0, 7) // past a turn: the whole circle
	if b := p.Bounds(); !near(b.Size.W, 20) || !near(b.Size.H, 20) {
		t.Fatalf("full arc bounds %v", b)
	}
}
