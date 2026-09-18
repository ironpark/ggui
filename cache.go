package ggui

// CachedWidget skips laying its subtree out again while nothing in it has
// changed. Build one with Cached. App already skips the whole layout on a
// still frame; Cached narrows that to a subtree, so a signal write in one
// panel does not re-measure every other panel.
//
// The cache holds while the constraints and everything inherited through
// the Env (text style, theme, Scroll viewport, Provide values) are the same
// as last time and nothing inside asked for a layout. Reactive, For, Scroll
// and TextInput ask when they change; a custom widget whose size depends on
// state outside a signal calls Invalidate with the Env it was laid out
// under.
type CachedWidget struct {
	child Widget

	outer   *CachedWidget // the nearest Cached above, told when this one is
	dirty   bool
	valid   bool
	cons    Constraints
	fontGen uint64
	rev     uint64 // the Env revision the size was measured under
	size    Size
}

var cacheOwner = NewEnvKey[*CachedWidget]("layoutCache")

// Cached wraps child in a layout cache.
func Cached(child Widget) *CachedWidget { return &CachedWidget{child: child} }

// invalidate marks this cache and every one above it stale.
func (c *CachedWidget) invalidate() {
	// The runtime lays out only when something moved; a stale cache is
	// such a thing even when no Signal was written.
	requestLayout()
	for ; c != nil && !c.dirty; c = c.outer {
		c.dirty = true
	}
}

// Layout implements Widget.
func (c *CachedWidget) Layout(cs Constraints, env Env) Size {
	c.outer, _ = env.Get(cacheOwner)
	if c.valid && !c.dirty && cs == c.cons && env.rev == c.rev && c.fontGen == fontGeneration {
		return c.size
	}
	c.size = c.child.Layout(cs, env.With(cacheOwner, c))
	c.fontGen = fontGeneration
	c.cons, c.rev, c.valid, c.dirty = cs, env.rev, true, false
	return c.size
}

// Paint implements Widget.
func (c *CachedWidget) Paint(dst *Canvas, r Rect) { dst.Paint(c.child, r) }
