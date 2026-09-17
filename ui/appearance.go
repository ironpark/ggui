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

// OverlayShadow customizes the highest elevation: dialogs and toasts.
var OverlayShadow = ggui.NewKey[ggui.ShadowStyle]("ui overlay shadow")

// themeOr reads a theme token, falling back to a derived default so custom
// themes work without having to set every key.
func themeOr[T any](t ggui.Theme, k ggui.Key[T], fallback T) T {
	if v, ok := t.Get(k); ok {
		return v
	}
	return fallback
}

func mutedSurface(t ggui.Theme) color.Color {
	return themeOr(t, SurfaceMuted, mix(t.Surface, t.Fg, .055))
}

func focusColor(t ggui.Theme) color.Color {
	return themeOr(t, FocusColor, mix(t.Surface, t.Accent, .4))
}

func panelShadow(t ggui.Theme) ggui.ShadowStyle {
	return themeOr(t, PanelShadow, ggui.ShadowStyle{Offset: ggui.Pt(0, 4), Blur: 10, Color: color.NRGBA{A: 26}})
}

func cardShadow(t ggui.Theme) ggui.ShadowStyle {
	return themeOr(t, CardShadow, ggui.ShadowStyle{Offset: ggui.Pt(0, 1), Blur: 2, Color: color.NRGBA{A: 15}})
}

func overlayShadow(t ggui.Theme) ggui.ShadowStyle {
	return themeOr(t, OverlayShadow, ggui.ShadowStyle{Offset: ggui.Pt(0, 12), Blur: 28, Color: color.NRGBA{A: 65}})
}

// panelBox applies the chrome every floating surface shares: menus, context
// menus, the menubar, select lists and the date picker panel.
func panelBox(b *ggui.BoxWidget, t ggui.Theme) *ggui.BoxWidget {
	return b.Shadow(panelShadow(t)).Fill(t.Surface).Border(1, t.Border).Radius(t.Radius).Padding(t.PanelPad)
}

// chevron draws the two-segment glyph a select and an accordion header share.
// dy points the tip down when positive and up when negative.
func chevron(dst *ggui.Canvas, center ggui.Point, dy float64, col color.Color) {
	dst.StrokeLine(ggui.Pt(center.X-4, center.Y-dy), ggui.Pt(center.X, center.Y+dy), 1.5, col)
	dst.StrokeLine(ggui.Pt(center.X, center.Y+dy), ggui.Pt(center.X+4, center.Y-dy), 1.5, col)
}

func fieldHalo(dst *ggui.Canvas, r ggui.Rect, radius float64, t ggui.Theme) {
	dst.StrokeRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4)), radius+2, 3, focusColor(t))
}
