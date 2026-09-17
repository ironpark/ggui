package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
)

// SurfaceMuted customizes subdued surfaces such as tabs and muted buttons.
var SurfaceMuted = ggui.NewKey[color.Color]("ui muted surface")

// FocusColor customizes the focus halo, separate from the primary action color.
var FocusColor = ggui.NewKey[color.Color]("ui focus color")

// PanelShadow customizes floating menus, select lists and date picker panels.
var PanelShadow = ggui.NewKey[ggui.ShadowStyle]("ui panel shadow")

// CardShadow customizes the default subtle card elevation.
var CardShadow = ggui.NewKey[ggui.ShadowStyle]("ui card shadow")

func mutedSurface(t ggui.Theme) color.Color {
	if c, ok := t.Get(SurfaceMuted); ok {
		return c
	}
	return mix(t.Surface, t.Fg, .055)
}
func focusColor(t ggui.Theme) color.Color {
	if c, ok := t.Get(FocusColor); ok {
		return c
	}
	return mix(t.Surface, t.Accent, .4)
}
func panelShadow(t ggui.Theme) ggui.ShadowStyle {
	if s, ok := t.Get(PanelShadow); ok {
		return s
	}
	return ggui.ShadowStyle{Offset: ggui.Pt(0, 4), Blur: 10, Color: color.NRGBA{A: 26}}
}
func cardShadow(t ggui.Theme) ggui.ShadowStyle {
	if s, ok := t.Get(CardShadow); ok {
		return s
	}
	return ggui.ShadowStyle{Offset: ggui.Pt(0, 1), Blur: 2, Color: color.NRGBA{A: 15}}
}
func fieldHalo(dst *ggui.Canvas, r ggui.Rect, radius float64, t ggui.Theme) {
	dst.StrokeRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4)), radius+2, 3, focusColor(t))
}
