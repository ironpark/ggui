package ggui

import (
	"slices"
	"testing"
	"time"

	"github.com/ironpark/ggui/internal/framestats"
)

// The tests here assert how a window paints the frames a scroll moves in,
// which an event handed to any widget in the process changes: they run
// without t.Parallel; see WindowProbe.

// frameTime is a 60Hz refresh.
const frameTime = 16 * time.Millisecond

// containedWindow is a window of a scroll of tall content, its rows
// tappable, beside a box, drawn once whole. It returns the window, the
// signal the box beside is built from and the count of taps on the rows.
func containedWindow(t *testing.T) (*WindowProbe, *StateValue[int], *int) {
	t.Helper()
	return containedWindowBeside(t, nil)
}

// containedWindowBeside is containedWindow with beside, when not nil, in
// place of the box.
func containedWindowBeside(t *testing.T, beside Widget) (*WindowProbe, *StateValue[int], *int) {
	t.Helper()
	other, taps := State(0), new(int)
	p := NewWindowProbe(func() Widget {
		rows := make([]Widget, 40)
		for i := range rows {
			rows[i] = Tap(Box().Size(100, 20), func() { *taps++ })
		}
		side := beside
		if side == nil {
			side = View(other, func(int) Widget { return Box().Size(100, 100) })
		}
		return Row(Box(Scroll(Column(rows...))).Size(100, 100), side)
	}, Sz(200, 100))
	t.Cleanup(p.Close)
	p.Step(0)
	return p, other, taps
}

// A wheel's move over a scroll paints the frame within the scroll's
// viewport alone, and what the pointer reached with pointerReach around
// it, its regions registered as a whole frame's are: a click the next
// frame takes lands on the row the move brought under it.
func TestAScrollsMovePaintsWithinItsViewport(t *testing.T) {
	p, _, taps := containedWindow(t)
	p.Scroll(Pt(50, 50), Pt(0, -30))
	fr := p.Step(frameTime)
	paintedWithin(t, "the wheel", []WindowFrame{fr}, false, pointerReach, false)
	if len(fr.Damage) != 1 {
		t.Fatalf("damage %v, want one region", fr.Damage)
	}
	p.Click(Pt(50, 50))
	p.Step(frameTime)
	if *taps != 1 {
		t.Fatalf("a click after the frame tapped %d rows, want 1: the frame registered no regions", *taps)
	}
}

// A write outside the scroll in the same frame paints all of it. The
// window names the writes outside a scroll from a frame it contained on,
// so the scroll moves alone first.
func TestAScrollsMoveWithAnotherChangePaintsWhole(t *testing.T) {
	p, other, _ := containedWindow(t)
	p.Scroll(Pt(50, 50), Pt(0, -30))
	if fr := p.Step(frameTime); fr.Painted != FrameContained {
		t.Fatalf("the scroll's move alone was painted %v, want within its viewport", fr.Painted)
	}
	p.Scroll(Pt(50, 50), Pt(0, -30))
	other.Set(1)
	fr := p.Step(frameTime)
	if fr.Painted != FrameWhole {
		t.Fatalf("a frame with another change was painted %v, want whole", fr.Painted)
	}
	if len(fr.Damage) != 1 || fr.Damage[0] != Rct(Point{}, Sz(200, 100)) {
		t.Fatalf("damage %v, want all of the screen", fr.Damage)
	}
	// The write is named as what kept the frame whole.
	const writer = "state changed github.com/ironpark/ggui.TestAScrollsMoveWithAnotherChangePaintsWhole"
	if framestats.ReadReport().Dirtiers[writer] == 0 {
		t.Fatalf("the write outside the scroll was not named: %v", framestats.ReadReport().Dirtiers)
	}
}

