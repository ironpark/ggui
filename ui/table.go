package ui

import (
	"image/color"
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
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
		return widget.TextOf(ggui.Map(r, text)).NoWrap()
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
//
// Rows are at least a line of text inside the cell padding tall, 37 under
// the stock themes. A table whose body does not scroll (no Height,
// MaxHeight or MaxRows) and has no RowHeight lets a row grow to its
// tallest cell, so a Button or a field in a cell keeps its own height.
// Under a scrolling body every row has one height, which the body needs to
// lay out only the rows in view: the text row's unless RowHeight sets
// another, so give such a table a RowHeight to make room for controls.
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
	rowSet         bool // RowHeight chose rowH; otherwise it follows the theme
	height         float64
	maxH           float64 // MaxHeight's limit, 0 without one
	maxRows        int     // MaxRows' limit, 0 without one
	label          func(T) string

	// The keyboard: the rows' Tab stop and focus, and the row Shift
	// extends a selection from.
	nav    rovingRows[T, K]
	anchor rowCursor[K]
	offset *ggui.StateValue[float64]
	bodyH  float64
	widths []*ggui.StateValue[float64] // a Resizable column's width

	body    *widget.EachWidget[T, K]
	head    *widget.RowWidget
	headBox *widget.BoxWidget
	foot    *widget.RowWidget  // the footer's cells, nil without Footer
	footBox *widget.BoxWidget  // foot at the height of a row
	caption *widget.TextWidget // nil without Caption
	capSize ggui.Size
	toolBox *widget.BoxWidget // Toolbar's widget in the cell padding, nil without one
	toolSz  ggui.Size
	scroll  *widget.ScrollWidget
	column  *widget.ColumnWidget
	theme   *uitheme.Theme
	headH   float64
	footH   float64
	footBg  color.Color // the footer surface, resolved each layout
	capH    float64
	toolH   float64
}

// tableLabelText gives a table's headings and footer the theme's medium
// label weight, whatever text widgets they hold.
func tableLabelText(e ggui.Env) ggui.Env { return e.WithText(uitheme.Of(e).Label) }

// tableRowHeight is the height of a row of text under t: a line inside the
// cell padding, and the line below the row.
func tableRowHeight(t *uitheme.Theme) float64 {
	return t.Text.Size*t.Text.LineHeight + 2*t.Space + t.BorderWidth
}

// Table creates a table over rows, one row per item keyed by key so a
// row's widgets survive edits and reorders, with one column per cols.
//
//	ui.Table(people, func(p Person) int { return p.ID },
//		ui.TextCol("Name", func(p Person) string { return p.Name }),
//		ui.TextCol("Age", func(p Person) string { return strconv.Itoa(p.Age) }).W(60).Right(),
//	).BindSelected(chosen).Height(240)
func Table[T any, K comparable](rows ggui.Readable[[]T], key func(T) K, cols ...Column[T]) *TableWidget[T, K] {
	t := &TableWidget[T, K]{cols: cols, rows: rows, key: key, rowH: 37} // until the first layout reads the theme
	t.label = func(item T) string { return sprint(key(item)) }
	t.widths = make([]*ggui.StateValue[float64], len(cols))
	heads := make([]ggui.Widget, len(cols))
	for i, c := range cols {
		if c.Resize {
			t.widths[i] = ggui.State(c.Width)
		}
		heading := c.Header
		if heading == nil {
			heading = widget.Text(c.Title).NoWrap().Ellipsis().Align(c.Align)
		}
		heads[i] = t.cell(i, heading, ggui.RoleHeader)
		if c.Resize {
			heads[i] = &columnGrip{cell: heads[i], width: t.widths[i]}
		}
	}
	t.head = widget.Row(heads...).Align(widget.AlignStretch)
	t.headBox = widget.Box(ggui.WithEnv(tableLabelText, t.head))
	t.body = widget.EachKeyed(rows, key, func(row widget.EachItem[T]) ggui.Widget { return t.row(row) }).Gap(0).Align(widget.AlignStretch).ItemExtent(t.rowH).Unlisted()
	t.offset = ggui.State(0.0)
	t.scroll = widget.Scroll(t.body).BindOffset(t.offset)
	t.compose()
	return t
}

