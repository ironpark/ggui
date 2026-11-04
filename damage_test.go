package ggui

import (
	"image/color"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggwd"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/pkg/geom"
)

// breathing reads the time as it paints, and draws past its Rect.
func breathing(reach float64) Widget {
	return FromFuncs(func(c Constraints, _ Env) Size { return c.Constrain(Sz(10, 10)) }, func(dst *Canvas, r Rect) {
		Now()
		dst.FillRect(geom.Grow(r, reach), color.White)
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
	all := geom.CoverAll(got)
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
	for name := range ReadMotionReport().Dirtiers {
		if strings.Contains(name, "dirty(signal)") && strings.Contains(name, "writeForTheReport") {
			return
		}
	}
	t.Fatalf("no dirtier names the work: %v", ReadMotionReport().Dirtiers)
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
	skipped := func() int64 { return ReadFrameStats().Skipped }
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
	w.seenGen = layoutGen() // as the frame's handler does after drawing
	if !w.motionLost || len(w.motion) != 0 {
		t.Fatalf("motion %v, lost %v: want motion no region says", w.motion, w.motionLost)
	}
	skipped := func() int64 { return ReadFrameStats().Skipped }
	before := skipped()
	w.draw(screen, 1, true, nil)
	if skipped() != before {
		t.Fatal("the frame after motion that could not be placed was skipped")
	}
}
