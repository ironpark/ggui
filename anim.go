package ggui

import (
	"image/color"
	"slices"
	"sync"
	"time"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/pkg/kinetics"
)

// Animation is a reactive value that moves toward its target over time
// instead of jumping, after Svelte's tweened and spring stores. Tween moves
// along an easing curve for a fixed duration; Spring moves with physics and
// keeps its momentum when retargeted mid-flight. Both are Readers, so a
// Reactive island that reads one rebuilds every frame the value moves:
//
//	width := ggui.Tween(0.0, 200*time.Millisecond)
//	...
//	ggui.Reactive(func() ggui.Widget { return ggui.Box().Size(width.Get(), 4) })
//	...
//	width.Set(120) // slides there over 200ms
//
// The runtime steps every running animation once per frame, before effects
// are flushed. Tween and Spring created under an owner stop and release their
// registration when that owner is disposed; their value then stays frozen.
// Create component-local animations in setup so they survive builder reruns.
// Animations created without an owner live until they settle or Jump stops them.

// Easing maps normalized time in [0, 1] to normalized progress.
type Easing func(t float64) float64

// EaseLinear moves at constant speed.
func EaseLinear(t float64) float64 { return t }

// EaseOut starts fast and decelerates (cubic). It is the default: most UI
// motion should arrive gently.
func EaseOut(t float64) float64 { return kinetics.EaseOut(t) }

// EaseIn starts slowly and accelerates (cubic).
func EaseIn(t float64) float64 { return kinetics.EaseIn(t) }

// EaseInOut accelerates, then decelerates (cubic).
func EaseInOut(t float64) float64 { return kinetics.EaseInOut(t) }

// The curves below are pkg/kinetics', where their tests and
// benchmarks are.

// CubicBezier returns the easing CSS writes cubic-bezier(x1, y1, x2, y2): the
// curve from (0, 0) to (1, 1) with control points (x1, y1) and (x2, y2),
// read as progress y at time x. x1 and x2 are held to [0, 1] so time runs
// forward; y1 and y2 may leave it to overshoot.
//
//	ease := ggui.CubicBezier(0.4, 0, 0.2, 1)
func CubicBezier(x1, y1, x2, y2 float64) Easing { return kinetics.CubicBezier(x1, y1, x2, y2) }

// Interval runs e over the stretch of time from begin to end, both in
// [0, 1]: zero before it, one after. Give the parts of one animation
// overlapping or consecutive intervals to sequence them. A nil e is linear.
//
//	fade := ggui.Interval(0, 0.5, ggui.EaseOut)   // first half
//	grow := ggui.Interval(0.25, 1, ggui.EaseInOut) // overlapping the rest
func Interval(begin, end float64, e Easing) Easing { return kinetics.Interval(begin, end, e) }

// Reverse returns e flipped end for end: t maps to 1 - e(1-t), so an easing
// in becomes the matching easing out. Use it for the way back, so a panel
// that slid in with EaseOutBack leaves with the same curve mirrored. A nil
// e is linear, as in Interval.
func Reverse(e Easing) Easing { return kinetics.Reverse(e) }

// EaseInOutCubic is EaseInOut, by its full name.
func EaseInOutCubic(t float64) float64 { return kinetics.EaseInOutCubic(t) }

// EaseInBack pulls back a little before it moves forward.
func EaseInBack(t float64) float64 { return kinetics.EaseInBack(t) }

// EaseOutBack overshoots its target a little and settles back.
func EaseOutBack(t float64) float64 { return kinetics.EaseOutBack(t) }

// EaseInOutBack pulls back at the start and overshoots at the end.
func EaseInOutBack(t float64) float64 { return kinetics.EaseInOutBack(t) }

// EaseOutElastic overshoots and rings around its target like a plucked
// band before settling.
func EaseOutElastic(t float64) float64 { return kinetics.EaseOutElastic(t) }

// EaseInElastic is EaseOutElastic reversed: it winds up before it leaves.
func EaseInElastic(t float64) float64 { return kinetics.EaseInElastic(t) }

