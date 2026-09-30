package a11ybridge

import (
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/a11y"
)

// axFake is an axPlatform that makes numbers instead of objects, so that
// everything above the platform seam can be tested without an assistive
// technology, a window, or Objective-C.
type axFake struct {
	on      bool
	next    uintptr
	made    []int64
	freed   []uintptr
	asked   int
	notes   []axNote
	flushes int
	handles map[uintptr]int64
}

func newAXFake(on bool) *axFake {
	return &axFake{on: on, handles: map[uintptr]int64{}}
}

func (f *axFake) active() bool { f.asked++; return f.on }

func (f *axFake) setWindow(*Bridge, *ggfx.Window) {}
func (f *axFake) close()                          {}

func (f *axFake) element(handle int64) uintptr {
	f.next++
	f.made = append(f.made, handle)
	f.handles[f.next] = handle
	return f.next
}

func (f *axFake) release(elems []uintptr) { f.freed = append(f.freed, elems...) }

func (f *axFake) notify(notes []axNote) { f.notes = append(f.notes, notes...); f.flushes++ }

// axRoots builds a tree of unnested nodes, which is all the element cache
// and the gating care about.
func axRoots(nodes ...a11y.SemNode) *a11y.SemTree {
	out := make([]a11y.SemNode, 0, len(nodes))
	for _, n := range nodes {
		n.Parent = -1
		if n.ID.Role == "" {
			n.ID.Role = n.Role
		}
		out = append(out, n)
	}
	return a11y.Build(out, -1)
}

func axNode(role a11y.Role, name string, id any, r a11y.Rect) a11y.SemNode {
	return a11y.SemNode{
		Role: role, Name: name,
		ID:   a11y.NodeID{ID: id, Rect: r, Role: role},
		Rect: r,
		Full: r,
	}
}

func TestAXKeyFollowsIdentityNotPosition(t *testing.T) {
	// A control that was rebuilt somewhere else is the same element, or
	// VoiceOver loses its place every time a list reflows.
	before := axKeyOf(a11y.NodeID{ID: "save", Rect: a11y.Rct(a11y.Pt(0, 0), a11y.Sz(10, 10)), Role: a11y.RoleButton})
	after := axKeyOf(a11y.NodeID{ID: "save", Rect: a11y.Rct(a11y.Pt(0, 40), a11y.Sz(10, 10)), Role: a11y.RoleButton})
	if before != after {
		t.Error("an identified node changed key by moving")
	}
	// Without an identity there is nothing to go on but where it painted.
	anon := axKeyOf(a11y.NodeID{Rect: a11y.Rct(a11y.Pt(0, 0), a11y.Sz(10, 10)), Role: a11y.RoleButton})
	moved := axKeyOf(a11y.NodeID{Rect: a11y.Rct(a11y.Pt(0, 40), a11y.Sz(10, 10)), Role: a11y.RoleButton})
	if anon == moved {
		t.Error("an unidentified node kept its key across a move")
	}
	// The role separates a container from the one child that fills it.
	if axKeyOf(a11y.NodeID{Rect: a11y.Rct(a11y.Pt(0, 0), a11y.Sz(10, 10)), Role: a11y.RoleRow}) == anon {
		t.Error("two roles at the same bounds share a key")
	}
	// An identity that cannot be a map key must not panic the lookup.
	k := axKeyOf(a11y.NodeID{ID: []int{1}, Rect: a11y.Rct(a11y.Pt(1, 2), a11y.Sz(3, 4)), Role: a11y.RoleText})
	if k.id != nil || k.rect != a11y.Rct(a11y.Pt(1, 2), a11y.Sz(3, 4)) {
		t.Errorf("uncomparable identity = %v, want a fall back to the bounds", k)
	}
}

