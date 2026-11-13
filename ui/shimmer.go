package ui

import (
	"image/color"
	"math"

	"github.com/ironpark/ggui"
)

// paintShimmer moves a highlight across text already painted over r in
// base: a band of the glyphs, drawn again toward highlight, that crosses
// the line every two seconds. It leaves the text in base. The band steps
// on loopStep and asks for the frames it needs, so the caller paints it
// only while the text is in view and motion is wanted.
func paintShimmer(dst *ggui.Canvas, text *ggui.TextWidget, r ggui.Rect, base, highlight color.Color) {
	const (
		bands = 16
		bandW = 4.0
	)
	phase := float64(ggui.Paced(loopStep).UnixMilli()%2000) / 2000
	center := (r.Size.W+80)*phase - 40
	for i := range bands {
		x := center + float64(i)*bandW - bands*bandW/2
		band := ggui.Rct(r.Origin.Add(ggui.Pt(x, 0.0)), ggui.Sz(bandW, r.Size.H)).Intersect(r)
		if band.Empty() {
			continue
		}
		strength := 1 - math.Abs(float64(i)-(bands-1)/2.0)/(bands/2)
		text.Color(mix(base, highlight, .8*strength))
		dst.Clip(band).Inert().Paint(text, r)
	}
	text.Color(base)
}
