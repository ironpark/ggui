package ui

import (
	"github.com/ironpark/ggui"
	"image/color"
)

// SwitchWidget is a sliding on/off toggle. Build one with Switch.
type SwitchWidget struct {
	toggle
	on       ggui.Binding[bool]
	onChange func(bool)
}

// Switch binds a toggle to on; a click flips it and the knob slides over.
func Switch(on ggui.Binding[bool], label string) *SwitchWidget {
	s := &SwitchWidget{on: on}
	s.Role, s.Name = ggui.RoleSwitch, label
	s.AutoKey()
	if label != "" {
		s.label = ggui.Text(label)
	}
	s.onTap = func() { setChanged(on, !on.Peek(), s.onChange) }
	return s
}

var knobSlot = ggui.NewSlot[*ggui.Motion]("knobSlot")

// OnChange fires with the new value after a click flipped it.
func (s *SwitchWidget) OnChange(fn func(bool)) *SwitchWidget { s.onChange = fn; return s }

// Disabled greys the switch out and ignores the pointer while v is true.
func (s *SwitchWidget) Disabled(v bool) *SwitchWidget { s.Inert = v; return s }

// DisabledWhen follows r for Disabled without a rebuild.
func (s *SwitchWidget) DisabledWhen(r ggui.Reader[bool]) *SwitchWidget { s.InertWhen(r); return s }

// Describe implements ggui.Describer: a switch reports whether it is on.
func (s *SwitchWidget) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleSwitch,
		Name:     s.Name,
		Checked:  ggui.Tri(s.on.Peek()),
		Disabled: s.Inert,
		Actions:  ggui.ActionPress | ggui.ActionFocus,
	}
}

// Layout implements Widget.
func (s *SwitchWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return s.layout(c, env, ggui.Sz(switchWidth, switchHeight))
}

// Paint implements Widget.
func (s *SwitchWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	box := s.paint(dst, r, s)
	on := s.on.Peek()
	k := dst.Ease(s.Anchor(box), knobSlot, pick(on, 1.0, 0.0), s.motion)
	track := pick(on, pick(s.Hovered, t.PrimaryHover, t.Primary), pick(s.Hovered, t.MutedFg, t.Border))
	if s.Inert {
		track = t.Border
	}
	dst.FillRoundRect(box, box.Size.H/2, track)
	radius := box.Size.H/2 - 2
	x := box.Origin.X + 2 + radius + k*(box.Size.W-4-2*radius)
	dst.FillCircle(ggui.Pt(x, box.Origin.Y+box.Size.H/2), radius, pick(s.Inert, t.MutedFg, pick(on, t.PrimaryFg, color.Color(color.White))))
	s.FocusRing(dst, box, box.Size.H/2, t.Ring)
}
