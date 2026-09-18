package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
	"github.com/ironpark/ggui/icons/lucide"
	"image/color"
)

// Icon is a shared semantic placeholder. Local Env and theme icon sets override
// Lucide defaults per role. Use Color, Size, and Alt to customize the widget.
func Icon(role icons.Role) *icons.Widget {
	return icons.Placeholder(role).Fallback(lucide.Set())
}

func paintIcon(dst *ggui.Canvas, env ggui.Env, role icons.Role, rect ggui.Rect, col color.Color, rotation float64) {
	icons.Resolve(env, lucide.Set(), role).Draw(dst, rect, col, rotation)
}