func TestAXElementsAreStableAndSweptByLiveness(t *testing.T) {
	f := newAXFake(true)
	b := &Bridge{plat: f, mode: a11y.Always}
	keep := axNode(a11y.RoleButton, "keep", "keep", a11y.Rct(a11y.Pt(0, 0), a11y.Sz(10, 10)))
	goes := axNode(a11y.RoleButton, "goes", "goes", a11y.Rct(a11y.Pt(0, 20), a11y.Sz(10, 10)))
	b.Publish(axRoots(keep, goes), nil)

	e1 := b.element(axKeyOf(keep.ID))
	e2 := b.element(axKeyOf(goes.ID))
	if e1 == 0 || e2 == 0 || e1 == e2 {
		t.Fatalf("elements = %d, %d; want two distinct objects", e1, e2)
	}
	if again := b.element(axKeyOf(keep.ID)); again != e1 {
		t.Errorf("element = %d, want the same object %d as last time", again, e1)
	}

	// The node changed everything but its identity: no new object, and
	// nothing released. Only liveness is diffed.
	moved := keep
	moved.Name, moved.Full, moved.ID.Rect = "renamed", a11y.Rct(a11y.Pt(5, 5), a11y.Sz(20, 20)), a11y.Rct(a11y.Pt(5, 5), a11y.Sz(20, 20))
	b.Publish(axRoots(moved), nil)
	if len(f.freed) != 1 || f.freed[0] != e2 {
		t.Errorf("freed = %v, want just the node that disappeared (%d)", f.freed, e2)
	}
	if again := b.element(axKeyOf(moved.ID)); again != e1 {
		t.Errorf("element = %d, want the surviving object %d", again, e1)
	}
	if b.element(axKeyOf(goes.ID)) != 0 {
		t.Error("a node that left the tree still has an element")
	}
}

func TestAXStaleHandleResolvesToNothing(t *testing.T) {
	// An assistive technology keeps the elements it is given, so a handle
	// can outlive its node and even its slot. It must answer nothing
	// rather than whatever moved in.
	f := newAXFake(true)
	b := &Bridge{plat: f, mode: a11y.Always}
	gone := axNode(a11y.RoleButton, "gone", "gone", a11y.Rct(a11y.Pt(0, 0), a11y.Sz(10, 10)))
	b.Publish(axRoots(gone), nil)
	e := b.element(axKeyOf(gone.ID))
	stale := f.handles[e]

	b.Publish(axRoots(), nil)
	next := axNode(a11y.RoleButton, "next", "next", a11y.Rct(a11y.Pt(0, 0), a11y.Sz(10, 10)))
	b.Publish(axRoots(next), nil)
	if b.element(axKeyOf(next.ID)) == 0 {
		t.Fatal("the new node got no element")
	}
	if n, ok := b.node(stale); ok {
		t.Errorf("a retired handle resolved to %v", n.Name)
	}
}

func TestAXGatingCostsNothingWhileNobodyIsListening(t *testing.T) {
	f := newAXFake(false)
	b := &Bridge{plat: f, mode: a11y.Auto}
	n := axNode(a11y.RoleButton, "x", "x", a11y.Rct(a11y.Pt(0, 0), a11y.Sz(10, 10)))
	for range axPollFrames * 2 {
		b.Publish(axRoots(n), nil)
	}
	if b.frame() != nil {
		t.Error("a tree was published while no assistive technology was attached")
	}
	if f.asked != 2 {
		t.Errorf("asked the platform %d times in %d frames, want 2", f.asked, axPollFrames*2)
	}

	// Attaching turns it on at the next poll, and detaching drops every
	// element rather than holding them for a listener that has gone.
	f.on = true
	for range axPollFrames + 1 {
		b.Publish(axRoots(n), nil)
	}
	if b.frame() == nil {
		t.Fatal("nothing was published after VoiceOver attached")
	}
	e := b.element(axKeyOf(n.ID))
	f.on = false
	for range axPollFrames + 1 {
		b.Publish(axRoots(n), nil)
	}
	if len(f.freed) != 1 || f.freed[0] != e {
		t.Errorf("freed = %v, want the one element (%d) dropped on detach", f.freed, e)
	}
}

func TestAXOffNeverTouchesThePlatform(t *testing.T) {
	b := &Bridge{mode: a11y.Off}
	b.Start(nil, a11y.Off)
	if b.plat != nil {
		t.Fatal("Off built a platform bridge")
	}
	b.Publish(axRoots(axNode(a11y.RoleButton, "x", "x", a11y.Rct(a11y.Pt(0, 0), a11y.Sz(10, 10)))), nil)
	if b.frame() != nil {
		t.Error("Off published a tree")
	}
}

