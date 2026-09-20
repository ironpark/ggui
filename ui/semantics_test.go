package ui_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// semantics paints w at size and returns the tree the frame published.
func semantics(t *testing.T, w ggui.Widget, size ggui.Size) *ggui.SemTree {
	t.Helper()
	p := ggui.NewProbe(w, size)
	t.Cleanup(p.Close)
	return p.Semantics()
}

// wantTree fails unless the tree's shape is exactly want.
func wantTree(t *testing.T, tree *ggui.SemTree, want string) {
	t.Helper()
	if got := tree.String(); got != strings.TrimLeft(want, "\n") {
		t.Fatalf("tree =\n%s\nwant\n%s", got, strings.TrimLeft(want, "\n"))
	}
}

// node returns the one node with role and name, or fails.
func node(t *testing.T, tree *ggui.SemTree, role ggui.Role, name string) ggui.SemNode {
	t.Helper()
	n, ok := tree.Find(role, name)
	if !ok {
		t.Fatalf("no %s named %q in\n%s", role, name, tree)
	}
	return n
}

// TestSemanticsDisabledButtonIsPresent covers the case the hit regions
// cannot: Interactive.Hit registers nothing for an Inert control, so a
// disabled button used to vanish from everything that could see it.
func TestSemanticsDisabledButtonIsPresent(t *testing.T) {
	tree := semantics(t, ui.Button("Save", nil).Disabled(true), ggui.Sz(200, 60))
	n := node(t, tree, ggui.RoleButton, "Save")
	if !n.Disabled {
		t.Error("the button is present but not marked disabled")
	}
	wantTree(t, tree, `
button "Save" disabled
`)
}

// TestSemanticsTextFieldIsOneNode covers the other verified bug: the box
// and the editor inside it both describe the same field, and one field is
// one element.
func TestSemanticsTextFieldIsOneNode(t *testing.T) {
	value := ggui.State("Ada")
	tree := semantics(t, ui.TextField(value).Named("Name"), ggui.Sz(200, 60))
	n := 0
	for range tree.Nodes(ggui.RoleTextField) {
		n++
	}
	if n != 1 {
		t.Fatalf("%d text field nodes, want 1:\n%s", n, tree)
	}
	if n := node(t, tree, ggui.RoleTextField, "Name"); n.Value != "Ada" {
		t.Errorf("value = %q, want %q", n.Value, "Ada")
	}
}

func TestSemanticsFieldNamesItsInput(t *testing.T) {
	tree := semantics(t, ui.Field("Email", ui.TextField(ggui.State(""))), ggui.Sz(200, 100))
	wantTree(t, tree, `
text "Email"
textfield "Email"
`)
}

func TestSemanticsTabsHoldTheirTabs(t *testing.T) {
	sel := ggui.State(1)
	tabs := ui.Tabs(sel, ui.Tab("One", ggui.Text("first")), ui.Tab("Two", ggui.Text("second")))
	wantTree(t, semantics(t, tabs, ggui.Sz(300, 200)), `
tabs
  tab "One"
  tab "Two" selected
  text "second"
`)
}

func TestSemanticsDialogPanelIsTheNode(t *testing.T) {
	// The scrim takes the clicks but is not an element; the panel is.
	tree := semantics(t, ui.Dialog(ggui.State(true), ggui.Text("body")).Title("Confirm"), ggui.Sz(300, 300))
	wantTree(t, tree, `
dialog "Confirm"
  heading "Confirm"
  text "body"
`)
}

func TestSemanticsSelectOwnsItsOptions(t *testing.T) {
	value := ggui.State(2)
	sel := ui.Select(value, []int{1, 2, 3}).Named("Count")
	p := ggui.NewProbe(ggui.Column(sel), ggui.Sz(200, 200))
	defer p.Close()
	wantTree(t, p.Semantics(), `
select "Count" value="2" collapsed
`)
	sel.Popup().Show()
	// The list paints through an overlay, long after the tree did, and
	// still belongs to the combobox that opened it.
	wantTree(t, p.Semantics(), `
select "Count" value="2" expanded
  group
    option "1"
    option "2" selected
    option "3"
`)
}

func TestSemanticsMenuOwnsItsItems(t *testing.T) {
	m := ui.Menu("File", ui.MenuItem("Open", nil), ui.MenuItem("Quit", nil).Disabled(true))
	p := ggui.NewProbe(m, ggui.Sz(200, 200))
	defer p.Close()
	p.Frame()
	m.Popup().Show()
	wantTree(t, p.Semantics(), `
menu "File" expanded
  menuitem "Open"
  menuitem "Quit" disabled
`)
}

func TestSemanticsComboboxOwnsItsSearch(t *testing.T) {
	c := ui.Combobox(ggui.State(1), []int{1, 2})
	p := ggui.NewProbe(c, ggui.Sz(400, 400))
	defer p.Close()
	p.Frame()
	c.Popup().Show()
	tree := p.Semantics()
	trigger := node(t, tree, ggui.RoleCombobox, "Choose option")
	if trigger.Expanded == nil || !*trigger.Expanded {
		t.Fatalf("the trigger does not report the open list:\n%s", tree)
	}
	if len(trigger.Children) == 0 {
		t.Fatalf("the search panel is not inside the trigger:\n%s", tree)
	}
	if _, ok := tree.Find(ggui.RoleTextField, "Search options"); !ok {
		t.Errorf("no search field:\n%s", tree)
	}
}

