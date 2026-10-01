package ui

import (
	"maps"
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Column describes one column of a Table: its heading, how wide it is and
// how a row's cell is built. Build one with Col or TextCol and tune it with
// the setters.
type Column[T any] struct {
	Title   string
	Header  ggui.Widget      // optional custom heading
	ID      string           // stable DataTable column identifier; defaults to Title
	Compare func(a, b T) int // optional ascending comparator
	Search  func(T) string   // optional searchable text
	Width   float64          // fixed width in logical pixels; 0 shares the rest by Flex
	Flex    float64          // share of the remaining width; 0 with Width 0 counts as 1
	Align   float64          // 0 left, 0.5 center, 1 right, for headings and cells
	Resize  bool             // the user may drag the heading's right edge; see Resizable
	Cell    func(ggui.Readable[T]) ggui.Widget
}

// Col creates a column whose cells come from cell, which gets the row's
// item as a Readable that follows the list.
func Col[T any](title string, cell func(ggui.Readable[T]) ggui.Widget) Column[T] {
	return Column[T]{Title: title, Cell: cell}
}

// TextCol creates a column of text taken from each item through text,
// which follows the item without a rebuild.
func TextCol[T any](title string, text func(T) string) Column[T] {
	c := Col(title, func(r ggui.Readable[T]) ggui.Widget {
		return ggui.TextOf(ggui.Map(r, text)).NoWrap()
	})
	c.Search = text
	return c
}

// Sortable enables DataTable sorting with an ascending comparator.
func (c Column[T]) Sortable(compare func(T, T) int) Column[T] { c.Compare = compare; return c }

// Identified gives a DataTable column a stable identifier independent of its title.
func (c Column[T]) Identified(id string) Column[T] { c.ID = id; return c }

// W fixes the column's width.
func (c Column[T]) W(w float64) Column[T] { c.Width = w; return c }

// Grow sets the column's share of the width left after the fixed ones.
func (c Column[T]) Grow(flex float64) Column[T] { c.Flex = flex; return c }

// Resizable lets the user drag the right edge of the column's heading to
// change its width, which starts at Width, or 120 without one.
func (c Column[T]) Resizable() Column[T] {
	c.Resize = true
	if c.Width <= 0 {
		c.Width = 120
	}
	return c
}

// Right aligns the heading and cells to the right, for numbers.
func (c Column[T]) Right() Column[T] { c.Align = 1; return c }

// Center centers the heading and cells.
func (c Column[T]) Center() Column[T] { c.Align = 0.5; return c }

// TableWidget is a keyed list of rows under a heading row, with a line
// between rows, hover and selection. Build one with Table.
type TableWidget[T any, K comparable] struct {
	props          property.Owner
	cols           []Column[T]
	rows           ggui.Readable[[]T]
	key            func(T) K
	selected       ggui.Binding[K]
	localSelected  *ggui.StateValue[K]
	selectedRows   ggui.Readable[map[K]bool]
	selection      ggui.Binding[map[K]bool] // BindSelection's, which the table edits
	localSelection *ggui.StateValue[map[K]bool]
	onSelect       func(T)
	rowH           float64
	height         float64
	label          func(T) string

	// The keyboard: the row the arrows move from, the one Tab lands on,
	// the one Shift extends a selection from, and the one to focus when it
	// next paints, which may be a row scrolled into view this frame.
	active, tab, anchor, focus rowCursor[K]
	offset                     *ggui.StateValue[float64]
	bodyH                      float64
	widths                     []*ggui.StateValue[float64] // a Resizable column's width

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
	t.widths = make([]*ggui.StateValue[float64], len(cols))
	heads := make([]ggui.Widget, len(cols))
	for i, c := range cols {
		if c.Resize {
			t.widths[i] = ggui.State(c.Width)
		}
		heading := c.Header
		if heading == nil {
			heading = ggui.Text(c.Title).NoWrap().Ellipsis().Align(c.Align)
		}
		heads[i] = t.cell(i, heading, ggui.RoleHeader)
		if c.Resize {
			heads[i] = &columnGrip{cell: heads[i], width: t.widths[i]}
		}
	}
	t.head = ggui.Row(heads...).Align(ggui.AlignStretch)
	t.headBox = ggui.Box(t.head).Height(40)
	t.body = ggui.EachKeyed(rows, key, func(row ggui.EachItem[T]) ggui.Widget { return t.row(row) }).Gap(0).Align(ggui.AlignStretch).ItemExtent(t.rowH).Unlisted()
	t.offset = ggui.State(0.0)
	t.scroll = ggui.Scroll(t.body).BindOffset(t.offset)
	t.column = ggui.Column(t.headBox, t.body).Align(ggui.AlignStretch)
	return t
}

// rowCursor is a row's key, or none.
type rowCursor[K comparable] struct {
	key K
	ok  bool
}

func at[K comparable](k K) rowCursor[K] { return rowCursor[K]{k, true} }

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

// BindSelectedRows follows a set of highlighted keys. Use OnSelect to define
// how row activation changes the set; child controls may update it independently.
func (t *TableWidget[T, K]) BindSelectedRows(r ggui.Readable[map[K]bool]) *TableWidget[T, K] {
	property.Require(r, "BindSelectedRows")
	if !property.Same(t.selectedRows, r) {
		t.selectedRows = r
		t.props.Changed()
	}
	return t
}

// BindSelection binds a set of selected keys that the table edits the way a
// file list does: a click selects one row, ⌘-click (Ctrl-click elsewhere)
// adds or removes one, Shift-click and Shift with the arrows select the
// run from the row picked last, Space toggles the focused row and ⌘A
// selects them all. Enter still activates the row for OnSelect.
func (t *TableWidget[T, K]) BindSelection(b ggui.Binding[map[K]bool]) *TableWidget[T, K] {
	property.Require(b, "BindSelection")
	if !property.Same(t.selection, b) {
		t.selection, t.selectedRows = b, b
		t.props.Changed()
	}
	return t
}

// Selection is BindSelection with a selection of the table's own, starting
// as keys.
func (t *TableWidget[T, K]) Selection(keys map[K]bool) *TableWidget[T, K] {
	if t.localSelection == nil {
		t.localSelection = ggui.State(keys)
	} else {
		t.localSelection.Set(keys)
	}
	return t.BindSelection(t.localSelection)
}

// SelectedRows replaces the highlighted-key reader with a literal set.
func (t *TableWidget[T, K]) SelectedRows(rows map[K]bool) *TableWidget[T, K] {
	return t.BindSelectedRows(ggui.Const(rows))
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

func (t *TableWidget[T, K]) selectable() bool {
	return t.selected != nil || t.onSelect != nil || t.selection != nil
}

// tabStop is the row Tab lands on: the active row, else the selected one,
// else the first.
func (t *TableWidget[T, K]) tabStop(rows []T) rowCursor[K] {
	var first, chosen rowCursor[K]
	var sel rowCursor[K]
	if t.selected != nil {
		sel = at(ggui.Untrack(t.selected.Get))
	}
	for i, item := range rows {
		k := t.key(item)
		switch {
		case t.active.ok && k == t.active.key:
			return t.active
		case i == 0:
			first = at(k)
		}
		if sel.ok && k == sel.key {
			chosen = sel
		}
	}
	if chosen.ok {
		return chosen
	}
	return first
}

// move takes the keyboard to row i, clamped to the rows: it becomes the
// active row and takes the focus when it paints, scrolled into view, and
// with a selection bound it is selected, or with extend the run from the
// anchor to it is.
func (t *TableWidget[T, K]) move(i int, extend bool) {
	rows := ggui.Untrack(t.rows.Get)
	if len(rows) == 0 {
		return
	}
	i = min(max(i, 0), len(rows)-1)
	k := t.key(rows[i])
	t.active, t.focus = at(k), at(k)
	switch {
	case t.selection != nil:
		t.selectAt(rows, i, extend, false)
	case t.selected != nil:
		t.selected.Set(k)
	}
	t.reveal(i)
}

// selectAt changes the bound selection for row i: to that row alone, to
// the run from the anchor with extend, or with toggle by adding or
// removing the row.
func (t *TableWidget[T, K]) selectAt(rows []T, i int, extend, toggle bool) {
	k := t.key(rows[i])
	next := map[K]bool{}
	switch {
	case extend && t.anchor.ok:
		from := slices.IndexFunc(rows, func(item T) bool { return t.key(item) == t.anchor.key })
		if from < 0 {
			from = i
		}
		for j := min(from, i); j <= max(from, i); j++ {
			next[t.key(rows[j])] = true
		}
	case toggle:
		maps.Copy(next, ggui.Untrack(t.selection.Get))
		if next[k] {
			delete(next, k)
		} else {
			next[k] = true
		}
		t.anchor = at(k)
	default:
		next[k] = true
		t.anchor = at(k)
	}
	t.selection.Set(next)
}

// reveal scrolls the body of a table with a Height so that row i is in
// view: the row may not be laid out yet, so the focus cannot do it.
func (t *TableWidget[T, K]) reveal(i int) {
	if t.height <= 0 || t.bodyH <= 0 {
		return
	}
	top, off := float64(i)*t.rowH, ggui.Untrack(t.offset.Get)
	switch {
	case top < off:
		off = top
	case top+t.rowH > off+t.bodyH:
		off = top + t.rowH - t.bodyH
	default:
		return
	}
	t.offset.Set(off)
}

// page is how many rows a Page key moves.
func (t *TableWidget[T, K]) page() int {
	if t.height <= 0 {
		return 10
	}
	return max(1, int(t.bodyH/t.rowH)-1)
}

// cell sizes w for column c: a fixed Box or a Flex share. role is the
// node it describes itself as, a heading or a cell.
func (t *TableWidget[T, K]) cell(i int, w ggui.Widget, role ggui.Role) ggui.Widget {
	c := t.cols[i]
	// Align the whole cell, including custom widgets, and clip long content so
	// it cannot paint or receive input in the adjacent column.
	w = &tableCell{box: ggui.Box(ggui.Align(w).At(c.Align, .5)).Pad(8), role: role}
	if width := t.widths[i]; width != nil {
		return ggui.Box(w).BindWidth(width)
	}
	if c.Width > 0 {
		return ggui.Box(w).Width(c.Width)
	}
	return ggui.Flex(w, pick(c.Flex > 0, c.Flex, 1))
}

func (t *TableWidget[T, K]) row(row ggui.EachItem[T]) ggui.Widget {
	item := row.Value
	cells := make([]ggui.Widget, len(t.cols))
	for i, c := range t.cols {
		cells[i] = t.cell(i, c.Cell(item), ggui.RoleCell)
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
	t.bodyH = sz.H - t.headH
	t.tab = t.tabStop(t.rows.Get())
	return sz
}

// Paint implements Widget.
func (t *TableWidget[T, K]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleTable}, func(dst *ggui.Canvas) { dst.Paint(t.column, r) })
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
	return (r.table.selectedRows != nil && r.table.selectedRows.Get()[r.key]) || (r.table.selected != nil && r.table.selected.Get() == r.key)
}

// Describe implements ggui.Describer: a row reports whether it is the
// selected one. Its cells describe themselves inside it.
func (r *tableRow[T, K]) Describe() ggui.Node {
	n := ggui.Node{Role: ggui.RoleRow, Name: r.SemanticName(), Selected: r.chosen(), Disabled: r.IsInert(),
		Min: 1, Now: float64(r.index.Get() + 1), Max: float64(len(r.table.rows.Get()))}
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
		if t := r.table; t.focus.ok && t.focus.key == r.key {
			dst.RequestFocus(r)
			t.focus = rowCursor[K]{}
		}
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

// pick activates the row: it becomes the selected row of BindSelected and
// goes to OnSelect.
func (r *tableRow[T, K]) pick() {
	if r.IsInert() || !r.table.selectable() {
		return
	}
	r.table.active = at(r.key)
	if r.table.selected != nil {
		r.table.selected.Set(r.key)
	}
	if r.table.onSelect != nil {
		r.table.onSelect(r.item.Get())
	}
}

// click is pick for a click, which with BindSelection first changes the
// selection by the modifiers held.
func (r *tableRow[T, K]) click(mods ggui.Mods) {
	if t := r.table; t.selection != nil && !r.IsInert() {
		t.selectAt(ggui.Untrack(t.rows.Get), ggui.Untrack(r.index.Get), mods.Shift, mods.Cmd())
	}
	r.pick()
}

// Act implements ggui.Actor: selecting the row.
func (r *tableRow[T, K]) Act(a ggui.Action) bool {
	if r.IsInert() || !r.table.selectable() || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
		return false
	}
	r.click(ggui.Mods{})
	return true
}

// HandleKey implements KeyHandler: Space or Enter selects the row, and the
// arrows, Home, End and the Page keys move between rows.
func (r *tableRow[T, K]) HandleKey(ev ggui.KeyEvent) {
	t := r.table
	if r.IsInert() || !t.selectable() {
		return
	}
	if ev.Kind == ggui.KeyFocus {
		t.active = at(r.key)
	}
	if ev.Kind == ggui.KeyPress {
		i, n := ggui.Untrack(r.index.Get), len(ggui.Untrack(t.rows.Get))
		switch ev.Key {
		case ggui.KeyArrowUp:
			t.move(i-1, ev.Mods.Shift)
			return
		case ggui.KeyArrowDown:
			t.move(i+1, ev.Mods.Shift)
			return
		case ggui.KeyHome:
			t.move(0, ev.Mods.Shift)
			return
		case ggui.KeyEnd:
			t.move(n-1, ev.Mods.Shift)
			return
		case ggui.KeyPageUp:
			t.move(i-t.page(), ev.Mods.Shift)
			return
		case ggui.KeyPageDown:
			t.move(i+t.page(), ev.Mods.Shift)
			return
		case ggui.KeyA:
			if t.selection != nil && ev.Mods.Cmd() {
				all := make(map[K]bool, n)
				for _, item := range ggui.Untrack(t.rows.Get) {
					all[t.key(item)] = true
				}
				t.selection.Set(all)
				return
			}
		case ggui.KeySpace:
			if t.selection != nil {
				t.selectAt(ggui.Untrack(t.rows.Get), i, false, true)
				return
			}
		}
	}
	r.Keyboard(ev, r.pick)
}

// ConsumesKey implements ggui.KeyConsumer: the keys that move between rows
// stay with the table.
func (r *tableRow[T, K]) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd, ggui.KeyPageUp, ggui.KeyPageDown:
		return ev.Kind == ggui.KeyPress
	}
	return r.Interactive.ConsumesKey(ev)
}

