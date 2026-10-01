package ui_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

type folder struct {
	Name string
	Kids []folder
}

func folders() ggui.Readable[[]folder] {
	return ggui.Const([]folder{
		{Name: "Fruits", Kids: []folder{{Name: "Apple"}, {Name: "Pear", Kids: []folder{{Name: "Conference"}}}}},
		{Name: "Vegetables", Kids: []folder{{Name: "Leek"}}},
		{Name: "Bread"},
	})
}

func folderTree() *ui.TreeWidget[folder, string] {
	return ui.Tree(folders(), func(f folder) string { return f.Name }, func(f folder) []folder { return f.Kids },
		func(f ggui.Readable[folder]) ggui.Widget {
			return ggui.TextOf(ggui.Map(f, func(f folder) string { return f.Name })).NoWrap()
		})
}

// treeItems lists the visible tree items' names in order.
func treeItems(p *ggui.Probe) []string {
	var out []string
	for _, n := range p.Semantics().Nodes(ggui.RoleTreeItem) {
		out = append(out, n.Name)
	}
	return out
}

func focusedItem(p *ggui.Probe) string {
	n, ok := p.Semantics().Focused()
	if !ok {
		return ""
	}
	return n.Name
}

// clickChevron clicks the disclosure chevron of the row named name.
func clickChevron(t *testing.T, p *ggui.Probe, name string) {
	t.Helper()
	n, ok := p.Semantics().Find(ggui.RoleTreeItem, name)
	if !ok {
		t.Fatalf("no tree item %q", name)
	}
	depth := float64(n.Level - 1)
	p.Click(ggui.Pt(n.Rect.Origin.X+8+depth*16+8, n.Rect.Origin.Y+n.Rect.Size.H/2))
}

func TestTreeChevronClickExpandsAndCollapses(t *testing.T) {
	t.Parallel()
	var picked []string
	p := ggui.NewProbe(folderTree().OnSelect(func(f folder) { picked = append(picked, f.Name) }), ggui.Sz(300, 400))
	defer p.Close()
	if got := treeItems(p); !slices.Equal(got, []string{"Fruits", "Vegetables", "Bread"}) {
		t.Fatalf("collapsed tree shows %q", got)
	}
	clickChevron(t, p, "Fruits")
	if got := treeItems(p); !slices.Equal(got, []string{"Fruits", "Apple", "Pear", "Vegetables", "Bread"}) {
		t.Fatalf("after opening Fruits: %q", got)
	}
	if len(picked) != 0 {
		t.Fatalf("the chevron activated the row: %q", picked)
	}
	clickChevron(t, p, "Fruits")
	if got := treeItems(p); !slices.Equal(got, []string{"Fruits", "Vegetables", "Bread"}) {
		t.Fatalf("after closing Fruits: %q", got)
	}
	p.Tap("Bread")
	if !slices.Equal(picked, []string{"Bread"}) {
		t.Fatalf("tapping a row picked %q, want Bread", picked)
	}
}

