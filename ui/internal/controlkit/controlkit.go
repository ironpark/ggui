// Package controlkit holds the look and input helpers that the controls of
// ui, ui/chart and ui/chat share.
package controlkit

import (
	"image/color"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/icons/lucide"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ColorOr is value, or fallback when value is nil.
func ColorOr(value, fallback color.Color) color.Color {
	if value != nil {
		return value
	}
	return fallback
}

// IsDark reports whether c is closer to black than to white.
func IsDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r+g+b < 3*32768
}

// SquareGlyph is the box a checkbox or radio draws itself in.
func SquareGlyph(t *uitheme.Theme) ggui.Size { return ggui.Sz(t.ControlSize, t.ControlSize) }

// Ease is Canvas.Ease along the theme's Ease, the curve of a Tailwind
// transition: what a control's hover, focus, checked and invalid states
// move on. A nil t is the default theme's.
func Ease(dst *ggui.Canvas, t *uitheme.Theme, at ggui.Anchor, s ggui.Slot[*ggui.Motion], target float64, d time.Duration) float64 {
	var c uitheme.Curve
	if t != nil {
		c = t.Ease
	}
	return dst.EaseWith(at, s, target, d, c.Easing())
}

// HaloStroke draws the three-pixel halo just outside r in col, as it is.
func HaloStroke(dst *ggui.Canvas, r ggui.Rect, radius float64, col color.Color) {
	dst.StrokeRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4)), radius+2, 3, col)
}

// FieldHalo draws the focus ring just outside a text field, at the theme's
// halo opacity.
func FieldHalo(dst *ggui.Canvas, t *uitheme.Theme, r ggui.Rect, radius float64, ring color.Color) {
	HaloStroke(dst, r, radius, t.Halo(ring))
}

// IconMark is an icon placed by its center: Role, resolved under the Env
// and falling back to Lucide, drawn Size square around Center, tinted Color
// and rotated clockwise by Rotation radians.
type IconMark struct {
	Role     icons.Role
	Center   ggui.Point
	Size     float64
	Color    color.Color
	Rotation float64
}

// PaintIconAt paints mark's icon centered on its Center.
func PaintIconAt(dst *ggui.Canvas, env ggui.Env, mark IconMark) {
	rect := ggui.Rct(mark.Center.Add(ggui.Pt(-mark.Size/2, -mark.Size/2)), ggui.Sz(mark.Size, mark.Size))
	icons.Resolve(env, lucide.Set(), mark.Role).Draw(dst, rect, mark.Color, mark.Rotation)
}

// Disclosure is a body folding open below At: laid out at Size, shown
// Width wide and Progress of its height, and Open unless it is closing.
type Disclosure struct {
	At       ggui.Point
	Width    float64
	Size     ggui.Size
	Progress float64
	Open     bool
}

// PaintDisclosure paints content folding open below d.At: the body is
// clipped to d.Progress of its height and inert while closing. The progress
// comes from Motion.LayoutToggle, which lays the disclosure out again for
// each step while it moves.
func PaintDisclosure(dst *ggui.Canvas, content ggui.Widget, d Disclosure) {
	if d.Progress <= 0 {
		return
	}
	body := dst.Clip(ggui.Rct(d.At, ggui.Sz(d.Width, d.Size.H*d.Progress)))
	if !d.Open {
		body = body.Inert()
	}
	body.Paint(content, ggui.Rct(d.At, d.Size))
}

// PaintShadows draws the layers of a shadow under r, for a surface that is
// painted rather than a Box.
func PaintShadows(dst *ggui.Canvas, r ggui.Rect, radius float64, layers ...ggui.ShadowStyle) {
	for _, s := range layers {
		if s.Color != nil {
			dst.Shadow(r, radius, s)
		}
	}
}
