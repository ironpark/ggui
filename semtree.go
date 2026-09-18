package ggui

import (
	"fmt"
	"iter"
	"slices"
	"strings"
)

// A platform accessibility API is a synchronous pull from a thread that is
// not the one ggui paints on: on every platform the assistive technology
// asks the process for the tree and blocks until it answers, and it asks
// from the main thread while Ebitengine runs frames on a goroutine of its
// own. Reading the live frame from there would be a race twice over, since
// App.Draw swaps and reuses its buffers every frame. So every frame ends by
// publishing a tree that is finished: never written again after publication,
// and handed out through an atomic pointer. A reader may keep one as long
// as it likes.

// NodeID identifies a node from frame to frame. It is the handler's
// Identified ID when it has one, so a control that was rebuilt and moved in
// the same frame keeps its identity, and the Rect it painted otherwise --
// the same pair, in the same order, that Canvas.adopt matches hit regions
// by, rather than a second matcher that could disagree with the first. The
// Role comes along because a container and the one child that fills it do
// share a Rect: a For list item and the row inside it.
// ID is an opaque identity token, not a copied description; keep identity
// values stable while a widget or one of its snapshots is alive.
type NodeID struct {
	ID   any
	Rect Rect
	Role Role
}

// SemNode is one element of a published SemTree: what the widget said about
// itself, where it went, and how it hangs among the others. Parent and
// Children are indices into the tree; Parent is -1 for a root.
type SemNode struct {
	Node
	ID       NodeID
	Rect     Rect // as clipped, which is empty for an Offscreen node
	Full     Rect // as painted, before clipping: where to scroll to
	Parent   int
	Children []int
}

// SemTree is one frame's accessibility tree, finished and frozen. Nothing
// writes to it after App.Draw or Probe.Frame published it, so it is safe to
// read from any goroutine and at any time; the slices it hands back are
// shared with every other reader and must not be written to.
type SemTree struct {
	nodes   []SemNode
	roots   []int
	focused int // index of the node holding keyboard focus, or -1
}

// Len returns the number of nodes in the tree.
func (t *SemTree) Len() int { return len(t.nodes) }

// At returns node i, in paint order: a parent always comes before its
// children, and siblings are in the order they were painted.
func (t *SemTree) At(i int) SemNode { return t.nodes[i] }

// Roots returns the indices of the nodes with no parent.
func (t *SemTree) Roots() []int { return t.roots }

// Focused returns the node that held keyboard focus when the frame was
// published. Focus lives on the UI goroutine, so this mirror of it is the
// only version a bridge may read.
func (t *SemTree) Focused() (SemNode, bool) {
	if t.focused < 0 {
		return SemNode{}, false
	}
	return t.nodes[t.focused], true
}

// Find returns the first node with the given role and name; either may be
// empty to match any.
func (t *SemTree) Find(role Role, name string) (SemNode, bool) {
	for _, n := range t.Nodes(role) {
		if name == "" || n.Name == name {
			return n, true
		}
	}
	return SemNode{}, false
}

// All yields every node with its index, in paint order.
func (t *SemTree) All() iter.Seq2[int, SemNode] {
	return func(yield func(int, SemNode) bool) {
		for i := range t.nodes {
			if !yield(i, t.nodes[i]) {
				return
			}
		}
	}
}

// Nodes yields every node with the given role, in paint order, or every
// node when role is empty. Collect with slices.Collect when a slice is needed.
func (t *SemTree) Nodes(role Role) iter.Seq2[int, SemNode] {
	return func(yield func(int, SemNode) bool) {
		for i := range t.nodes {
			if (role == "" || t.nodes[i].Role == role) && !yield(i, t.nodes[i]) {
				return
			}
		}
	}
}

// Ancestors yields node i and everything above it, innermost first. The
// inspector shows the chain, and a bridge needs it to answer "what am I
// inside of".
func (t *SemTree) Ancestors(i int) iter.Seq2[int, SemNode] {
	return func(yield func(int, SemNode) bool) {
		for i >= 0 {
			if !yield(i, t.nodes[i]) {
				return
			}
			i = t.nodes[i].Parent
		}
	}
}

