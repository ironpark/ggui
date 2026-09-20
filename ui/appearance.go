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
func fieldHalo(dst *ggui.Canvas, r ggui.Rect, radius float64, ring color.Color) {
	dst.StrokeRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4)), radius+2, 3, fade(ring, .5))
}

// fieldRing is the focus ring for a field: the theme's, or the destructive
// one while the field is invalid.
func fieldRing(t ggui.Theme, invalid bool) color.Color {
	if invalid {
		return destructiveRing(t)
	}
	return t.Ring
}

// destructiveRing is the focus ring of anything that warns: destructive
// buttons and invalid fields.
func destructiveRing(t ggui.Theme) color.Color { return fade(t.Destructive, .4) }

// isDark reports whether c is closer to black than to white.
func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r+g+b < 3*32768
}

// paintDisclosure paints content folding open below y: the body is clipped
// to progress of its height, inert while closing, and asks for another
// layout until the motion settles.
func paintDisclosure(dst *ggui.Canvas, env ggui.Env, content ggui.Widget, at ggui.Point, width float64, size ggui.Size, progress float64, open bool) {
	if progress != pick(open, 1.0, 0.0) {
		ggui.Invalidate(env)
	}
	if progress <= 0 {
		return
	}
	body := dst.Clip(ggui.Rct(at, ggui.Sz(width, size.H*progress)))
	if !open {
		body = body.Inert()
	}
	body.Paint(content, ggui.Rct(at, size))
}

func colorOr(value, fallback color.Color) color.Color {
	if value != nil {
		return value
	}
	return fallback
}
