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
const feather = 0.5; // the Go constant feather

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
// rectangle centred on the origin, rounded by radius; both shapes use it.
const roundRectDistance = `
fn round_rect_distance(p: vec2f, half_size: vec2f, radius: f32) -> f32 {
	let q = abs(p) - half_size + vec2f(radius);
	return length(max(q, vec2f(0.0))) + min(max(q.x, q.y), 0.0) - radius;
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
	// A corner never exceeds half the shape, which is a circle.
	radius := float32(min(max(f.radius, 0), f.half.W, f.half.H))
	width := float32(max(f.stroke, 0))
	var vs [4]ggfx.Vertex
	for i, corner := range corners(bounds) {
		p := Pt(corner.X-f.centre.X, corner.Y-f.centre.Y)
		vs[i] = ggfx.Vertex{
			DstX: float32(corner.X), DstY: float32(corner.Y),
			SrcX: radius, SrcY: width,
			ColorR: float32(red) / 65535, ColorG: float32(green) / 65535, ColorB: float32(blue) / 65535, ColorA: float32(alpha) / 65535,
			Custom0: float32(p.X*f.axis.X + p.Y*f.axis.Y), Custom1: float32(p.Y*f.axis.X - p.X*f.axis.Y),
			Custom2: float32(f.half.W), Custom3: float32(f.half.H),
		}
	}
	c.Image.DrawTrianglesShader(vs[:], quadIndices[:], sharedRoundRect(), nil)
}

// quadIndices are the two triangles of the quad corners returns.
var quadIndices = [...]uint16{0, 1, 2, 1, 3, 2}

// corners returns the corners of r: top left, top right, bottom left,
// bottom right.
func corners(r image.Rectangle) [4]Point {
	x0, y0, x1, y1 := float64(r.Min.X), float64(r.Min.Y), float64(r.Max.X), float64(r.Max.Y)
	return [4]Point{Pt(x0, y0), Pt(x1, y0), Pt(x0, y1), Pt(x1, y1)}
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

var sharedRoundRectMask = sync.OnceValue(func() *ggfx.Shader {
	shader, err := ggfx.NewShader([]byte(roundRectMaskSource))
	if err != nil {
		panic("ggui: compile round rect mask shader: " + err.Error())
	}
	return shader
})

var roundRectMaskUniforms = newUniformBlocks(sharedRoundRectMask)

// maskRoundRect draws img, a part of a layer, onto c where it lies within
// the logical Rect r with corners rounded by radius.
func (c *Canvas) maskRoundRect(img *ggfx.Image, r Rect, radius float64) {
	b := img.Bounds()
	scale := c.Scale()
	half, centre := Sz(r.Size.W*scale/2, r.Size.H*scale/2), r.Center()
	u := roundRectMaskUniforms.get()
	defer roundRectMaskUniforms.put(u)
	u.SetSlice("center", []float32{float32(c.px(centre.X) - float64(b.Min.X)), float32(c.px(centre.Y) - float64(b.Min.Y))})
	u.SetSlice("half_size", []float32{float32(half.W), float32(half.H)})
	u.Set("radius", float32(min(radius*scale, half.W, half.H)))
	u.Set("feather", float32(feather))
	op := &ggfx.DrawRectShaderOptions{UniformBlock: u}
	op.Images[0] = img
	op.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	c.Image.DrawRectShader(b.Dx(), b.Dy(), sharedRoundRectMask(), op)
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

var sharedRoundRectImage = sync.OnceValue(func() *ggfx.Shader {
	shader, err := ggfx.NewShader([]byte(roundRectImageSource))
	if err != nil {
		panic("ggui: compile round rect image shader: " + err.Error())
	}
	return shader
})

var roundRectImageUniforms = newUniformBlocks(sharedRoundRectImage)

// shadeImage draws img placed at the logical Rect at, cut to r with its
// corners rounded by o.Radius, in one draw: a quad over r and a pixel of
// edge, whose source positions follow at.
func (c *Canvas) shadeImage(img *ggfx.Image, r, at Rect, o ImageOptions) {
	scale := c.Scale()
	reach := 1 / scale
	if o.Rotation != 0 {
		reach += math.Hypot(r.Size.W, r.Size.H)/2 - min(r.Size.W, r.Size.H)/2
	}
	if at.Size.W <= 0 || at.Size.H <= 0 || !c.visiblePaintBounds(r, reach) {
		return
	}
	b := img.Bounds()
	kx, ky := float64(b.Dx())/at.Size.W, float64(b.Dy())/at.Size.H
	centre := r.Center()
	sin, cos := math.Sincos(o.Rotation)
	red, green, blue, alpha := float32(1), float32(1), float32(1), float32(1)
	if o.Tint != nil {
		cr, cg, cb, ca := o.Tint.RGBA()
		red, green, blue, alpha = float32(cr)/0xffff, float32(cg)/0xffff, float32(cb)/0xffff, float32(ca)/0xffff
	}
	if o.Fade != 0 {
		k := float32(1 - o.Fade)
		red, green, blue, alpha = red*k, green*k, blue*k, alpha*k
	}
	e := 1 / scale // one device pixel of edge to antialias into
	var vs [4]ggfx.Vertex
	for i, p := range [4]Point{
		{X: r.Origin.X - e, Y: r.Origin.Y - e}, {X: r.Origin.X + r.Size.W + e, Y: r.Origin.Y - e},
		{X: r.Origin.X - e, Y: r.Origin.Y + r.Size.H + e}, {X: r.Origin.X + r.Size.W + e, Y: r.Origin.Y + r.Size.H + e},
	} {
		d := Pt(p.X-centre.X, p.Y-centre.Y)
		turned := centre.Add(Pt(d.X*cos-d.Y*sin, d.X*sin+d.Y*cos))
		vs[i] = ggfx.Vertex{
			DstX: float32(c.px(turned.X)), DstY: float32(c.px(turned.Y)),
			SrcX: float32(float64(b.Min.X) + (p.X-at.Origin.X)*kx), SrcY: float32(float64(b.Min.Y) + (p.Y-at.Origin.Y)*ky),
			ColorR: red, ColorG: green, ColorB: blue, ColorA: alpha,
			Custom0: float32(d.X * scale), Custom1: float32(d.Y * scale),
		}
	}
	half := Sz(r.Size.W*scale/2, r.Size.H*scale/2)
	u := roundRectImageUniforms.get()
	defer roundRectImageUniforms.put(u)
	u.SetSlice("half_size", []float32{float32(half.W), float32(half.H)})
	u.Set("radius", float32(min(o.Radius*scale, half.W, half.H)))
	u.Set("feather", float32(feather))
	u.SetBool("pixelated", o.Pixelated)
	op := &ggfx.DrawTrianglesShaderOptions{UniformBlock: u}
	op.Images[0] = img
	c.Image.DrawTrianglesShader(vs[:], []uint16{0, 1, 2, 1, 3, 2}, sharedRoundRectImage(), op)
}
