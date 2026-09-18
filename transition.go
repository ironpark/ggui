package ggui

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
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

	buf *ebiten.Image // offscreen, for fade and scale
}

// transitionStart is the start time, retained on the Canvas.
type transitionStart struct{ at time.Time }

// Transition wraps child in an enter animation: a fade over 200ms until
// Fade, Slide or Scale say otherwise.
func Transition(child Widget) *TransitionWidget {
	return &TransitionWidget{child: child, duration: 200 * time.Millisecond, ease: EaseOut, id: autoID()}
}

// Fade animates opacity from transparent.
func (t *TransitionWidget) Fade() *TransitionWidget { t.fade = true; return t }

// Slide animates position from dx, dy away from the child's place.
func (t *TransitionWidget) Slide(dx, dy float64) *TransitionWidget { t.dx, t.dy = dx, dy; return t }

// Scale animates size from the given factor around the child's center.
func (t *TransitionWidget) Scale(from float64) *TransitionWidget { t.from = from; return t }

// Duration sets how long the animation takes.
func (t *TransitionWidget) Duration(d time.Duration) *TransitionWidget { t.duration = d; return t }

// Easing sets the curve; see EaseLinear, EaseIn, EaseOut, EaseInOut.
func (t *TransitionWidget) Easing(e Easing) *TransitionWidget { t.ease = e; return t }

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

// Layout implements Widget.
func (t *TransitionWidget) Layout(c Constraints, env Env) Size {
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
		now := Now()
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
	if !(fade || scale) || dst == nil || dst.Image == nil {
		target.Paint(t.child, at)
		return
	}
	// Fade and scale paint the child offscreen and composite it: hit
	// regions still register where the child painted itself.
	b := dst.root().Image.Bounds()
	if t.buf == nil || t.buf.Bounds().Size() != b.Size() {
		t.buf = ebiten.NewImage(b.Dx(), b.Dy())
	}
	t.buf.Clear()
	off := &Canvas{parent: target, scale: dst.scale, inert: target.inert, Image: t.buf}
	off.Paint(t.child, at)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	if scale {
		s := t.from + (1-t.from)*e
		cx, cy := dst.px(at.Origin.X+at.Size.W/2), dst.px(at.Origin.Y+at.Size.H/2)
		op.GeoM.Translate(-cx, -cy)
		op.GeoM.Scale(s, s)
		op.GeoM.Translate(cx, cy)
	}
	if fade {
		op.ColorScale.ScaleAlpha(float32(e))
	}
	dst.Image.DrawImage(t.buf, op)
}

var transitionSlot = NewSlot[transitionStart]("transition start")

// Presence keeps child on screen while it animates out. While show is
// true the child is laid out and painted as usual, playing its enter
// animation when it first appears; when show turns false the child stays,
// inert to input, and runs the same animation backwards before it is
// removed. child is a Transition, or is wrapped in a fading one.
//
//	ggui.Presence(open, ggui.Transition(panel).Slide(0, -8).Fade())
func Presence(show Reader[bool], child Widget) Widget {
	t, ok := child.(*TransitionWidget)
	if !ok {
		t = Transition(child).Fade()
	}
	return Component(func() Builder {
		var initial float64
		Untrack(func() {
			if show.Get() {
				initial = 1
			}
		})
		p := Tween(initial, t.duration).Easing(t.ease)
		// Read the duration at the toggle rather than at setup, so a
		// Duration set from Layout — a theme token, say — is not frozen to
		// whatever it was on the first frame. Tweened.Set reads it here too,
		// so nothing changes under a run already in flight.
		Watch(show, func(v bool) { p.Duration(t.duration).Set(pick(v, 1.0, 0.0)) })
		return func() Widget {
			shown, v := show.Get(), p.Get()
			if !shown && v == 0 {
				return Box()
			}
			t.drive(v, !shown)
			return t
		}
	})
}
