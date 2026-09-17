package ui

import (
	"github.com/ironpark/ggui"
)

// SwitchWidget is a sliding on/off toggle. Build one with Switch.
type SwitchWidget struct {
	toggle
	on       *ggui.Signal[bool]
	onChange func(bool)
}

// Switch binds a toggle to on; a click flips it and the knob slides over.
func Switch(on *ggui.Signal[bool], label string) *SwitchWidget {
	s := &SwitchWidget{on: on}
	s.glyph = ggui.Sz(switchWidth, switchHeight)
	if label != "" {
		s.label = ggui.Text(label)
	}
	s.onTap = func() { setChanged(on, !on.Peek(), s.onChange) }
	return s
}

var knobSlot = new(byte)

// OnChange fires with the new value after a click flipped it.
func (s *SwitchWidget) OnChange(fn func(bool)) *SwitchWidget { s.onChange = fn; return s }

// Disabled greys the switch out and ignores the pointer while v is true.
func (s *SwitchWidget) Disabled(v bool) *SwitchWidget { s.disabled = v; return s }

// Layout implements Widget.
func (s *SwitchWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size { return s.layout(c, env) }

// Paint implements Widget.
func (s *SwitchWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	box := s.paint(dst, r, s)
	on := s.on.Peek()
	k := motion(dst, box, knobSlot, pick(on, 1.0, 0.0))
	track := pick(on, pick(s.hovered, t.AccentHover, t.Accent), pick(s.hovered, t.Muted, t.Border))
	if s.disabled {
		track = t.Border
	}
	dst.FillRoundRect(box, box.Size.H/2, track)
	radius := box.Size.H/2 - 2
	x := box.Origin.X + 2 + radius + k*(box.Size.W-4-2*radius)
	dst.FillCircle(ggui.Pt(x, box.Origin.Y+box.Size.H/2), radius, pick(s.disabled, t.Surface, t.Field))
	s.focus.paintRing(dst, box, box.Size.H/2, t)
}