// compose stacks the table's parts: the headings, the rows (scrolling under
// a Height or a maximum), then the footer where there is one. The toolbar
// and the caption are laid out apart, above and under the column.
func (t *TableWidget[T, K]) compose() {
	parts := []ggui.Widget{t.headBox, t.body}
	if t.scrolls() {
		parts[1] = widget.Expanded(t.scroll)
	}
	if t.footBox != nil {
		parts = append(parts, t.footBox)
	}
	t.column = widget.Column(parts...).Align(widget.AlignStretch)
}

// Footer adds a summary row under the rows, as shadcn's TableFooter: on a
// muted surface, above a line, in the medium weight. Each cell sits in the
// column of the same position and takes its width and alignment; pass nil
// to leave a column empty. The footer stays in place under a scrolling
// body. Calling it with no cells removes it.
//
//	ui.Table(...).Footer(widget.Text("Total"), nil, widget.Text("$2,500.00"))
func (t *TableWidget[T, K]) Footer(cells ...ggui.Widget) *TableWidget[T, K] {
	defer property.Watch(&t.props, &t.column)()
	t.foot, t.footBox = nil, nil
	if len(cells) > 0 {
		row := make([]ggui.Widget, len(t.cols))
		for i := range row {
			var w ggui.Widget = widget.Box()
			if i < len(cells) && cells[i] != nil {
				w = cells[i]
			}
			row[i] = t.cell(i, w, ggui.RoleCell)
		}
		t.foot = widget.Row(row...).Align(widget.AlignStretch)
		t.footBox = widget.Box(ggui.WithEnv(tableLabelText, t.foot))
	}
	t.compose()
	return t
}

