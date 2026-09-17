package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
)

// panelBox applies the chrome every floating surface shares: menus, context
// menus, the menubar, select lists and the date picker panel.
func panelBox(b *ggui.BoxWidget, t ggui.Theme) *ggui.BoxWidget {
	return b.Shadow(t.PanelShadow).Fill(t.Popover).Border(t.BorderWidth, t.Border).Radius(t.Radius).Padding(t.PanelPad)
}

// chevron draws the two-segment glyph a select and an accordion header share.
// dy points the tip down when positive and up when negative.
func chevron(dst *ggui.Canvas, center ggui.Point, dy float64, col color.Color) {
	dst.StrokeLine(ggui.Pt(center.X-4, center.Y-dy), ggui.Pt(center.X, center.Y+dy), 1.5, col)
	dst.StrokeLine(ggui.Pt(center.X, center.Y+dy), ggui.Pt(center.X+4, center.Y-dy), 1.5, col)
}

// fieldHalo draws the focus ring just outside a text field.
func fieldHalo(dst *ggui.Canvas, r ggui.Rect, radius float64, t ggui.Theme) {
	dst.StrokeRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4)), radius+2, 3, t.Ring)
}
