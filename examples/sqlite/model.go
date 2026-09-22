package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/runtime"
)

type draftField struct {
	Name, Detail string
	Kind, Value  *ggui.StateValue[string]
}
type model struct {
	dialogs runtime.FilePicker // the host's; see main
	db      *sql.DB
	opened  []*sql.DB // Workers append; adopting removes on the UI thread; close reads after Wait.
	post    func(func())
	cancel  context.CancelFunc
	workers sync.WaitGroup
	closed  bool
	current tableData

	Path, Status, Error, Table, Loaded, SQL, Schema      *ggui.StateValue[string]
	Cell, Kind, Value, Search, Filter, Sort, QueryStatus *ggui.StateValue[string]
	Objects                                              *ggui.StateValue[[]object]
	Data, QueryData                                      *ggui.StateValue[result]
	Selections                                           *ggui.StateValue[map[int]bool]
	Total, PageSize                                      *ggui.StateValue[int]
	Selected, Tab, Offset                                *ggui.StateValue[int]

	Editing, Adding, ConfirmDelete, Busy, Connected    *ggui.StateValue[bool]
	ReadOnly, Locked, Dark, Desc, Editable, Insertable *ggui.StateValue[bool]
	Draft                                              *ggui.StateValue[[]draftField]
	History                                            *ggui.StateValue[[]string]
	FilterOpen, HistoryOpen                            *ggui.StateValue[bool]
	DropHover                                          *ggui.StateValue[bool] // files are being dragged over the window
}

func newModel() *model {
	return &model{
		Path:        ggui.State(""),
		Status:      ggui.State("Open a SQLite database to get started."),
		Error:       ggui.State(""),
		Table:       ggui.State(""),
		Loaded:      ggui.State(""),
		SQL:         ggui.State("SELECT name, type FROM sqlite_schema ORDER BY name;"),
		Schema:      ggui.State(""),
		Cell:        ggui.State(""),
		Kind:        ggui.State("TEXT"),
		Value:       ggui.State(""),
		Search:      ggui.State(""),
		Filter:      ggui.State(""),
		Sort:        ggui.State(""),
		QueryStatus: ggui.State("Run SQL to see results here."),
		Objects:     ggui.State([]object{}).WithEqual(slices.Equal),
		Data:        ggui.State(result{}),
		QueryData:   ggui.State(result{}),
		Selected:    ggui.State(0),
		Total:       ggui.State(0), PageSize: ggui.State(rowLimit),
		Selections:    ggui.State(map[int]bool{}),
		Tab:           ggui.State(0),
		Offset:        ggui.State(0),
		Editing:       ggui.State(false),
		Adding:        ggui.State(false),
		ConfirmDelete: ggui.State(false),
		Busy:          ggui.State(false),
		Connected:     ggui.State(false),
		DropHover:     ggui.State(false),
		ReadOnly:      ggui.State(false),
		Locked:        ggui.State(false),
		Dark:          ggui.State(true),
		Desc:          ggui.State(false),
		Editable:      ggui.State(false),
		Insertable:    ggui.State(false),
		Draft:         ggui.State([]draftField{}),
		History:       ggui.State([]string{}),
		FilterOpen:    ggui.State(false),
		HistoryOpen:   ggui.State(false),
	}
}
func (m *model) close() {
	m.closed = true
	if m.cancel != nil {
		m.cancel()
	}
	m.workers.Wait()
	for _, db := range m.opened {
		db.Close()
	}
	if m.db != nil {
		m.db.Close()
	}
}

