package ggui

import (
	"image/color"
	"testing"
	"time"

	"github.com/ironpark/ggui/internal/reactive"
)

func TestTweenMovesOverItsDuration(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	tw := Tween(0.0, time.Second).Easing(EaseLinear)
	tw.Set(100)
	t0 := time.Unix(0, 0)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(250 * time.Millisecond))
	if got := Untrack(tw.Get); got != 25 {
		t.Fatalf("value at 250ms = %v, want 25", got)
	}
	activeWorld().anims.step(t0.Add(2 * time.Second))
	if got := Untrack(tw.Get); got != 100 || len(activeWorld().anims.list) != 0 {
		t.Fatalf("value at the end = %v with %d animations still registered", got, len(activeWorld().anims.list))
	}
}

func TestTweenRetargetsFromCurrentValue(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	tw := Tween(0.0, time.Second).Easing(EaseLinear)
	tw.Set(100)
	t0 := time.Unix(0, 0)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(500 * time.Millisecond))
	tw.Set(0)
	activeWorld().anims.step(t0.Add(500 * time.Millisecond))
	activeWorld().anims.step(t0.Add(1000 * time.Millisecond))
	if got := Untrack(tw.Get); got != 25 {
		t.Fatalf("value = %v, want 25 (halfway back from 50)", got)
	}
	tw.Jump(7)
	activeWorld().anims.step(t0.Add(5 * time.Second))
	if Untrack(tw.Get) != 7 {
		t.Fatalf("Jump did not stick: %v", Untrack(tw.Get))
	}
}

func TestTweenDrivesEffects(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	tw := Tween(0, 100*time.Millisecond)
	runs := 0
	reactive.Observe(func() { tw.Get(); runs++ })
	tw.Set(10)
	t0 := time.Unix(0, 0)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(50 * time.Millisecond))
	reactive.Flush()
	if runs != 2 {
		t.Fatalf("effect ran %d times, want 2 after the value moved", runs)
	}
}

func TestSpringSettlesAtTarget(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	sp := Spring(0.0)
	sp.Set(1)
	now := time.Unix(0, 0)
	overshot := false
	for i := 0; i < 600 && len(activeWorld().anims.list) > 0; i++ {
		now = now.Add(time.Second / 60)
		activeWorld().anims.step(now)
		if Untrack(sp.Get) > 1 {
			overshot = true
		}
	}
	if len(activeWorld().anims.list) != 0 || Untrack(sp.Get) != 1 {
		t.Fatalf("spring did not settle: value %v, %d running", Untrack(sp.Get), len(activeWorld().anims.list))
	}
	if !overshot {
		t.Fatal("default spring should overshoot a little")
	}
}

func TestEasingEndpoints(t *testing.T) {
	t.Parallel()
	for _, e := range []Easing{EaseLinear, EaseIn, EaseOut, EaseInOut} {
		if e(0) != 0 || e(1) != 1 {
			t.Fatalf("easing does not map 0->0 and 1->1: %v %v", e(0), e(1))
		}
	}
	if EaseInOut(0.5) != 0.5 {
		t.Fatalf("EaseInOut(0.5) = %v", EaseInOut(0.5))
	}
}

func TestMotionRetargetsMidway(t *testing.T) {
	t.Parallel()
	var m Motion
	t0 := time.Unix(0, 0)
	m.MoveTo(0, t0, 100*time.Millisecond) // first target: no animation
	if m.Value(t0) != 0 {
		t.Fatal("initial value")
	}
	m.MoveTo(1, t0, 100*time.Millisecond)
	mid := m.Value(t0.Add(50 * time.Millisecond))
	if mid <= 0 || mid >= 1 {
		t.Fatalf("mid = %v", mid)
	}
	m.MoveTo(0, t0.Add(50*time.Millisecond), 100*time.Millisecond)
	if got := m.Value(t0.Add(50 * time.Millisecond)); got != mid {
		t.Fatalf("retarget jumped from %v to %v", mid, got)
	}
	if m.Value(t0.Add(time.Second)) != 0 {
		t.Fatal("did not arrive")
	}
}