func TestSemanticsDisclosuresReportOpen(t *testing.T) {
	open := ggui.State([]string{"a"})
	acc := ui.Accordion(open,
		ui.AccordionItem("a", "A", ggui.Text("ac")),
		ui.AccordionItem("b", "B", ggui.Text("bc")).Disabled(true),
	)
	wantTree(t, semantics(t, acc, ggui.Sz(200, 200)), `
accordion "Accordion"
  disclosure "A" expanded
  text "ac"
  disclosure "B" collapsed disabled
`)

	tree := semantics(t, ui.Collapsible(ggui.State(true), "More", ggui.Text("body")), ggui.Sz(200, 200))
	n := node(t, tree, ggui.RoleDisclosure, "More")
	if n.Expanded == nil || !*n.Expanded {
		t.Errorf("the collapsible does not report itself open:\n%s", tree)
	}
	if !n.Actions.Has(ggui.ActionCollapse) {
		t.Error("an open disclosure should offer to collapse")
	}
}

func TestSemanticsSliderReportsItsRange(t *testing.T) {
	tree := semantics(t, ui.Slider(ggui.State(4.0), 0, 10).Named("Volume"), ggui.Sz(200, 40))
	n := node(t, tree, ggui.RoleSlider, "Volume")
	if n.Min != 0 || n.Max != 10 || n.Now != 4 {
		t.Errorf("range = %g..%g at %g, want 0..10 at 4", n.Min, n.Max, n.Now)
	}
	if !n.Actions.Has(ggui.ActionIncrement | ggui.ActionDecrement) {
		t.Error("a slider should offer increment and decrement")
	}
}

func TestSemanticsTogglesReportChecked(t *testing.T) {
	on, off := ggui.State(true), ggui.State(false)
	chosen := ggui.State("b")
	w := ggui.Column(
		ui.Checkbox(on, "Ready"),
		ui.Switch(off, "Dark"),
		ui.Radio(chosen, "a", "A"),
		ui.Radio(chosen, "b", "B"),
	)
	wantTree(t, semantics(t, w, ggui.Sz(200, 200)), `
checkbox "Ready" checked
switch "Dark" unchecked
radio "A" unchecked
radio "B" checked selected
`)
}

func TestSemanticsRadiosAreOneGroup(t *testing.T) {
	tree := semantics(t, ui.Radios(ggui.State(1), []int{1, 2}), ggui.Sz(200, 100))
	wantTree(t, tree, `
group 0 of 1..2
  radio "1" checked selected
  radio "2" unchecked
`)
}

func TestSemanticsTableRowsHoldTheirCells(t *testing.T) {
	rows := ggui.State([]string{"Ada", "Alan"})
	table := ui.Table(rows, func(s string) string { return s },
		ui.TextCol("Name", func(s string) string { return s })).Selected(ggui.State("Alan"))
	wantTree(t, semantics(t, table, ggui.Sz(300, 200)), `
text "Name"
list 0 of 1..2
  listitem 1 of 1..2
    row "Ada"
      text "Ada"
  listitem 2 of 1..2
    row "Alan" selected
      text "Alan"
`)
}

func TestSemanticsCardGroupsAndProgressReports(t *testing.T) {
	wantTree(t, semantics(t, ui.Card(ggui.Text("inside")), ggui.Sz(200, 100)), `
group
  text "inside"
`)
	tree := semantics(t, ui.Progress(ggui.State(0.5)), ggui.Sz(200, 40))
	if n := node(t, tree, ggui.RoleProgress, ""); n.Max != 1 {
		t.Errorf("progress = %g of %g, want a 0..1 range", n.Now, n.Max)
	}
}

func TestSemanticsToastIsALiveRegion(t *testing.T) {
	toaster := ui.NewToaster()
	p := ggui.NewProbe(toaster, ggui.Sz(400, 300))
	defer p.Close()
	p.Frame()
	toaster.Push(ui.Toast("Saved", "the file is on disk"))
	tree := p.Semantics()
	n := node(t, tree, ggui.RoleStatus, "Saved")
	if len(n.Children) == 0 {
		t.Errorf("the notice holds nothing:\n%s", tree)
	}
}

func TestSemanticsScrolledRowsStayWithTheirBounds(t *testing.T) {
	rows := make([]string, 200)
	for i := range rows {
		rows[i] = "row " + strconv.Itoa(i)
	}
	list := ggui.Each(ggui.State(rows), func(rowItem ggui.EachItem[string]) ggui.Widget {
		s := rowItem.Value
		return ui.Button(s.Get(), nil)
	}).ItemExtent(20)
	tree := semantics(t, ggui.Scroll(list), ggui.Sz(200, 60))
	n, ok := tree.Find(ggui.RoleList, "")
	if !ok || n.Max != 200 {
		t.Fatalf("list = %v, want 200 items:\n%s", n.Node, tree)
	}
	// Only the rows in view are built at all, and each says where it sits.
	last := tree.At(n.Children[len(n.Children)-1])
	if last.Now < 1 || last.Now > 200 {
		t.Errorf("last item = %g of %g", last.Now, last.Max)
	}
}