func TestAXValuesFollowTheRole(t *testing.T) {
	for _, c := range []struct {
		name string
		node a11y.Node
		want float64
		num  bool
	}{
		{"unticked", a11y.Node{Role: a11y.RoleCheckbox, Checked: a11y.TriOff}, 0, true},
		{"ticked", a11y.Node{Role: a11y.RoleCheckbox, Checked: a11y.TriOn}, 1, true},
		{"mixed", a11y.Node{Role: a11y.RoleCheckbox, Checked: a11y.TriMixed}, 2, true},
		{"slider", a11y.Node{Role: a11y.RoleSlider, Now: 7}, 7, true},
		{"progress", a11y.Node{Role: a11y.RoleProgress, Now: 0.5}, 0.5, true},
		{"text", a11y.Node{Role: a11y.RoleTextField, Value: "hi"}, 0, false},
		{"plain", a11y.Node{Role: a11y.RoleButton}, 0, false},
	} {
		got, num := axNumber(c.node)
		if got != c.want || num != c.num {
			t.Errorf("%s: axNumber = %g, %v; want %g, %v", c.name, got, num, c.want, c.num)
		}
	}
	if _, _, ok := axRange(a11y.Node{Role: a11y.RoleButton, Min: 1, Max: 2}); ok {
		t.Error("a button reported a range")
	}
	if _, _, ok := axRange(a11y.Node{Role: a11y.RoleSlider}); ok {
		t.Error("a slider that gave no range reported one anyway")
	}
	if lo, hi, ok := axRange(a11y.Node{Role: a11y.RoleSlider, Min: 1, Max: 9}); !ok || lo != 1 || hi != 9 {
		t.Errorf("slider range = %g..%g, %v; want 1..9", lo, hi, ok)
	}
}

func TestAXHitTestFindsTheDeepestNode(t *testing.T) {
	// The array is in paint order, so a scan would answer with the last
	// node painted; a hit test must answer with the innermost one.
	tree := axHitTree()
	if got := axHitTest(tree, a11y.Pt(30, 30)); got != 1 {
		t.Errorf("hit at 30,30 = %d, want the child (1)", got)
	}
	if got := axHitTest(tree, a11y.Pt(90, 90)); got != 0 {
		t.Errorf("hit at 90,90 = %d, want the group (0)", got)
	}
	if got := axHitTest(tree, a11y.Pt(500, 500)); got != -1 {
		t.Errorf("hit outside everything = %d, want -1", got)
	}
	if got := axHitTest(tree, a11y.Pt(30, 130)); got != -1 {
		t.Errorf("hit on the offscreen node = %d; it is not where it says it is", got)
	}
}

// axHitTree is a group with a visible child and a clipped one.
func axHitTree() *a11y.SemTree {
	return a11y.Build([]a11y.SemNode{
		{Role: a11y.RoleGroup, Full: a11y.Rct(a11y.Pt(0, 0), a11y.Sz(100, 100)), Parent: -1},
		{Role: a11y.RoleButton, Full: a11y.Rct(a11y.Pt(10, 10), a11y.Sz(50, 50)), Parent: 0},
		{Role: a11y.RoleButton, Offscreen: true, Full: a11y.Rct(a11y.Pt(10, 110), a11y.Sz(50, 50)), Parent: 0},
	}, -1)
}

// axParent builds a container holding the given children, which is what the
// selection notification needs: it says which of a container's children is
// now the chosen one, so it goes to the container.
func axParent(parent a11y.SemNode, kids ...a11y.SemNode) *a11y.SemTree {
	return axParentFocus(-1, parent, kids...)
}

// axParentFocus is axParent with one of the nodes holding keyboard focus,
// named by its index in the tree.
func axParentFocus(focus int, parent a11y.SemNode, kids ...a11y.SemNode) *a11y.SemTree {
	parent.Parent = -1
	for i := range kids {
		kids[i].Parent = 0
	}
	return a11y.Build(append([]a11y.SemNode{parent}, kids...), focus)
}

func axKinds(notes []axNote) []axNotice {
	out := make([]axNotice, len(notes))
	for i, n := range notes {
		out[i] = n.kind
	}
	return out
}

func axFrameOf(t *a11y.SemTree) *axFrame { return newAXFrame(t) }

