package ggui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Canvas is what a Widget paints into: the target image plus the frame's list
// of interactive regions. Widgets that react to input register the Rect they
// painted with HitPointer or HitKey; the runtime dispatches the next frame's
// events to those regions, topmost (last painted) first. A nil Canvas paints
// nothing and collects nothing, which is what layout tests want.
type Canvas struct {
	Image *ebiten.Image

	scale   float64
	hits    []hitRegion
	parent  *Canvas // set on a Clip; hit regions go to the root
	clip    Rect
	clipped bool
}

// Scale is the number of Image pixels per logical pixel: the monitor's device
// scale factor on a HiDPI screen, 1 elsewhere, and 1 for a canvas that was
// never given one. Layout and Rects are in logical pixels; anything drawn on
// Image must be scaled by it. FillRect, Geo and Px do that; text scales its
// face size instead.
func (c *Canvas) Scale() float64 {
	if c == nil || c.scale == 0 {
		return 1
	}
	return c.scale
}

// px and dp convert a length between logical and Image pixels. Every
// conversion in the framework goes through this pair.
func (c *Canvas) px(v float64) float64 { return v * c.Scale() }
func (c *Canvas) dp(v float64) float64 { return v / c.Scale() }

// Px converts a logical length or coordinate to Image pixels.
func (c *Canvas) Px(v float64) float32 { return float32(c.px(v)) }

// Geo returns the transform that maps a widget's own logical coordinates,
// with its origin at, onto Image pixels. Use it in DrawImageOptions.
func (c *Canvas) Geo(at Point) ebiten.GeoM {
	var g ebiten.GeoM
	s := c.Scale()
	g.Scale(s, s)
	g.Translate(c.px(at.X), c.px(at.Y))
	return g
}

// FillRect fills the logical Rect r with col.
func (c *Canvas) FillRect(r Rect, col color.Color) {
	if c == nil || c.Image == nil || col == nil {
		return
	}
	vector.FillRect(c.Image, c.Px(r.Origin.X), c.Px(r.Origin.Y), c.Px(r.Size.W), c.Px(r.Size.H), col, true)
}

// FillCircle fills a circle of logical radius around center with col.
func (c *Canvas) FillCircle(center Point, radius float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || radius <= 0 {
		return
	}
	vector.FillCircle(c.Image, c.Px(center.X), c.Px(center.Y), c.Px(radius), col, true)
}

// StrokeLine draws a line of logical width w from a to b in col.
func (c *Canvas) StrokeLine(a, b Point, w float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || w <= 0 {
		return
	}
	vector.StrokeLine(c.Image, c.Px(a.X), c.Px(a.Y), c.Px(b.X), c.Px(b.Y), c.Px(w), col, true)
}

// roundRect traces r with corners of the given logical radius, in Image
// pixels. A zero radius traces a plain rectangle.
func (c *Canvas) roundRect(r Rect, radius float64) *vector.Path {
	x, y, w, h := c.Px(r.Origin.X), c.Px(r.Origin.Y), c.Px(r.Size.W), c.Px(r.Size.H)
	rad := min(c.Px(radius), w/2, h/2)
	var p vector.Path
	p.MoveTo(x+rad, y)
	p.LineTo(x+w-rad, y)
	p.ArcTo(x+w, y, x+w, y+rad, rad)
	p.LineTo(x+w, y+h-rad)
	p.ArcTo(x+w, y+h, x+w-rad, y+h, rad)
	p.LineTo(x+rad, y+h)
	p.ArcTo(x, y+h, x, y+h-rad, rad)
	p.LineTo(x, y+rad)
	p.ArcTo(x, y, x+rad, y, rad)
	p.Close()
	return &p
}

func pathOptions(col color.Color) *vector.DrawPathOptions {
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(col)
	return op
}

