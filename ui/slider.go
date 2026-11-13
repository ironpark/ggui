package ui

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// SliderWidget picks a number in a range by dragging a knob. Build one with
// Slider.
type SliderWidget struct {
	props property.Owner
	ggui.Interactive
	value           ggui.Binding[float64]
	min, max        float64
	step, largeStep float64
	vertical, rtl   bool
	onChange        func(float64)
	onCommit        func(float64)

	theme   *uitheme.Theme
	rect    ggui.Rect
	motion  time.Duration // MotionFast, or zero under reduced motion
	pointer ggui.Point    // where the pointer last was while over the slider
}

// Slider binds a horizontal slider to value, clamped to [lo, hi]. It fills
// the width it is given. Step defaults to 1, and LargeStep to 10.
func Slider(value ggui.Binding[float64], lo, hi float64) *SliderWidget {
	s := &SliderWidget{value: value, min: lo, max: hi, step: 1, largeStep: 10}
	s.Role = ggui.RoleSlider
	s.AutoKey()
	return s
}

// Name names the slider for Probe.Find and the inspector.
func (s *SliderWidget) Name(name string) *SliderWidget { s.SetName(name); return s }

// ConsumesKey implements ggui.KeyConsumer: the arrow keys move the value.
func (s *SliderWidget) ConsumesKey(ev ggui.KeyEvent) bool { return sliderConsumesKey(ev) }

func sliderConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd, ggui.KeyPageUp, ggui.KeyPageDown:
		return ev.Kind == ggui.KeyPress
	}
	return false
}

// Step snaps the value to multiples of step from the range's start.
// Zero permits continuous dragging and uses range/100 for arrow keys.
func (s *SliderWidget) Step(step float64) *SliderWidget {
	defer property.Watch(&s.props, &s.step)()
	s.step = step
	return s
}

// LargeStep sets the value increment for PageUp/PageDown and Shift+arrows.
// It defaults to 10 independently of Step.
func (s *SliderWidget) LargeStep(step float64) *SliderWidget {
	if math.IsNaN(step) || math.IsInf(step, 0) || step <= 0 {
		panic("ui: Slider large step must be finite and positive")
	}
	defer property.Watch(&s.props, &s.largeStep)()
	s.largeStep = step
	return s
}

// Vertical makes the slider grow upward, filling the available height.
func (s *SliderWidget) Vertical() *SliderWidget {
	defer property.Watch(&s.props, &s.vertical)()
	s.vertical = true
	return s
}

// OnChange fires with the new value after a drag or key press moved it.
func (s *SliderWidget) OnChange(fn func(float64)) *SliderWidget { s.onChange = fn; return s }

// set stores v, clamped to the range, and reports the change.
func (s *SliderWidget) set(v float64) {
	if math.IsNaN(v) {
		return
	}
	lo, hi := min(s.min, s.max), max(s.min, s.max)
	v = clamp(v, lo, hi)
	if s.step > 0 && v != s.min && v != s.max {
		v = s.min + math.Round((v-s.min)/s.step)*s.step
	}
	setChanged(s.value, clamp(v, lo, hi), s.onChange)
}

// OnCommit fires with the value when a drag ends or a key press moved it,
// for work too costly to do on every frame of a drag.
func (s *SliderWidget) OnCommit(fn func(float64)) *SliderWidget { s.onCommit = fn; return s }

func (s *SliderWidget) commit() {
	if s.onCommit != nil {
		s.onCommit(ggui.Untrack(s.value.Get))
	}
}

// Disabled greys the slider out and ignores the pointer while v is true.
func (s *SliderWidget) Disabled(v bool) *SliderWidget { s.SetInert(v); return s }

// BindDisabled follows r for Disabled without a rebuild.
func (s *SliderWidget) BindDisabled(r ggui.Readable[bool]) *SliderWidget { s.BindInert(r); return s }