func TestAXDiffSaysOnlyWhatChanged(t *testing.T) {
	one := axNode(a11y.RoleCheckbox, "a", "a", a11y.Rct(a11y.Pt(0, 0), a11y.Sz(10, 10)))
	one.Checked = a11y.TriOff
	two := axNode(a11y.RoleCheckbox, "b", "b", a11y.Rct(a11y.Pt(0, 20), a11y.Sz(10, 10)))
	first := axFrameOf(axRoots(one, two))

	if got := axKinds(axDiff(nil, first)); len(got) != 1 || got[0] != axLayoutChanged {
		t.Errorf("the first tree = %v, want one layout change", got)
	}
	// A frame in which nothing moved says nothing. This is the common case
	// and the one that must not chatter at sixty frames a second.
	if got := axDiff(first, axFrameOf(axRoots(one, two))); len(got) != 0 {
		t.Errorf("an unchanged tree = %v, want nothing", got)
	}

	ticked := one
	ticked.Checked = a11y.TriOn
	notes := axDiff(first, axFrameOf(axRoots(ticked, two)))
	if len(notes) != 1 || notes[0].kind != axValueChanged || notes[0].key != axKeyOf(one.ID) {
		t.Errorf("ticking = %v, want one value change on the check box", notes)
	}
	// A rename is not spoken: the node is read again when it is reached,
	// and a rebuild that rewrote every label would talk over the user.
	renamed := one
	renamed.Name = "different"
	if got := axDiff(first, axFrameOf(axRoots(renamed, two))); len(got) != 0 {
		t.Errorf("renaming = %v, want nothing", got)
	}

	if got := axKinds(axDiff(first, axFrameOf(axRoots(one)))); len(got) != 1 || got[0] != axLayoutChanged {
		t.Errorf("removing a node = %v, want one layout change", got)
	}
}

func TestAXDiffReportsFocusAndSelection(t *testing.T) {
	list := axNode(a11y.RoleList, "", "list", a11y.Rct(a11y.Pt(0, 0), a11y.Sz(50, 50)))
	a := axNode(a11y.RoleOption, "a", "a", a11y.Rct(a11y.Pt(0, 0), a11y.Sz(50, 20)))
	b := axNode(a11y.RoleOption, "b", "b", a11y.Rct(a11y.Pt(0, 20), a11y.Sz(50, 20)))
	a.Selected = true
	before := axFrameOf(axParent(list, a, b))

	a.Selected, b.Selected = false, true
	notes := axDiff(before, axFrameOf(axParent(list, a, b)))
	if len(notes) != 2 {
		t.Fatalf("notes = %v, want one per option whose selection moved", notes)
	}
	for _, n := range notes {
		if n.kind != axSelectionChanged || n.key != axKeyOf(list.ID) {
			t.Errorf("note = %v, want a selection change addressed to the list", n)
		}
	}

	// Focus is compared by identity, so a rebuild that shifted every node
	// along is not a focus move.
	focused := axParentFocus(1, list, a, b)
	moved := axDiff(before, axFrameOf(focused))
	if len(moved) == 0 || moved[len(moved)-1].kind != axFocusChanged {
		t.Fatalf("notes = %v, want a focus change last", moved)
	}
	same := axParentFocus(1, list, a, b)
	if got := axDiff(axFrameOf(focused), axFrameOf(same)); len(got) != 0 {
		t.Errorf("focus that stayed put = %v, want nothing", got)
	}
}

func TestAXNotifiesOncePerFrameAndOnlyWhenThereIsNews(t *testing.T) {
	f := newAXFake(true)
	b := &Bridge{plat: f, mode: a11y.Always}
	n := axNode(a11y.RoleSlider, "vol", "vol", a11y.Rct(a11y.Pt(0, 0), a11y.Sz(10, 10)))
	b.Publish(axRoots(n), nil)
	if f.flushes != 1 {
		t.Fatalf("flushes = %d, want the first tree to be announced once", f.flushes)
	}
	for range 10 {
		b.Publish(axRoots(n), nil)
	}
	if f.flushes != 1 {
		t.Errorf("flushes = %d after ten still frames, want 1", f.flushes)
	}
	n.Now = 5
	b.Publish(axRoots(n), []a11y.Announcement{{Text: "muted", Politeness: a11y.Assertive}})
	if f.flushes != 2 {
		t.Fatalf("flushes = %d, want one hop for the frame that changed", f.flushes)
	}
	last := f.notes[len(f.notes)-1]
	if last.kind != axAnnouncement || last.text != "muted" || !last.loud || !last.root {
		t.Errorf("announcement = %v, want an assertive one addressed to the window", last)
	}
}

