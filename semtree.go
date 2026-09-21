package ggui

import (
	"slices"

	"github.com/ironpark/ggui/a11y"
)

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
// from the main thread while Ebitengine runs frames on a goroutine of its
// own. Reading the live frame from there would be a race twice over, since
// App.Draw swaps and reuses its buffers every frame. So every frame ends by
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