// Work receives immutable snapshots. Only its returned callback touches UI state.
// Tests run synchronously; the app posts completions from a worker onto the UI thread.
// An onFail runs on the UI thread after a failure is reported.
func (m *model) work(label string, fn func(context.Context) (func(), error), onFail ...func()) {
	if m.Busy.Get() || m.closed {
		return
	}
	m.Busy.Set(true)
	m.Error.Set("")
	m.Status.Set(label + "…")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	m.cancel = cancel
	perform := func() {
		done, err := fn(ctx)
		cancel()
		finish := func() {
			if m.closed {
				return
			}
			m.Busy.Set(false)
			m.cancel = nil
			if err != nil {
				m.Error.Set(err.Error())
				m.Status.Set(label + " failed")
				for _, fail := range onFail {
					fail()
				}
				return
			}
			m.Status.Set(label + " complete")
			if done != nil {
				done()
			}
		}
		if m.post == nil {
			finish()
		} else {
			m.post(finish)
		}
	}
	if m.post == nil {
		perform()
	} else {
		m.workers.Add(1)
		go func() { defer m.workers.Done(); perform() }()
	}
}
func (m *model) stop() {
	if m.cancel != nil {
		m.cancel()
		m.Status.Set("Canceling…")
	}
}
func (m *model) open(path string) {
	if strings.TrimSpace(path) == "" {
		m.Error.Set("Choose a database file.")
		return
	}
	ro := m.ReadOnly.Get()
	old := m.db
	m.work("Open database", func(ctx context.Context) (func(), error) {
		db, os, err := openMode(path, ro)
		if err != nil {
			return nil, err
		}
		m.opened = append(m.opened, db)
		// Adopt on the UI thread; shutdown closes connections even if this callback is never delivered.
		return func() {
			if old != nil {
				old.Close()
			}
			m.db = db
			m.opened = slices.DeleteFunc(m.opened, func(d *sql.DB) bool { return d == db })
			m.Path.Set(path)
			m.Editing.Set(false)
			m.Adding.Set(false)
			m.ConfirmDelete.Set(false)
			m.HistoryOpen.Set(false)
			m.FilterOpen.Set(false)
			m.Filter.Set("")
			m.Sort.Set("")
			m.Desc.Set(false)
			m.Draft.Set(nil)
			m.QueryStatus.Set("Run SQL to see results here.")
			m.Locked.Set(ro)
			m.Connected.Set(true)
			m.Objects.Set(os)
			m.Table.Set("")
			m.Loaded.Set("")
			m.current = tableData{}
			m.Data.Set(result{})
			m.QueryData.Set(result{})
			m.Schema.Set("")
			m.Offset.Set(0)
			m.Total.Set(0)
			m.clearSelection()
			m.Editable.Set(false)
			m.Insertable.Set(false)
			m.History.Set(nil)
			m.Search.Set("")
			m.Tab.Set(0)
			m.Status.Set("Opened " + filepath.Base(path))
			if len(os) > 0 {
				m.selectTable(os[0].Name)
			}
		}, nil
	})
}

// acceptDrop opens the original file so SQLite keeps its WAL and journal next
// to the database. Never copy a dropped file into a temporary database.
func (m *model) acceptDrop(ev ggui.DropEvent) {
	if m.Busy.Get() {
		m.Error.Set("An operation is running. Wait or cancel it, then drop the database again.")
		return
	}
	if m.Editing.Get() || m.Adding.Get() || m.ConfirmDelete.Get() {
		m.Error.Set("Finish or cancel the row editor before opening another database.")
		return
	}
	if len(ev.Files) != 1 {
		m.Error.Set("Drop one SQLite database file at a time.")
		return
	}
	f := ev.Files[0]
	if f.Dir {
		m.Error.Set("Drop a SQLite database file, not a folder.")
		return
	}
	if f.Path == "" {
		m.Error.Set("This drop has no local path. Use Open database to choose the original file.")
		return
	}
	m.open(f.Path)
}

func (m *model) queryTable() {
	if m.Loaded.Get() == "" || m.Busy.Get() {
		return
	}
	m.SQL.Set("SELECT * FROM " + quote(m.Loaded.Get()) + " LIMIT 500;")
	m.Tab.Set(2)
}