// Caption sets a line of muted text centered under the table, as shadcn's
// TableCaption. An empty string removes it. A Height includes the caption.
func (t *TableWidget[T, K]) Caption(text string) *TableWidget[T, K] {
	switch {
	case text == "":
		if t.caption != nil {
			t.caption = nil
			t.props.Changed()
		}
	case t.caption != nil:
		t.caption.Content(text)
	default:
		t.caption = widget.Text(text).Align(.5)
		t.props.Changed()
	}
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

// RowHeight fixes every row's height, cells holding controls included,
// and the body then lays out only the rows in view where it scrolls, or
// inside a Scroll around the table. Without it a row is a line of text
// inside the cell padding, 37 under the stock themes: at least that and as
// tall as its tallest cell while the body does not scroll, and exactly
// that under Height, MaxHeight or MaxRows, where a cell's control is
// squeezed into it: there a default-size Button, 32 tall under the default
// theme, needs RowHeight(49) to keep its height in the 8 of cell padding
// and over the line.
func (t *TableWidget[T, K]) RowHeight(h float64) *TableWidget[T, K] {
	defer property.Watch(&t.props, &t.rowH)()
	h = max(1, h)
	t.rowH, t.rowSet = h, true
	t.body.ItemExtent(h)
	return t
}

// Height fixes the table's height, toolbar, footer and caption included,
// and scrolls the body under the fixed heading. Without it, or a
// MaxHeight or MaxRows, the table is as tall as its rows. It wins over
// MaxHeight and MaxRows; zero removes it.
func (t *TableWidget[T, K]) Height(h float64) *TableWidget[T, K] {
	return t.bound(func() { t.height = max(0, h) })
}

// MaxHeight lets the table be as tall as its rows up to h, toolbar, footer
// and caption included, and past that scrolls the body under the fixed
// heading. Height wins over it; zero removes it.
func (t *TableWidget[T, K]) MaxHeight(h float64) *TableWidget[T, K] {
	return t.bound(func() { t.maxH = max(0, h) })
}

// MaxRows is MaxHeight in rows: the table is as tall as its rows up to n of
// them under the heading, with the toolbar, footer and caption, and past
// that the body scrolls. The limit follows the row height at layout. With
// MaxHeight too the lower limit applies; Height wins over both. Zero
// removes it.
func (t *TableWidget[T, K]) MaxRows(n int) *TableWidget[T, K] {
	return t.bound(func() { t.maxRows = max(0, n) })
}

// bound runs set, which changes the height or a limit, and recomposes the
// table if that starts or stops the body scrolling.
func (t *TableWidget[T, K]) bound(set func()) *TableWidget[T, K] {
	defer property.Watch(&t.props, &t.column)()
	defer property.Watch(&t.props, &t.height)()
	defer property.Watch(&t.props, &t.maxH)()
	defer property.Watch(&t.props, &t.maxRows)()
	was := t.scrolls()
	set()
	if was != t.scrolls() {
		t.compose()
	}
	return t
}

// scrolls reports whether the body scrolls: under a Height or a maximum.
func (t *TableWidget[T, K]) scrolls() bool { return t.height > 0 || t.maxH > 0 || t.maxRows > 0 }

// fixedRows reports whether every row is rowH tall: under RowHeight or a
// scrolling body. Otherwise rows grow to their cells.
func (t *TableWidget[T, K]) fixedRows() bool { return t.rowSet || t.scrolls() }

// Toolbar puts w above the heading row, as wide as the table, in the
// cells' padding and over the line that parts the rows: a place for a
// filter field or actions. It stays in place while the body scrolls, and
// counts toward Height and MaxHeight. Nil removes it.
//
//	ui.Table(...).Toolbar(widget.Row(widget.Expanded(search), add).Gap(8))
func (t *TableWidget[T, K]) Toolbar(w ggui.Widget) *TableWidget[T, K] {
	switch {
	case w == nil && t.toolBox == nil:
		return t
	case w == nil:
		t.toolBox = nil
	default:
		t.toolBox = widget.Box(w)
	}
	t.props.Changed()
	return t
}

// RowName names each row for Probe.Find and the inspector; the default is
// the key.
func (t *TableWidget[T, K]) RowName(fn func(T) string) *TableWidget[T, K] { t.label = fn; return t }

func (t *TableWidget[T, K]) selectable() bool {
	return t.selected != nil || t.onSelect != nil || t.selection != nil
}

// target is the row a navigation key takes the keyboard to from row i of
// n, unclamped, and whether key is one.
func (t *TableWidget[T, K]) target(key ggui.KeyboardKey, i, n int) (int, bool) {
	if to, ok := navTarget(key, navVertical, i, n); ok {
		return to, true
	}
	switch key {
	case ggui.KeyPageUp:
		return i - t.page(), true
	case ggui.KeyPageDown:
		return i + t.page(), true
	}
	return 0, false
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
	i = clamp(i, 0, len(rows)-1)
	k := t.key(rows[i])
	t.nav.ask(k)
	switch {
	case t.selection != nil:
		t.selectAt(rows, i, extend, false)
	case t.selected != nil:
		t.selected.Set(k)
	}
	if t.scrolls() {
		revealRow(t.offset, i, t.rowH, t.bodyH)
	}
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
		cur := ggui.Untrack(t.selection.Get)
		next = withKey(cur, k, !cur[k])
		t.anchor = at(k)
	default:
		next[k] = true
		t.anchor = at(k)
	}
	t.selection.Set(next)
}

