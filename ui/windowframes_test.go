package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// frameStep is a display's refresh, the time between a window's frames.
const frameStep = 16 * time.Millisecond

// measuring wraps w to record the height it was last laid out at.
func measuring(w ggui.Widget, h *float64) ggui.Widget {
	return ggui.FromFuncs(func(c ggui.Constraints, e ggui.Env) ggui.Size {
		s := w.Layout(c, e)
		*h = s.H
		return s
	}, func(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(w, r) })
}

// atRest runs p's frames until the window asks for none, or fails.
func atRest(t *testing.T, p *ggui.WindowProbe) {
	t.Helper()
	frames := p.Run(frameStep, 120)
	if last := frames[len(frames)-1]; last.Next {
		t.Fatalf("the window never came to rest: %d frames", len(frames))
	}
}

// easeOpen runs p's frames with no input until the window comes to rest,
// and checks that the height h follows grows from closed to open through
// heights between, then stays.
func easeOpen(t *testing.T, p *ggui.WindowProbe, h *float64, closed, open float64) []ggui.WindowFrame {
	t.Helper()
	var heights []float64
	var frames []ggui.WindowFrame
	for range 120 {
		fr := p.Step(frameStep)
		frames = append(frames, fr)
		heights = append(heights, *h)
		if !fr.Next {
			break
		}
	}
	if frames[len(frames)-1].Next {
		t.Fatalf("the window never came to rest; heights %v", heights)
	}
	if got := heights[len(heights)-1]; got != open {
		t.Fatalf("came to rest %v tall, want %v open; heights %v", got, open, heights)
	}
	between := 0
	for i, got := range heights {
		if got > closed && got < open {
			between++
		}
		if i > 0 && got < heights[i-1] {
			t.Fatalf("height went back from %v to %v; heights %v", heights[i-1], got, heights)
		}
	}
	if between < 3 {
		t.Fatalf("%d frames between closed and open, want it eased over several; heights %v", between, heights)
	}
	return frames
}

// A collapsible opened with no input after it eases open in a window's
// own frames: each asks for the next until it is all the way open, and
// the window then rests. A Probe paints a frame whenever it is told, so
// only a window can tell a frame skipped as one nothing changed for.
func TestCollapsibleEasesOpenInAWindowsFrames(t *testing.T) {
	t.Parallel()
	for _, how := range []string{"written", "clicked"} {
		t.Run(how, func(t *testing.T) {
			open := ggui.State(false)
			var h float64
			c := measuring(ui.Collapsible(open, "More", ggui.Box().Height(100)), &h)
			p := ggui.NewWindowProbe(func() ggui.Widget { return ggui.Column(c) }, ggui.Sz(320, 400))
			defer p.Close()
			atRest(t, p)
			closed := h
			if how == "written" {
				open.Set(true)
			} else {
				f, ok := p.Find("More")
				if !ok {
					t.Fatal("no header to click")
				}
				p.Click(f.Center())
			}
			easeOpen(t, p, &h, closed, closed+100)
			if !ggui.Untrack(open.Get) {
				t.Fatal("the collapsible is not open")
			}
		})
	}
}

// An accordion's section opened with no input eases open in a window's
// own frames, and the window then rests.
func TestAccordionEasesOpenInAWindowsFrames(t *testing.T) {
	t.Parallel()
	open := ggui.State([]string{})
	var h float64
	a := measuring(ui.Accordion(open, ui.AccordionItem("a", "Reveal", ggui.Box().Height(100))), &h)
	p := ggui.NewWindowProbe(func() ggui.Widget { return ggui.Column(a) }, ggui.Sz(320, 400))
	defer p.Close()
	atRest(t, p)
	closed := h
	open.Set([]string{"a"})
	// Open, the section takes its content's height and the space around it.
	var full float64
	q := ggui.NewProbe(ggui.Column(measuring(ui.Accordion(ggui.State([]string{"a"}), ui.AccordionItem("a", "Reveal", ggui.Box().Height(100))), &full)), ggui.Sz(320, 400))
	defer q.Close()
	q.Advance(time.Second)
	q.Advance(time.Second)
	if full <= closed+100 {
		t.Fatalf("an open accordion is %v tall, closed %v", full, closed)
	}
	easeOpen(t, p, &h, closed, full)
}

// A collapsible easing open in a scrolling pane of fixed size, beside
// other content, is painted within the pane's viewport alone: the frames
// that ease it are contained, and their damage lies inside the viewport.
// It asserts how frames are painted, which input to other tests can
// change, so it runs alone.
func TestCollapsibleEasingInAScrollPaintsWithinItsViewport(t *testing.T) {
	open := ggui.State(false)
	var h float64
	c := measuring(ui.Collapsible(open, "More", ggui.Box().Height(100)), &h)
	p := ggui.NewWindowProbe(func() ggui.Widget {
		pane := ggui.Box(ggui.Scroll(ggui.Column(c, ggui.Box().Size(150, 400)))).Size(150, 100)
		return ggui.Row(pane, ggui.Box().Size(150, 100))
	}, ggui.Sz(300, 100))
	defer p.Close()
	atRest(t, p)
	closed := h
	open.Set(true)
	frames := easeOpen(t, p, &h, closed, closed+100)
	viewport := ggui.Rct(ggui.Point{}, ggui.Sz(150, 100))
	// The first frame paints the write, which is not the scroll's; the
	// layouts it asks for as it eases are.
	for i, fr := range frames[1:] {
		if fr.Painted != ggui.FrameContained {
			t.Fatalf("frame %d was painted %v, want contained; frames %v", i+1, fr.Painted, frames)
		}
		for _, d := range fr.Damage {
			if d.Empty() || d.Intersect(viewport) != d {
				t.Fatalf("frame %d damaged %v, want within the viewport %v", i+1, fr.Damage, viewport)
			}
		}
	}
}
