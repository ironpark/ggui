package ui

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// SpinnerWidget is an indeterminate loading indicator; reduced motion freezes it.
type SpinnerWidget struct {
	props   property.Owner
	size    float64     // negative is the theme's IconSize
	color   color.Color // nil is the text color around it
	ink     color.Color // what it is painted in, from the last Layout
	env     ggui.Env
	reduced bool
}

// Spinner creates an indicator the size of the theme's icons, in the color
// of the text around it. Add adjacent text to describe the operation.
func Spinner() *SpinnerWidget { return &SpinnerWidget{size: -1} }

// Size sets the requested diameter in logical pixels.
func (s *SpinnerWidget) Size(px float64) *SpinnerWidget {
	defer property.Watch(&s.props, &s.size)()
	s.size = max(0, px)
	return s
}

// Color paints the spinner in c instead of the text color around it; nil
// goes back to that.
func (s *SpinnerWidget) Color(c color.Color) *SpinnerWidget {
	defer property.Watch(&s.props, &s.color)()
	s.color = c
	return s
}

// Layout implements ggui.Widget.
func (s *SpinnerWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	t := uitheme.Of(env)
	s.env, s.reduced = env, env.ReducedMotion()
	s.ink = ggui.ResolveColor(colorOr(s.color, colorOr(env.Text().Color, t.Fg)), env)
	side := pick(s.size >= 0, s.size, t.IconSize)
	return c.Constrain(ggui.Sz(side, side))
}

// loopStep is how often a look that loops with no end steps, for
// ggui.Paced: 30 times a second, smooth for a spinner's turn a second, a
// quarter of the frames the display would ask for. Every loop here steps
// on it, so loops showing together share their frames: a spinner at 30 a
// second beside a pulse at 20 would wake the window 50 times a second,
// their steps falling between each other's.
const loopStep = time.Second / 30

// pulse is the factor of a look that breathes every two seconds: 1 at its
// fullest, lo at its faintest, stepped on loopStep.
func pulse(lo float64) float64 {
	mid := (1 + lo) / 2
	return mid + (1-mid)*math.Cos(float64(ggui.Paced(loopStep).UnixMilli()%2000)*2*math.Pi/2000)
}

// spinPhase is how far a spinner has turned, in radians: once a second,
// stepped on loopStep.
func spinPhase() float64 {
	return float64(ggui.Paced(loopStep).UnixMilli()%1000) * 2 * math.Pi / 1000
}

// Paint implements ggui.Widget.
func (s *SpinnerWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if min(r.Size.W, r.Size.H) <= 0 {
		return
	}
	phase := 0.0
	if !s.reduced {
		phase = spinPhase()
	}
	paintIcon(dst, s.env, icons.Loader, r, s.ink, phase)
}
