package ggui

import (
	"testing"
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
	handles map[uintptr]int64
}

func newAXFake(on bool) *axFake {
	return &axFake{on: on, handles: map[uintptr]int64{}}
}

func (f *axFake) active() bool { f.asked++; return f.on }

func (f *axFake) element(handle int64) uintptr {
	f.next++
	f.made = append(f.made, handle)
	f.handles[f.next] = handle
	return f.next
}

func (f *axFake) release(elems []uintptr) { f.freed = append(f.freed, elems...) }

// axRoots builds a tree of unnested nodes, which is all the element cache
// and the gating care about.
func axRoots(nodes ...SemNode) *SemTree {
	t := &SemTree{focused: -1}
	for i, n := range nodes {
		n.Parent = -1
		if n.ID.Role == "" {
			n.ID.Role = n.Role
		}
		t.nodes = append(t.nodes, n)
		t.roots = append(t.roots, i)
	}
	return t
}

func axNode(role Role, name string, id any, r Rect) SemNode {
	return SemNode{
		Node: Node{Role: role, Name: name},
		ID:   NodeID{ID: id, Rect: r, Role: role},
		Rect: r,
		Full: r,
	}
}

func TestAXKeyFollowsIdentityNotPosition(t *testing.T) {
	// A control that was rebuilt somewhere else is the same element, or
	// VoiceOver loses its place every time a list reflows.
	before := axKeyOf(NodeID{ID: "save", Rect: Rct(Pt(0, 0), Sz(10, 10)), Role: RoleButton})
	after := axKeyOf(NodeID{ID: "save", Rect: Rct(Pt(0, 40), Sz(10, 10)), Role: RoleButton})
	if before != after {
		t.Error("an identified node changed key by moving")
	}
	// Without an identity there is nothing to go on but where it painted.
	anon := axKeyOf(NodeID{Rect: Rct(Pt(0, 0), Sz(10, 10)), Role: RoleButton})
	moved := axKeyOf(NodeID{Rect: Rct(Pt(0, 40), Sz(10, 10)), Role: RoleButton})
	if anon == moved {
		t.Error("an unidentified node kept its key across a move")
	}
	// The role separates a container from the one child that fills it.
	if axKeyOf(NodeID{Rect: Rct(Pt(0, 0), Sz(10, 10)), Role: RoleRow}) == anon {
		t.Error("two roles at the same bounds share a key")
	}
	// An identity that cannot be a map key must not panic the lookup.
	k := axKeyOf(NodeID{ID: []int{1}, Rect: Rct(Pt(1, 2), Sz(3, 4)), Role: RoleText})
	if k.id != nil || k.rect != Rct(Pt(1, 2), Sz(3, 4)) {
		t.Errorf("uncomparable identity = %v, want a fall back to the bounds", k)
	}
}

func TestAXElementsAreStableAndSweptByLiveness(t *testing.T) {
	f := newAXFake(true)
	b := &axBridge{plat: f, mode: AccessibilityAlways}
	keep := axNode(RoleButton, "keep", "keep", Rct(Pt(0, 0), Sz(10, 10)))
	goes := axNode(RoleButton, "goes", "goes", Rct(Pt(0, 20), Sz(10, 10)))
	b.publish(axRoots(keep, goes))

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
	moved.Name, moved.Full, moved.ID.Rect = "renamed", Rct(Pt(5, 5), Sz(20, 20)), Rct(Pt(5, 5), Sz(20, 20))
	b.publish(axRoots(moved))
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
	b := &axBridge{plat: f, mode: AccessibilityAlways}
	gone := axNode(RoleButton, "gone", "gone", Rct(Pt(0, 0), Sz(10, 10)))
	b.publish(axRoots(gone))
	e := b.element(axKeyOf(gone.ID))
	stale := f.handles[e]

	b.publish(axRoots())
	next := axNode(RoleButton, "next", "next", Rct(Pt(0, 0), Sz(10, 10)))
	b.publish(axRoots(next))
	if b.element(axKeyOf(next.ID)) == 0 {
		t.Fatal("the new node got no element")
	}
	if n, ok := b.node(stale); ok {
		t.Errorf("a retired handle resolved to %v", n.Name)
	}
}

func TestAXGatingCostsNothingWhileNobodyIsListening(t *testing.T) {
	f := newAXFake(false)
	b := &axBridge{plat: f, mode: AccessibilityAuto}
	n := axNode(RoleButton, "x", "x", Rct(Pt(0, 0), Sz(10, 10)))
	for range axPollFrames * 2 {
		b.publish(axRoots(n))
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
		b.publish(axRoots(n))
	}
	if b.frame() == nil {
		t.Fatal("nothing was published after VoiceOver attached")
	}
	e := b.element(axKeyOf(n.ID))
	f.on = false
	for range axPollFrames + 1 {
		b.publish(axRoots(n))
	}
	if len(f.freed) != 1 || f.freed[0] != e {
		t.Errorf("freed = %v, want the one element (%d) dropped on detach", f.freed, e)
	}
}

func TestAXOffNeverTouchesThePlatform(t *testing.T) {
	b := &axBridge{mode: AccessibilityOff}
	b.start(nil, AccessibilityOff)
	if b.plat != nil {
		t.Fatal("AccessibilityOff built a platform bridge")
	}
	b.publish(axRoots(axNode(RoleButton, "x", "x", Rct(Pt(0, 0), Sz(10, 10)))))
	if b.frame() != nil {
		t.Error("AccessibilityOff published a tree")
	}
}

