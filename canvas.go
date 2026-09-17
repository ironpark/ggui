package ggui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Canvas is what a Widget paints into: the target image plus the frame's list
// of interactive regions. Widgets that react to input register the Rect they
// painted with HitPointer or HitKey; the runtime dispatches the next frame's
// events to those regions, topmost (last painted) first. A nil Canvas paints
// nothing and collects nothing, which is what layout tests want.
type Canvas struct {
	Image *ebiten.Image

	hits    []hitRegion
	parent  *Canvas // set on a Clip; hit regions go to the root
	clip    Rect
	clipped bool
}

// Clip returns a Canvas that draws only inside r and registers hit regions
// only where they overlap r. Coordinates are unchanged, and clips nest.
func (c *Canvas) Clip(r Rect) *Canvas {
	if c == nil {
		return nil
	}
	child := &Canvas{parent: c, clip: r, clipped: true}
	if c.clipped {
		child.clip = c.clip.Intersect(r)
	}
	if c.Image != nil {
		b := child.clip
		child.Image = c.Image.SubImage(image.Rect(
			int(math.Floor(b.Origin.X)), int(math.Floor(b.Origin.Y)),
			int(math.Ceil(b.Origin.X+b.Size.W)), int(math.Ceil(b.Origin.Y+b.Size.H)),
		)).(*ebiten.Image)
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