// EaseOutBounce drops onto its target and bounces to rest.
func EaseOutBounce(t float64) float64 { return kinetics.EaseOutBounce(t) }

// EaseInBounce is EaseOutBounce reversed: it bounces off its start.
func EaseInBounce(t float64) float64 { return kinetics.EaseInBounce(t) }

// EaseStandard is Material's standard curve, cubic-bezier(0.2, 0, 0, 1).
func EaseStandard(t float64) float64 { return kinetics.EaseStandard(t) }

// EaseStandardAccelerate is Material's standard curve for an exit.
func EaseStandardAccelerate(t float64) float64 { return kinetics.EaseStandardAccelerate(t) }

// EaseStandardDecelerate is Material's standard curve for an entrance.
func EaseStandardDecelerate(t float64) float64 { return kinetics.EaseStandardDecelerate(t) }

// EaseEmphasized is Material's emphasized curve: a slow start, a quick
// middle and a long, gentle arrival. It is the two halves below joined at
// (1/6, 0.4), as Material specifies it.
func EaseEmphasized(t float64) float64 { return kinetics.EaseEmphasized(t) }

// EaseEmphasizedAccelerate is Material's emphasized curve for an exit.
func EaseEmphasizedAccelerate(t float64) float64 { return kinetics.EaseEmphasizedAccelerate(t) }

// EaseEmphasizedDecelerate is Material's emphasized curve for an entrance.
func EaseEmphasizedDecelerate(t float64) float64 { return kinetics.EaseEmphasizedDecelerate(t) }

// stepper is an animation the runtime advances each frame.
type stepper interface {
	step(now time.Time) (running bool)
	active() bool
	// slot is where the animator keeps the animation's place in its list.
	slot() *animSlot
}

// animSlot is an animation's place in its animator's list, so that a Set
// on one already running, as a tween retargeted every frame is, finds it
// registered without searching the list or recording where it was set.
// The place is confirmed against the list rather than trusted, so a list
// emptied from outside, as tests do, leaves no animation thinking itself
// registered.
type animSlot struct{ index int }

type animator struct {
	mu   sync.Mutex
	list []stepper
	// origins are where list's animations were started, for MotionReport,
	// resolved only when a frame is counted against them.
	origins  []callStack
	stepping []stepper // step's copy of list
}

// registered reports whether s is in the list, at the place it keeps.
func (a *animator) registered(s stepper) bool {
	i := s.slot().index
	return i >= 0 && i < len(a.list) && a.list[i] == s
}

func (a *animator) add(s stepper) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.registered(s) {
		return
	}
	s.slot().index = len(a.list)
	a.list = append(a.list, s)
	a.origins = append(a.origins, callerStack())
}

// noteDirtiers counts where the running animations were started as having
// marked a frame dirty.
func (a *animator) noteDirtiers() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.origins {
		noteDirtier(dirtyAnim, a.origins[i].outside())
	}
}

// remove drops a stopped animation immediately, even if no more frames run.
func (a *animator) remove(s stepper) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.registered(s) {
		return
	}
	i := s.slot().index
	a.list = slices.Delete(a.list, i, i+1)
	a.origins = slices.Delete(a.origins, i, i+1)
	for j := i; j < len(a.list); j++ {
		a.list[j].slot().index = j
	}
}

// active reports whether any animation is running.
func (a *animator) active() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.list) > 0
}

// step advances every running animation and drops the ones that finished.
func (a *animator) step(now time.Time) {
	a.mu.Lock()
	// The copy steps outside the lock, in a buffer kept from step to step
	// (taken, so that a step within a step makes its own).
	list := append(a.stepping[:0], a.list...)
	a.stepping = nil
	a.mu.Unlock()

	for _, s := range list {
		s.step(now)
	}
	// Filter the live list once: callbacks may have added or stopped other
	// animations during step. Preserve new registrations, and avoid a
	// quadratic series of removals when many animations finish together.
	a.mu.Lock()
	n := 0
	for i, s := range a.list {
		if s.active() {
			a.list[n], a.origins[n] = s, a.origins[i]
			s.slot().index = n
			n++
		}
	}
	clear(a.list[n:])
	a.list, a.origins = a.list[:n], a.origins[:n]
	clear(list)
	a.stepping = list[:0]
	a.mu.Unlock()
}

