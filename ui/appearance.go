package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// panelBox applies the chrome every floating surface shares: menus, context
// menus, the menubar, select lists and the date picker panel.
func panelBox(b *ggui.BoxWidget, t *uitheme.Theme) *ggui.BoxWidget {
	return b.Shadow(t.PanelShadow).Fill(t.Popover).Border(t.BorderWidth, fade(t.Fg, .1)).Radius(t.Radius).Padding(t.PanelPad)
}

// fieldBox applies the chrome every input shares: the Input surface, the
// theme's padding and radius, and a border that becomes the focus ring
// while the editor inside has the keyboard. A disabled field drops back to
// the Card surface, so it reads as a surface rather than something to type
// in.
func fieldBox(b *ggui.BoxWidget, t *uitheme.Theme, focused, inert bool) *ggui.BoxWidget {
	return fieldBoxPadded(b, t, focused, inert, t.FieldPad)
}

// fieldBoxPadded is fieldBox with the caller's padding, set once: writing
// the theme's and then another would change the box twice a layout, and a
// box that changed after it was measured is never cached.
func fieldBoxPadded(b *ggui.BoxWidget, t *uitheme.Theme, focused, inert bool, pad ggui.EdgeInsets) *ggui.BoxWidget {
	fill, border := t.Input, pick(focused, t.Ring, t.InputBorder)
	if inert {
		fill = fade(t.InputBorder, pick(isDark(t.Bg), .8, .5))
		border = t.Disabled(border)
	}
	return b.Padding(pad).Radius(t.Radius).Fill(fill).Border(t.BorderWidth, border)
}

// chevron resolves the directional role shared by selects and accordions.
// dy points the tip down when positive and up when negative.
func chevron(dst *ggui.Canvas, env ggui.Env, t *uitheme.Theme, center ggui.Point, dy float64, col color.Color) {
	paintIconAt(dst, env, pick(dy < 0, icons.ChevronUp, icons.ChevronDown), center, t.IconSize, col, 0)
}

// fieldHalo draws the focus ring just outside a text field, at the theme's
// halo opacity.
func fieldHalo(dst *ggui.Canvas, t *uitheme.Theme, r ggui.Rect, radius float64, ring color.Color) {
	dst.StrokeRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4)), radius+2, 3, t.Halo(ring))
}

// fieldRing is the focus ring for a field: the theme's, or the destructive
// one while the field is invalid.
func fieldRing(t *uitheme.Theme, invalid bool) color.Color {
	if invalid {
		return destructiveRing(t)
	}
	return t.Ring
}

// destructiveRing is the focus ring of anything that warns: destructive
// buttons and invalid fields.
func destructiveRing(t *uitheme.Theme) color.Color {
	return fade(t.Destructive, pick(isDark(t.Bg), .8, .4))
}

// isDark reports whether c is closer to black than to white.
func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r+g+b < 3*32768
}

// iconButtonSide is the side of a square button holding one icon, as
// ButtonIcon pads it.
func iconButtonSide(t *uitheme.Theme) float64 { return max(t.IconSize, 20+2*t.ButtonPad.Top) }

// paintDisclosure paints content folding open below y: the body is clipped
// to progress of its height and inert while closing. The progress comes
// from Motion.LayoutToggle, which lays the disclosure out again for each
// step while it moves.
func paintDisclosure(dst *ggui.Canvas, content ggui.Widget, at ggui.Point, width float64, size ggui.Size, progress float64, open bool) {
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

// Shared hover timing for disclosure headers, independent of their opening motion.
var disclosureHoverSlot = ggui.NewSlot[*ggui.Motion]("disclosure hover")

// controlFocusRing matches the border and three-pixel soft focus halo.
func controlFocusRing(dst *ggui.Canvas, state *ggui.Interactive, t *uitheme.Theme, r ggui.Rect, radius float64, ring color.Color) {
	if !state.Focused || !state.FocusVisible || state.IsInert() {
		return
	}
	dst.StrokeRoundRect(r, radius, t.BorderWidth, ring)
	fieldHalo(dst, t, r, radius, ring)
}
