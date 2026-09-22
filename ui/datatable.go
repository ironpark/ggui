package ui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// TableSort describes a single column sort. An empty Column preserves source order.
type TableSort struct {
	Column     string
	Descending bool
}

// Toggle cycles id through ascending, descending and source order.
func (s TableSort) Toggle(id string) TableSort {
	switch {
	case s.Column != id:
		return TableSort{Column: id}
	case !s.Descending:
		return TableSort{Column: id, Descending: true}
	default:
		return TableSort{}
	}
}

// Mark is the heading suffix that shows whether id orders the rows.
func (s TableSort) Mark(id string) string {
	if s.Column != id {
		return " ↕"
	}
	return pick(s.Descending, " ↓", " ↑")
}

// sortHeading is a ghost heading button that toggles sorting by id and shows
// the current direction from sort.
func sortHeading(title string, sort ggui.Readable[TableSort], id string, toggle func()) *ButtonWidget {
	text := ggui.TextOf(ggui.Derived(func() string { return title + sort.Get().Mark(id) })).NoWrap()
	return ButtonOf(text, toggle).Ghost().Pad(0, 0).Name("Sort " + title)
}

// TableModel owns client-side DataTable operations independently of presentation.
// Source rows and column IDs must have unique, stable keys. Treat returned slices
// and maps as immutable. Construct the model once in component setup.
type TableModel[T any, K comparable] struct {
	rows                       ggui.Readable[[]T]
	key                        func(T) K
	cols                       []Column[T]
	query                      *ggui.StateValue[string]
	sort                       *ggui.StateValue[TableSort]
	page, pageSize             *ggui.StateValue[int]
	hidden                     *ggui.StateValue[map[string]bool]
	selected                   *ggui.StateValue[map[K]bool]
	search                     *ggui.DerivedValue[[][]string]
	filtered, ordered, visible *ggui.DerivedValue[[]T]
}

// NewTableModel creates a memoized filter → stable sort → page pipeline.
// TextCol participates in case-insensitive search; Sortable opts into sorting.
// The default page size is ten. Selection persists across filtering and pages.
func NewTableModel[T any, K comparable](rows ggui.Readable[[]T], key func(T) K, cols ...Column[T]) *TableModel[T, K] {
	m := &TableModel[T, K]{rows: rows, key: key, cols: slices.Clone(cols), query: ggui.State(""), sort: ggui.State(TableSort{}), page: ggui.State(1), pageSize: ggui.State(10), hidden: ggui.State(map[string]bool{}), selected: ggui.State(map[K]bool{})}
	ids := map[string]bool{}
	for i := range m.cols {
		c := &m.cols[i]
		if c.ID == "" {
			c.ID = c.Title
		}
		if c.ID == "" || ids[c.ID] {
			panic("ui.NewTableModel: columns require unique nonempty IDs (use Identified)")
		}
		ids[c.ID] = true
	}
	// Search text is lowered once per source change, not per keystroke.
	m.search = ggui.Derived(func() [][]string {
		rows := m.rows.Get()
		texts := make([][]string, len(rows))
		for i, r := range rows {
			for _, c := range m.cols {
				if c.Search != nil {
					texts[i] = append(texts[i], strings.ToLower(c.Search(r)))
				}
			}
		}
		return texts
	})
	m.filtered = ggui.Derived(func() []T {
		rows, q := m.rows.Get(), strings.ToLower(strings.TrimSpace(m.query.Get()))
		if q == "" {
			return rows
		}
		texts := m.search.Get()
		result := make([]T, 0, len(rows))
		for i, r := range rows {
			if slices.ContainsFunc(texts[i], func(t string) bool { return strings.Contains(t, q) }) {
				result = append(result, r)
			}
		}
		return result
	})
	m.ordered = ggui.Derived(func() []T {
		rows, sort := m.filtered.Get(), m.sort.Get()
		c := m.sortable(sort.Column)
		if c == nil {
			return rows
		}
		result := slices.Clone(rows)
		slices.SortStableFunc(result, func(a, b T) int {
			if sort.Descending {
				return c.Compare(b, a)
			}
			return c.Compare(a, b)
		})
		return result
	})
	m.visible = ggui.Derived(func() []T {
		rows := m.ordered.Get()
		size := m.pageSize.Get()
		start := min((m.Page()-1)*size, len(rows))
		return rows[start:min(start+size, len(rows))]
	})
	return m
}

// sortable returns the column with id when it has a comparator.
func (m *TableModel[T, K]) sortable(id string) *Column[T] {
	for i := range m.cols {
		if m.cols[i].ID == id && m.cols[i].Compare != nil {
			return &m.cols[i]
		}
	}
	return nil
}

// countSelected returns how many of rows are selected.
func (m *TableModel[T, K]) countSelected(rows []T) int {
	selected, n := m.selected.Get(), 0
	for _, r := range rows {
		if selected[m.key(r)] {
			n++
		}
	}
	return n
}

