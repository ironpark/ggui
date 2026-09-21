package main

import "github.com/ironpark/ggui"

// adaptive switches arrangements during measurement, preserving the controls
// and their state. It never creates widgets or subscriptions inside Layout.
type adaptive struct {
	breakpoint   float64
	wide, narrow ggui.Widget
	current      ggui.Widget
}

func (a *adaptive) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	a.current = a.wide
	if c.MaxW < a.breakpoint {
		a.current = a.narrow
	}
	return a.current.Layout(c, env)
}
func (a *adaptive) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	dst.Paint(a.current, rect)
}
