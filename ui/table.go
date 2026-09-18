package ui

import (
	"github.com/ironpark/ggui"
)

// Column describes one column of a Table: its heading, how wide it is and
// how a row's cell is built. Build one with Col or TextCol and tune it with
// the setters.
type Column[T any] struct {
	Title string
	Width float64 // fixed width in logical pixels; 0 shares the rest by Flex
	Flex  float64 // share of the remaining width; 0 with Width 0 counts as 1
	Align float64 // 0 left, 0.5 center, 1 right, for the heading and TextCol cells
	Cell  func(ggui.Readable[T]) ggui.Widget
}

// Col creates a column whose cells come from cell, which gets the row's
// item as a Readable that follows the list.
func Col[T any](title string, cell func(ggui.Readable[T]) ggui.Widget) Column[T] {
	return Column[T]{Title: title, Cell: cell}
}

// TextCol creates a column of text taken from each item through text,
// which follows the item without a rebuild.
func TextCol[T any](title string, text func(T) string) Column[T] {
	return Col(title, func(r ggui.Readable[T]) ggui.Widget {
		return ggui.TextOf(ggui.Map(r, text)).NoWrap()
	})
}

// W fixes the column's width.
func (c Column[T]) W(w float64) Column[T] { c.Width = w; return c }

// Grow sets the column's share of the width left after the fixed ones.
func (c Column[T]) Grow(flex float64) Column[T] { c.Flex = flex; return c }

// Right aligns the heading and TextCol cells to the right, for numbers.
func (c Column[T]) Right() Column[T] { c.Align = 1; return c }

// Center centers the heading and TextCol cells.
func (c Column[T]) Center() Column[T] { c.Align = 0.5; return c }

// TableWidget is a keyed list of rows under a heading row, with a line
// between rows, hover and selection. Build one with Table.
type TableWidget[T any, K comparable] struct {
	cols     []Column[T]
	rows     ggui.Readable[[]T]
	key      func(T) K
	selected ggui.Binding[K]
	onSelect func(T)
	rowH     float64
	height   float64
	label    func(T) string

	body    *ggui.EachWidget[T, K]
	head    *ggui.RowWidget
	headBox *ggui.BoxWidget
	scroll  *ggui.ScrollWidget
	column  *ggui.ColumnWidget
	theme   ggui.Theme
	headH   float64
}

// Table creates a table over rows, one row per item keyed by key so a
// row's widgets survive edits and reorders, with one column per cols.
//
//	ui.Table(people, func(p Person) int { return p.ID },
//		ui.TextCol("Name", func(p Person) string { return p.Name }),
//		ui.TextCol("Age", func(p Person) string { return strconv.Itoa(p.Age) }).W(60).Right(),
//	).Selected(chosen).Height(240)
func Table[T any, K comparable](rows ggui.Readable[[]T], key func(T) K, cols ...Column[T]) *TableWidget[T, K] {
	t := &TableWidget[T, K]{cols: cols, rows: rows, key: key, rowH: 32}
	t.label = func(item T) string { return sprint(key(item)) }
	heads := make([]ggui.Widget, len(cols))
	for i, c := range cols {
		heads[i] = t.cell(c, ggui.Text(c.Title).NoWrap().Align(c.Align))
	}
	t.head = ggui.Row(heads...)
	t.headBox = ggui.Box(t.head)
	t.body = ggui.EachKeyed(rows, key, func(row ggui.EachItem[T]) ggui.Widget { return t.row(row.Value) }).Gap(0).Align(ggui.AlignStretch).ItemExtent(t.rowH)
	t.scroll = ggui.Scroll(t.body)
	t.column = ggui.Column(t.headBox, t.body).Align(ggui.AlignStretch)
	return t
}

// Selected binds the key of the highlighted row: a click on a row sets
// it, and rows take focus so Space or Enter select too.
func (t *TableWidget[T, K]) Selected(b ggui.Binding[K]) *TableWidget[T, K] { t.selected = b; return t }

// OnSelect fires with the item whose row was clicked or activated.
func (t *TableWidget[T, K]) OnSelect(fn func(T)) *TableWidget[T, K] { t.onSelect = fn; return t }

// RowHeight fixes every row's height; the default is 32. With Height the
// body then lays out only the rows in view.
func (t *TableWidget[T, K]) RowHeight(h float64) *TableWidget[T, K] {
	t.rowH = h
	t.body.ItemExtent(h)
	return t
}

// Height bounds the table and scrolls the body under a fixed heading.
// Without it the table is as tall as its rows.
func (t *TableWidget[T, K]) Height(h float64) *TableWidget[T, K] {
	t.height = h
	t.column = ggui.Column(t.headBox, ggui.Expanded(t.scroll)).Align(ggui.AlignStretch)
	return t
}