// FillRoundRect fills the logical Rect r with col, with corners rounded by
// radius. A zero radius is FillRect.
func (c *Canvas) FillRoundRect(r Rect, radius float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil {
		return
	}
	if radius <= 0 {
		c.FillRect(r, col)
		return
	}
	vector.FillPath(c.Image, c.roundRect(r, radius), &vector.FillOptions{}, pathOptions(col))
}

// StrokeRoundRect draws a line of logical width w in col just inside r,
// with corners rounded by radius.
func (c *Canvas) StrokeRoundRect(r Rect, radius, w float64, col color.Color) {
	if c == nil || c.Image == nil || col == nil || w <= 0 {
		return
	}
	inset := Rct(r.Origin.Add(Pt(w/2, w/2)), Sz(r.Size.W-w, r.Size.H-w))
	vector.StrokePath(c.Image, c.roundRect(inset, max(radius-w/2, 0)), &vector.StrokeOptions{Width: c.Px(w)}, pathOptions(col))
}

// physical returns the Image pixels r covers, rounded outwards.
func (c *Canvas) physical(r Rect) image.Rectangle {
	return image.Rect(
		int(math.Floor(c.px(r.Origin.X))), int(math.Floor(c.px(r.Origin.Y))),
		int(math.Ceil(c.px(r.Origin.X+r.Size.W))), int(math.Ceil(c.px(r.Origin.Y+r.Size.H))),
	)
}

// Clip returns a Canvas that draws only inside r and registers hit regions
// only where they overlap r. Coordinates are unchanged, and clips nest.
func (c *Canvas) Clip(r Rect) *Canvas {
	if c == nil {
		return nil
	}
	child := &Canvas{parent: c, clip: r, clipped: true, scale: c.scale}
	if c.clipped {
		child.clip = c.clip.Intersect(r)
	}
	if c.Image != nil {
		child.Image = c.Image.SubImage(c.physical(child.clip)).(*ebiten.Image)
	}
	return child
}

// add records a hit region, trimmed to the clip, on the root Canvas. A
// region registered at the same Rect as the previous one is merged into it,
// so Pointer(Focus(w)) or a widget that calls HitPointer and HitKey for the
// same Rect is one region with both handlers.
func (c *Canvas) add(h hitRegion) {
	if c == nil {
		return
	}
	if c.clipped {
		h.rect = h.rect.Intersect(c.clip)
		if h.rect.Empty() {
			return
		}
	}
	root := c
	for root.parent != nil {
		root = root.parent
	}
	if n := len(root.hits); n > 0 && root.hits[n-1].merge(h) {
		return
	}
	root.hits = append(root.hits, h)
}

type hitRegion struct {
	rect    Rect
	pointer PointerHandler
	key     KeyHandler
	cursor  ebiten.CursorShapeType
}

// merge folds o into r when they share a Rect and o only adds what r lacks.
func (r *hitRegion) merge(o hitRegion) bool {
	if r.rect != o.rect ||
		(o.pointer != nil && r.pointer != nil) ||
		(o.key != nil && r.key != nil) ||
		(o.cursor != 0 && r.cursor != 0) {
		return false
	}
	if o.pointer != nil {
		r.pointer = o.pointer
	}
	if o.key != nil {
		r.key = o.key
	}
	if o.cursor != 0 {
		r.cursor = o.cursor
	}
	return true
}

// HitPointer registers r as a region that receives pointer events. Regions
// painted later sit on top of earlier ones, so a container registers itself
// before painting its children.
func (c *Canvas) HitPointer(r Rect, h PointerHandler) {
	c.add(hitRegion{rect: r, pointer: h})
}

// HitKey registers r as a region that receives keyboard events while focused.
func (c *Canvas) HitKey(r Rect, h KeyHandler) {
	c.add(hitRegion{rect: r, key: h})
}

// HitCursor asks for the mouse cursor to take shape while it is over r.
func (c *Canvas) HitCursor(r Rect, shape ebiten.CursorShapeType) {
	c.add(hitRegion{rect: r, cursor: shape})
}
