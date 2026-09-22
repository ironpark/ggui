package main

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"math"
	"testing"
)

func TestResultValueOrder(t *testing.T) {
	values := []any{nil, math.Inf(-1), int64(-2), float64(-1.5), int64(2), float64(10), float64(9007199254740992), int64(9007199254740993), math.Inf(1), "10", "2", []byte{0}, []byte{255}}
	for i, a := range values {
		for j, b := range values {
			got := compareValue(a, b)
			if (i < j && got >= 0) || (i == j && got != 0) || (i > j && got <= 0) {
				t.Fatalf("compare(%v, %v)=%d", a, b, got)
			}
		}
	}
	if compareValue(int64(2), float64(2)) != 0 {
		t.Fatal("equivalent numbers differ")
	}
}
func TestSQLResultSearchUsesFullValuesAndDuplicateNames(t *testing.T) {
	long := ""
	for range 140 {
		long += "x"
	}
	long += "needle"
	rows := ggui.Const([]record{{ID: 1, Values: []any{int64(10), long}}, {ID: 2, Values: []any{int64(2), nil}}})
	m := ui.NewTableModel(rows, func(r record) int { return r.ID }, resultColumn(0, "value"), resultColumn(1, "value"))
	m.ToggleSort("column-0")
	if m.Rows().Get()[0].ID != 2 {
		t.Fatal("numeric order")
	}
	m.SetQuery("needle")
	if len(m.Rows().Get()) != 1 || m.Rows().Get()[0].ID != 1 {
		t.Fatal("search used truncated text")
	}
}
func TestHeadingSortQueriesWholeDatabase(t *testing.T) {
	db, path := fixture(t)
	if _, err := db.Exec(`CREATE TABLE numbers(id INTEGER PRIMARY KEY); WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<510) INSERT INTO numbers SELECT x FROM n`); err != nil {
		t.Fatal(err)
	}
	m := newModel()
	defer m.close()
	m.open(path)
	m.selectTable("numbers")
	p := newProbe(t, m)
	p.Tap("Select row 2")
	if m.Selected.Get() != 2 {
		t.Fatal("checkbox did not select record")
	}
	p.Tap("Select row 2")
	if m.Selected.Get() != 0 {
		t.Fatal("checkbox did not deselect record")
	}
	p.Tap("Sort by id")
	p.Tap("Sort by id")
	if m.Data.Get().Rows[0].Values[0] != int64(510) || !m.Desc.Get() {
		t.Fatal("sort only affected loaded page")
	}
	if m.Offset.Get() != 0 || m.Selected.Get() != 0 {
		t.Fatal("sort retained stale page/selection")
	}
	p.Tap("Sort by id")
	if m.Sort.Get() != "" || m.Data.Get().Rows[0].Values[0] != int64(1) {
		t.Fatal("sort reset")
	}
	m.Filter.Set("absent")
	p.Tap("Apply")
	if _, ok := p.Semantics().Find("", "No matching rows"); !ok {
		t.Fatal("empty state missing")
	}
	p.Tap("More")
	p.Tap("Clear filters")
	if len(m.Data.Get().Rows) != 500 {
		t.Fatal("clear did not restore rows")
	}
}
func TestQueryResultsUseDataTable(t *testing.T) {
	_, path := fixture(t)
	m := newModel()
	defer m.close()
	m.open(path)
	m.SQL.Set(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<12) SELECT x AS number FROM n`)
	m.execute()
	p := newProbe(t, m)
	if _, ok := p.Find("Sort number"); !ok {
		t.Fatal("query sorting missing")
	}
	p.Scroll(ggui.Pt(600, 650), ggui.Pt(0, -100))
	p.Tap("Next")
	p.Scroll(ggui.Pt(600, 500), ggui.Pt(0, 100))
	if _, ok := p.Find("Select row 6"); !ok {
		t.Fatal("query pagination failed")
	}
	n, ok := p.Semantics().Find(ggui.RoleTextField, "Filter rows")
	if !ok {
		t.Fatal("query filter missing")
	}
	p.Perform(n.ID, ggui.Action{Kind: ggui.ActionSetValue, Text: "12"})
	if len(p.FindAll(ggui.RoleRow)) != 1 {
		t.Fatal("query filter failed")
	}
	if len(m.QueryData.Get().Rows) != 12 {
		t.Fatal("presentation altered export source")
	}
}

func TestBrowseMultipleSelection(t *testing.T) {
	db, path := fixture(t)
	if _, err := db.Exec(`INSERT INTO composite VALUES('second',3,'two'),('third',4,'three')`); err != nil {
		t.Fatal(err)
	}
	m := newModel()
	defer m.close()
	m.open(path)
	m.selectTable("composite")
	p := newProbe(t, m)
	p.Tap("Select row 1")
	p.Tap("Select row 2")
	if len(m.Selections.Get()) != 2 || m.Selected.Get() != 0 {
		t.Fatal("multi-selection replaced first row")
	}
	for _, label := range []string{"Select row 1", "Select row 2"} {
		n, ok := p.Semantics().Find(ggui.RoleCheckbox, label)
		if !ok || n.Checked != ggui.TriOn {
			t.Fatal("checkbox lost selection", label)
		}
	}
	n, _ := p.Semantics().Find(ggui.RoleCheckbox, "Select page")
	if n.Checked != ggui.TriMixed {
		t.Fatal("header not mixed")
	}
	n, _ = p.Semantics().Find(ggui.RoleButton, "View / edit cell")
	if !n.Disabled {
		t.Fatal("ambiguous edit enabled")
	}
	p.Tap("Select row 1")
	if m.Selected.Get() != 2 || len(m.Selections.Get()) != 1 {
		t.Fatal("deselection changed other row")
	}
	p.Tap("Select page")
	if len(m.Selections.Get()) != 3 {
		t.Fatal("select all")
	}
	p.Tap("Deselect")
	if len(m.Selections.Get()) != 0 || m.Selected.Get() != 0 {
		t.Fatal("clear all")
	}
	p.Tap("Select page")
	p.Tap("Refresh")
	if len(m.Selections.Get()) != 0 {
		t.Fatal("refresh retained stale row IDs")
	}
}

func TestDatabasePaginationControls(t *testing.T) {
	db, path := fixture(t)
	if _, err := db.Exec(`CREATE TABLE pages(id INTEGER PRIMARY KEY); WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<63) INSERT INTO pages SELECT x FROM n`); err != nil {
		t.Fatal(err)
	}
	m := newModel()
	defer m.close()
	m.open(path)
	m.selectTable("pages")
	pageSizeBinding{m}.Set(25)
	if m.Total.Get() != 63 || m.pageCount() != 3 || len(m.Data.Get().Rows) != 25 {
		t.Fatal("page size/count")
	}
	p := newProbe(t, m)
	p.Tap("Page 3")
	if m.Offset.Get() != 50 || len(m.Data.Get().Rows) != 13 || m.Data.Get().Limited {
		t.Fatal("last page")
	}
	m.selectRow(1, true)
	p.Tap("Previous")
	if m.Offset.Get() != 25 || m.selectionCount() != 0 {
		t.Fatal("previous page/selection")
	}
	m.Filter.Set("63")
	m.apply()
	if m.Total.Get() != 1 || m.pageCount() != 1 || m.Offset.Get() != 0 {
		t.Fatal("filtered count")
	}
	m.Filter.Set("")
	m.apply()
	databasePage{m}.Set(3)
	if _, err := db.Exec(`DELETE FROM pages WHERE id>2`); err != nil {
		t.Fatal(err)
	}
	m.browse()
	if m.Offset.Get() != 0 || m.Total.Get() != 2 || len(m.Data.Get().Rows) != 2 {
		t.Fatal("shrinking result did not clamp page")
	}
}
