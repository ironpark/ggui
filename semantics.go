package ggui

import "reflect"

// The accessibility tree is built explicitly, not inferred from the hit
// regions. It cannot be inferred: layout containers register no region at
// all, one widget may register several regions at the same depth, a
// disabled control registers none while a screen reader must still see it,
// and static text must stay out of the hit list, which input scans on every
// pointer event. So widgets say what they are, in their own words, into an
// array of their own: Canvas.Describe for a control that already has a
// handler, Canvas.Leaf and Canvas.Node for everything else.

// Tristate is a checkbox's state: on, off, or the mixed state a parent
// checkbox shows when only some of its children are ticked. The zero value
// says the node has no checked state at all, which is what every node that
// is not a checkbox, radio or switch reports.
type Tristate uint8

const (
	TriNone  Tristate = iota // not checkable
	TriOff                   // checkable and not checked
	TriOn                    // checked
	TriMixed                 // partially checked
)

// Tri returns TriOn for true and TriOff for false, for a control whose
// checked state is a plain bool.
func Tri(v bool) Tristate {
	if v {
		return TriOn
	}
	return TriOff
}

// Expandable returns a pointer to v, for Node.Expanded: a nil Expanded
// means the node does not expand at all, which is not the same as being
// expandable and closed.
func Expandable(v bool) *bool { return &v }

// ActionSet is the set of actions a node accepts, as a bitset. A node
// advertises what it supports so an assistive technology can offer it; see
// Actor for the side that performs them.
type ActionSet uint32

const (
	ActionPress          ActionSet = 1 << iota // activate: a button, a menu item, a checkbox
	ActionIncrement                            // raise a slider or stepper by one step
	ActionDecrement                            // lower it by one step
	ActionSetValue                             // replace the value outright: a text field, a slider
	ActionExpand                               // open a disclosure, accordion or combobox
	ActionCollapse                             // close one
	ActionSelect                               // make this the chosen tab, option or row
	ActionFocus                                // move keyboard focus here
	ActionScrollIntoView                       // bring the node into view
	ActionSetSelection                         // move the caret or the selection in a text field
)

// Has reports whether every action in b is in a.
func (a ActionSet) Has(b ActionSet) bool { return a&b == b }

// Node describes one element of the accessibility tree: what it is, what it
// is called, and what may be done to it. A widget fills in only the fields
// its role gives meaning to; the zero value of every other field says "not
// applicable", which is why Expanded is a pointer and Checked has a TriNone.
type Node struct {
	Role        Role
	Name        string   // what the element is called, read first
	Description string   // the longer explanation, read after the name
	Value       string   // a text field's contents, a select's current option
	Checked     Tristate // checkbox, radio, switch
	Expanded    *bool    // accordion, collapsible, combobox; nil is not expandable
	Selected    bool     // tab, option, row
	Disabled    bool     // from Interactive.Inert: present, but takes no input
	Min         float64  // slider, progress: the bottom of the range
	Max         float64  // the top of the range, or a list's item count
	Now         float64  // where the value sits in it, or an item's place in a list
	Actions     ActionSet
	Offscreen   bool // clipped out of view, but present with its true bounds

	// A text field carries where its caret and selection are, in bytes
	// into Value, and -- while something is reading the tree closely
	// enough to want it -- how Value was laid out on screen. A platform
	// text API asks for characters, lines and the rectangle a range covers
	// from a thread that must not touch the live widget, so the answers
	// are frozen into the snapshot with everything else.
	SelStart, SelEnd int
	Runs             []TextRun
}

// TextRun is one painted line of a text node: which bytes of Node.Value it
// covers, where it went, and where every character boundary inside it
// landed. Rect is in the same coordinates as SemNode.Full, and a stop's X
// is measured from Rect.Origin.X.
type TextRun struct {
	Start, End int
	Rect       Rect
	Stops      []TextStop
}

// TextStop is one character boundary within a run: a byte offset into
// Node.Value and the horizontal position it sits at. There is one for every
// rune boundary in the run, including both ends, so a caret between any two
// characters has a position.
type TextStop struct {
	Byte int
	X    float64
}

// At returns the horizontal position of byte offset b within the run,
// clamped to its ends. A byte in the middle of a rune takes that rune's
// starting position, which is where a caret would be drawn.
func (r TextRun) At(b int) float64 {
	if len(r.Stops) == 0 {
		return 0
	}
	last := r.Stops[0]
	for _, s := range r.Stops {
		if s.Byte > b {
			break
		}
		last = s
	}
	return last.X
}

// Describer is a handler that describes itself fully, beyond the Role and
// Name every Interactive carries. Canvas.Describe prefers it, so a slider
// reports its range, a checkbox its tick and a combobox whether it is open
// without any of that having to live on Interactive.
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
	group   any // the For entry that painted it, as for a hit region
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
// it before painting; the array is freshly allocated rather than reused
// because the snapshot published from the last frame is still being read,
// possibly from another thread.
func (c *Canvas) resetSemantics() {
	c.sem = nil
	c.semParent, c.semLast = 0, 0
	clear(c.semIndex)
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
	root := c.root()
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
	return SemRef{n: c.root().semIndex[semKey(h)]}
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
		n.Disabled = i.state().Inert
		n.Actions = ActionPress | ActionFocus
	}
	return n
}

// semKey returns h as a map key, or nil when h cannot be one: a handler
// holding a slice or a map would panic a lookup.
func semKey(h any) any {
	if h == nil {
		return nil
	}
	if t := reflect.TypeOf(h); t == nil || !t.Comparable() {
		return nil
	}
	return h
}

// addSem appends a node and returns a reference to it. A node clipped out
// of view is kept and flagged Offscreen rather than dropped, with full
// holding the bounds it would have had: an assistive technology scrolls to
// what it cannot see.
func (c *Canvas) addSem(r Rect, n Node, h any) SemRef {
	if c == nil || c.inert {
		return SemRef{}
	}
	root := c.root()
	key := semKey(h)
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
	root := c.root()
	if root.semLast == 0 {
		return false
	}
	last := &root.sem[root.semLast-1]
	return last.parent == root.semParent && last.node.Role.control() && covers(last.full, r)
}

// covers reports whether outer wholly contains inner.
func covers(outer, inner Rect) bool {
	return inner.Origin.X >= outer.Origin.X && inner.Origin.Y >= outer.Origin.Y &&
		inner.Origin.X+inner.Size.W <= outer.Origin.X+outer.Size.W &&
		inner.Origin.Y+inner.Size.H <= outer.Origin.Y+outer.Size.H
}

// control reports whether the role is one the user acts on, as opposed to
// content or grouping. Text inside one belongs to it.
func (r Role) control() bool {
	switch r {
	case RoleButton, RoleCheckbox, RoleRadio, RoleSwitch, RoleSlider, RoleTextField,
		RoleSelect, RoleOption, RoleMenu, RoleMenuItem, RoleTab, RoleDisclosure,
		RoleCombobox, RoleLink:
		return true
	}
	return false
}