// Tweened is a value that eases from where it is to its target over a fixed
// duration. Build one with Tween, or TweenWith for a value that is not a
// number.
type Tweened[T any] struct {
	w        *world // whose frames step it, from creation
	sig      *StateValue[T]
	duration time.Duration
	delay    time.Duration
	ease     Easing
	lerp     func(a, b T, t float64) T

	from, to T
	start    time.Time
	running  bool
	disposed bool
	place    animSlot
}

// Tween creates a Tweened at v that takes d to reach each new target, with
// EaseOut. T is inferred from v, so write Tween(0.0, d) for a float64.
func Tween[T Number](v T, d time.Duration) *Tweened[T] { return TweenWith(v, d, lerpNumber[T]) }

// TweenWith creates a Tweened at v for any type, moved between targets by
// lerp: LerpColor, LerpPoint, LerpSize, LerpRect, LerpInsets, or a func of
// your own that returns a at zero and b at one.
//
//	at := ggui.TweenWith(ggui.Pt(0, 0), 200*time.Millisecond, ggui.LerpPoint)
func TweenWith[T any](v T, d time.Duration, lerp func(a, b T, t float64) T) *Tweened[T] {
	if lerp == nil {
		panic("ggui: TweenWith requires a non-nil lerp")
	}
	t := &Tweened[T]{w: ownerWorld(), sig: State(v), duration: d, ease: EaseOut, lerp: lerp, from: v, to: v}
	if reactive.CurrentOwner() != nil {
		OnCleanup(func() {
			t.disposed, t.running = true, false
			t.w.anims.remove(t)
		})
	}
	return t
}

// TweenOf creates a Tweened that follows src: it starts at src's value and
// eases to each new one over d. It watches src, so like Watch it needs an
// owner: create it in App.Setup or component setup.
//
//	bar := ui.Progress(ggui.TweenOf(fraction, 300*time.Millisecond))
func TweenOf[T Number](src Readable[T], d time.Duration) *Tweened[T] {
	t := Tween(Untrack(src.Get), d)
	Watch(src, t.Set)
	return t
}

// Easing sets the curve; see EaseLinear, EaseIn, EaseOut, EaseInOut.
func (t *Tweened[T]) Easing(e Easing) *Tweened[T] { t.ease = e; return t }

// Duration sets how long each move takes.
func (t *Tweened[T]) Duration(d time.Duration) *Tweened[T] { t.duration = d; return t }

// Delay holds each move at its start for d before it begins: give the items
// of a list growing delays to stagger them.
func (t *Tweened[T]) Delay(d time.Duration) *Tweened[T] { t.delay = d; return t }

// Get returns the current value and subscribes the running Effect, so a
// Builder that reads it rebuilds as the value moves.
func (t *Tweened[T]) Get() T { return t.sig.Get() }

// GetAny returns the value as any and subscribes, for Sprintf.
func (t *Tweened[T]) GetAny() any { return t.Get() }

// Target returns where the value is heading.
func (t *Tweened[T]) Target() T { return t.to }

// Set starts moving from the current value to target.
func (t *Tweened[T]) Set(target T) {
	if t.disposed {
		return
	}
	t.from, t.to = Untrack(t.sig.Get), target
	if t.duration <= 0 && t.delay <= 0 || property.Equal(t.from, t.to) {
		t.sig.Set(target)
		t.running = false
		t.w.anims.remove(t)
		return
	}
	t.start = time.Time{} // taken from the first step
	t.running = true
	t.w.anims.add(t)
}

// Jump moves to v at once, with no animation.
func (t *Tweened[T]) Jump(v T) {
	if t.disposed {
		return
	}
	t.from, t.to, t.running = v, v, false
	t.w.anims.remove(t)
	t.sig.Set(v)
}

func (t *Tweened[T]) active() bool    { return t.running }
func (t *Tweened[T]) slot() *animSlot { return &t.place }

