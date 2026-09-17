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
// Component or Reactive.
type ComponentWidget struct {
	child Widget
}

// Component runs setup once, in the enclosing effect, and then runs the
// Builder it returns in an effect of its own. State created in setup lives
// as long as the parent's tree keeps this component; only the Builder re-runs
// when the signals it reads change:
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
// setup runs untracked, so signals it reads do not subscribe the parent.
func Component(setup func() Builder) *ComponentWidget {
	var build Builder
	Untrack(func() { build = setup() })
	return Reactive(build)
}

// Reactive gives build an effect of its own: when the signals it reads
// change, this subtree rebuilds and the parent does not. Use it to keep a
// parent's Builder static so the components it holds survive.
func Reactive(build Builder) *ComponentWidget {
	c := &ComponentWidget{}
	Effect(func() { c.child = build() })
	return c
}

// Layout implements Widget.
func (c *ComponentWidget) Layout(cs Constraints, env Env) Size { return c.child.Layout(cs, env) }

// Paint implements Widget.
func (c *ComponentWidget) Paint(dst *Canvas, r Rect) { dst.Paint(c.child, r) }

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
