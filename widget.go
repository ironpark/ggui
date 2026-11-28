package ggui

import (
	"math"
	"slices"
	"sync"

	"github.com/ironpark/ggui/geom"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/layout"
)

// Widget is the unit of composition. Layout asks it for a size under some
// Constraints; Paint then hands it a Canvas and the Rect its parent assigned:
// the origin the parent chose and the size Layout returned. Layout always runs
// before Paint in a frame, so a widget keeps only what the Rect cannot tell
// it, such as where its children go. Leaf widgets usually keep nothing at all.
//
// Value properties support runtime assignment; BindX connects a reader and X
// replaces it with a literal. Structure, identity and construction modes are
// configured before mount. Custom parents can set child literals before Layout;
// custom non-reactive data affecting size requires Invalidate on its layout Env.
type Widget interface {
	Layout(c Constraints, env Env) Size
	Paint(dst *Canvas, r Rect)
}

// Builder constructs a widget tree. App and Component setup run once;
// View, If, Key and Each rerun a builder when the value they follow changes.
// A builder runs untracked: a signal it reads itself is a snapshot, and a
// ggui_debug build reports each place that reads one. Reactive alone reruns
// on whatever its builder reads.
type Builder func() Widget

// ComponentWidget is a subtree with its own rebuild boundary. Build one with
// Component, Key or Reactive.
type ComponentWidget struct {
	child Widget
	// A rebuild boundary is a layout boundary too: the subtree is measured
	// again when this component rebuilds, when the constraints or the
	// inherited Env change, or when something inside calls Invalidate.
	// Without it every StateValue write re-measured the whole tree, since the
	// runtime lays out from the root whenever anything was written.
	cw    CachedWidget
	mount func() // runs setup at the first Layout; nil once mounted

	// For the inspector: how many times Reactive built the subtree, and in
	// a ggui_debug build where the Reactive or View was made.
	builds int
	origin string
}

// Component runs setup once when mounted. Reads in setup are untracked;
// use bindings, View, Reactive or control-flow blocks for later changes.
func Component(setup func() Widget) *ComponentWidget {
	c := &ComponentWidget{}
	owner := reactive.CurrentOwner()
	c.mount = func() {
		if owner != nil && owner.Disposed() {
			return
		}
		if owner == nil {
			owner = reactive.CurrentOwner()
		}
		// Disposed with the owner: a component is not unmounted on its own.
		c.child, _ = Scope{owner}.MountAs(c, "", setup)
		c.cw.child = c.child
	}
	return c
}

// Reactive gives build an effect of its own: when the signals it reads
// change, this subtree rebuilds and the parent does not. Use it to keep a
// parent's Builder static so the components it holds survive.
func Reactive[W Widget](build func() W) *ComponentWidget {
	c := &ComponentWidget{origin: reactive.Origin()}
	b := newBoundary()
	reactive.Observe(func() {
		c.builds++
		c.child = b.build(func() Widget { return build() })
		c.cw.child = c.child
		c.cw.invalidate()
	})
	return c
}

// View builds a widget from a reactive value and rebuilds it when the value
// changes, with the widget type inferred from the constructor. Only r is
// followed: build runs untracked like every other builder, so what it reads
// itself is a snapshot, and a value that changes is bound instead.
//
//	ggui.View(label, widget.Text)
//	ggui.View(rows, func(r []Row) *widget.ColumnWidget { return widget.List(r, rowWidget) })
func View[T any, W Widget](r Readable[T], build func(T) W) Widget {
	return Reactive(func() Widget {
		v := r.Get()
		return reactive.Build(func() Widget { return build(v) })
	})
}

// ViewOf builds a widget from the part of r that sel picks and rebuilds it
// only when that part changes, not on every change to the rest of r:
//
//	ggui.ViewOf(doc, func(d Doc) Selection { return d.Selection }, inspector)
//
// The part is compared the way a StateValue compares its value: with its
// Equal method or ==, so pick a comparable part, or a Map with WithEqual
// for one holding slices or maps. It is View over Map, with the Map made
// once, outside the rebuilds.
func ViewOf[T, K any, W Widget](r Readable[T], sel func(T) K, build func(K) W) Widget {
	return View(Map(r, sel), build)
}

