package widget

import (
	"time"

	"github.com/ironpark/ggui"
)

// Overscroll says what a Scroll does when a touch pan, a fling or the wheel
// pushes past either end of its content.
type Overscroll uint8

const (
	// OverscrollClamp stops at the end. It is the default.
	OverscrollClamp Overscroll = iota
	// OverscrollBounce lets the content follow past the end with growing
	// resistance and springs it back once the input stops, as iOS does.
	OverscrollBounce
	// OverscrollStretch stretches the content along the axis from the end
	// it is pulled at, instead of moving it, and relaxes it the same way.
	OverscrollStretch
)

// scrollPhysics is the part of a ScrollWidget that moves on its own:
// overscroll, snapping and the animated scrolls a ScrollController asks for.
type scrollPhysics struct {
	mode   Overscroll
	snap   float64 // snap interval; 0 snaps nowhere
	paging bool    // snap to the viewport's extent
	ctrl   *ScrollController

	scrollMotionState // carried across a rebuild by adopt

	applied *scrollRequest // what attach applied in this layout, until paint
	painted bool           // painted, so adopted what it replaces: requests apply at once
}

// scrollMotionState is what a scroll is doing on its own, which a rebuilt
// one carries on with.
type scrollMotionState struct {
	over       float64 // how far the content is pulled past an end: offset minus where it shows
	overMotion ggui.Motion
	relaxing   bool // over is springing back
	lastInput  time.Time
	from       float64 // the offset when the gesture began
	dir        float64 // the sign of the last movement
	settle     bool    // waiting for input to pause
	lifted     bool    // the fingers lifted, or their momentum stopped: settle now
	released   bool    // they lifted past an end; the momentum after is let go
	anim       ggui.Motion
	animating  bool
	steppedAt  time.Time // the frame anim last moved the window in
	stepping   bool      // anim is a wheel's notch easing; see wheelStep
}

func (p *scrollPhysics) adopt(q *scrollPhysics) { p.scrollMotionState = q.scrollMotionState }

// input notes a scroll gesture: what settling waits on and where it began.
// It stops a programmatic scroll and a spring-back under way.
func (p *scrollPhysics) input(s *ScrollWidget) {
	now := ggui.FrameTime()
	if p.lastInput.IsZero() || now.Sub(p.lastInput) >= scrollSettleDelay {
		p.from = s.position()
	}
	p.lastInput, p.lifted, p.released = now, false, false
	p.animating, p.stepping, p.relaxing = false, false, false
	p.settle = p.mode != OverscrollClamp || p.snap > 0 || p.paging
	if p.settle {
		ggui.WakeAt(now.Add(scrollSettleDelay))
	}
}

// step advances a programmatic scroll; Layout calls it before laying out
// the child for the new window. It moves the window once a frame: a frame
// lays the scroll out again until nothing changes, and content laid out
// at the new window, rows measured as they come into it, can move where
// the layout then holds the window, which another step to where the
// animation is would undo, writing a bound offset on every pass and never
// letting the frame settle.
func (p *scrollPhysics) step(s *ScrollWidget) {
	now := ggui.FrameTime()
	if !p.animating || now.Equal(p.steppedAt) {
		return
	}
	p.steppedAt = now
	v := p.anim.Value(now)
	if v == p.anim.Target() {
		p.animating = false
	}
	s.scrollTo(v)
}

// paint keeps a programmatic scroll and a spring-back moving, and settles
// a scroll once its input pauses.
func (p *scrollPhysics) paint(s *ScrollWidget) {
	now := ggui.FrameTime()
	p.applied, p.painted = nil, true // Adopt, if any, has run
	if p.animating {
		s.contain(true, s.relayout) // the next frame lays out the child for the next step
	}
	if p.relaxing {
		p.over = p.overMotion.Value(now)
		p.relaxing = p.over != 0
	}
	if !p.settle || s.dragging || p.animating && p.stepping {
		return
	}
	if wait := p.lastInput.Add(scrollSettleDelay); now.Before(wait) && !p.lifted {
		ggui.WakeAt(wait)
		return
	}
	p.settle, p.lifted = false, false
	if p.over != 0 {
		p.overMotion = ggui.Motion{}
		p.overMotion.MoveTo(p.over, now, 0)
		p.overMotion.MoveTo(0, now, s.barEnv.Motion(scrollSpringBack))
		p.over = p.overMotion.Value(now)
		p.relaxing = p.over != 0
	}
	if t, ok := s.snapTarget(); ok && t != s.position() {
		s.animateTo(t, true)
	}
}
