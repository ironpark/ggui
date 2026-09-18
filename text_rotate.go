package ggui

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
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
	if t.value != "" && !dst.named(r) {
		dst.Leaf(r, Node{Role: pick(t.role == roleTitle, RoleHeading, RoleText), Name: t.value})
	}
	if dst == nil || dst.Image == nil || dst.Image.Bounds().Empty() {
		return
	}
	center := r.Origin.Add(Pt(r.Size.W/2, r.Size.H/2))
	face := t.faceAt(dst.Scale())
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(t.current().Color)
	for i, line := range t.lines {
		x := r.Origin.X + (r.Size.W-t.widths[i])*t.align
		y := r.Origin.Y + float64(i)*t.spacing()
		op.GeoM.Reset()
		op.GeoM.Translate(dst.px(x-center.X), dst.px(y-center.Y))
		op.GeoM.Rotate(radians)
		op.GeoM.Translate(dst.px(center.X), dst.px(center.Y))
		drawText(dst.Image, line, face, op)
	}
}
