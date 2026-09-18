package ggui

import (
	"strings"
	"testing"
)

// described paints w at size through a Probe and returns the published tree.
func described(t *testing.T, w Widget, size Size) *SemTree {
	t.Helper()
	p := NewProbe(w, size)
	t.Cleanup(p.Close)
	return p.Semantics()
}

func TestSemanticsNestingComesFromTheBuilder(t *testing.T) {
	// Column registers nothing, so paint depth would fold the leaf under
	// the group beside it; the explicit scope is what says otherwise.
	inner := FromFuncs(
		func(c Constraints, _ Env) Size { return c.Constrain(Sz(50, 20)) },
		func(dst *Canvas, r Rect) {
			dst.Leaf(r, Node{Role: RoleButton, Name: "one"})
			dst.Node(r, Node{Role: RoleGroup, Name: "box"}, func(dst *Canvas) {
				dst.Leaf(r, Node{Role: RoleButton, Name: "two"})
			})
			dst.Leaf(r, Node{Role: RoleButton, Name: "three"})
		})
	got := described(t, Column(inner), Sz(100, 100)).String()
	want := `button "one"
group "box"
  button "two"
button "three"
`
	if got != want {
		t.Fatalf("tree =\n%s\nwant\n%s", got, want)
	}
}

func TestSemanticsKeepsClippedNodesWithTheirBounds(t *testing.T) {
	// A node scrolled out of view is present, flagged, and still says where
	// it is, so an assistive technology can ask to be taken there.
	tall := FromFuncs(
		func(c Constraints, _ Env) Size { return Sz(c.MaxW, 400) },
		func(dst *Canvas, r Rect) {
			dst.Leaf(Rct(r.Origin, Sz(r.Size.W, 20)), Node{Role: RoleButton, Name: "top"})
			dst.Leaf(Rct(Pt(r.Origin.X, r.Origin.Y+300), Sz(r.Size.W, 20)), Node{Role: RoleButton, Name: "far"})
		})
	tree := described(t, Scroll(tall), Sz(100, 50))
	far, ok := tree.Find(RoleButton, "far")
	if !ok {
		t.Fatal("the clipped node was dropped instead of flagged")
	}
	if !far.Offscreen {
		t.Error("far.Offscreen = false, want true")
	}
	if far.Full.Origin.Y != 300 || far.Full.Size.H != 20 {
		t.Errorf("far.Full = %v, want the bounds it painted at", far.Full)
	}
	if top, _ := tree.Find(RoleButton, "top"); top.Offscreen {
		t.Error("the visible node was flagged offscreen")
	}
}

// twice describes itself from two places, as ui.TextField does around the
// editor inside it.
type twice struct{ Interactive }

func (t *twice) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(40, 20)) }
func (t *twice) Paint(dst *Canvas, r Rect) {
	t.Hit(dst, r, t, CursorShapeText)
	dst.Describe(Rct(r.Origin, Sz(20, 10)), t)
}
func (t *twice) HandlePointer(PointerEvent) bool { return true }
func (t *twice) HandleKey(KeyEvent)              {}

func TestSemanticsDescribesOneHandlerOnce(t *testing.T) {
	w := &twice{}
	w.Role, w.Name = RoleTextField, "Name"
	tree := described(t, w, Sz(100, 100))
	n := 0
	for range tree.Nodes(RoleTextField) {
		n++
	}
	if n != 1 {
		t.Fatalf("%d textfield nodes, want 1:\n%s", n, tree)
	}
	if got := tree.At(0).Rect.Size; got == (Size{W: 20, H: 10}) {
		t.Error("kept the inner node; the first description of a handler wins")
	}
}

func TestSemanticsSnapshotIsFrozen(t *testing.T) {
	label := State("a")
	p := ProbeBuilder(func() Widget { return TextOf(label) }, Sz(100, 100))
	defer p.Close()
	first := p.Semantics()
	before := first.String()
	label.Set("b")
	second := p.Semantics()
	if first == second {
		t.Fatal("the same tree was published twice")
	}
	if first.String() != before {
		t.Errorf("the published tree changed under its reader:\n%s", first)
	}
	if !strings.Contains(second.String(), `"b"`) {
		t.Errorf("the new tree missed the new text:\n%s", second)
	}
}

