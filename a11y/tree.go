package a11y

import (
	"fmt"
	"iter"
	"strings"

	"github.com/ironpark/ggui/internal/fn"
)

// NodeID identifies a node from frame to frame. It is the handler's
// Identified ID when it has one, so a control that was rebuilt and moved in
// the same frame keeps its identity, and the Rect it painted otherwise --
// the same pair, in the same order, that Canvas.adopt matches hit regions
// by, rather than a second matcher that could disagree with the first. The
// Role comes along because a container and the one child that fills it do
// share a Rect: a EachKeyed list item and the row inside it.
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
			b.WriteString(n.Flags())
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Flags renders the state a node reports, leaving out whatever its role
// gives no meaning to.
func (n *SemNode) Flags() string {
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
		b.WriteString(fn.Pick(*n.Expanded, " expanded", " collapsed"))
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

// Build assembles a finished tree from nodes already in paint order, with
// each node's Parent set to the index of its parent or to -1, and focused
// set to the index of the focused node or to -1.
//
// It fills in the roots and every node's children. Counting first keeps
// this to two passes and one allocation for the whole tree, rather than
// growing a slice per parent as it goes.
//
// The tree is finished when it is returned and is never written again,
// which is what lets a bridge on another thread keep one as long as it
// likes.
func Build(nodes []SemNode, focused int) *SemTree {
	t := &SemTree{nodes: nodes, focused: focused}
	if len(nodes) == 0 {
		return t
	}
	counts := make([]int, len(nodes))
	for i := range nodes {
		if p := nodes[i].Parent; p >= 0 {
			counts[p]++
		} else {
			t.roots = append(t.roots, i)
		}
	}
	kids := make([]int, len(nodes)-len(t.roots))
	at := 0
	for i := range nodes {
		if counts[i] > 0 {
			nodes[i].Children = kids[at : at : at+counts[i]]
			at += counts[i]
		}
	}
	for i := range nodes {
		if p := nodes[i].Parent; p >= 0 {
			nodes[p].Children = append(nodes[p].Children, i)
		}
	}
	return t
}

// FocusIndex is where Focused sits in the tree, or -1 when nothing held
// focus. It is the index form of Focused, for a caller comparing one
// frame's focus with the next rather than reading the node.
func (t *SemTree) FocusIndex() int { return t.focused }
