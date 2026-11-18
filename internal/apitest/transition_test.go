package apitest_test

import (
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

func TestTransitionSlidesInOnceAndNotOnRebuild(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	defer ggui.SetClock(func() time.Time { return now })()

	var got ggui.Rect
	tick := ggui.State(0)
	tree := ggui.Reactive(func() ggui.Widget {
		tick.Get() // rebuilt on every tick
		return widget.Column(widget.Transition(probe(50, 20, &got)).Slide(0, 30).Easing(ggui.EaseLinear).Duration(100 * time.Millisecond))
	})
	p := ggui.NewProbe(tree, ggui.Sz(100, 100))
	p.Frame()
	if got.Origin.Y != 30 {
		t.Fatalf("first frame at y=%v, want 30 (fully slid out)", got.Origin.Y)
	}
	now = base.Add(50 * time.Millisecond)
	tick.Set(1)
	p.Frame()
	if got.Origin.Y != 15 {
		t.Fatalf("halfway after a rebuild at y=%v, want 15: the start time must be retained", got.Origin.Y)
	}
	now = base.Add(time.Second)
	p.Frame()
	if got.Origin.Y != 0 {
		t.Fatalf("finished at y=%v, want 0", got.Origin.Y)
	}
}

func TestPresenceKeepsChildInertWhileLeaving(t *testing.T) {
	t.Parallel()
	show := ggui.State(true)
	taps := 0
	var got ggui.Rect
	child := widget.Tap(probe(50, 20, &got), func() { taps++ })
	tree := widget.Column(widget.Presence(show, widget.Transition(child).Slide(0, 40).Easing(ggui.EaseLinear).Duration(100*time.Millisecond)))
	p := ggui.NewProbe(tree, ggui.Sz(100, 100))
	defer p.Close()
	p.Click(ggui.Pt(10, 10))
	if got.Origin.Y != 0 || taps != 1 {
		t.Fatalf("visible child at y=%v taps=%d; want in place and tappable", got.Origin.Y, taps)
	}
	show.Set(false)
	p.Advance(0) // the effect starts the tween
	p.Advance(0) // the first step takes its start time
	p.Advance(50 * time.Millisecond)
	got = ggui.Rect{}
	p.Click(ggui.Pt(10, 10))
	if got == (ggui.Rect{}) || got.Origin.Y < 15 || got.Origin.Y > 25 {
		t.Fatalf("leaving child painted at %+v, want halfway out", got)
	}
	if taps != 1 {
		t.Fatal("a leaving child took a tap")
	}
	p.Advance(time.Second)
	got = ggui.Rect{}
	p.Frame()
	if got != (ggui.Rect{}) {
		t.Fatalf("child still painted after leaving: %+v", got)
	}
	show.Set(true)
	p.Advance(time.Second)
	got = ggui.Rect{}
	p.Frame()
	if got == (ggui.Rect{}) || got.Origin.Y != 40 {
		t.Fatalf("re-entering child at %+v, want at the start of its slide", got)
	}
}

// Presence drives its Transition with linear progress, so the Transition's
// own easing is the only curve applied.
func TestPresenceAppliesTheEasingOnce(t *testing.T) {
	t.Parallel()
	show := ggui.State(true)
	var got ggui.Rect
	tree := widget.Column(widget.Presence(show, widget.Transition(probe(50, 20, &got)).Slide(0, 40).Easing(ggui.EaseIn).Duration(100*time.Millisecond)))
	p := ggui.NewProbe(tree, ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	show.Set(false)
	p.Advance(0)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	// Leaving, half way back: progress 0.5, offset (1 - EaseIn(0.5)) * 40.
	if want := (1 - ggui.EaseIn(0.5)) * 40; math.Abs(got.Origin.Y-want) > 1e-6 {
		t.Fatalf("half way out at y=%v, want %v", got.Origin.Y, want)
	}
}

func TestSwitcherCrossFadesAndDisposesTheOldSubtree(t *testing.T) {
	t.Parallel()
	key := ggui.State(0)
	var rects [3]ggui.Rect
	taps := [3]int{}
	disposed := [3]bool{}
	sw := widget.Switcher(key, func(k int) ggui.Widget {
		ggui.OnCleanup(func() { disposed[k] = true })
		return widget.Tap(probe(50, 20, &rects[k]), func() { taps[k]++ })
	}).Slide(0, 10).Easing(ggui.EaseLinear).Duration(100 * time.Millisecond)
	p := ggui.NewProbe(widget.Column(sw), ggui.Sz(100, 100))
	defer p.Close()
	p.Click(ggui.Pt(10, 10))
	if rects[0].Origin.Y != 0 || taps[0] != 1 {
		t.Fatalf("first subtree at %v with %d taps: it must appear in place at once", rects[0], taps[0])
	}

	key.Set(1)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	rects = [3]ggui.Rect{}
	p.Click(ggui.Pt(10, 10))
	if rects[0] == (ggui.Rect{}) || rects[1] == (ggui.Rect{}) {
		t.Fatalf("both subtrees must paint while switching: %v", rects)
	}
	if rects[1].Origin.Y != 5 || rects[0].Origin.Y != 5 {
		t.Fatalf("half way: new at y=%v, old at y=%v; want both 5", rects[1].Origin.Y, rects[0].Origin.Y)
	}
	if taps[0] != 1 || taps[1] != 1 {
		t.Fatalf("taps %v: the leaving subtree must be inert and the new one live", taps)
	}
	if disposed[0] {
		t.Fatal("the old subtree was disposed before it left")
	}

	p.Advance(200 * time.Millisecond)
	p.Advance(0)
	rects = [3]ggui.Rect{}
	p.Frame()
	if !disposed[0] || rects[0] != (ggui.Rect{}) || rects[1].Origin.Y != 0 {
		t.Fatalf("after the switch: old disposed %v at %v, new at %v", disposed[0], rects[0], rects[1])
	}
}

// sequentialSwitcher is a Sequential Switcher whose subtree for k is 50
// wide and 20 high, 40 for odd k, and records where it was painted, how
// often it was built, whether it was disposed and its taps. The leaving
// half takes 100ms and the entering one 200ms; both slide by 10.
type sequentialSwitcher struct {
	key      *ggui.StateValue[int]
	sw       *widget.SwitcherWidget[int]
	rects    [4]ggui.Rect
	built    [4]int
	disposed [4]bool
	taps     [4]int
	size     ggui.Size // of the switcher, as its last layout left it
}

func newSequentialSwitcher() (*sequentialSwitcher, *ggui.Probe) {
	s := &sequentialSwitcher{key: ggui.State(0)}
	s.sw = widget.Switcher(s.key, func(k int) ggui.Widget {
		s.built[k]++
		s.disposed[k] = false
		ggui.OnCleanup(func() { s.disposed[k] = true })
		return widget.Tap(probe(50, float64(20+20*(k%2)), &s.rects[k]), func() { s.taps[k]++ })
	}).Sequential().Slide(0, 10).Easing(ggui.EaseLinear).Durations(100*time.Millisecond, 200*time.Millisecond)
	sized := ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
		s.size = s.sw.Layout(c, env)
		return s.size
	}, s.sw.Paint)
	return s, ggui.NewProbe(widget.Column(sized), ggui.Sz(100, 100))
}

