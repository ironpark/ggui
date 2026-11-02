package ggui

import (
	"slices"
	"sync"

	"github.com/ironpark/ggui/internal/reactive"
)

// CachedWidget skips laying its subtree out again while nothing in it has
// changed. Build one with Cached. App already skips the whole layout on a
// still frame. Component, Reactive and Key cache their own subtrees, so a
// signal write in one panel does not
// re-measure every other panel. Cached is for a static subtree that sits
// under something which does rebuild.
//
// The cache holds while the constraints and everything inherited through
// the Env (text style, Scroll viewport, Provide values) are the same
// as last time and nothing inside asked for a layout. Reactive, EachKeyed, Scroll
// and TextInput ask when they change; a custom widget whose size depends on
// state outside a signal calls Invalidate with the Env it was laid out
// under.
type CachedWidget struct {
	child Widget
	// sources are what the last measurement read itself, each at the
	// version it read last, and kids the caches it laid out, each at the
	// measurement it took: a source read inside a nested cache is that
	// cache's alone, so a source is kept once, not by every cache above it.
	sources []sourceRead
	kids    []kidCache

	outer   *CachedWidget // the nearest Cached above, told when this one is
	dirty   bool
	valid   bool
	cons    Constraints
	fontGen uint64
	rev     uint64 // the Env revision the size was measured under
	size    Size
	gen     uint64 // counts the measurements
	// measuring is set while the cache measures its child.
	measuring bool
}

// mostLateKids is how many caches laid out after a cache measured, as a
// viewport lays out what scrolls into it, it keeps as its own before it
// measures afresh to let go of those gone.
const mostLateKids = 512

// sourceRead is a source a measurement read, at the version it read.
type sourceRead struct {
	src     reactive.LayoutSource
	version uint64
}

// kidCache is a cache laid out within another's measurement, at the
// measurement of its own it gave.
type kidCache struct {
	c   *CachedWidget
	gen uint64
}

var cacheOwner = NewEnvKey[*CachedWidget]("layoutCache")

// Cached wraps child in a layout cache.
func Cached(child Widget) *CachedWidget { return &CachedWidget{child: child} }

// invalidate marks this cache and every one above it stale.
func (c *CachedWidget) invalidate() {
	// The runtime lays out only when something moved; a stale cache is
	// such a thing even when no StateValue was written.
	reactive.RequestLayout()
	for ; c != nil; c = c.outer {
		c.dirty = true
	}
}

// Layout implements Widget.
func (c *CachedWidget) Layout(cs Constraints, env Env) Size {
	c.outer, _ = env.Get(cacheOwner)
	gen := fontGen()
	if c.valid && !c.dirty && cs == c.cons && env.rev == c.rev && c.fontGen == gen && c.inputsEqual() {
		c.tellOuter()
		return c.size
	}
	clear(c.kids)
	c.sources, c.kids = c.sources[:0], c.kids[:0]
	c.measuring = true
	reactive.Measure(c.record, func() { c.size = c.child.Layout(cs, env.With(cacheOwner, c)) })
	c.measuring = false
	c.sources = dedupeReads(c.sources)
	c.kids = slices.Clip(c.kids)
	c.gen++
	c.fontGen = gen
	c.cons, c.rev, c.valid, c.dirty = cs, env.rev, true, false
	c.tellOuter()
	return c.size
}

// tellOuter tells the cache above that this one's size, at its current
// measurement, is part of its own: laid out within it, or after it, as a
// viewport lays out what scrolls into it.
func (c *CachedWidget) tellOuter() {
	o := c.outer
	if o == nil {
		return
	}
	for i := range o.kids {
		if o.kids[i].c == c {
			o.kids[i].gen = c.gen
			return
		}
	}
	if !o.measuring && len(o.kids) >= mostLateKids {
		// Most of those are likely gone by now.
		o.invalidate()
		return
	}
	o.kids = append(o.kids, kidCache{c, c.gen})
}

// Paint implements Widget.
func (c *CachedWidget) Paint(dst *Canvas, r Rect) { dst.Paint(c.child, r) }

// Baseline implements Baseliner: the child's.
func (c *CachedWidget) Baseline() (float64, bool) { return baselineOf(c.child) }

// record notes src as read by this cache's measurement. Layout installs it
// as the reactive core's recorder for the duration of a measurement, saving
// and restoring the previous one so that nested caches nest correctly.
func (c *CachedWidget) record(src reactive.LayoutSource, version uint64) {
	// A widget reports a source at the start of its layout and again at
	// its end: the second replaces the first.
	if n := len(c.sources); n > 0 && c.sources[n-1].src == src {
		c.sources[n-1].version = version
		return
	}
	c.sources = append(c.sources, sourceRead{src, version})
}

// inputsEqual reports whether every source this cache's measurement read,
// itself or through the caches inside, is at the version it read, and those
// caches stand as they were.
func (c *CachedWidget) inputsEqual() bool {
	for _, r := range c.sources {
		if r.src.LayoutVersion() != r.version {
			return false
		}
	}
	for _, k := range c.kids {
		if k.c.gen != k.gen || k.c.dirty || !k.c.valid || !k.c.inputsEqual() {
			return false
		}
	}
	return true
}

// mostScannedReads is how many reads dedupeReads compares pairwise before
// it takes a map to.
const mostScannedReads = 32

// readsSeen are dedupeReads' maps, reused: each window lays out on a
// goroutine of its own.
var readsSeen = sync.Pool{New: func() any { return map[reactive.LayoutSource]int{} }}

// dedupeReads keeps each source once, at the version it was read last, in
// the order it was first read, in a slice no larger than it needs.
func dedupeReads(reads []sourceRead) []sourceRead {
	n := 0
	if len(reads) <= mostScannedReads {
	scan:
		for _, r := range reads {
			for j := range n {
				if reads[j].src == r.src {
					reads[j].version = r.version
					continue scan
				}
			}
			reads[n] = r
			n++
		}
	} else {
		readSeen := readsSeen.Get().(map[reactive.LayoutSource]int)
		defer readsSeen.Put(readSeen)
		for _, r := range reads {
			if j, ok := readSeen[r.src]; ok {
				reads[j].version = r.version
				continue
			}
			readSeen[r.src] = n
			reads[n] = r
			n++
		}
		clear(readSeen)
	}
	clear(reads[n:])
	reads = reads[:n]
	if cap(reads) > 2*n+4 {
		return slices.Clone(reads)
	}
	return reads
}

// Invalidate tells the runtime that the widget laid out under env changed
// size, or its children did: the tree is laid out again next frame and the
// nearest Cached above the widget measures its subtree afresh. A StateValue
// write does the first half by itself; a widget that keeps size-affecting
// state outside signals calls Invalidate when that state changes.
func Invalidate(env Env) {
	reactive.RequestLayout()
	if c, ok := env.Get(cacheOwner); ok {
		c.invalidate()
	}
}
