package ggui

import (
	"image"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/framestats"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/region"
	"github.com/ironpark/ggwd"
)

// breathing reads the time as it paints, and draws past its Rect.
func breathing(reach float64) Widget {
	return FromFuncs(func(c Constraints, _ Env) Size { return c.Constrain(Sz(10, 10)) }, func(dst *Canvas, r Rect) {
		Now()
		dst.FillRect(r.Grow(reach), color.White)
	})
}

func TestATimedWidgetLeavesWhereItDrewAsMotion(t *testing.T) {
	t.Parallel()
	still := Box().Size(100, 10).Fill(color.Black)
	p := NewProbe(topLeft(Column(still, Padding(breathing(3), 20))), Sz(200, 200))
	defer p.Close()
	img := ggfx.NewImage(200, 200)
	defer img.Deallocate()
	p.Draw(img)
	f := p.canvas.fs()
	if len(f.motion) != 1 || f.motionLost {
		t.Fatalf("motion %v, lost %v; want the one breathing widget", f.motion, f.motionLost)
	}
	// At (20, 30), 10 square, drawing 3 beyond it, with culling's pixel.
	if m := f.motion[0]; m.Origin.X > 17 || m.Origin.Y > 27 || m.Size.W < 16 || m.Size.H < 16 || m.Size.W > 20 {
		t.Fatalf("motion %v; want about (17, 27) 16×16", m)
	}
}

func TestAContainerOfATimedWidgetIsNotItselfMotion(t *testing.T) {
	t.Parallel()
	p := NewProbe(topLeft(Box(Padding(breathing(0), 40)).Size(300, 300).Fill(color.Black)), Sz(400, 400))
	defer p.Close()
	p.Frame()
	if m := p.canvas.fs().motion; len(m) != 1 || m[0].Size.W > 20 {
		t.Fatalf("motion %v; want the 10×10 widget alone, not its 300×300 box", m)
	}
}

func TestMotionUnderATransformCannotBePlaced(t *testing.T) {
	t.Parallel()
	moved := FromFuncs(func(c Constraints, env Env) Size { return c.Constrain(Sz(50, 50)) }, func(dst *Canvas, r Rect) {
		dst.Transform(Affine{}.Translate(5, 0), func(dst *Canvas) { dst.Paint(breathing(0), r) })
	})
	p := NewProbe(topLeft(moved), Sz(100, 100))
	defer p.Close()
	p.Frame()
	if f := p.canvas.fs(); !f.motionLost {
		t.Fatalf("motion %v under a transform was placed", f.motion)
	}
}

func TestMotionRegionsMergeWhereTheyMeet(t *testing.T) {
	window := Rct(Point{}, Sz(1000, 1000))
	got := motionRegions([]Rect{
		Rct(Pt(10, 10), Sz(10, 10)), Rct(Pt(22, 10), Sz(10, 10)), // close: one
		Rct(Pt(500, 500), Sz(10, 10)), // apart
		Rct(Pt(-50, -50), Sz(10, 10)), // off the window
	}, window)
	if len(got) != 2 {
		t.Fatalf("regions %v; want the two near ones as one, the far one, and none off the window", got)
	}
	var many []Rect
	for i := range mostMotionRegions + 1 {
		many = append(many, Rct(Pt(float64(i)*100, 0), Sz(10, 10)))
	}
	if got := motionRegions(many, window); len(got) != 1 {
		t.Fatalf("%d regions for %d apart; want one around them all", len(got), len(many))
	}
}

func TestAPartialFrameSkipsWhatItsRegionMisses(t *testing.T) {
	f := &frameState{partial: true, damage: Rct(Pt(100, 100), Sz(10, 10))}
	c := &Canvas{}
	if !f.skips(c, Rct(Pt(300, 300), Sz(20, 20))) {
		t.Fatal("a widget far from the region was painted")
	}
	if f.skips(c, Rct(Pt(100+10+damageReach-5, 100), Sz(20, 20))) {
		t.Fatal("a widget whose shadow may reach the region was skipped")
	}
	if f.skips(&Canvas{space: &paintSpace{}}, Rct(Pt(300, 300), Sz(20, 20))) {
		t.Fatal("a widget under a transform, which may be anywhere, was skipped")
	}
	f.partial = false
	if f.skips(c, Rct(Pt(300, 300), Sz(20, 20))) {
		t.Fatal("a whole frame skipped a widget")
	}
}

