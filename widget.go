package ggui

// Widget is the unit of composition. Layout asks it for a size under some
// Constraints; Paint then hands it a Canvas and the Rect its parent assigned:
// the origin the parent chose and the size Layout returned. Layout always runs
// before Paint in a frame, so a widget keeps only what the Rect cannot tell
// it, such as where its children go. Leaf widgets usually keep nothing at all.
type Widget interface {
	Layout(c Constraints, env Env) Size
	Paint(dst *Canvas, r Rect)
}

// Builder turns state into a Widget tree. It runs inside an Effect, so it
// re-runs when the signals it reads change. A plain function that returns a
// Widget is the simplest component; Component and Reactive give a subtree its
// own Effect so it rebuilds without its parent.
type Builder func() Widget

// ComponentWidget is a subtree with its own rebuild boundary. Build one with
// Component, Keyed, Mount or Reactive.
type ComponentWidget struct {
	child Widget
	cache *CachedWidget // the nearest Cached above, told on every rebuild
	mount func()        // runs setup at the first Layout; nil once mounted
}

// mounted is a keyed component's instance, kept by the owner effect across
// its re-runs.
type mounted struct {
	comp    *ComponentWidget
	props   any // *Signal[P]
	dispose func()
}

// Component runs setup once, at the component's first Layout, and then
// runs the Builder it returns in an effect of its own. State created in
// setup lives as long as the parent's tree keeps this component: a rebuild
// of the parent makes a new one (see Keyed to survive that). Only the
// Builder re-runs when the signals it reads change:
//
//	func button(label string, onTap func()) ggui.Widget {
//		return ggui.Component(func() ggui.Builder {
//			hovered := ggui.State(false)
//			return func() ggui.Widget {
//				return ggui.Pointer(ggui.Text(label)).OnTap(onTap).OnHover(hovered.Set)
//			}
//		})
//	}
//
// setup runs untracked under an owner of its own, so it may call OnCleanup
// and the signals it reads do not subscribe the parent. A component that is
// never laid out never runs setup.
func Component(setup func() Builder) *ComponentWidget {
	c := &ComponentWidget{}
	owner := currentOwner()
	c.mount = func() {
		if owner != nil && owner.disposed {
			return
		}
		withOwner(owner, func() { Root(func() { c.run(setup()) }) })
	}
	return c
}

// run starts the Builder's effect.
func (c *ComponentWidget) run(build Builder) {
	Effect(func() {
		c.child = build()
		c.cache.invalidate()
	})
}

// Keyed is a Component that survives a rebuild of its parent: the owner
// effect keeps one instance per key across its runs, so a run that
// constructs Keyed with the same key gets the mounted instance back, with
// its local state, effects and focus. Keys are unique within one Builder;
// a key used twice in one run panics. An instance the next run does not
// construct again is disposed.
//
// Controls, TextInput, Scroll, Popup and Transition constructed inside a
// keyed component take an identity from it, the mount instance plus their place in
// construction order, so they keep hit regions, retained state and
// adoption across the component's rebuilds without a Key of their own.
func Keyed(key any, setup func() Builder) *ComponentWidget {
	return Mount(key, struct{}{}, func(*Signal[struct{}]) Builder { return setup() })
}

// Mount is Keyed with props: the values the parent passes on each rebuild.
// Setup receives them as a Signal that Mount writes on every claim, so read
// props through it rather than capturing them.
//
//	ggui.Mount(id, todo, func(todo *ggui.Signal[Todo]) ggui.Builder {
//		return func() ggui.Widget { return ggui.Text(todo.Get().Title) }
//	})
func Mount[P any](key any, props P, setup func(*Signal[P]) Builder) *ComponentWidget {
	owner := currentOwner()
	if owner == nil {
		return Component(func() Builder { return setup(State(props)) })
	}
	if m := owner.claim(key); m != nil {
		m.props.(*Signal[P]).Set(props)
		return m.comp
	}
	sig := State(props)
	c := &ComponentWidget{}
	m := &mounted{comp: c, props: sig}
	c.mount = func() {
		if owner.disposed {
			return
		}
		// Under no owner: the instance belongs to the registry, not to the
		// run that constructed it, so the parent's re-run leaves it alone.
		withOwner(nil, func() { m.dispose = rootWith(m, "", func() { c.run(setup(sig)) }) })
	}
	m.dispose = func() {}
	owner.keep(key, m)
	return c
}

