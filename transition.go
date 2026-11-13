package ggui

import (
	"image/color"
	"math"
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
	origin   Point   // where the scale is anchored, as fractions of the child
	travel   Point   // how far a scaled entrance also comes from, in pixels
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
	return &TransitionWidget{child: child, origin: Pt(.5, .5), duration: 200 * time.Millisecond, ease: EaseOut, id: reactive.AutoID()}
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

// Scale animates size from the given factor around the child's center, or
// around the point ScaleOrigin names.
func (t *TransitionWidget) Scale(from float64) *TransitionWidget {
	defer property.Watch(&t.props, &t.from)()
	t.from = from
	return t
}

// ScaleOrigin sets the point a Scale grows from, as fractions of the child's
// width and height: 0, 0 is its top-left corner, 1, 1 its bottom-right, and
// .5, .5, the default, its center. It is CSS's transform-origin: a panel
// that opens below a button scales from its top edge with ScaleOrigin(.5, 0).
func (t *TransitionWidget) ScaleOrigin(x, y float64) *TransitionWidget {
	defer property.Watch(&t.props, &t.origin)()
	t.origin = Pt(x, y)
	return t
}

// popTravel is how far a floating panel travels from its anchor as it
// enters.
const popTravel = 8

// PopFrom makes a scaled entrance, PopIn's, come from the anchor of a
// floating panel that sits on side of it: the scale is anchored on the
// panel's edge facing the anchor, along of the way down or across that edge
// (0 its start, .5 its middle, 1 its end), and the panel travels eight
// pixels away from the anchor as it grows. side is one of PopupBottom,
// PopupTop, PopupLeft and PopupRight.
//
// The travel is drawn, not laid out: the panel's hit regions and its Rect
// are where it lands from the first frame, so a press during the entrance
// goes where it will be. Nothing here needs another layout either, so a
// panel that learns its side while it paints may call this then.
func (t *TransitionWidget) PopFrom(side PopupSide, along float64) *TransitionWidget {
	switch side {
	case PopupTop:
		t.origin, t.travel = Pt(along, 1), Pt(0, popTravel)
	case PopupLeft:
		t.origin, t.travel = Pt(1, along), Pt(popTravel, 0)
	case PopupRight:
		t.origin, t.travel = Pt(0, along), Pt(-popTravel, 0)
	default:
		t.origin, t.travel = Pt(along, 0), Pt(0, -popTravel)
	}
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
		// Scaling about a point moved travel/(1-from) off the origin moves
		// the child by travel at the start and by nothing at the end, which
		// is the slide, without moving what the child registers.
		k := 1 / (1 - t.from)
		o.About = at.Origin.Add(Pt(at.Size.W*t.origin.X+t.travel.X*k, at.Size.H*t.origin.Y+t.travel.Y*k))
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

	props   property.Owner
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
	moving := false
	for _, e := range s.entries {
		e.t.drive(pick(s.reduced, 1.0, e.progress(now)), e.leaving)
		moving = moving || e.leaving || e.t.progress < 1
		size := e.t.Layout(c.Loosen(), env)
		s.sizes = append(s.sizes, size)
		total.W, total.H = max(total.W, size.W), max(total.H, size.H)
	}
	if moving && !s.reduced {
		// The next step is driven, and a finished subtree dropped, by the
		// next layout, which the switcher asks for as a size on its way
		// does: held within the Scroll around it, if one holds it.
		Invalidate(env)
	}
	return c.Constrain(total)
}

// Paint implements Widget.
func (b *switcherBody[K]) Paint(dst *Canvas, r Rect) {
	s := b.s
	for i, e := range s.entries {
		dst.Paint(e.t, Rct(r.Origin, s.sizes[i]))
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

// AnimatedBoxWidget is a Box whose fill, border, corner radius, size and
// padding ease to each new value instead of jumping, whether the value is
// set again or comes from a bound reader. Build one with AnimatedBox.
//
// Theme colors are resolved before they blend, so a theme switch fades the
// box too. Under reduced motion every change lands at once. The motion
// lives in the widget: keep one box and change its values, rather than
// building a new box with the new values, which starts where it is told.
type AnimatedBoxWidget struct {
	props       property.Owner
	box         *BoxWidget // painted with the values of the moment
	duration    time.Duration
	ease        Easing
	collapsible bool // a size of zero is zero, eased like any other

	fill, border            property.Value[color.Color]
	width, height           property.Value[float64]
	radius, borderWidth     property.Value[float64]
	padding                 property.Value[EdgeInsets]
	fillTo, borderTo        follower[color.Color]
	widthTo, heightTo       follower[float64]
	radiusTo, borderWidthTo follower[float64]
	paddingTo               follower[EdgeInsets]
	size                    Size // as the last layout left it
}

// AnimatedBox wraps at most one child, as Box does, and animates its looks
// over 200ms with EaseOut until Duration and Easing say otherwise.
//
//	card := ggui.AnimatedBox(content).BindFill(tint).Radius(8).Pad(12)
func AnimatedBox(child ...Widget) *AnimatedBoxWidget {
	return &AnimatedBoxWidget{box: Box(child...), duration: 200 * time.Millisecond, ease: EaseOut}
}

// animSet assigns a literal and lays out again if it changed.
func animSet[T any](a *AnimatedBoxWidget, p *property.Value[T], v T) *AnimatedBoxWidget {
	property.Set(&a.props, p, v)
	return a
}

// animBind follows a reader and lays out again if it is a new one.
func animBind[T any](a *AnimatedBoxWidget, p *property.Value[T], r Readable[T], name string) *AnimatedBoxWidget {
	property.Bind(&a.props, p, r, name)
	return a
}

// Duration sets how long each change takes.
func (a *AnimatedBoxWidget) Duration(d time.Duration) *AnimatedBoxWidget {
	defer property.Watch(&a.props, &a.duration)()
	a.duration = d
	return a
}

// Easing sets the curve; see EaseLinear, EaseIn, EaseOut, EaseInOut.
func (a *AnimatedBoxWidget) Easing(e Easing) *AnimatedBoxWidget {
	a.ease = e
	return a
}

// Fill sets the background color. Nil paints nothing, and fades from or to
// transparent.
func (a *AnimatedBoxWidget) Fill(c color.Color) *AnimatedBoxWidget { return animSet(a, &a.fill, c) }

// BindFill follows a non-nil color reader, easing to each new color.
func (a *AnimatedBoxWidget) BindFill(r Readable[color.Color]) *AnimatedBoxWidget {
	return animBind(a, &a.fill, r, "BindFill")
}

// Border draws a line of width w in color c just inside the edge.
func (a *AnimatedBoxWidget) Border(w float64, c color.Color) *AnimatedBoxWidget {
	return animSet(animSet(a, &a.borderWidth, w), &a.border, c)
}

// BindBorderColor follows a non-nil reader for the border's color.
func (a *AnimatedBoxWidget) BindBorderColor(r Readable[color.Color]) *AnimatedBoxWidget {
	return animBind(a, &a.border, r, "BindBorderColor")
}

// Radius rounds the corners of the fill and border.
func (a *AnimatedBoxWidget) Radius(r float64) *AnimatedBoxWidget { return animSet(a, &a.radius, r) }

// BindRadius follows a non-nil corner radius reader.
func (a *AnimatedBoxWidget) BindRadius(r Readable[float64]) *AnimatedBoxWidget {
	return animBind(a, &a.radius, r, "BindRadius")
}

// Size fixes both dimensions. Zero leaves that dimension to the child.
func (a *AnimatedBoxWidget) Size(w, h float64) *AnimatedBoxWidget { return a.Width(w).Height(h) }

// Width fixes the width. Zero leaves it to the child: a change to zero
// lands at once, and one from zero eases from the size the child gave.
func (a *AnimatedBoxWidget) Width(w float64) *AnimatedBoxWidget { return animSet(a, &a.width, w) }

// BindWidth follows a non-nil width reader.
func (a *AnimatedBoxWidget) BindWidth(r Readable[float64]) *AnimatedBoxWidget {
	return animBind(a, &a.width, r, "BindWidth")
}

// Collapsible makes a width or height of zero mean zero, eased to and from
// like any other size, rather than leaving that dimension to the child: a
// gap that opens from nothing and closes back to it.
func (a *AnimatedBoxWidget) Collapsible() *AnimatedBoxWidget {
	defer property.Watch(&a.props, &a.collapsible)()
	a.collapsible = true
	return a
}

// Height fixes the height. Zero leaves it to the child, as in Width.
func (a *AnimatedBoxWidget) Height(h float64) *AnimatedBoxWidget { return animSet(a, &a.height, h) }

// BindHeight follows a non-nil height reader.
func (a *AnimatedBoxWidget) BindHeight(r Readable[float64]) *AnimatedBoxWidget {
	return animBind(a, &a.height, r, "BindHeight")
}

// Pad sets padding with the CSS shorthand Insets accepts.
func (a *AnimatedBoxWidget) Pad(sides ...float64) *AnimatedBoxWidget {
	return animSet(a, &a.padding, Insets(sides...))
}

// Padding sets per-side padding.
func (a *AnimatedBoxWidget) Padding(e EdgeInsets) *AnimatedBoxWidget {
	return animSet(a, &a.padding, e)
}

// BindPadding follows a non-nil padding reader.
func (a *AnimatedBoxWidget) BindPadding(r Readable[EdgeInsets]) *AnimatedBoxWidget {
	return animBind(a, &a.padding, r, "BindPadding")
}

// Layout implements Widget.
func (a *AnimatedBoxWidget) Layout(c Constraints, env Env) Size {
	defer a.props.Layout().EndLayout()
	d := env.Motion(a.duration)
	b := a.box
	b.Fill(a.fillTo.follow(env, ResolveColor(a.fill.Get(), env), d, a.ease, LerpColor))
	b.Border(a.borderWidthTo.follow(env, a.borderWidth.Get(), d, a.ease, LerpFloat),
		a.borderTo.follow(env, ResolveColor(a.border.Get(), env), d, a.ease, LerpColor))
	b.Radius(a.radiusTo.follow(env, a.radius.Get(), d, a.ease, LerpFloat))
	b.Width(a.boxSize(followSize(env, &a.widthTo, a.width.Get(), a.size.W, d, a.ease, a.collapsible)))
	b.Height(a.boxSize(followSize(env, &a.heightTo, a.height.Get(), a.size.H, d, a.ease, a.collapsible)))
	b.Padding(a.paddingTo.follow(env, a.padding.Get(), d, a.ease, LerpInsets))
	a.size = b.Layout(c, env)
	return a.size
}

// Paint implements Widget.
func (a *AnimatedBoxWidget) Paint(dst *Canvas, r Rect) { a.box.Paint(dst, r) }

// Baseline implements Baseliner: the child's, below the top padding.
func (a *AnimatedBoxWidget) Baseline() (float64, bool) { return a.box.Baseline() }

// follower eases one property of a widget toward the value its layout
// asks for, from where it was when the target changed. Its progress is a
// Motion moved in Layout, which lays the widget out again for each step
// while it moves, as a size eased there must be, whether the property is
// one or only paints: one Motion for both kinds keeps them in step.
type follower[T any] struct {
	from, to T
	p        Motion // from zero at from to one at to
	set      bool   // to is a target followed, not a zero value
}

// follow heads for target over d and returns the value of the moment.
func (f *follower[T]) follow(env Env, target T, d time.Duration, e Easing, lerp func(a, b T, t float64) T) T {
	if d <= 0 {
		f.jump(target)
		return target
	}
	if !f.set {
		f.jump(target)
		f.set = true
		return target
	}
	if !property.Equal(f.to, target) {
		f.start(f.at(FrameTime(), lerp), target, e)
	}
	f.p.LayoutTo(env, 1, d)
	return f.at(FrameTime(), lerp)
}

// jump lands on v at once, with nothing to follow from.
func (f *follower[T]) jump(v T) {
	f.from, f.to, f.set = v, v, false
	f.p = Motion{}
}

// start sets off from from toward to along e, from this frame.
func (f *follower[T]) start(from, to T, e Easing) {
	f.from, f.to = from, to
	f.p = Motion{}
	f.p.Easing(e).MoveTo(0, time.Time{}, 0)
}

// at is where the property is at now: to itself once there, so a target
// lands exactly, whatever lerp makes of one.
func (f *follower[T]) at(now time.Time, lerp func(a, b T, t float64) T) T {
	p, moving := f.p.at(now)
	if !moving {
		return f.to
	}
	return lerp(f.from, f.to, p)
}

// boxSize is the size the inner Box is given for v: where zero is zero,
// one too small to see, as a Box takes zero to mean its child's size.
func (a *AnimatedBoxWidget) boxSize(v float64) float64 {
	if a.collapsible && v <= 0 {
		return math.SmallestNonzeroFloat64
	}
	return v
}

// followSize eases a width or height, where zero is no number on the way
// but "as the child sizes it": a move to zero lands at once, and a move
// away from it starts at shown, the size the box last had. A collapsible
// box's zero is a number like any other.
func followSize(env Env, f *follower[float64], target, shown float64, d time.Duration, e Easing, collapsible bool) float64 {
	if collapsible {
		return f.follow(env, target, d, e, LerpFloat)
	}
	if target <= 0 {
		d = 0
	} else if !f.set && shown > 0 && d > 0 {
		f.jump(shown)
		f.set = true
	}
	return f.follow(env, target, d, e, LerpFloat)
}
