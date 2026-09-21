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

func quote(s string) string                     { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func openDatabase(path string) (*sql.DB, error) { return openMode(path, false) }
func openMode(path string, readOnly bool) (*sql.DB, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	mode := "rw"
	if readOnly {
		mode = "ro"
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=" + mode + "&_pragma=foreign_keys(1)&_pragma=busy_timeout(1000)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = db.ExecContext(ctx, "PRAGMA busy_timeout=1000"); err == nil {
		_, err = objects(ctx, db)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
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
	var out result
	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.Columns, err = rows.Columns()
	if err != nil {
		return out, err
	}
	for rows.Next() {
		if len(out.Rows) == rowLimit {
			out.Limited = true
			break
		}
		values := make([]any, len(out.Columns))
		dest := make([]any, len(values))
		for i := range values {
			dest[i] = &values[i]
		}
		if err = rows.Scan(dest...); err != nil {
			return result{}, err
		}
		out.Rows = append(out.Rows, record{ID: len(out.Rows) + 1, Values: values})
	}
	return out, rows.Err()
}

type browseOptions struct {
	Offset       int
	Filter, Sort string
	Desc         bool
}

func loadTable(ctx context.Context, db *sql.DB, o object) (tableData, error) {
	return loadPage(ctx, db, o, browseOptions{})
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
		for _, alias := range []string{"rowid", "_rowid_", "oid"} {
			shadow := false
			for _, c := range t.Columns {
				if strings.EqualFold(c.Name, alias) {
					shadow = true
				}
			}
			if shadow {
				continue
			}
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
			break
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
			for i, col := range t.Result.Columns {
				if col == name {
					key = append(key, r.Values[i])
				}
			}
		}
		t.KeyValues = append(t.KeyValues, key)
	}
	return t, err
}
func updateCell(ctx context.Context, db *sql.DB, t tableData, row, col int, value any) error {
	if len(t.Keys) == 0 || row < 0 || row >= len(t.Result.Rows) || col < 0 || col >= len(t.Result.Columns) {
		return fmt.Errorf("select an editable table cell")
	}
	name := t.Result.Columns[col]
	for _, c := range t.Columns {
		if c.Name == name && c.Hidden != 0 {
			return fmt.Errorf("generated columns cannot be edited")
		}
	}
	// Match the original row as well as its key to detect concurrent edits.
	var terms []string
	args := []any{value}
	for i, key := range t.Keys {
		terms = append(terms, quote(key)+" IS ?")
		args = append(args, t.KeyValues[row][i])
	}
	for i, col := range t.Result.Columns {
		terms = append(terms, quote(col)+" IS ?")
		args = append(args, t.Result.Rows[row].Values[i])
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "UPDATE "+quote(t.Object.Name)+" SET "+quote(name)+" = ? WHERE "+strings.Join(terms, " AND "), args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("expected one row, matched %d; refresh before editing", n)
	}
	return tx.Commit()
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

// The entire original row is matched, so a stale or ambiguous delete rolls back.
func deleteRow(ctx context.Context, db *sql.DB, t tableData, row int) error {
	if len(t.Keys) == 0 || row < 0 || row >= len(t.Result.Rows) {
		return fmt.Errorf("select an editable row")
	}
	var terms []string
	var args []any
	for i, key := range t.Keys {
		terms = append(terms, quote(key)+" IS ?")
		args = append(args, t.KeyValues[row][i])
	}
	for i, name := range t.Result.Columns {
		terms = append(terms, quote(name)+" IS ?")
		args = append(args, t.Result.Rows[row].Values[i])
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "DELETE FROM "+quote(t.Object.Name)+" WHERE "+strings.Join(terms, " AND "), args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("row changed; refresh before deleting (matched %d)", n)
	}
	return tx.Commit()
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
		valid := false
		for _, col := range t.Columns {
			if col.Name == input.Name && col.Hidden == 0 {
				valid = true
			}
		}
		if !valid {
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
	out.Columns, err = rows.Columns()
	if err != nil {
		rows.Close()
		return out, 0, err
	}
	for rows.Next() {
		if len(out.Rows) == rowLimit {
			out.Limited = true
			continue
		}
		values := make([]any, len(out.Columns))
		dest := make([]any, len(values))
		for i := range values {
			dest[i] = &values[i]
		}
		if err = rows.Scan(dest...); err != nil {
			rows.Close()
			return result{}, 0, err
		}
		out.Rows = append(out.Rows, record{ID: len(out.Rows) + 1, Values: values})
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
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
