package ggui

import (
	"slices"

	"github.com/ironpark/ggui/a11y"
)

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

	space *paintSpace // where it painted, for a tap given in its coordinates
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
func (c *Canvas) Describe(r Rect, h any) {
	if c.describing() {
		c.addSem(r, nodeOf(h), h)
	}
}

// DescribeNode is Describe for a handler that contains other elements: the
// children fn describes hang under it. A table row is one.
func (c *Canvas) DescribeNode(r Rect, h any, fn func(dst *Canvas)) {
	var ref SemRef
	if c.describing() {
		ref = c.addSem(r, nodeOf(h), h)
	}
	c.scoped(ref, fn)
}

// describing reports whether this frame records nodes from c: it does not
// when nobody asked for the tree or c is inert, and then a handler is not
// asked to describe itself either.
func (c *Canvas) describing() bool { return c != nil && !c.inert && !c.fs().semOff }

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
	if !c.describing() {
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
	e.scope, e.group, e.space = c.scope, c.group, c.space
	if c.clipped {
		e.rect = r.Intersect(c.clip)
	}
	if c.space != nil {
		e.rect, e.full = c.space.window(e.rect, false), c.toWindow(e.full)
	}
	if (c.clipped || c.space != nil) && e.rect.Empty() {
		e.node.Offscreen = true
		e.rect = Rect{Origin: e.full.Origin}
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
	return last.parent == root.semParent && a11y.Control(last.node.Role) && covers(last.full, c.toWindow(r))
}

// covers reports whether outer wholly contains inner.
func covers(outer, inner Rect) bool {
	return inner.Origin.X >= outer.Origin.X && inner.Origin.Y >= outer.Origin.Y &&
		inner.Origin.X+inner.Size.W <= outer.Origin.X+outer.Size.W &&
		inner.Origin.Y+inner.Size.H <= outer.Origin.Y+outer.Size.H
}

// The tree types live in the a11y package, which cannot import this one,
// so that a bridge reads the same types a widget wrote rather than a copy
// of them. These are those types under these names.
type (
	// NodeID names one node in a published tree, well enough for an action
	// aimed at it to find it again in the next one.
	NodeID = a11y.NodeID

	// SemNode is one node of a published tree: what a widget said about
	// itself, where it went, and how it hangs among the others. Parent and
	// Children are indices into the tree; Parent is -1 for a root.
	SemNode = a11y.SemNode

	// SemTree is one frame's finished semantic description, safe to read
	// from any goroutine for as long as it is held.
	SemTree = a11y.SemTree
)

// A platform accessibility API is a synchronous pull from a thread that is
// not the one ggui paints on: on every platform the assistive technology
// asks the process for the tree and blocks until it answers, and it asks
// from the main thread while ggfx runs frames on a goroutine of its own.
// Reading the live frame from there would be a race twice over, since
// painting swaps and reuses its buffers every frame. So every frame ends by
// publishing a tree that is finished: never written again after publication,
// and handed out through an atomic pointer. A reader may keep one as long
// as it likes.

// buildSemTree reuses the last snapshot only when all observable values are
// equal. Changed frames own fresh storage; readers can retain any old tree.
func buildSemTree(c *Canvas, focused *hitRegion, prev *SemTree) *SemTree {
	f := c.fs()
	focus := focusedNode(c, focused)
	if sameSemTree(c, focus, prev) {
		return prev
	}
	if len(f.sem) == 0 {
		return a11y.Build(nil, focus)
	}
	nodes := make([]SemNode, len(f.sem))
	for i := range f.sem {
		e := &f.sem[i]
		nodes[i] = SemNode{
			Node:   freezeNode(e.node),
			ID:     NodeID{ID: e.id, Rect: e.full, Role: e.node.Role},
			Rect:   e.rect,
			Full:   e.full,
			Parent: e.parent - 1,
		}
	}
	return a11y.Build(nodes, focus)
}

// Parent indices and paint order fully determine roots and child lists.
// Handlers, scopes and groups are frame-local and are not published.
func sameSemTree(c *Canvas, focus int, prev *SemTree) bool {
	f := c.fs()
	if prev == nil || prev.FocusIndex() != focus || prev.Len() != len(f.sem) {
		return false
	}
	for i := range f.sem {
		a, b := &f.sem[i], prev.Ref(i)
		if a.parent-1 != b.Parent || a.rect != b.Rect || a.full != b.Full ||
			!sameAny(a.id, b.ID.ID) || !sameNode(&a.node, &b.Node) {
			return false
		}
	}
	return true
}

func sameNode(a, b *Node) bool {
	if a.Role != b.Role || a.Name != b.Name || a.Description != b.Description ||
		a.Value != b.Value || a.Checked != b.Checked || a.Selected != b.Selected ||
		a.Disabled != b.Disabled || a.Min != b.Min || a.Max != b.Max || a.Now != b.Now ||
		a.Actions != b.Actions || a.Offscreen != b.Offscreen || a.Level != b.Level ||
		a.SelStart != b.SelStart || a.SelEnd != b.SelEnd {
		return false
	}
	if (a.Expanded == nil) != (b.Expanded == nil) ||
		a.Expanded != nil && *a.Expanded != *b.Expanded {
		return false
	}
	return (a.Runs == nil) == (b.Runs == nil) && slices.EqualFunc(a.Runs, b.Runs, func(a, b TextRun) bool {
		return a.Start == b.Start && a.End == b.End && a.Rect == b.Rect &&
			(a.Stops == nil) == (b.Stops == nil) && slices.Equal(a.Stops, b.Stops)
	})
}

// Describers may reuse their buffers next frame. Detach every mutable part
// of the description before publishing it to readers on other threads.
func freezeNode(n Node) Node {
	if n.Expanded != nil {
		expanded := *n.Expanded
		n.Expanded = &expanded
	}
	n.Runs = slices.Clone(n.Runs)
	for i := range n.Runs {
		n.Runs[i].Stops = slices.Clone(n.Runs[i].Stops)
	}
	return n
}

// focusedNode finds the node the focused hit region belongs to: by the
// handler itself, which is exact, then by identity and Rect, which is how
// input matches a region across a rebuild.
func focusedNode(c *Canvas, focused *hitRegion) int {
	if focused == nil || focused.key == nil {
		return -1
	}
	f := c.fs()
	if i := f.semIndex[a11y.Key(focused.key)]; i != 0 {
		return i - 1
	}
	for i := range f.sem {
		e := &f.sem[i]
		if e.id != nil && e.id == focused.id || e.id == nil && focused.id == nil && e.rect == focused.rect {
			return i
		}
	}
	return -1
}

// The request types live in the a11y package, which cannot import this
// one, so that a bridge raises the same types the app acts on rather than
// copies of them. These are those types under these names.
type (
	// Action is one request aimed at a node.
	Action = a11y.Action

	// Politeness says how urgent an announcement is.
	Politeness = a11y.Politeness

	// Announcement is one thing to say out loud, queued by Announce.
	Announcement = a11y.Announcement
)

// How urgent an announcement is.
const (
	Polite    = a11y.Polite
	Assertive = a11y.Assertive
)

// Actions come from outside the pointer and the keyboard: a screen reader
// user activates a button from a rotor, drags a slider with a gesture that
// never touches it, or types into a field through the platform's own text
// API. The tree says what each node accepts through Node.Actions; this is
// the side that carries them out.
//
// Every entry point marshals onto the UI goroutine through Post and returns
// at once, without waiting for the work to happen. That is not an
// optimization, it is the only thing that cannot deadlock: the assistive
// technology calls from the platform's main thread and blocks there, while
// the UI goroutine may itself be blocked inside a call that needs the main
// thread. Waiting for a reply would have each side holding what the other
// wants. So an action is a request, never a question; read the result from
// the next frame's tree.

// Actor is a widget that carries out an action aimed at it. A widget needs
// one only for what its own logic must do: pressing falls back to the key
// press a control already acts on, and focusing and scrolling into view are
// the runtime's to arrange. Return false for an action the widget does not
// handle, and the fallback runs instead.
type Actor interface {
	Act(a Action) bool
}

// Announce queues text to be spoken. A tree diff cannot express "say this
// now": a toast that appears and goes, a status that changed while focus
// stayed put, a search that found nothing. Those are live regions, and this
// is how a widget raises one.
//
// It may be called from any goroutine. Nothing reads the queue yet; the
// platform bridge will.
func (w *Window) Announce(text string, politeness Politeness) {
	w.announce(Announcement{Text: text, Politeness: politeness})
}

// Announce queues text as App.Announce does, for tests.
func (p *Probe) Announce(text string, politeness Politeness) {
	p.announce(Announcement{Text: text, Politeness: politeness})
}

// Perform asks the node identified by id to carry out act. It returns
// immediately: the work is queued onto the UI goroutine and happens before
// the next frame's input, and the frame after that publishes what came of
// it. See the note at the top of this file for why it must not wait.
//
// The action is found in the accessibility tree, which a window builds only
// while something reads it. A pending Perform is such a reader: while one
// waits, every frame builds the tree, and once it is carried out the window
// goes back to building it only when asked.
func (w *Window) Perform(id NodeID, act Action) {
	w.performs.Add(1)
	var run func()
	run = func() {
		if w.canvas.fs().semOff {
			// The last frame built no tree. The one about to run does,
			// since this Perform is pending, so try again after it.
			w.Post(run)
			return
		}
		w.performs.Add(-1)
		perform(&w.canvas, &w.input, id, act)
	}
	w.Post(run)
}

// Perform queues act as App.Perform does and then runs a frame, so that a
// test sees what came of it without having to drive the loop by hand. The
// queueing is the same, so the path under test is the real one.
func (p *Probe) Perform(id NodeID, act Action) {
	p.Post(func() { perform(&p.canvas, &p.in, id, act) })
	p.Frame()
}

// perform routes act to the widget that described the node. It runs on the
// UI goroutine, against the regions and the tree the last frame left.
func perform(c *Canvas, in *inputState, id NodeID, act Action) bool {
	e := findSemNode(c, id)
	if e == nil {
		return false
	}
	// A disabled control is in the tree so that it can be read out, not so
	// that it can be worked; only being looked at is allowed.
	if e.node.Disabled && act.Kind != ActionFocus && act.Kind != ActionScrollIntoView {
		return false
	}
	if a, ok := e.handler.(Actor); ok && a.Act(act) {
		return true
	}
	switch act.Kind {
	case ActionPress:
		return press(e)
	case ActionFocus:
		return focusNode(in, e)
	case ActionScrollIntoView:
		in.reveal(e.full)
		return true
	}
	return false
}

// findSemNode returns the node id names in the frame's tree, matched the way
// Canvas.adopt matches a hit region: by identity when it has one, else by
// the bounds it painted.
func findSemNode(c *Canvas, id NodeID) *semNode {
	sem := c.fs().sem
	for i := range sem {
		e := &sem[i]
		if e.node.Role != id.Role {
			continue
		}
		if id.ID != nil && sameAny(e.id, id.ID) || id.ID == nil && e.id == nil && e.full == id.Rect {
			return e
		}
	}
	return nil
}

// press activates a control the way the keyboard does. Every control acts
// on Space through Activates, so the existing path serves: nothing has to
// learn a second way to be pressed. A handler that takes only the pointer,
// such as a menu item, gets the tap instead.
func press(e *semNode) bool {
	if k, ok := e.handler.(KeyHandler); ok {
		k.HandleKey(KeyEvent{Kind: KeyPress, Key: KeySpace})
		return true
	}
	if p, ok := e.handler.(PointerHandler); ok {
		at := e.space.local(Pt(e.full.Origin.X+e.full.Size.W/2, e.full.Origin.Y+e.full.Size.H/2))
		p.HandlePointer(PointerEvent{Kind: PointerTap, Pos: at, Button: MouseButtonLeft})
		return true
	}
	return false
}

// focusNode moves keyboard focus to the node's region, as Tab would, so the
// control shows its focus ring and every Revealer scrolls it into view.
func focusNode(in *inputState, e *semNode) bool {
	r := in.topmost(func(r *hitRegion) bool { return r.key != nil && sameAny(r.key, e.handler) })
	if r == nil {
		return false
	}
	in.focus(r, true)
	return true
}
