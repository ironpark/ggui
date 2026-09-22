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
	t := preset.Light()
	if dark {
		t = preset.Dark()
	}
	t.Radius, t.RadiusSm, t.RadiusLg = 5, 3, 8
	t.Text.Size = 13
	t.Caption.Size = 12
	t.Title.Size = 19
	t.ButtonPad = ggui.Insets(7, 11)
	t.FieldPad = ggui.Insets(7, 10)
	if dark {
		t.Bg = color.NRGBA{R: 23, G: 23, B: 23, A: 255}
		t.Sidebar = color.NRGBA{R: 28, G: 28, B: 28, A: 255}
		t.Primary = color.NRGBA{R: 62, G: 207, B: 142, A: 255}
		t.PrimaryFg = color.NRGBA{R: 8, G: 40, B: 26, A: 255}
	}
	return t
}

func build(m *model) ggui.Widget {
	return ggui.Reactive(func() ggui.Widget { return buildClient(m) })
}

func buildClient(m *model) ggui.Widget {
	t := uitheme.Use()
	unavailable := ggui.Combine(m.Busy, m.Connected, func(b, c bool) bool { return b || !c })
	header := ggui.Padding(ggui.Row(
		ggui.Box(ggui.Text("S").Color(t.PrimaryFg)).Fill(t.Primary).Radius(6).Pad(7, 12),
		ggui.Text("SQLite Studio"), ui.Caption("/"),
		ggui.Expanded(ggui.TextOf(m.Path.Map(func(p string) string {
			if p == "" {
				return "Local workspace"
			}
			return filepath.Base(p)
		})).NoWrap()),
		ggui.If(m.Connected, func() ggui.Widget {
			return ggui.View(m.Locked, func(ro bool) ggui.Widget {
				if ro {
					return ui.Badge("READ ONLY")
				}
				return ui.Badge("LOCAL DATABASE")
			})
		}),
		ui.Button("Open database", m.choose).Outline().BindDisabled(m.Busy),
		ui.Button("Open path", func() { m.OpenDialog.Set(true) }).Ghost().BindDisabled(m.Busy),
		ui.ThemeSwitch(m.Dark),
	).Gap(12).Align(ggui.AlignCenter), 12, 18)

	tabs := ui.Tabs(m.Tab,
		ui.Tab("Data", dataView(m, unavailable)),
		ui.Tab("Structure", ggui.Scroll(schemaView(m))),
		ui.Tab("SQL", sqlView(m, unavailable)),
	).Line().OnChange(func(tab int) {
		if tab == 0 && m.db != nil && !m.Busy.Get() && m.Table.Get() != "" {
			m.browse()
		}
	})
	workspace := ggui.Row(sidebarView(m), ggui.Box().Width(1).Fill(t.Border),
		ggui.Expanded(ggui.Column(
			ggui.Padding(ggui.Row(ui.Caption("DATABASE"), ui.Caption("/"), ggui.TextOf(m.Loaded.Map(func(name string) string {
				if name == "" {
					return "Workspace"
				}
				return name
			})).NoWrap(), ggui.Spacer(), ui.Button("Query table", m.queryTable).Ghost().BindDisabled(ggui.Combine(m.Loaded, m.Busy, func(s string, b bool) bool { return s == "" || b }))).Gap(10), 14, 18),
			ui.Divider(), ggui.Expanded(ggui.Padding(tabs, 8, 16, 12, 16)),
		).Align(ggui.AlignStretch)),
	).Align(ggui.AlignStretch)
	content := ggui.If(m.Connected, func() ggui.Widget { return workspace }).Else(func() ggui.Widget { return welcomeView(m) })
	body := ggui.Column(header, ui.Divider(), ggui.Expanded(content),
		ggui.View(m.Error, func(message string) ggui.Widget {
			if message == "" {
				return ggui.Column()
			}
			return ggui.Padding(ggui.Row(ggui.Expanded(ui.Alert("Operation failed", message).Destructive()), ui.Button("Dismiss", func() { m.Error.Set("") }).Ghost()).Gap(8), 8, 16)
		}),
		ui.Divider(), ggui.Padding(ggui.Row(
			ggui.Text("●").Color(t.Primary), ggui.Expanded(ggui.TextOf(m.Status).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption).NoWrap()),
			ggui.If(m.Busy, func() ggui.Widget { return ui.Button("Cancel operation", m.stop).Ghost() }).
				ElseIf(m.DropHover, func() ggui.Widget {
					return ggui.Text("Release to open the database").Color(t.Primary).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption)
				}).
				Else(func() ggui.Widget { return ui.Caption("Drop a database anywhere  ·  ⌘/Ctrl O to open") }),
		).Gap(8).Align(ggui.AlignCenter), 8, 16),
	).Align(ggui.AlignStretch)
	return ggui.Pointer(ggui.Column(ggui.Expanded(body), openDialog(m), historyDialog(m), cellEditor(m), rowEditor(m),
		ui.AlertDialog(m.ConfirmDelete, "Delete selected row?", "This permanently removes the selected row. This action cannot be undone here.").Confirm("Delete", m.delete).Destructive(),
	).Align(ggui.AlignStretch)).OnDrop(m.acceptDrop).OnDropHover(m.DropHover.Set)
}