func (t *Tweened[T]) step(now time.Time) bool {
	if !t.running {
		return false
	}
	if t.start.IsZero() {
		t.start = now.Add(t.delay)
	}
	if now.Before(t.start) {
		return true // still waiting out the delay
	}
	p := 1.0
	if t.duration > 0 {
		p = float64(now.Sub(t.start)) / float64(t.duration)
	}
	if p >= 1 {
		t.sig.Set(t.to)
		t.running = false
		return false
	}
	f := t.ease(clamp(p, 0, 1))
	if !t.running {
		return false
	}
	t.sig.Set(t.lerp(t.from, t.to, f))
	return true
}

// lerpNumber is the straight line from a to b that Tween moves numbers on.
func lerpNumber[T Number](a, b T, t float64) T { return T(float64(a) + (float64(b)-float64(a))*t) }

// Sprung is a value that moves toward its target like a mass on a spring:
// it overshoots a little, keeps its momentum when the target changes, and
// settles. Build one with Spring, or SpringWith for a value that is not a
// number.
type Sprung[T any] struct {
	w        *world // whose frames step it, from creation
	sig      *StateValue[T]
	vec      Vector[T]
	k        [4]kinetics.Spring // one for each number vec splits T into
	goal     T                  // the target as given, which the spring lands on exactly
	last     time.Time
	running  bool
	disposed bool
	place    animSlot
}

// Spring creates a Sprung at v. Stiffness and Damping tune the motion; the
// defaults settle in a few hundred milliseconds with a slight overshoot.
func Spring[T Number](v T) *Sprung[T] {
	return SpringWith(v, Vector[T]{
		Split: func(v T) [4]float64 { return [4]float64{float64(v)} },
		Join:  func(c [4]float64) T { return T(c[0]) },
	})
}

// SpringWith creates a Sprung at v for any type vec splits into numbers:
// ColorVector, PointVector, SizeVector, RectVector, InsetsVector, or one of
// your own. Each number springs on its own, with the same stiffness.
//
//	at := ggui.SpringWith(ggui.Pt(0, 0), ggui.PointVector)
func SpringWith[T any](v T, vec Vector[T]) *Sprung[T] {
	if vec.Split == nil || vec.Join == nil {
		panic("ggui: SpringWith requires a Vector with Split and Join")
	}
	c := vec.Split(v)
	s := &Sprung[T]{w: ownerWorld(), sig: State(v), vec: vec, goal: v}
	for i := range s.k {
		s.k[i] = kinetics.NewSpring(c[i])
	}
	if reactive.CurrentOwner() != nil {
		OnCleanup(func() {
			s.disposed, s.running = true, false
			s.w.anims.remove(s)
		})
	}
	return s
}

// SpringOf creates a Sprung that follows src, as TweenOf does for a tween.
// It needs an owner, like Watch.
func SpringOf[T Number](src Readable[T]) *Sprung[T] {
	s := Spring(Untrack(src.Get))
	Watch(src, s.Set)
	return s
}

// Stiffness sets the spring constant: higher snaps faster.
func (s *Sprung[T]) Stiffness(k float64) *Sprung[T] {
	for i := range s.k {
		s.k[i].Stiffness = k
	}
	return s
}

// Damping sets the friction: lower overshoots more, higher creeps.
func (s *Sprung[T]) Damping(d float64) *Sprung[T] {
	for i := range s.k {
		s.k[i].Damping = d
	}
	return s
}

// Get returns the current value and subscribes the running Effect.
func (s *Sprung[T]) Get() T { return s.sig.Get() }

// GetAny returns the value as any and subscribes, for Sprintf.
func (s *Sprung[T]) GetAny() any { return s.Get() }

// Target returns where the value is heading.
func (s *Sprung[T]) Target() T { return s.goal }

// Set retargets the spring; motion already under way carries over.
func (s *Sprung[T]) Set(target T) {
	if s.disposed {
		return
	}
	s.goal = target
	for i, to := range s.vec.Split(target) {
		s.k[i].To = to
	}
	if !s.running {
		s.last = time.Time{}
		s.running = true
		s.w.anims.add(s)
	}
}

