package ggui

import "github.com/ironpark/ggui/a11y"

// The description types live in the a11y package, which cannot import
// this one, so that a bridge reads the same types a widget wrote rather
// than a copy of them. These are those types under these names.
type (
	// Tristate is a checkbox-like state: not checkable, off, on or mixed.
	Tristate = a11y.Tristate

	// ActionSet is the set of actions a node claims to support.
	ActionSet = a11y.ActionSet

	// Node is what a widget says about itself: its role, name, value,
	// state and the actions it offers.
	Node = a11y.Node

	// TextRun is one line of a text field's frozen layout.
	TextRun = a11y.TextRun

	// TextStop is one character boundary within a run.
	TextStop = a11y.TextStop
)

// The tristate values.
const (
	TriNone  = a11y.TriNone
	TriOff   = a11y.TriOff
	TriOn    = a11y.TriOn
	TriMixed = a11y.TriMixed
)

// The actions a node may offer.
const (
	ActionPress          = a11y.ActionPress
	ActionIncrement      = a11y.ActionIncrement
	ActionDecrement      = a11y.ActionDecrement
	ActionSetValue       = a11y.ActionSetValue
	ActionExpand         = a11y.ActionExpand
	ActionCollapse       = a11y.ActionCollapse
	ActionSelect         = a11y.ActionSelect
	ActionFocus          = a11y.ActionFocus
	ActionScrollIntoView = a11y.ActionScrollIntoView
	ActionSetSelection   = a11y.ActionSetSelection
)

// Tri returns TriOn for true and TriOff for false, for a control whose
// state is a plain boolean.
func Tri(v bool) Tristate { return a11y.Tri(v) }

// Expandable returns a pointer to v, for Node.Expanded.
func Expandable(v bool) *bool { return a11y.Expandable(v) }

// The accessibility tree is built explicitly, not inferred from the hit
// regions. It cannot be inferred: layout containers register no region at
// all, one widget may register several regions at the same depth, a
// disabled control registers none while a screen reader must still see it,
// and static text must stay out of the hit list, which input scans on every
// pointer event. So widgets say what they are, in their own words, into an
// array of their own: Canvas.Describe for a control that already has a
// handler, Canvas.Leaf and Canvas.Node for everything else.

// Describer is a handler that describes itself fully, beyond the Role and
// Name every Interactive carries. Canvas.Describe prefers it, so a slider
// reports its range, a checkbox its tick and a combobox whether it is open
// without any of that having to live on Interactive. Returned pointers and
// slices must remain unchanged until the frame finishes. The published
// snapshot owns copies, so the next frame may reuse the description buffers.
type Describer interface {
	Describe() Node
}

// semNode is one node as it was recorded during a paint: the description
// the widget gave, where it went, and enough identity to find it again.
// Parent is an index into the same array plus one, so that the zero value
// means "no parent" on a Canvas that was never reset.
type semNode struct {
	node    Node
	parent  int // index in sem, plus one; 0 is a root node
	rect    Rect
	full    Rect // before clipping, so an offscreen node still says where it is
	scope   *focusScope
	group   any // the EachKeyed entry that painted it, as for a hit region
	id      any // from an Identified handler, else nil
	handler any // the widget that described itself, for focus and dedup
}

// SemRef refers to a node recorded earlier in this frame, so that work
// deferred to an overlay can be attached under it instead of becoming a
// root of its own. The zero value refers to nothing.
type SemRef struct{ n int } // index in sem, plus one

// Valid reports whether the reference points at a node.
func (r SemRef) Valid() bool { return r.n != 0 }

// resetSemantics starts a new frame's tree. App.Draw and Probe.Frame call
// it before painting. buildSemTree copies nodes into a separate immutable
// snapshot, so this private staging buffer can be reused. Clear old entries
// to release handlers and text data when the next frame has fewer nodes.
func (c *Canvas) resetSemantics() {
	f := c.fs()
	clear(f.sem)
	f.sem = f.sem[:0]
	f.semParent, f.semLast = 0, 0
	clear(f.semIndex)
}

// Leaf records n as an element at r with no children of its own.
func (c *Canvas) Leaf(r Rect, n Node) { c.addSem(r, n, nil) }

// Node records n as an element at r and paints fn into it, so that
// everything fn describes becomes its child. The nesting comes from this
// call and nothing else: paint depth cannot say what contains what, since
// Column, Row, Box and Padding describe nothing.
//
//	dst.Node(header, ggui.Node{Role: ggui.RoleTabs}, func(dst *ggui.Canvas) {
//		for i, tab := range tabs {
//			dst.Leaf(rects[i], ggui.Node{Role: ggui.RoleTab, Name: tab.Label})
//		}
//	})
func (c *Canvas) Node(r Rect, n Node, fn func(dst *Canvas)) {
	c.scoped(c.addSem(r, n, nil), fn)
}