// Walk yields node i and its whole subtree in paint order, each with its
// depth below i, so a renderer can indent without recursing itself.
func (t *SemTree) Walk(i int) iter.Seq2[int, int] {
	return func(yield func(int, int) bool) {
		t.walk(i, 0, yield)
	}
}
func (t *SemTree) walk(i, depth int, yield func(int, int) bool) bool {
	if !yield(i, depth) {
		return false
	}
	for _, c := range t.nodes[i].Children {
		if !t.walk(c, depth+1, yield) {
			return false
		}
	}
	return true
}

// String renders the tree as indented lines, one per node, with the flags a
// role gives meaning to. It is what a snapshot test compares, so that a test
// asserts the shape of the tree and not merely that something was present.
func (t *SemTree) String() string {
	var b strings.Builder
	for _, r := range t.roots {
		for i, depth := range t.Walk(r) {
			n := &t.nodes[i]
			b.WriteString(strings.Repeat("  ", depth))
			b.WriteString(string(n.Role))
			if n.Name != "" {
				fmt.Fprintf(&b, " %q", n.Name)
			}
			b.WriteString(n.flags())
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// flags renders the state a node reports, leaving out whatever its role
// gives no meaning to.
func (n *SemNode) flags() string {
	var b strings.Builder
	if n.Value != "" {
		fmt.Fprintf(&b, " value=%q", n.Value)
	}
	switch n.Checked {
	case TriOff:
		b.WriteString(" unchecked")
	case TriOn:
		b.WriteString(" checked")
	case TriMixed:
		b.WriteString(" mixed")
	}
	if n.Expanded != nil {
		b.WriteString(pick(*n.Expanded, " expanded", " collapsed"))
	}
	if n.Selected {
		b.WriteString(" selected")
	}
	if n.Disabled {
		b.WriteString(" disabled")
	}
	if n.Offscreen {
		b.WriteString(" offscreen")
	}
	if n.Max != 0 || n.Now != 0 {
		fmt.Fprintf(&b, " %g of %g..%g", n.Now, n.Min, n.Max)
	}
	return b.String()
}

// buildSemTree reuses the last snapshot only when all observable values are
// equal. Changed frames own fresh storage; readers can retain any old tree.
func buildSemTree(c *Canvas, focused *hitRegion, prev *SemTree) *SemTree {
	focus := focusedNode(c, focused)
	if sameSemTree(c, focus, prev) {
		return prev
	}
	t := &SemTree{focused: focus}
	if len(c.sem) == 0 {
		return t
	}
	t.nodes = make([]SemNode, len(c.sem))
	for i := range c.sem {
		e := &c.sem[i]
		t.nodes[i] = SemNode{
			Node:   freezeNode(e.node),
			ID:     NodeID{ID: e.id, Rect: e.full, Role: e.node.Role},
			Rect:   e.rect,
			Full:   e.full,
			Parent: e.parent - 1,
		}
	}
	// Children in paint order. Counting first keeps this two passes and one
	// allocation per parent, rather than growing every slice as it goes.
	counts := make([]int, len(t.nodes))
	for i := range t.nodes {
		if p := t.nodes[i].Parent; p >= 0 {
			counts[p]++
		} else {
			t.roots = append(t.roots, i)
		}
	}
	kids := make([]int, len(t.nodes)-len(t.roots))
	at := 0
	for i := range t.nodes {
		if counts[i] > 0 {
			t.nodes[i].Children = kids[at : at : at+counts[i]]
			at += counts[i]
		}
	}
	for i := range t.nodes {
		if p := t.nodes[i].Parent; p >= 0 {
			t.nodes[p].Children = append(t.nodes[p].Children, i)
		}
	}
	return t
}

// Parent indices and paint order fully determine roots and child lists.
// Handlers, scopes and groups are frame-local and are not published.
func sameSemTree(c *Canvas, focus int, prev *SemTree) bool {
	if prev == nil || prev.focused != focus || len(prev.nodes) != len(c.sem) {
		return false
	}
	for i := range c.sem {
		a, b := &c.sem[i], &prev.nodes[i]
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
		a.Actions != b.Actions || a.Offscreen != b.Offscreen ||
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
	if i := c.semIndex[semKey(focused.key)]; i != 0 {
		return i - 1
	}
	for i := range c.sem {
		e := &c.sem[i]
		if e.id != nil && e.id == focused.id || e.id == nil && focused.id == nil && e.rect == focused.rect {
			return i
		}
	}
	return -1
}
