package ui

import (
	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// DividerWidget is a line of the theme's BorderWidth in its Border color.
// Build one with Divider.
type DividerWidget struct {
	vertical bool
	margin   float64 // room on either side of the line, in the theme's Space
	theme    *uitheme.Theme
}

// Divider creates a horizontal rule that fills the width it is given.
func Divider() *DividerWidget { return &DividerWidget{} }

// Vertical turns the rule to fill the height it is given instead.
func (d *DividerWidget) Vertical() *DividerWidget { d.vertical = true; return d }

// Layout implements Widget.
func (d *DividerWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	d.theme = uitheme.Of(env)
	across := d.theme.BorderWidth + 2*d.margin*d.theme.Space
	if d.vertical {
		return c.Constrain(ggui.Sz(across, bounded(c.MaxH, 0)))
	}
	return c.Constrain(ggui.Sz(bounded(c.MaxW, 0), across))
}

// Paint implements Widget.
func (d *DividerWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.FillRect(d.line(r), d.theme.Border) }

// line is the rule within r: all of it, or its middle when a margin keeps
// room on either side.
func (d *DividerWidget) line(r ggui.Rect) ggui.Rect {
	w := d.theme.BorderWidth
	if d.vertical {
		w = min(w, r.Size.W)
		return ggui.Rct(ggui.Pt(r.Origin.X+(r.Size.W-w)/2, r.Origin.Y), ggui.Sz(w, r.Size.H))
	}
	w = min(w, r.Size.H)
	return ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(r.Size.H-w)/2), ggui.Sz(r.Size.W, w))
}