func (m *model) choose() {
	if m.Busy.Get() {
		return
	}
	path, err := m.dialogs.OpenFile(runtime.FileDialog{Title: "Open SQLite database", Filters: []runtime.FileFilter{
		{Name: "SQLite databases", Extensions: []string{"db", "sqlite", "sqlite3", "db3"}},
		{Name: "All files"},
	}})
	if errors.Is(err, runtime.ErrCanceled) {
		return
	}
	if err != nil {
		m.Error.Set(err.Error())
		return
	}
	m.open(path)
}
func (m *model) selectTable(name string) {
	if m.Busy.Get() {
		return
	}
	m.Table.Set(name)
	m.Filter.Set("")
	m.Sort.Set("")
	m.Desc.Set(false)
	m.Tab.Set(0)
	m.browsePage(0)
}
func (m *model) browse()               { m.browsePage(m.Offset.Get()) }
func (m *model) browsePage(offset int) { m.browseSizedPage(offset, m.PageSize.Get()) }
func (m *model) browseSizedPage(offset, size int) {
	if m.db == nil || m.Busy.Get() {
		return
	}
	name := m.Table.Get()
	opts := browseOptions{Offset: offset, PageSize: size, Filter: m.Filter.Get(), Sort: m.Sort.Get(), Desc: m.Desc.Get()}
	db := m.db
	m.Editable.Set(false)
	m.Insertable.Set(false)
	m.work("Load data", func(ctx context.Context) (func(), error) {
		os, err := objects(ctx, db)
		if err != nil {
			return nil, err
		}
		i := slices.IndexFunc(os, func(o object) bool { return o.Name == name })
		if i < 0 {
			return nil, fmt.Errorf("table or view no longer exists; reopen the database to refresh the catalog")
		}
		o := os[i]
		data, err := loadPage(ctx, db, o, opts)
		if err != nil {
			return nil, err
		}
		return func() {
			m.Objects.Set(os)
			m.current = data
			m.Loaded.Set(name)
			m.PageSize.Set(size)
			m.Offset.Set(data.Offset)
			m.Total.Set(data.Total)
			m.Data.Set(data.Result)
			m.Schema.Set(o.Schema)
			m.clearSelection()
			m.Editable.Set(len(data.Keys) > 0 && !m.Locked.Get())
			m.Insertable.Set(o.Kind == "table" && !m.Locked.Get())
			m.Status.Set(fmt.Sprintf("%s · %d rows loaded", name, len(data.Result.Rows)))
		}, nil
	})
}

// sortColumn orders the database query, not just the currently loaded page.
func (m *model) sortColumn(name string) {
	if m.Busy.Get() || m.db == nil || m.Data.Get().index(name) < 0 {
		return
	}
	if m.Sort.Get() != name {
		m.Sort.Set(name)
		m.Desc.Set(false)
	} else if !m.Desc.Get() {
		m.Desc.Set(true)
	} else {
		m.Sort.Set("")
		m.Desc.Set(false)
	}
	m.apply()
}

