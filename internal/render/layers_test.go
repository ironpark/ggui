package render

import (
	"image"
	"testing"
)

// fakeTex stands in for a GPU image.
type fakeTex struct {
	b     image.Rectangle
	freed bool
}

func (t *fakeTex) Bounds() image.Rectangle { return t.b }
func (t *fakeTex) Deallocate()             { t.freed = true }

func allocFake(b image.Rectangle) *fakeTex { return &fakeTex{b: b} }

var window = image.Rect(0, 0, 1000, 800)

func TestLayerPoolReusesWhatHoldsTheArea(t *testing.T) {
	var p LayerPool[*fakeTex]
	a := p.Borrow(window, image.Rect(10, 10, 50, 40), allocFake)
	if a.b != image.Rect(0, 0, 64, 64) {
		t.Fatalf("bounds %v, want the area rounded out to the grid", a.b)
	}
	p.Return()
	// Siblings at one depth share the image that holds them both.
	if b := p.Borrow(window, image.Rect(20, 20, 60, 60), allocFake); b != a {
		t.Fatal("a sibling within the same grid cell took a new image")
	}
	p.Return()
	// A nested layer takes the next depth.
	p.Borrow(window, image.Rect(0, 0, 10, 10), allocFake)
	if n := p.Borrow(window, image.Rect(0, 0, 10, 10), allocFake); n == a || p.Depth() != 2 {
		t.Fatalf("nested: same image %v, depth %d", n == a, p.Depth())
	}
	p.Return()
	p.Return()
	if p.Depth() != 0 || p.Len() != 2 {
		t.Fatalf("depth %d, %d images; want 0 and 2", p.Depth(), p.Len())
	}
}

// A larger area's image replaces the ones it holds; an empty area is the
// whole window.
func TestLayerPoolGrowsAndReplaces(t *testing.T) {
	var p LayerPool[*fakeTex]
	small := p.Borrow(window, image.Rect(0, 0, 10, 10), allocFake)
	p.Return()
	big := p.Borrow(window, image.Rectangle{}, allocFake)
	p.Return()
	if big.b != window || !small.freed || p.Len() != 1 {
		t.Fatalf("whole window: bounds %v, small freed %v, %d kept", big.b, small.freed, p.Len())
	}
	// Past the window's edge, an area is cut to it.
	p.FreeAll()
	edge := p.Borrow(window, image.Rect(990, 790, 1000, 800), allocFake)
	if edge.b != image.Rect(960, 768, 1000, 800) {
		t.Fatalf("at the edge: %v", edge.b)
	}
}

func TestLayerPoolEndFrameFreesWhatWasNotTaken(t *testing.T) {
	var p LayerPool[*fakeTex]
	left := p.Borrow(window, image.Rect(0, 0, 10, 10), allocFake)
	p.Return()
	right := p.Borrow(window, image.Rect(500, 0, 510, 10), allocFake)
	deep := p.Borrow(window, image.Rect(500, 0, 510, 10), allocFake)
	p.Return()
	p.Return()
	p.EndFrame()
	if left.freed || right.freed || deep.freed || p.Len() != 3 {
		t.Fatal("an image taken this frame was freed")
	}
	// The next frame takes only left, at depth 0.
	p.Borrow(window, image.Rect(0, 0, 10, 10), allocFake)
	p.Return()
	p.EndFrame()
	if left.freed || !right.freed || !deep.freed || p.Len() != 1 {
		t.Fatalf("left freed %v, right %v, deep %v, %d kept", left.freed, right.freed, deep.freed, p.Len())
	}
	p.FreeAll()
	if !left.freed || p.Len() != 0 {
		t.Fatal("FreeAll kept an image")
	}
}

// A frame as a window with a few panes draws it: at depth 0 a layer for
// each pane's tab bar, one with a nested blur, all reused from the frame
// before.
func BenchmarkLayerPoolFrame(b *testing.B) {
	var p LayerPool[*fakeTex]
	areas := []image.Rectangle{
		image.Rect(0, 0, 400, 60), image.Rect(400, 0, 800, 60),
		image.Rect(0, 400, 400, 460), image.Rect(400, 400, 800, 460),
	}
	frame := func() {
		for i, a := range areas {
			p.Borrow(window, a, allocFake)
			if i == 1 {
				p.Borrow(window, a, allocFake)
				p.Return()
			}
			p.Return()
		}
		p.EndFrame()
	}
	frame()
	b.ReportAllocs()
	for b.Loop() {
		frame()
	}
}

// The gauge is told every image made and freed: what it holds is the bytes
// of the images kept, nothing once all are freed.
func TestLayerPoolGaugeBalances(t *testing.T) {
	var p LayerPool[*fakeTex]
	var gauge int64
	p.Gauge = func(b int64) { gauge += b }
	kept := func() int64 {
		var n int64
		for _, pool := range p.depths {
			for _, l := range pool {
				n += textureBytes(l.img)
			}
		}
		return n
	}
	check := func(when string) {
		t.Helper()
		if k := kept(); gauge != k {
			t.Fatalf("%s: gauge %d, images kept %d bytes", when, gauge, k)
		}
	}
	p.Borrow(window, image.Rect(0, 0, 10, 10), allocFake)
	p.Borrow(window, image.Rect(0, 0, 100, 100), allocFake)
	p.Return()
	p.Return()
	check("nested")
	if gauge == 0 {
		t.Fatal("the gauge counted nothing")
	}
	p.Borrow(window, image.Rectangle{}, allocFake) // replaces the small one
	p.Return()
	check("replaced")
	p.EndFrame()
	check("frame end")
	p.EndFrame() // nothing taken: all freed
	check("idle frame")
	p.Borrow(window, image.Rect(0, 0, 300, 300), allocFake)
	p.Return()
	p.FreeAll()
	if gauge != 0 {
		t.Fatalf("after FreeAll the gauge holds %d", gauge)
	}
}
