package ui_test

import (
	"cmp"
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
