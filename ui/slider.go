package ui

import (
	"image/color"
	"math"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// SliderWidget picks a number in a range by dragging a knob. Build one with
// Slider.
type SliderWidget struct {
	props property.Owner
	ggui.Interactive
	value    ggui.Binding[float64]
	min, max float64
	step     float64
	onChange func(float64)
	onCommit func(float64)

	theme uitheme.Theme
	rect  ggui.Rect
}

// Slider binds a horizontal slider to value, clamped to [lo, hi]. It fills
// the width it is given.
func Slider(value ggui.Binding[float64], lo, hi float64) *SliderWidget {
	s := &SliderWidget{value: value, min: lo, max: hi}
	s.Role = ggui.RoleSlider
	s.AutoKey()
	return s
}

// Name names the slider for Probe.Find and the inspector.
func (s *SliderWidget) Name(name string) *SliderWidget { s.SetName(name); return s }

// ConsumesKey implements ggui.KeyConsumer: the arrow keys move the value.
func (s *SliderWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyArrowDown:
		return ev.Kind == ggui.KeyPress
	}
	return false
}

// Step snaps the value to multiples of s from the range's start.
func (s *SliderWidget) Step(step float64) *SliderWidget {
	defer property.Watch(&s.props, &s.step)()
	s.step = step
	return s
}

// OnChange fires with the new value after a drag or key press moved it.
func (s *SliderWidget) OnChange(fn func(float64)) *SliderWidget { s.onChange = fn; return s }

// set stores v, clamped to the range, and reports the change.
func (s *SliderWidget) set(v float64) {
	setChanged(s.value, clamp(v, min(s.min, s.max), max(s.min, s.max)), s.onChange)
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
	step := s.step
	if step <= 0 {
		step = (s.max - s.min) / 100
	}
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
	defer s.props.Layout()()
	s.Sync()
	s.theme = uitheme.From(env)
	return c.Constrain(ggui.Sz(bounded(c.MaxW, defaultStripe), sliderKnob*2+4))
}

// fraction returns where value sits in the range, 0 to 1.
func (s *SliderWidget) fraction() float64 {
	if s.max == s.min {
		return 0
	}
	return clamp((ggui.Untrack(s.value.Get)-s.min)/(s.max-s.min), 0, 1)
}

// setFromX moves the value to where logical x falls on the track.
func (s *SliderWidget) setFromX(x float64) {
	track := s.rect.Size.W - 2*sliderKnob
	if track <= 0 {
		return
	}
	f := clamp((x-s.rect.Origin.X-sliderKnob)/track, 0, 1)
	v := s.min + f*(s.max-s.min)
	if s.step > 0 {
		v = s.min + math.Round((v-s.min)/s.step)*s.step
	}
	s.set(v)
}

// Paint implements Widget.
func (s *SliderWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	s.rect = r
	s.Hit(dst, r, s, ggui.CursorShapePointer)
	cy := r.Origin.Y + r.Size.H/2
	knob := min(float64(sliderKnob), r.Size.W/2, r.Size.H/2)
	x0, x1 := r.Origin.X+knob, r.Origin.X+r.Size.W-knob
	kx := x0 + (x1-x0)*s.fraction()
	dst.FillRoundRect(ggui.Rct(ggui.Pt(x0, cy-2), ggui.Sz(x1-x0, 4)), 2, t.Border)
	accent := pick(s.IsInert(), fade(t.Primary, .5), t.Primary)
	dst.FillRoundRect(ggui.Rct(ggui.Pt(x0, cy-2), ggui.Sz(kx-x0, 4)), 2, accent)
	radius := knob
	if !s.IsInert() && (s.Hovered || s.Pressed || (s.Focused && s.FocusVisible)) {
		dst.StrokeRoundRect(ggui.Rct(ggui.Pt(kx-radius-2, cy-radius-2), ggui.Sz(2*radius+4, 2*radius+4)), radius+2, 4, fade(t.Ring, .3))
	}
	dst.FillCircle(ggui.Pt(kx, cy), radius, accent)
	dst.FillCircle(ggui.Pt(kx, cy), max(radius-1, 0), pick(s.IsInert(), fade(color.White, .5), color.Color(color.White)))
	if s.Focused && s.FocusVisible {
		dst.StrokeRoundRect(ggui.Rct(ggui.Pt(kx-radius-2, cy-radius-2), ggui.Sz(2*radius+4, 2*radius+4)), radius+2, 2, t.Primary)
	}
}

// HandleKey implements KeyHandler: the arrow keys nudge the value by one
// step, or a hundredth of the range without one.
func (s *SliderWidget) HandleKey(ev ggui.KeyEvent) {
	s.Keyboard(ev, nil)
	if s.IsInert() || ev.Kind != ggui.KeyPress {
		return
	}
	step := s.step
	if step <= 0 {
		step = (s.max - s.min) / 100
	}
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowDown:
		step = -step
	case ggui.KeyArrowRight, ggui.KeyArrowUp:
	default:
		return
	}
	s.set(ggui.Untrack(s.value.Get) + step)
	s.commit()
}

// HandlePointer implements PointerHandler: a left press jumps to the
// pointer and a drag from there follows it, past the ends included.
func (s *SliderWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if s.IsInert() {
		return false
	}
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		s.setFromX(ev.Pos.X)
	case ggui.PointerDrag:
		if s.Pressed {
			s.setFromX(ev.Pos.X)
		}
	case ggui.PointerUp:
		if s.Pressed {
			s.commit()
		}
	}
	return s.Pointer(ev, nil)
}

// TextFieldWidget is a TextInput in a themed box: Field background, a
// border that turns Accent while focused, the theme's padding and radius.
