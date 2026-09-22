package main

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// disclosure shows its body only while open. The flag is an ordinary bool,
// not a signal, so nothing rebuilds when it flips; instead Invalidate tells
// the runtime the widget's size changed and the cache above it measures
// again. Scroll keeps its offset the same way.
type disclosure struct {
	open   bool
	env    ggui.Env
	header ggui.Widget
	body   ggui.Widget
	column *ggui.ColumnWidget
}

func newDisclosure(title string, body ggui.Widget) *disclosure {
	d := &disclosure{body: body}
	d.header = ggui.Tap(ggui.Row(ggui.Text(title), ggui.Spacer(), ui.Caption("tap to toggle")).Space(1), d.toggle)
	return d
}

func (d *disclosure) toggle() {
	d.open = !d.open
	ggui.Invalidate(d.env)
}

func (d *disclosure) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	d.env = env
	if d.open {
		d.column = ggui.Column(d.header, d.body).Space(1).Align(ggui.AlignStretch)
	} else {
		d.column = ggui.Column(d.header).Align(ggui.AlignStretch)
	}
	return d.column.Layout(c, env)
}

// Paint goes through dst.Paint so the inspector sees the children.
func (d *disclosure) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(d.column, r) }
