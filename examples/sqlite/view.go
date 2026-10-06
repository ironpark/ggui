package main

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons/lucide"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func clientTheme(dark bool) uitheme.Theme {
	preset := uitheme.Preset{Base: uitheme.BaseNeutral, Accent: uitheme.AccentEmerald}
	t := preset.For(dark)
	t.Radius, t.RadiusSm, t.RadiusLg = 5, 3, 8
	t.Text.Size = 13
	t.Caption.Size = 12
	t.Title.Size = 19
	t.ButtonPad = ggui.Insets(5, 9)
	t.FieldPad = ggui.Insets(5, 9)
	if dark {
		t.Bg = color.NRGBA{R: 18, G: 19, B: 18, A: 255}
		t.Sidebar = color.NRGBA{R: 20, G: 21, B: 20, A: 255}
		t.Primary = color.NRGBA{R: 62, G: 207, B: 142, A: 255}
		t.PrimaryFg = color.NRGBA{R: 8, G: 40, B: 26, A: 255}
		t.Border = color.NRGBA{R: 40, G: 41, B: 40, A: 255}
		t.Muted = color.NRGBA{R: 35, G: 37, B: 35, A: 255}
	}
	return t
}

func build(m *model) ggui.Widget {
	return buildClient(m)
}

func buildClient(m *model) ggui.Widget {
	unavailable := ggui.Combine(m.Busy, m.Connected, func(b, c bool) bool { return b || !c })
	header := ggui.Padding(ggui.Row(
		ggui.Box(ggui.Text("S").Color(uitheme.PrimaryFg)).Fill(uitheme.Primary).Radius(4).Pad(4, 8),
		ggui.Text("SQLite Studio"),
		ggui.Expanded(ggui.If(m.Connected, func() ggui.Widget {
			return ggui.Row(ui.Caption("/"), ggui.TextOf(m.Path.Map(filepath.Base)).NoWrap()).Gap(10)
		})),
		ggui.If(ggui.Combine(m.Connected, m.Locked, func(connected, locked bool) bool { return connected && locked }), func() ggui.Widget {
			return ui.Badge("READ ONLY")
		}),
		ggui.If(m.Connected, func() ggui.Widget {
			return ui.Button("Open database", m.choose).Outline().BindDisabled(m.Busy)
		}),
		ui.ThemeSwitch(m.Dark),
	).Gap(10).Align(ggui.AlignCenter), 7, 12)

	tabs := ui.Tabs(m.Tab,
		ui.Tab("Data", dataView(m, unavailable)),
		ui.Tab("Structure", ggui.Padding(ggui.Scroll(schemaView(m)), 12)),
		ui.Tab("SQL", ggui.Padding(sqlView(m, unavailable), 12)),
	).Line().OnChange(func(tab int) {
		if tab == 0 && m.db != nil && !m.Busy.Get() && m.Table.Get() != "" {
			m.browse()
		}
	})
	workspace := ggui.Row(sidebarView(m), ui.Divider().Vertical(),
		ggui.Expanded(ggui.Column(
			ggui.Expanded(ggui.Padding(tabs, 0, 0, 8, 0)),
		).Stretch()),
	).Stretch()
	content := ggui.If(m.Connected, func() ggui.Widget { return workspace }).Else(func() ggui.Widget { return welcomeView(m) })
	body := ggui.Column(
		header,
		ui.Divider(),
		ggui.Expanded(content),
		errorBanner(m),
		ui.Divider(),
		ggui.Padding(statusBar(m), 8, 16),
	).Stretch()
	confirmDelete := ui.AlertDialog(m.ConfirmDelete, "Delete selected row?",
		"This permanently removes the selected row. This action cannot be undone here.").
		Confirm("Delete", m.delete).Destructive()
	window := ggui.Column(ggui.Expanded(body), historyDialog(m), cellEditor(m), rowEditor(m), confirmDelete).Stretch()
	return ggui.Pointer(window).OnDrop(m.acceptDrop).OnDropHover(m.DropHover.Set)
}

// errorBanner shows the last operation's error until it is dismissed.
func errorBanner(m *model) ggui.Widget {
	return ggui.View(m.Error, func(message string) ggui.Widget {
		if message == "" {
			return ggui.Column()
		}
		alert := ui.Alert("Operation failed", message).Destructive()
		dismiss := ui.Button("Dismiss", func() { m.Error.Set("") }).Ghost()
		return ggui.Padding(ggui.Row(ggui.Expanded(alert), dismiss).Gap(8), 8, 16)
	})
}

