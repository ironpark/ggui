package ui

import (
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
	m.filtered = ggui.Derived(func() []T {
		rows, q := m.rows.Get(), strings.ToLower(strings.TrimSpace(m.query.Get()))
		if q == "" {
			return rows
		}
		result := make([]T, 0, len(rows))
		for _, r := range rows {
			for _, c := range m.cols {
				if c.Search != nil && strings.Contains(strings.ToLower(c.Search(r)), q) {
					result = append(result, r)
					break
				}
			}
		}
		return result
	})
	m.ordered = ggui.Derived(func() []T {
		rows, sort := m.filtered.Get(), m.sort.Get()
		for _, c := range m.cols {
			if c.ID == sort.Column && c.Compare != nil {
				result := slices.Clone(rows)
				slices.SortStableFunc(result, func(a, b T) int {
					if sort.Descending {
						return c.Compare(b, a)
					}
					return c.Compare(a, b)
				})
				return result
			}
		}
		return rows
	})
	m.visible = ggui.Derived(func() []T {
		rows := m.ordered.Get()
		size := m.pageSize.Get()
		start := min((m.Page()-1)*size, len(rows))
		return rows[start:min(start+size, len(rows))]
	})
	return m
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
	for _, c := range m.cols {
		if c.ID == id && c.Compare != nil {
			s := m.sort.Get()
			if s.Column != id {
				s = TableSort{Column: id}
			} else if !s.Descending {
				s.Descending = true
			} else {
				s = TableSort{}
			}
			m.sort.Set(s)
			m.page.Set(1)
			return
		}
	}
}

// PageCount is at least one, including when there are no matches.
func (m *TableModel[T, K]) PageCount() int {
	n := len(m.filtered.Get())
	size := m.pageSize.Get()
	return max(1, n/size+pick(n%size != 0, 1, 0))
}

// Page returns the one-based page, clamped after source changes.
func (m *TableModel[T, K]) Page() int { return min(max(1, m.page.Get()), m.PageCount()) }

// SetPage navigates to a one-based page.
func (m *TableModel[T, K]) SetPage(page int) { m.page.Set(min(max(1, page), m.PageCount())) }

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
func (m *TableModel[T, K]) pageSelection() (all, mixed bool) {
	rows := m.visible.Get()
	selected := m.selected.Get()
	n := 0
	for _, r := range rows {
		if selected[m.key(r)] {
			n++
		}
	}
	return len(rows) > 0 && n == len(rows), n > 0 && n < len(rows)
}

type tableBinding[V any] struct {
	read  func() V
	write func(V)
}

func (b tableBinding[V]) Get() V  { return b.read() }
func (b tableBinding[V]) Set(v V) { b.write(v) }

// DataTableWidget composes search, column visibility, row checkboxes, sortable
// headings, empty results and pagination using existing native controls.
type DataTableWidget[T any, K comparable] struct {
	child ggui.Widget
}

// DataTable presents a TableModel. Columns may use arbitrary Col cells for row
// menus, badges or actions. Keep the model outside reactive presentation rebuilds.
func DataTable[T any, K comparable](m *TableModel[T, K]) *DataTableWidget[T, K] {
	d := &DataTableWidget[T, K]{}
	d.child = ggui.Component(func() ggui.Widget {
		search := TextField(tableBinding[string]{m.Query, m.SetQuery}).Placeholder("Filter rows...").Name("Filter rows")
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
		table := ggui.Reactive(func() ggui.Widget {
			cols := []Column[T]{Col("", func(row ggui.Readable[T]) ggui.Widget {
				key := m.key(row.Get())
				return Checkbox(tableBinding[bool]{func() bool { return m.IsSelected(key) }, func(v bool) { m.Select(key, v) }}, "").Name(fmt.Sprintf("Select row %v", key))
			}).W(40)}
			all := Checkbox(tableBinding[bool]{func() bool { all, _ := m.pageSelection(); return all }, m.SelectPage}, "").Name("Select page").BindIndeterminate(ggui.Derived(func() bool { _, mixed := m.pageSelection(); return mixed })).BindDisabled(ggui.Derived(func() bool { return len(m.visible.Get()) == 0 }))
			cols[0].Header = all
			// Only visibility rebuilds the table. Sorting and data changes update its
			// keyed rows; heading labels read the sort state independently.
			for _, c := range m.cols {
				if m.ColumnVisible(c.ID) {
					if c.Compare != nil {
						id := c.ID
						title := c.Title
						c.Header = ButtonOf(ggui.TextOf(ggui.Derived(func() string {
							s := m.Sort()
							mark := " ↕"
							if s.Column == id {
								mark = pick(s.Descending, " ↓", " ↑")
							}
							return title + mark
						})).NoWrap(), func() { m.ToggleSort(id) }).Ghost().Pad(0, 0).Name("Sort " + title)
					}
					cols = append(cols, c)
				}
			}
			t := Table(m.visible, m.key, cols...).RowHeight(48)
			t.selectedRows = m.selected
			isEmpty := ggui.Map(m.visible, func(rows []T) bool { return len(rows) == 0 })
			empty := ggui.View(isEmpty, func(empty bool) ggui.Widget {
				if empty {
					return ggui.Box().Height(96)
				}
				return ggui.Box()
			})
			box := ggui.Box(ggui.Column(t, empty).Align(ggui.AlignStretch))
			minWidth := 0.0
			for _, c := range cols {
				if c.Width > 0 {
					minWidth += c.Width
				} else {
					minWidth += 160
				}
			}
			return &dataTableFrame{minWidth: max(560, minWidth), box: box, scroll: ggui.Scroll(box).Horizontal(), isEmpty: isEmpty, message: ggui.Center(Caption("No results."))}
		})
		summary := ggui.TextOf(ggui.Derived(func() string {
			selected := m.selected.Get()
			rows := m.filtered.Get()
			n := 0
			for _, r := range rows {
				if selected[m.key(r)] {
					n++
				}
			}
			return fmt.Sprintf("%d of %d row(s) selected.", n, len(rows))
		})).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption)
		page := ggui.TextOf(ggui.Derived(func() string { return fmt.Sprintf("Page %d of %d", m.Page(), m.PageCount()) })).NoWrap()
		previous := Button("Previous", func() { m.SetPage(m.Page() - 1) }).Outline().BindDisabled(ggui.Derived(func() bool { return m.Page() <= 1 }))
		next := Button("Next", func() { m.SetPage(m.Page() + 1) }).Outline().BindDisabled(ggui.Derived(func() bool { return m.Page() >= m.PageCount() }))
		size := ggui.Box(Select(tableBinding[int]{m.PageSize, m.SetPageSize}).Options([]int{5, 10, 20, 50}).Name("Rows per page")).Width(72)
		footer := ggui.Wrap(summary, ggui.Row(Caption("Rows per page"), size).Gap(8).Align(ggui.AlignCenter), page, ggui.Row(previous, next).Gap(8)).Gap(12).Align(ggui.AlignCenter)
		return ggui.Column(toolbar, table, footer).Gap(16).Align(ggui.AlignStretch)
	})
	return d
}

func (d *DataTableWidget[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return d.child.Layout(c, env)
}
func (d *DataTableWidget[T, K]) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(d.child, r) }

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