// Jump moves to v at once and stops.
func (s *Sprung[T]) Jump(v T) {
	if s.disposed {
		return
	}
	c := s.vec.Split(v)
	s.goal, s.running = v, false
	for i := range s.k {
		s.k[i].Pos, s.k[i].Vel, s.k[i].To = c[i], 0, c[i]
	}
	s.w.anims.remove(s)
	s.sig.Set(v)
}

func (s *Sprung[T]) active() bool    { return s.running }
func (s *Sprung[T]) slot() *animSlot { return &s.place }

func (s *Sprung[T]) step(now time.Time) bool {
	if !s.running {
		return false
	}
	if s.last.IsZero() {
		s.last = now
		return true
	}
	dt := now.Sub(s.last).Seconds()
	s.last = now
	rest := true
	var pos [4]float64
	for i := range s.k {
		rest = s.k[i].Step(dt) && rest
		pos[i] = s.k[i].Pos
	}
	if rest {
		s.running = false
		s.sig.Set(s.goal)
		return false
	}
	s.sig.Set(s.vec.Join(pos))
	return true
}

// Motion is a tween a widget runs inside Paint, for looks that move (a
// switch knob sliding over) without a signal or a rebuild. The zero value is
// ready: the first MoveTo sets the position without animating, later ones
// ease to the target with EaseOut, starting from wherever the previous move
// had got to, and Value reports where it is at a given time.
//
//	func (s *knob) Paint(dst *ggui.Canvas, r ggui.Rect) {
//		now := ggui.Now()
//		s.pos.MoveTo(target, now, 150*time.Millisecond)
//		x := r.Origin.X + s.pos.Value(now)*r.Size.W
//		...
//	}
type Motion struct {
	from, to float64
	start    time.Time
	duration time.Duration
	init     bool
}

// MoveTo retargets the motion, starting from its position at now and
// arriving after d.
func (m *Motion) MoveTo(target float64, now time.Time, d time.Duration) {
	if !m.init || d <= 0 {
		m.duration = 0
		m.from, m.to, m.init = target, target, true
		return
	}
	if target == m.to {
		return
	}
	m.from, m.to, m.start, m.duration = m.Value(now), target, now, d
}

// Toggle moves toward one while on and zero while off, over d, and
// returns the position at now: the progress of something revealing
// itself. A widget that shows or hides a panel calls it once per frame.
func (m *Motion) Toggle(on bool, now time.Time, d time.Duration) float64 {
	m.MoveTo(pick(on, 1.0, 0.0), now, d)
	return m.Value(now)
}

// Value returns the position at now. While the motion is still moving it
// asks for another frame, so the caller may read now with FrameTime.
func (m *Motion) Value(now time.Time) float64 {
	if m.duration <= 0 || m.from == m.to {
		return m.to
	}
	p := float64(now.Sub(m.start)) / float64(m.duration)
	if p >= 1 {
		m.from = m.to
		return m.to
	}
	activeWorld().frame.animate()
	return m.from + (m.to-m.from)*EaseOut(clamp(p, 0, 1))
}

// A lerp moves between two values: it returns a at t = 0, b at t = 1, and
// what lies between for the rest. TweenWith takes one, so a tween can move
// any type; the ones here cover the values widgets are configured with.
// t may leave [0, 1] under an easing that overshoots, such as EaseOutBack;
// these extrapolate along the same line, and LerpColor clamps.

// LerpColor blends a into b in premultiplied sRGB, as CSS transitions do,
// so a fade to or from transparent does not darken through a gray fringe.
// A nil color is transparent. It returns a and b themselves at the ends,
// so a theme token tweened to stays that token.
func LerpColor(a, b color.Color, t float64) color.Color {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return ColorVector.Join(lerp4(ColorVector.Split(a), ColorVector.Split(b), t))
}

// LerpFloat is the straight line from a to b.
func LerpFloat(a, b, t float64) float64 { return a + (b-a)*t }

// LerpPoint moves along the straight line from a to b.
func LerpPoint(a, b Point, t float64) Point { return PointVector.Lerp(a, b, t) }