// statusBar reports what the client is doing, and offers to cancel it.
func statusBar(m *model) ggui.Widget {
	action := ggui.If(m.Busy, func() ggui.Widget {
		return ui.Button("Cancel operation", m.stop).Ghost()
	}).ElseIf(m.DropHover, func() ggui.Widget {
		return ui.Caption("Release to open the database").Color(uitheme.Primary)
	}).Else(func() ggui.Widget {
		return ui.Caption("Drop a database anywhere  ·  ⌘/Ctrl O to open")
	})
	return ggui.Row(
		ggui.Text("●").Color(uitheme.Primary),
		ggui.Expanded(ui.CaptionOf(m.Status).NoWrap()),
		action,
	).Gap(8).Align(ggui.AlignCenter)
}

func sidebarView(m *model) ggui.Widget {
	list := ggui.View(ggui.Combine(m.Objects, m.Search, func(objects []object, search string) []object {
		out := []object{}
		for _, o := range objects {
			if strings.Contains(strings.ToLower(o.Name), strings.ToLower(search)) {
				out = append(out, o)
			}
		}
		return out
	}), func(objects []object) ggui.Widget {
		if len(objects) == 0 {
			return ggui.Padding(ui.Caption("No matching tables or views."), 12, 4)
		}
		rows := []ggui.Widget{}
		for _, kind := range []string{"table", "view"} {
			mark, title := "▦", "TABLES"
			if kind == "view" {
				mark, title = "◇", "VIEWS"
			}
			group := []ggui.Widget{}
			for _, o := range objects {
				if o.Kind != kind {
					continue
				}
				name := o.Name
				label := ggui.Row(ggui.Text(mark).Color(uitheme.MutedFg), ggui.Expanded(ggui.Text(name).NoWrap())).Gap(10)
				// Only the rows whose selection changes rebuild.
				selected := m.Table.Map(func(s string) bool { return s == name })
				group = append(group, ggui.View(selected, func(selected bool) ggui.Widget {
					b := ui.ButtonOf(label, func() { m.selectTable(name) }).Name(name).BindDisabled(m.Busy).Pad(6, 8)
					if selected {
						return ggui.Box(ggui.Row(ggui.Box().Width(2).Fill(uitheme.Primary), ggui.Expanded(b.Secondary())).Stretch())
					}
					return b.Ghost()
				}))
			}
			if len(group) > 0 {
				rows = append(rows, ggui.Padding(ui.Caption(fmt.Sprintf("%s  %d", title, len(group))), 12, 10, 6, 10))
				rows = append(rows, group...)
			}
		}
		return ggui.Column(rows...).Gap(2).Stretch()
	})
	return ggui.Box(ggui.Column(
		ggui.Row(ggui.Text("Table Editor"), ggui.Spacer(), ui.Badge("main")).Gap(8),
		ui.TextField(m.Search).Name("Find table").Placeholder("Search tables and views…"),
		ggui.Expanded(ggui.Scroll(list)), ui.Divider(),
		ui.Checkbox(m.ReadOnly, "Open read-only"), ui.Caption("Applies to the next database."),
		ui.Caption(".db · .sqlite · .sqlite3"),
	).Gap(12).Stretch()).Width(240).Pad(12, 12).Fill(uitheme.Sidebar)
}

func welcomeView(m *model) ggui.Widget {
	return ggui.Center(ggui.Padding(ggui.Box(ggui.Column(
		ggui.Column(
			ui.Title("Open database"),
			ui.Caption("Choose a SQLite file to get started."),
		).Gap(8).Align(ggui.AlignStart),
		ggui.View(m.DropHover, func(over bool) ggui.Widget {
			border, fill, title := uitheme.Border, uitheme.Sidebar, "Drop your database here"
			if over {
				border, fill, title = uitheme.Primary, uitheme.Accent, "Release to open"
			}
			return ggui.Box(ggui.Row(ggui.Column(
				lucide.Icon("file").Size(28).Color(uitheme.MutedFg),
				ggui.Text(title),
				ui.Caption(".db · .sqlite · .sqlite3"),
				ui.Button("Open database", m.choose).BindDisabled(m.Busy),
			).Gap(16).Align(ggui.AlignCenter)).Justify(ggui.JustifyCenter)).Pad(52, 32).Border(1, border).Radius(8).Fill(fill)
		}),
		ui.Checkbox(m.ReadOnly, "Open read-only").BindDisabled(m.Busy),
	).Gap(24).Stretch()).Width(520), 24))
}

