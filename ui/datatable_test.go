package ui_test

import (
	"cmp"
	"reflect"
	"strconv"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func dataModel(rows ggui.Readable[[]person]) *ui.TableModel[person, int] {
	return ui.NewTableModel(rows, func(p person) int { return p.ID },
		ui.TextCol("Name", func(p person) string { return p.Name }).Sortable(func(a, b person) int { return cmp.Compare(a.Name, b.Name) }),
		ui.TextCol("Age", func(p person) string { return strconv.Itoa(p.Age) }).Sortable(func(a, b person) int { return cmp.Compare(a.Age, b.Age) }),
	)
}
func ids(rows []person) []int {
	result := []int{}
	for _, r := range rows {
		result = append(result, r.ID)
	}
	return result
}
func TestDataTablePipeline(t *testing.T) {
	rows := people()
	m := dataModel(rows)
	m.SetPageSize(2)
	check := func(want ...int) {
		t.Helper()
		if got := ids(m.Rows().Get()); !reflect.DeepEqual(got, want) {
			t.Fatalf("rows %v want %v", got, want)
		}
	}
	check(1, 2)
	m.SetPage(2)
	check(3)
	m.ToggleSort("Age")
	check(3, 1)
	m.ToggleSort("Age")
	check(2, 1)
	m.ToggleSort("Age")
	check(1, 2)
	m.SetPage(2)
	m.SetQuery("  GRACE ")
	check(2)
	if m.Page() != 1 {
		t.Fatal("filter did not reset page")
	}
	m.SelectPage(true)
	m.SetQuery("")
	m.SelectPage(true)
	m.SetPage(2)
	m.SelectPage(true)
	if len(m.SelectedRows()) != 3 {
		t.Fatal("selection lost across pages")
	}
	m.SelectPage(false)
	if len(m.SelectedRows()) != 2 {
		t.Fatal("page clear affected other pages")
	}
	rows.Set([]person{{2, "Updated", 46}})
	check(2)
	if m.Page() != 1 || len(m.SelectedRows()) != 1 {
		t.Fatal("source shrink not reflected")
	}
	m.SetQuery("missing")
	if len(m.Rows().Get()) != 0 || m.PageCount() != 1 {
		t.Fatal("empty state")
	}
	m.SetPageSize(0)
	if m.PageSize() != 1 {
		t.Fatal("invalid page size")
	}
}
func TestDataTableStableSortAndMemoization(t *testing.T) {
	rows := ggui.State([]person{{1, "B", 2}, {2, "A", 2}, {3, "C", 1}})
	calls := 0
	m := ui.NewTableModel(rows, func(p person) int { return p.ID }, ui.TextCol("Name", func(p person) string { return p.Name }).Sortable(func(a, b person) int { calls++; return cmp.Compare(a.Age, b.Age) }))
	m.ToggleSort("Name")
	if got := ids(m.Rows().Get()); !reflect.DeepEqual(got, []int{3, 1, 2}) {
		t.Fatal(got)
	}
	n := calls
	m.Rows().Get()
	m.Select(1, true)
	m.SetPageSize(1)
	m.Rows().Get()
	m.SetPage(2)
	m.Rows().Get()
	if calls != n {
		t.Fatalf("selection/paging reran sort: %d -> %d", n, calls)
	}
	if got := ids(rows.Get()); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatal("source mutated")
	}
}
func TestDataTableControls(t *testing.T) {
	m := dataModel(people())
	m.SetPageSize(2)
	p := ggui.NewProbe(ggui.Column(ui.DataTable(m)).Align(ggui.AlignStretch), ggui.Sz(640, 600))
	defer p.Close()
	p.Tap("Select row 1")
	if !m.IsSelected(1) {
		t.Fatal("row checkbox")
	}
	n, ok := p.Semantics().Find(ggui.RoleCheckbox, "Select page")
	if !ok || n.Checked != ggui.TriMixed {
		t.Fatalf("mixed header: %+v", n)
	}
	p.Tap("Select page")
	if len(m.SelectedRows()) != 2 {
		t.Fatal("select page")
	}
	p.Tap("Next")
	if m.Page() != 2 {
		t.Fatal("next page")
	}
	p.Tap("Sort Name")
	if m.Page() != 1 || m.Sort().Column != "Name" {
		t.Fatal("sort button")
	}
	p.Tap("Filter rows")
	act(t, p, ggui.RoleTextField, "Filter rows", ggui.Action{Kind: ggui.ActionSetValue, Text: "Linus"})
	if len(m.Rows().Get()) != 1 || m.Rows().Get()[0].ID != 3 {
		t.Fatalf("search editor: query %q rows %+v", m.Query(), m.Rows().Get())
	}
	m.SetQuery("no match")
	if _, ok := p.Semantics().Find(ggui.RoleText, "No results."); !ok {
		t.Fatal("empty result message")
	}
	m.SetQuery("")
	p.Tap("Columns")
	p.Tap("Show Age")
	if m.ColumnVisible("Age") {
		t.Fatal("column toggle")
	}
	m.SetColumnVisible("Name", false)
	if !m.ColumnVisible("Name") {
		t.Fatal("hid last column")
	}
}

