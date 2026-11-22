package widget

import (
	"image/color"
	"testing"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
)

// The transitions' tests through the public surface are in
// internal/apitest; this one stays for the fill an AnimatedBox shows half
// way, which a probe that paints no pixels reads only from inside.
func TestAnimatedBoxEasesBoundValues(t *testing.T) {
	t.Parallel()
	width := ggui.State(100.0)
	pad := ggui.State(ggui.Insets(0))
	fill := ggui.State[color.Color](color.Black)
	var got ggui.Rect
	box := AnimatedBox(probe(0, 0, &got)).BindWidth(width).Height(20).BindPadding(pad).
		BindFill(fill).Radius(4).Easing(ggui.EaseLinear).Duration(100 * time.Millisecond)
	p := ggui.NewProbe(Column(box), ggui.Sz(300, 100))
	defer p.Close()
	p.Frame()
	if got.Size.W != 100 {
		t.Fatalf("starting width %v", got.Size.W)
	}
	width.Set(200)
	pad.Set(ggui.Insets(10))
	fill.Set(color.White)
	p.Advance(0)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	// Half way: 150 wide less 5 padding on each side.
	if got.Size.W != 140 || got.Origin.X != 5 {
		t.Fatalf("half way the child is at %v, want x=5 and 140 wide", got)
	}
	if g := color.GrayModel.Convert(box.box.fill.shown).(color.Gray); g.Y < 120 || g.Y > 135 {
		t.Fatalf("half way fill gray %v", g.Y)
	}
	p.Advance(time.Second)
	p.Frame()
	if got.Size.W != 180 || got.Origin.X != 10 || box.box.fill.shown != color.Color(color.White) {
		t.Fatalf("at the end the child is at %v with fill %v", got, box.box.fill.shown)
	}

	box.Width(50) // a literal after mount animates too
	p.Advance(0)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	if got.Size.W != 125-20 {
		t.Fatalf("a literal width half way = %v, want 105", got.Size.W)
	}
}

func TestTransitionFadeHoldsALayerOnlyWhileAnimating(t *testing.T) {
	t.Parallel()
	img := ggfx.NewImage(120, 80)
	defer img.Deallocate()
	fading := func() *ggui.Probe {
		p := ggui.NewProbe(Transition(Text("fading")), ggui.Sz(120, 80))
		p.RenderTo(img)
		p.Frame()
		if n := p.Layers(); n != 1 {
			t.Fatalf("mid-fade the window pools %d images; want 1", n)
		}
		return p
	}
	t.Run("fade ends", func(t *testing.T) {
		p := fading()
		defer p.Close()
		p.Advance(time.Second)
		p.Frame()
		if n := p.Layers(); n != 0 {
			t.Fatalf("after the fade the window pools %d images; want 0", n)
		}
	})
	t.Run("closed mid-fade", func(t *testing.T) {
		p := fading()
		p.Close()
		if n := p.Layers(); n != 0 {
			t.Fatalf("a closed probe pools %d images; want 0", n)
		}
	})
}

func TestTransitionNilEasingAndPresenceReducedMotion(t *testing.T) {
	t.Parallel()
	tr := Transition(Box().Size(20, 20)).Fade().Easing(nil)
	p1 := ggui.NewProbe(tr, ggui.Sz(50, 50))
	defer p1.Close()
	p1.Frame()

	key := ggui.State(1)
	sw := Switcher(key, func(int) ggui.Widget { return Box().Size(20, 20) }).Easing(nil)
	p2 := ggui.NewProbe(sw, ggui.Sz(50, 50))
	defer p2.Close()
	p2.Frame()
	key.Set(2)
	p2.Advance(50 * time.Millisecond)

	// Presence under ReducedMotion jumps immediately when hidden.
	show := ggui.State(true)
	painted := 0
	child := ggui.FromFuncs(
		func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(20, 20)) },
		func(*ggui.Canvas, ggui.Rect) { painted++ },
	)
	pres := Presence(show, Transition(child).Fade().Duration(time.Second))
	p3 := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, pres), ggui.Sz(50, 50))
	defer p3.Close()
	p3.Frame()
	if painted == 0 {
		t.Fatal("Presence child did not paint while shown")
	}
	painted = 0
	show.Set(false)
	p3.Frame()
	if painted != 0 {
		t.Fatalf("Presence under ReducedMotion still painted %d times after hiding", painted)
	}
}
