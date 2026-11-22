package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/icons/lucide"
)

// Icon is a shared semantic placeholder. Local Env and theme icon sets override
// Lucide defaults per role. Use Color, Size, and Alt to customize the widget.
func Icon(role icons.Role) *icons.Widget {
	return icons.Placeholder(role).Fallback(lucide.Set())
}

// PaintIcon resolves role under env (falling back to Lucide) and draws it into
// rect tinted with col and rotated clockwise by rotation radians.
func PaintIcon(dst *ggui.Canvas, env ggui.Env, role icons.Role, rect ggui.Rect, col color.Color, rotation float64) {
	icons.Resolve(env, lucide.Set(), role).Draw(dst, rect, col, rotation)
}

func paintIcon(dst *ggui.Canvas, env ggui.Env, role icons.Role, rect ggui.Rect, col color.Color, rotation float64) {
	PaintIcon(dst, env, role, rect, col, rotation)
}

// paintIconAt paints a size-square icon centered on center.
func paintIconAt(dst *ggui.Canvas, env ggui.Env, role icons.Role, center ggui.Point, size float64, col color.Color, rotation float64) {
	paintIcon(dst, env, role, ggui.Rct(center.Add(ggui.Pt(-size/2, -size/2)), ggui.Sz(size, size)), col, rotation)
}