// LerpSize grows or shrinks a toward b.
func LerpSize(a, b Size, t float64) Size { return SizeVector.Lerp(a, b, t) }

// LerpRect moves and resizes a toward b.
func LerpRect(a, b Rect, t float64) Rect { return RectVector.Lerp(a, b, t) }

// LerpInsets moves each side of a toward b's.
func LerpInsets(a, b EdgeInsets, t float64) EdgeInsets { return InsetsVector.Lerp(a, b, t) }

func lerp4(a, b [4]float64, t float64) (c [4]float64) {
	for i := range c {
		c[i] = LerpFloat(a[i], b[i], t)
	}
	return c
}

// Vector splits a value into at most four numbers and joins them back, so a
// Spring can move each number on its own. Unused numbers stay zero.
type Vector[T any] struct {
	Split func(T) [4]float64
	Join  func([4]float64) T
}

// Lerp blends a into b number by number: the lerp a Vector implies.
func (v Vector[T]) Lerp(a, b T, t float64) T { return v.Join(lerp4(v.Split(a), v.Split(b), t)) }

var (
	// ColorVector splits a color into premultiplied red, green, blue and
	// alpha from 0 to 255. Join clamps, so a spring overshooting a color
	// stays a valid one.
	ColorVector = Vector[color.Color]{Split: splitColor, Join: joinColor}

	// PointVector splits a Point into X and Y.
	PointVector = Vector[Point]{
		Split: func(p Point) [4]float64 { return [4]float64{p.X, p.Y} },
		Join:  func(c [4]float64) Point { return Pt(c[0], c[1]) },
	}

	// SizeVector splits a Size into W and H.
	SizeVector = Vector[Size]{
		Split: func(s Size) [4]float64 { return [4]float64{s.W, s.H} },
		Join:  func(c [4]float64) Size { return Sz(c[0], c[1]) },
	}

	// RectVector splits a Rect into its origin and size.
	RectVector = Vector[Rect]{
		Split: func(r Rect) [4]float64 { return [4]float64{r.Origin.X, r.Origin.Y, r.Size.W, r.Size.H} },
		Join:  func(c [4]float64) Rect { return Rct(Pt(c[0], c[1]), Sz(c[2], c[3])) },
	}

	// InsetsVector splits EdgeInsets into top, right, bottom and left.
	InsetsVector = Vector[EdgeInsets]{
		Split: func(e EdgeInsets) [4]float64 { return [4]float64{e.Top, e.Right, e.Bottom, e.Left} },
		Join:  func(c [4]float64) EdgeInsets { return EdgeInsets{Top: c[0], Right: c[1], Bottom: c[2], Left: c[3]} },
	}
)

func splitColor(c color.Color) [4]float64 {
	if c == nil {
		return [4]float64{}
	}
	r, g, b, a := c.RGBA()
	const k = 255.0 / 0xffff
	return [4]float64{float64(r) * k, float64(g) * k, float64(b) * k, float64(a) * k}
}

func joinColor(c [4]float64) color.Color {
	a := clamp(c[3], 0, 255)
	ch := func(v float64) uint16 { return uint16(clamp(v, 0, a)*0xffff/255 + 0.5) }
	return color.RGBA64{R: ch(c[0]), G: ch(c[1]), B: ch(c[2]), A: uint16(a*0xffff/255 + 0.5)}
}

// TweenColor creates a Tweened color that blends with LerpColor.
//
//	fill := ggui.TweenColor(idle, 150*time.Millisecond)
//	fill.Set(hover)
func TweenColor(c color.Color, d time.Duration) *Tweened[color.Color] {
	return TweenWith(c, d, LerpColor)
}

// TweenOfWith creates a Tweened that follows src, as TweenOf does, for any
// type lerp can move. It needs an owner, like Watch.
func TweenOfWith[T any](src Readable[T], d time.Duration, lerp func(a, b T, t float64) T) *Tweened[T] {
	t := TweenWith(Untrack(src.Get), d, lerp)
	Watch(src, t.Set)
	return t
}

