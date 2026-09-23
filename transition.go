package ggui

import (
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
	defer t.props.Layout()()
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
			frame.animate()
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
		o.About = Pt(at.Origin.X+at.Size.W/2, at.Origin.Y+at.Size.H/2)
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
		p := Tween(initial, t.duration).Easing(t.ease)
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