// RowName names each row for Probe.Find and the inspector; the default is
// the key.
func (t *TableWidget[T, K]) RowName(fn func(T) string) *TableWidget[T, K] { t.label = fn; return t }

func (t *TableWidget[T, K]) selectable() bool { return t.selected != nil || t.onSelect != nil }

// cell sizes w for column c: a fixed Box or a Flex share.
func (t *TableWidget[T, K]) cell(c Column[T], w ggui.Widget) ggui.Widget {
	if c.Width > 0 {
		return ggui.Box(w).Width(c.Width)
	}
	return ggui.Flex(w, pick(c.Flex > 0, c.Flex, 1))
}

func (t *TableWidget[T, K]) row(item ggui.Readable[T]) ggui.Widget {
	cells := make([]ggui.Widget, len(t.cols))
	for i, c := range t.cols {
		cells[i] = t.cell(c, c.Cell(item))
	}
	r := &tableRow[T, K]{table: t, item: item, key: t.key(item.Get()), cells: ggui.Row(cells...)}
	r.Role, r.Name = ggui.RoleRow, t.label(item.Get())
	r.AutoKey()
	return r
}

// Layout implements Widget.
func (t *TableWidget[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	th := env.Theme()
	t.theme = th
	t.head.Gap(th.Space)
	t.headBox.Padding(th.ItemPad)
	if t.height > 0 {
		h := min(t.height, c.MaxH)
		c.MinH, c.MaxH = h, h
	}
	sz := t.column.Layout(c, env)
	t.headH = t.headBox.Layout(ggui.Loose(sz), env).H
	return sz
}

// Paint implements Widget.
func (t *TableWidget[T, K]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(t.column, r)
	y := r.Origin.Y + t.headH
	dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, y-1), ggui.Sz(r.Size.W, 1)), t.theme.Border)
}

// tableRow is one row: its cells in a Row, hover and selection.
type tableRow[T any, K comparable] struct {
	ggui.Interactive
	table *TableWidget[T, K]
	item  ggui.Readable[T]
	key   K
	cells *ggui.RowWidget
	box   *ggui.BoxWidget
	theme ggui.Theme
}

func (r *tableRow[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	th := env.Theme()
	r.theme = th
	r.cells.Gap(th.Space).Align(ggui.AlignCenter)
	if r.box == nil {
		r.box = ggui.Box(r.cells)
	}
	r.box.Padding(th.ItemPad)
	return r.box.Layout(c, env)
}

func (r *tableRow[T, K]) chosen() bool {
	return r.table.selected != nil && r.table.selected.Get() == r.key
}

// Describe implements ggui.Describer: a row reports whether it is the
// selected one. Its cells describe themselves inside it.
func (r *tableRow[T, K]) Describe() ggui.Node {
	n := ggui.Node{Role: ggui.RoleRow, Name: r.Name, Selected: r.chosen(), Disabled: r.Inert}
	if r.table.selectable() {
		n.Actions = ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus
	}
	return n
}

func (r *tableRow[T, K]) Paint(dst *ggui.Canvas, rc ggui.Rect) {
	dst.DescribeNode(rc, r, func(dst *ggui.Canvas) { r.paint(dst, rc) })
}

func (r *tableRow[T, K]) paint(dst *ggui.Canvas, rc ggui.Rect) {
	th := r.theme
	if r.table.selectable() {
		r.Hit(dst, rc, r, ggui.CursorShapePointer)
		switch {
		case r.chosen():
			dst.FillRect(rc, th.Selection)
		case r.Hovered:
			dst.FillRect(rc, th.Muted)
		}
	}
	dst.Paint(r.box, rc)
	dst.FillRect(ggui.Rct(ggui.Pt(rc.Origin.X, rc.Origin.Y+rc.Size.H-1), ggui.Sz(rc.Size.W, 1)), th.Border)
	r.FocusRing(dst, rc, th.Radius, th.Ring)
}

func (r *tableRow[T, K]) pick() {
	if r.table.selected != nil {
		r.table.selected.Set(r.key)
	}
	if r.table.onSelect != nil {
		r.table.onSelect(r.item.Get())
	}
}

// Act implements ggui.Actor: selecting the row.
func (r *tableRow[T, K]) Act(a ggui.Action) bool {
	if r.Inert || !r.table.selectable() || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
		return false
	}
	r.pick()
	return true
}

// HandleKey implements KeyHandler: Space or Enter selects the row.
func (r *tableRow[T, K]) HandleKey(ev ggui.KeyEvent) { r.Keyboard(ev, r.pick) }

// HandlePointer implements PointerHandler.
func (r *tableRow[T, K]) HandlePointer(ev ggui.PointerEvent) bool { return r.Pointer(ev, r.pick) }
