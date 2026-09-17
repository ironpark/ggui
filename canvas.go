package ggui

import "github.com/hajimehoshi/ebiten/v2"

// Canvas is what a Widget paints into: the target image plus the frame's list
// of interactive regions. Widgets that react to input register the Rect they
// painted with HitPointer or HitKey; the runtime dispatches the next frame's
// events to those regions, topmost (last painted) first. A nil Canvas paints
// nothing and collects nothing, which is what layout tests want.
type Canvas struct {
	Image *ebiten.Image

	hits []hitRegion
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
	if c != nil {
		c.hits = append(c.hits, hitRegion{rect: r, pointer: h})
	}
}

// HitKey registers r as a region that receives keyboard events while focused.
func (c *Canvas) HitKey(r Rect, h KeyHandler) {
	if c != nil {
		c.hits = append(c.hits, hitRegion{rect: r, key: h})
	}
}