func sidebarView(m *model) ggui.Widget {
	t := uitheme.Use()
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
				label := ggui.Row(ggui.Text(mark).Color(t.MutedFg), ggui.Expanded(ggui.Text(name).NoWrap())).Gap(10)
				// Only the rows whose selection changes rebuild.
				selected := m.Table.Map(func(s string) bool { return s == name })
				group = append(group, ggui.View(selected, func(selected bool) ggui.Widget {
					b := ui.ButtonOf(label, func() { m.selectTable(name) }).Name(name).BindDisabled(m.Busy).Pad(9, 10)
					if selected {
						return b.Secondary()
					}
					return b.Ghost()
				}))
			}
			if len(group) > 0 {
				rows = append(rows, ggui.Padding(ui.Caption(fmt.Sprintf("%s  %d", title, len(group))), 12, 10, 6, 10))
				rows = append(rows, group...)
			}
		}
		return ggui.Column(rows...).Gap(2).Align(ggui.AlignStretch)
	})
	return ggui.Box(ggui.Column(
		ggui.Row(ggui.Text("Table editor"), ggui.Spacer(), ui.Badge("main")).Gap(8),
		ui.TextField(m.Search).Name("Find table").Placeholder("Search tables and views…"),
		ggui.Expanded(ggui.Scroll(list)), ui.Divider(),
		ui.Checkbox(m.ReadOnly, "Open read-only"), ui.Caption("Applies to the next database."),
		ui.Caption(".db · .sqlite · .sqlite3"),
	).Gap(12).Align(ggui.AlignStretch)).Width(226).Pad(18, 12).Fill(t.Sidebar)
}

func welcomeView(m *model) ggui.Widget {
	t := uitheme.Use()
	return ggui.Center(ggui.Box(ggui.Column(
		ggui.Row(ui.Badge("LOCAL FIRST"), ui.Badge("SQLITE")).Gap(8).Justify(ggui.JustifyCenter),
		ggui.Text("Your database. A clearer view.").StyleKey(uitheme.TitleKey, uitheme.Default().Title).Role(ggui.RoleHeading),
		ui.Caption("Browse records, inspect your schema, and write SQL in one workspace."),
		ggui.View(m.DropHover, func(over bool) ggui.Widget {
			// The zone is the whole window; the box lights up to say so.
			border, fill, title := t.Border, t.Sidebar, "Drop a SQLite database here"
			if over {
				border, fill, title = t.Primary, t.Accent, "Release to open it"
			}
			return ggui.Box(ggui.Column(
				lucide.Icon("download").Size(30).Color(t.Primary),
				ui.Title(title),
				ui.Caption("Open the original file directly from your computer."),
				ui.Button("Choose database…", m.choose).BindDisabled(m.Busy),
				ui.Caption(".db, .sqlite, .sqlite3 or any valid SQLite file"),
			).Gap(16).Align(ggui.AlignCenter)).Pad(32).Border(1, border).Radius(8).Fill(fill)
		}),
		ui.Checkbox(m.ReadOnly, "Open read-only"),
		ui.Caption("Local files · No account needed · Changes save directly to your database"),
	).Gap(22).Align(ggui.AlignCenter)).Width(650).Pad(24))
}

