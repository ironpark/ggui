package ui

import (
	"time"

	"github.com/ironpark/ggui"
)

// SwitchWidget is a sliding on/off toggle. Build one with Switch.
type SwitchWidget struct {
	toggle
	on   *ggui.Signal[bool]
	knob ggui.Motion
}

// Switch binds a toggle to on; a click flips it and the knob slides over.
func Switch(on *ggui.Signal[bool], label string) *SwitchWidget {
	s := &SwitchWidget{on: on}
	s.glyph = ggui.Sz(switchWidth, switchHeight)
	if label != "" {
		s.label = ggui.Text(label)
	}
	s.onTap = func() { ggui.Toggle(on) }
	return s
}

// Disabled greys the switch out and ignores the pointer while v is true.
func (s *SwitchWidget) Disabled(v bool) *SwitchWidget { s.disabled = v; return s }

// Layout implements Widget.
func (s *SwitchWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size { return s.layout(c, env) }

// Paint implements Widget.
func (s *SwitchWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	box := s.paint(dst, r, s)
	now := time.Now()
	on := s.on.Peek()
	s.knob.MoveTo(pick(on, 1.0, 0.0), now, knobDuration)
	k := s.knob.Value(now)
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

// Adopt implements ggui.Adopter: the knob keeps sliding across a rebuild,
// which matters because flipping a switch often rebuilds what holds it.
func (s *SwitchWidget) Adopt(prev any) {
	s.toggle.Adopt(prev)
	if p, ok := prev.(*SwitchWidget); ok {
		s.knob = p.knob
	}
}

// HandlePointer implements PointerHandler.
func (s *SwitchWidget) HandlePointer(ev ggui.PointerEvent) bool { return s.handle(ev) }