// paint runs a frame with the rects cleared, so only what painted in it
// has one.
func (s *sequentialSwitcher) paint(p *ggui.Probe) {
	s.rects = [4]ggui.Rect{}
	p.Frame()
}

// A Sequential switcher plays the old subtree out before the new one comes
// in: the new one is built at once but neither painted nor hit while it
// waits, and the switcher is the leaving subtree's size until it enters.
func TestSequentialSwitcherLeavesThenEnters(t *testing.T) {
	t.Parallel()
	s, p := newSequentialSwitcher()
	defer p.Close()
	p.Frame()
	s.key.Set(1)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	s.rects = [4]ggui.Rect{}
	p.Click(ggui.Pt(10, 10))
	if s.built[1] != 1 || s.rects[1] != (ggui.Rect{}) || s.taps != [4]int{} {
		t.Fatalf("while the old subtree leaves the new one was built %d times, painted at %v, taps %v; want built, unpainted and inert", s.built[1], s.rects[1], s.taps)
	}
	if s.rects[0].Origin.Y != 5 || s.size.H != 20 {
		t.Fatalf("half way out the old subtree is at y=%v in a switcher %v high; want 5 and its own 20", s.rects[0].Origin.Y, s.size.H)
	}

	p.Advance(50 * time.Millisecond)
	s.paint(p)
	if !s.disposed[0] || s.rects[0] != (ggui.Rect{}) || s.rects[1].Origin.Y != 10 || s.size.H != 40 {
		t.Fatalf("as the old subtree is gone (disposed %v, at %v) the new one is at %v in a switcher %v high; want it starting in at y=10, 40 high", s.disposed[0], s.rects[0], s.rects[1], s.size.H)
	}
	p.Advance(100 * time.Millisecond)
	s.paint(p)
	if s.rects[1].Origin.Y != 5 {
		t.Fatalf("half way through its 200ms the new subtree is at y=%v, want 5", s.rects[1].Origin.Y)
	}
	p.Advance(100 * time.Millisecond)
	p.Click(ggui.Pt(10, 10))
	if s.rects[1].Origin.Y != 0 || s.taps[1] != 1 {
		t.Fatalf("the settled subtree is at %v with %d taps", s.rects[1], s.taps[1])
	}
}

