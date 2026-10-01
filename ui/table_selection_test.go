package ui_test

import (
	"cmp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestTableOnSelectAloneMakesRowsSelectable(t *testing.T) {
	t.Parallel()
	var picked []string
	tbl := ui.Table(people(), func(p person) int { return p.ID },
		ui.TextCol("Name", func(p person) string { return p.Name }),
	).RowName(func(p person) string { return p.Name }).OnSelect(func(p person) { picked = append(picked, p.Name) })
	p := ggui.NewProbe(tbl, ggui.Sz(300, 200))
	defer p.Close()
	p.Tap("Grace")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	row, _ := p.Semantics().Find(ggui.RoleRow, "Linus")
	p.Perform(row.ID, ggui.Action{Kind: ggui.ActionSelect})
	if want := []string{"Grace", "Grace", "Linus"}; !slices.Equal(picked, want) {
		t.Fatalf("OnSelect saw %q, want %q", picked, want)
	}
	if row.Selected {
		t.Fatal("a table without a selection binding marked a row selected")
	}
}

func TestTableSelectedAndSelectedRowsMarkRows(t *testing.T) {
	t.Parallel()
	bound := ggui.State(1)
	tbl := ui.Table(people(), func(p person) int { return p.ID },
		ui.TextCol("Name", func(p person) string { return p.Name }),
	).RowName(func(p person) string { return p.Name }).BindSelected(bound).Selected(3).SelectedRows(map[int]bool{2: true})
	p := ggui.NewProbe(tbl, ggui.Sz(300, 200))
	defer p.Close()
	selected := func() []string {
		var out []string
		for _, n := range p.Semantics().Nodes(ggui.RoleRow) {
			if n.Selected {
				out = append(out, n.Name)
			}
		}
		return out
	}
	if got := selected(); !slices.Equal(got, []string{"Grace", "Linus"}) {
		t.Fatalf("selected rows %q, want Grace from SelectedRows and Linus from Selected", got)
	}
	p.Tap("Ada")
	if got := selected(); !slices.Equal(got, []string{"Ada", "Grace"}) || ggui.Untrack(bound.Get) != 1 {
		t.Fatalf("after tapping Ada: selected %q, detached binding %d; want Ada and Grace, binding untouched", got, ggui.Untrack(bound.Get))
	}
}

func TestTableColumnSizingAndAlignment(t *testing.T) {
	t.Parallel()
	tbl := ui.Table(people(), func(p person) int { return p.ID },
		ui.TextCol("Name", func(p person) string { return p.Name }).Grow(3),
		ui.TextCol("Age", func(p person) string { return strconv.Itoa(p.Age) }).Center(),
		ui.TextCol("ID", func(p person) string { return strconv.Itoa(p.ID) }).W(40).Right(),
	)
	p := ggui.NewProbe(tbl, ggui.Sz(440, 200))
	defer p.Close()
	tree := p.Semantics()
	name, _ := tree.Find(ggui.RoleText, "Name")
	age, _ := tree.Find(ggui.RoleText, "Age")
	id, _ := tree.Find(ggui.RoleText, "ID")
	// 400px of flexible width shared 3:1: Name's cell is 300 wide, Age's 100.
	if center := age.Rect.Origin.X + age.Rect.Size.W/2; center < 340 || center > 360 {
		t.Fatalf("centered Age heading is centered at %v, want about 350 (middle of 300..400)", center)
	}
	if end := id.Rect.Origin.X + id.Rect.Size.W; end < 424 || end > 440 {
		t.Fatalf("right-aligned ID heading ends at %v, want near the table's right edge", end)
	}
	if name.Rect.Origin.X > 16 {
		t.Fatalf("left-aligned Name heading starts at %v", name.Rect.Origin.X)
	}
}