// IfWidget mounts only the selected branch and disposes it on exit.
type IfWidget struct {
	comp     *ComponentWidget
	branches []ifBranch
	other    func() Widget
	mounted  bool
}
type ifBranch struct {
	cond Readable[bool]
	then func() Widget
}

// If creates a conditional block. Finish configuring its branches before mount.
func If(cond Readable[bool], then func() Widget) *IfWidget {
	w := &IfWidget{branches: []ifBranch{{cond, then}}}
	w.comp = Component(func() Widget {
		w.mounted = true
		selected := Derived(func() int {
			for i, branch := range w.branches {
				if branch.cond.Get() {
					return i
				}
			}
			return len(w.branches)
		})
		return Key(selected, func(index int) Widget {
			if index == len(w.branches) {
				if w.other != nil {
					return w.other()
				}
				return nil
			}
			return w.branches[index].then()
		})
	})
	return w
}
func (w *IfWidget) ElseIf(cond Readable[bool], then func() Widget) *IfWidget {
	if w.mounted {
		panic("ggui: If configured after mount")
	}
	w.branches = append(w.branches, ifBranch{cond, then})
	return w
}
func (w *IfWidget) Else(other func() Widget) *IfWidget {
	if w.mounted {
		panic("ggui: If configured after mount")
	}
	w.other = other
	return w
}
func (w *IfWidget) current() Widget { return blockChild(w.comp) }

// Vacant implements Vacant: the branch it is on builds nothing.
func (w *IfWidget) Vacant() bool { return w.current() == nil }

// blockChild unwraps component boundaries, including an empty branch.
func blockChild(w Widget) Widget {
	for {
		c, ok := w.(*ComponentWidget)
		if !ok {
			return w
		}
		w = c.child
	}
}

// Key recreates a subtree whenever its key changes. The branch factory is
// untracked and runs once per key; it owns all computations it creates.
func Key[K comparable](key Readable[K], build func(K) Widget) *ComponentWidget {
	return Component(func() Widget {
		c := &ComponentWidget{}
		var dispose Cleanup
		scope := UseScope()
		OnCleanup(func() {
			if dispose != nil {
				dispose()
			}
		})
		Follow(key, func(value K) {
			if dispose != nil {
				dispose()
				dispose = nil
			}
			c.child, dispose = scope.Mount(func() Widget { return build(value) })
			c.cw.child = c.child
			c.Invalidate()
		})
		return c
	})
}

// Follow calls fn with key's value now, before it returns, and again
// whenever key changes to a value other than the last fn was given,
// compared with ==, until stop is called or the builder or effect it was
// called in is disposed. fn runs untracked: what it reads itself does not
// call it again. Unlike Watch, whose first call waits for the frame's
// effects, after layout, a builder that mounts what the key picks with it
// has that mounted by its first layout; Key and Switcher follow their keys
// with it.
func Follow[K comparable](key Readable[K], fn func(K)) (stop func()) {
	var previous K
	var initialized bool
	return reactive.Observe(func() {
		v := key.Get()
		if initialized && v == previous {
			return
		}
		previous, initialized = v, true
		Untrack(func() struct{} { fn(v); return struct{}{} })
	})
}

// A Scope is where a builder runs: what is mounted in it, a subtree a Key
// branch or an EachKeyed row builds, is disposed with it, and before then
// whenever the Cleanup Mount returned is called. A widget that mounts
// subtrees of its own as it goes, keyed by a value or by the rows of a
// list, takes the Scope of the builder that made it with UseScope and
// mounts each in it, as Key does:
//
//	comp = ggui.Component(func() ggui.Widget {
//		scope := ggui.UseScope()
//		ggui.Follow(page, func(p Page) {
//			if dispose != nil {
//				dispose()
//			}
//			shown, dispose = scope.Mount(func() ggui.Widget { return view(p) })
//			comp.Invalidate()
//		})
//		return body
//	})
//
// The zero Scope belongs to nothing: what is mounted in it lives until its
// Cleanup is called.
type Scope struct {
	owner *reactive.Computation
}

