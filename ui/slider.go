package ui

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// SliderWidget picks a number in a range by dragging a knob. Build one with
// Slider.
type SliderWidget struct {
	interactive
	value    *ggui.Signal[float64]
	min, max float64
	step     float64
	onChange func(float64)

	theme ggui.Theme
	rect  ggui.Rect
}

// Slider binds a horizontal slider to value, clamped to [lo, hi]. It fills
// the width it is given.
func Slider(value *ggui.Signal[float64], lo, hi float64) *SliderWidget {
	return &SliderWidget{value: value, min: lo, max: hi}
}

// Step snaps the value to multiples of s from the range's start.
func (s *SliderWidget) Step(step float64) *SliderWidget { s.step = step; return s }

// OnChange fires with the new value after a drag or key press moved it.
func (s *SliderWidget) OnChange(fn func(float64)) *SliderWidget { s.onChange = fn; return s }

// set stores v, clamped to the range, and reports the change.
func (s *SliderWidget) set(v float64) {
	v = clamp(v, min(s.min, s.max), max(s.min, s.max))
	if v == s.value.Peek() {
		return
	}
	s.value.Set(v)
	if s.onChange != nil {
		s.onChange(v)
	}
}

// Disabled greys the slider out and ignores the pointer while v is true.
func (s *SliderWidget) Disabled(v bool) *SliderWidget { s.disabled = v; return s }

// Layout implements Widget.
func (s *SliderWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s.theme = env.Theme()
	return c.Constrain(ggui.Sz(bounded(c.MaxW, defaultStripe), sliderKnob*2+4))
}

// fraction returns where value sits in the range, 0 to 1.
func (s *SliderWidget) fraction() float64 {
	if s.max == s.min {
		return 0
	}
	return clamp((s.value.Peek()-s.min)/(s.max-s.min), 0, 1)
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
	s.hit(dst, r, s, ebiten.CursorShapePointer)
	cy := r.Origin.Y + r.Size.H/2
	x0, x1 := r.Origin.X+sliderKnob, r.Origin.X+r.Size.W-sliderKnob
	kx := x0 + (x1-x0)*s.fraction()
	dst.FillRoundRect(ggui.Rct(ggui.Pt(x0, cy-2), ggui.Sz(x1-x0, 4)), 2, t.Border)
	accent := pick(s.disabled, t.Muted, pick(s.hovered || s.pressed, t.AccentHover, t.Accent))
	dst.FillRoundRect(ggui.Rct(ggui.Pt(x0, cy-2), ggui.Sz(kx-x0, 4)), 2, accent)
	radius := float64(sliderKnob)
	if s.hovered || s.pressed {
		radius++
	}
	dst.FillCircle(ggui.Pt(kx, cy), radius, accent)
	dst.FillCircle(ggui.Pt(kx, cy), radius-3, t.Field)
	if s.focus.focused && s.focus.ring {
		dst.StrokeRoundRect(ggui.Rct(ggui.Pt(kx-radius-2, cy-radius-2), ggui.Sz(2*radius+4, 2*radius+4)), radius+2, 2, t.Accent)
	}
}

// HandleKey implements KeyHandler: the arrow keys nudge the value by one
// step, or a hundredth of the range without one.
func (s *SliderWidget) HandleKey(ev ggui.KeyEvent) {
	s.focus.handle(ev)
	if ev.Kind != ggui.KeyPress {
		return
	}
	step := s.step
	if step <= 0 {
		step = (s.max - s.min) / 100
	}
	switch ev.Key {
	case ebiten.KeyArrowLeft, ebiten.KeyArrowDown:
		step = -step
	case ebiten.KeyArrowRight, ebiten.KeyArrowUp:
	default:
		return
	}
	s.set(s.value.Peek() + step)
}

// HandlePointer implements PointerHandler: a left press jumps to the
// pointer and a drag from there follows it, past the ends included.
func (s *SliderWidget) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ebiten.MouseButtonLeft {
			return false
		}
		s.setFromX(ev.Pos.X)
	case ggui.PointerDrag:
		if s.pressed {
			s.setFromX(ev.Pos.X)
		}
	}
	return s.pointer(ev, nil)
}

// TextFieldWidget is a TextInput in a themed box: Field background, a
// border that turns Accent while focused, the theme's padding and radius.
