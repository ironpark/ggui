package main

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// openDatabase and loadTable are the writable, first-page defaults the
// tests ask for most.
func openDatabase(path string) (*sql.DB, error) {
	db, _, err := openMode(path, false)
	return db, err
}
func loadTable(ctx context.Context, db *sql.DB, o object) (tableData, error) {
	return loadPage(ctx, db, o, browseOptions{})
}

func fixture(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample #?.db")
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path}).String())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE "odd"" name" (id INTEGER PRIMARY KEY, value TEXT, payload BLOB, optional TEXT); INSERT INTO "odd"" name" VALUES(1,'original',x'00ff',NULL); CREATE TABLE composite(a TEXT,b INTEGER,value TEXT,PRIMARY KEY(a,b)) WITHOUT ROWID; INSERT INTO composite VALUES('key',2,'before'); CREATE VIEW example_view AS SELECT * FROM composite;`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}
func TestBrowseAndPersist(t *testing.T) {
	db, path := fixture(t)
	ctx := context.Background()
	os, err := objects(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range os {
		data, err := loadTable(ctx, db, o)
		if err != nil {
			t.Fatal(err)
		}
		if o.Kind == "view" {
			if len(data.Keys) != 0 {
				t.Fatal("view must be read-only")
			}
			continue
		}
		idx := 0
		for i, c := range data.Result.Columns {
			if c == "value" {
				idx = i
			}
		}
		if err = updateCell(ctx, db, data, 0, idx, "edited ' text"); err != nil {
			t.Fatal(err)
		}
		if err = updateCell(ctx, db, data, 0, idx, "stale"); err == nil {
			t.Fatal("stale update succeeded")
		}
	}
	other, err := openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	r, err := query(ctx, other, `SELECT value,payload,optional FROM "odd"" name"`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Rows[0].Values[0] != "edited ' text" || display(r.Rows[0].Values[1]) != "00ff" || r.Rows[0].Values[2] != nil {
		t.Fatalf("unexpected persisted row: %#v", r)
	}
}
func TestLimitsAndErrors(t *testing.T) {
	db, _ := fixture(t)
	ctx := context.Background()
	r, err := query(ctx, db, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<510) SELECT x FROM n`)
	if err != nil || len(r.Rows) != 500 || !r.Limited {
		t.Fatalf("limit: %d %v %v", len(r.Rows), r.Limited, err)
	}
	if _, err = query(ctx, db, "not sql"); err == nil {
		t.Fatal("expected SQL error")
	}
	if _, err = openDatabase(filepath.Join(t.TempDir(), "missing.db")); err == nil {
		t.Fatal("must not create missing files")
	}
	for _, kind := range []string{"INTEGER", "REAL", "BLOB"} {
		if _, err = parseValue(kind, "invalid!"); err == nil {
			t.Fatalf("accepted invalid %s", kind)
		}
	}
}
func TestAmbiguousKeyRollsBack(t *testing.T) {
	db, _ := fixture(t)
	_, err := db.Exec(`CREATE TABLE shadow(rowid TEXT,_rowid_ TEXT,oid TEXT,k TEXT PRIMARY KEY,v TEXT); INSERT INTO shadow VALUES('a','b','c',NULL,'old'),('a','b','c',NULL,'old')`)
	if err != nil {
		t.Fatal(err)
	}
	data, err := loadTable(context.Background(), db, object{Name: "shadow", Kind: "table"})
	if err != nil {
		t.Fatal(err)
	}
	if err = updateCell(context.Background(), db, data, 0, 4, "new"); err == nil {
		t.Fatal("ambiguous update succeeded")
	}
	var count int
	db.QueryRow("SELECT count(*) FROM shadow WHERE v='old'").Scan(&count)
	if count != 2 {
		t.Fatal("update was not rolled back")
	}
}
func TestUI(t *testing.T) {
	_, path := fixture(t)
	m := newModel()
	defer m.close()
	m.open(path)
	m.Table.Set("composite")
	p := ggui.ProbeBuilder(func() ggui.Widget { return build(m) }, ggui.Sz(1100, 860))
	defer p.Close()
	p.Tap("Refresh")
	if len(m.Data.Get().Rows) != 1 {
		t.Fatal(m.Status.Get())
	}
	m.Selected.Set(1)
	p.Tap("View / edit cell")
	if !m.Editing.Get() {
		t.Fatal("editor did not open")
	}
	m.Cell.Set("value")
	m.loadCell()
	m.Value.Set("from UI")
	p.Tap("Save cell")
	if m.Editing.Get() {
		t.Fatal(m.Status.Get())
	}
	m.SQL.Set("SELECT value FROM composite")
	m.execute()
	if !strings.Contains(display(m.QueryData.Get().Rows[0].Values[0]), "from UI") {
		t.Fatal("edit was not saved")
	}
}

