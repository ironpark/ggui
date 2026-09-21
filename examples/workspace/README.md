# Workspace

A small project dashboard that connects GGUI features through real actions.
It uses local, in-memory data and needs no services or credentials.

```sh
go run ./examples/workspace
```

On macOS, `task run-workspace` builds and launches an app bundle.
For the browser, use `task serve EXAMPLE=workspace`.

## Try it

1. Select a task row and choose **Edit selected**. Change its title, assignee,
   or status, then save. Cancel discards the draft.
2. Search by title or person, or filter by status. Create a task with
   **New task** or **⌘/Ctrl+N**; empty and overlong titles cannot be saved.
3. Open **Insights** to see the status chart and recent actions. The summary
   cards and animated completion bar reflect all tasks, regardless of filters.
4. In **Settings**, enable the extended team. Both the people preview and
   the editor's searchable assignee menu follow the same options source.
   Disabling the extended team preserves existing assignments.
5. Adjust the desktop metric card width, resize the window, or switch themes.
   Narrow windows keep three compact metric columns and show task cards instead
   of squeezing the table.
   Press **F1** to inspect the widget tree.
6. Delete a selected task. Cancel keeps it; confirmation removes it and
   updates the dashboard.

The table shows the filtered rows. Selection remains by task ID when a filter
hides a row, so **Edit selected** and **Delete selected** still act on that task.
All changes disappear when the app closes.

## Code map

- [main.go](main.go): app lifetime, theme binding, and keyboard shortcut.
- [model.go](model.go): state, derived search and counts, validation, and actions.
- [layout.go](layout.go): responsive arrangements that preserve controls across resizing.
- [view.go](view.go): layout, table selection, draft lenses, borrowed bindings,
  charts, dialogs, and toast feedback.
- [main_test.go](main_test.go): headless interaction tests using the same view.

Run `go test ./examples/workspace` to exercise the example.


## Layout previews

Render the actual widgets in light and dark themes at 1040, 760, and 480 pixels:

```sh
go run ./examples/workspace -render-dir /tmp/workspace-previews
```

This writes Tasks, Insights, Settings, and editor screenshots. If Metal is
unavailable in a macOS test environment, prefix the command with
`EBITENGINE_GRAPHICS_LIBRARY=opengl`.