// A switch while the old subtree leaves replaces the subtree waiting to
// enter, which goes without ever showing; one while the new subtree enters
// sends it back out from as far as it came.
func TestSequentialSwitcherUnderRapidChanges(t *testing.T) {
	t.Parallel()
	s, p := newSequentialSwitcher()
	defer p.Close()
	p.Frame()
	s.key.Set(1)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	s.key.Set(2)
	p.Advance(0)
	if !s.disposed[1] || s.built[2] != 1 || s.disposed[0] {
		t.Fatalf("a switch during the leave: waiting subtree disposed %v, next built %d times, leaving one disposed %v", s.disposed[1], s.built[2], s.disposed[0])
	}
	p.Advance(25 * time.Millisecond)
	s.paint(p)
	if s.rects[0].Origin.Y != 7.5 || s.rects[1] != (ggui.Rect{}) || s.rects[2] != (ggui.Rect{}) {
		t.Fatalf("75ms into the leave: old at %v, replaced %v, waiting %v; want the old one going on at y=7.5 alone", s.rects[0], s.rects[1], s.rects[2])
	}
	p.Advance(25 * time.Millisecond)
	p.Advance(100 * time.Millisecond)
	s.paint(p)
	if s.rects[2].Origin.Y != 5 {
		t.Fatalf("half way in the next subtree is at y=%v, want 5", s.rects[2].Origin.Y)
	}

	s.key.Set(3)
	p.Advance(0)
	p.Advance(25 * time.Millisecond)
	s.paint(p)
	if s.rects[2].Origin.Y != 7.5 || s.rects[3] != (ggui.Rect{}) || s.size.H != 20 {
		t.Fatalf("25ms out from half way in: at %v with the next %v in a switcher %v high; want y=7.5, the next waiting, 20 high", s.rects[2], s.rects[3], s.size.H)
	}
	p.Advance(25 * time.Millisecond)
	s.paint(p)
	if !s.disposed[2] || s.rects[3].Origin.Y != 10 {
		t.Fatalf("out after 50ms: disposed %v, the next at %v", s.disposed[2], s.rects[3])
	}
}

// Durations times a cross-fade's two halves apart.
func TestSwitcherDurationsTimeTheHalvesApart(t *testing.T) {
	t.Parallel()
	key := ggui.State(0)
	var rects [2]ggui.Rect
	sw := widget.Switcher(key, func(k int) ggui.Widget { return probe(50, 20, &rects[k]) }).
		Slide(0, 10).Easing(ggui.EaseLinear).Durations(100*time.Millisecond, 200*time.Millisecond)
	p := ggui.NewProbe(widget.Column(sw), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	key.Set(1)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	if rects[0].Origin.Y != 5 || rects[1].Origin.Y != 7.5 {
		t.Fatalf("50ms in: leaving at y=%v, entering at y=%v; want 5 and 7.5", rects[0].Origin.Y, rects[1].Origin.Y)
	}
}

// Reduced motion switches a Sequential switcher at once too.
func TestSequentialSwitcherUnderReducedMotionSwitchesAtOnce(t *testing.T) {
	t.Parallel()
	key := ggui.State(0)
	var rects [2]ggui.Rect
	disposed := [2]bool{}
	sw := widget.Switcher(key, func(k int) ggui.Widget {
		ggui.OnCleanup(func() { disposed[k] = true })
		return probe(50, 20, &rects[k])
	}).Sequential()
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, widget.Column(sw)), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	key.Set(1)
	p.Frame()
	rects = [2]ggui.Rect{}
	p.Frame()
	if !disposed[0] || rects[0] != (ggui.Rect{}) || rects[1] != ggui.Rct(ggui.Pt(0, 0), ggui.Sz(50, 20)) {
		t.Fatalf("reduced motion: old disposed %v painted %v, new painted %v", disposed[0], rects[0], rects[1])
	}
}

