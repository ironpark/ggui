package ggui

import (
	"fmt"
	"strings"
)

// A platform accessibility API is a synchronous pull from a thread that is
// not the one ggui paints on: on every platform the assistive technology
// asks the process for the tree and blocks until it answers, and it asks
// from the main thread while Ebitengine runs frames on a goroutine of its
// own. Reading the live frame from there would be a race twice over, since
// App.Draw swaps and reuses its buffers every frame. So every frame ends by
// publishing a tree that is finished: allocated fresh, never written again,
// and handed out through an atomic pointer. A reader may keep one as long
// as it likes.

// NodeID identifies a node from frame to frame. It is the handler's
// Identified ID when it has one, so a control that was rebuilt and moved in
// the same frame keeps its identity, and the Rect it painted otherwise --
// the same pair, in the same order, that Canvas.adopt matches hit regions
// by, rather than a second matcher that could disagree with the first.
type NodeID struct {
	ID   any
	Rect Rect
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
	for i := range t.nodes {
		n := &t.nodes[i]
		if (role == "" || n.Role == role) && (name == "" || n.Name == name) {
			return *n, true
		}
	}
	return SemNode{}, false
}

// FindAll returns every node with the given role, in paint order, or every
// node when role is empty.
func (t *SemTree) FindAll(role Role) []SemNode {
	var out []SemNode
	for i := range t.nodes {
		if role == "" || t.nodes[i].Role == role {
			out = append(out, t.nodes[i])
		}
	}
	return out
}

// Ancestors returns node i and everything above it, innermost first. The
// inspector shows the chain, and a bridge needs it to answer "what am I
// inside of".
func (t *SemTree) Ancestors(i int) []SemNode {
	var out []SemNode
	for i >= 0 {
		out = append(out, t.nodes[i])
		i = t.nodes[i].Parent
	}
	return out
}

// String renders the tree as indented lines, one per node, with the flags a
// role gives meaning to. It is what a snapshot test compares, so that a test
// asserts the shape of the tree and not merely that something was present.
func (t *SemTree) String() string {
	var b strings.Builder
	var walk func(i, depth int)
	walk = func(i, depth int) {
		n := &t.nodes[i]
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteString(string(n.Role))
		if n.Name != "" {
			fmt.Fprintf(&b, " %q", n.Name)
		}
		b.WriteString(n.flags())
		b.WriteByte('\n')
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	for _, r := range t.roots {
		walk(r, 0)
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

// buildSemTree freezes what a paint described into a tree that outlives the
// frame. Everything is allocated here and nothing is reused, since the tree
// published last frame may still be being read.
func buildSemTree(c *Canvas, focused *hitRegion) *SemTree {
	t := &SemTree{focused: -1}
	if len(c.sem) == 0 {
		return t
	}
	t.nodes = make([]SemNode, len(c.sem))
	for i := range c.sem {
		e := &c.sem[i]
		t.nodes[i] = SemNode{
			Node:   e.node,
			ID:     NodeID{ID: e.id, Rect: e.full},
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
	t.focused = focusedNode(c, focused)
	return t
}

// focusedNode finds the node the focused hit region belongs to: by the
// handler itself, which is exact, then by identity and Rect, which is how
// input matches a region across a rebuild.
func focusedNode(c *Canvas, focused *hitRegion) int {
	if focused == nil || focused.key == nil {
		return -1
	}
	for i := range c.sem {
		if sameAny(c.sem[i].handler, focused.key) {
			return i
		}
	}
	for i := range c.sem {
		e := &c.sem[i]
		if e.id != nil && e.id == focused.id || e.id == nil && focused.id == nil && e.rect == focused.rect {
			return i
		}
	}
	return -1
}
