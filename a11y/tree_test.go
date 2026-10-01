package a11y

import (
	"slices"
	"testing"
)

// sampleTree builds a window holding a toolbar with two buttons and a
// focused text field, followed by a second root, in paint order.
func sampleTree() *SemTree {
	return Build([]SemNode{
		{Node: Node{Role: RoleWindow, Name: "Main"}, Parent: -1},                      // 0
		{Node: Node{Role: RoleToolbar}, Parent: 0},                                    // 1
		{Node: Node{Role: RoleButton, Name: "Save"}, Parent: 1},                       // 2
		{Node: Node{Role: RoleButton, Name: "Open", Disabled: true}, Parent: 1},       // 3
		{Node: Node{Role: RoleTextField, Name: "Title", Value: "draft"}, Parent: 0},   // 4
		{Node: Node{Role: RoleDialog, Name: "About"}, Parent: -1},                     // 5
		{Node: Node{Role: RoleButton, Name: "OK", Actions: ActionPress}, Parent: 5},   // 6
		{Node: Node{Role: RoleCheckbox, Name: "Agree", Checked: TriMixed}, Parent: 5}, // 7
	}, 4)
}

func TestBuildLinksRootsAndChildrenInPaintOrder(t *testing.T) {
	t.Parallel()
	tr := sampleTree()
	if tr.Len() != 8 {
		t.Fatalf("Len = %d, want 8", tr.Len())
	}
	if got := tr.Roots(); !slices.Equal(got, []int{0, 5}) {
		t.Errorf("Roots = %v, want [0 5]", got)
	}
	want := map[int][]int{0: {1, 4}, 1: {2, 3}, 5: {6, 7}}
	for i := range tr.Len() {
		if got := tr.At(i).Children; !slices.Equal(got, want[i]) {
			t.Errorf("node %d has children %v, want %v", i, got, want[i])
		}
		for _, c := range tr.At(i).Children {
			if tr.At(c).Parent != i {
				t.Errorf("child %d of %d names parent %d", c, i, tr.At(c).Parent)
			}
		}
	}
	// The children slices share one backing array, so appending to one must
	// not have spilled into its neighbour's.
	if cap(tr.At(1).Children) != 2 {
		t.Errorf("toolbar's children have capacity %d, want exactly its 2 children", cap(tr.At(1).Children))
	}
	if ref := tr.Ref(2); ref != tr.Ref(2) || ref.Name != "Save" {
		t.Error("Ref should point into the tree's own storage")
	}
}

func TestBuildHandlesEmptyAndFlatTrees(t *testing.T) {
	t.Parallel()
	empty := Build(nil, -1)
	if empty.Len() != 0 || len(empty.Roots()) != 0 || empty.String() != "" {
		t.Errorf("empty tree: Len=%d Roots=%v String=%q", empty.Len(), empty.Roots(), empty.String())
	}
	if _, ok := empty.Focused(); ok || empty.FocusIndex() != -1 {
		t.Error("an empty tree reports focus")
	}
	if _, ok := empty.Find("", ""); ok {
		t.Error("Find matched in an empty tree")
	}
	flat := Build([]SemNode{{Parent: -1}, {Parent: -1}, {Parent: -1}}, -1)
	if got := flat.Roots(); !slices.Equal(got, []int{0, 1, 2}) {
		t.Errorf("flat roots = %v, want [0 1 2]", got)
	}
	for i := range flat.Len() {
		if len(flat.At(i).Children) != 0 {
			t.Errorf("flat node %d has children %v", i, flat.At(i).Children)
		}
	}
}

func TestFocusedMirrorsTheFocusIndex(t *testing.T) {
	t.Parallel()
	tr := sampleTree()
	n, ok := tr.Focused()
	if !ok || n.Name != "Title" || tr.FocusIndex() != 4 {
		t.Errorf("Focused = %q, %v at %d; want the Title field at 4", n.Name, ok, tr.FocusIndex())
	}
}

