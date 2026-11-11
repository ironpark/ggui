package ui_test

import (
	"fmt"
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

// woken counts the frames that came for a Wake, not as the next of one
// still moving.
func woken(first ggui.WindowFrame, frames []ggui.WindowFrame) int {
	n := 0
	for i, fr := range frames {
		prev := first
		if i > 0 {
			prev = frames[i-1]
		}
		if !prev.Next && fr.At.Equal(prev.Wake) {
			n++
		}
	}
	return n
}

func carouselSlides() []ggui.Widget {
	return []ggui.Widget{ui.Card(ggui.Center(ui.Title("1"))), ui.Card(ggui.Center(ui.Title("2"))), ui.Card(ggui.Center(ui.Title("3")))}
}

// A carousel with nothing to do asks for no frames: it reads the time only
// while a slide glides into place.
func TestACarouselAtRestAsksForNoFrames(t *testing.T) {
	selected := ggui.State(0)
	p := ggui.NewWindowProbe(func() ggui.Widget { return ui.Carousel(selected, carouselSlides()...) }, ggui.Sz(500, 240))
	defer p.Close()
	atRest(t, p)
	if frames := p.Idle(frameStep, time.Second); len(frames) != 0 {
		t.Fatalf("a carousel at rest ran %d frames: %v", len(frames), frames)
	}
	selected.Set(1)
	frames := append([]ggui.WindowFrame{p.Step(frameStep)}, p.Idle(frameStep, 2*time.Second)...)
	if len(frames) < 3 {
		t.Fatalf("the slide glided over %d frames, want several", len(frames))
	}
	if last := frames[len(frames)-1]; last.Next || !last.Wake.IsZero() {
		t.Fatalf("the carousel did not come to rest after gliding over %d frames", len(frames))
	}
}

// An autoplaying carousel wakes the window once at each interval to move
// on, rather than painting every refresh to watch the time; held by the
// pointer it asks for nothing, and let go it counts its interval again.
func TestAnAutoplayingCarouselWakesOnlyAtItsInterval(t *testing.T) {
	const interval = 3 * time.Second
	selected := ggui.State(0)
	moves := 0
	c := ui.Carousel(selected, carouselSlides()...).Autoplay(interval).StopOnInteraction(false).OnChange(func(int) { moves++ })
	p := ggui.NewWindowProbe(func() ggui.Widget { return ggui.Padding(c, 40) }, ggui.Sz(580, 320))
	defer p.Close()
	first := p.Step(0)
	frames := p.Idle(frameStep, 3*interval+interval/2)
	if moves != 3 || woken(first, frames) != 3 {
		t.Fatalf("in %v the carousel moved %d times, woken for %d frames, want 3 of each", 3*interval+interval/2, moves, woken(first, frames))
	}
	// Gliding takes the frames it takes; waiting between slides takes none.
	if glide := len(frames) / 3; glide > int(1500*time.Millisecond/frameStep) {
		t.Fatalf("%d frames for each slide", glide)
	}
	p.Move(ggui.Pt(290, 160))
	if fr := p.Step(frameStep); !fr.Wake.IsZero() {
		t.Fatalf("a hovered carousel asked to be woken at %v", fr.Wake)
	}
	if frames := p.Idle(frameStep, 2*interval); len(frames) != 0 || moves != 3 {
		t.Fatalf("a hovered carousel ran %d frames and moved %d times", len(frames), moves-3)
	}
	p.Move(ggui.Pt(10, 10))
	fr := p.Step(frameStep)
	if want := fr.At.Add(interval); !fr.Wake.Equal(want) {
		t.Fatalf("let go, the carousel woke at %v, want an interval on at %v", fr.Wake, want)
	}
	p.Idle(frameStep, interval+interval/2)
	if moves != 4 {
		t.Fatalf("let go, the carousel moved %d times, want once", moves-3)
	}
}

// A toast that stays until dismissed asks for no frames once it has come
// in; one with a countdown asks for them until it has gone.
func TestAPersistentToastAsksForNoFrames(t *testing.T) {
	toaster := ui.NewToaster()
	p := ggui.NewWindowProbe(func() ggui.Widget { return ggui.Column(toaster) }, ggui.Sz(500, 400))
	defer p.Close()
	atRest(t, p)
	toaster.Push(ui.Toast("Saved", "Stays until dismissed").Duration(0))
	p.Window().Repaint()
	p.Step(frameStep)
	frames := p.Idle(frameStep, 2*time.Second)
	if len(frames) == 0 || len(frames) > 30 || frames[len(frames)-1].Next {
		t.Fatalf("a persistent toast came in over %d frames, want it to rest after", len(frames))
	}
	if toaster.Len() != 1 {
		t.Fatal("the persistent toast went")
	}
	toaster.Push(ui.Toast("Sent", "Counts down").Duration(time.Second))
	p.Window().Repaint()
	p.Step(frameStep)
	frames = p.Idle(frameStep, 3*time.Second)
	if toaster.Len() != 1 || len(frames) == 0 || frames[len(frames)-1].Next {
		t.Fatalf("after %d frames %d toasts remain, want the timed one gone and the window at rest", len(frames), toaster.Len())
	}
}

// Loops showing together step on the same beat: a spinner, a pulsing
// skeleton and an upload's shimmer wake the window at the spinner's pace,
// not at each of theirs in turn, and paint only where they are.
func TestLoopsShowingTogetherShareTheirFrames(t *testing.T) {
	p := ggui.NewWindowProbe(func() ggui.Widget {
		return ggui.Column(
			ui.Spinner(),
			ui.Skeleton(120, 16),
			ui.Attachment("report.pdf", "1 MB").State(ui.AttachmentUploading).Width(220),
			ggui.Box().Size(600, 400),
		)
	}, ggui.Sz(600, 600))
	defer p.Close()
	p.Step(0)
	p.Idle(frameStep, 500*time.Millisecond)
	frames := p.Idle(frameStep, time.Second)
	if len(frames) < 25 || len(frames) > 31 {
		t.Fatalf("%d frames in a second, want the spinner's 30", len(frames))
	}
	for i, fr := range frames {
		if fr.Next || fr.Painted != ggui.FramePartial {
			t.Fatalf("frame %d was painted %v and asked for the next %v; want painted in part, woken for", i, fr.Painted, fr.Next)
		}
		area := 0.0
		for _, d := range fr.Damage {
			area += d.Size.W * d.Size.H
		}
		if area > 600*600/4 {
			t.Fatalf("frame %d painted %v, %.0f of the window's %d", i, fr.Damage, area, 600*600)
		}
	}
}

// A wheel's snap in an attachment strip waits for the wheel to stop with
// no frames, then snaps in one.
func TestAnAttachmentStripSnapsAfterTheWheelWithoutWatching(t *testing.T) {
	var cards []*ui.AttachmentWidget
	for i := range 4 {
		cards = append(cards, ui.Attachment(fmt.Sprintf("File %d", i), "PDF").Width(180))
	}
	group := ui.AttachmentGroup(cards...)
	p := ggui.NewWindowProbe(func() ggui.Widget { return ggui.Column(group) }, ggui.Sz(230, 140))
	defer p.Close()
	atRest(t, p)
	p.Scroll(ggui.Pt(100, 20), ggui.Pt(-8, 0))
	p.Step(frameStep)
	frames := p.Idle(frameStep, time.Second)
	if group.Position() != 192 {
		t.Fatalf("the strip came to %v, want the snap at 192", group.Position())
	}
	if len(frames) > 2 {
		t.Fatalf("%d frames to snap, want the one it waited for: %v", len(frames), frames)
	}
}

// A transcript scrolls a wheel's notch as far as the platform does, eased,
// with the next notch adding to where the last was heading, and follows a
// trackpad at once.
func TestAMessageScrollerEasesAWheelsNotchAndFollowsATrackpad(t *testing.T) {
	rows := ggui.State(transcript(12))
	s := ui.MessageScroller(rows).Height(200).Gap(10)
	p := ggui.NewWindowProbe(func() ggui.Widget { return s }, ggui.Sz(300, 200))
	defer p.Close()
	p.Run(frameStep, 10)
	end := s.Position()
	p.Wheel(ggui.Pt(10, 100), ggui.Pt(0, 1), 100)
	var at []float64
	for range 30 {
		p.Step(frameStep)
		at = append(at, s.Position())
	}
	between := 0
	for _, v := range at {
		if v < end && v > end-100 {
			between++
		}
	}
	if got := at[len(at)-1]; got != end-100 || between < 3 {
		t.Fatalf("a notch of 100 from %v came to rest at %v with %d frames on the way: %v", end, got, between, at)
	}
	p.Wheel(ggui.Pt(10, 100), ggui.Pt(0, 1), 100)
	p.Step(frameStep)
	p.Step(frameStep)
	p.Wheel(ggui.Pt(10, 100), ggui.Pt(0, 1), 100)
	for range 30 {
		p.Step(frameStep)
	}
	if got := s.Position(); got != end-300 {
		t.Fatalf("two more notches came to rest at %v, want %v", got, end-300)
	}
	p.Trackpad(ggui.Pt(10, 100), ggui.Pt(0, 12.5))
	p.Step(frameStep)
	if got := s.Position(); got != end-312.5 {
		t.Fatalf("a trackpad's 12.5 pixels left the transcript at %v in its frame, want %v", got, end-312.5)
	}
}

// A spinner turning beside a button with a tooltip, which reads where the
// pointer is as it paints, is painted in part: the pointer does not move,
// so the button shows what it showed.
func TestASpinnerBesideATooltipIsPaintedInPart(t *testing.T) {
	p := ggui.NewWindowProbe(func() ggui.Widget {
		return ggui.Row(ui.Spinner(), ui.Tooltip(ui.Button("Stop", func() {}), "Stop the work")).Gap(8)
	}, ggui.Sz(400, 300))
	defer p.Close()
	p.Move(ggui.Pt(300, 200))
	p.Run(frameStep, 5)
	for i, fr := range p.Run(frameStep, 20) {
		if fr.Painted != ggui.FramePartial {
			t.Fatalf("frame %d was painted %v, want in part", i, fr.Painted)
		}
	}
}