// page is how many rows a Page key moves.
func (t *TableWidget[T, K]) page() int {
	if !t.scrolls() {
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
	w = &tableCell{box: widget.Box(widget.Align(w).At(c.Align, .5)), role: role}
	if width := t.widths[i]; width != nil {
		return widget.Box(w).BindWidth(width)
	}
	if c.Width > 0 {
		return widget.Box(w).Width(c.Width)
	}
	return widget.Flex(w, pick(c.Flex > 0, c.Flex, 1))
}

func (t *TableWidget[T, K]) row(row widget.EachItem[T]) ggui.Widget {
	item := row.Value
	cells := make([]ggui.Widget, len(t.cols))
	for i, c := range t.cols {
		cells[i] = t.cell(i, c.Cell(item), ggui.RoleCell)
	}
	first := ggui.Untrack(item.Get)
	r := &tableRow[T, K]{table: t, item: item, index: row.Index, key: t.key(first), cells: widget.Row(cells...)}
	r.Role = ggui.RoleRow
	r.SetName(t.label(first))
	r.AutoKey()
	return r
}

// Layout implements Widget.
func (t *TableWidget[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout().EndLayout()
	th := uitheme.Of(env)
	t.theme = th
	t.head.Gap(0)
	if !t.rowSet {
		t.rowH = tableRowHeight(th)
	}
	// Rows of one height where it is fixed; otherwise each takes its own.
	t.body.ItemExtent(pick(t.fixedRows(), t.rowH, 0))
	// shadcn's h-10 heading row, and a footer row as tall as the others.
	t.headH, t.footH, t.capH = 5*th.Space, 0, 0
	t.headBox.Height(t.headH)
	if t.footBox != nil {
		t.foot.Gap(0)
		t.footH = t.rowH
		t.footBox.Height(t.footH)
		t.footBg = fade(th.Muted, .5)
	}
	if t.caption != nil {
		// shadcn's mt-4 above a line of muted text.
		t.capSize = t.caption.Color(th.MutedFg).Layout(ggui.Loose(ggui.Sz(c.MaxW, c.MaxH)), env)
		t.capH = t.capSize.H + 2*th.Space
	}
	t.toolH = 0
	if t.toolBox != nil {
		// The cells' p-2 around it, over the line under it.
		t.toolBox.Padding(ggui.Insets(th.Space, th.Space, th.Space+th.BorderWidth, th.Space))
		tc := ggui.Constraints{MaxW: c.MaxW, MaxH: ggui.Unbounded}
		if c.MaxW != ggui.Unbounded {
			tc.MinW = c.MaxW // a table with flexible columns is as wide as it may be
		}
		t.toolSz = t.toolBox.Layout(tc, env)
		t.toolH = t.toolSz.H
		c.MinW = max(c.MinW, min(t.toolSz.W, c.MaxW))
	}
	around := t.toolH + t.headH + t.footH + t.capH // what is not the body
	switch {
	case t.height > 0:
		h := min(t.height, c.MaxH)
		c.MinH, c.MaxH = h, h
	case t.scrolls():
		// As tall as the rows, every one rowH, up to the limits.
		h := around + float64(len(t.rows.Get()))*t.rowH
		if t.maxH > 0 {
			h = min(h, t.maxH)
		}
		if t.maxRows > 0 {
			h = min(h, around+float64(t.maxRows)*t.rowH)
		}
		h = min(h, c.MaxH)
		c.MinH, c.MaxH = h, h
	}
	apart := t.toolH + t.capH
	c.MinH, c.MaxH = max(0, c.MinH-apart), max(0, c.MaxH-apart)
	sz := t.column.Layout(c, env)
	t.bodyH = sz.H - t.headH - t.footH
	if t.toolBox != nil && t.toolSz.W != sz.W {
		t.toolSz = t.toolBox.Layout(ggui.Tight(ggui.Sz(sz.W, t.toolH)), env)
	}
	return ggui.Sz(sz.W, sz.H+apart)
}

// Paint implements Widget.
func (t *TableWidget[T, K]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	th := t.theme
	footer := ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+r.Size.H-t.capH-t.footH), ggui.Sz(r.Size.W, t.footH))
	if t.footH > 0 {
		dst.FillRect(footer, t.footBg)
	}
	top := r.Origin.Y + t.toolH // where the column starts
	dst.Node(r, ggui.Node{Role: ggui.RoleTable}, func(dst *ggui.Canvas) {
		if t.toolBox != nil {
			dst.Paint(t.toolBox, ggui.Rct(r.Origin, ggui.Sz(r.Size.W, t.toolH)))
		}
		dst.Paint(t.column, ggui.Rct(ggui.Pt(r.Origin.X, top), ggui.Sz(r.Size.W, r.Size.H-t.toolH-t.capH)))
		if t.caption != nil {
			at := ggui.Pt(r.Origin.X+(r.Size.W-t.capSize.W)/2, r.Origin.Y+r.Size.H-t.capSize.H)
			dst.Paint(t.caption, ggui.Rct(at, t.capSize))
		}
	})
	line := ggui.Sz(r.Size.W, th.BorderWidth)
	if t.toolBox != nil {
		dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, top-th.BorderWidth), line), th.Border)
	}
	dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, top+t.headH-th.BorderWidth), line), th.Border)
	if t.footH > 0 {
		dst.FillRect(ggui.Rct(footer.Origin, line), th.Border)
	}
}

