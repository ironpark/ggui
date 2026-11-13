package ui

import (
	"fmt"
	"math"
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// RangeSliderWidget edits an ordered slice of values. Each thumb has its own
// stable identity, Tab stop and accessible range; thumbs never cross.
type RangeSliderWidget struct {
	ggui.Interactive
	props               property.Owner
	value               ggui.Binding[[]float64]
	min, max, step, gap float64
	largeStep           float64
	vertical, rtl       bool
	names               []string
	thumbs              []*rangeSliderThumb
	active              int
	focusThumb          bool
	onChange, onCommit  func([]float64)
	theme               *uitheme.Theme
	env                 ggui.Env
	rect                ggui.Rect
}

// RangeSlider binds any number of thumbs to values in [lo, hi]. Slice order
// identifies thumbs. An empty slice draws an inactive track. Invalid external
// values are projected into an ordered, bounded range for display, without
// writing the binding until a user edits it. Reversed bounds are accepted.
// Step defaults to 1 and LargeStep to 10.
func RangeSlider(value ggui.Binding[[]float64], lo, hi float64) *RangeSliderWidget {
	property.Require(value, "RangeSlider")
	if math.IsNaN(lo) || math.IsNaN(hi) || math.IsInf(lo, 0) || math.IsInf(hi, 0) || math.IsInf(hi-lo, 0) {
		panic("ui: RangeSlider requires finite bounds")
	}
	s := &RangeSliderWidget{value: value, min: min(lo, hi), max: max(lo, hi), step: 1, largeStep: 10, active: -1}
	s.Role = ggui.RoleGroup
	s.AutoKey()
	return s
}

func (s *RangeSliderWidget) Name(name string) *RangeSliderWidget { s.SetName(name); return s }

// BindName follows an accessible group name without rebuilding.
func (s *RangeSliderWidget) BindName(name ggui.Readable[string]) *RangeSliderWidget {
	s.Interactive.BindName(name)
	return s
}

func (s *RangeSliderWidget) Key(key any) *RangeSliderWidget     { s.SetKey(key); return s }
func (s *RangeSliderWidget) Disabled(v bool) *RangeSliderWidget { s.SetInert(v); return s }
func (s *RangeSliderWidget) BindDisabled(r ggui.Readable[bool]) *RangeSliderWidget {
	s.BindInert(r)
	return s
}

// LargeStep sets the value increment for PageUp/PageDown and Shift+arrows.
func (s *RangeSliderWidget) LargeStep(step float64) *RangeSliderWidget {
	if math.IsNaN(step) || math.IsInf(step, 0) || step <= 0 {
		panic("ui: RangeSlider large step must be finite and positive")
	}
	defer property.Watch(&s.props, &s.largeStep)()
	s.largeStep = step
	return s
}

// Vertical makes the range grow upward and fill the available height.
func (s *RangeSliderWidget) Vertical() *RangeSliderWidget {
	defer property.Watch(&s.props, &s.vertical)()
	s.vertical = true
	return s
}

// Step snaps edited values to multiples of step from the lower bound. Zero
// permits continuous dragging and uses one hundredth of the range for keys.
func (s *RangeSliderWidget) Step(step float64) *RangeSliderWidget {
	if math.IsNaN(step) || math.IsInf(step, 0) || step < 0 {
		panic("ui: RangeSlider step must be finite and nonnegative")
	}
	defer property.Watch(&s.props, &s.step)()
	s.step = step
	return s
}

// MinGap sets the minimum distance in value units between adjacent thumbs.
// If the range cannot fit the requested gap, it is reduced to fit all thumbs.
// A gap boundary takes precedence over Step when no step fits the interval.
func (s *RangeSliderWidget) MinGap(gap float64) *RangeSliderWidget {
	if math.IsNaN(gap) || math.IsInf(gap, 0) || gap < 0 {
		panic("ui: RangeSlider gap must be finite and nonnegative")
	}
	defer property.Watch(&s.props, &s.gap)()
	s.gap = gap
	return s
}

// ThumbNames supplies accessible labels in slice order. Missing names default
// to the group name followed by the one-based thumb number.
func (s *RangeSliderWidget) ThumbNames(names ...string) *RangeSliderWidget {
	if !slices.Equal(s.names, names) {
		s.names = slices.Clone(names)
		s.props.Changed()
	}
	return s
}

// OnChange reports a separate copy of the requested values after each edit.
// A controlled binding may reject the request; its getter remains authoritative.
func (s *RangeSliderWidget) OnChange(fn func([]float64)) *RangeSliderWidget {
	s.onChange = fn
	return s
}

// OnCommit reports the binding's accepted values once on pointer release, or
// after a keyboard/accessibility edit. Callback slices never alias the binding.
func (s *RangeSliderWidget) OnCommit(fn func([]float64)) *RangeSliderWidget {
	s.onCommit = fn
	return s
}

func (s *RangeSliderWidget) effectiveGap(n int) float64 {
	if n < 2 {
		return 0
	}
	return min(s.gap, (s.max-s.min)/float64(n-1))
}

func (s *RangeSliderWidget) values() []float64 {
	vs := slices.Clone(ggui.Untrack(s.value.Get))
	gap := s.effectiveGap(len(vs))
	for i, v := range vs {
		lo := s.min
		if i > 0 {
			lo = vs[i-1] + gap
		}
		hi := s.max - float64(len(vs)-1-i)*gap
		if math.IsNaN(v) {
			v = lo
		}
		vs[i] = clamp(v, min(lo, hi), hi)
	}
	return vs
}

func (s *RangeSliderWidget) limits(vs []float64, i int) (float64, float64) {
	lo, hi := s.min, s.max
	gap := s.effectiveGap(len(vs))
	if i > 0 {
		lo = vs[i-1] + gap
	}
	if i+1 < len(vs) {
		hi = vs[i+1] - gap
	}
	return min(lo, hi), hi
}

func (s *RangeSliderWidget) set(i int, v float64) {
	vs := s.values()
	if s.IsInert() || i < 0 || i >= len(vs) || math.IsNaN(v) {
		return
	}
	lo, hi := s.limits(vs, i)
	v = clamp(v, lo, hi)
	if s.step > 0 && v != s.min && v != s.max {
		first := math.Ceil((lo-s.min)/s.step - 1e-10)
		last := math.Floor((hi-s.min)/s.step + 1e-10)
		if first <= last {
			v = s.min + clamp(math.Round((v-s.min)/s.step), first, last)*s.step
		}
	}
	vs[i] = clamp(v, lo, hi)
	if slices.Equal(vs, ggui.Untrack(s.value.Get)) {
		return
	}
	s.value.Set(slices.Clone(vs))
	ggui.Invalidate(s.env)
	if s.onChange != nil {
		s.onChange(slices.Clone(vs))
	}
}

func (s *RangeSliderWidget) commit() {
	if s.onCommit != nil {
		s.onCommit(slices.Clone(ggui.Untrack(s.value.Get)))
	}
}

func (s *RangeSliderWidget) keyStep() float64 {
	if s.step > 0 {
		return s.step
	}
	return (s.max - s.min) / 100
}

func (s *RangeSliderWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	s.Sync()
	s.env, s.theme = env, uitheme.Of(env)
	s.rtl = env.Direction() == ggui.DirRTL
	n := len(s.value.Get())
	for len(s.thumbs) < n {
		h := &rangeSliderThumb{owner: s, index: len(s.thumbs)}
		h.Role = ggui.RoleSlider
		s.thumbs = append(s.thumbs, h)
	}
	s.thumbs = s.thumbs[:n]
	for i, h := range s.thumbs {
		h.SetInert(s.IsInert())
		if s.IsInert() {
			h.Pressed = false
			s.Pressed = false
			s.active = -1
		}
		name := s.SemanticName()
		if name == "" {
			name = "Value"
		}
		name = fmt.Sprintf("%s %d", name, i+1)
		if i < len(s.names) && s.names[i] != "" {
			name = s.names[i]
		}
		h.SetName(name)
	}
	if s.active >= n {
		s.active = -1
		s.Pressed = false
	}
	if s.vertical {
		return c.Constrain(ggui.Sz(s.theme.ControlSize+4, bounded(c.MaxH, defaultStripe)))
	}
	return c.Constrain(ggui.Sz(bounded(c.MaxW, defaultStripe), s.theme.ControlSize+4))
}

func (s *RangeSliderWidget) geometry() sliderGeometry {
	return newSliderGeometry(s.rect, s.theme.ControlSize*.375, s.vertical, s.rtl)
}
func (s *RangeSliderWidget) fraction(v float64) float64 {
	if s.max == s.min {
		return 0
	}
	return (v - s.min) / (s.max - s.min)
}
func (s *RangeSliderWidget) setFromPosition(i int, pos ggui.Point) {
	if f, ok := s.geometry().fraction(pos); ok {
		s.set(i, s.min+f*(s.max-s.min))
	}
}

func (s *RangeSliderWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleGroup, Name: s.SemanticName(), Disabled: s.IsInert()}
}

