package ui

import (
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func carouselSlides(n int, basis float64) []ggui.Widget {
	w := make([]ggui.Widget, n)
	for i := range w {
		w[i] = CarouselItem(ggui.Text("Slide")).Basis(basis)
	}
	return w
}
func TestCarouselNavigationAndClipping(t *testing.T) {
	value := ggui.State(0)
	changes := 0
	c := Carousel(value, carouselSlides(5, 1)...).OnChange(func(int) { changes++ })
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Frame()
	if c.SnapCount() != 5 || c.CanPrevious() || !c.CanNext() {
		t.Fatal("initial navigation")
	}
	p.Click(ggui.Pt(402, 120))
	p.Advance(2 * time.Second)
	if value.Get() != 1 || changes != 1 || math.Abs(c.offset-336) > .01 {
		t.Fatalf("next: %d %d %f", value.Get(), changes, c.offset)
	}
	p.Click(ggui.Pt(200, 120))
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	p.Advance(time.Second)
	if value.Get() != 4 || c.CanNext() {
		t.Fatal("end")
	}
	p.Type(ggui.Mods{}, ggui.KeyHome)
	if value.Get() != 0 {
		t.Fatal("home")
	}
	value.Set(3)
	p.Frame()
	p.Advance(2 * time.Second)
	if math.Abs(c.offset-c.snaps[3]) > .01 {
		t.Fatal("external selection not followed")
	}
}
func TestCarouselSizingAndLoop(t *testing.T) {
	for _, basis := range []float64{1, .5, 1. / 3} {
		c := Carousel(ggui.State(0), carouselSlides(5, basis)...).Align(0)
		p := ggui.NewProbe(c, ggui.Sz(416, 240))
		p.Frame()
		want := 6 - int(math.Round(1/basis))
		if c.SnapCount() != want {
			t.Errorf("basis %v: %d snaps, want %d", basis, c.SnapCount(), want)
		}
		p.Close()
	}
	c := Carousel(ggui.State(0), carouselSlides(5, 1)...).Loop(true)
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Frame()
	c.Previous()
	p.Advance(2 * time.Second)
	if c.Selected() != 4 || c.offset >= 0 {
		t.Fatalf("loop previous: %d %f", c.Selected(), c.offset)
	}
	c.Next()
	p.Advance(2 * time.Second)
	if c.Selected() != 0 || math.Abs(c.offset) > .01 {
		t.Fatal("loop next")
	}
}
func TestCarouselMotionReducedAndAutoplay(t *testing.T) {
	c := Carousel(ggui.State(0), carouselSlides(3, 1)...).Autoplay(time.Second).StopOnInteraction(false)
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Advance(0)
	c.Next()
	p.Advance(150 * time.Millisecond)
	if c.offset <= 0 || c.offset >= c.target {
		t.Fatal("no intermediate motion")
	}
	p.Advance(time.Second)
	if c.Selected() != 2 {
		t.Fatal("autoplay")
	}
	c.Pause()
	p.Advance(2 * time.Second)
	if c.Selected() != 2 {
		t.Fatal("pause")
	}
	c.Play()
	p.Advance(time.Second)
	if c.Selected() != 0 {
		t.Fatal("autoplay restart")
	}
	r := Carousel(ggui.State(0), carouselSlides(3, 1)...).Autoplay(time.Millisecond)
	rp := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, r), ggui.Sz(416, 240))
	defer rp.Close()
	rp.Frame()
	r.Next()
	rp.Frame()
	if r.offset != r.target {
		t.Fatal("reduced motion")
	}
	rp.Advance(time.Second)
	if r.Selected() != 1 {
		t.Fatal("reduced motion autoplay")
	}
}
func TestCarouselVerticalRTLDisabledEmpty(t *testing.T) {
	for _, vertical := range []bool{false, true} {
		c := Carousel(ggui.State(0), carouselSlides(3, 1)...).RTL(true)
		if vertical {
			c.Vertical()
		}
		p := ggui.NewProbe(c, ggui.Sz(416, 336))
		p.Frame()
		p.Click(ggui.Pt(150, 120))
		key := ggui.KeyArrowLeft
		if vertical {
			key = ggui.KeyArrowDown
		}
		p.Type(ggui.Mods{}, key)
		if c.Selected() != 1 {
			t.Fatal("direction")
		}
		c.Disabled(true)
		p.Frame()
		c.Next()
		c.Act(ggui.Action{Kind: ggui.ActionSetValue, Num: 2})
		if c.Selected() != 1 {
			t.Fatal("disabled")
		}
		p.Close()
	}
	for _, size := range []ggui.Size{ggui.Sz(0, 0), ggui.Sz(20, 10), ggui.Sz(400, 240)} {
		c := Carousel(ggui.State(0))
		p := ggui.NewProbe(c, size)
		p.Frame()
		c.Next()
		if c.Selected() != -1 || c.CanNext() {
			t.Fatal("empty")
		}
		p.Close()
	}
}
func TestCarouselVisibleSlidesOnly(t *testing.T) {
	paints := make([]int, 1000)
	items := make([]ggui.Widget, len(paints))
	for i := range items {
		items[i] = &carouselPaintCounter{count: &paints[i]}
	}
	c := Carousel(ggui.State(500), items...)
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Frame()
	for i, n := range paints {
		if i == 500 && n == 0 || i != 500 && n != 0 {
			t.Fatalf("slide %d painted %d times", i, n)
		}
	}
}