// tableRow is one row: its cells in a Row, hover and selection.
type tableRow[T any, K comparable] struct {
	ggui.Interactive
	table  *TableWidget[T, K]
	item   ggui.Readable[T]
	key    K
	index  ggui.Readable[int]
	motion time.Duration
	cells  *widget.RowWidget
	box    *widget.BoxWidget
	theme  *uitheme.Theme
}

func (r *tableRow[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	th := uitheme.Of(env)
	r.theme = th
	r.Sync()
	r.SetName(r.table.label(r.item.Get()))
	r.motion = env.Motion(th.MotionFast)
	r.cells.Gap(0).Align(widget.AlignStretch)
	if r.box == nil {
		r.box = widget.Box(r.cells)
	}
	if r.table.fixedRows() {
		r.box.Height(r.table.rowH)
	} else {
		// At least a row of text, and as tall as the tallest cell: the
		// cells' Align would take all the height it is offered.
		r.box.Height(0)
		c.MinH, c.MaxH = r.table.rowH, ggui.Unbounded
	}
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
		if r.table.nav.claim(r.key) {
			dst.RequestFocus(r)
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
	amount := ease(dst, r.theme, r.Anchor(rc), tableRowFillSlot, target, r.motion)
	if amount > 0 {
		dst.FillRect(rc, fade(th.Muted, amount))
	}
	dst.Clip(rc).Paint(r.box, rc)
	if r.index.Get() < len(r.table.rows.Get())-1 {
		dst.FillRect(ggui.Rct(ggui.Pt(rc.Origin.X, rc.Origin.Y+rc.Size.H-th.BorderWidth), ggui.Sz(rc.Size.W, th.BorderWidth)), th.Border)
	}
	FocusRing(dst, &r.Interactive, Ring{Theme: th, Rect: rc, Radius: th.Radius, Color: th.Ring, Motion: r.motion})
}

// pick activates the row: it becomes the selected row of BindSelected and
// goes to OnSelect.
func (r *tableRow[T, K]) pick() {
	if r.IsInert() || !r.table.selectable() {
		return
	}
	r.table.nav.active = at(r.key)
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
		t.nav.active = at(r.key)
	}
	if ev.Kind == ggui.KeyPress {
		i, n := ggui.Untrack(r.index.Get), len(ggui.Untrack(t.rows.Get))
		if to, ok := t.target(ev.Key, i, n); ok {
			t.move(to, ev.Mods.Shift)
			return
		}
		switch ev.Key {
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
	if navMoveOf(ev.Key, navVertical) != navNone || ev.Key == ggui.KeyPageUp || ev.Key == ggui.KeyPageDown {
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
func (r *tableRow[T, K]) TabStop() bool {
	t := r.table
	var sel rowCursor[K]
	if t.selected != nil {
		sel = at(ggui.Untrack(t.selected.Get))
	}
	return t.nav.isStop(r.key, ggui.Untrack(t.rows.Get), t.key, sel)
}

// HandlePointer implements PointerHandler.
func (r *tableRow[T, K]) HandlePointer(ev ggui.PointerEvent) bool {
	if r.IsInert() {
		return false
	}
	if !r.table.selectable() {
		// It takes no press, but claims the hover, as its fill shows it: on
		// the hover path it hears the pointer leave, over a button in a cell
		// too, where a row that passed the move kept its fill for good.
		r.Pointer(ev, nil)
		switch ev.Kind {
		case ggui.PointerMove, ggui.PointerEnter, ggui.PointerExit:
			return true
		}
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
	box  *widget.BoxWidget
	role ggui.Role
}

func (c *tableCell) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	// shadcn's p-2; the row's height decides the vertical room.
	return c.box.Pad(uitheme.Of(env).Space).Layout(cs, env)
}
func (c *tableCell) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: c.role}, func(dst *ggui.Canvas) { dst.Clip(r).Paint(c.box, r) })
}
