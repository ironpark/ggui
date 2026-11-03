package ggui

import (
	"image"
	"testing"

	"github.com/ironpark/ggfx"
)

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
	before := ReadFrameStats().Contained
	var damage []image.Rectangle
	w.draw(screen, 1, true, func(rs []image.Rectangle) { damage = rs })
	if ReadFrameStats().Contained != before+1 {
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
	before := ReadFrameStats().Contained
	var damage []image.Rectangle
	w.draw(screen, 1, true, func(rs []image.Rectangle) { damage = rs })
	if ReadFrameStats().Contained != before {
		t.Fatal("a frame with another change was painted within the scroll")
	}
	if len(damage) != 1 || damage[0] != screen.Bounds() {
		t.Fatalf("damage %v, want all of the screen", damage)
	}
	// The write is named as what kept the frame whole.
	const writer = "state changed github.com/ironpark/ggui.TestAScrollsMoveWithAnotherChangePaintsWhole"
	if ReadMotionReport().Dirtiers[writer] == 0 {
		t.Fatalf("the write outside the scroll was not named: %v", ReadMotionReport().Dirtiers)
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
	before := ReadFrameStats().Contained
	w.draw(screen, 1, true, func([]image.Rectangle) {})
	if ReadFrameStats().Contained != before+1 {
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
		before := ReadFrameStats().Contained
		w.draw(screen, 1, true, func([]image.Rectangle) {})
		if ReadFrameStats().Contained != before+1 {
			t.Fatalf("move %d was painted whole: %v %v", k, ReadMotionReport().Refusals, ReadMotionReport().Dirtiers)
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
		before, built := ReadFrameStats().Contained, builds
		w.draw(screen, 1, true, func([]image.Rectangle) {})
		contained := ReadFrameStats().Contained != before
		if crossed := builds != built; crossed == contained {
			t.Fatalf("move %d to %v: the reader beside was rebuilt %v, the frame contained %v", k, offset.Get(), crossed, contained)
		}
	}
}