func TestPagingFilteringAndSorting(t *testing.T) {
	db, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(`CREATE TABLE paging(id INTEGER PRIMARY KEY,label TEXT); WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<510) INSERT INTO paging SELECT x,printf('item %03d',x) FROM n`)
	if err != nil {
		t.Fatal(err)
	}
	o := object{Name: "paging", Kind: "table"}
	a, err := loadPage(ctx, db, o, browseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := loadPage(ctx, db, o, browseOptions{Offset: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Result.Rows) != 500 || !a.Result.Limited || len(b.Result.Rows) != 10 || b.Result.Limited || b.Result.Rows[0].Values[0] != int64(501) {
		t.Fatal("incorrect page boundary")
	}
	c, err := loadPage(ctx, db, o, browseOptions{Filter: "item 50", Sort: "id", Desc: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Result.Rows) != 10 || c.Result.Rows[0].Values[0] != int64(509) {
		t.Fatalf("filter/sort: %#v", c.Result)
	}
	c, err = loadPage(ctx, db, o, browseOptions{Filter: "' OR 1=1 --"})
	if err != nil || len(c.Result.Rows) != 0 {
		t.Fatal("filter must be literal text", err)
	}
}
func TestInsertDeleteAndConstraints(t *testing.T) {
	db, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(`CREATE TABLE parent(id INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT 'untitled', doubled INTEGER GENERATED ALWAYS AS (id*2)); CREATE TABLE child(id INTEGER PRIMARY KEY,parent INTEGER REFERENCES parent(id));`)
	if err != nil {
		t.Fatal(err)
	}
	data, err := loadTable(ctx, db, object{Name: "parent", Kind: "table"})
	if err != nil {
		t.Fatal(err)
	}
	if err = insertRow(ctx, db, data, []inputValue{{Name: "id", Kind: "DEFAULT"}, {Name: "name", Kind: "TEXT", Text: "new"}}); err != nil {
		t.Fatal(err)
	}
	data, err = loadTable(ctx, db, data.Object)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Result.Rows) != 1 || data.Result.Rows[0].Values[2] != int64(2) {
		t.Fatal("default/generated values not populated")
	}
	if err = updateCell(ctx, db, data, 0, 2, int64(9)); err == nil {
		t.Fatal("generated column editable")
	}
	if _, err = db.Exec("INSERT INTO child VALUES(1,1)"); err != nil {
		t.Fatal(err)
	}
	if err = deleteRow(ctx, db, data, 0); err == nil {
		t.Fatal("foreign keys not enforced")
	}
	db.Exec("DELETE FROM child")
	if err = deleteRow(ctx, db, data, 0); err != nil {
		t.Fatal(err)
	}
	if err = deleteRow(ctx, db, data, 0); err == nil {
		t.Fatal("stale delete accepted")
	}
}
func TestUnifiedSQL(t *testing.T) {
	db, path := fixture(t)
	ctx := context.Background()
	if _, _, err := runSQL(ctx, db, "CREATE TABLE sql_run(id INTEGER PRIMARY KEY, name TEXT UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	r, n, err := runSQL(ctx, db, `INSERT INTO sql_run(name) VALUES('first') RETURNING id,name`)
	if err != nil || n != 1 || len(r.Rows) != 1 {
		t.Fatalf("returning: %#v %d %v", r, n, err)
	}
	if _, _, err = runSQL(ctx, db, `INSERT INTO sql_run(name) VALUES('second'),('first')`); err == nil {
		t.Fatal("constraint error expected")
	}
	r, _, err = runSQL(ctx, db, "SELECT count(*) FROM sql_run")
	if err != nil || r.Rows[0].Values[0] != int64(1) {
		t.Fatal("failed write did not roll back", err)
	}
	if _, _, err = runSQL(ctx, db, `SELECT 1; DELETE FROM sql_run`); err == nil {
		t.Fatal("multi statement accepted")
	}
	ro, _, err := openMode(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if _, _, err = runSQL(ctx, ro, `DELETE FROM sql_run`); err == nil {
		t.Fatal("read-only write accepted")
	}
	r, _, err = runSQL(ctx, ro, `PRAGMA table_info(sql_run)`)
	if err != nil || len(r.Rows) != 2 {
		t.Fatal("read-only schema query failed", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err = runSQL(canceled, db, "SELECT 1"); err == nil {
		t.Fatal("canceled query succeeded")
	}
}
func TestStatementAndCSV(t *testing.T) {
	for _, s := range []string{"SELECT ';' AS [x;y]; -- trailing comment", "/* comment */ SELECT 'it''s;ok'", "SELECT \"strange;name\"", "SELECT 1 /* a comment */;"} {
		if err := validateStatement(s); err != nil {
			t.Errorf("valid statement %q: %v", s, err)
		}
	}
	for _, s := range []string{"-- comment only", "/* unterminated", "BEGIN", "/* hi */ COMMIT", "SELECT 1;DELETE FROM x", "SELECT 'broken"} {
		if err := validateStatement(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
	var out strings.Builder
	if err := writeCSV(&out, result{Columns: []string{"text", "blob", "null"}, Rows: []record{{Values: []any{"a,\"b\"\nc", []byte{0, 255}, nil}}}}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "text,blob,null\n\"a,\"\"b\"\"\nc\",00ff,\n" {
		t.Fatalf("CSV escaping: %q", got)
	}
}

func TestAsyncCancellationAndSeparateResults(t *testing.T) {
	_, path := fixture(t)
	m := newModel()
	defer m.close()
	m.open(path)
	m.selectTable("composite")
	before := m.Data.Get()
	m.SQL.Set("SELECT 42 AS answer")
	m.execute()
	if len(m.Data.Get().Rows) != len(before.Rows) || m.Data.Get().Rows[0].Values[0] != before.Rows[0].Values[0] {
		t.Fatal("query replaced browser data")
	}
	posted := make(chan func(), 1)
	m.post = func(fn func()) { posted <- fn }
	m.SQL.Set("WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n) SELECT sum(x) FROM n")
	m.execute()
	if !m.Busy.Get() {
		t.Fatal("worker did not mark busy")
	}
	m.stop()
	select {
	case finish := <-posted:
		finish()
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not finish")
	}
	if m.Busy.Get() || m.Error.Get() == "" {
		t.Fatal("cancel status not applied")
	}
	if m.QueryData.Get().Rows[0].Values[0] != int64(42) {
		t.Fatal("failed query destroyed previous result")
	}
}
func TestReturningLimitCompletesAllWrites(t *testing.T) {
	db, _ := fixture(t)
	_, err := db.Exec("CREATE TABLE many(id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatal(err)
	}
	r, n, err := runSQL(context.Background(), db, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<510) INSERT INTO many SELECT x FROM n RETURNING id`)
	if err != nil || len(r.Rows) != 500 || !r.Limited || n != 510 {
		t.Fatalf("RETURNING completion: %d %d %v", len(r.Rows), n, err)
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM many").Scan(&count); err != nil || count != 510 {
		t.Fatal("some writes were lost", err)
	}
}
