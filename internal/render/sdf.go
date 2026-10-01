// Package render draws the shapes ggui's Canvas is built from onto a ggfx
// image, in device pixels. The Canvas turns logical coordinates into device
// ones and decides what to draw; this package owns the shaders, the vertices
// they read and the uniform blocks they fill.
package render

import (
	"image"
	"image/color"
	"math"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/geom"
)

// Rounded rectangles are drawn from a signed distance field rather than
// traced as vector paths. ggfx fills an anti-aliased path by rendering it
// eight times into an offscreen stencil buffer and compositing the result,
// and every hop between the screen and that buffer commits the whole command
// buffer: one rounded rectangle costs two. A distance field needs no
// intermediate image at all, so the frame stays in one render pass, and its
// coverage is computed rather than sampled, which is also the more accurate
// edge.
//
// The same shape serves a fill and a border. Width is half the border, in
// pixels; at zero the field is filled to its outline instead. A line is
// this shape turned to lie along it, and a border is the band around one.
//
// Everything that differs from one shape to the next rides on the
// vertices, not in uniforms: ggfx merges consecutive draws only when their
// uniforms are equal, so shapes carried this way share one draw call.
// custom.xy is the fragment's place in the shape's own frame, centred and
// unturned, which is linear in the vertices and so interpolates exactly;
// custom.zw is the half size, src_pos is the radius and the width, and
// color is the tint.
const roundRectSource = roundRectDistance + `
const feather = 0.5; // the Go constant Feather

fn fragment(v: Vertex) -> vec4f {
	var distance = round_rect_distance(v.custom.xy, v.custom.zw, v.src_pos.x);
	if (v.src_pos.y > 0.0) {
		distance = abs(distance) - v.src_pos.y;
	}
	let coverage = 1.0 - smoothstep(-feather, feather, distance);
	return v.color * coverage;
}
`

// roundRectDistance is the signed distance from p to the outline of a
// rectangle centred on the origin, rounded by radius; every shape uses it.
const roundRectDistance = `
fn round_rect_distance(p: vec2f, half_size: vec2f, radius: f32) -> f32 {
	let q = abs(p) - half_size + vec2f(radius);
	return length(max(q, vec2f(0.0))) + min(max(q.x, q.y), 0.0) - radius;
}
`

var sharedRoundRect = compile("round rect shader", roundRectSource)

// Bounds returns the pixels of target a distance-field shape covers: the
// silhouette centred at cx, cy with half size hw, hh, grown by reach for the
// edge it feathers over and for a border drawn outside the outline. Anything
// the target cannot show is left out, so an offscreen shape needs no quad.
func Bounds(target image.Rectangle, cx, cy, hw, hh, reach float64) (image.Rectangle, bool) {
	left, top := max(cx-hw-reach, float64(target.Min.X)), max(cy-hh-reach, float64(target.Min.Y))
	right, bottom := min(cx+hw+reach, float64(target.Max.X)), min(cy+hh+reach, float64(target.Max.Y))
	if right <= left || bottom <= top {
		return image.Rectangle{}, false
	}
	b := image.Rect(int(math.Floor(left)), int(math.Floor(top)), int(math.Ceil(right)), int(math.Ceil(bottom))).Intersect(target)
	return b, !b.Empty()
}

// Feather is the width in pixels of the edge a distance field fades across,
// half to either side, which is the narrowest ramp that still antialiases.
const Feather = .5

// Field is a shape for the shader to fill, in pixels. Axis is the unit
// vector the half size's width runs along, which is how a line is placed;
// Radius rounds the corners, and a Stroke of half that width is drawn as
// the band around the outline instead of filling it.
type Field struct {
	Centre, Axis   geom.Point
	Half           geom.Size
	Radius, Stroke float64
}

