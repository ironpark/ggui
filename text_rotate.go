package ggui

import (
	"math"

	"github.com/ironpark/ggfx/text/v2"
)

// PaintRotated paints already-laid-out text clockwise around the center of r.
// It uses the regular glyph cache without allocating an intermediate image.
// Layout is unchanged; the caller reserves space for the rotated bounds.
func (t *TextWidget) PaintRotated(dst *Canvas, r Rect, radians float64) {
	if radians == 0 {
		t.Paint(dst, r)
		return
	}
	if math.IsNaN(radians) || math.IsInf(radians, 0) {
		return
	}
	center := r.Center()
	t.paintLines(dst, r, func(op *text.DrawOptions, x, y float64) {
		op.GeoM.Translate(dst.px(x-center.X), dst.px(y-center.Y))
		op.GeoM.Rotate(radians)
		op.GeoM.Translate(dst.px(center.X), dst.px(center.Y))
	})
}
