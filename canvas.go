package ggui

import "github.com/hajimehoshi/ebiten/v2"

// Canvas is what a Widget paints into: the target image plus the frame's list
// of interactive regions. Widgets that react to input register the Rect they
// painted with Hit; the runtime dispatches the next frame's events to those
// regions, topmost (last painted) first.
type Canvas struct {
	Image *ebiten.Image

	hits *[]hitRegion // nil outside the runtime, e.g. in tests
}

type hitRegion struct {
	rect    Rect
	pointer PointerHandler
	key     KeyHandler
}

// Hit registers r as an interactive region. handler must implement
// PointerHandler, KeyHandler or both. Regions painted later sit on top of
// earlier ones, so a container registers itself before painting its children.
func (c *Canvas) Hit(r Rect, handler any) {
	if c == nil || c.hits == nil {
		return
	}
	h := hitRegion{rect: r}
	h.pointer, _ = handler.(PointerHandler)
	h.key, _ = handler.(KeyHandler)
	if h.pointer == nil && h.key == nil {
		panic("ggui: Hit handler implements neither PointerHandler nor KeyHandler")
	}
	*c.hits = append(*c.hits, h)
}
