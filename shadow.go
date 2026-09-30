package ggui

import (
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/ironpark/ggfx"
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

// shadowSource takes every value on the vertices, as roundRectSource does,
// so that shadows drawn one after another share a draw call. custom.xy is
// the fragment's place relative to the silhouette's centre, custom.zw its
// half size, src_pos the corner radius and the feather, and color the tint.
const shadowSource = roundRectDistance + `
fn fragment(v: Vertex) -> vec4f {
	let distance = round_rect_distance(v.custom.xy, v.custom.zw, v.src_pos.x);
	let feather = v.src_pos.y;
	let coverage = 1.0 - smoothstep(-feather, feather, distance);
	return v.color * coverage;
}
`

var sharedShadow = sync.OnceValue(func() *ggfx.Shader {
	shader, err := ggfx.NewShader([]byte(shadowSource))
	if err != nil {
		panic("ggui: compile shadow shader: " + err.Error())
	}
	return shader
})

type shadowGeometry struct {
	bounds          image.Rectangle
	centre          Point
	half            Size
	radius, feather float32
}

func (c *Canvas) shadowGeometry(r Rect, radius float64, s ShadowStyle) (shadowGeometry, bool) {
	for _, v := range [...]float64{r.Origin.X, r.Origin.Y, r.Size.W, r.Size.H, radius, s.Offset.X, s.Offset.Y, s.Blur, s.Spread, c.Scale()} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return shadowGeometry{}, false
		}
	}
	if r.Size.W <= 0 || r.Size.H <= 0 {
		return shadowGeometry{}, false
	}
	w, h := r.Size.W+2*s.Spread, r.Size.H+2*s.Spread
	if w <= 0 || h <= 0 {
		return shadowGeometry{}, false
	}
	scale := c.Scale()
	feather := max(max(s.Blur, 0)*scale, .5)
	center := r.Origin.Add(s.Offset).Add(Pt(r.Size.W/2, r.Size.H/2))
	cx, cy := c.px(center.X), c.px(center.Y)
	hw, hh := w*scale/2, h*scale/2
	// Clip before converting to integers: large offscreen shadows need no large
	// intermediate texture or unbounded draw quad.
	bounds, ok := sdfBounds(c.Image.Bounds(), cx, cy, hw, hh, feather)
	if !ok {
		return shadowGeometry{}, false
	}
	// The corner never exceeds half the rect, grows with the spread and, once
	// scaled, never exceeds half the silhouette.
	corner := max(min(max(radius, 0), r.Size.W/2, r.Size.H/2)+s.Spread, 0)
	return shadowGeometry{
		bounds:  bounds,
		centre:  Pt(cx, cy),
		half:    Sz(hw, hh),
		radius:  float32(min(corner*scale, hw, hh)),
		feather: float32(feather),
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
	tint := premul(s.Color)
	if tint[3] == 0 {
		return
	}
	g, ok := c.shadowGeometry(r, radius, s)
	if !ok {
		return
	}
	vs := sdfQuad(g.bounds, g.centre, Pt(1, 0), g.half, g.radius, g.feather, tint)
	c.Image.DrawTrianglesShader(vs[:], quadIndices[:], sharedShadow(), nil)
}