type carouselPaintCounter struct{ count *int }

func (w *carouselPaintCounter) Layout(c ggui.Constraints, _ ggui.Env) ggui.Size {
	return c.Constrain(ggui.Sz(100, 100))
}
func (w *carouselPaintCounter) Paint(*ggui.Canvas, ggui.Rect) { *w.count++ }

func TestCarouselDragCaptureAndAutoplayPause(t *testing.T) {
	c := Carousel(ggui.State(0), carouselSlides(5, 1)...).Autoplay(time.Second)
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Advance(0)
	p.Press(ggui.Pt(340, 120))
	p.Advance(50 * time.Millisecond)
	p.Move(ggui.Pt(80, 120))
	p.Advance(150 * time.Millisecond)
	p.Release(ggui.Pt(80, 120))
	p.Advance(time.Second)
	if c.Selected() != 1 || c.dragging {
		t.Fatalf("drag: selected %d dragging %v", c.Selected(), c.dragging)
	}
	p.Move(ggui.Pt(500, 300))
	p.Advance(2 * time.Second)
	if c.Selected() != 1 {
		t.Fatal("interaction must stop autoplay")
	}
	c.Play()
	p.Move(ggui.Pt(200, 120))
	p.Advance(2 * time.Second)
	if c.Selected() != 1 {
		t.Fatal("hover must pause autoplay")
	}
	p.Click(ggui.Pt(500, 300))
	p.Advance(2 * time.Second)
	if c.Selected() != 2 {
		t.Fatal("autoplay did not resume")
	}
}

func BenchmarkCarouselVisiblePaint10000(b *testing.B) {
	c := Carousel(ggui.State(5000), carouselSlides(10000, 1)...).Loop(true)
	c.Layout(ggui.Tight(ggui.Sz(416, 240)), uitheme.Default().Apply(ggui.Env{}))
	r := ggui.Rct(ggui.Point{}, ggui.Sz(416, 240))
	c.Paint(nil, r)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.Paint(nil, r)
	}
}

func TestCarouselAutoplayPausesForFocusedSlideChild(t *testing.T) {
	child := Button("Inside", func() {})
	c := Carousel(ggui.State(0), child, ggui.Text("Second")).Autoplay(time.Second).StopOnInteraction(false)
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Advance(0)
	f, ok := p.Find("Inside")
	if !ok {
		t.Fatal("child absent")
	}
	p.Click(f.Center())
	p.Move(ggui.Pt(500, 300))
	p.Advance(3 * time.Second)
	if c.Selected() != 0 {
		t.Fatal("autoplay moved focused child")
	}
}
func TestCarouselSpringMatchesReferenceSteps(t *testing.T) {
	c := Carousel(ggui.State(0), carouselSlides(2, 1)...)
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Advance(0)
	c.Next()
	pos, velocity := 0., 0.
	for i := 1; i <= 20; i++ {
		velocity += (336 - pos) / 25
		velocity *= .68
		pos += velocity
		p.Advance(time.Second / 60)
		if math.Abs(c.position()-pos) > .001 {
			t.Fatalf("step %d: %f != %f", i, c.position(), pos)
		}
	}
}