func dataView(m *model, unavailable ggui.Readable[bool]) ggui.Widget {
	loaded := m.Loaded.Map(func(s string) bool { return s != "" })
	grid := ggui.If(loaded, func() ggui.Widget { return resultGrid(m.Data, m) }).Else(func() ggui.Widget {
		return ggui.Center(ui.Empty("No table selected", "Choose a table, or use SQL to create your first one."))
	})
	return ggui.Column(
		ggui.Padding(dataToolbar(m, unavailable), 8, 10),
		ggui.If(m.FilterOpen, func() ggui.Widget { return sortBar(m, unavailable) }),
		selectionBar(m),
		ggui.Expanded(grid),
		ui.Divider(),
		ggui.Padding(pager(m, unavailable), 6, 10),
	).Gap(0).Stretch()
}

// dataToolbar filters, sorts, refreshes and adds to the table shown.
func dataToolbar(m *model, unavailable ggui.Readable[bool]) ggui.Widget {
	filter := ui.TextField(m.Filter).Name("Filter rows").Placeholder("Filter rows…").
		OnSubmit(func(string) { m.apply() }).BindDisabled(unavailable)
	clearFilters := func() {
		m.Filter.Set("")
		m.Sort.Set("")
		m.Desc.Set(false)
		m.apply()
	}
	more := ui.Menu("More",
		ui.MenuItem("Clear filters", clearFilters),
		ui.MenuItem("Export CSV", func() { m.export(false) }),
		ui.MenuItem("Query table", m.queryTable),
	).BindDisabled(unavailable)
	return ggui.Row(
		ggui.Expanded(filter),
		ui.Button("Apply", m.apply).Ghost().BindDisabled(unavailable),
		ui.Button("Sort", func() { ggui.Toggle(m.FilterOpen) }).Outline().BindDisabled(unavailable),
		ui.Button("↻", m.browse).Name("Refresh").Outline().BindDisabled(unavailable),
		more,
		ui.Button("Add row", m.add).BindDisabled(ggui.Or(ggui.Not(m.Insertable), m.Busy)),
	).Gap(8).Align(ggui.AlignCenter)
}

// sortBar picks the column the rows are sorted by.
func sortBar(m *model, unavailable ggui.Readable[bool]) ggui.Widget {
	columns := m.Data.Map(func(r result) []string { return append([]string{""}, r.Columns...) })
	column := ui.Select(m.Sort).BindOptions(columns).Name("Sort column").Format(func(name string) string {
		if name == "" {
			return "Default order"
		}
		return name
	}).BindDisabled(unavailable)
	return ggui.Row(
		ui.Caption("Sort by"),
		ggui.Expanded(column),
		ui.Checkbox(m.Desc, "Descending"),
		ui.Button("Sort", m.apply).Outline().BindDisabled(unavailable),
	).Gap(10)
}

// selectionBar acts on the selected rows, while there are any.
func selectionBar(m *model) ggui.Widget {
	count := ggui.Derived(m.selectionCount)
	notOne := count.Map(func(n int) bool { return n != 1 })
	cannotEdit := ggui.Combine(m.Selected, m.Editable, func(row int, editable bool) bool { return row == 0 || !editable })
	return ggui.If(count.Map(func(n int) bool { return n > 0 }), func() ggui.Widget {
		return ggui.Row(
			ui.Captionf("%d rows selected", count),
			ui.Button("View / edit cell", m.edit).Outline().BindDisabled(ggui.Or(m.Busy, notOne)),
			ui.Button("Delete row", m.askDelete).Destructive().BindDisabled(ggui.Or(cannotEdit, m.Busy)),
			ggui.Spacer(),
			ui.Button("Deselect", m.clearSelection).Ghost(),
		).Gap(8)
	})
}

