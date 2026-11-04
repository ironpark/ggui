package ggui

import (
	"slices"
	"time"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
)

// TransitionWidget animates its child in when it first appears: it fades,
// slides or scales from a starting state to its place over Duration. Build
// one with Transition, and use Presence to animate a child out as well.
//
// Whether the child is new is judged against the previous frame by the
// identity a keyed component gives the transition, else by its Rect, so a
// Builder that rebuilds every frame does not restart the animation; a
// widget that appears at a new place plays it.
type TransitionWidget struct {
	props    property.Owner
	child    Widget
	fade     bool
	dx, dy   float64
	from     float64 // starting scale, 0 for none
	duration time.Duration
	ease     Easing
	id       any // from the keyed component it was constructed in

	// Presence drives progress from outside.
	driven   bool
	progress float64
	leaving  bool
	reduced  bool // the Env asked for no animation
}

// transitionStart is the start time, retained on the Canvas.
type transitionStart struct{ at time.Time }

// Transition wraps child in an enter animation: a fade over 200ms until
// Fade, Slide or Scale say otherwise.
func Transition(child Widget) *TransitionWidget {
	return &TransitionWidget{child: child, duration: 200 * time.Millisecond, ease: EaseOut, id: reactive.AutoID()}
}

// PopIn is the entrance every floating panel shares: a fade with a slight
// scale up, driven through Progress by a Motion. The easing is linear
// because Motion.Value already eases; a curve here would apply twice.
func PopIn(child Widget) *TransitionWidget {
	return Transition(child).Fade().Scale(.95).Easing(EaseLinear)
}

// Fade animates opacity from transparent.
func (t *TransitionWidget) Fade() *TransitionWidget {
	defer property.Watch(&t.props, &t.fade)()
	t.fade = true
	return t
}

// Slide animates position from dx, dy away from the child's place.
func (t *TransitionWidget) Slide(dx, dy float64) *TransitionWidget {
	defer property.Watch(&t.props, &t.dx)()
	defer property.Watch(&t.props, &t.dy)()
	t.dx, t.dy = dx, dy
	return t
}

// Scale animates size from the given factor around the child's center.
func (t *TransitionWidget) Scale(from float64) *TransitionWidget {
	defer property.Watch(&t.props, &t.from)()
	t.from = from
	return t
}

// Duration sets how long the animation takes.
func (t *TransitionWidget) Duration(d time.Duration) *TransitionWidget {
	defer property.Watch(&t.props, &t.duration)()
	t.duration = d
	return t
}

// Easing sets the curve; see EaseLinear, EaseIn, EaseOut, EaseInOut.
func (t *TransitionWidget) Easing(e Easing) *TransitionWidget {
	defer property.Watch(&t.props, &t.ease)()
	t.ease = e
	return t
}

// effects reports whether any visual is set; a bare Transition fades.
func (t *TransitionWidget) effects() (fade bool, slide bool, scale bool) {
	fade = t.fade || (!t.fade && t.dx == 0 && t.dy == 0 && t.from == 0)
	return fade, t.dx != 0 || t.dy != 0, t.from != 0 && t.from != 1
}

// drive sets progress from outside: 0 is fully out, 1 in place; leaving
// makes the child inert.
func (t *TransitionWidget) drive(progress float64, leaving bool) {
	t.driven, t.progress, t.leaving = true, progress, leaving
}

// Progress drives the effect explicitly: zero is hidden, one is fully visible.
// A leaving effect paints without accepting input. Call on each frame when
// using a Motion to coordinate a panel and its backdrop.
func (t *TransitionWidget) Progress(value float64, leaving bool) *TransitionWidget {
	t.drive(clamp(value, 0, 1), leaving)
	return t
}

// Baseline implements Baseliner: the child's, where it lands.
func (t *TransitionWidget) Baseline() (float64, bool) { return baselineOf(t.child) }

// Layout implements Widget.
func (t *TransitionWidget) Layout(c Constraints, env Env) Size {
	defer t.props.Layout().EndLayout()
	t.reduced = env.ReducedMotion()
	return t.child.Layout(c, env)
}

// Paint implements Widget.
func (t *TransitionWidget) Paint(dst *Canvas, r Rect) {
	p := t.progress
	if t.reduced {
		// Reduced motion: in place at once; a leaving child is still inert
		// until Presence removes it.
		p = 1
	} else if !t.driven {
		now := FrameTime()
		start := now
		at := Anchor{Rect: r, ID: t.id}
		if s, ok := dst.Retained(at, transitionSlot); ok {
			start = s.at
		}
		dst.Retain(at, transitionSlot, transitionStart{start})
		p = 1
		if t.duration > 0 {
			p = clamp(float64(now.Sub(start))/float64(t.duration), 0, 1)
		}
		if p < 1 {
			activeWorld().frame.animate()
		}
	}
	if p >= 1 && !t.leaving {
		dst.Paint(t.child, r)
		return
	}
	target := dst
	if t.leaving {
		target = dst.Inert()
	}
	fade, slide, scale := t.effects()
	e := t.ease(p)
	at := r
	if slide {
		at.Origin = at.Origin.Add(Pt((1-e)*t.dx, (1-e)*t.dy))
	}
	if !(fade || scale) {
		target.Paint(t.child, at)
		return
	}
	// Fade and scale apply to the child as a whole, so it paints into a
	// layer that is composited: hit regions still register where the child
	// painted itself.
	var o LayerOptions
	if scale {
		o.Scale = t.from + (1-t.from)*e
		o.About = at.Center()
	}
	if fade {
		o.Fade = 1 - e
	}
	target.Layer(o, func(layer *Canvas) { layer.Paint(t.child, at) })
}

var transitionSlot = NewSlot[transitionStart]("transition start")

// Presence keeps child on screen while it animates out. While show is
// true the child is laid out and painted as usual, playing its enter
// animation when it first appears; when show turns false the child stays,
// inert to input, and runs the same animation backwards before it is
// removed. child is a Transition, or is wrapped in a fading one.
//
//	ggui.Presence(open, ggui.Transition(panel).Slide(0, -8).Fade())
func Presence(show Readable[bool], child Widget) Widget {
	t, ok := child.(*TransitionWidget)
	if !ok {
		t = Transition(child).Fade()
	}
	return Component(func() Widget {
		var initial float64
		Untrack(func() struct {
		} {
			if show.Get() {
				initial = 1
			}
			return struct {
			}{}
		})
		// The tween is linear: the Transition applies its easing to the
		// progress it is driven with, and a curve here would apply twice.
		p := Tween(initial, t.duration).Easing(EaseLinear)
		// Read the duration at the toggle rather than at setup, so a
		// Duration set from Layout — a theme token, say — is not frozen to
		// whatever it was on the first frame. Tweened.Set reads it here too,
		// so nothing changes under a run already in flight.
		Watch(show, func(v bool) { p.Duration(t.duration).Set(pick(v, 1.0, 0.0)) })
		return Reactive(func() Widget {
			shown, v := show.Get(), p.Get()
			if !shown && v == 0 {
				return Box()
			}
			t.drive(v, !shown)
			return t
		})
	})
}

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
