package ggui

import (
	"image"
	"image/color"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Path is a reusable vector outline in logical pixels. Mutating a path invalidates
// its device-scale cache. Like widgets, paths belong to the UI thread.
type Path struct {
	gradientMask   *ebiten.Image
	gradientBounds Rect
	gradientRect   image.Rectangle
	gradientScale  float64
	gradientDirty  bool
	path           vector.Path
	scaled         vector.Path
	scale          float64
	dirty          bool
}

func (p *Path) MoveTo(x, y float64) {
	p.path.MoveTo(float32(x), float32(y))
	p.dirty = true
	p.gradientDirty = true
}
func (p *Path) LineTo(x, y float64) {
	p.path.LineTo(float32(x), float32(y))
	p.dirty = true
	p.gradientDirty = true
}

// QuadTo appends a quadratic Bezier segment.
func (p *Path) QuadTo(x1, y1, x2, y2 float64) {
	p.path.QuadTo(float32(x1), float32(y1), float32(x2), float32(y2))
	p.dirty = true
	p.gradientDirty = true
}
func (p *Path) CubicTo(x1, y1, x2, y2, x3, y3 float64) {
	p.path.CubicTo(float32(x1), float32(y1), float32(x2), float32(y2), float32(x3), float32(y3))
	p.dirty = true
	p.gradientDirty = true
}
func (p *Path) Close() { p.path.Close(); p.dirty = true; p.gradientDirty = true }
func (p *Path) Reset() { p.path.Reset(); p.dirty = true; p.gradientDirty = true }
func (p *Path) device(scale float64) *vector.Path {
	if scale == 1 {
		return &p.path
	}
	if p.dirty || p.scale != scale {
		p.scaled.Reset()
		op := &vector.AddPathOptions{}
		op.GeoM.Scale(scale, scale)
		p.scaled.AddPath(&p.path, op)
		p.scale = scale
		p.dirty = false
	}
	return &p.scaled
}

// FillPath fills a closed outline, honoring the canvas clip and device scale.
func (c *Canvas) FillPath(path *Path, col color.Color) {
	if c == nil || c.Image == nil || path == nil || col == nil {
		return
	}
	vector.FillPath(c.Image, path.device(c.Scale()), nil, pathOptions(col))
}

// StrokePath strokes an outline with round joins and a logical pixel width.
func (c *Canvas) StrokePath(path *Path, width float64, col color.Color) {
	if c == nil || c.Image == nil || path == nil || col == nil || width <= 0 {
		return
	}
	vector.StrokePath(c.Image, path.device(c.Scale()), &vector.StrokeOptions{Width: c.Px(width), LineJoin: vector.LineJoinRound}, pathOptions(col))
}

// FillPathGradient fills a path with a vertical premultiplied color gradient.
// A reusable GPU coverage mask avoids translucent fan-triangulation artifacts.
// The mask is rebuilt only when geometry, bounds or device scale changes.
func (c *Canvas) FillPathGradient(path *Path, bounds Rect, top, bottom color.Color) {
	if c == nil || c.Image == nil || path == nil || top == nil || bottom == nil || bounds.Empty() {
		return
	}
	scale := c.Scale()
	device := path.device(scale)
	coverage := device.Bounds().Intersect(c.physical(bounds))
	w, h := coverage.Dx(), coverage.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	if path.gradientDirty || path.gradientScale != scale || path.gradientBounds != bounds || path.gradientRect != coverage || path.gradientMask == nil {
		if path.gradientMask == nil || path.gradientMask.Bounds().Dx() != w || path.gradientMask.Bounds().Dy() != h {
			if path.gradientMask != nil {
				path.gradientMask.Deallocate()
			}
			path.gradientMask = ebiten.NewImage(w, h)
		} else {
			path.gradientMask.Clear()
		}
		var local vector.Path
		op := &vector.AddPathOptions{}
		op.GeoM.Translate(-float64(coverage.Min.X), -float64(coverage.Min.Y))
		local.AddPath(path.device(scale), op)
		vector.FillPath(path.gradientMask, &local, nil, pathOptions(color.White))
		path.gradientScale = scale
		path.gradientBounds = bounds
		path.gradientRect = coverage
		path.gradientDirty = false
	}
	rgba := func(col color.Color) [4]float32 {
		r, g, b, a := col.RGBA()
		return [4]float32{float32(r) / 65535, float32(g) / 65535, float32(b) / 65535, float32(a) / 65535}
	}
	op := &ebiten.DrawRectShaderOptions{Uniforms: map[string]any{"Top": rgba(top), "Bottom": rgba(bottom), "Height": float32(bounds.Size.H * scale), "Offset": float32(float64(coverage.Min.Y) - bounds.Origin.Y*scale)}}
	op.Images[0] = path.gradientMask
	op.GeoM.Translate(float64(coverage.Min.X), float64(coverage.Min.Y))
	c.Image.DrawRectShader(w, h, sharedPathGradient(), op)
}

const pathGradientSource = `//kage:unit pixels
package main
var Top vec4
var Bottom vec4
var Height float
var Offset float
func Fragment(dst vec4,src vec2,color vec4) vec4 {
 y := src.y-imageSrc0Origin().y+Offset
 return mix(Top,Bottom,clamp(y/Height,0,1))*imageSrc0At(src).a
}
`

var sharedPathGradient = sync.OnceValue(func() *ebiten.Shader {
	shader, err := ebiten.NewShader([]byte(pathGradientSource))
	if err != nil {
		panic("ggui: compile path gradient: " + err.Error())
	}
	return shader
})