// UseScope returns the Scope of the builder or effect running now: a
// Component's setup, a Key branch, an EachKeyed row, an App's builder.
// Outside every one it returns the zero Scope.
func UseScope() Scope { return Scope{reactive.CurrentOwner()} }

// Mount builds a subtree with build, under s, and returns it with the
// Cleanup that disposes it. build runs as a Key branch does: untracked, as
// the root of computations of its own, with a Remember memory of its own
// that goes when the subtree does, and with an identity of its own, so the
// controls of a subtree mounted again for the same key start afresh rather
// than taking the state of those the last mount built. What was mounted
// before stays mounted until its own Cleanup is called: a Switcher keeps
// the subtree it switched from while it fades out.
func (s Scope) Mount(build func() Widget) (Widget, Cleanup) {
	// A unique root gives remounted controls fresh identities.
	return s.MountAs(new(int), "", build)
}

// MountAs is Mount with the identity the controls in the subtree derive
// theirs from given, which a subtree mounted again with the same id keeps,
// and with name, which the subtree's computations are listed under in a
// ggui_debug build and the inspector. EachKeyed mounts each row as the
// entry it keeps for it, named by its key: a row whose key comes back after
// it was disposed is a new entry, and starts afresh.
func (s Scope) MountAs(id any, name string, build func() Widget) (child Widget, dispose Cleanup) {
	reactive.WithOwner(s.owner, func() {
		dispose = reactive.RootWith(id, name, func() {
			child = remembering(build)
		})
	})
	return child, dispose
}

// Invalidate marks the component's own layout cache stale, and every one
// above it, and asks for a layout: for a widget that changed what the
// component holds without the component rebuilding, as a Switcher does when
// it mounts the subtree of a new key beside the one leaving.
func (c *ComponentWidget) Invalidate() { c.cw.invalidate() }

// Vacant is a widget that may show nothing, as an If whose condition is
// false does: Vacant reports that it currently does, and a Row, Column or
// Wrap around it then leaves out the gap it would place beside it, as if
// it were not there. It is asked after the widget's Layout, which is when
// an If knows its branch.
type Vacant interface {
	Vacant() bool
}

// IsVacant reports whether w is a Vacant that currently shows nothing.
func IsVacant(w Widget) bool {
	v, ok := w.(Vacant)
	return ok && v.Vacant()
}

// Layout implements Widget. Showing nothing, it takes no space at all.
func (w *IfWidget) Layout(c Constraints, env Env) Size {
	size := w.comp.Layout(c, env)
	if w.comp.child == nil {
		return Size{}
	}
	return size
}

// Paint implements Widget. The component paints directly, so the inspector
// lists the branch under the If rather than under an extra Component.
func (w *IfWidget) Paint(dst *Canvas, r Rect) { w.comp.Paint(dst, r) }

// Layout implements Widget.
func (c *ComponentWidget) Layout(cs Constraints, env Env) Size {
	c.cw.outer, _ = env.Get(cacheOwner)
	if c.mount != nil {
		c.mount()
		c.mount = nil
	}
	if c.child == nil {
		return cs.Constrain(Size{})
	}
	return c.cw.Layout(cs, env)
}

// Baseline implements Baseliner: the mounted child's.
func (c *ComponentWidget) Baseline() (float64, bool) { return BaselineOf(c.child) }

// Baseline implements Baseliner: the shown branch's.
func (w *IfWidget) Baseline() (float64, bool) { return w.comp.Baseline() }

// Paint implements Widget.
func (c *ComponentWidget) Paint(dst *Canvas, r Rect) {
	if c.child != nil {
		dst.Paint(&c.cw, r)
	}
}

