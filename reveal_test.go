package ggui

import (
	"testing"
	"time"
)

// counted is a child 80 by 40 that counts its layouts and paints, and its
// taps.
type counted struct{ laid, painted, taps int }

func (k *counted) widget() Widget {
	return Tap(FromFuncs(func(c Constraints, _ Env) Size {
		k.laid++
		return c.Constrain(Sz(80, 40))
	}, func(*Canvas, Rect) { k.painted++ }), func() { k.taps++ })
}

// revealing runs p's frames 16ms apart until the window rests, and checks
// that the height h keeps moving one way from one end to the other through
// several heights between.
func revealing(t *testing.T, p *WindowProbe, h *float64, from, to float64) []WindowFrame {
	t.Helper()
	var heights []float64
	var frames []WindowFrame
	for range 60 {
		fr := p.Step(16 * time.Millisecond)
		frames = append(frames, fr)
		heights = append(heights, *h)
		if !fr.Next {
			break
		}
	}
	if frames[len(frames)-1].Next || *h != to {
		t.Fatalf("came to %v over %d frames, want to rest at %v: %v", *h, len(frames), to, heights)
	}
	between := 0
	for i, got := range heights {
		if min(from, to) < got && got < max(from, to) {
			between++
		}
		if i > 0 && (got-heights[i-1])*(to-from) < 0 {
			t.Fatalf("went back from %v to %v: %v", heights[i-1], got, heights)
		}
	}
	if between < 3 {
		t.Fatalf("%d frames between %v and %v, want several: %v", between, from, to, heights)
	}
	return frames
}

// heightOf wraps w to keep the height it was last laid out at.
func heightOf(w Widget, h *float64) Widget {
	return FromFuncs(func(c Constraints, e Env) Size {
		s := w.Layout(c, e)
		*h = s.H
		return s
	}, func(dst *Canvas, r Rect) { dst.Paint(w, r) })
}

// A Reveal eases open and closed in a window's own frames and rests at
// each end. Closed, its child is neither laid out nor painted; closing,
// the child is painted but takes no input.
func TestARevealEasesOpenAndClosedInAWindowsFrames(t *testing.T) {
	open := State(false)
	var k counted
	var h float64
	r := Reveal(open, k.widget())
	p := NewWindowProbe(func() Widget { return topLeft(Column(heightOf(r, &h), Box().Size(80, 10))) }, Sz(200, 200))
	defer p.Close()
	p.Run(16*time.Millisecond, 10)
	if h != 0 || k.laid != 0 || k.painted != 0 {
		t.Fatalf("closed at first: %v tall, child laid out %d times, painted %d", h, k.laid, k.painted)
	}

	open.Set(true)
	revealing(t, p, &h, 0, 40)
	if r.Progress() != 1 {
		t.Fatalf("open, progress %v", r.Progress())
	}

	open.Set(false)
	p.Step(16 * time.Millisecond)
	p.Click(Pt(10, 5))
	p.Step(16 * time.Millisecond)
	if k.taps != 0 {
		t.Fatal("the child took a tap while it folded away")
	}
	revealing(t, p, &h, h, 0)

	laid, painted := k.laid, k.painted
	p.Window().Repaint()
	p.Step(16 * time.Millisecond)
	open.Set(false) // no change: nothing to lay out
	p.Step(16 * time.Millisecond)
	if k.laid != laid || k.painted != painted {
		t.Fatalf("closed, the child was laid out %d and painted %d more times", k.laid-laid, k.painted-painted)
	}
}

// Opened, a Reveal's child takes input as soon as it shows.
func TestAnOpenRevealsChildTakesInput(t *testing.T) {
	open := State(true)
	var k counted
	p := NewWindowProbe(func() Widget { return topLeft(Reveal(open, k.widget())) }, Sz(200, 200))
	defer p.Close()
	p.Run(16*time.Millisecond, 10)
	p.Click(Pt(10, 5))
	p.Step(16 * time.Millisecond)
	if k.taps != 1 {
		t.Fatalf("the open child took %d taps, want 1", k.taps)
	}
}

// Under reduced motion a Reveal opens and closes at once.
func TestARevealUnderReducedMotionOpensAtOnce(t *testing.T) {
	open := State(false)
	var k counted
	var h float64
	p := NewWindowProbe(func() Widget {
		return Provide(ReducedMotionKey, true, topLeft(heightOf(Reveal(open, k.widget()), &h)))
	}, Sz(200, 200))
	defer p.Close()
	p.Run(16*time.Millisecond, 10)
	open.Set(true)
	if frames := p.Run(16*time.Millisecond, 10); len(frames) != 1 || h != 40 {
		t.Fatalf("opened to %v over %d frames, want 40 in one", h, len(frames))
	}
	open.Set(false)
	if frames := p.Run(16*time.Millisecond, 10); len(frames) != 1 || h != 0 {
		t.Fatalf("closed to %v over %d frames, want 0 in one", h, len(frames))
	}
}

// A horizontal Reveal eases its width and leaves its height to the child.
func TestAHorizontalRevealEasesItsWidth(t *testing.T) {
	open := State(false)
	var k counted
	var w float64
	r := Reveal(open, k.widget()).Horizontal().Duration(100 * time.Millisecond).Easing(EaseLinear)
	p := NewWindowProbe(func() Widget {
		return topLeft(FromFuncs(func(c Constraints, e Env) Size {
			s := r.Layout(c, e)
			w = s.W
			if s.H != 0 && s.H != 40 {
				t.Errorf("a horizontal reveal %v tall", s.H)
			}
			return s
		}, func(dst *Canvas, rect Rect) { dst.Paint(r, rect) }))
	}, Sz(200, 200))
	defer p.Close()
	p.Run(10*time.Millisecond, 10)
	open.Set(true)
	revealing(t, p, &w, 0, 80)
}

// A Reveal easing open in a scrolling pane of fixed size, beside other
// content, is painted within the pane's viewport alone, the frame of the
// write that opened it aside.
func TestARevealEasingInAScrollPaintsWithinItsViewport(t *testing.T) {
	open := State(false)
	var k counted
	var h float64
	p := NewWindowProbe(func() Widget {
		pane := Box(Scroll(Column(heightOf(Reveal(open, k.widget()), &h), Box().Size(100, 400)))).Size(100, 100)
		return Row(pane, Box().Size(100, 100))
	}, Sz(200, 100))
	defer p.Close()
	p.Run(16*time.Millisecond, 10)
	open.Set(true)
	frames := revealing(t, p, &h, 0, 40)
	viewport := Rct(Point{}, Sz(100, 100))
	for i, fr := range frames[1:] {
		if fr.Painted != FrameContained || len(fr.Damage) == 0 {
			t.Fatalf("frame %d was painted %v, want contained: %v", i+1, fr.Painted, frames)
		}
		for _, d := range fr.Damage {
			if d.Empty() || d.Intersect(viewport) != d {
				t.Fatalf("frame %d damaged %v, want within the viewport %v", i+1, fr.Damage, viewport)
			}
		}
	}
}