// pager says which rows are shown and moves between pages.
func pager(m *model, unavailable ggui.Readable[bool]) ggui.Widget {
	shown := ggui.Derived(func() string {
		n := len(m.Data.Get().Rows)
		if n == 0 {
			return "0 rows"
		}
		return fmt.Sprintf("%d–%d of %d rows", m.Offset.Get()+1, m.Offset.Get()+n, m.Total.Get())
	})
	pageSize := ui.Select(ggui.Bind(m.PageSize.Get, m.setPageSize)).
		Options([]int{25, 50, 100, 250, 500}).Name("Rows per page").BindDisabled(unavailable)
	return ggui.Wrap(
		ui.CaptionOf(shown),
		ggui.Row(ui.Caption("Rows per page"), ggui.Box(pageSize).Width(76)).Gap(8).Align(ggui.AlignCenter),
		ui.Pagination(ggui.Bind(m.currentPage, m.setPage), ggui.Derived(m.pageCount)).BindDisabled(unavailable),
	).Gap(12).Align(ggui.AlignCenter)
}

func schemaView(m *model) ggui.Widget {
	count := m.Data.Map(func(r result) string { return fmt.Sprintf("%d columns", len(r.Columns)) })
	headings := ggui.Row(
		ggui.Box(ui.Caption("COLUMN")).Width(180),
		ggui.Box(ui.Caption("TYPE")).Width(110),
		ui.Caption("CONSTRAINTS / DEFAULT"),
	).Gap(12)
	columns := ggui.View(m.Data, func(result) ggui.Widget {
		rows := []ggui.Widget{}
		for _, c := range m.current.Columns {
			rows = append(rows, columnRow(c), ui.Divider())
		}
		return ggui.Column(rows...).Gap(12).Stretch()
	})
	return ggui.Column(
		ggui.Padding(ggui.Row(ui.Title("Table structure"), ggui.Spacer(), ui.CaptionOf(count)), 10, 0),
		headings,
		ui.Divider(),
		columns,
		ui.Caption("CREATE STATEMENT"),
		ui.Card(ggui.TextOf(m.Schema)).Pad(18),
	).Gap(16).Stretch()
}

// columnRow is one column of the table's structure.
func columnRow(c column) ggui.Widget {
	flags := []string{}
	if c.PK > 0 {
		flags = append(flags, "PRIMARY KEY")
	}
	if c.NotNull {
		flags = append(flags, "NOT NULL")
	}
	if c.Hidden != 0 {
		flags = append(flags, "GENERATED / HIDDEN")
	}
	if c.Default.Valid {
		flags = append(flags, "DEFAULT "+c.Default.String)
	}
	if len(flags) == 0 {
		flags = append(flags, "Nullable")
	}
	return ggui.Row(
		ggui.Box(ggui.Text(c.Name)).Width(180),
		ggui.Box(ui.Caption(c.Type)).Width(110),
		ggui.Expanded(ui.Caption(strings.Join(flags, " · "))),
	).Gap(12)
}

func sqlView(m *model, unavailable ggui.Readable[bool]) ggui.Widget {
	header := ggui.Row(
		ui.Caption("SQL EDITOR"),
		ggui.Spacer(),
		ui.Button("Recent queries", func() { m.HistoryOpen.Set(true) }).Ghost(),
		ui.Caption("⌘/Ctrl + Enter"),
	).Gap(10)
	run := ggui.Row(
		ui.Button("Run SQL", m.execute).BindDisabled(unavailable),
		ui.Button("Cancel query", m.stop).Ghost().BindDisabled(ggui.Not(m.Busy)),
		ggui.Spacer(),
		ui.Caption("One statement · Auto-commit on success"),
	).Gap(8)
	results := ggui.Row(
		ggui.Text("Results"),
		ggui.Expanded(ui.CaptionOf(m.QueryStatus)),
		ui.Button("Export loaded results", func() { m.export(true) }).Outline().BindDisabled(unavailable),
	).Gap(12)
	return ggui.Column(
		header,
		ui.TextField(m.SQL).Name("SQL").Multiline().Lines(7).BindDisabled(m.Busy),
		run,
		ui.Divider(),
		results,
		ggui.Expanded(ggui.Scroll(queryResultTable(m.QueryData))),
	).Gap(12).Stretch()
}

func historyDialog(m *model) ggui.Widget {
	return ui.Dialog(m.HistoryOpen, ggui.Box(ggui.Scroll(ggui.View(m.History, func(history []string) ggui.Widget {
		if len(history) == 0 {
			return ui.Empty("No queries yet", "Your last 20 successful statements appear here.")
		}
		rows := []ggui.Widget{}
		for _, text := range history {
			label := truncate(strings.Join(strings.Fields(text), " "), 90)
			rows = append(rows, ui.Button(label, func() { m.SQL.Set(text); m.HistoryOpen.Set(false); m.Tab.Set(2) }).Ghost().BindDisabled(m.Busy))
		}
		return ggui.Column(rows...).Gap(8).Stretch()
	}))).Height(360)).Title("Recent queries").Width(700)
}

