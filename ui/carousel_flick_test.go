package ui

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// A drag is released as Embla releases one: a short quick flick goes to
// the slide next to the selected one, a slow drag settles on the nearest
// snap, and a flick after a long drag still moves one slide only.
func TestCarouselReleaseFollowsEmbla(t *testing.T) {
	t.Parallel()
	drag := func(from, to float64, over, rest time.Duration) int {
		c := Carousel(ggui.State(2), carouselSlides(5, 1)...)
		p := ggui.NewProbe(c, ggui.Sz(416, 240))
		defer p.Close()
		p.Advance(0)
		p.Press(ggui.Pt(from, 120))
		p.Advance(over)
		p.Move(ggui.Pt(to, 120))
		p.Advance(rest)
		p.Release(ggui.Pt(to, 120))
		p.Advance(time.Second)
		return c.Selected()
	}
	for _, c := range []struct {
		name       string
		from, to   float64
		over, rest time.Duration
		want       int
	}{
		// 60px in 100ms is 600px/s: 180px of carry, past a fifth of the view.
		{"a short flick on", 300, 240, 100 * time.Millisecond, 10 * time.Millisecond, 3},
		{"a short flick back", 240, 300, 100 * time.Millisecond, 10 * time.Millisecond, 1},
		// The same 60px with the pointer at rest before it lets go.
		{"a short slow drag", 300, 240, 100 * time.Millisecond, 300 * time.Millisecond, 2},
		// More than half a slide, slowly: the nearest snap is the next.
		{"a long slow drag", 340, 80, 100 * time.Millisecond, 300 * time.Millisecond, 3},
		// 20px in 100ms is 200px/s: 60px of carry nudges and no more.
		{"a gentle flick", 300, 280, 100 * time.Millisecond, 10 * time.Millisecond, 2},
	} {
		if got := drag(c.from, c.to, c.over, c.rest); got != c.want {
			t.Errorf("%s: selected %d, want %d", c.name, got, c.want)
		}
	}
}
