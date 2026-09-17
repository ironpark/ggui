package ggui

// CachedWidget skips laying its subtree out again while nothing in it has
// changed. Build one with Cached. App already skips the whole layout on a
// still frame; Cached narrows that to a subtree, so a signal write in one
// panel does not re-measure every other panel.
//
// The cache holds while the constraints, the inherited text style, the
// theme and the enclosing Scroll's viewport are the same as last time and
// nothing inside asked for a layout. Reactive, For, Scroll and TextInput
// ask when they change; a custom widget whose size depends on state
// outside a signal calls InvalidateLayout with the Env it was laid out
// under.
type CachedWidget struct {
	child Widget

	outer *CachedWidget // the nearest Cached above, told when this one is
	dirty bool
	valid bool
	cons  Constraints
	key   cacheKey
	size  Size
}

// cacheKey is the part of an Env a layout can depend on.
type cacheKey struct {
	text  TextStyle
	theme Theme
	vp    Viewport
	hasVP bool
}

var cacheOwner = NewKey[*CachedWidget]("layoutCache")

// Cached wraps child in a layout cache.
func Cached(child Widget) *CachedWidget { return &CachedWidget{child: child} }

// invalidate marks this cache and every one above it stale.
func (c *CachedWidget) invalidate() {
	for ; c != nil && !c.dirty; c = c.outer {
		c.dirty = true
	}
}

// InvalidateLayout tells the nearest Cached above the widget laid out
// under env that its subtree must be measured again. It is a no-op when
// there is none; RequestLayout is still needed for App to run a layout at
// all when no signal was written.
func InvalidateLayout(env Env) {
	if c, ok := env.Get(cacheOwner); ok {
		c.invalidate()
	}
}

// Layout implements Widget.
func (c *CachedWidget) Layout(cs Constraints, env Env) Size {
	vp, hasVP := env.Get(viewportKey)
	key := cacheKey{text: env.Text(), theme: env.Theme(), vp: vp, hasVP: hasVP}
	c.outer, _ = env.Get(cacheOwner)
	if c.valid && !c.dirty && cs == c.cons && key == c.key {
		return c.size
	}
	c.size = c.child.Layout(cs, env.With(cacheOwner, c))
	c.cons, c.key, c.valid, c.dirty = cs, key, true, false
	return c.size
}

// Paint implements Widget.
func (c *CachedWidget) Paint(dst *Canvas, r Rect) { dst.Paint(c.child, r) }
