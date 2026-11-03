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
	other := State(0)
	w := newWindow(nil, nil, Config{}, func() Widget {
		rows := make([]Widget, 40)
		for i := range rows {
			rows[i] = Box().Size(100, 20)
		}
		return Row(Box(Scroll(Column(rows...))).Size(100, 100), View(other, func(int) Widget { return Box().Size(100, 100) }))
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
	t.Logf("contained %v regions %d", w.frameLoop.contained, len(w.input.regions))
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
}