// Describe implements ggui.Describer: a slider reports its range and where
// in it the value sits, which is what a screen reader reads out and what an
// increment or decrement action steps through.
func (s *SliderWidget) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleSlider,
		Name:     s.SemanticName(),
		Min:      min(s.min, s.max),
		Max:      max(s.min, s.max),
		Now:      ggui.Untrack(s.value.Get),
		Disabled: s.IsInert(),
		Actions:  ggui.ActionIncrement | ggui.ActionDecrement | ggui.ActionSetValue | ggui.ActionFocus,
	}
}

// Act implements ggui.Actor: an assistive technology steps the value or
// sets it outright, and each ends with the commit a key press would.
func (s *SliderWidget) Act(a ggui.Action) bool {
	if s.IsInert() {
		return false
	}
	step := sliderKeyStep(s.step, s.min, s.max)
	switch a.Kind {
	case ggui.ActionIncrement:
		s.set(ggui.Untrack(s.value.Get) + step)
	case ggui.ActionDecrement:
		s.set(ggui.Untrack(s.value.Get) - step)
	case ggui.ActionSetValue:
		s.set(a.Num)
	default:
		return false
	}
	s.commit()
	return true
}

// Layout implements Widget.
func (s *SliderWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	s.Sync()
	if s.IsInert() {
		s.Pressed = false
	}
	s.theme = uitheme.Of(env)
	s.motion = env.Motion(s.theme.MotionFast)
	s.rtl = env.Direction() == ggui.DirRTL
	if s.vertical {
		return c.Constrain(ggui.Sz(s.theme.ControlSize+4, bounded(c.MaxH, defaultStripe)))
	}
	return c.Constrain(ggui.Sz(bounded(c.MaxW, defaultStripe), s.theme.ControlSize+4))
}

// fraction returns where value sits in the range, 0 to 1.
func (s *SliderWidget) fraction() float64 {
	if s.max == s.min {
		return 0
	}
	return clamp((ggui.Untrack(s.value.Get)-s.min)/(s.max-s.min), 0, 1)
}

func (s *SliderWidget) geometry() sliderGeometry {
	return newSliderGeometry(s.rect, s.theme.ControlSize*.375, s.vertical, s.rtl)
}

// setFromPosition moves the value to the pointer along the active axis.
func (s *SliderWidget) setFromPosition(pos ggui.Point) {
	f, ok := s.geometry().fraction(pos)
	if !ok {
		return
	}
	v := s.min + f*(s.max-s.min)
	if f == 0 {
		v = s.min
	} else if f == 1 {
		v = s.max
	}
	s.set(v)
}

// Paint implements Widget.
func (s *SliderWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	s.rect = r
	g := s.geometry()
	inert := s.IsInert()
	f := s.fraction()
	// The thumb is taken from a little outside the slider too, as shadcn's
	// after:-inset-2 has it.
	s.HitReach(dst, r, g.knobReach(f, t.Space), s, ggui.CursorShapeDefault)
	dst.FillRoundRect(g.track(0, 1), g.thick/2, sliderTone(t, t.Muted, inert))
	dst.FillRoundRect(g.track(0, f), g.thick/2, sliderTone(t, t.Primary, inert))
	// The halo belongs to the thumb, as shadcn's hover:ring-3 does: the
	// pointer elsewhere on the track leaves it alone.
	over := s.Hovered && (g.thumbReach(f, t.Space).Contains(s.pointer) || g.knobReach(f, t.Space).Contains(s.pointer))
	active := !inert && (over || s.Pressed || (s.Focused && s.FocusVisible))
	g.paintThumb(dst, t, f, inert, dst.Ease(s.Anchor(r), sliderHaloSlot, pick(active, 1.0, 0.0), s.motion))
}

// sliderHaloSlot eases the halo of a slider's thumb.
var sliderHaloSlot = ggui.NewSlot[*ggui.Motion]("slider halo")

