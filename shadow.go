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
const shadowSource = `
fn fragment(v: Vertex) -> vec4f {
	let radius = v.src_pos.x;
	let feather = v.src_pos.y;
	let q = abs(v.custom.xy) - v.custom.zw + vec2f(radius);
	let distance = length(max(q, vec2f(0.0))) + min(max(q.x, q.y), 0.0) - radius;
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
	center, half    [2]float32
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
		center:  [2]float32{float32(cx - float64(bounds.Min.X)), float32(cy - float64(bounds.Min.Y))},
		half:    [2]float32{float32(hw), float32(hh)},
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
	red, green, blue, alpha := s.Color.RGBA()
	if alpha == 0 {
		return
	}
	g, ok := c.shadowGeometry(r, radius, s)
	if !ok {
		return
	}
	var vs [4]ggfx.Vertex
	for i, corner := range corners(g.bounds) {
		vs[i] = ggfx.Vertex{
			DstX: float32(corner.X), DstY: float32(corner.Y),
			SrcX: g.radius, SrcY: g.feather,
			ColorR: float32(red) / 65535, ColorG: float32(green) / 65535, ColorB: float32(blue) / 65535, ColorA: float32(alpha) / 65535,
			Custom0: float32(corner.X-float64(g.bounds.Min.X)) - g.center[0], Custom1: float32(corner.Y-float64(g.bounds.Min.Y)) - g.center[1],
			Custom2: g.half[0], Custom3: g.half[1],
		}
	}
	c.Image.DrawTrianglesShader(vs[:], quadIndices[:], sharedShadow(), nil)
}