// A widget beside a scroll is told that a frame painted within the
// scroll's viewport does not show it, and that a whole one does. It stands
// past the pointerReach around the scroll that a wheel's frame paints too.
func TestAWidgetBesideAScrollsMoveIsNotShown(t *testing.T) {
	var shown []bool
	beside := Row(Box().Size(pointerReach, 100), Box(FromFuncs(func(c Constraints, _ Env) Size { return c.Max() }, func(dst *Canvas, r Rect) { shown = append(shown, dst.Shows(r)) })).Size(52, 100))
	p, _, _ := containedWindowBeside(t, beside)
	if len(shown) != 1 || !shown[0] {
		t.Fatalf("a whole frame showed the widget %v, want [true]", shown)
	}
	p.Scroll(Pt(50, 50), Pt(0, -30))
	if fr := p.Step(frameTime); fr.Painted != FrameContained {
		t.Fatalf("the scroll's frame was painted %v, want within its viewport", fr.Painted)
	}
	if len(shown) != 2 || shown[1] {
		t.Fatalf("the scroll's frame showed the widget beside it: %v", shown)
	}
}

// A wheel's move up a long list mounts rows taller than it guessed, which
// learn their height a pass later, and the frame lays the list out again to keep
// what is in view in place: all of that is contained in the viewport.
func TestAScrollMountingRowsItGuessedWrongStaysContained(t *testing.T) {
	items := State(make([]int, 200))
	for i := range items.Get() {
		items.Get()[i] = i
	}
	p := NewWindowProbe(func() Widget {
		list := EachKeyed(items, func(i int) int { return i }, func(row EachItem[int]) Widget {
			// A row learns its height as it is laid out, as a chat's turn
			// does: the frame lays the list out again.
			return View(row.Value, func(i int) Widget {
				h := State(20.0)
				want := float64(30 + i%3*20)
				return View(h, func(got float64) Widget {
					return FromFuncs(func(Constraints, Env) Size {
						if got != want {
							h.Set(want)
						}
						return Sz(100, got)
					}, func(*Canvas, Rect) {})
				})
			})
		}).EstimatedExtent(20)
		return Row(Box(Scroll(list).FollowEnd()).Size(100, 100), Box().Size(100, 100))
	}, Sz(200, 100))
	defer p.Close()
	atRest(t, p)
	for k := range 60 {
		p.Scroll(Pt(50, 50), Pt(0, 3))
		if fr := p.Step(frameTime); fr.Painted != FrameContained {
			t.Fatalf("move %d was painted %v: %v %v", k, fr.Painted, framestats.ReadReport().Refusals, framestats.ReadReport().Dirtiers)
		}
	}
}

// A widget beside a scroll that reads its offset, as a button to the end
// does, is painted when what it shows changes: that frame is whole.
func TestAReaderOfAScrollsOffsetBesideItIsPainted(t *testing.T) {
	offset := State(0.0)
	builds := 0
	p := NewWindowProbe(func() Widget {
		rows := make([]Widget, 40)
		for i := range rows {
			rows[i] = Box().Size(100, 20)
		}
		far := Map(offset, func(o float64) bool { return o > 100 })
		side := View(far, func(bool) Widget { builds++; return Box().Size(100, 100) })
		return Row(Box(Scroll(Column(rows...)).BindOffset(offset)).Size(100, 100), side)
	}, Sz(200, 100))
	defer p.Close()
	p.Step(0)
	for k := 0; offset.Get() <= 100; k++ {
		if k > 20 {
			t.Fatal("the scroll did not move")
		}
		p.Scroll(Pt(50, 50), Pt(0, -1))
		built := builds
		contained := p.Step(frameTime).Painted == FrameContained
		if crossed := builds != built; crossed == contained {
			t.Fatalf("move %d to %v: the reader beside was rebuilt %v, the frame contained %v", k, offset.Get(), crossed, contained)
		}
	}
}

// disclosure is a widget that grows a step taller at each layout until it
// is steps tall, asking for the next layout as it paints, the way a
// disclosure easing open does. It reports the layouts run and the last one
// painted.
func disclosure(steps int) (w Widget, laid, painted *int) {
	var env Env
	laid, painted = new(int), new(int)
	return FromFuncs(func(c Constraints, e Env) Size {
		env = e
		*laid++
		return c.Constrain(Sz(10, float64(*laid)))
	}, func(dst *Canvas, r Rect) {
		*painted = *laid
		if *laid < steps {
			Invalidate(env)
		}
	}), laid, painted
}

