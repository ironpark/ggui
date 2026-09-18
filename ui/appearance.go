package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
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
		Border(t.BorderWidth, pick(focused, t.Ring, colorOr(t.InputBorder, t.Border)))
}

// chevron resolves the directional role shared by selects and accordions.
// dy points the tip down when positive and up when negative.
func chevron(dst *ggui.Canvas, env ggui.Env, center ggui.Point, dy float64, col color.Color) {
	paintIcon(dst, env, pick(dy < 0, icons.ChevronUp, icons.ChevronDown), ggui.Rct(center.Add(ggui.Pt(-8, -8)), ggui.Sz(16, 16)), col, 0)
}

// fieldHalo draws the focus ring just outside a text field.
func fieldHalo(dst *ggui.Canvas, r ggui.Rect, radius float64, t ggui.Theme) {
	dst.StrokeRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4)), radius+2, 3, t.Ring)
}

func colorOr(value, fallback color.Color) color.Color {
	if value != nil {
		return value
	}
	return fallback
}