func TestDataTableReusesCellsAndKeyboardMenu(t *testing.T) {
	builds := 0
	rows := people()
	col := ui.Col("Name", func(p ggui.Readable[person]) ggui.Widget {
		builds++
		return ggui.TextOf(ggui.Map(p, func(p person) string { return p.Name }))
	}).Sortable(func(a, b person) int { return cmp.Compare(a.Name, b.Name) })
	m := ui.NewTableModel(rows, func(p person) int { return p.ID }, col, ui.TextCol("Age", func(p person) string { return strconv.Itoa(p.Age) }))
	p := ggui.NewProbe(ggui.Column(ui.DataTable(m)), ggui.Sz(640, 600))
	defer p.Close()
	p.Frame()
	initial := builds
	m.ToggleSort("Name")
	p.Frame()
	m.ToggleSort("Name")
	p.Frame()
	m.Select(1, true)
	p.Frame()
	if builds != initial {
		t.Fatalf("sorting/selection rebuilt cells: %d -> %d", initial, builds)
	}
	p.Tap("Columns")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if m.ColumnVisible("Name") {
		t.Fatal("keyboard column toggle")
	}
	if !m.IsSelected(1) {
		t.Fatal("visibility lost selection")
	}
}

func TestDataTableNarrowLayout(t *testing.T) {
	m := dataModel(people())
	m.SetPageSize(2)
	p := ggui.NewProbe(ggui.Column(ui.DataTable(m)).Align(ggui.AlignStretch), ggui.Sz(280, 600))
	defer p.Close()
	next, ok := p.Find("Next")
	if !ok || next.Rect.Origin.Y+next.Rect.Size.H > 600 || next.Rect.Origin.X+next.Rect.Size.W > 280 {
		t.Fatalf("pagination overflow: %+v", next)
	}
	p.Tap("Next")
	if m.Page() != 2 {
		t.Fatal("narrow next")
	}
	m.SetPage(1)
	p.Frame()
	p.Scroll(ggui.Pt(140, 80), ggui.Pt(-100, 0))
	if _, ok := p.Find("Sort Age"); !ok {
		t.Fatal("cannot reveal right column")
	}
}

func TestDataTableEmptyMessageStaysInsideNarrowViewport(t *testing.T) {
	m := dataModel(people())
	m.SetQuery("absent")
	p := ggui.NewProbe(ggui.Column(ui.DataTable(m)).Align(ggui.AlignStretch), ggui.Sz(280, 600))
	defer p.Close()
	n, ok := p.Semantics().Find(ggui.RoleText, "No results.")
	if !ok {
		t.Fatal("missing empty message")
	}
	if n.Rect.Origin.X < 0 || n.Rect.Origin.X+n.Rect.Size.W > 280 {
		t.Fatalf("empty message clipped: %+v", n.Rect)
	}
}

func TestDataTableWideColumnsRemainReachable(t *testing.T) {
	cols := make([]ui.Column[person], 6)
	for i := range cols {
		cols[i] = ui.TextCol("Column "+strconv.Itoa(i), func(p person) string { return p.Name }).W(180).Sortable(func(a, b person) int { return cmp.Compare(a.Name, b.Name) })
	}
	m := ui.NewTableModel(people(), func(p person) int { return p.ID }, cols...)
	p := ggui.NewProbe(ggui.Column(ui.DataTable(m)).Align(ggui.AlignStretch), ggui.Sz(640, 600))
	defer p.Close()
	p.Scroll(ggui.Pt(320, 80), ggui.Pt(-1000, 0))
	if _, ok := p.Find("Sort Column 5"); !ok {
		t.Fatal("last fixed-width column is unreachable")
	}
	p.Tap("Sort Column 5")
	if m.Sort().Column != "Column 5" {
		t.Fatal("last column cannot sort")
	}
}