// A change of size inside a scroll whose viewport keeps its own shows in
// that viewport alone: the frames that ease it are painted there, not over
// the whole window, and register the regions a click lands on.
func TestALayoutEasedInsideAScrollPaintsWithinItsViewport(t *testing.T) {
	const steps = 5
	grows, laid, painted := disclosure(steps)
	taps := 0
	p := NewWindowProbe(func() Widget {
		return Row(Box(Scroll(Column(grows, Tap(Box().Size(100, 400), func() { taps++ })))).Size(100, 100), Box().Size(100, 100))
	}, Sz(200, 100))
	defer p.Close()
	p.Step(0)
	frames := p.Run(0, 2*steps)
	if *laid != steps || *painted != steps {
		t.Fatalf("laid out %d times and painted the %dth; want %d of each", *laid, *painted, steps)
	}
	if len(frames) != steps-1 {
		t.Fatalf("%d frames after the first, want %d: %v", len(frames), steps-1, frames)
	}
	viewport := Rct(Point{}, Sz(100, 100))
	for i, fr := range frames {
		if fr.Painted != FrameContained || len(fr.Damage) == 0 {
			t.Fatalf("frame %d was painted %v, want within the scroll: %v", i, fr.Painted, frames)
		}
		for _, d := range fr.Damage {
			if d.Empty() || d.Intersect(viewport) != d {
				t.Fatalf("frame %d damaged %v, want within the scroll's viewport", i, fr.Damage)
			}
		}
	}
	p.Click(Pt(50, 50))
	p.Step(frameTime)
	if taps != 1 {
		t.Fatalf("a click after the frames tapped %d times, want 1: the frames registered no regions", taps)
	}
}

// A scroll as long as its content moves what lies below it when the
// content grows: the change is not held there, and the frame is painted
// whole, or within a scroll around it that keeps its size.
func TestALayoutEasedInsideAScrollSizedByItsContentIsNotHeldThere(t *testing.T) {
	const steps = 6
	// A scroll no longer than its content, with a box below it that the
	// content's growth moves.
	shrunk := func(w Widget) Widget { return Column(Scroll(w).Shrink(), Box().Size(50, 50)) }
	for _, tc := range []struct {
		name   string
		around func(Widget) Widget
		whole  bool
	}{
		{"alone", func(w Widget) Widget { return Align(shrunk(w)).At(0, 0) }, true},
		{"in a scroll that keeps its size", func(w Widget) Widget {
			return Row(Box(Scroll(Column(shrunk(w), Box().Size(100, 400)))).Size(100, 100), Box().Size(100, 100))
		}, false},
	} {
		grows, laid, painted := disclosure(steps)
		p := NewWindowProbe(func() Widget { return tc.around(grows) }, Sz(200, 100))
		frames := p.Run(0, 3*steps)
		p.Close()
		if *laid < steps || *painted != *laid {
			t.Fatalf("%s: laid out %d times and painted the %dth; want %d", tc.name, *laid, *painted, steps)
		}
		// Whatever the first frames found, the last ones show how the
		// change is painted once the scroll is known to follow its content.
		last := frames[len(frames)-1]
		if whole := last.Painted == FrameWhole; whole != tc.whole {
			t.Fatalf("%s: the last of %v was painted %v; whole %v, want %v", tc.name, frames, last.Painted, whole, tc.whole)
		}
		if !tc.whole && (len(last.Damage) != 1 || last.Damage[0].Intersect(Rct(Point{}, Sz(100, 100))) != last.Damage[0]) {
			t.Fatalf("%s: last damage %v, want within the outer scroll's viewport", tc.name, last.Damage)
		}
	}
}

// pointerReach is how far around what the pointer reached the window
// paints a frame again, for what a widget draws past its Rect, and
// clockReach how far around what reads the clock: the kernel's damageReach
// and motionSlack, which are not exported.
const (
	pointerReach = 48
	clockReach   = 4
)

// scrolledList is a window of a scroll of a virtualized list of 200 rows,
// 20 tall, with a sticky header every tenth, beside a box: the scroll is
// the left half of it. config configures the scroll.
func scrolledList(sc *ScrollController, config func(*ScrollWidget)) *WindowProbe {
	items := make([]int, 200)
	for i := range items {
		items[i] = i
	}
	list := EachKeyed(State(items), func(i int) int { return i }, func(EachItem[int]) Widget { return Box().Size(100, 20) }).
		EstimatedExtent(20).StickyHeaders(func(i int) bool { return i%10 == 0 })
	return NewWindowProbe(func() Widget {
		s := Scroll(list).Controller(sc)
		if config != nil {
			config(s)
		}
		return Row(Box(s).Size(100, 100), Box().Size(100, 100))
	}, Sz(200, 100))
}