// SpringOfWith creates a Sprung that follows src, as SpringOf does, for any
// type vec splits. It needs an owner, like Watch.
func SpringOfWith[T any](src Readable[T], vec Vector[T]) *Sprung[T] {
	s := SpringWith(Untrack(src.Get), vec)
	Watch(src, s.Set)
	return s
}

// TimelineValue is the progress of one animation from zero to one, played
// forward and back on command: what several values move by together, so
// they start, reverse and stop as one. It is a Readable; derive the values
// it drives with Interval, Stagger and Interpolate. Build one with
// Timeline.
//
//	tl := ggui.Timeline(400 * time.Millisecond)
//	fade := tl.Interval(0, 0.5, ggui.EaseOut)
//	rise := ggui.Interpolate(tl.Interval(0.2, 1, ggui.EaseOutBack), 24.0, 0.0, ggui.LerpFloat)
//	...
//	tl.Play()
//
// Its progress itself is linear in time; the curves belong to what reads it.
// When the application's environment asks for reduced motion (SetEnv with
// ReducedMotionKey), Play and Reverse land on their end at once.
type TimelineValue struct {
	w         *world // whose frames step it, from creation
	sig       *StateValue[float64]
	duration  time.Duration
	repeats   int // extra cycles each play runs, negative for ever
	alternate bool
	done      []func()

	from, dir float64 // progress the current cycle started at, and its direction
	left      int     // cycles still to run after this one
	start     time.Time
	running   bool
	disposed  bool
	place     animSlot
}

// Timeline creates a TimelineValue at zero that takes d to play through.
// Like Tween, one created under an owner stops when the owner is disposed;
// create it in setup.
func Timeline(d time.Duration) *TimelineValue {
	tl := &TimelineValue{w: ownerWorld(), sig: State(0.0), duration: d, dir: 1}
	if reactive.CurrentOwner() != nil {
		OnCleanup(func() {
			tl.disposed, tl.running = true, false
			tl.w.anims.remove(tl)
		})
	}
	return tl
}

// Duration sets how long playing from zero to one takes.
func (tl *TimelineValue) Duration(d time.Duration) *TimelineValue { tl.duration = d; return tl }

// Repeat makes each Play or Reverse run n more cycles after the first,
// starting each again from its beginning; a negative n repeats until Stop.
func (tl *TimelineValue) Repeat(n int) *TimelineValue { tl.repeats = n; return tl }

// Alternate makes repeated cycles turn around at each end instead of
// starting over, for a pulse or a ping-pong.
func (tl *TimelineValue) Alternate() *TimelineValue { tl.alternate = true; return tl }

// OnDone calls fn each time a play or reverse runs to its end, every
// repeat included. Stop and Seek do not call it.
func (tl *TimelineValue) OnDone(fn func()) *TimelineValue { tl.done = append(tl.done, fn); return tl }

// Get returns the progress, from zero to one, and subscribes the running
// Effect.
func (tl *TimelineValue) Get() float64 { return tl.sig.Get() }

// GetAny returns the progress as any and subscribes, for Sprintf.
func (tl *TimelineValue) GetAny() any { return tl.Get() }

// Running reports whether the timeline is playing, without subscribing.
func (tl *TimelineValue) Running() bool { return tl.running }

// Play runs the progress forward to one from where it is, or from zero
// when it is already there.
func (tl *TimelineValue) Play() { tl.run(1) }

// Reverse runs the progress back to zero from where it is, or from one
// when it is already there.
func (tl *TimelineValue) Reverse() { tl.run(-1) }

// Stop freezes the progress where it is.
func (tl *TimelineValue) Stop() {
	tl.running = false
	tl.w.anims.remove(tl)
}

// Seek stops the timeline and puts its progress at p, held to [0, 1].
func (tl *TimelineValue) Seek(p float64) {
	if tl.disposed {
		return
	}
	tl.Stop()
	tl.sig.Set(clamp(p, 0, 1))
}

