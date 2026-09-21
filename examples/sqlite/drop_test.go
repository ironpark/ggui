package main

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/ironpark/ggui"
)

// newProbe drives the app around m, sized like the desktop window.
func newProbe(t *testing.T, m *model) *ggui.Probe {
	t.Helper()
	p := ggui.ProbeBuilder(func() ggui.Widget { return build(m) }, ggui.Sz(1000, 740))
	t.Cleanup(p.Close)
	m.dialogs = p.Dialogs() // so a tapped Open or Export is a recorded cancel
	return p
}

func TestDropOpensOriginalDatabase(t *testing.T) {
	_, path := fixture(t)
	m := newModel()
	defer m.close()
	p := newProbe(t, m)
	m.ReadOnly.Set(true)
	p.DropPaths(ggui.Pt(500, 360), path)
	if !m.Connected.Get() || m.Path.Get() != path || !m.Locked.Get() || len(m.Objects.Get()) != 3 {
		t.Fatalf("drop did not open original database: %s %s", m.Path.Get(), m.Error.Get())
	}
	m.SQL.Set("DELETE FROM composite")
	m.execute()
	if m.Error.Get() == "" {
		t.Fatal("drop ignored read-only setting")
	}
	// A second drop over the header works, even with a connection open.
	_, other := fixture(t)
	m.ReadOnly.Set(false)
	p.DropPaths(ggui.Pt(12, 12), other)
	if m.Path.Get() != other || m.Locked.Get() || m.Error.Get() != "" {
		t.Fatalf("replacement failed: %s", m.Error.Get())
	}
	if len(m.QueryData.Get().Columns) != 0 || m.QueryStatus.Get() != "Run SQL to see results here." {
		t.Fatal("stale results after switching databases")
	}
}

func TestInvalidDropsPreserveConnection(t *testing.T) {
	_, path := fixture(t)
	invalid := filepath.Join(t.TempDir(), "not-a-database.sqlite")
	if err := os.WriteFile(invalid, []byte("this is not SQLite"), 0600); err != nil {
		t.Fatal(err)
	}
	m := newModel()
	defer m.close()
	m.open(path)
	before := m.db
	p := newProbe(t, m)
	cases := []struct {
		name string
		drop func()
	}{
		{"invalid file", func() { p.DropPaths(ggui.Pt(600, 350), invalid) }},
		{"missing file", func() { p.DropPaths(ggui.Pt(600, 350), invalid+"-missing") }},
		{"folder", func() { p.DropPaths(ggui.Pt(600, 350), filepath.Dir(invalid)) }},
		{"multiple", func() { p.DropPaths(ggui.Pt(600, 350), path, invalid) }},
		{"no local path", func() { p.Drop(ggui.Pt(600, 350), fstest.MapFS{"data.db": {Data: []byte("SQLite format 3\x00")}}) }},
		{"busy", func() { m.Busy.Set(true); defer m.Busy.Set(false); p.DropPaths(ggui.Pt(600, 350), path) }},
		{"editing", func() {
			m.Editing.Set(true)
			defer m.Editing.Set(false)
			m.acceptDrop(ggui.DropEvent{Files: []ggui.DroppedFile{{Path: path}}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m.Error.Set("")
			tc.drop()
			if m.Error.Get() == "" {
				t.Fatal("missing actionable error")
			}
			if m.db != before || m.Path.Get() != path || !m.Connected.Get() {
				t.Fatal("invalid drop replaced existing connection")
			}
		})
	}
	m.SQL.Set("SELECT value FROM composite")
	m.execute()
	if m.Error.Get() != "" || len(m.QueryData.Get().Rows) != 1 {
		t.Fatal("existing connection unusable", m.Error.Get())
	}
}

func TestQueryTableQuotesIdentifier(t *testing.T) {
	_, path := fixture(t)
	m := newModel()
	defer m.close()
	m.open(path)
	m.selectTable(`odd" name`)
	m.queryTable()
	if m.Tab.Get() != 2 {
		t.Fatal("SQL editor not selected")
	}
	m.execute()
	if m.Error.Get() != "" || len(m.QueryData.Get().Rows) != 1 {
		t.Fatal("query table failed", m.Error.Get())
	}
}
