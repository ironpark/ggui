package ggui

import (
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Rounded rectangles are drawn from a signed distance field rather than
// traced as vector paths. ebiten fills an anti-aliased path by rendering it
// eight times into an offscreen stencil buffer and compositing the result,
// and every hop between the screen and that buffer commits the whole command
// buffer: one rounded rectangle costs two. A distance field needs no
// intermediate image at all, so the frame stays in one render pass, and its
// coverage is computed rather than sampled, which is also the more accurate
// edge.
//
// The same shape serves a fill and a border. Width is half the border, in
// pixels; at zero the field is filled to its outline instead.
const roundRectSource = `//kage:unit pixels
package main
var Center vec2
var HalfSize vec2
var Radius float
var Width float
var Feather float
var Tint vec4
func Fragment(dst vec4, src vec2, color vec4) vec4 {
 q := abs(src-Center)-HalfSize+vec2(Radius)
 distance := length(max(q,vec2(0)))+min(max(q.x,q.y),0)-Radius
 if Width > 0 {
  distance = abs(distance)-Width
 }
 coverage := 1-smoothstep(-Feather,Feather,distance)
 return Tint*coverage
}
`

var sharedRoundRect = sync.OnceValue(func() *ebiten.Shader {
	shader, err := ebiten.NewShader([]byte(roundRectSource))
	if err != nil {
		panic("ggui: compile round rect shader: " + err.Error())
	}
	return shader
})

// sdfBounds returns the pixels of target a distance-field shape covers: the
// silhouette centred at cx, cy with half size hw, hh, grown by reach for the
// edge it feathers over and for a border drawn outside the outline. Anything
// the target cannot show is left out, so an offscreen shape needs no quad.
func sdfBounds(target image.Rectangle, cx, cy, hw, hh, reach float64) (image.Rectangle, bool) {
	left, top := max(cx-hw-reach, float64(target.Min.X)), max(cy-hh-reach, float64(target.Min.Y))
	right, bottom := min(cx+hw+reach, float64(target.Max.X)), min(cy+hh+reach, float64(target.Max.Y))
	if right <= left || bottom <= top {
		return image.Rectangle{}, false
	}
	b := image.Rect(int(math.Floor(left)), int(math.Floor(top)), int(math.Ceil(right)), int(math.Ceil(bottom))).Intersect(target)
	return b, !b.Empty()
}

// feather is the width in pixels of the edge a distance field fades across,
// half to either side, which is the narrowest ramp that still antialiases.
const feather = .5

// shadeRoundRect draws the rounded rectangle shape in col. A zero halfStroke
// fills it; otherwise shape is the centreline of a border of twice that
// width. Both are logical, as radius is.
func (c *Canvas) shadeRoundRect(shape Rect, radius, halfStroke float64, col color.Color) {
	red, green, blue, alpha := col.RGBA()
	if alpha == 0 {
		return
	}
	scale := c.Scale()
	for _, v := range [...]float64{shape.Origin.X, shape.Origin.Y, shape.Size.W, shape.Size.H, radius, halfStroke, scale} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
	}
	if shape.Size.W < 0 || shape.Size.H < 0 {
		return
	}
	hw, hh := shape.Size.W*scale/2, shape.Size.H*scale/2
	cx, cy := c.px(shape.Origin.X+shape.Size.W/2), c.px(shape.Origin.Y+shape.Size.H/2)
	stroke := max(halfStroke, 0) * scale
	bounds, ok := sdfBounds(c.Image.Bounds(), cx, cy, hw, hh, stroke+feather+1)
	if !ok {
		return
	}
	op := &ebiten.DrawRectShaderOptions{Uniforms: map[string]any{
		"Center":   []float32{float32(cx - float64(bounds.Min.X)), float32(cy - float64(bounds.Min.Y))},
		"HalfSize": []float32{float32(hw), float32(hh)},
		// A corner never exceeds half the shape, which is a circle.
		"Radius":  float32(min(max(radius, 0)*scale, hw, hh)),
		"Width":   float32(stroke),
		"Feather": float32(feather),
		"Tint":    []float32{float32(red) / 65535, float32(green) / 65535, float32(blue) / 65535, float32(alpha) / 65535},
	}}
	op.GeoM.Translate(float64(bounds.Min.X), float64(bounds.Min.Y))
	c.Image.DrawRectShader(bounds.Dx(), bounds.Dy(), sharedRoundRect(), op)
}
