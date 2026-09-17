package ui

import (
	"github.com/ironpark/ggui"
)

// DividerWidget is a one-pixel line in the theme's Border color. Build one
// with Divider.
type DividerWidget struct {
	vertical bool
	theme    ggui.Theme
}

// Divider creates a horizontal rule that fills the width it is given.
func Divider() *DividerWidget { return &DividerWidget{} }

// Vertical turns the rule to fill the height it is given instead.
func (d *DividerWidget) Vertical() *DividerWidget { d.vertical = true; return d }

// Layout implements Widget.
func (d *DividerWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	d.theme = env.Theme()
	if d.vertical {
		return c.Constrain(ggui.Sz(1, bounded(c.MaxH, 0)))
	}
	return c.Constrain(ggui.Sz(bounded(c.MaxW, 0), 1))
}

// Paint implements Widget.
func (d *DividerWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.FillRect(r, d.theme.Border) }
