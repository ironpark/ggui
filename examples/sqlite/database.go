package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const rowLimit = 500

type record struct {
	ID     int
	Values []any
}
type result struct {
	Columns []string
	Rows    []record
	Limited bool
}
type object struct{ Name, Kind, Schema string }
type column struct {
	Name, Type string
	PK, Hidden int
	NotNull    bool
	Default    sql.NullString
}
type tableData struct {
	Object    object
	Columns   []column
	Result    result
	Keys      []string
	KeyValues [][]any
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// index is the position of the named column in the result, or -1.
func (r result) index(name string) int { return slices.Index(r.Columns, name) }

// column is the named column of the table, if it has one.
func (t tableData) column(name string) (column, bool) {
	i := slices.IndexFunc(t.Columns, func(c column) bool { return c.Name == name })
	if i < 0 {
		return column{}, false
	}
	return t.Columns[i], true
}

// openMode opens the database and reads its catalog, which is what proves
// the file is one.
func openMode(path string, readOnly bool) (*sql.DB, []object, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	mode := "rw"
	if readOnly {
		mode = "ro"
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=" + mode + "&_pragma=foreign_keys(1)&_pragma=busy_timeout(1000)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	os, err := objects(ctx, db)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	return db, os, nil
}
func objects(ctx context.Context, db *sql.DB) ([]object, error) {
	rows, err := db.QueryContext(ctx, `SELECT name,type,coalesce(sql,'') FROM sqlite_schema WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []object
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.Name, &o.Kind, &o.Schema); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func query(ctx context.Context, db querier, statement string, args ...any) (result, error) {
	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		return result{}, err
	}
	return scanRows(rows, false)
}

// scanRows reads up to rowLimit rows and closes rows. With drain, the rows
// past the limit are stepped through rather than left unread, which is
// what a statement with RETURNING needs to complete its writes.
func scanRows(rows *sql.Rows, drain bool) (result, error) {
	var out result
	fail := func(err error) (result, error) { rows.Close(); return result{}, err }
	columns, err := rows.Columns()
	if err != nil {
		return fail(err)
	}
	out.Columns = columns
	for rows.Next() {
		if len(out.Rows) == rowLimit {
			out.Limited = true
			if drain {
				continue
			}
			break
		}
		values := make([]any, len(out.Columns))
		dest := make([]any, len(values))
		for i := range values {
			dest[i] = &values[i]
		}
		if err = rows.Scan(dest...); err != nil {
			return fail(err)
		}
		out.Rows = append(out.Rows, record{ID: len(out.Rows) + 1, Values: values})
	}
	err = rows.Err()
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return result{}, err
	}
	return out, nil
}

type browseOptions struct {
	Offset       int
	Filter, Sort string
	Desc         bool
}

func loadPage(ctx context.Context, db *sql.DB, o object, opts browseOptions) (tableData, error) {
	t := tableData{Object: o}
	rows, err := db.QueryContext(ctx, `SELECT name,type,pk,hidden,"notnull",dflt_value FROM pragma_table_xinfo(?)`, o.Name)
	if err != nil {
		return t, err
	}
	for rows.Next() {
		var c column
		if err = rows.Scan(&c.Name, &c.Type, &c.PK, &c.Hidden, &c.NotNull, &c.Default); err != nil {
			rows.Close()
			return t, err
		}
		t.Columns = append(t.Columns, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return t, err
	}
	suffix := ""
	var params []any
	if opts.Filter != "" {
		var terms []string
		for _, c := range t.Columns {
			if c.Hidden != 1 {
				terms = append(terms, "instr(lower(CAST("+quote(c.Name)+" AS TEXT)), lower(?)) > 0")
				params = append(params, opts.Filter)
			}
		}
		if len(terms) > 0 {
			suffix = " WHERE (" + strings.Join(terms, " OR ") + ")"
		}
	}
	order := func(keys []string) string {
		var terms []string
		for _, c := range t.Columns {
			if c.Name == opts.Sort {
				direction := " ASC"
				if opts.Desc {
					direction = " DESC"
				}
				terms = append(terms, quote(c.Name)+direction)
			}
		}
		for _, key := range keys {
			terms = append(terms, quote(key))
		}
		if len(terms) == 0 {
			for _, c := range t.Columns {
				if c.Hidden != 1 {
					terms = append(terms, quote(c.Name))
				}
			}
		}
		if len(terms) == 0 {
			return ""
		}
		return " ORDER BY " + strings.Join(terms, ",")
	}
	tail := func(keys []string) string {
		return suffix + order(keys) + fmt.Sprintf(" LIMIT %d OFFSET %d", rowLimit+1, max(0, opts.Offset))
	}
	// Prefer an unshadowed rowid; WITHOUT ROWID tables fall back to their primary key.
	if o.Kind == "table" {
		if alias := rowidAlias(t.Columns); alias != "" {
			r, e := query(ctx, db, "SELECT "+alias+", * FROM "+quote(o.Name)+tail([]string{alias}), params...)
			if e == nil {
				t.Keys = []string{alias}
				for i := range r.Rows {
					t.KeyValues = append(t.KeyValues, []any{r.Rows[i].Values[0]})
					r.Rows[i].Values = r.Rows[i].Values[1:]
				}
				r.Columns = r.Columns[1:]
				t.Result = r
				return t, nil
			}
		}
		for _, c := range t.Columns {
			if c.PK > 0 {
				t.Keys = append(t.Keys, c.Name)
			}
		}
	}
	t.Result, err = query(ctx, db, "SELECT * FROM "+quote(o.Name)+tail(t.Keys), params...)
	for _, r := range t.Result.Rows {
		var key []any
		for _, name := range t.Keys {
			if i := t.Result.index(name); i >= 0 {
				key = append(key, r.Values[i])
			}
		}
		t.KeyValues = append(t.KeyValues, key)
	}
	return t, err
}

// rowidAlias is the first rowid alias no column of the table shadows, or
// empty when every one is taken.
func rowidAlias(columns []column) string {
	for _, alias := range []string{"rowid", "_rowid_", "oid"} {
		shadowed := slices.ContainsFunc(columns, func(c column) bool { return strings.EqualFold(c.Name, alias) })
		if !shadowed {
			return alias
		}
	}
	return ""
}

// rowMatch is the WHERE clause that finds one loaded row again: its key
// and every displayed value, so a row changed since it was loaded is not
// matched. It is what makes a stale edit or delete fail rather than land.
func rowMatch(t tableData, row int) (string, []any) {
	var terms []string
	var args []any
	for i, key := range t.Keys {
		terms = append(terms, quote(key)+" IS ?")
		args = append(args, t.KeyValues[row][i])
	}
	for i, col := range t.Result.Columns {
		terms = append(terms, quote(col)+" IS ?")
		args = append(args, t.Result.Rows[row].Values[i])
	}
	return strings.Join(terms, " AND "), args
}

// execOne runs a statement in its own transaction and commits only if it
// touched exactly one row.
func execOne(ctx context.Context, db *sql.DB, statement string, args ...any) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("row changed; refresh and try again (matched %d)", n)
	}
	return tx.Commit()
}

func updateCell(ctx context.Context, db *sql.DB, t tableData, row, col int, value any) error {
	if len(t.Keys) == 0 || row < 0 || row >= len(t.Result.Rows) || col < 0 || col >= len(t.Result.Columns) {
		return fmt.Errorf("select an editable table cell")
	}
	name := t.Result.Columns[col]
	if c, ok := t.column(name); ok && c.Hidden != 0 {
		return fmt.Errorf("generated columns cannot be edited")
	}
	where, args := rowMatch(t, row)
	return execOne(ctx, db, "UPDATE "+quote(t.Object.Name)+" SET "+quote(name)+" = ? WHERE "+where, append([]any{value}, args...)...)
}
func display(v any) string {
	if v == nil {
		return "NULL"
	}
	if b, ok := v.([]byte); ok {
		return hex.EncodeToString(b)
	}
	return fmt.Sprint(v)
}
func parseValue(kind, text string) (any, error) {
	switch kind {
	case "NULL":
		return nil, nil
	case "INTEGER":
		return strconv.ParseInt(text, 10, 64)
	case "REAL":
		return strconv.ParseFloat(text, 64)
	case "BLOB":
		return hex.DecodeString(text)
	default:
		return text, nil
	}
}

func deleteRow(ctx context.Context, db *sql.DB, t tableData, row int) error {
	if len(t.Keys) == 0 || row < 0 || row >= len(t.Result.Rows) {
		return fmt.Errorf("select an editable row")
	}
	where, args := rowMatch(t, row)
	return execOne(ctx, db, "DELETE FROM "+quote(t.Object.Name)+" WHERE "+where, args...)
}

type inputValue struct{ Name, Kind, Text string }

func insertRow(ctx context.Context, db *sql.DB, t tableData, inputs []inputValue) error {
	if t.Object.Kind != "table" {
		return fmt.Errorf("select a table")
	}
	var names, marks []string
	var args []any
	for _, input := range inputs {
		if input.Kind == "DEFAULT" {
			continue
		}
		if c, ok := t.column(input.Name); !ok || c.Hidden != 0 {
			return fmt.Errorf("column %s is not writable", input.Name)
		}
		v, err := parseValue(input.Kind, input.Text)
		if err != nil {
			return fmt.Errorf("%s: %w", input.Name, err)
		}
		names = append(names, quote(input.Name))
		marks = append(marks, "?")
		args = append(args, v)
	}
	statement := "INSERT INTO " + quote(t.Object.Name) + " DEFAULT VALUES"
	if len(names) > 0 {
		statement = "INSERT INTO " + quote(t.Object.Name) + " (" + strings.Join(names, ",") + ") VALUES (" + strings.Join(marks, ",") + ")"
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, statement, args...); err != nil {
		return err
	}
	return tx.Commit()
}

// Drain every result row before committing, including RETURNING beyond the display cap.
// Each statement rolls back on failure. Transaction-control statements are not supported.
func runSQL(ctx context.Context, db *sql.DB, statement string) (result, int64, error) {
	var out result
	if err := validateStatement(statement); err != nil {
		return out, 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return out, 0, err
	}
	defer tx.Rollback()
	var before, after int64
	if err = tx.QueryRowContext(ctx, "SELECT total_changes()").Scan(&before); err != nil {
		return out, 0, err
	}
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return out, 0, err
	}
	if out, err = scanRows(rows, true); err != nil {
		return result{}, 0, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT total_changes()").Scan(&after); err != nil {
		return result{}, 0, err
	}
	if err = tx.Commit(); err != nil {
		return result{}, 0, err
	}
	return out, after - before, nil
}
func writeCSV(w io.Writer, r result) error {
	out := csv.NewWriter(w)
	if err := out.Write(r.Columns); err != nil {
		return err
	}
	for _, row := range r.Rows {
		values := make([]string, len(row.Values))
		for i, v := range row.Values {
			if v != nil {
				values[i] = display(v)
			}
		}
		if err := out.Write(values); err != nil {
			return err
		}
	}
	out.Flush()
	return out.Error()
}