func (s *RangeSliderWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	s.rect = r
	dst.DescribeNode(r, s, func(dst *ggui.Canvas) {
		if !s.IsInert() {
			dst.HitPointer(r, s)
			dst.HitCursor(r, ggui.CursorShapePointer)
		}
		t, vs := s.theme, s.values()
		g := s.geometry()
		inert := s.IsInert()
		dst.FillRoundRect(g.track(0, 1), g.thick/2, sliderTone(t, t.Muted, inert))
		if len(vs) > 0 {
			start := 0.0
			if len(vs) > 1 {
				start = s.fraction(vs[0])
			}
			dst.FillRoundRect(g.track(start, s.fraction(vs[len(vs)-1])), g.thick/2, sliderTone(t, t.Primary, inert))
		}
		for i, h := range s.thumbs {
			group := s.HitID()
			if group == nil {
				group = r
			}
			h.SetKey(struct {
				Group any
				Index int
			}{group, i})
			if i >= len(vs) {
				break
			}
			f := s.fraction(vs[i])
			h.Hit(dst, g.thumb(f), h, ggui.CursorShapePointer)
			active := !inert && (h.Hovered || h.Pressed || (h.Focused && h.FocusVisible))
			g.paintThumb(dst, t, f, inert, dst.Ease(h.Anchor(g.thumb(f)), sliderHaloSlot, pick(active, 1.0, 0.0), s.env.Motion(t.MotionFast)))
		}
		if s.focusThumb && s.active >= 0 && s.active < len(s.thumbs) {
			dst.RequestFocus(s.thumbs[s.active])
			s.focusThumb = false
		}
	})
}