func TestTableModelFilteredRowsKeepSourceOrderBeforePaging(t *testing.T) {
	t.Parallel()
	m := dataModel(people())
	m.SetPageSize(1)
	m.ToggleSort("Age")
	m.SetQuery("a") // Ada and Grace
	if got := ids(m.FilteredRows().Get()); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("filtered rows %v, want [1 2] in source order", got)
	}
	if got := ids(m.Rows().Get()); !slices.Equal(got, []int{1}) {
		t.Fatalf("first sorted page %v, want [1] (Ada is younger)", got)
	}
}

func TestTableColumnIdentifiedReplacesTheTitleAsID(t *testing.T) {
	t.Parallel()
	m := ui.NewTableModel(people(), func(p person) int { return p.ID },
		ui.TextCol("Age", func(p person) string { return strconv.Itoa(p.Age) }).Identified("age").Sortable(func(a, b person) int { return cmp.Compare(a.Age, b.Age) }),
	)
	m.ToggleSort("Age")
	if m.Sort().Column != "" {
		t.Fatal("the title still sorts a column that has an ID")
	}
	m.ToggleSort("age")
	if got := ids(m.Rows().Get()); !slices.Equal(got, []int{3, 1, 2}) {
		t.Fatalf("sorting by ID age gave %v, want [3 1 2]", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("two columns with one ID did not panic")
		}
	}()
	ui.NewTableModel(people(), func(p person) int { return p.ID },
		ui.TextCol("A", func(person) string { return "" }).Identified("x"),
		ui.TextCol("B", func(person) string { return "" }).Identified("x"),
	)
}

// fuzzRow is a row with many ties, so that stability shows.
type fuzzRow struct {
	ID    int
	Name  string
	Score int
}

// FuzzTableModelPagesAreTheSortedFilter checks the DataTable pipeline over
// random rows, queries, sorts and page sizes against a direct reference:
// filtered rows keep source order, every page holds at most the page size,
// the pages concatenate to the stable sort of the filter, and pages clamp.
func FuzzTableModelPagesAreTheSortedFilter(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, "a", uint8(1), uint8(3))
	f.Add([]byte{5, 5, 5, 5, 5, 5}, "", uint8(6), uint8(1))
	f.Add([]byte{}, "zz", uint8(0), uint8(0))
	f.Add([]byte{1, 2, 3, 250, 17, 99, 0, 0}, " B ", uint8(3), uint8(4))
	f.Fuzz(func(t *testing.T, raw []byte, query string, sortOps, size uint8) {
		if len(raw) > 80 || len(query) > 4 {
			t.Skip()
		}
		var rows []fuzzRow
		for i := 0; i+1 < len(raw); i += 2 {
			name := string(rune('a'+raw[i]%5)) + string(rune('A'+raw[i+1]%3))
			rows = append(rows, fuzzRow{ID: i / 2, Name: name, Score: int(raw[i+1] % 4)})
		}
		byName := func(a, b fuzzRow) int { return cmp.Compare(a.Name, b.Name) }
		byScore := func(a, b fuzzRow) int { return cmp.Compare(a.Score, b.Score) }
		source := ggui.State(rows)
		m := ui.NewTableModel(source, func(r fuzzRow) int { return r.ID },
			ui.TextCol("Name", func(r fuzzRow) string { return r.Name }).Sortable(byName),
			ui.TextCol("Score", func(r fuzzRow) string { return strconv.Itoa(r.Score) }).Sortable(byScore),
		)
		m.SetPageSize(int(size % 7))
		m.SetQuery(query)
		column := []string{"Name", "Score"}[sortOps&1]
		for range sortOps >> 1 & 3 {
			m.ToggleSort(column)
		}
		// The reference pipeline.
		q := strings.ToLower(strings.TrimSpace(query))
		var want []fuzzRow
		for _, r := range rows {
			if q == "" || strings.Contains(strings.ToLower(r.Name), q) || strings.Contains(strconv.Itoa(r.Score), q) {
				want = append(want, r)
			}
		}
		if got := m.FilteredRows().Get(); !slices.Equal(got, want) {
			t.Fatalf("filter %q = %v, want %v", query, got, want)
		}
		if s := m.Sort(); s.Column != "" {
			compare := map[string]func(a, b fuzzRow) int{"Name": byName, "Score": byScore}[s.Column]
			want = slices.Clone(want)
			slices.SortStableFunc(want, func(a, b fuzzRow) int {
				if s.Descending {
					return compare(b, a)
				}
				return compare(a, b)
			})
		}
		pageSize := max(1, int(size%7))
		if got, wantPages := m.PageCount(), max(1, (len(want)+pageSize-1)/pageSize); got != wantPages {
			t.Fatalf("%d rows in pages of %d: PageCount %d, want %d", len(want), pageSize, got, wantPages)
		}
		var all []fuzzRow
		for page := 1; page <= m.PageCount(); page++ {
			m.SetPage(page)
			rows := m.Rows().Get()
			if len(rows) > pageSize || len(rows) == 0 && len(want) > 0 {
				t.Fatalf("page %d holds %d rows, want 1..%d", page, len(rows), pageSize)
			}
			all = append(all, rows...)
		}
		if !slices.Equal(all, want) {
			t.Fatalf("pages concatenate to %v, want %v", all, want)
		}
		m.SetPage(-3)
		if m.Page() != 1 {
			t.Fatalf("SetPage(-3) left page %d", m.Page())
		}
		m.SetPage(1 << 20)
		last := m.Page()
		if last != m.PageCount() {
			t.Fatalf("SetPage(huge) left page %d of %d", last, m.PageCount())
		}
		source.Set(rows[:len(rows)/2])
		if p := m.Page(); p < 1 || p > m.PageCount() {
			t.Fatalf("after the source shrank the page is %d of %d", p, m.PageCount())
		}
	})
}