// atRest runs p's frames until the window asks for no other, as its list
// mounts and measures its rows.
func atRest(t *testing.T, p *WindowProbe) {
	t.Helper()
	if fr := p.Run(frameTime, 10); fr[len(fr)-1].Next {
		t.Fatalf("the window did not come to rest: %v", fr)
	}
}

// paintedWithin fails t unless every frame was painted contained, or in
// part where partial says it may be, within the scroll's viewport grown by
// reach, and, for rest, the last asks for no other.
func paintedWithin(t *testing.T, name string, frames []WindowFrame, partial bool, reach float64, rest bool) {
	t.Helper()
	viewport := Rct(Point{}, Sz(100, 100)).Grow(reach)
	for i, fr := range frames {
		if fr.Painted != FrameContained && (!partial || fr.Painted != FramePartial) || len(fr.Damage) == 0 {
			t.Fatalf("%s: frame %d was painted %v, want within the scroll: %v", name, i, fr.Painted, frames)
		}
		for _, d := range fr.Damage {
			if d.Empty() || d.Intersect(viewport) != d {
				t.Fatalf("%s: frame %d damaged %v, want within the scroll's viewport", name, i, fr.Damage)
			}
		}
	}
	if rest && (len(frames) == 0 || frames[len(frames)-1].Next) {
		t.Fatalf("%s: the window did not come to rest: %v", name, frames)
	}
}

// A scroll moving on its own, as a ScrollController eases it, changes what
// its viewport shows and nothing else: every frame of the move is painted
// there, the rows that scroll into the list mounting within it.
func TestAScrollControllersMovePaintsWithinItsViewport(t *testing.T) {
	for _, tc := range []struct {
		name string
		move func(*ScrollController)
		want float64
	}{
		{"ScrollTo", func(sc *ScrollController) { sc.ScrollTo(1000) }, 1000},
		{"JumpTo", func(sc *ScrollController) { sc.JumpTo(1000) }, 1000},
		{"ScrollToKey", func(sc *ScrollController) { sc.ScrollToKey(150) }, 3000},
	} {
		var sc ScrollController
		p := scrolledList(&sc, nil)
		atRest(t, p)
		tc.move(&sc)
		frames := p.Run(frameTime, 60)
		p.Close()
		paintedWithin(t, tc.name, frames, false, 0, true)
		if sc.Offset() != tc.want {
			t.Fatalf("%s reached %v, want %v", tc.name, sc.Offset(), tc.want)
		}
	}
}

// A snapping scroll settles once the wheel stops, within its viewport: the
// frames that wait on the pause read the clock alone and are painted in
// part, and the eased settle is contained as the wheel's move was.
func TestAScrollSettlingOnASnapPaintsWithinItsViewport(t *testing.T) {
	var sc ScrollController
	p := scrolledList(&sc, func(s *ScrollWidget) { s.Snap(20) })
	defer p.Close()
	atRest(t, p)
	p.Scroll(Pt(50, 50), Pt(0, -0.6)) // 12 pixels down
	// The wheel's frame paints what the pointer reached with pointerReach
	// around it, and what reads the clock is painted with clockReach.
	paintedWithin(t, "the wheel", []WindowFrame{p.Step(frameTime)}, false, pointerReach, false)
	frames := p.Idle(frameTime, time.Second)
	paintedWithin(t, "snap", frames, true, clockReach, true)
	if !slices.ContainsFunc(frames, func(fr WindowFrame) bool { return fr.Painted == FrameContained }) {
		t.Fatalf("no frame of the settle was contained: %v", frames)
	}
	if sc.Offset() != 20 {
		t.Fatalf("the scroll settled at %v, want 20", sc.Offset())
	}
}

