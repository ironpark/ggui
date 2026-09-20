package ggui

// CachedWidget skips laying its subtree out again while nothing in it has
// changed. Build one with Cached. App already skips the whole layout on a
// still frame. Component, Reactive and Key cache their own subtrees, so a
// signal write in one panel does not
// re-measure every other panel. Cached is for a static subtree that sits
// under something which does rebuild.
//
// The cache holds while the constraints and everything inherited through
// the Env (text style, theme, Scroll viewport, Provide values) are the same
// as last time and nothing inside asked for a layout. Reactive, EachKeyed, Scroll
// and TextInput ask when they change; a custom widget whose size depends on
// state outside a signal calls Invalidate with the Env it was laid out
// under.
type CachedWidget struct {
	child   Widget
	sources map[layoutSource]uint64

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
	// such a thing even when no StateValue was written.
	requestLayout()
	for ; c != nil; c = c.outer {
		c.dirty = true
	}
}

// Layout implements Widget.
func (c *CachedWidget) Layout(cs Constraints, env Env) Size {
	c.outer, _ = env.Get(cacheOwner)
	if c.valid && !c.dirty && cs == c.cons && env.rev == c.rev && c.fontGen == fontGeneration && c.inputsEqual() {
		if measuring != nil {
			for src, version := range c.sources {
				measuring.record(src, version)
			}
		}
		return c.size
	}
	clear(c.sources)
	previous := measuring
	measuring = c
	defer func() { measuring = previous }()
	c.size = c.child.Layout(cs, env.With(cacheOwner, c))
	c.fontGen = fontGeneration
	c.cons, c.rev, c.valid, c.dirty = cs, env.rev, true, false
	return c.size
}

// Paint implements Widget.
func (c *CachedWidget) Paint(dst *Canvas, r Rect) { dst.Paint(c.child, r) }

// Baseline implements Baseliner: the child's.
func (c *CachedWidget) Baseline() (float64, bool) { return baselineOf(c.child) }

// Measurement dependencies are separate from reactive subscriptions: even an
// Untrack read affects layout, but never subscribes the enclosing computation.
type layoutSource interface{ layoutVersion() uint64 }

// measuring is the cache the running Layout measures into, so a source read
// during it is recorded against the right one. Layout is UI-goroutine work,
// and the value is saved and restored around each nested measurement rather
// than assigned outright.
var measuring *CachedWidget

func (c *CachedWidget) record(src layoutSource, version uint64) {
	for ; c != nil; c = c.outer {
		if c.sources == nil {
			c.sources = make(map[layoutSource]uint64)
		}
		c.sources[src] = version
	}
}
func (c *CachedWidget) inputsEqual() bool {
	for src, version := range c.sources {
		if src.layoutVersion() != version {
			return false
		}
	}
	return true
}
