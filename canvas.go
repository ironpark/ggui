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

	// Scale is the number of Image pixels per logical pixel: the monitor's
	// device scale factor on a HiDPI screen, 1 elsewhere. Layout and Rects
	// are in logical pixels; anything drawn on Image must be scaled by it.
	// FillRect, Geo and Px do that; text scales its face size instead.
	Scale float64

	hits    []hitRegion
	parent  *Canvas // set on a Clip; hit regions go to the root
	clip    Rect
	clipped bool
}

func (c *Canvas) scale() float64 {
	if c == nil || c.Scale == 0 {
		return 1
	}
	return c.Scale
}

// Px converts a logical length or coordinate to Image pixels.
func (c *Canvas) Px(v float64) float32 { return float32(v * c.scale()) }

// Geo returns the transform that maps a widget's own logical coordinates,
// with its origin at, onto Image pixels. Use it in DrawImageOptions.
func (c *Canvas) Geo(at Point) ebiten.GeoM {
	var g ebiten.GeoM
	s := c.scale()
	g.Scale(s, s)
	g.Translate(at.X*s, at.Y*s)
	return g
}

// FillRect fills the logical Rect r with col.
func (c *Canvas) FillRect(r Rect, col color.Color) {
	if c == nil || c.Image == nil || col == nil {
		return
	}
	vector.DrawFilledRect(c.Image, c.Px(r.Origin.X), c.Px(r.Origin.Y), c.Px(r.Size.W), c.Px(r.Size.H), col, true)
}

// physical returns the Image pixels r covers, rounded outwards.
func (c *Canvas) physical(r Rect) image.Rectangle {
	s := c.scale()
	return image.Rect(
		int(math.Floor(r.Origin.X*s)), int(math.Floor(r.Origin.Y*s)),
		int(math.Ceil((r.Origin.X+r.Size.W)*s)), int(math.Ceil((r.Origin.Y+r.Size.H)*s)),
	)
}

// Clip returns a Canvas that draws only inside r and registers hit regions
// only where they overlap r. Coordinates are unchanged, and clips nest.
func (c *Canvas) Clip(r Rect) *Canvas {
	if c == nil {
		return nil
	}
	child := &Canvas{parent: c, clip: r, clipped: true, Scale: c.Scale}
	if c.clipped {
		child.clip = c.clip.Intersect(r)
	}
	if c.Image != nil {
		child.Image = c.Image.SubImage(c.physical(child.clip)).(*ebiten.Image)
	}
	return child
}

// add records a hit region, trimmed to the clip, on the root Canvas.
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
	root.hits = append(root.hits, h)
}

type hitRegion struct {
	rect    Rect
	pointer PointerHandler
	key     KeyHandler
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