func TestSwitcherUnderReducedMotionSwitchesAtOnce(t *testing.T) {
	t.Parallel()
	key := ggui.State(0)
	var rects [2]ggui.Rect
	disposed := [2]bool{}
	sw := widget.Switcher(key, func(k int) ggui.Widget {
		ggui.OnCleanup(func() { disposed[k] = true })
		return probe(50, 20, &rects[k])
	})
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, widget.Column(sw)), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	key.Set(1)
	p.Frame()
	rects = [2]ggui.Rect{}
	p.Frame()
	if !disposed[0] || rects[0] != (ggui.Rect{}) || rects[1] == (ggui.Rect{}) {
		t.Fatalf("reduced motion: old disposed %v painted %v, new painted %v", disposed[0], rects[0], rects[1])
	}
}

// Options are value setters: changed after mount, they apply from the
// next switch, and a switch under way keeps the duration it started with.
func TestSwitcherOptionsApplyFromTheNextSwitch(t *testing.T) {
	t.Parallel()
	key := ggui.State(0)
	var rects [3]ggui.Rect
	sw := widget.Switcher(key, func(k int) ggui.Widget { return probe(50, 20, &rects[k]) }).
		Slide(0, 10).Easing(ggui.EaseLinear).Duration(100 * time.Millisecond)
	p := ggui.NewProbe(widget.Column(sw), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	sw.Duration(200*time.Millisecond).Slide(0, 20)
	key.Set(1)
	p.Advance(0)
	p.Advance(0)
	p.Advance(100 * time.Millisecond)
	if rects[1].Origin.Y != 10 {
		t.Fatalf("half way through a 200ms slide of 20 the new subtree is at y=%v, want 10", rects[1].Origin.Y)
	}
	sw.Duration(time.Second)
	p.Advance(50 * time.Millisecond)
	if rects[1].Origin.Y != 5 {
		t.Fatalf("a duration changed mid-switch moved it to y=%v, want 5", rects[1].Origin.Y)
	}
}

func TestAnimatedBoxUnderReducedMotionJumps(t *testing.T) {
	t.Parallel()
	width := ggui.State(100.0)
	var got ggui.Rect
	box := widget.AnimatedBox(probe(0, 0, &got)).BindWidth(width).Height(10)
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, widget.Column(box)), ggui.Sz(300, 100))
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
	width := ggui.State(0.0)
	var got ggui.Rect
	box := widget.AnimatedBox(probe(60, 10, &got)).BindWidth(width).Easing(ggui.EaseLinear).Duration(100 * time.Millisecond)
	p := ggui.NewProbe(widget.Column(box), ggui.Sz(300, 100))
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

// A collapsible box's zero is zero: it eases closed and open again.
func TestAnimatedBoxCollapsibleEasesToZero(t *testing.T) {
	t.Parallel()
	width := ggui.State(100.0)
	var got ggui.Rect
	box := widget.AnimatedBox(probe(60, 10, &got)).BindWidth(width).Collapsible().Easing(ggui.EaseLinear).Duration(100 * time.Millisecond)
	var shown ggui.Rect
	p := ggui.NewProbe(widget.Column(widget.Row(box, probe(10, 10, &shown))), ggui.Sz(300, 100))
	defer p.Close()
	p.Frame()
	if shown.Origin.X != 100 {
		t.Fatalf("at 100 wide the next child is at %v", shown.Origin.X)
	}
	width.Set(0)
	p.Advance(0)
	p.Advance(0)
	p.Advance(50 * time.Millisecond)
	if shown.Origin.X != 50 {
		t.Fatalf("half way to zero the next child is at %v, want 50", shown.Origin.X)
	}
	p.Advance(time.Second)
	p.Frame()
	if shown.Origin.X > 1e-6 {
		t.Fatalf("closed, the next child is at %v, want 0", shown.Origin.X)
	}
}

