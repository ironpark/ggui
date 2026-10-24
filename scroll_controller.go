package ggui

import "github.com/ironpark/ggui/internal/reactive"

// ScrollController moves a Scroll from code, eased the way the theme's
// motion is or at once, and scrolls a keyed row into view. The zero value
// is ready; attach it with Scroll.Controller and call it from handlers,
// effects or posted work. A request made before the scroll is first laid
// out is kept until it is.
//
//	var sc ggui.ScrollController
//	ggui.Scroll(list).Controller(&sc)
//	// in a handler:
//	sc.ScrollTo(0)
//	sc.ScrollToKey(todo.ID)
//
// BindOffset still reports and sets the offset; the controller adds the
// animation and the lookup by key.
type ScrollController struct {
	scroll  *ScrollWidget
	pending *scrollRequest
	key     any
	keyed   bool // key is waiting for a list to place it
	animate bool // whether the key's scroll is animated
}

type scrollRequest struct {
	offset  float64
	animate bool
}

// Controller attaches c, which then drives this scroll and every scroll
// rebuilt in its place. Configure it before mount.
func (s *ScrollWidget) Controller(c *ScrollController) *ScrollWidget {
	s.phys.ctrl = c
	if c != nil {
		c.scroll = s
	}
	return s
}

// Offset returns the scroll's offset, or zero while it is not attached.
func (c *ScrollController) Offset() float64 {
	if c.scroll == nil {
		return 0
	}
	return c.scroll.position()
}

// MaxOffset returns the largest offset the content allows, as of the last
// layout, or zero before one.
func (c *ScrollController) MaxOffset() float64 {
	if c.scroll == nil || !c.scroll.laidOut {
		return 0
	}
	return c.scroll.maxOffset()
}

// Animating reports whether a scroll the controller started is under way.
func (c *ScrollController) Animating() bool { return c.scroll != nil && c.scroll.phys.animating }

// JumpTo moves the window to offset v at once, clamped to the content.
func (c *ScrollController) JumpTo(v float64) { c.request(v, false) }

// ScrollTo eases the window to offset v, clamped to the content: 0 is the
// start and math.Inf(1) the end. Wheel or touch input takes over from it.
// Under reduced motion it jumps.
func (c *ScrollController) ScrollTo(v float64) { c.request(v, true) }

// ScrollBy eases the window by d from where the scroll is heading, so
// repeated calls add up.
func (c *ScrollController) ScrollBy(d float64) {
	base := c.Offset()
	if r := c.pending; r != nil {
		base = r.offset
	} else if c.scroll != nil && c.scroll.phys.animating {
		base = c.scroll.phys.anim.to
	}
	c.request(base+d, true)
}

// ScrollToKey eases the window until the row of an EachKeyed list keyed k
// sits at its start, or as near as the content allows. The list inside the
// scroll places the row when it is next painted, so a row a virtualized
// list has not built yet is found too. k must have the list's key type.
func (c *ScrollController) ScrollToKey(k any) {
	c.key, c.keyed, c.animate = k, true, true
	c.pending = nil
	reactive.RequestLayout()
}

// JumpToKey is ScrollToKey without the animation.
func (c *ScrollController) JumpToKey(k any) {
	c.ScrollToKey(k)
	c.animate = false
}

func (c *ScrollController) request(v float64, animate bool) {
	c.keyed, c.key = false, nil
	if c.scroll == nil || !c.scroll.phys.painted {
		c.pending = &scrollRequest{offset: v, animate: animate}
		reactive.RequestLayout()
		return
	}
	c.pending = nil
	c.scroll.animateTo(v, animate)
}

// placeKey is how a list inside s answers a ScrollToKey: match reports
// whether the waiting key is one of its rows and, if so, where that row
// starts in window coordinates along the axis.
func (s *ScrollWidget) placeKey(match func(k any) (float64, bool)) {
	if !s.keyPending() {
		return
	}
	c := s.phys.ctrl
	at, ok := match(c.key)
	if !ok {
		return
	}
	c.keyed, c.key = false, nil
	start := pick(s.horizontal, s.rect.Origin.X, s.rect.Origin.Y)
	// at is where the row is drawn now; the content moved by the visible
	// offset to put it there.
	s.animateTo(s.visualOffset()+at-start, c.animate)
}

// keyPending reports whether a ScrollToKey on s waits for a list to place
// its key, which a list checks before it builds the match.
func (s *ScrollWidget) keyPending() bool {
	c := s.phys.ctrl
	return c != nil && c.keyed && c.scroll == s
}
