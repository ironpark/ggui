package ggui

import (
	"image/color"
	"testing"
	"time"
)

func TestLerpColorBlendsPremultiplied(t *testing.T) {
	t.Parallel()
	red := color.NRGBA{255, 0, 0, 255}
	clear := color.NRGBA{0, 0, 255, 0} // transparent blue: no hue to blend in
	got := color.NRGBAModel.Convert(LerpColor(red, clear, 0.5)).(color.NRGBA)
	if got.R != 255 || got.B != 0 || got.A < 127 || got.A > 128 {
		t.Fatalf("half way from red to transparent = %+v, want red at half alpha", got)
	}
	if LerpColor(red, nil, 1) != nil || LerpColor(red, clear, 0) != color.Color(red) {
		t.Fatal("LerpColor must return its endpoints themselves")
	}
	white, black := color.White, color.Black
	g := color.GrayModel.Convert(LerpColor(black, white, 0.25)).(color.Gray)
	if g.Y < 63 || g.Y > 64 {
		t.Fatalf("a quarter from black to white = %v", g.Y)
	}
	over := color.NRGBAModel.Convert(LerpColor(black, white, 1.5).(color.Color))
	if _, _, _, a := over.RGBA(); a != 0xffff {
		t.Fatal("overshoot must clamp to a valid color")
	}
}

func TestLerpGeometry(t *testing.T) {
	t.Parallel()
	if got := LerpPoint(Pt(0, 10), Pt(10, 30), 0.5); got != Pt(5, 20) {
		t.Fatalf("LerpPoint = %v", got)
	}
	if got := LerpSize(Sz(0, 0), Sz(40, 20), 0.25); got != Sz(10, 5) {
		t.Fatalf("LerpSize = %v", got)
	}
	if got := LerpRect(Rct(Pt(0, 0), Sz(10, 10)), Rct(Pt(10, 20), Sz(30, 10)), 0.5); got != Rct(Pt(5, 10), Sz(20, 10)) {
		t.Fatalf("LerpRect = %v", got)
	}
	if got := LerpInsets(Insets(0), Insets(8, 4), 0.5); got != Insets(4, 2) {
		t.Fatalf("LerpInsets = %v", got)
	}
	if LerpFloat(2, 4, 0.5) != 3 {
		t.Fatal("LerpFloat")
	}
	if got := RectVector.Lerp(Rct(Pt(0, 0), Sz(0, 0)), Rct(Pt(4, 4), Sz(8, 8)), 0.5); got != Rct(Pt(2, 2), Sz(4, 4)) {
		t.Fatalf("Vector.Lerp = %v", got)
	}
	for _, c := range []color.Color{color.Black, color.NRGBA{10, 20, 30, 128}} {
		r1, g1, b1, a1 := c.RGBA()
		r2, g2, b2, a2 := ColorVector.Join(ColorVector.Split(c)).RGBA()
		if r1>>8 != r2>>8 || g1>>8 != g2>>8 || b1>>8 != b2>>8 || a1>>8 != a2>>8 {
			t.Fatalf("ColorVector does not round-trip %v", c)
		}
	}
}

func TestTweenWithMovesAnyType(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	tw := TweenWith(Pt(0, 0), time.Second, LerpPoint).Easing(EaseLinear)
	tw.Set(Pt(100, 40))
	t0 := time.Unix(0, 0)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(500 * time.Millisecond))
	if got := Untrack(tw.Get); got != Pt(50, 20) {
		t.Fatalf("point at 500ms = %v", got)
	}
	activeWorld().anims.step(t0.Add(time.Second))
	if got := Untrack(tw.Get); got != Pt(100, 40) || tw.Target() != Pt(100, 40) {
		t.Fatalf("point at the end = %v", got)
	}

	c := TweenColor(color.Black, time.Second).Easing(EaseLinear)
	c.Set(color.White)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(500 * time.Millisecond))
	if g := color.GrayModel.Convert(Untrack(c.Get)).(color.Gray); g.Y < 127 || g.Y > 128 {
		t.Fatalf("color at 500ms = %v", g)
	}
	activeWorld().anims.step(t0.Add(time.Second))
	if Untrack(c.Get) != color.Color(color.White) {
		t.Fatal("a color tween must land on its target itself")
	}

	n := Tween(0, time.Second).Easing(EaseLinear) // ints still tween
	n.Set(10)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(500 * time.Millisecond))
	if Untrack(n.Get) != 5 {
		t.Fatalf("int at 500ms = %v", Untrack(n.Get))
	}
	n.Jump(0)
}

