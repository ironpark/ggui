package ui

import (
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Column describes one column of a Table: its heading, how wide it is and
// how a row's cell is built. Build one with Col or TextCol and tune it with
// the setters.
type Column[T any] struct {
	Title string
	Width float64 // fixed width in logical pixels; 0 shares the rest by Flex
	Flex  float64 // share of the remaining width; 0 with Width 0 counts as 1
	Align float64 // 0 left, 0.5 center, 1 right, for headings and cells
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

// Right aligns the heading and cells to the right, for numbers.
func (c Column[T]) Right() Column[T] { c.Align = 1; return c }

// Center centers the heading and cells.
func (c Column[T]) Center() Column[T] { c.Align = 0.5; return c }

// TableWidget is a keyed list of rows under a heading row, with a line
// between rows, hover and selection. Build one with Table.
type TableWidget[T any, K comparable] struct {
	props         property.Owner
	cols          []Column[T]
	rows          ggui.Readable[[]T]
	key           func(T) K
	selected      ggui.Binding[K]
	localSelected *ggui.StateValue[K]
	onSelect      func(T)
	rowH          float64
	height        float64
	label         func(T) string

	body    *ggui.EachWidget[T, K]
	head    *ggui.RowWidget
	headBox *ggui.BoxWidget
	scroll  *ggui.ScrollWidget
	column  *ggui.ColumnWidget
	theme   uitheme.Theme
	headH   float64
}

// Table creates a table over rows, one row per item keyed by key so a
// row's widgets survive edits and reorders, with one column per cols.
//
//	ui.Table(people, func(p Person) int { return p.ID },
//		ui.TextCol("Name", func(p Person) string { return p.Name }),
//		ui.TextCol("Age", func(p Person) string { return strconv.Itoa(p.Age) }).W(60).Right(),
//	).BindSelected(chosen).Height(240)
func Table[T any, K comparable](rows ggui.Readable[[]T], key func(T) K, cols ...Column[T]) *TableWidget[T, K] {
	t := &TableWidget[T, K]{cols: cols, rows: rows, key: key, rowH: 40}
	t.label = func(item T) string { return sprint(key(item)) }
	heads := make([]ggui.Widget, len(cols))
	for i, c := range cols {
		heads[i] = t.cell(c, ggui.Text(c.Title).NoWrap().Align(c.Align))
	}
	t.head = ggui.Row(heads...).Align(ggui.AlignStretch)
	t.headBox = ggui.Box(t.head).Height(40)
	t.body = ggui.EachKeyed(rows, key, func(row ggui.EachItem[T]) ggui.Widget { return t.row(row) }).Gap(0).Align(ggui.AlignStretch).ItemExtent(t.rowH)
	t.scroll = ggui.Scroll(t.body)
	t.column = ggui.Column(t.headBox, t.body).Align(ggui.AlignStretch)
	return t
}

// BindSelected binds the key of the highlighted row: a click on a row sets
// it, and rows take focus so Space or Enter select too.
func (t *TableWidget[T, K]) BindSelected(b ggui.Binding[K]) *TableWidget[T, K] {
	property.Require(b, "BindSelected")
	if !property.Same(t.selected, b) {
		t.selected = b
		t.props.Changed()
	}
	return t
}

// Selected detaches a row-selection binding and selects a local row key.
func (t *TableWidget[T, K]) Selected(v K) *TableWidget[T, K] {
	if t.localSelected == nil {
		t.localSelected = ggui.State(v)
	} else {
		t.localSelected.Set(v)
	}
	return t.BindSelected(t.localSelected)
}

// OnSelect fires with the item whose row was clicked or activated.
func (t *TableWidget[T, K]) OnSelect(fn func(T)) *TableWidget[T, K] { t.onSelect = fn; return t }

// RowHeight fixes every row's height; the default is 40. With Height the
// body then lays out only the rows in view.
func (t *TableWidget[T, K]) RowHeight(h float64) *TableWidget[T, K] {
	defer property.Watch(&t.props, &t.rowH)()
	h = max(1, h)
	t.rowH = h
	t.body.ItemExtent(h)
	return t
}

// Height bounds the table and scrolls the body under a fixed heading.
// Without it the table is as tall as its rows.
func (t *TableWidget[T, K]) Height(h float64) *TableWidget[T, K] {
	defer property.Watch(&t.props, &t.column)()
	defer property.Watch(&t.props, &t.height)()
	if (t.height > 0) != (h > 0) {
		if h > 0 {
			t.column = ggui.Column(t.headBox, ggui.Expanded(t.scroll)).Align(ggui.AlignStretch)
		} else {
			t.column = ggui.Column(t.headBox, t.body).Align(ggui.AlignStretch)
		}
	}
	t.height = h
	return t
}

// RowName names each row for Probe.Find and the inspector; the default is
// the key.
func (t *TableWidget[T, K]) RowName(fn func(T) string) *TableWidget[T, K] { t.label = fn; return t }

func (t *TableWidget[T, K]) selectable() bool { return t.selected != nil || t.onSelect != nil }

// cell sizes w for column c: a fixed Box or a Flex share.
func (t *TableWidget[T, K]) cell(c Column[T], w ggui.Widget) ggui.Widget {
	// Align the whole cell, including custom widgets, and clip long content so
	// it cannot paint or receive input in the adjacent column.
	w = &tableCell{box: ggui.Box(ggui.Align(w).At(c.Align, .5)).Pad(8)}
	if c.Width > 0 {
		return ggui.Box(w).Width(c.Width)
	}
	return ggui.Flex(w, pick(c.Flex > 0, c.Flex, 1))
}

func (t *TableWidget[T, K]) row(row ggui.EachItem[T]) ggui.Widget {
	item := row.Value
	cells := make([]ggui.Widget, len(t.cols))
	for i, c := range t.cols {
		cells[i] = t.cell(c, c.Cell(item))
	}
	r := &tableRow[T, K]{table: t, item: item, index: row.Index, key: t.key(item.Get()), cells: ggui.Row(cells...)}
	r.Role = ggui.RoleRow
	r.SetName(t.label(item.Get()))
	r.AutoKey()
	return r
}

// Layout implements Widget.
func (t *TableWidget[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout()()
	th := uitheme.From(env)
	t.theme = th
	t.head.Gap(0)
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
	table  *TableWidget[T, K]
	item   ggui.Readable[T]
	key    K
	index  ggui.Readable[int]
	motion time.Duration
	cells  *ggui.RowWidget
	box    *ggui.BoxWidget
	theme  uitheme.Theme
}

func (r *tableRow[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	th := uitheme.From(env)
	r.theme = th
	r.Sync()
	r.SetName(r.table.label(r.item.Get()))
	r.motion = env.Motion(th.MotionFast)
	r.cells.Gap(0).Align(ggui.AlignStretch)
	if r.box == nil {
		r.box = ggui.Box(r.cells)
	}
	r.box.Height(r.table.rowH)
	return r.box.Layout(c, env)
}

func (r *tableRow[T, K]) chosen() bool {
	return r.table.selected != nil && r.table.selected.Get() == r.key
}

// Describe implements ggui.Describer: a row reports whether it is the
// selected one. Its cells describe themselves inside it.
func (r *tableRow[T, K]) Describe() ggui.Node {
	n := ggui.Node{Role: ggui.RoleRow, Name: r.SemanticName(), Selected: r.chosen(), Disabled: r.IsInert()}
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
	} else if !r.IsInert() {
		dst.HitPointer(rc, r)
	}
	target := 0.0
	if r.chosen() {
		target = 1
	} else if r.Hovered && !r.IsInert() {
		target = .5
	}
	amount := dst.Ease(r.Anchor(rc), tableRowFillSlot, target, r.motion)
	if amount > 0 {
		dst.FillRect(rc, fade(th.Muted, amount))
	}
	dst.Clip(rc).Paint(r.box, rc)
	if r.index.Get() < len(r.table.rows.Get())-1 {
		dst.FillRect(ggui.Rct(ggui.Pt(rc.Origin.X, rc.Origin.Y+rc.Size.H-1), ggui.Sz(rc.Size.W, 1)), th.Border)
	}
	r.FocusRing(dst, rc, th.Radius, th.Ring)
}

func (r *tableRow[T, K]) pick() {
	if r.IsInert() || !r.table.selectable() {
		return
	}
	if r.table.selected != nil {
		r.table.selected.Set(r.key)
	}
	if r.table.onSelect != nil {
		r.table.onSelect(r.item.Get())
	}
}

// Act implements ggui.Actor: selecting the row.
func (r *tableRow[T, K]) Act(a ggui.Action) bool {
	if r.IsInert() || !r.table.selectable() || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
		return false
	}
	r.pick()
	return true
}

// HandleKey implements KeyHandler: Space or Enter selects the row.
func (r *tableRow[T, K]) HandleKey(ev ggui.KeyEvent) {
	if r.IsInert() || !r.table.selectable() {
		return
	}
	r.Keyboard(ev, r.pick)
}

// HandlePointer implements PointerHandler.
func (r *tableRow[T, K]) HandlePointer(ev ggui.PointerEvent) bool {
	if r.IsInert() {
		return false
	}
	if !r.table.selectable() {
		r.Pointer(ev, nil)
		return false
	}
	return r.Pointer(ev, r.pick)
}

var tableRowFillSlot = ggui.NewSlot[*ggui.Motion]("table row fill")

type tableCell struct{ box *ggui.BoxWidget }

func (c *tableCell) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size { return c.box.Layout(cs, env) }
func (c *tableCell) Paint(dst *ggui.Canvas, r ggui.Rect)                { dst.Clip(r).Paint(c.box, r) }
