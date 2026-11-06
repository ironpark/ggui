package ggui

import (
	"image"
	"image/color"
	"testing"

	"github.com/ironpark/ggfx"
)

// probeFrames returns a Probe painting w into an image of its own, a few
// frames in, so that what each later frame allocates is the frame's own.
func probeFrames(w Widget) *Probe {
	p := NewProbe(w, Sz(64, 64))
	p.RenderTo(ggfx.NewImage(64, 64))
	for range 4 {
		p.Frame()
	}
	return p
}

// A rotating Transform paints through a layer each frame, under a new
// matrix: what a spinner costs a frame.
func BenchmarkRotatingTransformFrame(b *testing.B) {
	angle := State(0.0)
	p := probeFrames(Transform(Box().Size(32, 32).Fill(color.White)).BindRotate(angle))
	defer p.Close()
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		angle.Set(float64(i%64) * 0.1)
		p.Frame()
	}
}

// A half-transparent subtree paints through a layer each frame.
func BenchmarkOpacityLayerFrame(b *testing.B) {
	p := probeFrames(Opacity(Box().Size(32, 32).Fill(color.White), 0.5))
	defer p.Close()
	b.ReportAllocs()
	for b.Loop() {
		p.Frame()
	}
}

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
	w, screen := drawnWindow(Box().Size(10, 10))
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

// A scroll's thumb gives the same ID frame after frame without boxing it
// anew, and a new one once what it is made of changes.
func TestScrollThumbHitIDAllocatesNothing(t *testing.T) {
	s := Scroll(Box())
	s.rect = Rct(Pt(0, 0), Sz(10, 10))
	h := scrollThumb{s}
	first := h.HitID()
	if n := testing.AllocsPerRun(100, func() { _ = h.HitID() }); n != 0 {
		t.Fatalf("the thumb's HitID allocates %v times", n)
	}
	s.rect = Rct(Pt(0, 0), Sz(20, 10))
	if moved := h.HitID(); moved == first || moved != (struct{ Thumb Rect }{s.rect}) {
		t.Fatalf("a moved thumb's ID is %v, was %v", moved, first)
	}
	s.Key("k")
	if keyed := h.HitID(); keyed != (struct{ Thumb any }{"k"}) {
		t.Fatalf("a keyed thumb's ID is %v", keyed)
	}
	if n := testing.AllocsPerRun(100, func() { _ = h.HitID() }); n != 0 {
		t.Fatalf("a keyed thumb's HitID allocates %v times", n)
	}
}

// A Scroll stores its viewport in the Env boxed once a frame, for both of
// the keys it goes under, moving or not.
func TestScrollFrameBoxesItsViewportOnce(t *testing.T) {
	painted := map[int]Rect{}
	offset := State(0.0)
	s := Scroll(Column(Box().Size(50, 500))).BindOffset(offset)
	for range 4 {
		frame(s, painted)
	}
	if n := testing.AllocsPerRun(100, func() { frame(s, painted) }); n > 1 {
		t.Fatalf("a still scroll frame allocates %v times; want its viewport boxed once", n)
	}
	i := 0
	if n := testing.AllocsPerRun(100, func() {
		i++
		offset.Set(float64(i % 2 * 5))
		frame(s, painted)
	}); n > 1 {
		t.Fatalf("a moving scroll frame allocates %v times; want its viewport boxed once", n)
	}
}
