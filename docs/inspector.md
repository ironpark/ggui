# Widget inspector

[Documentation](README.md) · [Project README](../README.md)

Inspect widget geometry, resolved properties, and accessibility while your application runs.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## Inspector

`Config{Inspector: "f1"}` binds a key that toggles a development
overlay in the shape of a browser's element panel; `App.Inspector(on)` does
the same from code. It is the quickest way to see why something sits where
it does.

The inspector is a development tool, so it ships only in builds tagged
`ggui_inspector`:

```sh
go run -tags ggui_inspector ./yourapp
```

Without the tag a no-op stands in: the configuration above still compiles
and `App.Inspector` still exists, but the call does nothing and none of the
inspector — including its own monospaced font — reaches the binary. The
`task run-*` and `task serve` development loops set the tag for you;
`task bundle` does not, because that is the release path.

The panel uses a compact, syntax-colored widget tree with type, accessible
name, role or layout badges, and dimensions. Moving the pointer follows the
frontmost visible widget. Click a tree row or breadcrumb to pin a selection;
use the picker tool to select directly in the app without activating its
controls. Escape unpins the selection or leaves picker mode.

The details panes show read-only runtime values:

- **Layout:** position, size, hierarchy, and a box model showing actual Box
  padding, painted border, and content dimensions in logical pixels. Borders
  paint inside the bounds; no CSS margin is inferred.
- **Computed:** resolved properties such as colors, typography, layout gaps,
  alignment, scrolling, and interaction state, where the widget exposes them.
- **Accessibility:** the selected widget's role, name, value, state, actions,
  and semantic ancestry. The copy button copies the selection's properties.

Click the search field to filter by type, accessible name, or role. With the
panel focused, Up/Down and Home/End navigate visible rows; Left/Right fold,
expand, or navigate ancestors and children. Each pane scrolls independently.
While the panel has keyboard focus it consumes every key press, including
ones it does not use. Click outside the panel to return keyboard focus to
the app; application drags that started outside the panel continue across
it. Closing the inspector clears the selection, folds and filter; docking,
outlines, sizes and the chosen tab persist.

| Shortcut (while open) | Action |
| --- | --- |
| Ctrl/Cmd+F | Focus search |
| Ctrl/Cmd+Shift+C | Toggle element picker |
| Ctrl/Cmd+Shift+D | Switch docking edge |
| Ctrl/Cmd+Shift+O | Toggle all widget outlines |

The inspector docks to the bottom by default and highlights only the selected
widget. Use `ShowOutlines: true` to initially outline every widget. The toolbar switches between
bottom and right docking, toggles outlines, and closes the inspector.
Drag the panel edge to resize it or the tree divider to adjust the split.
Wide bottom panels show tree, properties, and layout in three columns;
narrow panels stack the tree above tabbed details. Light and dark palettes
follow the app theme. The selected widget stays highlighted when all-widget
outlines are off. Custom widgets participate when painted through
`Canvas.Paint`. `App.SetInspector` sets the initial docking and outline options:

```go
app.SetInspector(ggui.InspectorOptions{Dock: ggui.InspectorBottom})
```
