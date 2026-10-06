package ggui

// ResponsiveWidget lays out one of two arrangements by the width it is
// given. Build one with Responsive.
type ResponsiveWidget struct {
	breakpoint   float64
	wide, narrow Widget
	current      Widget
}

// Responsive lays out wide while it is given at least breakpoint of width,
// and narrow below that. The two arrangements may hold the same widgets, so
// a control keeps its state, focus included, as the window crosses the
// breakpoint:
//
//	title, actions := ui.Title("Tasks"), ui.Button("New", create)
//	ggui.Responsive(680,
//		ggui.Row(ggui.Expanded(title), actions),
//		ggui.Column(title, actions).Stretch())
func Responsive(breakpoint float64, wide, narrow Widget) *ResponsiveWidget {
	return &ResponsiveWidget{breakpoint: breakpoint, wide: wide, narrow: narrow, current: wide}
}

// Layout implements Widget.
func (r *ResponsiveWidget) Layout(c Constraints, env Env) Size {
	r.current = pick(c.MaxW < r.breakpoint, r.narrow, r.wide)
	return r.current.Layout(c, env)
}

// Baseline implements Baseliner: the shown arrangement's.
func (r *ResponsiveWidget) Baseline() (float64, bool) { return baselineOf(r.current) }

// Paint implements Widget.
func (r *ResponsiveWidget) Paint(dst *Canvas, rect Rect) { dst.Paint(r.current, rect) }
