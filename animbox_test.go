package ggui

import (
	"image/color"
	"testing"
	"time"
)

func TestAnimatedBoxEasesBoundValues(t *testing.T) {
	t.Parallel()
	width := State(100.0)
	pad := State(Insets(0))
	fill := State[color.Color](color.Black)
	var got Rect
	box := AnimatedBox(probe(0, 0, &got)).BindWidth(width).Height(20).BindPadding(pad).
		BindFill(fill).Radius(4).Easing(EaseLinear).Duration(100 * time.Millisecond)
	p := NewProbe(Column(box), Sz(300, 100))
	defer p.Close()
	p.Frame()
	if got.Size.W != 100 {
		t.Fatalf("starting width %v", got.Size.W)
	}
	width.Set(200)
	pad.Set(Insets(10))
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

func TestAnimatedBoxUnderReducedMotionJumps(t *testing.T) {
	t.Parallel()
	width := State(100.0)
	var got Rect
	box := AnimatedBox(probe(0, 0, &got)).BindWidth(width).Height(10)
	p := NewProbe(Provide(ReducedMotionKey, true, Column(box)), Sz(300, 100))
	defer p.Close()
	p.Frame()
	width.Set(200)
	p.Frame()
	if got.Size.W != 200 {
		t.Fatalf("reduced motion width %v, want 200 at once", got.Size.W)
	}
}

// Zero sizes to the child, so no number between leads to it: a box growing
// from zero starts at the child's size, and one shrinking to zero lands.
func TestAnimatedBoxEasesFromTheChildSize(t *testing.T) {
	t.Parallel()
	width := State(0.0)
	var got Rect
	box := AnimatedBox(probe(60, 10, &got)).BindWidth(width).Easing(EaseLinear).Duration(100 * time.Millisecond)
	p := NewProbe(Column(box), Sz(300, 100))
	defer p.Close()
	p.Frame()
	if got.Size.W != 60 {
		t.Fatalf("sized to the child: %v, want 60", got.Size.W)
	}
	width.Set(160)
	p.Advance(0)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	if got.Size.W != 110 {
		t.Fatalf("half way from the child's 60 to 160: %v, want 110", got.Size.W)
	}
	p.Advance(time.Second)
	p.Frame()
	width.Set(0)
	p.Frame()
	if got.Size.W != 60 {
		t.Fatalf("back to the child's size: %v, want 60 at once", got.Size.W)
	}
}
