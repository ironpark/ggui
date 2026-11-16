package ggui

import (
	"image"
	"testing"

	"github.com/ironpark/ggfx"

	"github.com/ironpark/ggui/internal/gfxtest"
)

// The tests paint with no renderer: see gfxtest.MainHeadless.
func TestMain(m *testing.M) { gfxtest.MainHeadless(m) }

// drawnWindow returns a window that painted w whole a few times.
func drawnWindow(w Widget) (*Window, *ggfx.Image) {
	win := newWindow(nil, nil, Config{}, func() Widget { return w })
	screen := ggfx.NewImage(64, 64)
	for range 4 {
		win.draw(screen, 1, false, func([]image.Rectangle) {})
	}
	return win, screen
}

// A whole frame of a window, damage and all, around a tree that costs
// nothing to paint: what Window.draw itself costs a frame.
func BenchmarkWindowDraw(b *testing.B) {
	w, screen := drawnWindow(sized(10, 10))
	setDamage := func([]image.Rectangle) {}
	b.ReportAllocs()
	for b.Loop() {
		w.draw(screen, 1, false, setDamage)
	}
}

// A layer's, a transform's and an inert paint's Canvases are the frame's,
// as a Clip's are: once the frame has made them, the next makes none.
func TestDerivedCanvasesAllocateNothing(t *testing.T) {
	c := &Canvas{}
	f := c.fs()
	derive := func() {
		f.resetCanvases()
		c.Inert().into(nil).Inert()
	}
	derive()
	if n := testing.AllocsPerRun(100, derive); n != 0 {
		t.Fatalf("deriving Canvases allocates %v times a frame", n)
	}
}