func TestAnimationCleanupOnlyStopsOwnedValues(t *testing.T) {
	t.Parallel()
	outside := Tween(0.0, time.Second).Easing(EaseLinear)
	defer outside.Jump(0)
	var tw *Tweened[float64]
	var sp *Sprung[float64]
	p := NewProbe(Box(), Sz(10, 10)).Setup(func() {
		tw = Tween(0.0, time.Second).Easing(EaseLinear)
		sp = Spring(0.0)
	})
	defer p.Close()
	p.Frame()
	tw.Set(100)
	sp.Set(100)
	outside.Set(100)
	p.Advance(0)
	p.Advance(100 * time.Millisecond)
	tv, sv := Untrack(tw.Get), Untrack(sp.Get)
	p.Close()
	for _, a := range activeWorld().anims.list {
		if a == tw || a == sp {
			t.Fatal("closed probe retains its animation")
		}
	}
	// Stale handles cannot restart animations after the owner's cleanup.
	tw.Set(200)
	sp.Set(200)
	tw.Jump(300)
	sp.Jump(300)
	activeWorld().anims.step(outside.start.Add(500 * time.Millisecond))
	if Untrack(tw.Get) != tv || Untrack(sp.Get) != sv {
		t.Fatal("a disposed owner's animation changed value")
	}
	if Untrack(outside.Get) != 50 {
		t.Fatalf("unowned animation was stopped: %v", Untrack(outside.Get))
	}
}

func TestAnimationCreatedDuringStepIsNotDropped(t *testing.T) {
	t.Parallel()
	second := Tween(0.0, time.Second).Easing(EaseLinear)
	defer second.Jump(0)
	first := Tween(0.0, time.Second).Easing(func(p float64) float64 {
		if !second.running {
			second.Set(10)
		}
		return p
	})
	defer first.Jump(0)
	t0 := time.Unix(100, 0)
	first.Set(10)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(100 * time.Millisecond))
	activeWorld().anims.step(t0.Add(200 * time.Millisecond))
	if Untrack(second.Get) <= 0 {
		t.Fatal("animation registered by an easing callback was lost")
	}
}

// TweenOf and SpringOf start at the source's value and retarget as it
// changes, through a Watch that their owner disposes.
func TestTweenOfAndSpringOfFollowTheirSource(t *testing.T) {
	t.Parallel()
	activeWorld().anims.list = nil
	src := State(10.0)
	var tw *Tweened[float64]
	var sp *Sprung[float64]
	dispose := Root(func() {
		tw = TweenOf(src, time.Second).Easing(EaseLinear)
		sp = SpringOf(src)
	})
	if Untrack(tw.Get) != 10 || Untrack(sp.Get) != 10 {
		t.Fatalf("start = %v, %v; want the source's 10", Untrack(tw.Get), Untrack(sp.Get))
	}
	reactive.FlushUsers(nil) // the first Watch run sets the target it already has
	src.Set(110)
	reactive.Flush()
	reactive.FlushUsers(nil)
	t0 := time.Unix(0, 0)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(500 * time.Millisecond))
	if got := Untrack(tw.Get); got != 60 {
		t.Fatalf("tween halfway = %v, want 60", got)
	}
	if sp.Target() != 110 {
		t.Fatalf("spring target = %v, want 110", sp.Target())
	}
	dispose()
	src.Set(0)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if tw.Target() != 110 {
		t.Fatal("a disposed TweenOf still followed its source")
	}
}

func TestAnimationCanLoseOwnerDuringStep(t *testing.T) {
	t.Parallel()
	var tw *Tweened[float64]
	var dispose func()
	dispose = Root(func() {
		tw = Tween(0.0, time.Second).Easing(func(p float64) float64 {
			if p > 0 {
				dispose()
			}
			return p
		})
	})
	defer dispose()
	tw.Set(100)
	t0 := time.Unix(100, 0)
	activeWorld().anims.step(t0)
	activeWorld().anims.step(t0.Add(500 * time.Millisecond))
	if Untrack(tw.Get) != 0 || tw.running {
		t.Fatal("step updated an animation after its owner was disposed")
	}
	for _, s := range activeWorld().anims.list {
		if s == tw {
			t.Fatal("step restored the disposed animation's registration")
		}
	}
}

func TestMotionReducedWhileMoving(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	var m Motion
	m.MoveTo(0, now, time.Second)
	m.MoveTo(1, now, time.Second)
	m.MoveTo(1, now.Add(100*time.Millisecond), 0)
	if m.Value(now.Add(100*time.Millisecond)) != 1 {
		t.Fatal("zero duration must settle an existing target immediately")
	}
}

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
	over := color.NRGBAModel.Convert(LerpColor(black, white, 1.5))
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