func dataView(m *model, unavailable ggui.Readable[bool]) ggui.Widget {
	insertDisabled := ggui.Combine(m.Insertable, m.Busy, func(a, b bool) bool { return !a || b })
	editDisabled := ggui.Combine(ggui.Combine(m.Selected, m.Editable, func(row int, editable bool) bool { return row == 0 || !editable }), m.Busy, func(a, b bool) bool { return a || b })
	filters := ggui.If(m.FilterOpen, func() ggui.Widget {
		return ggui.Column(
			ggui.Row(ggui.Expanded(ui.TextField(m.Filter).Name("Filter rows").Placeholder("Find text in any column…").OnSubmit(func(string) { m.apply() }).BindDisabled(unavailable)), ui.Button("Apply", m.apply).Outline().BindDisabled(unavailable), ui.Button("Clear", func() { m.Filter.Set(""); m.Sort.Set(""); m.Desc.Set(false); m.apply() }).Ghost().BindDisabled(unavailable)).Gap(8),
			ggui.Row(ui.Caption("Sort by"), ggui.Expanded(ui.Select(m.Sort).BindOptions(m.Data.Map(func(r result) []string { return append([]string{""}, r.Columns...) })).Name("Sort column").Format(func(name string) string {
				if name == "" {
					return "Default order"
				}
				return name
			}).BindDisabled(unavailable)), ui.Checkbox(m.Desc, "Descending"), ui.Button("Sort", m.apply).Outline().BindDisabled(unavailable)).Gap(10),
		).Gap(8).Align(ggui.AlignStretch)
	})
	grid := ggui.If(m.Loaded.Map(func(s string) bool { return s != "" }), func() ggui.Widget { return resultGrid(m.Data, m.Selected) }).Else(func() ggui.Widget {
		return ggui.Center(ui.Empty("No table selected", "Choose a table, or use SQL to create your first one."))
	})
	return ggui.Column(
		ggui.Row(ui.Button("Filter / Sort", func() { m.FilterOpen.Set(!m.FilterOpen.Get()) }).Outline().BindDisabled(unavailable), ui.Button("Refresh", m.browse).Ghost().BindDisabled(unavailable),
			ggui.If(m.Filter.Map(func(s string) bool { return s != "" }), func() ggui.Widget { return ui.Badge("FILTERED") }),
			ggui.Spacer(), ui.Button("Export CSV", func() { m.export(false) }).Outline().BindDisabled(unavailable), ui.Button("Add row", m.add).BindDisabled(insertDisabled),
		).Gap(8), filters,
		ggui.If(m.Selected.Map(func(n int) bool { return n > 0 }), func() ggui.Widget {
			return ggui.Row(
				ggui.TextOf(m.Selected.Map(func(n int) string { return fmt.Sprintf("Row %d selected", n) })).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption),
				ui.Button("View / edit cell", m.edit).Outline().BindDisabled(m.Busy), ui.Button("Delete row", m.askDelete).Destructive().BindDisabled(editDisabled), ggui.Spacer(), ui.Button("Deselect", func() { m.Selected.Set(0) }).Ghost(),
			).Gap(8)
		}),
		ggui.Expanded(grid), ui.Divider(),
		ggui.Row(ggui.TextOf(ggui.Combine(m.Offset, m.Data, func(offset int, r result) string {
			if len(r.Rows) == 0 {
				return "No rows"
			}
			return fmt.Sprintf("Rows %d–%d", offset+1, offset+len(r.Rows))
		})).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption),
			ui.Caption("· 500 / page"), ggui.Spacer(),
			ui.Button("Previous", func() { m.page(-1) }).Outline().BindDisabled(ggui.Combine(m.Offset, m.Busy, func(n int, b bool) bool { return n == 0 || b })),
			ggui.TextOf(m.Offset.Map(func(n int) string { return fmt.Sprintf("Page %d", n/rowLimit+1) })).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption),
			ui.Button("Next", func() { m.page(1) }).Outline().BindDisabled(ggui.Combine(m.Data, m.Busy, func(r result, b bool) bool { return !r.Limited || b })),
		).Gap(10),
	).Gap(10).Align(ggui.AlignStretch)
}

