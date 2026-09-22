package main

import (
	"bytes"
	"cmp"
	"fmt"
	"math/big"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Keep presentation separate from raw values: sorting and searching never use
// the abbreviated cell label, and duplicate SQL column names have distinct IDs.
func resultColumn(index int, name string) ui.Column[record] {
	c := ui.Col(name, func(row ggui.Readable[record]) ggui.Widget {
		return ggui.View(ggui.Map(row, func(r record) any { return r.Values[index] }), func(value any) ggui.Widget {
			text := ggui.Text(cellText(value)).NoWrap()
			if value == nil {
				text.StyleKey(uitheme.CaptionKey, uitheme.Default().Caption)
			}
			return text
		})
	}).Identified(fmt.Sprintf("column-%d", index))
	c.Search = func(row record) string { return display(row.Values[index]) }
	return c.Sortable(func(a, b record) int { return compareValue(a.Values[index], b.Values[index]) })
}

// Match SQLite's storage-class order for cached query results. Exact rational
// comparison preserves INTEGER precision above 2^53 when mixed with REALs.
func compareValue(a, b any) int {
	rank := func(v any) int {
		switch v.(type) {
		case nil:
			return 0
		case int64, float64:
			return 1
		case string:
			return 2
		case []byte:
			return 3
		default:
			return 4
		}
	}
	if d := cmp.Compare(rank(a), rank(b)); d != 0 {
		return d
	}
	switch a := a.(type) {
	case nil:
		return 0
	case int64:
		if b, ok := b.(int64); ok {
			return cmp.Compare(a, b)
		}
		return compareNumber(a, b)
	case float64:
		if b, ok := b.(float64); ok {
			return cmp.Compare(a, b)
		}
		return compareNumber(a, b)
	case string:
		return strings.Compare(a, b.(string))
	case []byte:
		return bytes.Compare(a, b.([]byte))
	default:
		return strings.Compare(display(a), display(b))
	}
}
func compareNumber(a, b any) int {
	number := func(v any) *big.Rat {
		if n, ok := v.(int64); ok {
			return new(big.Rat).SetInt64(n)
		}
		return new(big.Rat).SetFloat64(v.(float64))
	}
	// SQLite normally maps NaN to NULL. Handle non-finite REALs without panics.
	ar, br := number(a), number(b)
	if ar != nil && br != nil {
		return ar.Cmp(br)
	}
	floating := func(v any) float64 {
		if n, ok := v.(int64); ok {
			return float64(n)
		}
		return v.(float64)
	}
	return cmp.Compare(floating(a), floating(b))
}

func queryResultTable(source ggui.Readable[result]) ggui.Widget {
	return ggui.View(source, func(r result) ggui.Widget {
		if len(r.Columns) == 0 {
			return ggui.Padding(ui.Empty("No result set", "Run a SELECT statement to explore rows here."), 24)
		}
		return ggui.Component(func() ggui.Widget {
			cols := make([]ui.Column[record], len(r.Columns))
			for i, name := range r.Columns {
				width := 160.0
				if len(name) > 20 {
					width = 220
				}
				cols[i] = resultColumn(i, name).Grow(width)
				if numericColumn(r, i) {
					cols[i] = cols[i].Right()
				}
			}
			table := ui.NewTableModel(ggui.Const(r.Rows), func(r record) int { return r.ID }, cols...)
			table.SetPageSize(5)
			notice := "Search and sort the loaded SQL result. Export includes all loaded rows."
			if r.Limited {
				notice = "Showing the first 500 rows. Refine your SQL to retrieve a different result window."
			}
			return ggui.Column(ui.Caption(notice), ui.DataTable(table)).Gap(12).Align(ggui.AlignStretch)
		})
	})
}

// Mixed storage classes stay left-aligned; numeric columns align by magnitude.
func numericColumn(r result, index int) bool {
	found := false
	for _, row := range r.Rows {
		switch row.Values[index].(type) {
		case nil:
		case int64, float64:
			found = true
		default:
			return false
		}
	}
	return found
}

// Selection is scoped to the loaded page; a single selected record remains the
// editing target. Multiple selections never choose an arbitrary edit/delete target.
type rowSelection struct {
	model *model
	id    int
}

func (s rowSelection) Get() bool   { return s.model.Selections.Get()[s.id] }
func (s rowSelection) Set(on bool) { s.model.selectRow(s.id, on) }
func (m *model) selectRow(id int, on bool) {
	next := make(map[int]bool, len(m.Selections.Get())+1)
	for key, value := range m.Selections.Get() {
		if value {
			next[key] = true
		}
	}
	if on {
		next[id] = true
	} else {
		delete(next, id)
	}
	m.Selections.Set(next)
	target := 0
	if len(next) == 1 {
		for key := range next {
			target = key
		}
	}
	m.Selected.Set(target)
}
func (m *model) clearSelection() { m.Selections.Set(map[int]bool{}); m.Selected.Set(0) }
func (m *model) selectionCount() int {
	if n := len(m.Selections.Get()); n > 0 {
		return n
	}
	if m.Selected.Get() > 0 {
		return 1
	}
	return 0
}
func (m *model) selectPage(on bool) {
	next := map[int]bool{}
	if on {
		for _, row := range m.Data.Get().Rows {
			next[row.ID] = true
		}
	}
	m.Selections.Set(next)
	target := 0
	if len(next) == 1 {
		for key := range next {
			target = key
		}
	}
	m.Selected.Set(target)
}

type pageSelection struct{ model *model }

func (s pageSelection) Get() bool {
	return len(s.model.Data.Get().Rows) > 0 && len(s.model.Selections.Get()) == len(s.model.Data.Get().Rows)
}
func (s pageSelection) Set(on bool) { s.model.selectPage(on) }

func (m *model) pageCount() int {
	n, size := m.Total.Get(), max(1, m.PageSize.Get())
	return max(1, (n+size-1)/size)
}

type databasePage struct{ model *model }

func (b databasePage) Get() int { return b.model.Offset.Get()/max(1, b.model.PageSize.Get()) + 1 }
func (b databasePage) Set(page int) {
	m := b.model
	if m.Busy.Get() {
		return
	}
	page = min(max(1, page), m.pageCount())
	m.browsePage((page - 1) * m.PageSize.Get())
}

type pageSizeBinding struct{ model *model }

func (b pageSizeBinding) Get() int { return b.model.PageSize.Get() }
func (b pageSizeBinding) Set(size int) {
	m := b.model
	if m.Busy.Get() {
		return
	}
	m.browseSizedPage(0, min(rowLimit, max(1, size)))
}