func TestRepaintRectQueuesRegionsForTheNextFrame(t *testing.T) {
	w := &Window{}
	w.RepaintRect(Rect{})
	w.RepaintRect(Rct(Pt(10, 10), Sz(20, 20)))
	w.RepaintRect(Rct(Pt(300, 10), Sz(20, 20)))
	if got := w.takeRepaints(); len(got) != 2 {
		t.Fatalf("asked %v; want the two regions, not the empty one", got)
	}
	if got := w.takeRepaints(); len(got) != 0 {
		t.Fatalf("asked %v after they were taken", got)
	}
	for i := range mostRepaints + 1 {
		w.RepaintRect(Rct(Pt(float64(i)*10, 0), Sz(5, 5)))
	}
	got := w.takeRepaints()
	if len(got) > mostRepaints {
		t.Fatalf("%d regions kept apart; want at most %d", len(got), mostRepaints)
	}
	all := region.CoverAll(got)
	if all.Origin.X != 0 || all.Origin.X+all.Size.W != float64(mostRepaints)*10+5 {
		t.Fatalf("regions %v lost some of what was asked", got)
	}
}

func TestWindowRectIsKnownOutsideATransformOnly(t *testing.T) {
	r := Rct(Pt(5, 5), Sz(10, 10))
	if got, ok := (&Canvas{}).WindowRect(r); !ok || got != r {
		t.Fatalf("got %v, %v; want %v", got, ok, r)
	}
	if _, ok := (&Canvas{space: &paintSpace{}}).WindowRect(r); ok {
		t.Fatal("a rect under a transform was placed in the window")
	}
}

// Work posted to a window wakes it without asking for the whole of it, so
// a frame with something moving may still be painted in part; Repaint asks
// for the whole, for a change no signal or layout records.
func TestPostedWorkLeavesTheFrameFreeToBePaintedInPart(t *testing.T) {
	w := newWindow(nil, nil, Config{}, func() Widget { return Box() })
	w.Post(func() {})
	if w.dirty.Load() != 0 {
		t.Fatal("posting work asked for the whole window")
	}
	if !w.hasPosted() || !w.wantsFrame(frameInput{}) {
		t.Fatal("posted work does not get a frame")
	}
	if w.dirty.Load() != 0 {
		t.Fatal("the frame for posted work was taken as dirty")
	}
	w.Repaint()
	if w.dirty.Load() == 0 {
		t.Fatal("Repaint did not ask for the whole window")
	}
}

// Work run while the window draws no frame, as a covered one, asks for the
// whole window only when it changed a signal or the layout: catching up
// settles the layout, so the frame could no longer tell.
func TestDrainedWorkAsksForTheWholeOnlyWhenItChangedSomething(t *testing.T) {
	a := New(Config{}, func() Widget { return Box() })
	defer a.Close()
	a.running = true
	changed := State(0)
	for _, tc := range []struct {
		name  string
		work  func()
		whole bool
	}{
		{"nothing", func() {}, false},
		{"a signal", func() { changed.Set(changed.Get() + 1) }, true},
		{"the layout", reactive.RequestLayout, true},
	} {
		a.dirty.Store(0)
		a.Post(tc.work)
		if err := a.drain(); err != nil {
			t.Fatal(err)
		}
		if (a.dirty.Load() != 0) != tc.whole {
			t.Errorf("work that changed %s: whole window asked %v, want %v", tc.name, a.dirty.Load() != 0, tc.whole)
		}
	}
}