// A Switcher cross-fading in a scrolling pane of fixed size, beside other
// content, is painted within the pane's viewport alone: the layouts it
// asks for to drive each step are held there, as a size easing is. Only
// the frame of the write that switched it is painted whole. It asserts how
// frames are painted, which input to other tests can change, so it runs
// alone.
func TestASwitcherInAScrollPaintsWithinItsViewport(t *testing.T) {
	key := ggui.State(0)
	disposed := 0
	p := ggui.NewWindowProbe(func() ggui.Widget {
		sw := widget.Switcher(key, func(k int) ggui.Widget {
			ggui.OnCleanup(func() { disposed++ })
			return widget.Box().Size(80, float64(20+10*k))
		}).Slide(0, 10)
		pane := widget.Box(widget.Scroll(widget.Column(sw, widget.Box().Size(100, 400)))).Size(100, 100)
		return widget.Row(pane, widget.Box().Size(100, 100))
	}, ggui.Sz(200, 100))
	defer p.Close()
	p.Run(16*time.Millisecond, 10)
	key.Set(1)
	frames := p.Run(16*time.Millisecond, 60)
	if n := len(frames); n < 5 || frames[n-1].Next {
		t.Fatalf("switched over %d frames, want several and then rest: %v", n, frames)
	}
	if disposed != 1 {
		t.Fatalf("%d subtrees disposed after the switch, want the old one", disposed)
	}
	viewport := ggui.Rct(ggui.Point{}, ggui.Sz(100, 100))
	for i, fr := range frames[1:] {
		if fr.Painted != ggui.FrameContained || len(fr.Damage) == 0 {
			t.Fatalf("frame %d was painted %v, want contained: %v", i+1, fr.Painted, frames)
		}
		for _, d := range fr.Damage {
			if d.Empty() || d.Intersect(viewport) != d {
				t.Fatalf("frame %d damaged %v, want within the viewport %v", i+1, fr.Damage, viewport)
			}
		}
	}
}

// An AnimatedBox in a scrolling pane of fixed size eases its height in a
// window's own frames, painted within the pane's viewport, lands exactly
// on its target and rests.
func TestAnAnimatedBoxEasesInAWindowsFramesWithinAScroll(t *testing.T) {
	height := ggui.State(20.0)
	var got ggui.Rect
	box := widget.AnimatedBox(probe(0, 0, &got)).Width(80).BindHeight(height).Duration(100 * time.Millisecond)
	p := ggui.NewWindowProbe(func() ggui.Widget {
		pane := widget.Box(widget.Scroll(widget.Column(box, widget.Box().Size(100, 400)))).Size(100, 100)
		return widget.Row(pane, widget.Box().Size(100, 100))
	}, ggui.Sz(200, 100))
	defer p.Close()
	p.Run(16*time.Millisecond, 10)
	height.Set(60)
	var heights []float64
	var frames []ggui.WindowFrame
	for range 30 {
		fr := p.Step(16 * time.Millisecond)
		frames, heights = append(frames, fr), append(heights, got.Size.H)
		if !fr.Next {
			break
		}
	}
	if n := len(frames); n < 5 || frames[n-1].Next || got.Size.H != 60 {
		t.Fatalf("came to %v over %d frames, want 60 after several, then rest: %v", got.Size.H, n, heights)
	}
	for i, fr := range frames[1:] {
		if fr.Painted != ggui.FrameContained {
			t.Fatalf("frame %d was painted %v, want contained: %v", i+1, fr.Painted, frames)
		}
	}
}

// Ease moves along EaseOut; EaseWith along the curve it is given.
func TestCanvasEaseWithMovesAlongTheCurveGiven(t *testing.T) {
	slot := ggui.NewSlot[*ggui.Motion]("ease with test")
	for _, c := range []struct {
		name string
		ease ggui.Easing
		want float64
	}{
		{"default", nil, ggui.EaseOut(0.5)},
		{"linear", ggui.EaseLinear, 0.5},
		{"bezier", ggui.CubicBezier(0.4, 0, 0.2, 1), ggui.CubicBezier(0.4, 0, 0.2, 1)(0.5)},
	} {
		target, got := 0.0, 0.0
		w := ggui.FromFuncs(
			func(cs ggui.Constraints, _ ggui.Env) ggui.Size { return cs.Constrain(ggui.Sz(10, 10)) },
			func(dst *ggui.Canvas, r ggui.Rect) {
				if c.ease == nil {
					got = dst.Ease(ggui.Anchor{Rect: r}, slot, target, 100*time.Millisecond)
					return
				}
				got = dst.EaseWith(ggui.Anchor{Rect: r}, slot, target, 100*time.Millisecond, c.ease)
			})
		p := ggui.NewProbe(w, ggui.Sz(100, 100))
		p.Advance(0)
		target = 1
		p.Advance(0)
		p.Advance(50 * time.Millisecond)
		if math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: halfway in time is %v of the way there, want %v", c.name, got, c.want)
		}
		p.Close()
	}
}