// Children builds one Widget per item. It is the bridge from data to tree for
// any widget that takes children:
//
//	Column(Children(rows, func(r Row) Widget { return rowWidget(r) })...)
func Children[T any](items []T, build func(T) Widget) []Widget {
	out := make([]Widget, len(items))
	for i, item := range items {
		out[i] = build(item)
	}
	return out
}

type widgetFunc struct {
	layout func(Constraints, Env) Size
	paint  func(*Canvas, Rect)
}

func (w widgetFunc) Layout(c Constraints, env Env) Size { return w.layout(c, env) }
func (w widgetFunc) Paint(dst *Canvas, r Rect)          { w.paint(dst, r) }

// FromFuncs builds a Widget from a layout and a paint function, for one-off
// widgets that do not deserve a named type.
func FromFuncs(layout func(Constraints, Env) Size, paint func(*Canvas, Rect)) Widget {
	return widgetFunc{layout: layout, paint: paint}
}

// Vacant implements Vacant: it, or a component it mounted, mounted nothing.
func (c *ComponentWidget) Vacant() bool { return blockChild(c) == nil }

// Baseliner is a widget that knows where its first line of text sits: the
// distance from the top of the Rect it was laid out for to that line's
// baseline. Row.Align(AlignBaseline) lines children up on it, so a small
// caption beside a large title shares its baseline instead of its centre.
//
// The answer is valid after Layout, for the size Layout returned. Text and
// widget.TextInput report their face's ascent; Box, Column, Align and the other
// containers report their first child's baseline moved by where that child
// went. A widget with no text in it reports false, and a Row then rests it
// on the baseline by its bottom edge, as CSS does with an image in a line
// of text.
type Baseliner interface {
	Baseline() (float64, bool)
}

// BaselineOf asks w for its baseline, or reports false when w is not a
// Baseliner. A container that is one reports its child's through it, moved
// by where it placed the child.
func BaselineOf(w Widget) (float64, bool) {
	if b, ok := w.(Baseliner); ok {
		return b.Baseline()
	}
	return 0, false
}

// Geometry and constraints are owned by the renderer-independent layout
// package. These aliases keep ggui's public geometry API unchanged.
type (
	// Number is any built-in numeric type. The geometry constructors take
	// one so that pixel counts from ggfx (int) and layout math
	// (float64) can be mixed without a cast at every call site.
	Number = layout.Number

	// Point is a position in logical (device-independent) pixels.
	Point = layout.Point

	// Size is a width/height pair in logical pixels.
	Size = layout.Size

	// Rect is an axis-aligned rectangle anchored at Origin. Paint receives
	// one: the origin its parent chose and the size its own Layout returned.
	Rect = layout.Rect

	// Affine is a 2D affine transform of logical points. The zero value
	// is the identity, and each method returns the transform followed by
	// one more step: Affine{}.Scale(2, 2).Translate(10, 0) scales a point
	// and then moves it.
	Affine = geom.Affine
)

// Pt returns the Point at (x, y), whatever numeric type they are.
func Pt[T Number](x, y T) Point { return layout.Pt(x, y) }

// Sz returns the Size w by h, whatever numeric type they are.
func Sz[T Number](w, h T) Size { return layout.Sz(w, h) }

// Rct returns the Rect at origin with size.
func Rct(origin Point, size Size) Rect { return layout.Rct(origin, size) }

// Unbounded is the maximum a Scroll gives its child along the scroll axis:
// take whatever you need. Widgets that would fill the space they are given
// fall back to their content size on an unbounded axis.
var Unbounded = math.Inf(1)

// Constraints bound the size a widget may choose during layout.
type Constraints = layout.Constraints

// Tight returns constraints that permit exactly one size.
func Tight(s Size) Constraints { return layout.Tight(s) }

// Loose returns constraints that permit anything up to s.
func Loose(s Size) Constraints { return layout.Loose(s) }

// EdgeInsets is padding on the four sides of a box: Top, Right, Bottom and
// Left. Shrink removes it from constraints and Inflate adds it to a size.
type EdgeInsets = layout.Insets

