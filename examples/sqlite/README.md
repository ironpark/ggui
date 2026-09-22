# SQLite client

A desktop SQLite workspace built with GGUI and `modernc.org/sqlite`.
The layout takes inspiration from [Supabase Studio](https://supabase.com/blog/tabs-dashboard-updates):
a compact database header, searchable object sidebar, and a full-height table
workspace with an emerald accent and dark/light themes.
It opens existing local databases without creating demo tables.

The example is its own Go module, so `modernc.org/sqlite` stays out of the
library's dependency graph; the root `go.work` lets the commands below run
from the repository root, and `task test` covers it alongside the rest.

```sh
go run ./examples/sqlite -db /path/to/database.sqlite
go run ./examples/sqlite -db /path/to/database.sqlite -readonly
# Or start without -db and drop a database onto the window.
```

## Opening a database

Drop **one SQLite file anywhere in the window** or choose **Open database**.
The file chooser includes SQLite filters
and an All files option; drops accept any filename SQLite can open. The original
file is opened in place, preserving access to its WAL/journal files.

Folders, multiple files, and drops without a local path display an actionable
error. Invalid files preserve the current connection and results. While an
operation or row editor is active, finish/cancel it before dropping another
file. **Open read-only** applies to the next open, including drag and drop.
While a file is dragged over the window the welcome box and the status bar
say so, on the platforms that report the drag (macOS today); elsewhere the
drop itself is the first the app hears of it.

## Workflow

The left sidebar groups tables and views and filters them by name. Selecting an
object loads its first page. The main workspace keeps three separate tabs:

- **Data** — browse 25, 50, 100, 250 or 500 rows per page (default 500), filter text across columns, sort by a
  column, and jump between numbered pages. The footer shows the loaded range
  and the total matching row count. Page size changes return to page one; a
  refresh clamps the page if rows were removed. Search is always visible; press Enter or **Apply**
  to search the whole database. Click a column heading to cycle ascending,
  descending and default order, or open **Sort**. Sorting starts on page
  one and clears the previous selection. **More → Clear filters** resets search and sorting.
  The compact toolbar also contains refresh, **More → Export CSV**,
  **More → Query table**, and **Add row**.
  The grid uses the available window height with fixed headers and a persistent
  pagination footer. Column widths adapt to the data; wide tables scroll horizontally. The compact
  grid uses 34 px rows, vertical dividers, declared column types and a green
  primary-key marker. Select rows or their checkboxes independently; the heading checkbox selects
  the loaded page and shows a mixed state for partial selection. **Deselect**
  clears the set. Selection resets after loading another page or refreshing.
  Select exactly one row to enable the row actions and use **View / edit cell** to inspect its full value, including multiline
  text and hexadecimal BLOBs. NULL is distinct from empty text.
- **Structure** — inspect column types, primary keys, nullability, default
  values, generated columns and the object's CREATE statement.
- **SQL** — run a statement with one Run SQL button for queries, writes and DDL.
  Result columns, elapsed time, returned rows and the change count appear below
  the editor. The last 20 successful statements can be recalled from **Recent queries**.
  **Query table** prepares a safely quoted SELECT for the selected object.
  Table data and SQL results remain separate when switching tabs.
  SQL results use `ui.DataTable`: search all loaded values, sort headings, hide
  columns, select rows, and choose a page size (five by default). Search uses the
  full value even when its cell is abbreviated. NULL, numbers, text and BLOBs
  have distinct sort groups; INTEGER/REAL comparisons preserve integer precision.
  Numeric columns align right and NULL cells use muted text. These controls operate
  on the loaded result (at most 500 rows), without rerunning the SQL statement.
  The result area scrolls vertically when the window cannot fit the whole page.

**Add row** supplies an input for each writable column. DEFAULT omits the column
so SQLite supplies its default (including an automatic INTEGER PRIMARY KEY).
Choose TEXT, INTEGER, REAL, BLOB or NULL to supply a value explicitly. **Save
cell** commits a cell edit; **Delete row** requires confirmation. Changes are
saved immediately, with database constraints and foreign keys enforced.

Cell saves and deletes match both a usable row identifier and the original
values. Zero or multiple matches roll back the change, protecting against stale
or ambiguous edits. Composite keys and WITHOUT ROWID tables are supported.
Views, generated columns, and tables without a usable row identifier cannot be
updated through the form. Their values can still be inspected. Read-only mode
uses SQLite's `mode=ro` and disables data-editing controls; the checkbox applies
to the **next file open**, while the badge shows the current connection's mode.

Database operations run on a worker, leaving the UI responsive. Cancel stops a
running query; operations time out after 30 seconds (opening uses five seconds).
Failed queries preserve the previous result and display an error. Each SQL run
executes one statement inside a transaction and commits only on success.
The display keeps at most 500 result rows but consumes all RETURNING rows before
committing. A result limit notice appears if more rows were returned.

**Export CSV** exports the loaded database page. **Export loaded results** exports
the complete loaded SQL result, regardless of its local filter or visible DataTable
page. Both include
column headings. NULL and empty text both export as empty fields; BLOBs export
as hexadecimal. Use SQL filters or LIMIT/OFFSET to select another result window.

Keyboard shortcuts: **Cmd/Ctrl+O** opens a file, **Cmd/Ctrl+Enter** runs SQL,
**Cmd/Ctrl+R** refreshes the selected table, and **F1** opens the GGUI inspector.
Dark mode is the default; a light/dark theme switch is available in the header.

## Scope

This is a single-connection desktop example, designed for windows about
1000 px wide or larger. It does not provide an SQL language server, multi-tab
connections, full-result streaming export or undo after a committed edit.

The SQL editor accepts one statement per run. Explicit transaction control,
multi-statement scripts and trigger definitions containing multiple statements
are not supported. VACUUM and journal-mode changes that require execution outside
a transaction are not supported by this runner. Connection-changing PRAGMAs may
have SQLite's normal restrictions inside transactions. The browser shows objects
from the main database; use SQL to inspect temporary or attached databases.

Filtering is a literal substring search using SQLite's built-in lower()/instr()
semantics, not a SQL WHERE expression. Sorting appends the row identifier as a
tie-breaker where available. Offset pagination can shift if another connection
inserts or deletes rows; refresh to obtain the latest data.

## Validation

```sh
go test ./examples/sqlite
go test -race ./examples/sqlite
go run ./examples/sqlite -render-dir /tmp/sqlite-previews
```

The renderer creates its own temporary fixture and exports welcome, data, filters,
structure, query, cell editor, row editor and dark-theme screenshots at 1280 and
1000 px.
Tests cover file paths and quoted identifiers, pagination/filtering/sorting,
NULL/BLOB handling, composite and ambiguous keys, generated columns, defaults,
foreign keys, read-only writes, rollback, RETURNING limits, cancellation,
separate result state, CSV escaping, UI editing, window-wide file drops, invalid
drop recovery, read-only drops, quoted table-to-SQL navigation, heading-driven
server sorting beyond the first page, result pagination and filtering, duplicate
SQL column names, large integer comparisons, and searching abbreviated values.

`database.go` holds SQLite access, `statement.go` checks statement boundaries,
`model.go` coordinates asynchronous actions, `results.go` adapts query values to
DataTable columns, and `view.go` builds the interface.
Driver reference: https://pkg.go.dev/modernc.org/sqlite