// HandleKey implements KeyHandler: the arrow keys nudge the value by one
// step (1 by default), or a hundredth of the range with Step(0).
func (s *SliderWidget) HandleKey(ev ggui.KeyEvent) {
	s.Keyboard(ev, nil)
	if s.IsInert() || ev.Kind != ggui.KeyPress {
		return
	}
	step := sliderKeyStep(s.step, s.min, s.max)
	if ev.Mods.Shift || ev.Key == ggui.KeyPageUp || ev.Key == ggui.KeyPageDown {
		step = s.largeStep
	}
	ev = directionalKey(ev, s.rtl && !s.vertical)
	switch ev.Key {
	case ggui.KeyHome:
		s.set(s.min)
		s.commit()
		return
	case ggui.KeyEnd:
		s.set(s.max)
		s.commit()
		return
	case ggui.KeyArrowLeft, ggui.KeyArrowDown, ggui.KeyPageDown:
		step = -step
	case ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyPageUp:
	default:
		return
	}
	s.set(ggui.Untrack(s.value.Get) + step)
	s.commit()
}

// CaptureTouchDrag keeps slider gestures from being taken by a parent scroll.
func (s *SliderWidget) CaptureTouchDrag() bool { return !s.IsInert() }

// HandlePointer implements PointerHandler: a left press jumps to the
// pointer and a drag from there follows it, past the ends included.
func (s *SliderWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if s.IsInert() {
		s.Pressed = false
		return false
	}
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		s.pointer = ev.Pos
	case ggui.PointerDown:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		s.pointer = ev.Pos
		s.setFromPosition(ev.Pos)
	case ggui.PointerDrag:
		s.pointer = ev.Pos
		if s.Pressed {
			s.setFromPosition(ev.Pos)
		}
	case ggui.PointerUp:
		s.pointer = ev.Pos
		if s.Pressed {
			s.setFromPosition(ev.Pos)
			s.commit()
		}
	}
	return s.Pointer(ev, nil)
}

// TextFieldWidget is a TextInput in a themed box: Field background, a
// border that turns Accent while focused, the theme's padding and radius.

// sliderGeometry keeps pointer mapping, track fill and thumb placement in the
// same coordinate system, including small constraints and RTL/vertical axes.
// The track spans the whole rect and the thumb stays inside it, its edge
// meeting the track's end at either extreme, as shadcn's edge-aligned thumb
// does.
type sliderGeometry struct {
	rect              ggui.Rect
	knob              float64 // the thumb's radius
	thick             float64 // the track's thickness
	vertical, reverse bool
}

// newSliderGeometry lays a slider out in r around a thumb of radius knob;
// the track is two thirds of that thick, a quarter of the control glyph.
func newSliderGeometry(r ggui.Rect, knob float64, vertical, rtl bool) sliderGeometry {
	knob = max(0, min(knob, r.Size.W/2, r.Size.H/2))
	return sliderGeometry{r, knob, knob * 2 / 3, vertical, vertical || rtl}
}

// point is the thumb's center at fraction f.
func (g sliderGeometry) point(f float64) ggui.Point {
	if g.reverse {
		f = 1 - f
	}
	if g.vertical {
		return ggui.Pt(g.rect.Origin.X+g.rect.Size.W/2, g.rect.Origin.Y+g.knob+(g.rect.Size.H-2*g.knob)*f)
	}
	return ggui.Pt(g.rect.Origin.X+g.knob+(g.rect.Size.W-2*g.knob)*f, g.rect.Origin.Y+g.rect.Size.H/2)
}
func (g sliderGeometry) fraction(pos ggui.Point) (float64, bool) {
	at, start, length := pos.X, g.rect.Origin.X, g.rect.Size.W
	if g.vertical {
		at, start, length = pos.Y, g.rect.Origin.Y, g.rect.Size.H
	}
	if length <= 2*g.knob {
		return 0, false
	}
	f := clamp((at-start-g.knob)/(length-2*g.knob), 0, 1)
	if g.reverse {
		f = 1 - f
	}
	return f, true
}