func schemaView(m *model) ggui.Widget {
	return ggui.Column(ggui.Padding(ggui.Row(ui.Title("Table structure"), ggui.Spacer(), ggui.TextOf(m.Data.Map(func(r result) string { return fmt.Sprintf("%d columns", len(r.Columns)) })).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption)), 10, 0),
		ggui.Row(ggui.Box(ui.Caption("COLUMN")).Width(180), ggui.Box(ui.Caption("TYPE")).Width(110), ui.Caption("CONSTRAINTS / DEFAULT")).Gap(12), ui.Divider(),
		ggui.View(m.Data, func(result) ggui.Widget {
			rows := []ggui.Widget{}
			for _, c := range m.current.Columns {
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
				rows = append(rows, ggui.Row(ggui.Box(ggui.Text(c.Name)).Width(180), ggui.Box(ui.Caption(c.Type)).Width(110), ggui.Expanded(ui.Caption(strings.Join(flags, " · ")))).Gap(12), ui.Divider())
			}
			return ggui.Column(rows...).Gap(12).Align(ggui.AlignStretch)
		}), ui.Caption("CREATE STATEMENT"), ui.Card(ggui.TextOf(m.Schema)).Pad(18),
	).Gap(16).Align(ggui.AlignStretch)
}

func sqlView(m *model, unavailable ggui.Readable[bool]) ggui.Widget {
	return ggui.Column(
		ggui.Row(ui.Caption("SQL EDITOR"), ggui.Spacer(), ui.Button("Recent queries", func() { m.HistoryOpen.Set(true) }).Ghost(), ui.Caption("⌘/Ctrl + Enter")).Gap(10),
		ui.TextField(m.SQL).Name("SQL").Multiline().Lines(7).BindDisabled(m.Busy),
		ggui.Row(ui.Button("Run SQL", m.execute).BindDisabled(unavailable), ui.Button("Cancel query", m.stop).Ghost().BindDisabled(m.Busy.Map(func(b bool) bool { return !b })), ggui.Spacer(), ui.Caption("One statement · Auto-commit on success")).Gap(8),
		ui.Divider(), ggui.Row(ggui.Text("Results"), ggui.Expanded(ggui.TextOf(m.QueryStatus).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption)), ui.Button("Export results", func() { m.export(true) }).Outline().BindDisabled(unavailable)).Gap(12),
		ggui.Expanded(resultGrid(m.QueryData, ggui.State(0))),
	).Gap(12).Align(ggui.AlignStretch)
}

func openDialog(m *model) ggui.Widget {
	return ui.Dialog(m.OpenDialog, ggui.Column(
		ui.Caption("Paste the path of an existing SQLite database, or drop it onto the window."),
		ui.TextField(m.OpenPath).Name("Database path").Placeholder("/path/to/database.sqlite").OnSubmit(func(string) { m.open(m.OpenPath.Get()) }).BindDisabled(m.Busy),
		ui.Checkbox(m.ReadOnly, "Open read-only"), ggui.TextOf(m.Error).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption),
		ggui.Row(ui.Button("Cancel", func() { m.OpenDialog.Set(false) }).Ghost().BindDisabled(m.Busy), ui.Button("Open", func() { m.open(m.OpenPath.Get()) }).BindDisabled(m.Busy)).Gap(8).Justify(ggui.JustifyEnd),
	).Gap(16).Align(ggui.AlignStretch)).Title("Open database by path").Width(600)
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
		return ggui.Column(rows...).Gap(8).Align(ggui.AlignStretch)
	}))).Height(360)).Title("Recent queries").Width(700)
}