func resultGrid(source ggui.Readable[result], m *model) ggui.Widget {
	return ggui.View(source, func(r result) ggui.Widget {
		if len(r.Columns) == 0 {
			return ggui.Center(ui.Empty("Ready when you are", "Run a query to see its results here."))
		}
		cols := make([]ui.Column[record], len(r.Columns))
		total := 0.0
		sort := m.tableSort()
		for i, name := range r.Columns {
			width := max(128.0, min(280.0, float64(utf8.RuneCountInString(name)*8+80)))
			for _, row := range r.Rows[:min(30, len(r.Rows))] {
				width = max(width, min(280, float64(utf8.RuneCountInString(cellText(row.Values[i]))*7+28)))
			}
			total += width
			cols[i] = resultColumn(i, name).Grow(width)
			if numericColumn(r, i) {
				cols[i] = cols[i].Right()
			}
			meta, _ := m.current.column(name)
			heading := ggui.TextOf(ggui.Derived(func() string { return name + sort.Get().Mark(name) })).NoWrap()
			label := ggui.Row(heading, ui.Caption(strings.ToLower(meta.Type))).Gap(8).Align(ggui.AlignCenter)
			if meta.PK > 0 {
				label = ggui.Row(ggui.Text("◆").Color(uitheme.Primary), label).Gap(6)
			}
			cols[i].Header = ui.ButtonOf(label, func() { m.sortColumn(name) }).Name("Sort by "+name).Ghost().Pad(0, 0).BindDisabled(m.Busy)
		}
		selectRow := ui.Col("", func(row ggui.Readable[record]) ggui.Widget {
			id := ggui.Peek(row).ID
			on := ggui.Bind(func() bool { return m.Selections.Get()[id] }, func(on bool) { m.selectRow(id, on) })
			return ui.Checkbox(on, "").Name(fmt.Sprintf("Select row %d", id)).BindDisabled(m.Busy)
		}).W(32)
		cannotSelect := ggui.Derived(func() bool { return m.Busy.Get() || len(m.Data.Get().Rows) == 0 })
		some := ggui.Derived(func() bool { n := len(m.Selections.Get()); return n > 0 && n < len(m.Data.Get().Rows) })
		selectRow.Header = ui.Checkbox(ggui.Bind(m.pageSelected, m.selectPage), "").Name("Select page").
			BindDisabled(cannotSelect).BindIndeterminate(some)
		cols = append([]ui.Column[record]{selectRow}, cols...)
		table := ui.Table(ggui.State(r.Rows), func(row record) int { return row.ID }, cols...).
			BindSelectedRows(m.Selections).
			OnSelect(func(row record) { m.selectRow(row.ID, !m.Selections.Get()[row.ID]) }).
			RowHeight(34)
		border := uitheme.Border
		grid := ggui.FromFuncs(table.Layout, func(dst *ggui.Canvas, rc ggui.Rect) {
			dst.Paint(table, rc)
			x := rc.Origin.X + 32
			dst.FillRect(ggui.Rct(ggui.Pt(x, rc.Origin.Y), ggui.Sz(1, rc.Size.H)), border)
			for _, c := range cols[1 : len(cols)-1] {
				x += (rc.Size.W - 32) * c.Flex / total
				dst.FillRect(ggui.Rct(ggui.Pt(x, rc.Origin.Y), ggui.Sz(1, rc.Size.H)), border)
			}
		})
		box := ggui.Box(grid)
		empty := ggui.Center(ui.Empty("No matching rows", "Clear the filter or choose another table."))
		scroll := ggui.Scroll(box).Horizontal()
		// Size the viewport from the workspace, keeping the column headings and
		// pagination visible while only the table body scrolls vertically.
		return ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
			table.Height(max(80, c.MaxH))
			if len(r.Rows) == 0 {
				empty.Layout(ggui.Loose(ggui.Sz(c.MaxW, max(0, c.MaxH-40))), env)
			}
			box.Width(max(total+32, c.MaxW))
			return scroll.Layout(c, env)
		}, func(dst *ggui.Canvas, rc ggui.Rect) {
			dst.Paint(scroll, rc)
			if len(r.Rows) == 0 {
				dst.Clip(rc).Paint(empty, ggui.Rct(ggui.Pt(rc.Origin.X, rc.Origin.Y+40), ggui.Sz(rc.Size.W, max(0, rc.Size.H-40))))
			}
		})
	})
}

