package a11y

import "slices"

// Source is the next tree as Builder.Build reads it: its nodes in paint
// order, each one's parent, and a way to tell whether a node of the tree
// before is still right.
type Source interface {
	// Len is the number of nodes.
	Len() int
	// Parent is the index of node i's parent, below i, or -1 for a root.
	Parent(i int) int
	// Same reports whether old already says everything node i would: every
	// field but Parent and Children, which Build compares itself.
	Same(i int, old *SemNode) bool
	// Fill writes node i into dst, which is zero. Build sets Parent and
	// Children afterwards. Nothing dst holds may be written again once Fill
	// returns: the tree is published to other goroutines.
	Fill(i int, dst *SemNode)
}

// Builder publishes trees that share every unchanged page of nodes with
// the tree before, so a frame that changed little allocates little. It
// keeps scratch from one build to the next and is for one goroutine; the
// trees it returns are frozen and are for any.
type Builder struct {
	start []int // node i's children are kids[start[i]:start[i+1]]
	next  []int // where the next child of node i goes in kids
	kids  []int
	roots []int
}

// Build assembles the tree src describes, focused naming the focused node
// or -1. Every page of prev whose nodes src calls Same, and whose links are
// unchanged, is shared rather than copied; when that is every page, and
// the roots and focus are unchanged too, Build returns prev itself. Pages
// are never written once a tree holding them has been returned, so prev
// and every tree before it stay exactly as they were.
func (b *Builder) Build(prev *SemTree, src Source, focused int) *SemTree {
	n := src.Len()
	b.link(src, n)
	if prev != nil && prev.n == n && prev.focused == focused && slices.Equal(prev.roots, b.roots) {
		if b.samePages(prev, src, n, nil) {
			return prev
		}
	}
	t := &SemTree{n: n, focused: focused, pages: make([][]SemNode, (n+pageMask)>>pageShift)}
	if prev != nil && slices.Equal(prev.roots, b.roots) {
		t.roots = prev.roots
	} else if len(b.roots) > 0 {
		t.roots = slices.Clone(b.roots)
	}
	b.samePages(prev, src, n, t.pages)
	for k := range t.pages {
		if t.pages[k] == nil {
			t.pages[k] = b.page(src, k, min(n, (k+1)<<pageShift))
		}
	}
	return t
}

// link counts and lists every node's children, in paint order, and the
// roots, into the builder's scratch.
func (b *Builder) link(src Source, n int) {
	b.start = slices.Grow(b.start[:0], n+1)[:n+1]
	clear(b.start)
	b.roots = b.roots[:0]
	for i := range n {
		if p := src.Parent(i); p >= 0 {
			b.start[p+1]++
		} else {
			b.roots = append(b.roots, i)
		}
	}
	for i := range n {
		b.start[i+1] += b.start[i]
	}
	b.next = append(b.next[:0], b.start[:n]...)
	b.kids = slices.Grow(b.kids[:0], b.start[n])[:b.start[n]]
	for i := range n {
		if p := src.Parent(i); p >= 0 {
			b.kids[b.next[p]] = i
			b.next[p]++
		}
	}
}

// samePages reports whether every page of prev still holds what src says.
// With out, it records each such page there and checks every page; without
// it, it stops at the first that differs.
func (b *Builder) samePages(prev *SemTree, src Source, n int, out [][]SemNode) bool {
	if prev == nil {
		return false
	}
	all := true
	for k := 0; k<<pageShift < n; k++ {
		lo, hi := k<<pageShift, min(n, (k+1)<<pageShift)
		if k >= len(prev.pages) || len(prev.pages[k]) != hi-lo || !b.samePage(prev.pages[k], src, lo) {
			if out == nil {
				return false
			}
			all = false
			continue
		}
		if out != nil {
			out[k] = prev.pages[k]
		}
	}
	return all
}

func (b *Builder) samePage(page []SemNode, src Source, lo int) bool {
	for j := range page {
		i, old := lo+j, &page[j]
		if old.Parent != src.Parent(i) || !slices.Equal(old.Children, b.kids[b.start[i]:b.start[i+1]]) || !src.Same(i, old) {
			return false
		}
	}
	return true
}

// page makes a fresh page of nodes lo to hi, with one array for all of
// their children.
func (b *Builder) page(src Source, k, hi int) []SemNode {
	lo := k << pageShift
	page := make([]SemNode, hi-lo)
	var kids []int
	if c := b.start[hi] - b.start[lo]; c > 0 {
		kids = slices.Clone(b.kids[b.start[lo]:b.start[hi]])
	}
	for j := range page {
		i, n := lo+j, &page[j]
		src.Fill(i, n)
		n.Parent = src.Parent(i)
		n.Children = nil
		if s, e := b.start[i]-b.start[lo], b.start[i+1]-b.start[lo]; e > s {
			n.Children = kids[s:e:e]
		}
		if len(n.Runs) > 0 {
			n.indexRuns()
		}
	}
	return page
}