func resultGrid(source ggui.Readable[result], selected *ggui.StateValue[int]) ggui.Widget {
	return ggui.View(source, func(r result) ggui.Widget {
		if len(r.Columns) == 0 {
			return ggui.Center(ui.Empty("Ready when you are", "Run a query to see its results here."))
		}
		cols := make([]ui.Column[record], len(r.Columns))
		total := 0.0
		for i, name := range r.Columns {
			width := max(112.0, min(260.0, float64(utf8.RuneCountInString(name)*8+28)))
			for _, row := range r.Rows[:min(30, len(r.Rows))] {
				width = max(width, min(280, float64(utf8.RuneCountInString(cellText(row.Values[i]))*7+28)))
			}
			total += width
			cols[i] = ui.TextCol(name, func(row record) string { return cellText(row.Values[i]) }).Grow(width)
		}
		table := ui.Table(ggui.State(r.Rows), func(row record) int { return row.ID }, cols...).BindSelected(selected).RowHeight(36)
		box := ggui.Box(table)
		scroll := ggui.Scroll(box).Horizontal()
		// Size the viewport from the workspace, keeping the column headings and
		// pagination visible while only the table body scrolls vertically.
		return ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
			table.Height(max(80, c.MaxH))
			box.Width(max(total, c.MaxW))
			return scroll.Layout(c, env)
		}, func(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(scroll, r) })
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
	return ui.Dialog(m.Editing, ggui.Column(
		ui.Field("Column", ui.Select(m.Cell).BindOptions(m.Data.Map(func(result) []string {
			out := []string{}
			for _, c := range m.current.Columns {
				if c.Hidden != 1 {
					out = append(out, c.Name)
				}
			}
			return out
		})).OnChange(func(string) { m.loadCell() }).BindDisabled(m.Busy)),
		ui.Field("Storage type", ui.Select(m.Kind).Options([]string{"TEXT", "INTEGER", "REAL", "BLOB", "NULL"}).BindDisabled(m.Busy)),
		ui.Field("Value", ui.TextField(m.Value).Multiline().Lines(5).BindDisabled(ggui.Combine(m.Kind, m.Busy, func(kind string, b bool) bool { return kind == "NULL" || b }))).Help("BLOB values use hexadecimal. NULL is different from empty text."),
		ggui.TextOf(m.Error).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption), ggui.Row(ui.Button("Cancel", func() { m.Editing.Set(false) }).Outline().BindDisabled(m.Busy), ui.Button("Save cell", m.save).BindDisabled(ggui.Combine(ggui.Combine(m.Cell, m.Editable, func(name string, editable bool) bool {
			if !editable {
				return true
			}
			c, ok := m.current.column(name)
			return !ok || c.Hidden != 0
		}), m.Busy, func(a, b bool) bool { return a || b }))).Gap(8).Justify(ggui.JustifyEnd),
	).Gap(14).Align(ggui.AlignStretch)).Title("Cell details").Width(600)
}
func rowEditor(m *model) ggui.Widget {
	fields := ggui.View(m.Draft, func(fields []draftField) ggui.Widget {
		rows := []ggui.Widget{}
		for _, field := range fields {
			rows = append(rows, ggui.Column(ggui.Row(ggui.Text(field.Name), ui.Caption(field.Detail)).Gap(12), ggui.Row(ggui.Box(ui.Select(field.Kind).Options([]string{"DEFAULT", "TEXT", "INTEGER", "REAL", "BLOB", "NULL"}).BindDisabled(m.Busy)).Width(140), ggui.Expanded(ui.TextField(field.Value).BindDisabled(ggui.Combine(field.Kind, m.Busy, func(kind string, b bool) bool { return b || kind == "DEFAULT" || kind == "NULL" })))).Gap(10)).Gap(6))
		}
		return ggui.Column(rows...).Gap(16).Align(ggui.AlignStretch)
	})
	return ui.Dialog(m.Adding, ggui.Column(ui.Caption("DEFAULT omits the column so SQLite supplies its default or generated ID."), ggui.Box(ggui.Scroll(fields)).Height(360), ggui.TextOf(m.Error).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption), ggui.Row(ui.Button("Cancel insert", func() { m.Adding.Set(false) }).Outline().BindDisabled(m.Busy), ui.Button("Insert row", m.insert).BindDisabled(m.Busy)).Gap(8).Justify(ggui.JustifyEnd)).Gap(14).Align(ggui.AlignStretch)).Title("Add row").Width(720)
}