// Shade draws f in col onto dst, over no more of dst than f can reach.
func Shade(dst *ggfx.Image, f Field, col color.Color) {
	if _, _, _, alpha := col.RGBA(); alpha == 0 {
		return
	}
	for _, v := range [...]float64{f.Centre.X, f.Centre.Y, f.Axis.X, f.Axis.Y, f.Half.W, f.Half.H, f.Radius, f.Stroke} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
	}
	if f.Half.W < 0 || f.Half.H < 0 {
		return
	}
	// A turned shape covers the box its corners reach, not its own.
	ax, ay := math.Abs(f.Axis.X), math.Abs(f.Axis.Y)
	reach := f.Stroke + Feather + 1
	hx, hy := ax*f.Half.W+ay*f.Half.H, ay*f.Half.W+ax*f.Half.H
	bounds, ok := Bounds(dst.Bounds(), f.Centre.X, f.Centre.Y, hx, hy, reach)
	if !ok {
		return
	}
	// A corner never exceeds half the shape, which is a circle.
	radius := min(max(f.Radius, 0), f.Half.W, f.Half.H)
	vs := quad(bounds, f.Centre, f.Axis, f.Half, float32(radius), float32(max(f.Stroke, 0)), Premul(col))
	dst.DrawTrianglesShader(vs[:], quadIndices[:], sharedRoundRect(), nil)
}

// quad is the quad over bounds that draws a distance-field shape centred
// at centre, turned to axis, with half size half, as roundRectSource and
// shadowSource read it: the fragment's place in the shape's own frame in
// custom.xy, the half size in custom.zw, the two scalars in src_pos and
// the tint as the colour.
func quad(bounds image.Rectangle, centre, axis geom.Point, half geom.Size, s0, s1 float32, tint [4]float32) [4]ggfx.Vertex {
	x0, y0, x1, y1 := float64(bounds.Min.X), float64(bounds.Min.Y), float64(bounds.Max.X), float64(bounds.Max.Y)
	var vs [4]ggfx.Vertex
	for i, corner := range [4]geom.Point{geom.Pt(x0, y0), geom.Pt(x1, y0), geom.Pt(x0, y1), geom.Pt(x1, y1)} {
		p := geom.Pt(corner.X-centre.X, corner.Y-centre.Y)
		vs[i] = ggfx.Vertex{
			DstX: float32(corner.X), DstY: float32(corner.Y),
			SrcX: s0, SrcY: s1,
			ColorR: tint[0], ColorG: tint[1], ColorB: tint[2], ColorA: tint[3],
			Custom0: float32(p.X*axis.X + p.Y*axis.Y), Custom1: float32(p.Y*axis.X - p.X*axis.Y),
			Custom2: float32(half.W), Custom3: float32(half.H),
		}
	}
	return vs
}

// quadIndices are the two triangles of a quad whose corners run top left,
// top right, bottom left, bottom right, as uint32, which a draw takes
// without converting.
var quadIndices = [...]uint32{0, 1, 2, 1, 3, 2}

// Premul is col as the premultiplied floats a shader takes.
func Premul(col color.Color) [4]float32 {
	r, g, b, a := col.RGBA()
	return [4]float32{float32(r) / 0xffff, float32(g) / 0xffff, float32(b) / 0xffff, float32(a) / 0xffff}
}

// roundRectMaskSource draws source image 0 through the rounded rectangle
// half_size around center, both relative to the image's own origin, which is
// how ClipRoundRect composites its layer.
const roundRectMaskSource = roundRectDistance + `
struct Uniforms {
	center: vec2f,
	half_size: vec2f,
	radius: f32,
	feather: f32,
}
@group(1) @binding(0) var<uniform> u: Uniforms;
fn fragment(v: Vertex) -> vec4f {
	let distance = round_rect_distance(v.src_pos - src0_origin() - u.center, u.half_size, u.radius);
	return src0_at(v.src_pos) * (1.0 - smoothstep(-u.feather, u.feather, distance));
}
`

var (
	sharedRoundRectMask   = compile("round rect mask shader", roundRectMaskSource)
	roundRectMaskUniforms = newUniformBlocks(sharedRoundRectMask)
)