func TestAXBoundsFlipTheYAxis(t *testing.T) {
	// ggui measures down from the top of the window and Cocoa measures up
	// from the bottom, and the bounds reported are the ones the node
	// painted at, not the ones it was clipped to.
	n := axNode(RoleButton, "b", nil, Rct(Pt(10, 20), Sz(30, 40)))
	n.Rect, n.Offscreen = Rect{}, true
	x, y, w, h := axBounds(n, 600)
	if x != 10 || y != 600-60 || w != 30 || h != 40 {
		t.Errorf("bounds = %g,%g %gx%g; want 10,540 30x40", x, y, w, h)
	}
}

func TestAXValuesFollowTheRole(t *testing.T) {
	for _, c := range []struct {
		name string
		node Node
		want float64
		num  bool
	}{
		{"unticked", Node{Role: RoleCheckbox, Checked: TriOff}, 0, true},
		{"ticked", Node{Role: RoleCheckbox, Checked: TriOn}, 1, true},
		{"mixed", Node{Role: RoleCheckbox, Checked: TriMixed}, 2, true},
		{"slider", Node{Role: RoleSlider, Now: 7}, 7, true},
		{"progress", Node{Role: RoleProgress, Now: 0.5}, 0.5, true},
		{"text", Node{Role: RoleTextField, Value: "hi"}, 0, false},
		{"plain", Node{Role: RoleButton}, 0, false},
	} {
		got, num := axNumber(c.node)
		if got != c.want || num != c.num {
			t.Errorf("%s: axNumber = %g, %v; want %g, %v", c.name, got, num, c.want, c.num)
		}
	}
	if _, _, ok := axRange(Node{Role: RoleButton, Min: 1, Max: 2}); ok {
		t.Error("a button reported a range")
	}
	if _, _, ok := axRange(Node{Role: RoleSlider}); ok {
		t.Error("a slider that gave no range reported one anyway")
	}
	if lo, hi, ok := axRange(Node{Role: RoleSlider, Min: 1, Max: 9}); !ok || lo != 1 || hi != 9 {
		t.Errorf("slider range = %g..%g, %v; want 1..9", lo, hi, ok)
	}
}

func TestAXRolesCoverEveryRole(t *testing.T) {
	all := []Role{
		RoleButton, RoleCheckbox, RoleRadio, RoleSwitch, RoleSlider, RoleTextField,
		RoleSelect, RoleOption, RoleMenu, RoleMenuItem, RoleTab, RoleTabs,
		RoleDisclosure, RoleDialog, RoleRow, RoleAccordion, RoleCombobox,
		RoleSeparator, RoleText, RoleHeading, RoleImage, RoleList, RoleListItem,
		RoleGroup, RoleProgress, RoleLink, RoleToolbar, RoleStatus, RoleWindow,
	}
	for _, r := range all {
		role, _ := axRole(r)
		if role == "AXUnknown" {
			t.Errorf("%s maps to no AppKit role", r)
		}
	}
	if role, sub := axRole(RoleSwitch); role != "AXCheckBox" || sub != "AXSwitch" {
		t.Errorf("switch = %s/%s, want AXCheckBox/AXSwitch", role, sub)
	}
	if role, sub := axRole(RoleTab); role != "AXRadioButton" || sub != "AXTabButton" {
		t.Errorf("tab = %s/%s, want AXRadioButton/AXTabButton", role, sub)
	}
	if role, sub := axRole(RoleDialog); role != "AXWindow" || sub != "AXDialog" {
		t.Errorf("dialog = %s/%s, want AXWindow/AXDialog", role, sub)
	}
	if role, _ := axRole(Role("nonsense")); role != "AXUnknown" {
		t.Errorf("an unknown role mapped to %s", role)
	}
}

func TestAXHitTestFindsTheDeepestNode(t *testing.T) {
	// The array is in paint order, so a scan would answer with the last
	// node painted; a hit test must answer with the innermost one.
	tree := axHitTree()
	if got := axHitTest(tree, Pt(30, 30)); got != 1 {
		t.Errorf("hit at 30,30 = %d, want the child (1)", got)
	}
	if got := axHitTest(tree, Pt(90, 90)); got != 0 {
		t.Errorf("hit at 90,90 = %d, want the group (0)", got)
	}
	if got := axHitTest(tree, Pt(500, 500)); got != -1 {
		t.Errorf("hit outside everything = %d, want -1", got)
	}
	if got := axHitTest(tree, Pt(30, 130)); got != -1 {
		t.Errorf("hit on the offscreen node = %d; it is not where it says it is", got)
	}
}

// axHitTree is a group with a visible child and a clipped one.
func axHitTree() *SemTree {
	t := &SemTree{focused: -1}
	t.nodes = []SemNode{
		{Node: Node{Role: RoleGroup}, Full: Rct(Pt(0, 0), Sz(100, 100)), Parent: -1, Children: []int{1, 2}},
		{Node: Node{Role: RoleButton}, Full: Rct(Pt(10, 10), Sz(50, 50)), Parent: 0},
		{Node: Node{Role: RoleButton, Offscreen: true}, Full: Rct(Pt(10, 110), Sz(50, 50)), Parent: 0},
	}
	t.roots = []int{0}
	return t
}
