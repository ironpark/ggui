package ui

import (
	"math"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// SkeletonWidget reserves space while content loads. It pulses unless motion is reduced.
type SkeletonWidget struct {
	props           property.Owner
	width, height   float64
	theme           uitheme.Theme
	reduced, circle bool
}

// Skeleton creates a placeholder with a requested size in logical pixels.
func Skeleton(width, height float64) *SkeletonWidget {
	return &SkeletonWidget{width: max(0, width), height: max(0, height)}
}

// Circle rounds the placeholder into a pill, or a circle when square.
func (s *SkeletonWidget) Circle() *SkeletonWidget {
	defer property.Watch(&s.props, &s.circle)()
	s.circle = true
	return s
}

// Layout implements ggui.Widget.
func (s *SkeletonWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout()()
	s.theme, s.reduced = uitheme.From(env), env.ReducedMotion()
	return c.Constrain(ggui.Sz(s.width, s.height))
}

// Paint implements ggui.Widget.
func (s *SkeletonWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	radius := s.theme.Radius
	if s.circle {
		radius = min(r.Size.W, r.Size.H) / 2
	}
	factor := 1.0
	if !s.reduced {
		factor = .75 + .25*math.Cos(float64(ggui.Now().UnixMilli()%2000)*2*math.Pi/2000)
	}
	dst.FillRoundRect(r, radius, fade(s.theme.Muted, factor))
}
