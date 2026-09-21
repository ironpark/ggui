package ggui

// Widget is the unit of composition. Layout asks it for a size under some
// Constraints; Paint then hands it a Canvas and the Rect its parent assigned:
// the origin the parent chose and the size Layout returned. Layout always runs
// before Paint in a frame, so a widget keeps only what the Rect cannot tell
// it, such as where its children go. Leaf widgets usually keep nothing at all.
//
// Unless a method documents runtime updates, configure widgets before their
// first Layout. After mount, use bindings or rebuild a subtree with View or
// Reactive. Configuration setters do not generally invalidate layout caches.
// Custom widgets may configure their children before laying them out; when
// their own non-reactive state affects layout, they call Invalidate.
type Widget interface {
	Layout(c Constraints, env Env) Size
	Paint(dst *Canvas, r Rect)
}

// Builder constructs a widget tree. App and Component setup run once;
// Reactive and View explicitly rerun a builder when its dependencies change.
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
}

// Component runs setup once when mounted. Reads in setup are untracked;
// use bindings, View, Reactive or control-flow blocks for later changes.
func Component(setup func() Widget) *ComponentWidget {
	c := &ComponentWidget{}
	owner := currentOwner()
	c.mount = func() {
		if owner != nil && owner.disposed {
			return
		}
		if owner == nil {
			owner = currentOwner()
		}
		withOwner(owner, func() {
			rootWith(c, "", func() {
				c.child = setup()
				c.cw.child = c.child
			})
		})
	}
	return c
}

// Reactive gives build an effect of its own: when the signals it reads
// change, this subtree rebuilds and the parent does not. Use it to keep a
// parent's Builder static so the components it holds survive.
func Reactive[W Widget](build func() W) *ComponentWidget {
	c := &ComponentWidget{}
	observe(func() {
		c.child = build()
		c.cw.child = c.child
		c.cw.invalidate()
	})
	return c
}

// View builds a widget from a reactive value and rebuilds it when the value
// changes, with the widget type inferred from the constructor:
//
//	ggui.View(label, ggui.Text)
//	ggui.View(rows, func(r []Row) *ggui.ColumnWidget { return ggui.List(r, rowWidget) })
func View[T any, W Widget](r Readable[T], build func(T) W) Widget {
	return Reactive(func() Widget { return build(r.Get()) })
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
func (w *IfWidget) absent() bool    { return w.current() == nil }

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
		var previous K
		var initialized bool
		var dispose Cleanup
		owner := currentOwner()
		OnCleanup(func() {
			if dispose != nil {
				dispose()
			}
		})
		observe(func() {
			value := key.Get()
			if initialized && value == previous {
				return
			}
			if dispose != nil {
				dispose()
				dispose = nil
			}
			previous, initialized = value, true
			withOwner(owner, func() {
				// A unique root gives remounted controls fresh identities.
				identity := new(int)
				dispose = rootWith(identity, "", func() { c.child = build(value); c.cw.child = c.child })
			})
			c.cw.invalidate()
		})
		return c
	})
}

// vacant is reported by a widget that currently shows nothing, so the flow
// around it leaves out the gap it would otherwise place beside it. It is
// asked after the widget's Layout, which is when an If knows its branch.
type vacant interface{ absent() bool }

func isAbsent(w Widget) bool {
	v, ok := w.(vacant)
	return ok && v.absent()
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
func (c *ComponentWidget) Baseline() (float64, bool) { return baselineOf(c.child) }

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

func (c *ComponentWidget) absent() bool { return blockChild(c) == nil }
