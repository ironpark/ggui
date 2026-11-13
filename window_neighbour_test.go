package ggui

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// What a window's own input reached decides how much of its frame is
// painted again, and input handled in another world at the same moment,
// as a probe on another goroutine handles its own, says nothing of it: a
// pointer moving over a paragraph still paints the frame in part while
// other probes move focus about.
func TestInputInAnotherWorldLeavesAWindowsFramePartial(t *testing.T) {
	var done atomic.Bool
	var neighbours sync.WaitGroup
	for range 4 {
		neighbours.Go(func() {
			a, b := Focus(Box().Size(50, 50)), Focus(Box().Size(50, 50))
			p := NewProbe(Column(a, b), Sz(50, 100))
			defer p.Close()
			for !done.Load() {
				p.Click(Pt(25, 25))
				p.Click(Pt(25, 75))
			}
		})
	}
	defer neighbours.Wait()
	defer done.Store(true)

	var texts []Widget
	for range 8 {
		texts = append(texts, Text("an item of a list").Selectable())
	}
	area := SelectionArea(Scroll(Column(texts...).Gap(30).Stretch()))
	p := NewWindowProbe(func() Widget { return area }, Sz(300, 500))
	defer p.Close()
	step := func() WindowFrame { return p.Step(16 * time.Millisecond) }
	step()
	p.Idle(16*time.Millisecond, time.Second)
	for i := range 400 {
		w := &texts[2+i%4].(*TextWidget).rich.w
		p.Move(textAt(w, w.rect, 3))
		if fr := step(); fr.Painted != FramePartial {
			t.Fatalf("hover %d painted the frame %v while other worlds handled input, want it in part", i, fr.Painted)
		}
	}
}