// Reactive gives build an effect of its own: when the signals it reads
// change, this subtree rebuilds and the parent does not. Use it to keep a
// parent's Builder static so the components it holds survive.
func Reactive[W Widget](build func() W) *ComponentWidget {
	c := &ComponentWidget{}
	Effect(func() {
		c.child = build()
		c.cache.invalidate()
	})
	return c
}

// View builds a widget from a reactive value and rebuilds it when the value
// changes, with the widget type inferred from the constructor:
//
//	ggui.View(label, ggui.Text)
//	ggui.View(rows, func(r []Row) *ggui.ColumnWidget { return ggui.List(r, rowWidget) })
func View[T any, W Widget](r Reader[T], build func(T) W) Widget {
	return Reactive(func() Widget { return build(r.Get()) })
}

// IfWidget shows the first branch whose condition holds, the Else branch
// when none does, or nothing. Branches are constructed once, up front, so
// the one shown keeps its state while the others wait. Build one with If.
type IfWidget struct {
	comp     *ComponentWidget
	branches []ifBranch
	other    Widget
}

type ifBranch struct {
	cond Reader[bool]
	then Widget
}

// If shows then while cond is true. ElseIf and Else add branches, read in
// order; without an Else nothing is shown when no condition holds:
//
//	ggui.If(loading, ui.Spinner()).
//		ElseIf(failed, ggui.Text("Could not load")).
//		Else(ggui.View(rows, resultTable))
//
// The choice is made in an effect of its own that starts at the first
// Layout, once the chain is complete, so every condition is subscribed and
// the parent does not rebuild when one changes. A branch that can only be
// constructed while its condition holds goes through Component, which
// builds at the first Layout:
//
//	ggui.If(signedIn, ggui.Component(func() ggui.Builder {
//		return func() ggui.Widget { return profile(user.Get()) }
//	}))
func If(cond Reader[bool], then Widget) *IfWidget {
	w := &IfWidget{branches: []ifBranch{{cond, then}}}
	w.comp = Component(func() Builder { return w.pick })
	return w
}

// ElseIf adds a branch tried when every earlier condition is false.
func (w *IfWidget) ElseIf(cond Reader[bool], then Widget) *IfWidget {
	w.branches = append(w.branches, ifBranch{cond, then})
	return w
}

// Else sets what is shown when no condition holds.
func (w *IfWidget) Else(other Widget) *IfWidget { w.other = other; return w }

// pick reads the conditions up to the first true one, so the effect re-runs
// only when a condition that mattered changes.
func (w *IfWidget) pick() Widget {
	for _, b := range w.branches {
		if b.cond.Get() {
			return b.then
		}
	}
	return w.other // nil lays out as nothing
}

// current is the branch shown, for tests.
func (w *IfWidget) current() Widget { return w.comp.child }

// absent implements vacant: an If showing no branch takes no gap in a
// Column, Row or Wrap, as if it were not there.
func (w *IfWidget) absent() bool { return w.comp.child == nil }

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

// When is If with a single Else: then while cond is true, otherwise (or
// nothing) while it is false.
func When(cond Reader[bool], then Widget, otherwise ...Widget) Widget {
	w := If(cond, then)
	if len(otherwise) > 0 {
		w.Else(otherwise[0])
	}
	return w
}

// Layout implements Widget.
func (c *ComponentWidget) Layout(cs Constraints, env Env) Size {
	c.cache, _ = env.Get(cacheOwner)
	if c.mount != nil {
		c.mount()
		c.mount = nil
	}
	if c.child == nil {
		return cs.Constrain(Size{})
	}
	return c.child.Layout(cs, env)
}

// Paint implements Widget.
func (c *ComponentWidget) Paint(dst *Canvas, r Rect) {
	if c.child != nil {
		dst.Paint(c.child, r)
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