func (tl *TimelineValue) run(dir float64) {
	if tl.disposed {
		return
	}
	end := pick(dir > 0, 1.0, 0.0)
	if tl.duration <= 0 || Untrack(tl.w.environment().Get).ReducedMotion() {
		tl.Seek(end)
		tl.finish()
		return
	}
	p := Untrack(tl.sig.Get)
	if p == end {
		p = 1 - end
	}
	tl.from, tl.dir, tl.left = p, dir, tl.repeats
	tl.start = time.Time{} // taken from the first step
	tl.running = true
	tl.w.anims.add(tl)
}

func (tl *TimelineValue) finish() {
	for _, fn := range tl.done {
		fn()
	}
}

// end is the progress the current run is heading for.
func (tl *TimelineValue) end() float64 { return pick(tl.dir > 0, 1.0, 0.0) }

func (tl *TimelineValue) active() bool    { return tl.running }
func (tl *TimelineValue) slot() *animSlot { return &tl.place }

func (tl *TimelineValue) step(now time.Time) bool {
	if !tl.running {
		return false
	}
	if tl.duration <= 0 {
		// Duration was cut to zero mid-run: no cycle would ever advance the
		// clock, so land on the end as run does, rather than loop for ever.
		tl.running = false
		tl.sig.Set(tl.end())
		tl.finish()
		return false
	}
	if tl.start.IsZero() {
		tl.start = now
	}
	d := float64(tl.duration)
	for {
		end := tl.end()
		left := (end - tl.from) * tl.dir // the part of the run still ahead
		elapsed := float64(now.Sub(tl.start)) / d
		if elapsed < left {
			tl.sig.Set(tl.from + tl.dir*elapsed)
			return true
		}
		if tl.left == 0 {
			tl.running = false
			tl.sig.Set(end)
			tl.finish()
			return false
		}
		// The cycle ended inside this frame: start the next where it did.
		tl.start = tl.start.Add(time.Duration(left * d))
		if tl.left > 0 {
			tl.left--
		}
		tl.sig.Set(end) // what a done callback reads
		tl.finish()
		if !tl.running || tl.start.IsZero() {
			// A done callback stopped it, or played it again from where it
			// stands: that new run takes its start from the next step.
			return tl.running
		}
		if tl.alternate {
			tl.from, tl.dir = end, -tl.dir
		} else {
			tl.from = 1 - end
		}
	}
}

// Interval returns the timeline's progress through the stretch from begin
// to end, eased by e: zero before it, one after. See the package Interval.
func (tl *TimelineValue) Interval(begin, end float64, e Easing) Readable[float64] {
	return mapped[float64, float64]{tl, Interval(begin, end, e)}
}

// Stagger returns the progress of item i of n, eased by e, when each item
// plays for span of the timeline and their starts spread evenly across the
// rest: span 1 moves them all together, 1/n one after another. Use it for
// the rows of a list arriving in turn.
//
//	for i, row := range rows {
//		p := tl.Stagger(i, len(rows), 0.4, ggui.EaseOut)
//		... ggui.Interpolate(p, 12.0, 0.0, ggui.LerpFloat) ...
//	}
func (tl *TimelineValue) Stagger(i, n int, span float64, e Easing) Readable[float64] {
	span = clamp(span, 0, 1)
	if n <= 1 || span >= 1 {
		return tl.Interval(0, 1, e)
	}
	begin := float64(i) * (1 - span) / float64(n-1)
	return tl.Interval(begin, begin+span, e)
}

// mapped reads src through fn on every Get. Interval and Interpolate
// return one rather than a Map, which is a memo: that needs an owner and
// runs eagerly, and these are built anywhere and read once per frame.
type mapped[T, U any] struct {
	src Readable[T]
	fn  func(T) U
}

func (m mapped[T, U]) Get() U { return m.fn(m.src.Get()) }

// Interpolate returns the value lerp gives between from and to at progress
// p: a Readable to bind a widget's property to, so it moves as p does.
//
//	box.BindWidth(ggui.Interpolate(tl, 40.0, 240.0, ggui.LerpFloat))
func Interpolate[T any](p Readable[float64], from, to T, lerp func(a, b T, t float64) T) Readable[T] {
	return mapped[float64, T]{p, func(t float64) T { return lerp(from, to, t) }}
}