// A dragged scrollbar thumb moves the content within the viewport alone;
// what the pointer touched, the thumb, is painted again with pointerReach
// around it as for any pointer event.
func TestAScrollsThumbDraggedPaintsWithinItsViewport(t *testing.T) {
	var sc ScrollController
	p := scrolledList(&sc, nil)
	defer p.Close()
	atRest(t, p)
	p.Press(Pt(97, 5))
	p.Step(frameTime) // pressing focuses and is painted whole
	var frames []WindowFrame
	for i := 1; i <= 4; i++ {
		p.Move(Pt(97, 5+float64(i)*10))
		frames = append(frames, p.Step(frameTime))
	}
	// Held, the thumb asks for frames until it is let go.
	paintedWithin(t, "thumb", frames, false, pointerReach, false)
	p.Release(Pt(97, 45))
	atRest(t, p)
	travel := 100 - 16.0 // the track less the shortest thumb
	if want := 40 * sc.MaxOffset() / travel; sc.Offset() != want {
		t.Fatalf("the drag moved the scroll to %v, want %v", sc.Offset(), want)
	}
}

// A scroll following the end of a log that grows paints the growth whole,
// which it changes the layout for, and the move to the new end it asks for
// within its viewport, the rows that move brings into view mounting there.
func TestAScrollFollowingTheEndMovesWithinItsViewport(t *testing.T) {
	lines := State([]int{})
	for i := range 30 {
		lines.Set(append(lines.Get(), i))
	}
	var sc ScrollController
	p := NewWindowProbe(func() Widget {
		// A line learns its height as it is laid out, as a chat's turn does:
		// one mounting writes it.
		line := func(EachItem[int]) Widget {
			h := State(10.0)
			return View(h, func(got float64) Widget {
				return FromFuncs(func(Constraints, Env) Size {
					h.Set(20)
					return Sz(100, got)
				}, func(*Canvas, Rect) {})
			})
		}
		log := EachKeyed(lines, func(i int) int { return i }, line).EstimatedExtent(20)
		return Row(Box(Scroll(log).FollowEnd().Controller(&sc)).Size(100, 100), Box().Size(100, 100))
	}, Sz(200, 100))
	defer p.Close()
	atRest(t, p)
	// A line at a time, and a page and more of them at once.
	for _, n := range []int{1, 1, 20, 1} {
		before := sc.Offset()
		more := slices.Clone(lines.Get())
		for range n {
			more = append(more, len(more))
		}
		lines.Set(more)
		frames := p.Run(frameTime, 10)
		if frames[0].Painted != FrameWhole {
			t.Fatalf("%d lines: the log's growth was painted %v, want whole", n, frames[0].Painted)
		}
		paintedWithin(t, "FollowEnd", frames[1:], false, 0, false)
		if frames[len(frames)-1].Next {
			t.Fatalf("%d lines: the window did not come to rest: %v", n, frames)
		}
		if sc.Offset() <= before || sc.Offset() != sc.MaxOffset() {
			t.Fatalf("%d lines: the scroll moved from %v to %v, want on to the end, %v", n, before, sc.Offset(), sc.MaxOffset())
		}
	}
}

// A move a scroll makes as it lays out, following the end, asks for the
// layout of the next frame, which lays the content out for the new window:
// what the content does there, noting the window as a virtualized list
// mounts the rows it brings, is contained as the move was.
func TestTheLayoutAScrollsOwnMoveAsksForIsContained(t *testing.T) {
	n := State(10)
	window := State(0.0)
	var sc ScrollController
	p := NewWindowProbe(func() Widget {
		content := View(n, func(k int) Widget {
			return FromFuncs(func(c Constraints, env Env) Size {
				vp, _ := ScrollViewport(env)
				window.Set(vp.Offset)
				return Sz(100, float64(k)*20)
			}, func(*Canvas, Rect) {})
		})
		return Row(Box(Scroll(content).FollowEnd().Controller(&sc)).Size(100, 100), Box().Size(100, 100))
	}, Sz(200, 100))
	defer p.Close()
	atRest(t, p)
	n.Set(20)
	frames := p.Run(frameTime, 10)
	if len(frames) < 2 || frames[0].Painted != FrameWhole {
		t.Fatalf("the content's growth was painted %v, want whole and a frame after", frames)
	}
	paintedWithin(t, "the layout after", frames[1:], false, 0, true)
	if window.Get() != 300 || sc.Offset() != 300 {
		t.Fatalf("the content was laid out for %v, the scroll at %v; want both at the end, 300", window.Get(), sc.Offset())
	}
}