// Insets builds EdgeInsets with CSS shorthand: one value for every side, two
// for vertical then horizontal, four for top, right, bottom, left.
func Insets(sides ...float64) EdgeInsets {
	switch len(sides) {
	case 0:
		return EdgeInsets{}
	case 1:
		return EdgeInsets{Top: sides[0], Right: sides[0], Bottom: sides[0], Left: sides[0]}
	case 2:
		return EdgeInsets{Top: sides[0], Right: sides[1], Bottom: sides[0], Left: sides[1]}
	case 4:
		return EdgeInsets{Top: sides[0], Right: sides[1], Bottom: sides[2], Left: sides[3]}
	default:
		panic("ggui: Insets takes 1, 2 or 4 values")
	}
}

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
// and widget.TextInput ask when they change; a custom widget whose size depends on
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
	kidIdx  map[*CachedWidget]int // lazy index into kids once len(kids) > mostScannedReads

	outer   *CachedWidget // the nearest Cached above, told when this one is
	dirty   bool
	valid   bool
	cons    Constraints
	fontGen uint64
	scale   float64
	rev     uint64 // the Env revision the size was measured under
	size    Size
	gen     uint64 // counts the measurements
	// measuring is set while the cache measures its child, and again that
	// a Layout inside that measurement called Invalidate; see stale.
	measuring bool
	again     bool
	// recordFn caches the bound c.record method value so Layout does not
	// allocate a closure on every measurement pass.
	recordFn func(reactive.LayoutSource, uint64)
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

// stale marks this cache and every one above it stale, as invalidate does,
// for the next layout, asking for none itself. A cache measuring now, as
// one is above a Layout that calls Invalidate, takes the mark when the
// measurement ends: marked now, the end would clear it as done with, and
// the layout asked for would find every cache above the widget as it was
// and lay none of it out again.
func (c *CachedWidget) stale() {
	for ; c != nil; c = c.outer {
		if c.measuring {
			c.again = true
		} else {
			c.dirty = true
		}
	}
}

// Layout implements Widget.
func (c *CachedWidget) Layout(cs Constraints, env Env) Size {
	c.outer, _ = env.Get(cacheOwner)
	gen, scale := fontGen(), UseScale()
	if c.valid && !c.dirty && cs == c.cons && env.rev == c.rev && c.fontGen == gen && c.scale == scale && c.inputsEqual() {
		c.tellOuter()
		return c.size
	}
	clear(c.kids)
	c.sources, c.kids = c.sources[:0], c.kids[:0]
	clear(c.kidIdx)
	if c.recordFn == nil {
		c.recordFn = c.record
	}
	c.measuring = true
	reactive.Measure(c.recordFn, func() { c.size = c.child.Layout(cs, env.With(cacheOwner, c)) })
	c.measuring = false
	c.sources = dedupeReads(c.sources)
	c.kids = slices.Clip(c.kids)
	c.gen++
	c.fontGen, c.scale = gen, scale
	c.cons, c.rev, c.valid, c.dirty, c.again = cs, env.rev, true, c.again, false
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
	if o.kidIdx != nil && len(o.kidIdx) == len(o.kids) {
		if i, ok := o.kidIdx[c]; ok {
			o.kids[i].gen = c.gen
			return
		}
	} else if len(o.kids) > mostScannedReads {
		if o.kidIdx == nil {
			o.kidIdx = make(map[*CachedWidget]int, len(o.kids)+1)
		} else {
			clear(o.kidIdx)
		}
		for i, k := range o.kids {
			o.kidIdx[k.c] = i
		}
		if i, ok := o.kidIdx[c]; ok {
			o.kids[i].gen = c.gen
			return
		}
	} else {
		for i := range o.kids {
			if o.kids[i].c == c {
				o.kids[i].gen = c.gen
				return
			}
		}
	}
	if !o.measuring && len(o.kids) >= mostLateKids {
		// Most of those are likely gone by now.
		o.invalidate()
		return
	}
	idx := len(o.kids)
	o.kids = append(o.kids, kidCache{c, c.gen})
	if o.kidIdx != nil {
		o.kidIdx[c] = idx
	}
}

