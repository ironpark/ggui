package ui

import (
	"math"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
	"github.com/ironpark/ggui/internal/property"
)

// SpinnerWidget is an indeterminate loading indicator; reduced motion freezes it.
type SpinnerWidget struct {
	props   property.Owner
	size    float64
	theme   ggui.Theme
	env     ggui.Env
	reduced bool
}

// Spinner creates a 20-pixel indicator. Add adjacent text to describe the operation.
func Spinner() *SpinnerWidget { return &SpinnerWidget{size: 20} }

// Size sets the requested diameter in logical pixels.
func (s *SpinnerWidget) Size(px float64) *SpinnerWidget {
	defer property.Watch(&s.props, &s.size)()
	s.size = max(0, px)
	return s
}

// Layout implements ggui.Widget.
func (s *SpinnerWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout()()
	s.env = env
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
	paintIcon(dst, s.env, icons.Loader, r, s.theme.Primary, phase)
}
