package ggui

import (
	"slices"
	"time"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
)

// SwitcherWidget shows the subtree built for the current key and animates
// between subtrees when the key changes: the new one plays its enter
// Transition while the old one plays the same Transition backwards on top
// of nothing, inert to input, and is disposed when it is gone. Build one
// with Switcher.
type SwitcherWidget[K comparable] struct {
	comp  *ComponentWidget
	key   Readable[K]
	build func(K) Widget

	fade     bool
	dx, dy   float64
	from     float64 // starting scale, 0 for none
	duration time.Duration
	ease     Easing

	props   property.Owner // changed while moving, to lay out again
	entries []*switchEntry // oldest first; the last is the current key's
	sizes   []Size
	reduced bool
}

// switchEntry is one subtree the switcher holds: the current key's, or one
// on its way out.
type switchEntry struct {
	t       *TransitionWidget
	dispose Cleanup
	since   time.Time     // when it started its current move
	from    float64       // its progress then
	d       time.Duration // how long a full move takes, as of its switch
	leaving bool
}

// progress returns how far in e is at now, from zero to one.
func (e *switchEntry) progress(now time.Time) float64 {
	d := e.d
	if d <= 0 {
		return pick(e.leaving, 0.0, 1.0)
	}
	moved := float64(now.Sub(e.since)) / float64(d)
	if e.leaving {
		return clamp(e.from-moved, 0, 1)
	}
	return clamp(e.from+moved, 0, 1)
}

// Switcher builds a subtree for key's value, as Key does, and cross-fades
// to a new one when the value changes: the old subtree stays, inert, until
// it has faded out. The first subtree appears without animating. Like Key,
// build is untracked and owns what it creates; each subtree is disposed
// once it has left. Its options may change after mount; they apply from
// the next switch.
//
//	ggui.Switcher(tab, func(t Tab) ggui.Widget { return page(t) }).Slide(0, 8)
func Switcher[K comparable](key Readable[K], build func(K) Widget) *SwitcherWidget[K] {
	s := &SwitcherWidget[K]{key: key, build: build, duration: 200 * time.Millisecond, ease: EaseOut}
	s.comp = Component(func() Widget {
		owner := reactive.CurrentOwner()
		OnCleanup(func() {
			for _, e := range s.entries {
				e.dispose()
			}
			s.entries = nil
		})
		observeKey(key, func(v K, first bool) {
			reactive.Untrack(func() struct{} { s.change(v, owner, first); return struct{}{} })
		})
		return &switcherBody[K]{s}
	})
	return s
}

// Fade fades the subtrees in and out; it is the default until Slide or
// Scale is set, and combines with them.
func (s *SwitcherWidget[K]) Fade() *SwitcherWidget[K] { s.fade = true; return s }

// Slide moves an entering subtree in from dx, dy away, and a leaving one out
// the same way.
func (s *SwitcherWidget[K]) Slide(dx, dy float64) *SwitcherWidget[K] {
	s.dx, s.dy = dx, dy
	return s
}

// Scale grows an entering subtree from the given factor, and shrinks a
// leaving one to it.
func (s *SwitcherWidget[K]) Scale(from float64) *SwitcherWidget[K] {
	s.from = from
	return s
}

// Duration sets how long a switch takes, 200ms by default.
func (s *SwitcherWidget[K]) Duration(d time.Duration) *SwitcherWidget[K] {
	s.duration = d
	return s
}

// Easing sets the curve, EaseOut by default.
func (s *SwitcherWidget[K]) Easing(e Easing) *SwitcherWidget[K] {
	s.ease = e
	return s
}

// change makes v's subtree the current one and sends the rest away.
func (s *SwitcherWidget[K]) change(v K, owner *reactive.Computation, first bool) {
	now := FrameTime()
	for _, e := range s.entries {
		if !e.leaving {
			e.from, e.since, e.leaving = e.progress(now), now, true
		}
	}
	e := &switchEntry{since: now, from: pick(first, 1.0, 0.0), d: s.duration}
	child, dispose := mountKeyed(owner, func() Widget { return s.build(v) })
	e.dispose = dispose
	if child == nil {
		child = Box()
	}
	t := Transition(child).Duration(s.duration).Easing(s.ease)
	if s.fade {
		t.Fade()
	}
	if s.dx != 0 || s.dy != 0 {
		t.Slide(s.dx, s.dy)
	}
	if s.from != 0 {
		t.Scale(s.from)
	}
	e.t = t
	s.entries = append(s.entries, e)
	s.comp.cw.invalidate()
}

// switcherBody lays out and paints the switcher's subtrees inside its
// component.
type switcherBody[K comparable] struct{ s *SwitcherWidget[K] }

// Layout implements Widget: as large as the largest subtree, the leaving
// ones included, all anchored at the top-left corner as in a Stack.
func (b *switcherBody[K]) Layout(c Constraints, env Env) Size {
	s := b.s
	defer s.props.Layout().EndLayout()
	s.reduced = env.ReducedMotion()
	now := FrameTime()
	s.entries = slices.DeleteFunc(s.entries, func(e *switchEntry) bool {
		if e.leaving && (s.reduced || e.progress(now) <= 0) {
			e.dispose()
			return true
		}
		return false
	})
	s.sizes = s.sizes[:0]
	var total Size
	for _, e := range s.entries {
		e.t.drive(pick(s.reduced, 1.0, e.progress(now)), e.leaving)
		size := e.t.Layout(c.Loosen(), env)
		s.sizes = append(s.sizes, size)
		total.W, total.H = max(total.W, size.W), max(total.H, size.H)
	}
	return c.Constrain(total)
}

// Paint implements Widget. While a subtree is moving it asks for another
// frame and another layout, in which the finished ones are dropped.
func (b *switcherBody[K]) Paint(dst *Canvas, r Rect) {
	s := b.s
	moving := false
	for i, e := range s.entries {
		moving = moving || e.leaving || e.t.progress < 1
		dst.Paint(e.t, Rct(r.Origin, s.sizes[i]))
	}
	if moving && !s.reduced {
		// Paint runs as layout does, where a change schedules nothing: ask
		// for the next layout outright.
		activeWorld().frame.animate()
		s.props.Changed()
		reactive.RequestLayout()
	}
}

// Baseline implements Baseliner: the current subtree's.
func (b *switcherBody[K]) Baseline() (float64, bool) {
	if n := len(b.s.entries); n > 0 {
		return baselineOf(b.s.entries[n-1].t)
	}
	return 0, false
}

// Layout implements Widget.
func (s *SwitcherWidget[K]) Layout(c Constraints, env Env) Size { return s.comp.Layout(c, env) }

// Paint implements Widget.
func (s *SwitcherWidget[K]) Paint(dst *Canvas, r Rect) { s.comp.Paint(dst, r) }

// Baseline implements Baseliner: the current subtree's.
func (s *SwitcherWidget[K]) Baseline() (float64, bool) { return s.comp.Baseline() }