func TestFindAndNodesFilterByRoleAndName(t *testing.T) {
	t.Parallel()
	tr := sampleTree()
	cases := []struct {
		role Role
		name string
		want string
		ok   bool
	}{
		{RoleButton, "", "Save", true},
		{RoleButton, "OK", "OK", true},
		{"", "Agree", "Agree", true},
		{"", "", "Main", true},
		{RoleButton, "Agree", "", false},
		{RoleSlider, "", "", false},
	}
	for _, c := range cases {
		n, ok := tr.Find(c.role, c.name)
		if ok != c.ok || n.Name != c.want {
			t.Errorf("Find(%q, %q) = %q, %v; want %q, %v", c.role, c.name, n.Name, ok, c.want, c.ok)
		}
	}
	var buttons []int
	for i, n := range tr.Nodes(RoleButton) {
		if n.Role != RoleButton {
			t.Errorf("Nodes(button) yielded a %s", n.Role)
		}
		buttons = append(buttons, i)
	}
	if !slices.Equal(buttons, []int{2, 3, 6}) {
		t.Errorf("buttons at %v, want [2 3 6]", buttons)
	}
	var all []int
	for i := range tr.Nodes("") {
		all = append(all, i)
	}
	var viaAll []int
	for i, n := range tr.All() {
		if n.Name != tr.At(i).Name {
			t.Errorf("All yielded node %d as %q, want %q", i, n.Name, tr.At(i).Name)
		}
		viaAll = append(viaAll, i)
	}
	if !slices.Equal(all, viaAll) || len(all) != tr.Len() {
		t.Errorf("Nodes(\"\") = %v and All = %v; want every index once", all, viaAll)
	}
}

func TestIteratorsStopWhenTheLoopBreaks(t *testing.T) {
	t.Parallel()
	tr := sampleTree()
	count := func(seq func(func(int, SemNode) bool)) int {
		n := 0
		for range seq {
			n++
			break
		}
		return n
	}
	if count(tr.All()) != 1 || count(tr.Nodes(RoleButton)) != 1 || count(tr.Ancestors(3)) != 1 {
		t.Error("a node iterator kept yielding after the loop broke")
	}
	n := 0
	for i := range tr.Walk(0) {
		n++
		if i == 2 {
			break
		}
	}
	if n != 3 {
		t.Errorf("Walk yielded %d nodes before breaking at the first button, want 3", n)
	}
}

func TestAncestorsAndWalkFollowTheHierarchy(t *testing.T) {
	t.Parallel()
	tr := sampleTree()
	var chain []int
	for i := range tr.Ancestors(3) {
		chain = append(chain, i)
	}
	if !slices.Equal(chain, []int{3, 1, 0}) {
		t.Errorf("Ancestors(3) = %v, want [3 1 0]", chain)
	}
	type step struct{ i, depth int }
	var walked []step
	for i, d := range tr.Walk(0) {
		walked = append(walked, step{i, d})
	}
	want := []step{{0, 0}, {1, 1}, {2, 2}, {3, 2}, {4, 1}}
	if !slices.Equal(walked, want) {
		t.Errorf("Walk(0) = %v, want %v", walked, want)
	}
	walked = walked[:0]
	for i, d := range tr.Walk(1) {
		walked = append(walked, step{i, d})
	}
	if !slices.Equal(walked, []step{{1, 0}, {2, 1}, {3, 1}}) {
		t.Errorf("Walk(1) = %v, want depths measured from the toolbar", walked)
	}
}

func TestStringRendersIndentedNodesWithFlags(t *testing.T) {
	t.Parallel()
	want := `window "Main"
  toolbar
    button "Save"
    button "Open" disabled
  textfield "Title" value="draft"
dialog "About"
  button "OK"
  checkbox "Agree" mixed
`
	if got := sampleTree().String(); got != want {
		t.Errorf("String =\n%s\nwant\n%s", got, want)
	}
}

func TestFlagsReportOnlyMeaningfulState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		n    Node
		want string
	}{
		{Node{Role: RoleText}, ""},
		{Node{Checked: TriOff}, " unchecked"},
		{Node{Checked: TriOn}, " checked"},
		{Node{Expanded: Expandable(true)}, " expanded"},
		{Node{Expanded: Expandable(false)}, " collapsed"},
		{Node{Selected: true, Offscreen: true}, " selected offscreen"},
		{Node{Min: 0, Max: 10, Now: 2.5}, " 2.5 of 0..10"},
		{Node{Min: 5}, ""}, // a range with neither end nor position set is not shown
		{Node{Value: `say "hi"`, Checked: TriOn, Disabled: true}, ` value="say \"hi\"" checked disabled`},
	}
	for _, c := range cases {
		n := SemNode{Node: c.n}
		if got := n.Flags(); got != c.want {
			t.Errorf("Flags(%+v) = %q, want %q", c.n, got, c.want)
		}
	}
}
