package ui

import (
	"math"

	"github.com/ironpark/ggui"
)

// SpinnerWidget is an indeterminate loading indicator; reduced motion freezes it.
type SpinnerWidget struct {
	size    float64
	theme   ggui.Theme
	reduced bool
}

// Spinner creates a 20-pixel indicator. Add adjacent text to describe the operation.
func Spinner() *SpinnerWidget { return &SpinnerWidget{size: 20} }

// Size sets the requested diameter in logical pixels.
func (s *SpinnerWidget) Size(px float64) *SpinnerWidget { s.size = max(0, px); return s }

// Layout implements ggui.Widget.
func (s *SpinnerWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s.theme, s.reduced = env.Theme(), env.ReducedMotion()
	return c.Constrain(ggui.Sz(s.size, s.size))
}

// Paint implements ggui.Widget.
func (s *SpinnerWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	diameter := min(r.Size.W, r.Size.H)
	if diameter <= 0 {
		return
	}
	phase := 0.0
	if !s.reduced {
		phase = float64(ggui.Now().UnixMilli()%1000) * 2 * math.Pi / 1000
	}
	cx, cy := r.Origin.X+r.Size.W/2, r.Origin.Y+r.Size.H/2
	radius, stroke := diameter*.4, diameter*.1
	for i := range 24 {
		a := phase + float64(i)*math.Pi*1.5/24
		b := phase + float64(i+1)*math.Pi*1.5/24
		dst.StrokeLine(ggui.Pt(cx+radius*math.Cos(a), cy+radius*math.Sin(a)), ggui.Pt(cx+radius*math.Cos(b), cy+radius*math.Sin(b)), stroke, s.theme.Primary)
	}
}