func TestAXReusedSnapshotStillAnnouncesAndPolls(t *testing.T) {
	oldDetail := a11y.WantsDetail()
	defer a11y.SetWantsDetail(oldDetail)
	f := newAXFake(true)
	b := &Bridge{plat: f, mode: a11y.Auto}
	tree := axRoots(axNode(a11y.RoleButton, "save", "save", a11y.Rect{}))
	b.Publish(tree, nil)
	first := b.frame()
	elem := b.element(axKeyOf(tree.At(0).ID))
	b.Publish(tree, []a11y.Announcement{{Text: "saved", Politeness: a11y.Polite}})
	if b.frame() != first || f.flushes != 2 || f.notes[len(f.notes)-1].text != "saved" {
		t.Fatal("reused tree rebuilt the index or lost an announcement")
	}
	f.on, b.poll = false, 0
	b.Publish(tree, nil)
	if b.frame() != nil || len(f.freed) != 1 || f.freed[0] != elem {
		t.Fatal("reused tree prevented detach cleanup")
	}
	f.on, b.poll = true, 0
	b.Publish(tree, nil)
	if b.frame() == nil || b.frame() == first || b.frame().tree != tree {
		t.Fatal("same tree did not republish on reattach")
	}
}

func TestAXActionsAreOnlyWhatTheNodeClaims(t *testing.T) {
	slider := a11y.Node{Role: a11y.RoleSlider, Actions: a11y.ActionIncrement | a11y.ActionDecrement | a11y.ActionFocus}
	if axAllows(slider, axPress) {
		t.Error("a slider offered a press it never claimed")
	}
	if !axAllows(slider, axIncrement) || !axAllows(slider, axDecrement) {
		t.Error("a slider withheld the actions it claimed")
	}
	// A disabled control is in the tree to be read out, not to be worked:
	// everything but looking at it is refused.
	off := a11y.Node{Role: a11y.RoleButton, Actions: a11y.ActionPress | a11y.ActionFocus, Disabled: true}
	if axAllows(off, axPress) || axAllows(off, axConfirm) {
		t.Error("a disabled button could be pressed")
	}
	if !axAllows(off, axSetFocus) {
		t.Error("a disabled button could not even be focused")
	}
	if axActOf(axConfirm) != a11y.ActionPress || axActOf(axShowMenu) != a11y.ActionExpand {
		t.Error("the AppKit actions map onto the wrong ggui ones")
	}
}

func TestAXPressReachesTheApp(t *testing.T) {
	// The bridge's half of the round trip: an element resolves its node
	// out of the published tree and asks the app to press it. What the app
	// then does to the widget is the other half, and is tested where the
	// widgets are; see TestAXPressRingsTheWidget in the ggui package.
	var got []a11y.Action
	var at []a11y.NodeID
	b := &Bridge{plat: newAXFake(true), mode: a11y.Always,
		act: func(id a11y.NodeID, a a11y.Action) { at, got = append(at, id), append(got, a) }}
	button := axNode(a11y.RoleButton, "ring", "bell", a11y.Rct(a11y.Pt(0, 0), a11y.Sz(40, 20)))
	button.Actions = a11y.ActionPress | a11y.ActionFocus
	b.Publish(axRoots(button), nil)

	node, ok := b.frame().tree.Find(a11y.RoleButton, "ring")
	if !ok {
		t.Fatal("the button is not in the published tree")
	}
	if !axAllows(node.Node, axPress) {
		t.Fatal("the button does not offer a press")
	}
	b.perform(node.ID, a11y.Action{Kind: a11y.ActionPress})
	if len(got) != 1 || got[0].Kind != a11y.ActionPress {
		t.Fatalf("actions = %v, want one press", got)
	}
	if len(at) != 1 || at[0].ID != "bell" {
		t.Errorf("press aimed at %v, want the button", at)
	}
}
