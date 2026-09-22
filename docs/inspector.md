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
panel — including its own monospaced font — reaches the binary. What stays
in every build is small: the `inspect` package that models a frame, the
`InspectFields` hooks on the built-in widgets, and `App.OnInspect`. The
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

## Custom widgets

The tree shows every widget painted through `Canvas.Paint` by its type
name, bounds and accessible role. A widget adds its own lines to the
Computed pane by implementing `inspect.Fielder` from the `inspect` package:

```go
func (w *GaugeWidget) InspectFields() []inspect.Field {
	return []inspect.Field{
		{Key: "Gauge"},
		{Key: "value", Value: inspect.Num(w.value), Number: true},
		{Key: "track", Value: inspect.Color(w.track)},
	}
}
```

A `Field` with no value is a section heading; `Number` colors a measurement.
`inspect.Num` and `inspect.Color` format the way the built-in widgets do.

## Frames for another viewer

The panel reads an `inspect.Frame`: the widgets as painted, in paint order
with parents before children, and the accessibility tree published beside
them. `App.OnInspect` hands the same frame to a function of your own after
every painted frame, for a viewer that lives elsewhere, such as a tool in
another process fed over a connection:

```go
app.OnInspect(func(fr *inspect.Frame) {
	fr.DescribeAll() // labels and roles, before the widgets are out of reach
	send(encode(fr))
})
```

A frame is valid until the function returns, since the next frame reuses its
buffers. What is cheap is on every node already: name, bounds, depth and
structural path. What costs a call into the widget is fetched through the
frame's `Source` when asked: `Describe` for the label and role, `Details`
for a pane's fields, `BoxOf` for the box model and `SemanticAt` for the
accessibility node under a point. A viewer in another process implements
`inspect.Source` over its connection to answer the same questions. Like the
panel, `OnInspect` does nothing without the `ggui_inspector` tag.
