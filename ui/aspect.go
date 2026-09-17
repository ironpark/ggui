package ui

import (
	"math"

	"github.com/ironpark/ggui"
)

// AspectRatioWidget gives its child a fixed width-to-height ratio within the
// space it is offered. Build one with AspectRatio.
type AspectRatioWidget struct {
	child ggui.Widget
	ratio float64
}

// AspectRatio sizes child to ratio, its width divided by its height: 16.0/9
// for video, 1 for a square. It takes the width it is given and derives the
// height, unless that would not fit, in which case the height leads.
//
//	ui.AspectRatio(16.0/9, ggui.Image(cover).Fit(ggui.FitCover))
func AspectRatio(ratio float64, child ggui.Widget) *AspectRatioWidget {
	return &AspectRatioWidget{child: child, ratio: ratio}
}

// Layout implements ggui.Widget.
func (a *AspectRatioWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	ratio := a.ratio
	if !(ratio > 0) || math.IsInf(ratio, 0) {
		ratio = 1
	}
	w, h := c.MaxW, c.MaxH
	switch {
	case math.IsInf(w, 1) && math.IsInf(h, 1):
		// Nothing bounds it, so there is no length to divide.
		w, h = 0, 0
	case math.IsInf(w, 1):
		w = h * ratio
	default:
		h = w / ratio
		if h > c.MaxH {
			h, w = c.MaxH, c.MaxH*ratio
		}
	}
	size := c.Constrain(ggui.Sz(w, h))
	if a.child != nil {
		a.child.Layout(ggui.Tight(size), env)
	}
	return size
}

// Paint implements ggui.Widget.
func (a *AspectRatioWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if a.child != nil {
		dst.Paint(a.child, r)
	}
}
