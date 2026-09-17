package ggui

import (
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
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

const shadowSource = `//kage:unit pixels
package main
var Center vec2
var HalfSize vec2
var Radius float
var Feather float
var Tint vec4
func Fragment(dst vec4, src vec2, color vec4) vec4 {
 q := abs(src-Center)-HalfSize+vec2(Radius)
 distance := length(max(q,vec2(0)))+min(max(q.x,q.y),0)-Radius
 coverage := 1-smoothstep(-Feather,Feather,distance)
 return Tint*coverage
}
`

var sharedShadow = sync.OnceValue(func() *ebiten.Shader {
	shader, err := ebiten.NewShader([]byte(shadowSource))
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
	values := []float64{r.Origin.X, r.Origin.Y, r.Size.W, r.Size.H, radius, s.Offset.X, s.Offset.Y, s.Blur, s.Spread, c.Scale()}
	for _, v := range values {
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
	target := c.Image.Bounds()
	left, top := max(cx-hw-feather, float64(target.Min.X)), max(cy-hh-feather, float64(target.Min.Y))
	right, bottom := min(cx+hw+feather, float64(target.Max.X)), min(cy+hh+feather, float64(target.Max.Y))
	if right <= left || bottom <= top {
		return shadowGeometry{}, false
	}
	bounds := image.Rect(int(math.Floor(left)), int(math.Floor(top)), int(math.Ceil(right)), int(math.Ceil(bottom))).Intersect(target)
	return shadowGeometry{bounds: bounds, center: [2]float32{float32(cx - float64(bounds.Min.X)), float32(cy - float64(bounds.Min.Y))}, half: [2]float32{float32(hw), float32(hh)}, radius: float32(min(max(min(max(radius, 0), r.Size.W/2, r.Size.H/2)+s.Spread, 0)*scale, hw, hh)), feather: float32(feather)}, !bounds.Empty()
}

// Shadow draws a soft rounded-rectangle silhouette behind a surface. Paint the
// surface afterward. It uses one shared Kage shader, one DrawRectShader call,
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
	op := &ebiten.DrawRectShaderOptions{Uniforms: map[string]any{
		"Center": g.center[:], "HalfSize": g.half[:], "Radius": g.radius, "Feather": g.feather,
		"Tint": []float32{float32(red) / 65535, float32(green) / 65535, float32(blue) / 65535, float32(alpha) / 65535},
	}}
	op.GeoM.Translate(float64(g.bounds.Min.X), float64(g.bounds.Min.Y))
	c.Image.DrawRectShader(g.bounds.Dx(), g.bounds.Dy(), sharedShadow(), op)
}
