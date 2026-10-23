package ggui

import (
	"image"
	"math"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
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

const (
	// scrollSettleDelay is how long input must pause before an overscroll
	// springs back and a snapping scroll settles. ggfx reports neither a
	// lifted finger on a trackpad nor the end of a wheel gesture, so a
	// pause is the signal.
	scrollSettleDelay = 120 * time.Millisecond
	scrollSpringBack  = 350 * time.Millisecond
	scrollAnimation   = 300 * time.Millisecond
)

// scrollPhysics is the part of a ScrollWidget that moves on its own:
// overscroll, snapping and the animated scrolls a ScrollController asks for.
type scrollPhysics struct {
	mode   Overscroll
	snap   float64 // snap interval; 0 snaps nowhere
	paging bool    // snap to the viewport's extent
	ctrl   *ScrollController

	// State, carried across a rebuild by adopt.
	over       float64 // how far the content is pulled past an end: offset minus where it shows
	overMotion Motion
	relaxing   bool // over is springing back
	lastInput  time.Time
	from       float64 // the offset when the gesture began
	dir        float64 // the sign of the last movement
	settle     bool    // waiting for input to pause
	anim       Motion
	animTo     float64
	animating  bool

	applied *scrollRequest // what attach applied in this layout, until paint
	painted bool           // painted, so adopted what it replaces: requests apply at once
}

func (p *scrollPhysics) adopt(q *scrollPhysics) {
	p.over, p.overMotion, p.relaxing = q.over, q.overMotion, q.relaxing
	p.lastInput, p.from, p.dir, p.settle = q.lastInput, q.from, q.dir, q.settle
	p.anim, p.animTo, p.animating = q.anim, q.animTo, q.animating
}

// Overscroll sets what happens at either end: OverscrollClamp, the
// default, stops there; OverscrollBounce and OverscrollStretch follow the
// input past it and spring back when it stops.
func (s *ScrollWidget) Overscroll(o Overscroll) *ScrollWidget {
	defer property.Watch(&s.props, &s.phys.mode)()
	s.phys.mode = o
	return s
}

// Snap settles the offset on a multiple of v, or on the end, once scrolling
// stops: rows of height v always line up with the top of the window. Zero
// turns snapping off.
func (s *ScrollWidget) Snap(v float64) *ScrollWidget {
	defer property.Watch(&s.props, &s.phys.snap)()
	s.phys.snap = max(v, 0)
	return s
}

// Paging snaps to whole viewports, as a pager does: a gesture that moves
// more than a sixth of a page settles on the next page in its direction,
// a shorter one returns to the page it started from.
func (s *ScrollWidget) Paging() *ScrollWidget {
	defer property.Watch(&s.props, &s.phys.paging)()
	s.phys.paging = true
	return s
}

// input notes a scroll gesture: what settling waits on and where it began.
// It stops a programmatic scroll and a spring-back under way.
func (p *scrollPhysics) input(s *ScrollWidget) {
	now := FrameTime()
	if p.lastInput.IsZero() || now.Sub(p.lastInput) >= scrollSettleDelay {
		p.from = s.position()
	}
	p.lastInput = now
	p.animating, p.relaxing = false, false
	p.settle = p.mode != OverscrollClamp || p.snap > 0 || p.paging
	if p.settle {
		WakeAt(now.Add(scrollSettleDelay))
	}
}

// wheel moves the content by d along the axis, as the wheel, a trackpad or
// a touch pan asks, and reports whether the event was used.
func (s *ScrollWidget) wheel(d float64, momentum bool) bool {
	p := &s.phys
	p.input(s)
	before, over := s.position(), p.over
	if p.mode == OverscrollClamp {
		s.scrollTo(before - d)
	} else {
		raw := s.rubber(before-p.over, -d, momentum)
		s.scrollTo(raw)
		p.over = s.position() - raw
	}
	if moved := s.position() - before; moved != 0 {
		p.dir = math.Copysign(1, moved)
	} else if d != 0 {
		p.dir = math.Copysign(1, -d)
	}
	if p.over != over {
		activeWorld().frame.animate()
	}
	if momentum && math.Abs(p.over) >= s.extent(s.viewport)/8 {
		return false // a fling that reached the end stops there and bounces back
	}
	return !momentum || s.position() != before || p.over != over
}

// rubber moves raw, the offset the content shows at, by d, with movement
// past either end resisted the further it goes.
func (s *ScrollWidget) rubber(raw, d float64, momentum bool) float64 {
	lo, hi := 0.0, s.maxOffset()
	next := raw + d
	if next >= lo && next <= hi || raw < lo && d > 0 || raw > hi && d < 0 {
		return next
	}
	edge := pick(next < lo, lo, hi)
	from := raw
	if raw >= lo && raw <= hi {
		from = edge
	}
	extent := max(s.extent(s.viewport), 1)
	k := 0.5 * (1 - min(math.Abs(from-edge)/extent, 1))
	if momentum {
		k *= 0.4
	}
	return from + (next-from)*k
}

// step advances a programmatic scroll; Layout calls it before laying out
// the child for the new window.
func (p *scrollPhysics) step(s *ScrollWidget) {
	if !p.animating {
		return
	}
	v := p.anim.Value(FrameTime())
	if v == p.animTo {
		p.animating = false
	}
	s.scrollTo(v)
}

// attach applies what the controller asked for before the first layout.
func (p *scrollPhysics) attach(s *ScrollWidget) {
	c := p.ctrl
	if c == nil {
		return
	}
	c.scroll = s
	if r := c.pending; r != nil {
		c.pending = nil
		s.animateTo(r.offset, r.animate)
		// A rebuilt scroll adopts the one it replaces only at paint, which
		// would overwrite what this did; Adopt applies it again.
		p.applied = r
	}
}

// reapply redoes, after Adopt, a request attach applied before it.
func (p *scrollPhysics) reapply(s *ScrollWidget) {
	if r := p.applied; r != nil {
		p.applied = nil
		s.animateTo(r.offset, r.animate)
	}
}

// paint keeps a programmatic scroll and a spring-back moving, and settles
// a scroll once its input pauses.
func (p *scrollPhysics) paint(s *ScrollWidget) {
	now := FrameTime()
	p.applied, p.painted = nil, true // Adopt, if any, has run
	if p.animating {
		s.cache.invalidate() // the next frame lays out the child for the next step
	}
	if p.relaxing {
		p.over = p.overMotion.Value(now)
		p.relaxing = p.over != 0
	}
	if !p.settle || s.dragging {
		return
	}
	if wait := p.lastInput.Add(scrollSettleDelay); now.Before(wait) {
		WakeAt(wait)
		return
	}
	p.settle = false
	if p.over != 0 {
		p.overMotion = Motion{}
		p.overMotion.MoveTo(p.over, now, 0)
		p.overMotion.MoveTo(0, now, s.barEnv.Motion(scrollSpringBack))
		p.over = p.overMotion.Value(now)
		p.relaxing = p.over != 0
	}
	if t, ok := s.snapTarget(); ok && t != s.position() {
		s.animateTo(t, true)
	}
}

// snapTarget returns where a snapping scroll settles from where it is.
func (s *ScrollWidget) snapTarget() (float64, bool) {
	p := &s.phys
	step := p.snap
	if p.paging {
		step = s.extent(s.viewport)
	}
	if step <= 0 || !s.laidOut {
		return 0, false
	}
	pos, end := s.position(), s.maxOffset()
	n := pos / step
	t := math.Round(n)
	if p.paging {
		t = math.Round(p.from / step)
		if math.Abs(pos-p.from) > step/6 {
			t = pick(p.dir > 0, math.Ceil(n), math.Floor(n))
		}
	}
	v := clamp(t*step, 0, end)
	if !p.paging && math.Abs(end-pos) < math.Abs(v-pos) {
		v = end // the end is a stop too, so the last rows can be reached
	}
	return v, true
}

// animateTo moves the window to v, eased over the scroll animation unless
// animate is false or motion is reduced.
func (s *ScrollWidget) animateTo(v float64, animate bool) {
	p := &s.phys
	if s.laidOut {
		v = clamp(v, 0, s.maxOffset())
	}
	d := s.barEnv.Motion(scrollAnimation)
	if !animate || d <= 0 || !s.laidOut || v == s.position() {
		p.animating = false
		s.jumpTo(v)
		return
	}
	now := FrameTime()
	p.anim = Motion{}
	p.anim.MoveTo(s.position(), now, 0)
	p.anim.MoveTo(v, now, d)
	p.animTo, p.animating = v, true
	s.cache.invalidate()
}

// jumpTo moves the window to v at once. Before the first layout the extent
// is unknown, so v is kept for Layout to clamp.
func (s *ScrollWidget) jumpTo(v float64) {
	if s.laidOut {
		s.scrollTo(v)
		return
	}
	if s.bound != nil {
		s.bound.Set(v)
	} else {
		s.offset = v
	}
	reactive.RequestLayout()
}

// paintContent paints the child at the window's offset, moved or stretched
// by any overscroll.
func (s *ScrollWidget) paintContent(dst *Canvas, r Rect) {
	p := &s.phys
	off := s.position()
	if p.mode != OverscrollStretch {
		off -= p.over
	}
	origin := r.Origin
	if s.horizontal {
		origin.X -= off
	} else {
		origin.Y -= off
	}
	content := Rct(origin, s.childSize)
	extent := s.extent(r.Size)
	if p.mode != OverscrollStretch || p.over == 0 || extent <= 0 || dst == nil || dst.Image == nil {
		dst.Paint(s.child, content)
		return
	}
	// Stretched from the end being pulled: the start when the content is
	// pulled down (or right), the far end when it is pulled up.
	k := 1 + 0.5*min(math.Abs(p.over)/extent, 1)
	about := r.Origin
	if p.over < 0 {
		about = about.Add(pick(s.horizontal, Pt(r.Size.W, 0), Pt(0, r.Size.H)))
	}
	sx, sy := pick(s.horizontal, k, 1.0), pick(s.horizontal, 1.0, k)
	dst.layer(image.Rectangle{}, func(layer *Canvas) { layer.Paint(s.child, content) }, func(img *ggfx.Image) {
		op := &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear}
		op.GeoM.Translate(float64(img.Bounds().Min.X), float64(img.Bounds().Min.Y))
		ax, ay := dst.px(about.X), dst.px(about.Y)
		op.GeoM.Translate(-ax, -ay)
		op.GeoM.Scale(sx, sy)
		op.GeoM.Translate(ax, ay)
		dst.Image.DrawImage(img, op)
	})
}

// visualOffset is the offset the content is drawn at, past either end
// while it is pulled there.
func (s *ScrollWidget) visualOffset() float64 {
	if s.phys.mode == OverscrollStretch {
		return s.position()
	}
	return s.position() - s.phys.over
}

// scrollPortKey carries the nearest Scroll to its subtree, so a sticky
// header can find the window's edge and a list can answer ScrollToKey.
var scrollPortKey = NewEnvKey[*ScrollWidget]("scroll port")

// scrollPortOf returns the nearest enclosing Scroll in env, if any.
func scrollPortOf(env Env) *ScrollWidget {
	s, _ := env.Get(scrollPortKey)
	return s
}
