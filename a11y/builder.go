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
	parents []int // node i's parent, as src.Parent gave it
	start   []int // node i's children are kids[start[i]:start[i+1]]
	next    []int // where the next child of node i goes in kids
	kids    []int
	roots   []int
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
	np := (n + pageMask) >> pageShift
	// pages is made only at the first page that differs from prev's, so
	// an unchanged frame allocates nothing.
	var pages [][]SemNode
	for k := range np {
		lo, hi := k<<pageShift, min(n, (k+1)<<pageShift)
		if prev != nil && k < len(prev.pages) && len(prev.pages[k]) == hi-lo && b.samePage(prev.pages[k], src, lo) {
			if pages != nil {
				pages[k] = prev.pages[k]
			}
			continue
		}
		if pages == nil {
			pages = make([][]SemNode, np)
			if prev != nil {
				copy(pages, prev.pages[:k])
			}
		}
		pages[k] = b.page(src, k, hi)
	}
	sameRoots := prev != nil && slices.Equal(prev.roots, b.roots)
	if pages == nil {
		if sameRoots && prev.n == n && prev.focused == focused {
			return prev
		}
		pages = make([][]SemNode, np)
		if prev != nil {
			copy(pages, prev.pages[:np])
		}
	}
	t := &SemTree{n: n, focused: focused, pages: pages}
	if sameRoots {
		t.roots = prev.roots
	} else if len(b.roots) > 0 {
		t.roots = slices.Clone(b.roots)
	}
	return t
}

// link records every node's parent, counts and lists its children, in
// paint order, and the roots, into the builder's scratch.
func (b *Builder) link(src Source, n int) {
	b.parents = slices.Grow(b.parents[:0], n)[:n]
	b.start = slices.Grow(b.start[:0], n+1)[:n+1]
	clear(b.start)
	b.roots = b.roots[:0]
	for i := range n {
		p := src.Parent(i)
		b.parents[i] = p
		if p >= 0 {
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
	for i, p := range b.parents {
		if p >= 0 {
			b.kids[b.next[p]] = i
			b.next[p]++
		}
	}
}

// samePage reports whether page, of the nodes from lo, still holds what
// src says.
func (b *Builder) samePage(page []SemNode, src Source, lo int) bool {
	for j := range page {
		i, old := lo+j, &page[j]
		if old.Parent != b.parents[i] || !slices.Equal(old.Children, b.kids[b.start[i]:b.start[i+1]]) || !src.Same(i, old) {
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
		n.Parent = b.parents[i]
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
