package render

import (
	"image"
	"math"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/geom"
)

// MaxStops is how many colour stops a gradient carries to the shader.
const MaxStops = 16

// The kinds of gradient Paint draws.
const (
	Linear = iota
	Radial
	Sweep
)

// The coverages a gradient is drawn through: a plain rectangle, a rounded
// one from its distance field, or the alpha of a mask image.
const (
	coverRect = iota
	coverRoundRect
	coverMask
)

// gradientSource fills with a gradient through one of three coverages. The
// fragment's place in the destination's pixels rides in custom.xy; a mask
// is read at src_pos. Colours are premultiplied and mixed that way, which
// is what keeps a fade to transparent from turning grey.
const gradientSource = roundRectDistance + `
struct Uniforms {
	colors: array<vec4f, 16>,
	stops: array<vec4f, 16>,
	count: i32,
	kind: i32,
	cover: i32,
	feather: f32,
	start: vec2f,
	end: vec2f,
	center: vec2f,
	half_size: vec2f,
	radius: f32,
}
@group(1) @binding(0) var<uniform> u: Uniforms;

const tau = 6.283185307179586;

fn color_at(t: f32) -> vec4f {
	if (t <= u.stops[0].x) {
		return u.colors[0];
	}
	for (var i = 1; i < u.count; i++) {
		let s = u.stops[i].x;
		if (t <= s) {
			let p = u.stops[i - 1].x;
			var k = 1.0;
			if (s > p) {
				k = (t - p) / (s - p);
			}
			return mix(u.colors[i - 1], u.colors[i], k);
		}
	}
	return u.colors[u.count - 1];
}

fn fragment(v: Vertex) -> vec4f {
	let p = v.custom.xy;
	var t = 0.0;
	if (u.kind == 0) {
		let d = u.end - u.start;
		t = dot(p - u.start, d) / max(dot(d, d), 1e-6);
	} else if (u.kind == 1) {
		t = length((p - u.start) / max(u.end, vec2f(1e-6)));
	} else {
		let q = p - u.start;
		t = fract((atan2(q.y, q.x) - u.end.x) / tau);
	}
	var coverage = 1.0;
	if (u.cover == 1) {
		let distance = round_rect_distance(p - u.center, u.half_size, u.radius);
		coverage = 1.0 - smoothstep(-u.feather, u.feather, distance);
	} else if (u.cover == 2) {
		coverage = src0_at(v.src_pos).a;
	}
	if (coverage <= 0.0) {
		return vec4f(0.0);
	}
	return color_at(t) * coverage;
}
`

var (
	sharedGradient   = compile("gradient shader", gradientSource)
	gradientUniforms = newUniformBlocks(sharedGradient)
)

// Gradient is a gradient in a destination's pixels. Start and End are its
// line for a Linear one; Start is the centre and End the radii for a Radial
// one, and Start the centre and End.X the angle it starts at for a Sweep.
// Colors are premultiplied, one per stop, and Stops are ascending offsets.
type Gradient struct {
	Kind       int
	Start, End geom.Point
	Colors     [][4]float32
	Stops      []float64
}

// set fills u with g, keeping at most MaxStops stops.
func (g *Gradient) set(u *ggfx.Uniforms) {
	n := min(len(g.Colors), len(g.Stops), MaxStops)
	var colors, stops [MaxStops * 4]float32
	for i := range n {
		copy(colors[i*4:], g.Colors[i][:])
		stops[i*4] = float32(g.Stops[i])
	}
	u.SetSlice("colors", colors[:])
	u.SetSlice("stops", stops[:])
	u.Set("count", int32(n))
	u.Set("kind", int32(g.Kind))
	u.Set("feather", float32(Feather))
	u.SetSlice("start", []float32{float32(g.Start.X), float32(g.Start.Y)})
	u.SetSlice("end", []float32{float32(g.End.X), float32(g.End.Y)})
}

// valid reports whether g has a stop and finite geometry.
func (g *Gradient) valid() bool {
	for _, v := range [...]float64{g.Start.X, g.Start.Y, g.End.X, g.End.Y} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return min(len(g.Colors), len(g.Stops)) > 0
}

// FillGradientRoundRect fills the rectangle centred at centre with half size
// half, its corners rounded by radius, with g; a zero radius is a plain
// rectangle, drawn without antialiasing as FillRect draws one.
func FillGradientRoundRect(dst *ggfx.Image, g *Gradient, centre geom.Point, half geom.Size, radius float64) {
	if !g.valid() || half.W <= 0 || half.H <= 0 {
		return
	}
	reach := 0.0
	cover := coverRect
	if radius > 0 {
		reach, cover = Feather+1, coverRoundRect
	}
	bounds, ok := Bounds(dst.Bounds(), centre.X, centre.Y, half.W, half.H, reach)
	if !ok {
		return
	}
	u := gradientUniforms.get()
	defer gradientUniforms.put(u)
	g.set(u)
	u.Set("cover", int32(cover))
	u.SetSlice("center", []float32{float32(centre.X), float32(centre.Y)})
	u.SetSlice("half_size", []float32{float32(half.W), float32(half.H)})
	u.Set("radius", float32(min(max(radius, 0), half.W, half.H)))
	vs := gradientQuad(bounds, image.Point{})
	op := &ggfx.DrawTrianglesShaderOptions{UniformBlock: u}
	dst.DrawTrianglesShader(vs[:], quadIndices[:], sharedGradient(), op)
}

// FillGradientMask fills with g through the alpha of mask, drawn onto dst
// with its first pixel at at.
func FillGradientMask(dst, mask *ggfx.Image, g *Gradient, at image.Point) {
	if !g.valid() {
		return
	}
	b := mask.Bounds()
	bounds := image.Rectangle{Min: at, Max: at.Add(b.Size())}
	u := gradientUniforms.get()
	defer gradientUniforms.put(u)
	g.set(u)
	u.Set("cover", int32(coverMask))
	u.SetSlice("center", []float32{0, 0})
	u.SetSlice("half_size", []float32{0, 0})
	u.Set("radius", float32(0))
	vs := gradientQuad(bounds, b.Min.Sub(at))
	op := &ggfx.DrawTrianglesShaderOptions{UniformBlock: u}
	op.Images[0] = mask
	dst.DrawTrianglesShader(vs[:], quadIndices[:], sharedGradient(), op)
}

// gradientQuad is the quad over bounds whose custom.xy is the destination
// pixel and whose source is that pixel moved by src.
func gradientQuad(bounds image.Rectangle, src image.Point) [4]ggfx.Vertex {
	x0, y0, x1, y1 := float32(bounds.Min.X), float32(bounds.Min.Y), float32(bounds.Max.X), float32(bounds.Max.Y)
	var vs [4]ggfx.Vertex
	for i, c := range [4][2]float32{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		vs[i] = ggfx.Vertex{
			DstX: c[0], DstY: c[1],
			SrcX: c[0] + float32(src.X), SrcY: c[1] + float32(src.Y),
			ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1,
			Custom0: c[0], Custom1: c[1],
		}
	}
	return vs
}