// Describe records the node the handler h reports at r. A handler that
// implements Describer says everything about itself; one that only
// implements Semantic gets its Role and Name, marked Disabled when its
// Interactive is Inert. Interactive.Hit calls it for every control,
// including a disabled one, which registers no input region at all but must
// still be announced.
//
// Describing the same handler twice in a frame keeps the first node, so a
// control painted as a box around an editor, as ui.TextField is, is one
// element and not two.
func (c *Canvas) Describe(r Rect, h any) { c.addSem(r, nodeOf(h), h) }

// DescribeNode is Describe for a handler that contains other elements: the
// children fn describes hang under it. A table row is one.
func (c *Canvas) DescribeNode(r Rect, h any, fn func(dst *Canvas)) {
	c.scoped(c.addSem(r, nodeOf(h), h), fn)
}

// scoped paints fn with ref as the parent of everything it describes.
func (c *Canvas) scoped(ref SemRef, fn func(dst *Canvas)) {
	if c == nil {
		fn(nil)
		return
	}
	root := c.fs()
	prev := root.semParent
	if ref.n != 0 {
		root.semParent = ref.n
	}
	defer func() { root.semParent = prev }()
	fn(c)
}

// SemanticRef returns the node h described this frame, if it described one.
// Overlay takes it as an owner, so a dropdown's list belongs to the
// combobox that opened it rather than standing beside the whole tree.
func (c *Canvas) SemanticRef(h any) SemRef {
	if c == nil || h == nil {
		return SemRef{}
	}
	return SemRef{n: c.fs().semIndex[a11y.Key(h)]}
}

// nodeOf is what a handler says about itself: its own Describe, else its
// Role and Name with Inert folded into Disabled.
func nodeOf(h any) Node {
	if d, ok := h.(Describer); ok {
		return d.Describe()
	}
	var n Node
	if s, ok := h.(Semantic); ok {
		n.Role, n.Name = s.Semantics()
	}
	if i, ok := h.(interface{ state() *Interactive }); ok {
		n.Disabled = i.state().IsInert()
		n.Actions = ActionPress | ActionFocus
	}
	return n
}

// addSem appends a node and returns a reference to it. A node clipped out
// of view is kept and flagged Offscreen rather than dropped, with full
// holding the bounds it would have had: an assistive technology scrolls to
// what it cannot see.
func (c *Canvas) addSem(r Rect, n Node, h any) SemRef {
	if c == nil || c.inert {
		return SemRef{}
	}
	root := c.fs()
	key := a11y.Key(h)
	if key != nil {
		if i := root.semIndex[key]; i != 0 {
			return SemRef{n: i}
		}
	}
	e := semNode{node: n, parent: root.semParent, rect: r, full: r, handler: h, id: idOf(h)}
	e.scope, e.group = c.scope, c.group
	if c.clipped {
		e.rect = r.Intersect(c.clip)
		if e.rect.Empty() {
			e.node.Offscreen = true
			e.rect = Rect{Origin: e.full.Origin}
		}
	}
	root.sem = append(root.sem, e)
	ref := SemRef{n: len(root.sem)}
	root.semLast = ref.n
	if key != nil {
		if root.semIndex == nil {
			root.semIndex = make(map[any]int)
		}
		root.semIndex[key] = ref.n
	}
	return ref
}

// named reports whether the element painted just before r, under the same
// parent, is a control that covers it: a button's own label, a checkbox's
// text. Such text is part of the control's name, not an element beside it,
// so TextWidget stays quiet inside one. The comparison is against the
// control's unclipped bounds, so a row scrolled out of view does not
// suddenly grow a text node of its own.
func (c *Canvas) named(r Rect) bool {
	if c == nil {
		return false
	}
	root := c.fs()
	if root.semLast == 0 {
		return false
	}
	last := &root.sem[root.semLast-1]
	return last.parent == root.semParent && a11y.Control(last.node.Role) && covers(last.full, r)
}

// covers reports whether outer wholly contains inner.
func covers(outer, inner Rect) bool {
	return inner.Origin.X >= outer.Origin.X && inner.Origin.Y >= outer.Origin.Y &&
		inner.Origin.X+inner.Size.W <= outer.Origin.X+outer.Size.W &&
		inner.Origin.Y+inner.Size.H <= outer.Origin.Y+outer.Size.H
}