// Paint implements Widget.
func (c *CachedWidget) Paint(dst *Canvas, r Rect) { dst.Paint(c.child, r) }

// Baseline implements Baseliner: the child's.
func (c *CachedWidget) Baseline() (float64, bool) { return BaselineOf(c.child) }

// record notes src as read by this cache's measurement. Layout installs it
// as the reactive core's recorder for the duration of a measurement, saving
// and restoring the previous one so that nested caches nest correctly.
func (c *CachedWidget) record(src reactive.LayoutSource, version uint64) {
	// Most widgets read only a small set of reactive sources, often repeatedly
	// within a single layout pass. Updating an existing entry in place avoids
	// growing c.sources with duplicate reads and eliminates the grow-then-shrink
	// allocation churn in dedupeReads.
	n := len(c.sources)
	if n > 0 && c.sources[n-1].src == src {
		c.sources[n-1].version = version
		return
	}
	if n <= mostScannedReads {
		for i := n - 2; i >= 0; i-- {
			if c.sources[i].src == src {
				c.sources[i].version = version
				return
			}
		}
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
	var n int
	if len(reads) <= mostScannedReads {
		// record already deduplicated in place while len <= mostScannedReads.
		n = len(reads)
	} else {
		n = dedupeByMap(reads)
	}
	clear(reads[n:])
	reads = reads[:n]
	if cap(reads) > 4*n+16 {
		return slices.Clone(reads)
	}
	return reads
}

// dedupeByMap is dedupeByScan for many reads, finding those kept in a map.
func dedupeByMap(reads []sourceRead) int {
	readSeen := readsSeen.Get().(map[reactive.LayoutSource]int)
	defer readsSeen.Put(readSeen)
	n := 0
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
	return n
}

// Invalidate tells the runtime that the widget laid out under env changed
// size, or its children did: the tree is laid out again next frame and the
// nearest Cached above the widget measures its subtree afresh. A StateValue
// write does the first half by itself; a widget that keeps size-affecting
// state outside signals calls Invalidate when that state changes.
//
// Under a layout boundary, a Scroll whose viewport keeps its size whatever
// its content's, a Contain or a pane of ui.Resizable, the change shows
// within that boundary alone, and a frame nothing else changed for is
// painted there and nowhere else: a disclosure easing open in a scrolling
// pane or a fixed panel does not paint the window again at every step. A
// boundary that turns out to follow its content's size is painted whole
// once and passes the next change to the one around it; see
// LayoutBoundary.
//
// It may be called from the widget's own Layout, for a size that is still
// on its way somewhere, as Motion.LayoutTo does: the layout asked for is
// the next frame's, and runs then, not again in this one.
func Invalidate(env Env) {
	c, _ := env.Get(cacheOwner)
	if b := boundaryOf(env).holding(); b != nil {
		reactive.Contained(reactive.RequestLayout)
		c.stale()
		b.hold()
		return
	}
	reactive.RequestLayout()
	c.stale()
}

// Relayout asks for a layout that no boundary holds, and marks the layout
// caches above the widget laid out under env stale, at once: the next
// frame lays the tree out again and measures that widget's subtree afresh.
// It is for a change that shows outside any boundary around the widget, or
// for the widget that is the boundary, whose own layout must run though
// its size has not changed, as a Scroll's does for content to lay out at a
// new offset. Made inside the boundary's LayoutBoundary.Contained, the
// layout it asks for is held there after all, as a Scroll's moves are. A
// cache measuring as it is called, as one around a Layout that calls it
// is, keeps no mark: that measurement is under way, and ends valid.
//
// Invalidate stays the call for a widget whose size changed: it lets the
// boundary around hold that. A widget keeps the Env of its last Layout to
// call either later.
func Relayout(env Env) {
	c, _ := env.Get(cacheOwner)
	c.invalidate()
}