func TestAKeptFrameIsPaintedOverWithWhatChangesNow(t *testing.T) {
	window := Rct(Point{}, Sz(1000, 1000))
	now := []Rect{Rct(Pt(800, 100), Sz(10, 10))}
	if out, why := repaintFor(true, now, window); why != "" || !slices.Equal(out, now) {
		t.Fatalf("kept: %v, %q; want the change now alone", out, why)
	}
	if _, why := repaintFor(false, now, window); why != "not kept" {
		t.Fatalf("not kept: %q; want it painted whole", why)
	}
	if _, why := repaintFor(true, []Rect{Rct(Point{}, Sz(900, 900))}, window); why != "too much changed" {
		t.Fatalf("most of the window: %q; want it painted whole", why)
	}
}

func TestTheReportNamesThePostedWorkThatMadeTheWholeWindowDirty(t *testing.T) {
	a := New(Config{}, func() Widget { return Box() })
	defer a.Close()
	a.running = true
	changed := State(0)
	a.Post(writeForTheReport(changed))
	if err := a.drain(); err != nil {
		t.Fatal(err)
	}
	for name := range framestats.ReadReport().Dirtiers {
		if strings.Contains(name, "dirty(signal)") && strings.Contains(name, "writeForTheReport") {
			return
		}
	}
	t.Fatalf("no dirtier names the work: %v", framestats.ReadReport().Dirtiers)
}

// writeForTheReport is work that writes s.
func writeForTheReport(s *StateValue[int]) func() {
	return func() { s.Set(s.Get() + 1) }
}

// A mouse move asks for the whole window only when the last paint read
// where the pointer is; otherwise what the move reaches is painted again.
func TestAMouseMoveRepaintsTheWholeOnlyWhenPaintReadThePointer(t *testing.T) {
	w := newWindow(nil, nil, Config{}, func() Widget { return Box() })
	if err := w.handle(ggwd.MouseMoveEvent{X: 5, Y: 5}); err != nil {
		t.Fatal(err)
	}
	if w.dirty.Load() != 0 {
		t.Fatal("a mouse move nothing read asked for the whole window")
	}
	w.canvas.Pointer()
	if err := w.handle(ggwd.MouseMoveEvent{X: 6, Y: 6}); err != nil {
		t.Fatal(err)
	}
	if w.dirty.Load() == 0 {
		t.Fatal("a mouse move did not ask for the whole window a paint read the pointer in")
	}
}

// The regions pointer events reach are what the frame paints again.
func TestDeliveredPointerEventsNoteTheirRegions(t *testing.T) {
	var in inputState
	r := &hitRegion{rect: Rct(Pt(10, 20), Sz(30, 40)), pointer: takesPointer{}}
	in.deliver(r, PointerEvent{Kind: PointerEnter})
	got, placed := in.takeTouched()
	if !placed || len(got) != 1 || got[0] != r.rect {
		t.Fatalf("touched %v, %v", got, placed)
	}
	r.space = &paintSpace{}
	in.deliver(r, PointerEvent{Kind: PointerExit})
	if _, placed := in.takeTouched(); placed {
		t.Fatal("a region under a transform was taken as placed")
	}
}

// takesPointer consumes every pointer event.
type takesPointer struct{}

func (takesPointer) HandlePointer(PointerEvent) bool { return true }

// A move is noted only where it was claimed, and only when the pointer
// moved since the last frame's: each frame hands one to the regions
// under a pointer at rest.
func TestAMoveIsNotedOnlyWhenThePointerMovedAndItWasClaimed(t *testing.T) {
	var in inputState
	r := &hitRegion{rect: Rct(Pt(0, 0), Sz(10, 10)), pointer: takesPointer{}}
	in.moved = true
	in.deliver(r, PointerEvent{Kind: PointerMove})
	if got, _ := in.takeTouched(); len(got) != 1 {
		t.Fatalf("a claimed move after the pointer moved noted %v", got)
	}
	in.moved = false
	in.deliver(r, PointerEvent{Kind: PointerMove})
	if got, _ := in.takeTouched(); len(got) != 0 {
		t.Fatalf("a move of a pointer at rest noted %v", got)
	}
	in.moved = true
	in.deliver(&hitRegion{rect: r.rect, pointer: passesPointer{}}, PointerEvent{Kind: PointerMove})
	if got, _ := in.takeTouched(); len(got) != 0 {
		t.Fatalf("a move passed on noted %v", got)
	}
}