func (m *model) apply()         { m.browsePage(0) }
func (m *model) page(delta int) { m.browsePage(max(0, m.Offset.Get()+delta*m.PageSize.Get())) }
func (m *model) execute() {
	if m.db == nil || m.Busy.Get() {
		return
	}
	text := strings.TrimSpace(m.SQL.Get())
	if text == "" {
		m.Error.Set("Enter a SQL statement.")
		return
	}
	db := m.db
	m.Tab.Set(2)
	m.QueryStatus.Set("Running… Previous results shown until completion.")
	started := time.Now()
	m.work("Run SQL", func(ctx context.Context) (func(), error) {
		r, n, err := runSQL(ctx, db, text)
		if err != nil {
			return nil, err
		}
		os, err := objects(ctx, db)
		if err != nil {
			return nil, err
		}
		return func() {
			m.QueryData.Set(r)
			m.Objects.Set(os)
			note := fmt.Sprintf("%d rows · %d changes · %s", len(r.Rows), n, time.Since(started).Round(time.Millisecond))
			if r.Limited {
				note += " · showing first 500"
			}
			m.QueryStatus.Set(note)
			m.Status.Set("SQL completed")
			history := append([]string{text}, m.History.Get()...)
			m.History.Set(history[:min(20, len(history))])
			m.Editable.Set(false)
			m.Insertable.Set(false)
		}, nil
	}, func() { m.QueryStatus.Set("Failed — previous results retained.") })
}
func (m *model) edit() {
	row := m.Selected.Get() - 1
	if m.Busy.Get() || len(m.Selections.Get()) > 1 || row < 0 || row >= len(m.current.Result.Rows) {
		return
	}
	for _, c := range m.current.Columns {
		if c.Hidden == 0 {
			m.Cell.Set(c.Name)
			break
		}
	}
	m.loadCell()
	m.Error.Set("")
	m.Editing.Set(true)
}
func (m *model) loadCell() {
	row := m.Selected.Get() - 1
	if row < 0 || row >= len(m.current.Result.Rows) {
		return
	}
	i := m.current.Result.index(m.Cell.Get())
	if i < 0 {
		return
	}
	v := m.current.Result.Rows[row].Values[i]
	kind := "TEXT"
	switch v.(type) {
	case nil:
		kind = "NULL"
	case int64:
		kind = "INTEGER"
	case float64:
		kind = "REAL"
	case []byte:
		kind = "BLOB"
	}
	m.Kind.Set(kind)
	if v == nil {
		m.Value.Set("")
	} else {
		m.Value.Set(display(v))
	}
}
func (m *model) save() {
	if !m.Editable.Get() || m.Busy.Get() {
		return
	}
	value, err := parseValue(m.Kind.Get(), m.Value.Get())
	if err != nil {
		m.Error.Set(err.Error())
		return
	}
	col := m.current.Result.index(m.Cell.Get())
	db, t, row := m.db, m.current, m.Selected.Get()-1
	m.work("Save cell", func(ctx context.Context) (func(), error) {
		if err := updateCell(ctx, db, t, row, col, value); err != nil {
			return nil, err
		}
		return func() { m.Editing.Set(false); m.browse() }, nil
	})
}
func (m *model) add() {
	if !m.Insertable.Get() {
		return
	}
	fields := []draftField{}
	for _, c := range m.current.Columns {
		if c.Hidden != 0 {
			continue
		}
		detail := c.Type
		if c.PK > 0 {
			detail += " · PK"
		}
		if c.NotNull {
			detail += " · NOT NULL"
		}
		if c.Default.Valid {
			detail += " · default " + c.Default.String
		}
		fields = append(fields, draftField{Name: c.Name, Detail: detail, Kind: ggui.State("DEFAULT"), Value: ggui.State("")})
	}
	m.Draft.Set(fields)
	m.Error.Set("")
	m.Adding.Set(true)
}
func (m *model) insert() {
	if !m.Insertable.Get() {
		return
	}
	values := []inputValue{}
	for _, f := range m.Draft.Get() {
		values = append(values, inputValue{Name: f.Name, Kind: f.Kind.Get(), Text: f.Value.Get()})
	}
	db, t := m.db, m.current
	m.work("Insert row", func(ctx context.Context) (func(), error) {
		if err := insertRow(ctx, db, t, values); err != nil {
			return nil, err
		}
		return func() { m.Adding.Set(false); m.browse() }, nil
	})
}
func (m *model) askDelete() {
	if m.Editable.Get() && m.Selected.Get() > 0 {
		m.ConfirmDelete.Set(true)
	}
}
func (m *model) delete() {
	if !m.Editable.Get() {
		return
	}
	db, t, row := m.db, m.current, m.Selected.Get()-1
	m.work("Delete row", func(ctx context.Context) (func(), error) {
		if err := deleteRow(ctx, db, t, row); err != nil {
			return nil, err
		}
		return m.browse, nil
	})
}
func (m *model) export(query bool) {
	r := m.Data.Get()
	if query {
		r = m.QueryData.Get()
	}
	if len(r.Columns) == 0 || m.Busy.Get() {
		return
	}
	path, err := m.dialogs.SaveFile(runtime.FileDialog{Title: "Export displayed rows", FileName: "results.csv"})
	if errors.Is(err, runtime.ErrCanceled) {
		return
	}
	if err != nil {
		m.Error.Set(err.Error())
		return
	}
	m.work("Export CSV", func(context.Context) (func(), error) {
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		err = writeCSV(f, r)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		return func() { m.Status.Set(fmt.Sprintf("Exported %d displayed rows to %s", len(r.Rows), path)) }, err
	})
}
