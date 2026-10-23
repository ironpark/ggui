package render

import (
	"image"
	"math"

	"github.com/ironpark/ggfx"
)

// blurSource is one pass of a separable Gaussian blur: taps either side of
// each pixel along dir, weighted by the bell of sigma. A blur in two passes,
// one along each axis, is the same as the full kernel at a fraction of the
// reads. With clamp_edge set, a tap past the image reads its nearest edge
// pixel instead of transparency, so a backdrop does not darken at its rim.
const blurSource = `
struct Uniforms {
	dir: vec2f,
	sigma: f32,
	taps: i32,
	clamp_edge: i32,
}
@group(1) @binding(0) var<uniform> u: Uniforms;

fn tap(p: vec2f) -> vec4f {
	if (u.clamp_edge != 0) {
		return src0_at(clamp(p, src0_origin() + 0.5, src0_origin() + src0_size() - 0.5));
	}
	return src0_at(p);
}

fn fragment(v: Vertex) -> vec4f {
	var sum = tap(v.src_pos);
	var total = 1.0;
	let k = -0.5 / (u.sigma * u.sigma);
	for (var i = 1; i <= u.taps; i++) {
		let w = exp(f32(i * i) * k);
		let o = u.dir * f32(i);
		sum += (tap(v.src_pos + o) + tap(v.src_pos - o)) * w;
		total += 2.0 * w;
	}
	return sum / total;
}
`

var (
	sharedBlur   = compile("blur shader", blurSource)
	blurUniforms = newUniformBlocks(sharedBlur)
)

// BlurSigma is the most a single pass blurs, in pixels of the image it
// reads; a wider blur is done on a smaller copy and scaled back up, which
// the eye cannot tell from the full one.
const BlurSigma = 4

// BlurFactor returns how many times smaller to blur an image by sigma
// pixels so that each pass stays within BlurSigma.
func BlurFactor(sigma float64) int {
	if sigma <= BlurSigma || math.IsNaN(sigma) {
		return 1
	}
	return int(math.Ceil(sigma / BlurSigma))
}

// BlurPass draws src blurred by sigma pixels along one axis, horizontal or
// not, onto dst at src's own place.
func BlurPass(dst, src *ggfx.Image, sigma float64, horizontal, clampEdge bool) {
	b := src.Bounds()
	if b.Empty() {
		return
	}
	u := blurUniforms.get()
	defer blurUniforms.put(u)
	dir := []float32{0, 1}
	if horizontal {
		dir = []float32{1, 0}
	}
	u.SetSlice("dir", dir)
	u.Set("sigma", float32(max(sigma, 1e-3)))
	u.Set("taps", int32(min(math.Ceil(sigma*3), 3*BlurSigma)))
	u.SetBool("clamp_edge", clampEdge)
	// The pass replaces what dst held, so a scratch image is reused
	// between passes without clearing.
	op := &ggfx.DrawRectShaderOptions{UniformBlock: u, Blend: ggfx.BlendCopy}
	op.Images[0] = src
	op.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	dst.DrawRectShader(b.Dx(), b.Dy(), sharedBlur(), op)
}

// Shrunk returns the part of an image covering area that holds area made k
// times smaller, from the same corner.
func Shrunk(area image.Rectangle, k int) image.Rectangle {
	size := area.Size()
	return image.Rectangle{Min: area.Min, Max: area.Min.Add(image.Pt((size.X+k-1)/k, (size.Y+k-1)/k))}
}