// passesPointer claims no pointer event.
type passesPointer struct{}

func (passesPointer) HandlePointer(PointerEvent) bool { return false }

// A frame for which nothing changed, as for posted work that wrote
// nothing, paints nothing over the kept screen, and a Repaint paints it.
func TestAFrameNothingChangedForPaintsNothing(t *testing.T) {
	w := newWindow(nil, nil, Config{}, func() Widget { return Box() })
	screen := ggfx.NewImage(16, 16)
	w.draw(screen, 1, false, nil)
	skipped := func() int64 { return framestats.Read().Skipped }
	before := skipped()
	w.draw(screen, 1, true, nil)
	if skipped() != before+1 {
		t.Fatal("a frame nothing changed for was painted")
	}
	w.Repaint()
	w.draw(screen, 1, true, nil)
	if skipped() != before+1 {
		t.Fatal("a frame after Repaint painted nothing")
	}
	w.draw(screen, 1, false, nil)
	if skipped() != before+1 {
		t.Fatal("a frame on a screen not kept painted nothing")
	}
}

// A frame after one whose motion could not be placed, as a dialog's,
// which takes no room and paints its panel over the window, is painted
// whole, not skipped as one nothing changed for: the dialog would stand
// half faded until the next input.
func TestAFrameAfterMotionNotPlacedIsPainted(t *testing.T) {
	dialog := FromFuncs(func(c Constraints, _ Env) Size { return Size{} }, func(dst *Canvas, r Rect) { Now() })
	w := newWindow(nil, nil, Config{}, func() Widget { return Stack(Box().Size(10, 10), dialog) })
	w.start()
	setRunning(&w.frameLoop)
	screen := ggfx.NewImage(16, 16)
	w.draw(screen, 1, false, nil)
	w.endFrame(frameInput{})
	if !w.motionLost || len(w.motion) != 0 {
		t.Fatalf("motion %v, lost %v: want motion no region says", w.motion, w.motionLost)
	}
	skipped := func() int64 { return framestats.Read().Skipped }
	before := skipped()
	w.draw(screen, 1, true, nil)
	if skipped() != before {
		t.Fatal("the frame after motion that could not be placed was skipped")
	}
}

// containedWindow is a window of a scroll of tall content beside a box,
// drawn once whole.
func containedWindow(t *testing.T) (*Window, *ggfx.Image, *StateValue[int]) {
	t.Helper()
	return containedWindowBeside(t, nil)
}

// containedWindowBeside is containedWindow with beside, when not nil, in
// place of the box.
func containedWindowBeside(t *testing.T, beside Widget) (*Window, *ggfx.Image, *StateValue[int]) {
	t.Helper()
	other := State(0)
	w := newWindow(nil, nil, Config{}, func() Widget {
		rows := make([]Widget, 40)
		for i := range rows {
			rows[i] = Box().Size(100, 20)
		}
		side := beside
		if side == nil {
			side = View(other, func(int) Widget { return Box().Size(100, 100) })
		}
		return Row(Box(Scroll(Column(rows...))).Size(100, 100), side)
	})
	w.start()
	setRunning(&w.frameLoop)
	screen := ggfx.NewImage(200, 100)
	w.draw(screen, 1, false, nil)
	return w, screen, other
}

// A wheel's move over a scroll paints the frame within the scroll's
// viewport alone, its regions registered as a whole frame's are.
func TestAScrollsMovePaintsWithinItsViewport(t *testing.T) {
	w, screen, _ := containedWindow(t)
	setRunning(&w.frameLoop)
	w.dispatchInput(frameInput{pos: Pt(50, 50), wheel: Pt(0, -30)})
	before := framestats.Read().Contained
	var damage []image.Rectangle
	w.draw(screen, 1, true, func(rs []image.Rectangle) { damage = rs })
	if framestats.Read().Contained != before+1 {
		t.Fatalf("the scroll's frame was not painted within its viewport; damage %v", damage)
	}
	if len(damage) != 1 || !damage[0].In(image.Rect(0, 0, 100, 100)) || damage[0].Empty() {
		t.Fatalf("damage %v, want within the scroll's viewport", damage)
	}
	if len(w.input.regions) == 0 {
		t.Fatal("the frame registered no regions")
	}
}

