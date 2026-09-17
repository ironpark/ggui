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

// fieldBox applies the chrome every input shares: the Input surface, the
// theme's padding and radius, and a border that becomes the focus ring
// while the editor inside has the keyboard. A disabled field drops back to
// the Card surface, so it reads as a surface rather than something to type
// in.
func fieldBox(b *ggui.BoxWidget, t ggui.Theme, focused, inert bool) *ggui.BoxWidget {
	return b.Padding(t.FieldPad).Radius(t.Radius).
		Fill(pick(inert, t.Card, t.Input)).
		Border(t.BorderWidth, pick(focused, t.Ring, t.Border))
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