// CaptureTouchDrag keeps a thumb or track drag from becoming a scroll gesture.
func (s *RangeSliderWidget) CaptureTouchDrag() bool { return !s.IsInert() }

func (s *RangeSliderWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if s.IsInert() {
		s.Pressed = false
		return false
	}
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		vs := s.values()
		if len(vs) == 0 {
			return false
		}
		best := 0
		f, _ := s.geometry().fraction(ev.Pos)
		for i := 1; i < len(vs); i++ {
			if math.Abs(s.fraction(vs[i])-f) < math.Abs(s.fraction(vs[best])-f) {
				best = i
			}
		}
		s.active, s.focusThumb = best, true
		s.setFromPosition(best, ev.Pos)
	case ggui.PointerDrag:
		if s.Pressed {
			s.setFromPosition(s.active, ev.Pos)
		}
	case ggui.PointerUp:
		if s.Pressed {
			s.setFromPosition(s.active, ev.Pos)
			s.commit()
		}
	}
	return s.Pointer(ev, nil)
}

func (s *RangeSliderWidget) Adopt(prev any) {
	s.Interactive.Adopt(prev)
	if p, ok := prev.(*RangeSliderWidget); ok {
		s.active = p.active
		s.focusThumb = p.focusThumb
	}
}

type rangeSliderThumb struct {
	ggui.Interactive
	owner *RangeSliderWidget
	index int
}

func (h *rangeSliderThumb) CaptureTouchDrag() bool { return !h.owner.IsInert() }
func (h *rangeSliderThumb) Describe() ggui.Node {
	s, vs := h.owner, h.owner.values()
	if h.index >= len(vs) {
		return ggui.Node{Role: ggui.RoleSlider, Name: h.SemanticName(), Disabled: true}
	}
	lo, hi := s.limits(vs, h.index)
	return ggui.Node{Role: ggui.RoleSlider, Name: h.SemanticName(), Min: lo, Max: hi, Now: vs[h.index], Disabled: s.IsInert(), Actions: ggui.ActionFocus | ggui.ActionIncrement | ggui.ActionDecrement | ggui.ActionSetValue}
}
func (h *rangeSliderThumb) ConsumesKey(ev ggui.KeyEvent) bool {
	return sliderConsumesKey(ev)
}
func (h *rangeSliderThumb) HandleKey(ev ggui.KeyEvent) {
	h.Keyboard(ev, nil)
	s, vs := h.owner, h.owner.values()
	if s.IsInert() || ev.Kind != ggui.KeyPress || h.index >= len(vs) {
		return
	}
	step := s.keyStep()
	if ev.Mods.Shift || ev.Key == ggui.KeyPageUp || ev.Key == ggui.KeyPageDown {
		step = s.largeStep
	}
	v := vs[h.index]
	lo, hi := s.limits(vs, h.index)
	ev = directionalKey(ev, s.rtl && !s.vertical)
	switch ev.Key {
	case ggui.KeyHome:
		v = lo
	case ggui.KeyEnd:
		v = hi
	case ggui.KeyArrowLeft, ggui.KeyArrowDown, ggui.KeyPageDown:
		v -= step
	case ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyPageUp:
		v += step
	default:
		return
	}
	s.set(h.index, v)
	s.commit()
}
func (h *rangeSliderThumb) Act(a ggui.Action) bool {
	s, vs := h.owner, h.owner.values()
	if s.IsInert() || h.index >= len(vs) {
		return false
	}
	v := vs[h.index]
	switch a.Kind {
	case ggui.ActionIncrement:
		v += s.keyStep()
	case ggui.ActionDecrement:
		v -= s.keyStep()
	case ggui.ActionSetValue:
		v = a.Num
	default:
		return false
	}
	s.set(h.index, v)
	s.commit()
	return true
}
func (h *rangeSliderThumb) HandlePointer(ev ggui.PointerEvent) bool {
	s := h.owner
	if s.IsInert() {
		h.Pressed = false
		return false
	}
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		s.active = h.index
		s.setFromPosition(h.index, ev.Pos)
	case ggui.PointerDrag:
		if h.Pressed {
			s.setFromPosition(h.index, ev.Pos)
		}
	case ggui.PointerUp:
		if h.Pressed {
			s.setFromPosition(h.index, ev.Pos)
			s.commit()
		}
	}
	return h.Pointer(ev, nil)
}
