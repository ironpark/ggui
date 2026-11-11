package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// refresh is a display's, the time between a window's frames.
const refresh = 16 * time.Millisecond

// galleryWindow runs the gallery as a window of size, for what its frames
// cost with no input coming.
func galleryWindow(size ggui.Size) *ggui.WindowProbe {
	var p *ggui.WindowProbe
	dispose := ggui.Root(func() {
		build, setup, _ := newGallery()
		p = ggui.NewWindowProbe(build, size)
		p.Window().Setup(setup)
	})
	p.Window().Setup(func() { ggui.OnCleanup(dispose) })
	p.Step(0)
	return p
}

// idleCost runs a second of p's frames with no input and fails unless
// they are what loops on screen cost: woken at a loop's pace, at most 30
// times a second, never asking for the next at every refresh, painted in
// part within no more than share of the window.
func idleCost(t *testing.T, p *ggui.WindowProbe, size ggui.Size, share float64, where string) []ggui.WindowFrame {
	t.Helper()
	frames := p.Idle(refresh, time.Second)
	if len(frames) > 31 {
		t.Fatalf("%s: %d frames in a second at rest, want no more than a loop's 30", where, len(frames))
	}
	for i, fr := range frames {
		if fr.Next {
			t.Fatalf("%s: frame %d at rest asked for the next at the display's refresh", where, i)
		}
		if fr.Painted != ggui.FramePartial {
			t.Fatalf("%s: frame %d at rest was painted %v, want in part", where, i, fr.Painted)
		}
		area := 0.0
		for _, d := range fr.Damage {
			area += d.Size.W * d.Size.H
		}
		if area > share*size.W*size.H {
			t.Fatalf("%s: frame %d at rest painted %v, %.0f of the window's %.0f", where, i, fr.Damage, area, size.W*size.H)
		}
	}
	return frames
}

// The gallery as first shown, with no input, paints only its spinner: at
// the spinner's pace, in part, within the spinner. Nothing else on the
// page asks for a frame.
func TestTheGalleryAtRestPaintsOnlyItsSpinner(t *testing.T) {
	size := ggui.Sz(1180, 820)
	p := galleryWindow(size)
	defer p.Close()
	p.Idle(refresh, 3*time.Second)
	frames := idleCost(t, p, size, .01, "the first page")
	if len(frames) < 25 {
		t.Fatalf("%d frames in a second, want the spinner in view turning at its pace", len(frames))
	}
}

// Scrolled from top to bottom, wherever the gallery comes to rest, what
// shows costs frames only at a loop's pace and painted in part: a spinner,
// a pulsing skeleton or an upload's shimmer. The carousel at the bottom
// asks for none.
func TestTheGalleryAtRestAnywhereCostsOnlyItsLoops(t *testing.T) {
	size := ggui.Sz(1180, 820)
	p := galleryWindow(size)
	defer p.Close()
	p.Idle(refresh, 3*time.Second)
	carousel := false
	for i := range 40 {
		p.Scroll(ggui.Pt(600, 500), ggui.Pt(0, -20))
		p.Step(refresh)
		p.Idle(refresh, 3*time.Second)
		idleCost(t, p, size, .05, fmt.Sprintf("scrolled %d times", i+1))
		if f, ok := p.Find("Carousel"); ok && f.Rect.Origin.Y >= 0 && f.Rect.Origin.Y+f.Rect.Size.H < size.H {
			carousel = true
			if frames := p.Idle(refresh, time.Second); len(frames) != 0 {
				t.Fatalf("with the carousel in view the gallery ran %d frames at rest", len(frames))
			}
		}
	}
	if !carousel {
		t.Fatal("the gallery never scrolled the carousel into view")
	}
}