func TestTreeKeysNavigateExpandAndSelect(t *testing.T) {
	t.Parallel()
	chosen := ggui.State("")
	var picked []string
	tree := folderTree().BindSelected(chosen).OnSelect(func(f folder) { picked = append(picked, f.Name) })
	p := ggui.NewProbe(tree, ggui.Sz(300, 400))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if got := focusedItem(p); got != "Fruits" {
		t.Fatalf("Tab focused %q, want the first row", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowRight) // open Fruits
	p.Type(ggui.Mods{}, ggui.KeyArrowRight) // to Apple
	if got, sel := focusedItem(p), ggui.Untrack(chosen.Get); got != "Apple" || sel != "Apple" {
		t.Fatalf("Right, Right: focus %q, selected %q; want Apple for both", got, sel)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyArrowRight, ggui.KeyArrowRight)
	if got := focusedItem(p); got != "Conference" {
		t.Fatalf("Down, Right, Right: focus %q, want Conference", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft)
	if got := focusedItem(p); got != "Pear" {
		t.Fatalf("Left from a leaf: focus %q, want its parent Pear", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft)
	if got := treeItems(p); slices.Contains(got, "Conference") {
		t.Fatalf("Left on open Pear left it open: %q", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft, ggui.KeyArrowLeft)
	if got := treeItems(p); !slices.Equal(got, []string{"Fruits", "Vegetables", "Bread"}) || focusedItem(p) != "Fruits" {
		t.Fatalf("Left, Left from Pear: rows %q focus %q; want Fruits focused and closed", got, focusedItem(p))
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	if got, sel := focusedItem(p), ggui.Untrack(chosen.Get); got != "Bread" || sel != "Bread" {
		t.Fatalf("End: focus %q selected %q", got, sel)
	}
	p.Type(ggui.Mods{}, ggui.KeyHome, ggui.KeyArrowUp)
	if got := focusedItem(p); got != "Fruits" {
		t.Fatalf("Home, Up: focus %q, want Fruits", got)
	}
	if len(picked) != 0 {
		t.Fatalf("arrow keys fired OnSelect: %q", picked)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if !slices.Equal(picked, []string{"Fruits"}) {
		t.Fatalf("Enter picked %q, want Fruits", picked)
	}
}

func TestTreeIsOneTabStop(t *testing.T) {
	t.Parallel()
	open := ggui.State(map[string]bool{"Fruits": true})
	p := ggui.NewProbe(ggui.Column(
		folderTree().BindExpanded(open).Selected("Pear"),
		ui.Button("After", func() {}),
	), ggui.Sz(300, 400))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if got := focusedItem(p); got != "Pear" {
		t.Fatalf("Tab focused %q, want the selected row", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if got := focusedItem(p); got != "After" {
		t.Fatalf("second Tab focused %q, want the button after the tree", got)
	}
	p.Type(ggui.Mods{Shift: true}, ggui.KeyTab)
	if got := focusedItem(p); got != "Pear" {
		t.Fatalf("Shift+Tab focused %q, want back to Pear", got)
	}
}

func TestTreeSemantics(t *testing.T) {
	t.Parallel()
	open := ggui.State(map[string]bool{"Fruits": true})
	p := ggui.NewProbe(folderTree().BindExpanded(open).Name("Groceries").Selected("Apple"), ggui.Sz(300, 400))
	defer p.Close()
	sem := p.Semantics()
	root, ok := sem.Find(ggui.RoleTree, "Groceries")
	if !ok {
		t.Fatal("no tree node")
	}
	type item struct {
		name     string
		level    int
		expanded string
		selected bool
	}
	state := func(b *bool) string {
		switch {
		case b == nil:
			return "leaf"
		case *b:
			return "open"
		}
		return "closed"
	}
	var got []item
	for _, n := range sem.Nodes(ggui.RoleTreeItem) {
		if sem.At(n.Parent).ID != root.ID {
			t.Fatalf("tree item %q is not a child of the tree:\n%s", n.Name, sem)
		}
		got = append(got, item{n.Name, n.Level, state(n.Expanded), n.Selected})
	}
	want := []item{
		{"Fruits", 1, "open", false},
		{"Apple", 2, "leaf", true},
		{"Pear", 2, "closed", false},
		{"Vegetables", 1, "closed", false},
		{"Bread", 1, "leaf", false},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tree items\n got %v\nwant %v", got, want)
	}
	if _, ok := sem.Find(ggui.RoleTreeItem, "Leek"); ok {
		t.Fatal("a collapsed subtree's row is present")
	}
	veg, _ := sem.Find(ggui.RoleTreeItem, "Vegetables")
	p.Perform(veg.ID, ggui.Action{Kind: ggui.ActionExpand})
	if _, ok := p.Semantics().Find(ggui.RoleTreeItem, "Leek"); !ok || !ggui.Untrack(open.Get)["Vegetables"] {
		t.Fatal("ActionExpand did not open Vegetables through the binding")
	}
	fruits, _ := p.Semantics().Find(ggui.RoleTreeItem, "Fruits")
	p.Perform(fruits.ID, ggui.Action{Kind: ggui.ActionCollapse})
	if got := treeItems(p); !slices.Equal(got, []string{"Fruits", "Vegetables", "Leek", "Bread"}) {
		t.Fatalf("after collapsing Fruits: %q", got)
	}
	bread, _ := p.Semantics().Find(ggui.RoleTreeItem, "Bread")
	p.Perform(bread.ID, ggui.Action{Kind: ggui.ActionSelect})
	if bread, _ := p.Semantics().Find(ggui.RoleTreeItem, "Bread"); !bread.Selected {
		t.Fatal("ActionSelect did not select Bread")
	}
}

func TestTreeKeyboardReachesRowsScrolledOutOfView(t *testing.T) {
	t.Parallel()
	var many []folder
	for i := range 200 {
		many = append(many, folder{Name: "Item " + strconv.Itoa(i)})
	}
	tree := ui.Tree(ggui.Const(many), func(f folder) string { return f.Name }, func(f folder) []folder { return f.Kids },
		func(f ggui.Readable[folder]) ggui.Widget {
			return ggui.TextOf(ggui.Map(f, func(f folder) string { return f.Name }))
		}).Height(160)
	p := ggui.NewProbe(tree, ggui.Sz(300, 400))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyEnd)
	if got := focusedItem(p); got != "Item 199" {
		t.Fatalf("End focused %q, want the last row", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyHome)
	if got := focusedItem(p); got != "Item 0" {
		t.Fatalf("Home focused %q, want the first row", got)
	}
}
