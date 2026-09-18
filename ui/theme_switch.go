package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
)

// ThemeSwitchWidget is a day/night switch bound to whether dark mode is enabled.
// Like Switch, it updates its binding; the application applies the chosen theme.
type ThemeSwitchWidget struct {
	*SwitchWidget
}

// ThemeSwitch shows an animated sun and moon. True means dark mode.
// Its default accessible name is "Dark mode"; no text label is drawn.
func ThemeSwitch(dark ggui.Binding[bool]) *ThemeSwitchWidget {
	s := &ThemeSwitchWidget{SwitchWidget: Switch(dark, "")}
	s.Name = "Dark mode"
	return s
}

// Named sets the accessible name without adding a visible label.
func (s *ThemeSwitchWidget) Named(name string) *ThemeSwitchWidget { s.Name = name; return s }

// OnChange fires after the user changes the mode.
func (s *ThemeSwitchWidget) OnChange(fn func(bool)) *ThemeSwitchWidget {
	s.SwitchWidget.OnChange(fn)
	return s
}

// Disabled prevents pointer and keyboard changes.
func (s *ThemeSwitchWidget) Disabled(v bool) *ThemeSwitchWidget {
	s.SetInert(v)
	return s
}

// DisabledWhen follows a disabled-state binding without rebuilding.
func (s *ThemeSwitchWidget) DisabledWhen(r ggui.Reader[bool]) *ThemeSwitchWidget {
	s.InertWhen(r)
	return s
}

// Layout implements ggui.Widget.
func (s *ThemeSwitchWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	size := s.layout(c, env, ggui.Sz(64, 36))
	s.motion = env.Motion(s.theme.MotionSlow)
	return size
}

var themeSwitchSlot = ggui.NewSlot[*ggui.Motion]("themeSwitch")

// Paint implements ggui.Widget.
func (s *ThemeSwitchWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	box := s.paint(dst, r, s)
	k := dst.Ease(s.Anchor(box), themeSwitchSlot, pick(s.on.Peek(), 1.0, 0.0), s.motion)
	// The celestial body is the thumb itself, with a softly shaded rim.
	track := mix(color.NRGBA{R: 103, G: 187, B: 232, A: 255}, color.NRGBA{R: 33, G: 24, B: 79, A: 255}, k)
	body := mix(color.NRGBA{R: 255, G: 220, B: 111, A: 255}, color.NRGBA{R: 255, G: 251, B: 252, A: 255}, k)
	rim := mix(color.NRGBA{R: 247, G: 182, B: 76, A: 255}, color.NRGBA{R: 239, G: 218, B: 237, A: 255}, k)
	if s.Inert {
		track = mix(track, s.theme.Muted, .65)
		body = mix(body, track, .4)
		rim = mix(rim, track, .4)
	}
	// Layered low-opacity shadows lift the pill without a hard outline.
	for i := 3; i >= 1; i-- {
		d := float64(i)
		shadow := ggui.Rct(box.Origin.Add(ggui.Pt(-d/2, d)), ggui.Sz(box.Size.W+d, box.Size.H))
		dst.FillRoundRect(shadow, 18, fade(color.Black, pick(s.Inert, .01, .025)))
	}
	dst.FillRoundRect(box, 18, track)
	at := func(x, y float64) ggui.Point { return box.Origin.Add(ggui.Pt(x, y)) }
	// At night the moon rests on the left, revealing the stars to its right.
	stars := fade(color.White, k*pick(s.Inert, .3, .9))
	for _, star := range []struct{ x, y, radius float64 }{
		{37, 8, .6}, {35, 18, .4}, {48, 25, .6}, {53, 17, .35}, {59, 20, .5},
	} {
		dst.FillCircle(at(star.x, star.y+2*(1-k)), star.radius, stars)
	}
	cloud := fade(color.White, (1-k)*pick(s.Inert, .3, .8))
	for _, c := range []struct{ x, y, radius float64 }{{8, 23, 3}, {13, 21, 4.5}, {18, 24, 3.5}} {
		dst.FillCircle(at(c.x, c.y+3*k), c.radius, cloud)
	}
	center := at(46-28*k, 18)
	// A restrained halo and warm rim soften the large, full-disc thumb.
	dst.FillCircle(center, 16, fade(body, pick(s.Inert, .04, .1)))
	dst.FillCircle(center.Add(ggui.Pt(0, 1)), 14, fade(color.Black, .08))
	dst.FillCircle(center, 14, rim)
	dst.FillCircle(center.Add(ggui.Pt(1, -1)), 13, body)
	crater := fade(mix(body, color.NRGBA{R: 223, G: 211, B: 222, A: 255}, .4), k*pick(s.Inert, .3, .65))
	for _, c := range []struct{ x, y, radius float64 }{{-3, 4, 2}, {5, 2, 3.2}, {1, -4, 1.2}, {5, -10, 2.5}} {
		dst.FillCircle(center.Add(ggui.Pt(c.x, c.y)), c.radius, crater)
	}
	if s.Hovered && !s.Inert {
		dst.StrokeRoundRect(box, 18, 1, fade(body, .25))
	}
	s.FocusRing(dst, box, 18, s.theme.Ring)
}
