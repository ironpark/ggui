package ggui

import (
	"image/color"
	"math"

	"github.com/ironpark/ggui/internal/render"
)

// ShadowStyle describes an outer, rounded-rectangle shadow in logical pixels.
// Blur is the feather distance on either side of the edge, not Gaussian sigma.
// Positive Spread grows the silhouette; negative Spread shrinks it. Nil Color
// disables the shadow. Shadows do not change layout or pointer hit regions.
type ShadowStyle struct {
	Offset       Point
	Blur, Spread float64
	Color        color.Color
}

func (c *Canvas) shadowGeometry(r Rect, radius float64, s ShadowStyle) (render.Shadow, bool) {
	for _, v := range [...]float64{r.Origin.X, r.Origin.Y, r.Size.W, r.Size.H, radius, s.Offset.X, s.Offset.Y, s.Blur, s.Spread, c.Scale()} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return render.Shadow{}, false
		}
	}
	if r.Size.W <= 0 || r.Size.H <= 0 {
		return render.Shadow{}, false
	}
	w, h := r.Size.W+2*s.Spread, r.Size.H+2*s.Spread
	if w <= 0 || h <= 0 {
		return render.Shadow{}, false
	}
	scale := c.Scale()
	feather := max(max(s.Blur, 0)*scale, .5)
	center := r.Origin.Add(s.Offset).Add(Pt(r.Size.W/2, r.Size.H/2))
	cx, cy := c.px(center.X), c.px(center.Y)
	hw, hh := w*scale/2, h*scale/2
	// Clip before converting to integers: large offscreen shadows need no large
	// intermediate texture or unbounded draw quad.
	bounds, ok := render.Bounds(c.Image.Bounds(), cx, cy, hw, hh, feather)
	if !ok {
		return render.Shadow{}, false
	}
	// The corner never exceeds half the rect, grows with the spread and, once
	// scaled, never exceeds half the silhouette.
	corner := max(min(max(radius, 0), r.Size.W/2, r.Size.H/2)+s.Spread, 0)
	return render.Shadow{
		Bounds:  bounds,
		Centre:  Pt(cx, cy),
		Half:    Sz(hw, hh),
		Radius:  float32(min(corner*scale, hw, hh)),
		Feather: float32(feather),
	}, true
}

// Shadow draws a soft rounded-rectangle silhouette behind a surface. Paint the
// surface afterward. It uses one shared shader, one DrawTrianglesShader call,
// and no intermediate images or CPU rasterization. This distance-field feather
// approximates UI shadows; it is not an image blur. Parent clipping is respected.
func (c *Canvas) Shadow(r Rect, radius float64, s ShadowStyle) {
	if c == nil || c.Image == nil || s.Color == nil {
		return
	}
	tint := render.Premul(s.Color)
	if tint[3] == 0 {
		return
	}
	g, ok := c.shadowGeometry(r, radius, s)
	if !ok {
		return
	}
	render.DrawShadow(c.Image, g, tint)
}