// MaskRoundRect draws img onto dst where it lies within the rounded
// rectangle centred at centre with half size half and corners rounded by
// radius, all in dst's pixels. img is drawn at its own bounds.
func MaskRoundRect(dst, img *ggfx.Image, centre geom.Point, half geom.Size, radius float64) {
	b := img.Bounds()
	u := roundRectMaskUniforms.get()
	defer roundRectMaskUniforms.put(u)
	u.SetSlice("center", []float32{float32(centre.X - float64(b.Min.X)), float32(centre.Y - float64(b.Min.Y))})
	u.SetSlice("half_size", []float32{float32(half.W), float32(half.H)})
	u.Set("radius", float32(min(radius, half.W, half.H)))
	u.Set("feather", float32(Feather))
	op := &ggfx.DrawRectShaderOptions{UniformBlock: u}
	op.Images[0] = img
	op.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	dst.DrawRectShader(b.Dx(), b.Dy(), sharedRoundRectMask(), op)
}

// roundRectImageSource draws source image 0 cut to a rounded rectangle in
// one pass. custom.xy is the fragment's place relative to the rectangle's
// centre, unturned. The image is filtered here rather than by a mipmap,
// which a shader does not get: bilinear at one texel a pixel or more, and
// shrunk further, the average of a grid of bilinear taps spread over the
// pixel's footprint, up to eight each way.
const roundRectImageSource = roundRectDistance + `
struct Uniforms {
	half_size: vec2f,
	radius: f32,
	feather: f32,
	pixelated: i32,
}
@group(1) @binding(0) var<uniform> u: Uniforms;
fn bilinear(p: vec2f) -> vec4f {
	let q = clamp(p, src0_origin() + 0.5, src0_origin() + src0_size() - 0.5);
	if (u.pixelated != 0) {
		return src0_at(q);
	}
	let p0 = q - 0.5;
	let p1 = q + 0.5;
	let rate = fract(p1);
	let top = mix(src0_at(p0), src0_at(vec2f(p1.x, p0.y)), rate.x);
	let bottom = mix(src0_at(vec2f(p0.x, p1.y)), src0_at(p1), rate.x);
	return mix(top, bottom, rate.y);
}
fn fragment(v: Vertex) -> vec4f {
	let dx = dpdx(v.src_pos);
	let dy = dpdy(v.src_pos);
	let coverage = 1.0 - smoothstep(-u.feather, u.feather, round_rect_distance(v.custom.xy, u.half_size, u.radius));
	if (coverage <= 0.0) {
		return vec4f(0.0);
	}
	var n = vec2i(1, 1);
	if (u.pixelated == 0) {
		n = vec2i(clamp(ceil(vec2f(length(dx), length(dy))), vec2f(1.0), vec2f(8.0)));
	}
	var sum = vec4f(0.0);
	for (var j = 0; j < n.y; j++) {
		for (var i = 0; i < n.x; i++) {
			let t = (vec2f(f32(i), f32(j)) + 0.5) / vec2f(n) - 0.5;
			sum += bilinear(v.src_pos + t.x * dx + t.y * dy);
		}
	}
	return sum / f32(n.x * n.y) * v.color * coverage;
}
`

var (
	sharedRoundRectImage   = compile("round rect image shader", roundRectImageSource)
	roundRectImageUniforms = newUniformBlocks(sharedRoundRectImage)
)

// RoundRectImage draws img through the quad vs, cut to a rounded rectangle
// with half size half and corners rounded by radius, in dst's pixels. Each
// vertex carries its place in img as its source, its tint as its colour and
// its offset from the rectangle's centre, unturned, in Custom0 and Custom1.
func RoundRectImage(dst, img *ggfx.Image, vs [4]ggfx.Vertex, half geom.Size, radius float64, pixelated bool) {
	u := roundRectImageUniforms.get()
	defer roundRectImageUniforms.put(u)
	u.SetSlice("half_size", []float32{float32(half.W), float32(half.H)})
	u.Set("radius", float32(min(radius, half.W, half.H)))
	u.Set("feather", float32(Feather))
	u.SetBool("pixelated", pixelated)
	op := &ggfx.DrawTrianglesShaderOptions{UniformBlock: u}
	op.Images[0] = img
	dst.DrawTrianglesShader(vs[:], quadIndices[:], sharedRoundRectImage(), op)
}