// Rows returns the current page as a cached reactive reader.
func (m *TableModel[T, K]) Rows() ggui.Readable[[]T] { return m.visible }

// FilteredRows returns matching rows in source order, before paging.
func (m *TableModel[T, K]) FilteredRows() ggui.Readable[[]T] { return m.filtered }

// Query returns the search text.
func (m *TableModel[T, K]) Query() string { return m.query.Get() }

// SetQuery searches all searchable columns and resets to page one.
func (m *TableModel[T, K]) SetQuery(q string) {
	if q != m.query.Get() {
		m.query.Set(q)
		m.page.Set(1)
	}
}

// Sort returns the current ordering.
func (m *TableModel[T, K]) Sort() TableSort { return m.sort.Get() }

// ToggleSort cycles ascending, descending, source order for a sortable column.
func (m *TableModel[T, K]) ToggleSort(id string) {
	if m.sortable(id) == nil {
		return
	}
	m.sort.Set(m.sort.Get().Toggle(id))
	m.page.Set(1)
}

// PageCount is at least one, including when there are no matches.
func (m *TableModel[T, K]) PageCount() int {
	n := len(m.filtered.Get())
	size := m.pageSize.Get()
	return max(1, (n+size-1)/size)
}

// Page returns the one-based page, clamped after source changes.
func (m *TableModel[T, K]) Page() int { return clamp(m.page.Get(), 1, m.PageCount()) }

// SetPage navigates to a one-based page.
func (m *TableModel[T, K]) SetPage(page int) { m.page.Set(clamp(page, 1, m.PageCount())) }

// PageSize returns the maximum rows on a page.
func (m *TableModel[T, K]) PageSize() int { return m.pageSize.Get() }

// SetPageSize clamps size to at least one and resets to page one.
func (m *TableModel[T, K]) SetPageSize(size int) {
	size = max(1, size)
	if size != m.pageSize.Get() {
		m.pageSize.Set(size)
		m.page.Set(1)
	}
}

// ColumnVisible reports visibility by stable column ID.
func (m *TableModel[T, K]) ColumnVisible(id string) bool { return !m.hidden.Get()[id] }

// SetColumnVisible changes visibility, keeping at least one data column visible.
func (m *TableModel[T, K]) SetColumnVisible(id string, visible bool) {
	found, count := false, 0
	for _, c := range m.cols {
		if c.ID == id {
			found = true
		}
		if m.ColumnVisible(c.ID) {
			count++
		}
	}
	if !found || (!visible && m.ColumnVisible(id) && count <= 1) {
		return
	}
	hidden := maps.Clone(m.hidden.Get())
	if visible {
		delete(hidden, id)
	} else {
		hidden[id] = true
	}
	m.hidden.Set(hidden)
}

// IsSelected reports selection by row key.
func (m *TableModel[T, K]) IsSelected(key K) bool { return m.selected.Get()[key] }

// Select updates one key without disturbing other pages.
func (m *TableModel[T, K]) Select(key K, selected bool) {
	next := maps.Clone(m.selected.Get())
	if selected {
		next[key] = true
	} else {
		delete(next, key)
	}
	m.selected.Set(next)
}

// SelectPage selects or clears only rows on the current filtered page.
func (m *TableModel[T, K]) SelectPage(selected bool) {
	next := maps.Clone(m.selected.Get())
	for _, r := range m.visible.Get() {
		if selected {
			next[m.key(r)] = true
		} else {
			delete(next, m.key(r))
		}
	}
	m.selected.Set(next)
}

// SelectedRows returns selected items still present in the source, in source order.
func (m *TableModel[T, K]) SelectedRows() []T {
	selected := m.selected.Get()
	var rows []T
	for _, r := range m.rows.Get() {
		if selected[m.key(r)] {
			rows = append(rows, r)
		}
	}
	return rows
}

// pageSelection reports whether every or only some rows on the page are selected.
type pageSelection struct{ all, mixed bool }

func (m *TableModel[T, K]) pageSelection() pageSelection {
	rows := m.visible.Get()
	n := m.countSelected(rows)
	return pageSelection{all: len(rows) > 0 && n == len(rows), mixed: n > 0 && n < len(rows)}
}

