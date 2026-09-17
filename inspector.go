package ggui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// paintInspector draws the frame's trace: an outline for every widget,
// colored by depth, and a highlight with a label for the deepest widget
// under the cursor.
func paintInspector(dst *Canvas) {
	palette := []color.Color{
		color.RGBA{0xe5, 0x39, 0x35, 0xff}, color.RGBA{0xfb, 0x8c, 0x00, 0xff},
		color.RGBA{0x43, 0xa0, 0x47, 0xff}, color.RGBA{0x1e, 0x88, 0xe5, 0xff},
		color.RGBA{0x8e, 0x24, 0xaa, 0xff}, color.RGBA{0x00, 0x89, 0x7b, 0xff},
	}
	var hit *traceEntry
	p, ok := dst.Pointer()
	for i := range dst.trace {
		e := &dst.trace[i]
		dst.StrokeRoundRect(e.rect, 0, 1, palette[e.depth%len(palette)])
		if ok && e.rect.Contains(p) && (hit == nil || e.depth >= hit.depth) {
			hit = e
		}
	}
	if hit == nil {
		return
	}
	dst.FillRect(hit.rect, color.RGBA{0x1e, 0x88, 0xe5, 0x40})
	label := fmt.Sprintf("%s  %g×%g  @ %g,%g", hit.name, hit.rect.Size.W, hit.rect.Size.H, hit.rect.Origin.X, hit.rect.Origin.Y)
	face := fallbackFont().face(11 * dst.Scale())
	w := dst.dp(text.Advance(label, face))
	m := face.Metrics()
	h := dst.dp(m.HAscent + m.HDescent)
	const pad = 4
	at := Pt(hit.rect.Origin.X, hit.rect.Origin.Y+hit.rect.Size.H+2)
	if size := dst.Size(); size != (Size{}) {
		at.X = clamp(at.X, 0, max(size.W-w-2*pad, 0))
		if at.Y+h+2*pad > size.H {
			at.Y = max(hit.rect.Origin.Y-h-2*pad-2, 0)
		}
	}
	dst.FillRoundRect(Rct(at, Sz(w+2*pad, h+2*pad)), 3, color.RGBA{0x1f, 0x23, 0x28, 0xe6})
	op := &text.DrawOptions{}
	op.GeoM.Translate(dst.px(at.X+pad), dst.px(at.Y+pad))
	op.ColorScale.ScaleWithColor(color.White)
	text.Draw(dst.Image, label, face, op)
}