// along is where fraction f lies on the track's axis: the thumb's center
// there, or the track's very end at 0 and 1, so that the track and a range
// reaching an extreme run the full length of the rect.
func (g sliderGeometry) along(f float64) float64 {
	p := g.point(f)
	at, start, length := p.X, g.rect.Origin.X, g.rect.Size.W
	if g.vertical {
		at, start, length = p.Y, g.rect.Origin.Y, g.rect.Size.H
	}
	if f <= 0 || f >= 1 {
		if (f >= 1) != g.reverse {
			return start + length
		}
		return start
	}
	return at
}

// track is the stretch of the track between two fractions.
func (g sliderGeometry) track(from, to float64) ggui.Rect {
	a, b := g.along(from), g.along(to)
	lo, length := min(a, b), math.Abs(b-a)
	if g.vertical {
		return ggui.Rct(ggui.Pt(g.rect.Origin.X+(g.rect.Size.W-g.thick)/2, lo), ggui.Sz(g.thick, length))
	}
	return ggui.Rct(ggui.Pt(lo, g.rect.Origin.Y+(g.rect.Size.H-g.thick)/2), ggui.Sz(length, g.thick))
}

// paintThumb draws the knob at fraction f with its halo at strength halo,
// from 0 for none to 1.
func (g sliderGeometry) paintThumb(dst *ggui.Canvas, t *uitheme.Theme, f float64, inert bool, halo float64) {
	center, radius := g.point(f), g.knob
	if halo > 0 && !inert {
		haloStroke(dst, ggui.Rct(center.Add(ggui.Pt(-radius, -radius)), ggui.Sz(2*radius, 2*radius)), radius, fade(t.Halo(t.Ring), halo))
	}
	ring, face := t.Ring, color.Color(color.White)
	if inert {
		ring, face = t.Disabled(ring), t.Disabled(face)
	}
	dst.FillCircle(center, radius, ring)
	dst.FillCircle(center, max(radius-t.BorderWidth, 0), face)
}

// sliderTone is c, dimmed for an inert slider.
func sliderTone(t *uitheme.Theme, c color.Color, inert bool) color.Color {
	if inert {
		return t.Disabled(c)
	}
	return c
}

// sliderKeyStep is the distance one arrow key moves: step, or a hundredth
// of the range when there is none.
func sliderKeyStep(step, lo, hi float64) float64 {
	if step > 0 {
		return step
	}
	return math.Abs(hi-lo) / 100
}

// thumbReach is thumb grown by reach along the track: where the pointer
// counts as over the thumb, as the margin around shadcn's does.
func (g sliderGeometry) thumbReach(f, reach float64) ggui.Rect {
	r := g.thumb(f)
	if g.vertical {
		r.Origin.Y -= reach
		r.Size.H += 2 * reach
	} else {
		r.Origin.X -= reach
		r.Size.W += 2 * reach
	}
	return r
}

// knobReach is the knob at fraction f grown by reach on every side: the
// margin around shadcn's thumb, which takes the pointer past the slider's
// own edges.
func (g sliderGeometry) knobReach(f, reach float64) ggui.Rect {
	p, d := g.point(f), g.knob+reach
	return ggui.Rct(ggui.Pt(p.X-d, p.Y-d), ggui.Sz(2*d, 2*d))
}

// thumb is the hit rect of the thumb at fraction f: the knob along the
// track and the whole slider across it.
func (g sliderGeometry) thumb(f float64) ggui.Rect {
	p := g.point(f)
	if g.vertical {
		return ggui.Rct(ggui.Pt(g.rect.Origin.X, p.Y-g.knob), ggui.Sz(g.rect.Size.W, 2*g.knob))
	}
	return ggui.Rct(ggui.Pt(p.X-g.knob, g.rect.Origin.Y), ggui.Sz(2*g.knob, g.rect.Size.H))
}
