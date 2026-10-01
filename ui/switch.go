package ui

import (
	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
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
	s.Role = ggui.RoleSwitch
	s.SetName(label)
	s.AutoKey()
	if label != "" {
		s.label = ggui.Text(label)
	}
	s.onTap = func() { setChanged(on, !ggui.Untrack(on.Get), s.onChange) }
	return s
}

var knobSlot = ggui.NewSlot[*ggui.Motion]("knobSlot")

// OnChange fires with the new value after a click flipped it.
func (s *SwitchWidget) OnChange(fn func(bool)) *SwitchWidget { s.onChange = fn; return s }

// Disabled greys the switch out and ignores the pointer while v is true.
func (s *SwitchWidget) Disabled(v bool) *SwitchWidget { s.SetInert(v); return s }

// BindDisabled follows r for Disabled without a rebuild.
func (s *SwitchWidget) BindDisabled(r ggui.Readable[bool]) *SwitchWidget { s.BindInert(r); return s }

// Describe implements ggui.Describer: a switch reports whether it is on.
func (s *SwitchWidget) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleSwitch,
		Name:     s.SemanticName(),
		Checked:  ggui.Tri(ggui.Untrack(s.on.Get)),
		Disabled: s.IsInert(),
		Actions:  ggui.ActionPress | ggui.ActionFocus,
	}
}

// Layout implements Widget.
func (s *SwitchWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	// The track is a control glyph tall, plus the thumb's inset.
	h := uitheme.From(env).ControlSize + 4
	return s.layout(c, env, ggui.Sz(h*1.8, h))
}

// Paint implements Widget.
func (s *SwitchWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	box := s.paint(dst, r, s)
	on := ggui.Untrack(s.on.Get)
	k := dst.Ease(s.Anchor(box), knobSlot, pick(on, 1.0, 0.0), s.motion)
	// As shadcn's: the page's background on a light page, and on a dark one
	// the foreground, or PrimaryFg once on.
	dark := isDark(t.Bg)
	track := mix(t.InputBorder, t.Primary, k)
	thumb := mix(pick(dark, t.Fg, t.Bg), pick(dark, t.PrimaryFg, t.Bg), k)
	if s.IsInert() {
		track, thumb = t.Disabled(track), t.Disabled(thumb)
	}
	dst.FillRoundRect(box, box.Size.H/2, track)
	radius := box.Size.H/2 - 2
	x := box.Origin.X + 2 + radius + k*(box.Size.W-4-2*radius)
	dst.FillCircle(ggui.Pt(x, box.Origin.Y+box.Size.H/2), radius, thumb)
	s.FocusRing(dst, box, box.Size.H/2, t.Ring)
}