// ClaimsChord implements ggui.ChordClaimer: ⌘A selects the rows of a table
// with a selection bound.
func (r *tableRow[T, K]) ClaimsChord(ev ggui.KeyEvent) bool {
	return r.table.selection != nil && ev.Kind == ggui.KeyPress && ev.Key == ggui.KeyA && ev.Mods.Cmd()
}

// TabStop implements ggui.TabStopper: Tab lands on one row, and the arrows
// reach the others.
func (r *tableRow[T, K]) TabStop() bool { return !r.table.tab.ok || r.table.tab.key == r.key }

// HandlePointer implements PointerHandler.
func (r *tableRow[T, K]) HandlePointer(ev ggui.PointerEvent) bool {
	if r.IsInert() {
		return false
	}
	if !r.table.selectable() {
		r.Pointer(ev, nil)
		return false
	}
	return r.Pointer(ev, func() { r.click(ev.Mods) })
}

// columnGrip is a Resizable column's heading: dragging its right edge
// changes the width every cell of the column follows.
type columnGrip struct {
	cell         ggui.Widget
	width        *ggui.StateValue[float64]
	from, startX float64 // the width and pointer x a drag started at
}

// gripWidth is how far in from a heading's right edge a drag takes hold.
const gripWidth = 6

func (g *columnGrip) Layout(c ggui.Constraints, env ggui.Env) ggui.Size { return g.cell.Layout(c, env) }

func (g *columnGrip) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(g.cell, r)
	edge := ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-gripWidth, r.Origin.Y), ggui.Sz(gripWidth, r.Size.H))
	dst.HitPointer(edge, g)
	dst.HitCursor(edge, ggui.CursorShapeEWResize)
}

// HandlePointer implements ggui.PointerHandler.
func (g *columnGrip) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerDown:
		g.from, g.startX = ggui.Untrack(g.width.Get), ev.Pos.X
	case ggui.PointerDrag:
		g.width.Set(max(24, g.from+ev.Pos.X-g.startX))
	case ggui.PointerScroll:
		return false
	}
	return true
}

var tableRowFillSlot = ggui.NewSlot[*ggui.Motion]("table row fill")

type tableCell struct {
	box  *ggui.BoxWidget
	role ggui.Role
}

func (c *tableCell) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size { return c.box.Layout(cs, env) }
func (c *tableCell) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: c.role}, func(dst *ggui.Canvas) { dst.Clip(r).Paint(c.box, r) })
}
