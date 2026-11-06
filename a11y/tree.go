package a11y

import (
	"fmt"
	"iter"
	"strings"

	"github.com/ironpark/ggui/pkg/gem"
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
//
// The nodes are kept in pages of pageSize, every page full but the last.
// A tree that Builder.Build published shares each page whose nodes did not
// change with the tree before it, so a frame that changed one row copies
// one page rather than the whole tree. A shared page is never written
// again, by either tree.
type SemTree struct {
	pages   [][]SemNode
	n       int
	roots   []int
	focused int // index of the node holding keyboard focus, or -1
}

// pageSize is how many nodes a page holds: a power of two, so that finding
// a node is a shift and a mask. Smaller pages copy less around a change and
// cost a slice header each.
const (
	pageShift = 6
	pageSize  = 1 << pageShift
	pageMask  = pageSize - 1
)

// Len returns the number of nodes in the tree.
func (t *SemTree) Len() int { return t.n }

// At returns node i, in paint order: a parent always comes before its
// children, and siblings are in the order they were painted.
func (t *SemTree) At(i int) SemNode { return *t.Ref(i) }

// Ref is At without the copy: a pointer into the tree's own storage, for a
// caller that reads a field or two from many nodes. The tree is frozen, so
// the pointer stays valid; what it points at must not be written.
func (t *SemTree) Ref(i int) *SemNode {
	if uint(i) >= uint(t.n) {
		panic(fmt.Sprintf("a11y: node index %d out of range [0:%d]", i, t.n))
	}
	return &t.pages[i>>pageShift][i&pageMask]
}

// Roots returns the indices of the nodes with no parent.
func (t *SemTree) Roots() []int { return t.roots }

// Focused returns the node that held keyboard focus when the frame was
// published. Focus lives on the UI goroutine, so this mirror of it is the
// only version a bridge may read.
func (t *SemTree) Focused() (SemNode, bool) {
	if t.focused < 0 {
		return SemNode{}, false
	}
	return t.At(t.focused), true
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
		for i := range t.n {
			if !yield(i, *t.Ref(i)) {
				return
			}
		}
	}
}

// Nodes yields every node with the given role, in paint order, or every
// node when role is empty. Collect with slices.Collect when a slice is needed.
func (t *SemTree) Nodes(role Role) iter.Seq2[int, SemNode] {
	return func(yield func(int, SemNode) bool) {
		for i := range t.n {
			if n := t.Ref(i); (role == "" || n.Role == role) && !yield(i, *n) {
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
			n := t.Ref(i)
			if !yield(i, *n) {
				return
			}
			i = n.Parent
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
	for _, c := range t.Ref(i).Children {
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
			n := t.Ref(i)
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
	if n.Expanded.Expandable() {
		b.WriteString(gem.Pick(n.Expanded.Open(), " expanded", " collapsed"))
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
	if n.Level != 0 {
		fmt.Fprintf(&b, " level %d", n.Level)
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
// growing a slice per parent as it goes. A text node that froze its layout
// also gets the UTF-16 offset of each line, so that the offsets a platform
// text API asks in are converted from the start of a line, not of the text.
//
// The tree is finished when it is returned and is never written again,
// which is what lets a bridge on another thread keep one as long as it
// likes.
//
// The tree takes nodes as its storage without copying it; the caller must
// not write to nodes afterwards. Builder.Build is the form that shares
// storage with an earlier tree instead.
func Build(nodes []SemNode, focused int) *SemTree {
	t := &SemTree{n: len(nodes), focused: focused}
	for lo := 0; lo < len(nodes); lo += pageSize {
		hi := min(lo+pageSize, len(nodes))
		t.pages = append(t.pages, nodes[lo:hi:hi])
	}
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
		if len(nodes[i].Runs) > 0 {
			nodes[i].indexRuns()
		}
	}
	return t
}

// FocusIndex is where Focused sits in the tree, or -1 when nothing held
// focus. It is the index form of Focused, for a caller comparing one
// frame's focus with the next rather than reading the node.
func (t *SemTree) FocusIndex() int { return t.focused }