func focusedName(t *testing.T, p *ggui.Probe) string {
	t.Helper()
	n, ok := p.Semantics().Focused()
	if !ok {
		return ""
	}
	return n.Name
}

func TestTableArrowsMoveFocusAndTheSelection(t *testing.T) {
	t.Parallel()
	tbl, chosen := table(people())
	p := ggui.NewProbe(tbl, ggui.Sz(300, 200))
	defer p.Close()
	p.Tap("Ada")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	if got := focusedName(t, p); got != "Grace" || ggui.Untrack(chosen.Get) != 2 {
		t.Fatalf("after Down: focus on %q, selected %d; want Grace, 2", got, ggui.Untrack(chosen.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	p.Type(ggui.Mods{}, ggui.KeyArrowDown) // stays on the last row
	if got := focusedName(t, p); got != "Linus" || ggui.Untrack(chosen.Get) != 3 {
		t.Fatalf("after End: focus on %q, selected %d; want Linus, 3", got, ggui.Untrack(chosen.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyHome)
	if got := focusedName(t, p); got != "Ada" {
		t.Fatalf("after Home: focus on %q, want Ada", got)
	}
}

func TestTableIsOneTabStop(t *testing.T) {
	t.Parallel()
	tbl, chosen := table(people())
	chosen.Set(2)
	p := ggui.NewProbe(ggui.Column(ui.Button("before", func() {}), tbl, ui.Button("after", func() {})), ggui.Sz(300, 300))
	defer p.Close()
	var stops []string
	for range 3 {
		p.Type(ggui.Mods{}, ggui.KeyTab)
		stops = append(stops, focusedName(t, p))
	}
	if want := []string{"before", "Grace", "after"}; !slices.Equal(stops, want) {
		t.Fatalf("Tab stopped at %q, want %q: the selected row is the table's one stop", stops, want)
	}
}

func TestTableArrowsScrollAVirtualBody(t *testing.T) {
	t.Parallel()
	var many []person
	for i := range 200 {
		many = append(many, person{ID: i + 1, Name: "p" + strconv.Itoa(i+1)})
	}
	tbl, chosen := table(ggui.State(many))
	tbl.Height(200)
	p := ggui.NewProbe(tbl, ggui.Sz(300, 200))
	defer p.Close()
	p.Tap("p1")
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	if got := focusedName(t, p); got != "p200" || ggui.Untrack(chosen.Get) != 200 {
		t.Fatalf("after End: focus on %q, selected %d; want the last row, scrolled to", got, ggui.Untrack(chosen.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyPageUp)
	if got := ggui.Untrack(chosen.Get); got >= 200 || got < 190 {
		t.Fatalf("PageUp selected %d, want a page above the last row", got)
	}
	if focusedName(t, p) != "p"+strconv.Itoa(ggui.Untrack(chosen.Get)) {
		t.Fatal("the focus did not follow PageUp")
	}
}

func TestTableBindSelectionPicksLikeAFileList(t *testing.T) {
	t.Parallel()
	sel := ggui.State(map[int]bool{})
	activated := 0
	tbl := ui.Table(people(), func(p person) int { return p.ID },
		ui.TextCol("Name", func(p person) string { return p.Name }),
	).RowName(func(p person) string { return p.Name }).BindSelection(sel).OnSelect(func(person) { activated++ })
	p := ggui.NewProbe(tbl, ggui.Sz(300, 200))
	defer p.Close()
	keys := func() []int {
		var out []int
		for k := range ggui.Untrack(sel.Get) {
			out = append(out, k)
		}
		slices.Sort(out)
		return out
	}
	at := func(name string) ggui.Point {
		f, ok := p.Find(name)
		if !ok {
			t.Fatalf("no row %q", name)
		}
		return f.Center()
	}
	p.Tap("Ada")
	p.ClickWith(at("Linus"), ggui.Mods{Shift: true})
	if got := keys(); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("Shift-click selected %v, want the run 1..3", got)
	}
	toggle := ggui.Mods{Ctrl: true}
	if runtime.GOOS == "darwin" {
		toggle = ggui.Mods{Meta: true}
	}
	p.ClickWith(at("Grace"), toggle)
	if got := keys(); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("⌘-click left %v, want Grace taken out", got)
	}
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if got := keys(); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("Space left %v, want Grace back", got)
	}
	p.Tap("Ada")
	p.Type(ggui.Mods{Shift: true}, ggui.KeyArrowDown)
	if got := keys(); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("Shift+Down selected %v, want 1 and 2", got)
	}
	p.Type(toggle, ggui.KeyA)
	if got := keys(); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("⌘A selected %v, want every row", got)
	}
	if activated == 0 {
		t.Fatal("a click no longer reaches OnSelect")
	}
}

func TestTableResizableColumnFollowsADragOnItsHeading(t *testing.T) {
	t.Parallel()
	tbl := ui.Table(people(), func(p person) int { return p.ID },
		ui.TextCol("Name", func(p person) string { return p.Name }).W(100).Resizable(),
		ui.TextCol("Age", func(p person) string { return strconv.Itoa(p.Age) }),
	)
	p := ggui.NewProbe(tbl, ggui.Sz(400, 200))
	defer p.Close()
	widths := func() (head, cell float64) {
		tree := p.Semantics()
		for _, n := range tree.Nodes(ggui.RoleHeader) {
			head = n.Full.Size.W
			break
		}
		for _, n := range tree.Nodes(ggui.RoleCell) {
			cell = n.Full.Size.W
			break
		}
		return head, cell
	}
	if h, c := widths(); h != 100 || c != 100 {
		t.Fatalf("before the drag: heading %v, cell %v; want 100", h, c)
	}
	p.Press(ggui.Pt(98, 20))
	p.Move(ggui.Pt(148, 20))
	p.Release(ggui.Pt(148, 20))
	if h, c := widths(); h != 150 || c != 150 {
		t.Fatalf("after dragging 50 right: heading %v, cell %v; want 150", h, c)
	}
}