// A write outside the scroll in the same frame paints all of it.
func TestAScrollsMoveWithAnotherChangePaintsWhole(t *testing.T) {
	w, screen, other := containedWindow(t)
	setRunning(&w.frameLoop)
	w.dispatchInput(frameInput{pos: Pt(50, 50), wheel: Pt(0, -30)})
	other.Set(1)
	before := framestats.Read().Contained
	var damage []image.Rectangle
	w.draw(screen, 1, true, func(rs []image.Rectangle) { damage = rs })
	if framestats.Read().Contained != before {
		t.Fatal("a frame with another change was painted within the scroll")
	}
	if len(damage) != 1 || damage[0] != screen.Bounds() {
		t.Fatalf("damage %v, want all of the screen", damage)
	}
	// The write is named as what kept the frame whole.
	const writer = "state changed github.com/ironpark/ggui.TestAScrollsMoveWithAnotherChangePaintsWhole"
	if framestats.ReadReport().Dirtiers[writer] == 0 {
		t.Fatalf("the write outside the scroll was not named: %v", framestats.ReadReport().Dirtiers)
	}
}

// A widget beside a scroll is told that a frame painted within the
// scroll's viewport does not show it, and that a whole one does.
func TestAWidgetBesideAScrollsMoveIsNotShown(t *testing.T) {
	var shown []bool
	beside := Box(FromFuncs(func(c Constraints, _ Env) Size { return c.Max() }, func(dst *Canvas, r Rect) { shown = append(shown, dst.Shows(r)) })).Size(100, 100)
	w, screen, _ := containedWindowBeside(t, beside)
	if len(shown) != 1 || !shown[0] {
		t.Fatalf("a whole frame showed the widget %v, want [true]", shown)
	}
	setRunning(&w.frameLoop)
	w.dispatchInput(frameInput{pos: Pt(50, 50), wheel: Pt(0, -30)})
	before := framestats.Read().Contained
	w.draw(screen, 1, true, func([]image.Rectangle) {})
	if framestats.Read().Contained != before+1 {
		t.Fatal("the scroll's frame was not painted within its viewport")
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
	w := newWindow(nil, nil, Config{}, func() Widget {
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
	})
	w.start()
	setRunning(&w.frameLoop)
	screen := ggfx.NewImage(200, 100)
	for range 3 {
		w.draw(screen, 1, false, nil)
	}
	for k := range 60 {
		setRunning(&w.frameLoop)
		w.dispatchInput(frameInput{pos: Pt(50, 50), wheel: Pt(0, 3)})
		before := framestats.Read().Contained
		w.draw(screen, 1, true, func([]image.Rectangle) {})
		if framestats.Read().Contained != before+1 {
			t.Fatalf("move %d was painted whole: %v %v", k, framestats.ReadReport().Refusals, framestats.ReadReport().Dirtiers)
		}
	}
}

// A widget beside a scroll that reads its offset, as a button to the end
// does, is painted when what it shows changes: that frame is whole.
func TestAReaderOfAScrollsOffsetBesideItIsPainted(t *testing.T) {
	offset := State(0.0)
	builds := 0
	w := newWindow(nil, nil, Config{}, func() Widget {
		rows := make([]Widget, 40)
		for i := range rows {
			rows[i] = Box().Size(100, 20)
		}
		far := Map(offset, func(o float64) bool { return o > 100 })
		side := View(far, func(bool) Widget { builds++; return Box().Size(100, 100) })
		return Row(Box(Scroll(Column(rows...)).BindOffset(offset)).Size(100, 100), side)
	})
	w.start()
	setRunning(&w.frameLoop)
	screen := ggfx.NewImage(200, 100)
	w.draw(screen, 1, false, nil)
	for k := 0; offset.Get() <= 100; k++ {
		if k > 20 {
			t.Fatal("the scroll did not move")
		}
		setRunning(&w.frameLoop)
		w.dispatchInput(frameInput{pos: Pt(50, 50), wheel: Pt(0, -1)})
		before, built := framestats.Read().Contained, builds
		w.draw(screen, 1, true, func([]image.Rectangle) {})
		contained := framestats.Read().Contained != before
		if crossed := builds != built; crossed == contained {
			t.Fatalf("move %d to %v: the reader beside was rebuilt %v, the frame contained %v", k, offset.Get(), crossed, contained)
		}
	}
}

// A widget that eases its size, as a disclosure opening does, asks while
// it paints for the layout that moves it on. The window runs that layout in
// a frame of its own and paints it whole: the frame would otherwise be
// taken as one nothing changed for, and the widget would stand where it
// was until the next input.
func TestALayoutAskedForWhilePaintingGetsAWholeFrame(t *testing.T) {
	const steps = 4
	grows, laid, painted := easing(steps)
	p := NewWindowProbe(func() Widget { return topLeft(grows) }, Sz(16, 16))
	defer p.Close()
	frames := p.Run(0, 2*steps+1)
	if *laid != steps || *painted != steps {
		t.Fatalf("laid out %d times and painted the %dth; want %d of each, with no input", *laid, *painted, steps)
	}
	for i, fr := range frames {
		if fr.Painted != FrameWhole {
			t.Fatalf("frame %d was painted %v, want whole: %v", i, fr.Painted, frames)
		}
	}
	if frames[len(frames)-1].Next {
		t.Fatal("the window still asks for frames after the layout settled")
	}
}

// easing is a widget that grows a step taller at each layout until it is
// steps tall, asking for the next layout as it paints, the way a disclosure
// easing open does. It reports the layouts run and the last one painted.
func easing(steps int) (w Widget, laid, painted *int) {
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
// the whole window.
func TestALayoutEasedInsideAScrollPaintsWithinItsViewport(t *testing.T) {
	const steps = 5
	grows, laid, painted := easing(steps)
	p := NewWindowProbe(func() Widget {
		return Row(Box(Scroll(Column(grows, Box().Size(100, 400)))).Size(100, 100), Box().Size(100, 100))
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
	if len(p.Window().input.regions) == 0 {
		t.Fatal("the frames registered no regions")
	}
}

// A scroll as long as its content moves what lies below it when the
// content grows: the change is not held there, and the frame is painted
// whole, or within a scroll around it that keeps its size.
func TestALayoutEasedInsideAScrollSizedByItsContentIsNotHeldThere(t *testing.T) {
	const steps = 6
	// A scroll no longer than its content, with a box below it that the
	// content's growth moves.
	sized := func(w Widget) Widget { return Column(Scroll(w).Shrink(), Box().Size(50, 50)) }
	for _, tc := range []struct {
		name   string
		around func(Widget) Widget
		whole  bool
	}{
		{"alone", func(w Widget) Widget { return topLeft(sized(w)) }, true},
		{"in a scroll that keeps its size", func(w Widget) Widget {
			return Row(Box(Scroll(Column(sized(w), Box().Size(100, 400)))).Size(100, 100), Box().Size(100, 100))
		}, false},
	} {
		grows, laid, painted := easing(steps)
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

// frameTime is a 60Hz refresh.
const frameTime = 16 * time.Millisecond

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
	// The wheel's frame paints what the pointer reached with damageReach
	// around it, and what reads the clock is painted with motionSlack.
	paintedWithin(t, "the wheel", []WindowFrame{p.Step(frameTime)}, false, damageReach, false)
	frames := p.Idle(frameTime, time.Second)
	paintedWithin(t, "snap", frames, true, motionSlack, true)
	if !slices.ContainsFunc(frames, func(fr WindowFrame) bool { return fr.Painted == FrameContained }) {
		t.Fatalf("no frame of the settle was contained: %v", frames)
	}
	if sc.Offset() != 20 {
		t.Fatalf("the scroll settled at %v, want 20", sc.Offset())
	}
}

// A dragged scrollbar thumb moves the content within the viewport alone;
// what the pointer touched, the thumb, is painted again with damageReach
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
	paintedWithin(t, "thumb", frames, false, damageReach, false)
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