func TestSemanticsMirrorsFocus(t *testing.T) {
	one, two := Focus(Box().Size(40, 20)), Focus(Box().Size(40, 20))
	p := NewProbe(Column(one, two), Sz(100, 100))
	defer p.Close()
	p.Click(Pt(20, 10))
	if _, ok := p.Semantics().Focused(); ok {
		t.Error("a bare Focus describes nothing, so nothing can be focused in the tree")
	}
	// A described control is found by its handler, whatever it moved to.
	w := &twice{}
	w.Role, w.Name = RoleTextField, "Name"
	q := NewProbe(w, Sz(100, 100))
	defer q.Close()
	q.Click(Pt(10, 5))
	n, ok := q.Semantics().Focused()
	if !ok || n.Name != "Name" {
		t.Fatalf("focused node = %v, %v; want the text field", n, ok)
	}
}

type uncomparableFocus []int

func (uncomparableFocus) HandleKey(KeyEvent) {}

func TestFocusedNodePrefersHandlerAndFallsBackAfterRebuild(t *testing.T) {
	c := Canvas{}
	r := Rct(Pt(0, 0), Sz(100, 20))
	first, exact, rebuilt := &twice{}, &twice{}, &twice{}
	for _, w := range []*twice{first, exact, rebuilt} {
		w.Role = RoleTextField
		w.Key("shared")
	}
	c.Describe(r, first)
	c.Describe(r, exact)
	for _, test := range []struct {
		name string
		hit  hitRegion
		want int
	}{
		{"exact before identity", hitRegion{key: exact, id: "shared", rect: r}, 1},
		{"rebuilt identity", hitRegion{key: rebuilt, id: "shared", rect: Rct(Pt(0, 50), r.Size)}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := focusedNode(&c, &test.hit); got != test.want {
				t.Fatalf("focused node = %d, want %d", got, test.want)
			}
		})
	}
	// No handler index entry exists for a noncomparable handler; the
	// unkeyed geometry fallback must still work without a map-key panic.
	c.resetSemantics()
	h := uncomparableFocus{1}
	c.addSem(r, Node{Role: RoleTextField}, h)
	if got := focusedNode(&c, &hitRegion{key: h, rect: r}); got != 0 {
		t.Fatalf("uncomparable handler fallback = %d, want 0", got)
	}
	if got := focusedNode(&c, &hitRegion{key: &twice{}, rect: r}); got != 0 {
		t.Fatalf("rebuilt unkeyed handler fallback = %d, want 0", got)
	}
}

func TestSemanticsListCountsItems(t *testing.T) {
	items := State([]string{"a", "b", "c"})
	tree := described(t, Each(items, func(s Reader[string]) Widget { return TextOf(s) }), Sz(100, 200))
	list, ok := tree.Find(RoleList, "")
	if !ok {
		t.Fatalf("no list:\n%s", tree)
	}
	if list.Max != 3 {
		t.Errorf("list.Max = %g, want 3", list.Max)
	}
	if len(list.Children) != 3 {
		t.Fatalf("%d items, want 3:\n%s", len(list.Children), tree)
	}
	third := tree.At(list.Children[2])
	if third.Role != RoleListItem || third.Now != 3 || third.Max != 3 {
		t.Errorf("third item = %v, want item 3 of 3", third.Node)
	}
	if kids := third.Children; len(kids) != 1 || tree.At(kids[0]).Name != "c" {
		t.Errorf("the item does not hold its row:\n%s", tree)
	}
}

func TestSemanticsTextRoles(t *testing.T) {
	tree := described(t, Column(Title("Heading"), Text("body"), Text("")), Sz(200, 200))
	want := `heading "Heading"
text "body"
`
	if got := tree.String(); got != want {
		t.Fatalf("tree =\n%s\nwant\n%s", got, want)
	}
}

func TestSemanticsStagingReusePreservesOldTree(t *testing.T) {
	rows := State([]string{"one", "two", "three"})
	p := ProbeBuilder(func() Widget {
		return Each(rows, func(s Reader[string]) Widget { return TextOf(s) })
	}, Sz(100, 200))
	defer p.Close()
	old := p.Semantics()
	before := old.String()
	rows.Set([]string{"new"})
	current := p.Semantics()
	if old.String() != before || old.At(0).Max != 3 {
		t.Fatal("reusing staging nodes changed an already published tree")
	}
	if current.At(0).Max != 1 || !strings.Contains(current.String(), `"new"`) {
		t.Fatalf("new snapshot has stale nodes: %s", current)
	}
	// Shrinking must release references in unused staging slots as well.
	for _, n := range p.canvas.sem[len(p.canvas.sem):cap(p.canvas.sem)] {
		if n.handler != nil || n.node.Name != "" || n.node.Runs != nil {
			t.Fatal("unused staging slot retains a previous frame's data")
		}
	}
}
