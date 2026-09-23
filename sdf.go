package ggui

import (
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/ironpark/ggfx"
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
// pixels; at zero the field is filled to its outline instead. Axis is the
// unit vector the shape's own width runs along, so a line is this shape
// turned to lie along it and a border is the band around one.
const roundRectSource = `
struct Uniforms {
	center: vec2f,
	axis: vec2f,
	half_size: vec2f,
	radius: f32,
	width: f32,
	feather: f32,
	tint: vec4f,
}
@group(1) @binding(0) var<uniform> u: Uniforms;
fn fragment(v: Vertex) -> vec4f {
	let p = v.src_pos - u.center;
	let local = vec2f(dot(p, u.axis), dot(p, vec2f(-u.axis.y, u.axis.x)));
	let q = abs(local) - u.half_size + vec2f(u.radius);
	var distance = length(max(q, vec2f(0.0))) + min(max(q.x, q.y), 0.0) - u.radius;
	if (u.width > 0.0) {
		distance = abs(distance) - u.width;
	}
	let coverage = 1.0 - smoothstep(-u.feather, u.feather, distance);
	return u.tint * coverage;
}
`

var sharedRoundRect = sync.OnceValue(func() *ggfx.Shader {
	shader, err := ggfx.NewShader([]byte(roundRectSource))
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

// field is a shape for the shader to fill, in pixels. Axis is the unit
// vector the half size's width runs along, which is how a line is placed;
// radius rounds the corners, and a stroke of half that width is drawn as
// the band around the outline instead of filling it.
type field struct {
	centre, axis   Point
	half           Size
	radius, stroke float64
}

// shade draws f in col, over no more of the target than f can reach.
func (c *Canvas) shade(f field, col color.Color) {
	red, green, blue, alpha := col.RGBA()
	if alpha == 0 {
		return
	}
	for _, v := range [...]float64{f.centre.X, f.centre.Y, f.axis.X, f.axis.Y, f.half.W, f.half.H, f.radius, f.stroke} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
	}
	if f.half.W < 0 || f.half.H < 0 {
		return
	}
	// A turned shape covers the box its corners reach, not its own.
	ax, ay := math.Abs(f.axis.X), math.Abs(f.axis.Y)
	reach := f.stroke + feather + 1
	hx, hy := ax*f.half.W+ay*f.half.H, ay*f.half.W+ax*f.half.H
	bounds, ok := sdfBounds(c.Image.Bounds(), f.centre.X, f.centre.Y, hx, hy, reach)
	if !ok {
		return
	}
	op := &ggfx.DrawRectShaderOptions{Uniforms: map[string]any{
		"center":    []float32{float32(f.centre.X - float64(bounds.Min.X)), float32(f.centre.Y - float64(bounds.Min.Y))},
		"axis":      []float32{float32(f.axis.X), float32(f.axis.Y)},
		"half_size": []float32{float32(f.half.W), float32(f.half.H)},
		// A corner never exceeds half the shape, which is a circle.
		"radius":  float32(min(max(f.radius, 0), f.half.W, f.half.H)),
		"width":   float32(max(f.stroke, 0)),
		"feather": float32(feather),
		"tint":    []float32{float32(red) / 65535, float32(green) / 65535, float32(blue) / 65535, float32(alpha) / 65535},
	}}
	op.GeoM.Translate(float64(bounds.Min.X), float64(bounds.Min.Y))
	c.Image.DrawRectShader(bounds.Dx(), bounds.Dy(), sharedRoundRect(), op)
}

// shadeRoundRect draws the rounded rectangle shape in col. A zero halfStroke
// fills it; otherwise shape is the centreline of a border of twice that
// width. Both are logical, as radius is.
func (c *Canvas) shadeRoundRect(shape Rect, radius, halfStroke float64, col color.Color) {
	scale := c.Scale()
	if math.IsNaN(scale) || math.IsInf(scale, 0) {
		return
	}
	c.shade(field{
		centre: Pt(c.px(shape.Origin.X+shape.Size.W/2), c.px(shape.Origin.Y+shape.Size.H/2)),
		axis:   Pt(1, 0),
		half:   Sz(shape.Size.W*scale/2, shape.Size.H*scale/2),
		radius: radius * scale,
		stroke: halfStroke * scale,
	}, col)
}

// shadeSegment draws the line from a to b, w wide, in col. The ends are cut
// square, as the vector renderer's default cap is.
func (c *Canvas) shadeSegment(a, b Point, w float64, col color.Color) {
	scale := c.Scale()
	if math.IsNaN(scale) || math.IsInf(scale, 0) {
		return
	}
	dx, dy := c.px(b.X-a.X), c.px(b.Y-a.Y)
	length := math.Hypot(dx, dy)
	if length == 0 || math.IsNaN(length) || math.IsInf(length, 0) {
		return
	}
	c.shade(field{
		centre: Pt(c.px((a.X+b.X)/2), c.px((a.Y+b.Y)/2)),
		axis:   Pt(dx/length, dy/length),
		half:   Sz(length/2, w*scale/2),
	}, col)
}

// roundRectMaskSource draws source image 0 through the rounded rectangle
// half_size around center, both relative to the image's own origin, which is
// how ClipRoundRect composites its layer.
const roundRectMaskSource = `
struct Uniforms {
	center: vec2f,
	half_size: vec2f,
	radius: f32,
	feather: f32,
}
@group(1) @binding(0) var<uniform> u: Uniforms;
fn fragment(v: Vertex) -> vec4f {
	let q = abs(v.src_pos - src0_origin() - u.center) - u.half_size + vec2f(u.radius);
	let distance = length(max(q, vec2f(0.0))) + min(max(q.x, q.y), 0.0) - u.radius;
	return src0_at(v.src_pos) * (1.0 - smoothstep(-u.feather, u.feather, distance));
}
`

var sharedRoundRectMask = sync.OnceValue(func() *ggfx.Shader {
	shader, err := ggfx.NewShader([]byte(roundRectMaskSource))
	if err != nil {
		panic("ggui: compile round rect mask shader: " + err.Error())
	}
	return shader
})

// maskRoundRect draws img, a part of a layer, onto c where it lies within
// the logical Rect r with corners rounded by radius.
func (c *Canvas) maskRoundRect(img *ggfx.Image, r Rect, radius float64) {
	b := img.Bounds()
	scale := c.Scale()
	half := Sz(r.Size.W*scale/2, r.Size.H*scale/2)
	op := &ggfx.DrawRectShaderOptions{Uniforms: map[string]any{
		"center":    []float32{float32(c.px(r.Origin.X+r.Size.W/2) - float64(b.Min.X)), float32(c.px(r.Origin.Y+r.Size.H/2) - float64(b.Min.Y))},
		"half_size": []float32{float32(half.W), float32(half.H)},
		"radius":    float32(min(radius*scale, half.W, half.H)),
		"feather":   float32(feather),
	}}
	op.Images[0] = img
	op.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	c.Image.DrawRectShader(b.Dx(), b.Dy(), sharedRoundRectMask(), op)
}