// A widget beside a scroll that reads its bound offset is painted when what
// it shows changes as a ScrollController eases the scroll: that frame is
// whole, and those it reads nothing new in are contained.
func TestAReaderOfAnAnimatedScrollsOffsetBesideItIsPainted(t *testing.T) {
	offset := State(0.0)
	builds := 0
	var sc ScrollController
	p := NewWindowProbe(func() Widget {
		rows := make([]Widget, 40)
		for i := range rows {
			rows[i] = Box().Size(100, 20)
		}
		far := Map(offset, func(o float64) bool { return o > 100 })
		side := View(far, func(bool) Widget { builds++; return Box().Size(100, 100) })
		return Row(Box(Scroll(Column(rows...)).BindOffset(offset).Controller(&sc)).Size(100, 100), side)
	}, Sz(200, 100))
	defer p.Close()
	p.Step(0)
	sc.ScrollTo(300)
	crossings := 0
	for k := 0; ; k++ {
		if k > 60 {
			t.Fatal("the scroll did not come to rest")
		}
		built := builds
		fr := p.Step(frameTime)
		crossed := builds != built
		if crossed {
			crossings++
		}
		if whole := fr.Painted == FrameWhole; crossed != whole || !whole && fr.Painted != FrameContained {
			t.Fatalf("frame %d at %v: the reader beside was rebuilt %v, the frame painted %v", k, offset.Get(), crossed, fr.Painted)
		}
		if !fr.Next {
			break
		}
	}
	if crossings != 1 || offset.Get() != 300 {
		t.Fatalf("the reader was rebuilt %d times on the way to %v, want once on the way to 300", crossings, offset.Get())
	}
}

// A scroll whose viewport follows its content's size, which here changes
// with the offset, moves what lies below it as it moves: those frames are
// painted whole. One that lays it out at the offset it was at, its height
// as it was, moves nothing and may be contained.
func TestAnAnimatedScrollSizedByItsContentIsNotHeldThere(t *testing.T) {
	var h float64
	content := FromFuncs(func(c Constraints, env Env) Size {
		vp, _ := ScrollViewport(env)
		h = 20 + vp.Offset/10
		return Sz(400, h)
	}, func(*Canvas, Rect) {})
	var sc ScrollController
	p := NewWindowProbe(func() Widget {
		// Inside a vertical scroll, the horizontal one is as tall as its
		// content, and the box below it moves as it grows.
		inner := Scroll(content).Horizontal().Controller(&sc)
		return Row(Box(Scroll(Column(inner, Box().Size(100, 400)))).Size(100, 100), Box().Size(100, 100))
	}, Sz(200, 100))
	defer p.Close()
	atRest(t, p)
	sc.ScrollTo(300)
	var frames []WindowFrame
	for i := range 60 {
		was := h
		fr := p.Step(frameTime)
		frames = append(frames, fr)
		if h != was && fr.Painted == FrameContained {
			t.Fatalf("frame %d, the scroll's height changing from %v to %v, was contained: %v", i, was, h, frames)
		}
		if !fr.Next {
			break
		}
	}
	if sc.Offset() != 300 || frames[len(frames)-1].Next {
		t.Fatalf("the scroll reached %v, want 300 and at rest: %v", sc.Offset(), frames)
	}
}

// A scroll under a Transform cannot tell where its viewport is in the
// window: its eased move is painted whole.
func TestAnAnimatedScrollUnderATransformPaintsWhole(t *testing.T) {
	var sc ScrollController
	p := NewWindowProbe(func() Widget {
		rows := make([]Widget, 40)
		for i := range rows {
			rows[i] = Box().Size(100, 20)
		}
		return Row(Transform(Box(Scroll(Column(rows...)).Controller(&sc)).Size(100, 100)).Rotate(0.01), Box().Size(100, 100))
	}, Sz(200, 100))
	defer p.Close()
	p.Step(0)
	sc.ScrollTo(300)
	frames := p.Run(frameTime, 60)
	for i, fr := range frames {
		if fr.Painted != FrameWhole {
			t.Fatalf("frame %d was painted %v, want whole: %v", i, fr.Painted, frames)
		}
	}
	if sc.Offset() != 300 {
		t.Fatalf("the scroll reached %v, want 300", sc.Offset())
	}
}