// cellText is a value as a grid cell shows it: on one line, cut to what a
// cell can show. A blob is summarized rather than hex-encoded whole, since
// only its first bytes would be visible.
func cellText(v any) string {
	switch b := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		head := display(b[:min(len(b), 24)])
		if len(b) > 24 {
			head += "…"
		}
		return fmt.Sprintf("BLOB · %d bytes · %s", len(b), head)
	}
	return truncate(strings.ReplaceAll(display(v), "\n", " ↵ "), 120)
}

// truncate cuts s to n runes, marking the cut.
func truncate(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i] + "…"
		}
		n--
	}
	return s
}

func cellEditor(m *model) ggui.Widget {
	columns := m.Data.Map(func(result) []string {
		out := []string{}
		for _, c := range m.current.Columns {
			if c.Hidden != 1 {
				out = append(out, c.Name)
			}
		}
		return out
	})
	column := ui.Select(m.Cell).BindOptions(columns).OnChange(func(string) { m.loadCell() }).BindDisabled(m.Busy)
	kind := ui.Select(m.Kind).Options([]string{"TEXT", "INTEGER", "REAL", "BLOB", "NULL"}).BindDisabled(m.Busy)
	isNull := m.Kind.Map(func(kind string) bool { return kind == "NULL" })
	value := ui.TextField(m.Value).Multiline().Lines(5).BindDisabled(ggui.Or(isNull, m.Busy))
	readOnly := ggui.Combine(m.Cell, m.Editable, func(name string, editable bool) bool {
		if !editable {
			return true
		}
		c, ok := m.current.column(name)
		return !ok || c.Hidden != 0
	})
	actions := ggui.Row(
		ui.Button("Cancel", func() { m.Editing.Set(false) }).Outline().BindDisabled(m.Busy),
		ui.Button("Save cell", m.save).BindDisabled(ggui.Or(readOnly, m.Busy)),
	).Gap(8).Justify(ggui.JustifyEnd)
	return ui.Dialog(m.Editing, ggui.Column(
		ui.Field("Column", column),
		ui.Field("Storage type", kind),
		ui.Field("Value", value).Help("BLOB values use hexadecimal. NULL is different from empty text."),
		ui.CaptionOf(m.Error),
		actions,
	).Gap(14).Stretch()).Title("Cell details").Width(600)
}

func rowEditor(m *model) ggui.Widget {
	fields := ggui.View(m.Draft, func(fields []draftField) ggui.Widget {
		rows := []ggui.Widget{}
		for _, field := range fields {
			rows = append(rows, draftFieldRow(m, field))
		}
		return ggui.Column(rows...).Gap(16).Stretch()
	})
	actions := ggui.Row(
		ui.Button("Cancel insert", func() { m.Adding.Set(false) }).Outline().BindDisabled(m.Busy),
		ui.Button("Insert row", m.insert).BindDisabled(m.Busy),
	).Gap(8).Justify(ggui.JustifyEnd)
	return ui.Dialog(m.Adding, ggui.Column(
		ui.Caption("DEFAULT omits the column so SQLite supplies its default or generated ID."),
		ggui.Box(ggui.Scroll(fields)).Height(360),
		ui.CaptionOf(m.Error),
		actions,
	).Gap(14).Stretch()).Title("Add row").Width(720)
}

// draftFieldRow edits one column of the row being added.
func draftFieldRow(m *model, field draftField) ggui.Widget {
	kind := ui.Select(field.Kind).Options([]string{"DEFAULT", "TEXT", "INTEGER", "REAL", "BLOB", "NULL"}).BindDisabled(m.Busy)
	noValue := ggui.Combine(field.Kind, m.Busy, func(kind string, busy bool) bool {
		return busy || kind == "DEFAULT" || kind == "NULL"
	})
	return ggui.Column(
		ggui.Row(ggui.Text(field.Name), ui.Caption(field.Detail)).Gap(12),
		ggui.Row(ggui.Box(kind).Width(140), ggui.Expanded(ui.TextField(field.Value).BindDisabled(noValue))).Gap(10),
	).Gap(6)
}