// DataTable composes search, column visibility, row checkboxes, sortable
// headings, empty results and pagination around a TableModel using existing
// native controls. Columns may use arbitrary Col cells for row menus, badges or
// actions. Keep the model outside reactive presentation rebuilds.
func DataTable[T any, K comparable](m *TableModel[T, K]) ggui.Widget {
	return ggui.Component(func() ggui.Widget {
		search := TextField(ggui.Bind(m.Query, m.SetQuery)).Placeholder("Filter rows...").Name("Filter rows")
		entries := make([]ggui.Widget, 0, len(m.cols))
		for _, c := range m.cols {
			title := c.Title
			if title == "" {
				title = c.ID
			}
			item := MenuItem(title, func() { m.SetColumnVisible(c.ID, !m.ColumnVisible(c.ID)) })
			item.SetName("Show " + title)
			item.text.BindContent(ggui.Derived(func() string {
				if m.ColumnVisible(c.ID) {
					return "✓  " + title
				}
				return "    " + title
			}))
			entries = append(entries, item)
		}
		toolbar := ggui.Row(ggui.Expanded(search), Menu("Columns", entries...)).Gap(12)
		isEmpty := ggui.Map(m.visible, func(rows []T) bool { return len(rows) == 0 })
		page := ggui.Derived(m.pageSelection)
		table := ggui.Reactive(func() ggui.Widget {
			cols := []Column[T]{Col("", func(row ggui.Readable[T]) ggui.Widget {
				key := m.key(row.Get())
				return Checkbox(ggui.Bind(func() bool { return m.IsSelected(key) }, func(v bool) { m.Select(key, v) }), "").Name(fmt.Sprintf("Select row %v", key))
			}).W(40)}
			cols[0].Header = Checkbox(ggui.Bind(func() bool { return page.Get().all }, m.SelectPage), "").Name("Select page").
				BindIndeterminate(ggui.Map(page, func(p pageSelection) bool { return p.mixed })).BindDisabled(isEmpty)
			// Only visibility rebuilds the table. Sorting and data changes update its
			// keyed rows; heading labels read the sort state independently.
			for _, c := range m.cols {
				if m.ColumnVisible(c.ID) {
					if c.Compare != nil {
						c.Header = sortHeading(c.Title, m.sort, c.ID, func() { m.ToggleSort(c.ID) })
					}
					cols = append(cols, c)
				}
			}
			t := Table(m.visible, m.key, cols...).RowHeight(48).BindSelectedRows(m.selected)
			empty := ggui.View(isEmpty, func(empty bool) ggui.Widget {
				if empty {
					return ggui.Box().Height(96)
				}
				return ggui.Box()
			})
			box := ggui.Box(ggui.Column(t, empty).Align(ggui.AlignStretch))
			minWidth := 0.0
			for _, c := range cols {
				minWidth += cmp.Or(c.Width, 160)
			}
			return &dataTableFrame{minWidth: max(560, minWidth), box: box, scroll: ggui.Scroll(box).Horizontal(), isEmpty: isEmpty, message: ggui.Center(Caption("No results."))}
		})
		summary := ggui.TextOf(ggui.Derived(func() string {
			rows := m.filtered.Get()
			return fmt.Sprintf("%d of %d row(s) selected.", m.countSelected(rows), len(rows))
		})).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption)
		pageLabel := ggui.TextOf(ggui.Derived(func() string { return fmt.Sprintf("Page %d of %d", m.Page(), m.PageCount()) })).NoWrap()
		previous := Button("Previous", func() { m.SetPage(m.Page() - 1) }).Outline().BindDisabled(ggui.Derived(func() bool { return m.Page() <= 1 }))
		next := Button("Next", func() { m.SetPage(m.Page() + 1) }).Outline().BindDisabled(ggui.Derived(func() bool { return m.Page() >= m.PageCount() }))
		size := ggui.Box(Select(ggui.Bind(m.PageSize, m.SetPageSize)).Options([]int{5, 10, 20, 50}).Name("Rows per page")).Width(72)
		footer := ggui.Wrap(summary, ggui.Row(Caption("Rows per page"), size).Gap(8).Align(ggui.AlignCenter), pageLabel, ggui.Row(previous, next).Gap(8)).Gap(12).Align(ggui.AlignCenter)
		return ggui.Column(toolbar, table, footer).Gap(16).Align(ggui.AlignStretch)
	})
}

// A narrow container scrolls whole columns rather than crushing their text.
type dataTableFrame struct {
	minWidth float64
	box      *ggui.BoxWidget
	scroll   *ggui.ScrollWidget
	isEmpty  ggui.Readable[bool]
	message  ggui.Widget
}

func (f *dataTableFrame) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.From(env)
	width := c.MaxW
	if width == ggui.Unbounded {
		width = f.minWidth
	}
	f.box.Width(max(f.minWidth, width)).Border(1, t.Border).Radius(t.Radius)
	natural := f.box.Layout(ggui.Constraints{MaxW: max(f.minWidth, width), MaxH: ggui.Unbounded}, env)
	c.MinH = min(c.MinH, natural.H)
	c.MaxH = min(c.MaxH, natural.H)
	size := f.scroll.Layout(c, env)
	if f.isEmpty.Get() {
		f.message.Layout(ggui.Constraints{MinW: size.W, MaxW: size.W, MinH: 96, MaxH: 96}, env)
	}
	return size
}
func (f *dataTableFrame) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(f.scroll, r)
	if f.isEmpty.Get() {
		dst.Clip(r).Paint(f.message, ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+40), ggui.Sz(r.Size.W, 96)))
	}
}