func TestTweenDelayHoldsTheStart(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	tw := Tween(0.0, 100*time.Millisecond).Easing(EaseLinear).Delay(200 * time.Millisecond)
	tw.Set(10)
	t0 := time.Unix(0, 0)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(150 * time.Millisecond))
	if Untrack(tw.Get) != 0 {
		t.Fatalf("moved during its delay: %v", Untrack(tw.Get))
	}
	activeWorld().anims.step(t0.Add(250 * time.Millisecond))
	if Untrack(tw.Get) != 5 {
		t.Fatalf("value 50ms after the delay = %v, want 5", Untrack(tw.Get))
	}
	activeWorld().anims.step(t0.Add(time.Second))
	if Untrack(tw.Get) != 10 || len(activeWorld().anims.list) != 0 {
		t.Fatal("did not finish after its delay")
	}

	// A delay alone still waits.
	j := Tween(0.0, 0).Delay(100 * time.Millisecond)
	j.Set(1)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(50 * time.Millisecond))
	if Untrack(j.Get) != 0 {
		t.Fatal("a zero-duration tween with a delay jumped early")
	}
	activeWorld().anims.step(t0.Add(100 * time.Millisecond))
	if Untrack(j.Get) != 1 {
		t.Fatal("a zero-duration tween with a delay did not jump after it")
	}
}

func TestSpringWithSettlesEveryComponent(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	sp := SpringWith(Pt(0, 0), PointVector)
	target := Pt(100, -50)
	sp.Set(target)
	if sp.Target() != target {
		t.Fatal("Target")
	}
	now := time.Unix(0, 0)
	mid := false
	for i := 0; i < 600 && len(activeWorld().anims.list) > 0; i++ {
		now = now.Add(time.Second / 60)
		activeWorld().anims.step(now)
		if p := Untrack(sp.Get); p.X > 10 && p.X < 90 && p.Y < -5 && p.Y > -45 {
			mid = true
		}
	}
	if Untrack(sp.Get) != target || !mid {
		t.Fatalf("spring ended at %v (moved through the middle: %v)", Untrack(sp.Get), mid)
	}

	c := SpringWith[color.Color](color.Black, ColorVector)
	c.Set(color.White)
	for i := 0; i < 600 && len(activeWorld().anims.list) > 0; i++ {
		now = now.Add(time.Second / 60)
		activeWorld().anims.step(now)
		if _, _, _, a := Untrack(c.Get).RGBA(); a != 0xffff {
			t.Fatal("a springing opaque color lost its alpha")
		}
	}
	if Untrack(c.Get) != color.Color(color.White) {
		t.Fatal("color spring must land on its target itself")
	}
}

func TestTweenAndSpringOfWithFollowASource(t *testing.T) {
	t.Parallel()
	src := State(Sz(0, 0))
	var tw *Tweened[Size]
	var sp *Sprung[Size]
	p := NewProbe(Box(), Sz(10, 10)).Setup(func() {
		tw = TweenOfWith(src, 100*time.Millisecond, LerpSize)
		sp = SpringOfWith(src, SizeVector)
	})
	defer p.Close()
	p.Frame()
	src.Set(Sz(20, 20))
	for range 60 {
		p.Advance(50 * time.Millisecond)
	}
	if Untrack(tw.Get) != Sz(20, 20) || Untrack(sp.Get) != Sz(20, 20) {
		t.Fatalf("followers ended at %v and %v", Untrack(tw.Get), Untrack(sp.Get))
	}
}

// A missing lerp or Vector fails where it is given, not frames later in
// the animation step.
func TestTweenWithAndSpringWithRejectMissingFuncs(t *testing.T) {
	t.Parallel()
	mustPanic(t, "TweenWith(nil lerp)", func() { TweenWith(Pt(0, 0), time.Second, nil) })
	mustPanic(t, "SpringWith(zero Vector)", func() { SpringWith(Pt(0, 0), Vector[Point]{}) })
	if r := Reverse(nil); r(0.25) != 0.25 {
		t.Fatalf("Reverse(nil)(0.25) = %v, want linear", r(0.25))
	}
}
