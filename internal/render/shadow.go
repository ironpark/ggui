package render

import (
	"image"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/geom"
)

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

var sharedShadow = compile("shadow shader", shadowSource)

// Shadow is a soft rounded-rectangle silhouette in a target's pixels:
// Bounds is the part of the target it tints, found with the package's
// Bounds, and Centre, Half and Radius the shape it feathers Feather to
// either side of.
type Shadow struct {
	Bounds          image.Rectangle
	Centre          geom.Point
	Half            geom.Size
	Radius, Feather float32
}

// DrawShadow draws s onto dst in tint, a premultiplied colour.
func DrawShadow(dst *ggfx.Image, s Shadow, tint [4]float32) {
	vs := quad(s.Bounds, s.Centre, geom.Pt(1, 0), s.Half, s.Radius, s.Feather, tint)
	dst.DrawTrianglesShader(vs[:], quadIndices[:], sharedShadow(), nil)
}
