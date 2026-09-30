package render

import (
	"image"

	"github.com/ironpark/ggfx"
)

const pathGradientSource = `
struct Uniforms {
	top: vec4f,
	bottom: vec4f,
	height: f32,
	offset: f32,
}
@group(1) @binding(0) var<uniform> u: Uniforms;
fn fragment(v: Vertex) -> vec4f {
	let y = v.src_pos.y - src0_origin().y + u.offset;
	return mix(u.top, u.bottom, clamp(y / u.height, 0.0, 1.0)) * src0_at(v.src_pos).a;
}
`

var (
	sharedPathGradient   = compile("path gradient", pathGradientSource)
	pathGradientUniforms = newUniformBlocks(sharedPathGradient)
)

// VerticalGradient draws a gradient from top to bottom, premultiplied
// colours, onto dst at at through the alpha of mask. The gradient runs over
// height pixels; offset is how far down it the mask's first row lies.
func VerticalGradient(dst, mask *ggfx.Image, at image.Point, top, bottom [4]float32, height, offset float64) {
	b := mask.Bounds()
	u := pathGradientUniforms.get()
	defer pathGradientUniforms.put(u)
	u.SetSlice("top", top[:])
	u.SetSlice("bottom", bottom[:])
	u.Set("height", float32(height))
	u.Set("offset", float32(offset))
	op := &ggfx.DrawRectShaderOptions{UniformBlock: u}
	op.Images[0] = mask
	op.GeoM.Translate(float64(at.X), float64(at.Y))
	dst.DrawRectShader(b.Dx(), b.Dy(), sharedPathGradient(), op)
}
