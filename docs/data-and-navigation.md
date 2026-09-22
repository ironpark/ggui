# Data and navigation

[Documentation](README.md) · [Project README](../README.md)

Build tables, menus, date pickers, search, and navigation using themed controls.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## On this page

- [Containers and feedback](#containers-and-feedback)
- [Tables](#tables)
- [Select and menu](#select-and-menu)
- [Menubar](#menubar)
- [Calendar and date picker](#calendar-and-date-picker)
- [Pagination](#pagination)
- [Accordions and search](#accordions-and-search)
- [Sidebar navigation](#sidebar-navigation)
- [Command and menu presentation](#command-and-menu-presentation)

## Containers and feedback

| Control | Behavior |
| --- | --- |
| `ui.Tabs(selected, ui.Tab("One", page), ...)` | Displays the selected index, with an animated segmented selection and Left/Right navigation; `.Line()` uses an underline. Only the active page is laid out. |
| `ui.Collapsible(open, "Title", content)` | Animates an expandable section with `Presence`. |
| `ui.Card(child)` | Adds a surface, border, radius, padding and subtle shadow. |
| `ui.Badge("new")` | Displays a small label; `.Accent()` emphasizes it. |
| `ui.Progress(value)` | Eases toward a fraction from a `Readable[float64]`. |
| `ui.Dialog(open, content)` | Shows a modal while the binding is true; see [focus scopes](input.md#focus-scopes). |

## Tables

`ui.Table(rows, key, cols...)` preserves row identity across edits and reorders.
Choose unique keys, then configure columns and selection:

| API | Purpose |
| --- | --- |
| `ui.TextCol(title, func(T) string)` | Text that follows the row's current item. |
| `ui.Col(title, func(ggui.Readable[T]) ggui.Widget)` | Custom cell content. |
| `.W(px)` / `.Grow(weight)` | Fixed width or a share of remaining width. |
| `.Right()` / `.Center()` | Align the heading and text cells. |
| `.BindSelected(binding)` | Read and write the selected row key on click, Space, or Enter. |
| `.OnSelect(fn)` | Receive the activated item. |
| `.Height(h)` | Scroll the body beneath a fixed heading and virtualize rows. |
| `.RowHeight(h)` | Set the fixed row height (default: 32 logical pixels). |
| `.RowName(fn)` | Set each row's accessible name; otherwise the key is used. |

Without `Height`, the table grows to fit all rows.
```go
ui.Table(people, func(p Person) int { return p.ID },
	ui.TextCol("Name", func(p Person) string { return p.Name }),
	ui.TextCol("Age", func(p Person) string { return strconv.Itoa(p.Age) }).W(60).Right(),
).BindSelected(chosen).Height(240)
```

## Select and menu

`ui.Select(value).Options(options)` is a dropdown bound to a
signal, labelled through `fmt.Sprint` or `.Format(fn)`: a click
or Space opens the list in a `Popup`, the arrow keys move through it (or
step the value while it is closed), Enter picks, Escape closes.

Select, Combobox, Radios and ToggleGroup shallow-copy their option slices; changing the caller's slice never replaces displayed options.
Treat objects referenced by slice elements as immutable.

For all four selection controls, `.Options(items)` replaces the options after mount and
`.BindOptions(reader)` follows a `Readable[[]T]` at layout:

```go
plans := ggui.State([]string{"free", "pro"})
selected := ggui.State("free")
picker := ui.Combobox(selected).BindOptions(plans).Name("Plan")
// Later, on the UI goroutine:
plans.Set([]string{"free", "pro", "team"})
```

The last setting wins: `Options` detaches an earlier reader. `BindOptions` requires
a non-nil reader; use `Options(items)` to detach explicitly. Each changed list is copied;
lists with the same elements in the same order keep their current rows. Neither
method writes the value binding or calls `OnChange`. A value removed from the
list remains formatted in Select; Combobox displays its placeholder.

Updating the options keeps an open popup open. Select highlights the current
value if present, otherwise nothing; the next Down/Up starts at the first/last
option. Combobox retains its query and search editor, re-filters the new list,
highlights the first match and resets result scrolling. Empty lists are valid.
Configure `.Format(fn)` before layout; replacement options use that formatter.
Radios keeps surviving value/duplicate-occurrence identities and their focus.
ToggleGroup keeps group focus; neither group lets an old press select a replacement.
All four constructors start empty and take only the selected-value binding.

`ui.Menu("File", ui.MenuItem("New", fn), ui.MenuDivider(), ...)` is a
secondary button that opens a list of actions the same way; an item runs
its function and closes the menu.

`ui.ContextMenu(content, entries...)` attaches the same `MenuItem` and
`MenuDivider` entries to a secondary-click target. Right-click opens at the
pointer, with placement adjusted to fit the window. Left clicks and scrolling
continue to the wrapped content. Use `.Name("File actions")` for its accessible
name; `.Disabled(true)` disables the menu while keeping the content usable.

Tab focuses the wrapper, then Shift+F10, Enter or Space opens it. Up/Down wrap
through enabled items, Home/End jump to the first/last enabled item, and
Enter/Space runs the highlighted action. Escape or an outside click closes it.
Accessibility clients can expand/collapse the menu and press its items. Keep
the instance in a persistent tree or assign a stable `.Key(...)` across rebuilds.
Nested submenus are not yet supported.

```go
ui.ContextMenu(ggui.Text("Project notes"),
    ui.MenuItem("Open", openNotes),
    ui.MenuDivider(),
    ui.MenuItem("Archive", archiveNotes),
).Name("Project notes actions")
```

## Menubar

`ui.Menubar(ui.Menu("File", entries...), ui.Menu("Edit", entries...))` creates
an in-window menu strip with one keyboard tab stop and one shared popup. Menus
passed to a bar belong to it and should not also be painted independently.
`.Name(name)` names the strip; `Menu.Disabled(true)` disables a top-level menu.

Left/Right wrap across enabled menus. Enter/Space or Down opens the first enabled
action; Up opens the last. Within an open menu, Up/Down move through enabled
actions and Home/End go to the first/last. Left/Right switch menus without closing
the popup. Hovering another trigger also switches menus. Escape and outside
clicks dismiss it; selection runs the action and restores focus to the bar.
Use a persistent instance or a stable `Key` across rebuilds. This is an in-window
control, not the macOS system menu bar; nested submenus are not included.

## Calendar and date picker

`ui.Calendar(date)` binds a `Binding[time.Time]` to a single civil date. A zero
value means no selection; the initial view shows today. External value changes
update the visible month without invoking `OnChange`. User selection stores
midnight in `.Location(...)` (default `time.Local`); selecting the same civil
date preserves the original value and does not invoke `OnChange`.

```go
date := ggui.State(time.Time{})
calendar := ui.Calendar(date).
    WeekStartsOn(time.Monday).
    Bounds(firstAllowedDate, lastAllowedDate).
    DisabledDate(func(d time.Time) bool { return d.Weekday() == time.Sunday })
picker := ui.DatePicker(date).Name("Due date").OnChange(saveDate)
picker.Calendar().WeekStartsOn(time.Monday)
```

Bounds are inclusive; zero endpoints are unbounded. Dates are interpreted in
the configured timezone. Reversed bounds allow no selections. Arrow keys move
the focused day by one day or week; Home/End move within the configured week;
PageUp/PageDown change month, clamping the day for shorter months. Navigation
does not select or modify the binding. Disabled dates can receive the keyboard
highlight for orientation, but Enter/Space and accessibility selection cannot
select them. Month controls and the date grid are separate tab stops.

`.WeekdayLabels([7]string{...})` takes Sunday-to-Saturday labels;
`.MonthLabel(fn)` formats the heading. `.Disabled(true)` disables the calendar.
The calendar includes six weeks, with adjacent-month dates muted but selectable.
Keep it persistent or give it a stable `Key` when rebuilding.

`ui.DatePicker(date)` opens the same calendar in a popup. `.Calendar()` exposes
its bounds, timezone, localization and disabled-date settings. `.Format(fn)`
formats the trigger value, and `.Placeholder(text)` replaces "Choose date" for
an empty value. Selecting a date closes the popup, including the current date;
Escape or an outside click cancels navigation without changing the value.
`.Disabled(true)` closes and disables the picker. DatePicker owns the disabled
state of the Calendar returned by `.Calendar()`; configure `Disabled` or
`BindDisabled` on the picker, not that internal calendar. `.Key(key)` preserves its
state across rebuilds. Date ranges and editable date text are not included.

## Pagination

`ui.Pagination(page, pageCount)` binds a one-based page and reads the number of
pages. It shows at most five numbered buttons, Previous and Next, with standard
button keyboard support. `.OnChange(fn)` reports user changes; `.Disabled(v)`
disables navigation. Data slicing or fetching stays with the application.

Explore these controls in the [component gallery](../examples/gallery).

## Accordions and search

`ui.Accordion(openKeys, ui.AccordionItem(key, title, content), ...)` groups
keyed disclosures; `.Multiple()` allows several open sections. The group supports
Up/Down, Home/End and Enter/Space, and its open content remains tabbable.

`ui.Combobox(value).Options(options)` adds search to dropdown selection. `ui.Command(query,
ui.CommandItem(label, action), ...)` provides an inline command search; put it in
`ui.Dialog` for a palette. Both keep IME input with the existing editor.

## Sidebar navigation

`ui.Sidebar(selected, entries...)` is a column of destinations down the side of
a window. `ui.SidebarItem(key, label)` is a destination, whose key the binding
holds while it is the current one, and `ui.SidebarSection(title)` is a heading
over the ones that follow. `.Header(widget)` and `.Footer(widget)` frame the
list, `.Width(px)` sets the column width, and `.BindCollapsed(reader)` takes the
sidebar off the page while the reader is true, which is what a narrow window
wants. There is no icon rail: an entry is named by its label alone.

```go
page := ggui.State("inbox")
ui.Sidebar(page,
	ui.SidebarSection("Mail"),
	ui.SidebarItem("inbox", "Inbox"),
	ui.SidebarItem("sent", "Sent"),
	ui.SidebarItem("spam", "Spam").Disabled(true),
).Header(ui.Title("Acme")).BindCollapsed(narrow) // narrow is a Readable[bool]
```

The column is one keyboard tab stop: Up and Down move the highlight over the
enabled entries and wrap, skipping headings and disabled items, Home and End go
to the ends, and Space or Enter goes to the highlighted destination. A click
goes there directly. The highlight follows its entry by key across a rebuild.

## Command and menu presentation

### Command palettes

Menu panels default to the theme's `MenuWidth` (224 logical pixels) rather
than stretching across the window. `Menu(...).Width(w)` overrides this. Menubars inset their triggers and
show a focus outline only for keyboard focus; pointer hover and the open menu
use the shared muted surface.

Command uses an integrated search header and divider. For a dialog palette,
`.Borderless().Height(144).StableHeight().Hints()` avoids nested panel borders,
keeps the popup from moving as search results change, and shows a keyboard-help
footer. Without `StableHeight`, the results shrink to their content. Empty
results display a centered message. Pointer movement updates the same selection
used by Enter; a stationary pointer no longer resets keyboard menu navigation.

`MenuItem(...).Shortcut("⌘N")` and `CommandItem(...).Shortcut("⌘N")` display
right-aligned hints only. Register the actual shortcut with `App.Shortcut`.

For a compact command palette, use `ui.CommandDialog(open, command)`. It supplies
an accessible "Commands" name without a visible heading, removes the surrounding
dialog padding, and gives the search field a muted inset background. Use
`.Name("...")` on the returned dialog to customize its accessible name.

```go
ui.CommandDialog(open, ui.Command(query,
    ui.CommandItem("Change theme", changeTheme).Group("Appearance"),
    ui.CommandItem("Show notification", notify).Group("Actions"),
).Height(176).StableHeight().Hints())
```

`CommandEntry.Group` labels consecutive entries. Filtering hides headings with no
matching entries, and keyboard navigation skips headings. `Command.InsetSearch()`
also enables the rounded search treatment for inline commands.
`Menubar.Compact()` keeps the painted strip at its content width inside a stretched
layout; omit it for a full-width application menu bar.

### Navigation appearance

Navigation uses restrained selection styles: Pagination outlines only the current
page and uses ghost buttons for other pages, with ellipses indicating hidden
ranges. Its current page is also exposed as selected to accessibility clients.
Accordion places its chevron on the right, aligns header and body text, and adds
space beneath expanded content. Tabs uses a compact segmented strip; wrap the
content in a Card only when a separate content surface is needed. Menubar and
ContextMenu share the same subdued floating-panel shadow.
