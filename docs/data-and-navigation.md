# Data and navigation

[Documentation](README.md) · [Project README](../README.md)

Build tables, menus, date pickers, search, and navigation using themed controls.

Examples use `ggui`, `ui` and `ui/icons` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## On this page

- [Containers and feedback](#containers-and-feedback)
- [Tables](#tables)
- [Trees](#trees)
- [List boxes](#list-boxes)
- [Data tables](#data-tables)
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
| `ui.Tabs(selected, ui.Tab("One", page), ...)` | Displays the selected index, with an animated segmented selection. Arrows move focus; Enter/Space activates. `.ActivateOnFocus(true)` opts into automatic selection; `.Line()` uses an underline. Only the active page is laid out. `Tab(...).Header(widget)` draws an icon, count or badge in place of the label's text, which still names the tab; `Tab(...).Tooltip(text)` explains it. |
| `ui.Collapsible(open, "Title", content)` | A titled section whose content eases open and closed, as `Reveal` does. `ui.CollapsibleOf(open, trigger, content)` takes a header of your own, painted as it is, all of which toggles the content. |
| `ui.Card(child)` | Adds a surface, foreground/10 outline, radius and padding; `.Shadow(...)` opts into elevation. Given `CardHeader`, `CardContent` and `CardFooter` sections it lays them out as shadcn's card does; see [composition helpers](feedback-and-composition.md#composition-helpers). |
| `ui.Badge("new")` | Displays a small label; `.Secondary()`, `.Outline()`, `.Ghost()`, `.Link()` and `.Destructive()` change its look, and `.Icon(role)` or `.TrailingIcon(role)` add an icon. |
| `ui.Progress(value)` | Eases toward a fraction from a `Readable[float64]`. `.Label(text)` writes a label above its left end, `.ShowValue()` the percentage above its right end, and `.FormatValue(fn)` that value in words of your own, "3 of 12". |
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
| `.Resizable()` | Let the user drag the heading's right edge to change the column's width. |
| `.BindSelected(binding)` | Read and write the selected row key on click, the arrows, Space, or Enter. |
| `.BindSelection(binding)` | Edit a set of selected keys as a file list does: ⌘-click (Ctrl-click) toggles, Shift-click and Shift+arrows select a run, Space toggles, ⌘A selects all. |
| `.OnSelect(fn)` | Receive the activated item. |
| `.Height(h)` | Scroll the body beneath a fixed heading and virtualize rows. |
| `.RowHeight(h)` | Set the fixed row height (default: 40 logical pixels). |
| `.RowName(fn)` | Set each row's accessible name; otherwise the key is used. |
| `.Footer(cells...)` | A summary row under the body on a muted surface, one cell per column (`nil` leaves one empty); it stays put under a scrolling body. |
| `.Caption(text)` | A line of muted text centered under the table; a `Height` includes it. |

Without `Height`, the table grows to fit all rows.

A selectable table is one Tab stop: the row last used, or the selected one.
The arrows, Home, End, Page Up and Page Down move between rows, scrolling a
`Height` table to the row, and with `BindSelected` the selection follows.
Screen readers read it as a table of rows and cells under column headings.

```go
ui.Table(people, func(p Person) int { return p.ID },
	ui.TextCol("Name", func(p Person) string { return p.Name }),
	ui.TextCol("Age", func(p Person) string { return strconv.Itoa(p.Age) }).W(60).Right(),
).BindSelected(chosen).Height(240)
```

## Trees

`ui.Tree(roots, key, children, label)` shows a hierarchy as indented, keyed
rows with a disclosure chevron on each item that has children. Keys must be
unique across the whole tree; rows keep their state across expand and collapse.

| API | Purpose |
| --- | --- |
| `.BindExpanded(binding)` / `.Expanded(set)` | The keys of open items; the tree keeps its own set otherwise. |
| `.BindSelected(binding)` / `.Selected(key)` | The selected key, set by click, Space, Enter and arrow moves. |
| `.OnSelect(fn)` | Receive the item activated by click, Space or Enter. |
| `.RowName(fn)` | Set each row's accessible name; otherwise the key is used. |
| `.Name(s)` | Name the tree for assistive technology. |
| `.RowHeight(h)` / `.Indent(px)` | Row height (default 32) and per-level inset (default 16). |
| `.Height(h)` | Scroll the rows and virtualize them. |

The tree is one Tab stop (the last focused row, else the selected one, else
the first). Up/Down move between visible rows, Home/End jump to the ends,
Right opens an item or moves to its first child, and Left closes it or moves
to its parent. Rows report `RoleTreeItem` with `Level` and `Expanded`.

```go
ui.Tree(roots, func(f File) string { return f.Path },
	func(f File) []File { return f.Children },
	func(f ggui.Readable[File]) ggui.Widget {
		return ggui.TextOf(ggui.Map(f, func(f File) string { return f.Name }))
	},
).RowName(func(f File) string { return f.Name }).BindSelected(chosen).Height(320)
```

## List boxes

`ui.ListBox(items, key, row)` is a flat list of keyed rows of which one is
selected, such as a file list beside its preview. Each row's content is
yours; the list paints the selection and hover fill under it.

| API | Purpose |
| --- | --- |
| `.BindSelected(binding)` / `.Selected(key)` | The selected key, set by click, Space, Enter and arrow moves. |
| `.OnSelect(fn)` | Receive the item activated by click, Space or Enter. |
| `.RowName(fn)` | Set each row's accessible name; otherwise the key is used. |
| `.Name(s)` | Name the list for assistive technology. |
| `.Gap(v)` / `.Else(build)` | Space between rows (default 2), and what shows while there are none. |

The list is one Tab stop, as a tree is. Up/Down move the selection and the
focus with it, and Home/End jump to the ends. Rows report `RoleOption` with
`Selected`. Put the list in a `Scroll` to bound it: a selection changed from
elsewhere, such as a canvas showing the same items, scrolls its row into view
without taking the focus.

```go
ggui.Scroll(ui.ListBox(files, func(f File) string { return f.Path },
	func(f ggui.Readable[File]) ggui.Widget {
		return ggui.TextOf(ggui.Map(f, func(f File) string { return f.Name }))
	},
).RowName(func(f File) string { return f.Name }).BindSelected(chosen))
```

## Data tables

`ui.NewTableModel(rows, key, columns...)` handles client-side data operations;
`ui.DataTable(model)` presents search, sortable headings, column visibility,
selection checkboxes, an empty state and pagination. It follows the
[shadcn Data Table](https://ui.shadcn.com/docs/components/base/data-table)
composition using native ggui widgets without a JavaScript dependency.

Create the model once in component setup, outside a `Reactive` builder:

```go
model := ui.NewTableModel(people, func(p Person) int { return p.ID },
    ui.TextCol("Name", func(p Person) string { return p.Name }).
        Sortable(func(a, b Person) int { return strings.Compare(a.Name, b.Name) }),
    ui.TextCol("Age", func(p Person) string { return strconv.Itoa(p.Age) }).
        W(80).Right().Sortable(func(a, b Person) int { return cmp.Compare(a.Age, b.Age) }),
)
model.SetPageSize(10)
return ui.DataTable(model)
```

The Columns menu, a button with a chevron, lists each hideable column as a
checked row; sortable headings show their direction with an icon, and the
pagination buttons are the small size. Use `Col` for custom cells, including
menus and row actions. `TextCol` supplies
searchable text automatically; custom columns can set `Column.Search`.
Columns use their title as an ID by default. Set `.Identified("actions")` for
blank titles or titles that are not unique. Keys and column IDs must be unique
and stable. `Column.Header` supplies a custom header for plain `Table` or a
non-sortable DataTable column.

| Model API | Behavior |
| --- | --- |
| `SetQuery(text)` / `Query()` | Case-insensitive substring search across searchable columns, including hidden ones. Resets to page one. |
| `ToggleSort(id)` / `Sort()` | Ascending → descending → source order. Only `Sortable` columns participate. Resets to page one. |
| `Rows()` / `FilteredRows()` | Reactive current-page rows / matching rows before sorting and paging. |
| `SetPage(n)` / `Page()` / `PageCount()` | One-based pages, clamped to the available data. Empty results still have one page. |
| `SetPageSize(n)` / `PageSize()` | At least one row per page; resets to page one. Default ten. |
| `SetColumnVisible(id, visible)` / `ColumnVisible(id)` | Keeps at least one column visible. |
| `Select(key, selected)` / `IsSelected(key)` | Selection by key, retained across search, sorting and paging. |
| `SelectPage(selected)` | Changes only the current filtered page. |
| `SelectedRows()` | Selected items still present in the source, in source order. |

The header checkbox reports a mixed state when only some visible rows are
selected. The footer count covers all filtered rows, across pages. Removing an
item excludes it from `SelectedRows`; reintroducing the same key restores its
selection. The shared Checkbox also supports `Indeterminate(bool)` and
`BindIndeterminate(reader)` for custom presentations.

Filtering and stable sorting are memoized separately; selection and page
navigation do not repeat either operation. The source slice is never sorted in
place. Current-page rows reuse `Table`'s keyed cells. Visibility changes rebuild
the table structure. Treat model-returned slices as immutable. All mutations
belong on the UI goroutine, like other ggui state.

The default view scrolls horizontally on narrow screens, wraps footer controls,
and uses the current theme's hover, focus and popup animations, including reduced
motion. This model processes a local slice; server-side paging and multi-column
sorting are outside this API. For a custom toolbar or server-driven table,
compose the existing `Table`, fields and navigation controls directly.

See `examples/gallery/datatable.go` for a payment table with row actions.

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
option. Combobox retains what was typed in its input, re-filters the new list,
highlights the first match and resets result scrolling. Empty lists are valid.
Configure `.Format(fn)` before layout; replacement options use that formatter.
Radios keeps surviving value/duplicate-occurrence identities and their focus.
ToggleGroup keeps group focus; neither group lets an old press select a replacement.
All four constructors start empty and take only the selected-value binding.

`ui.Menu("File", ui.MenuItem("New", fn), ui.MenuDivider(), ...)` is a
secondary button that opens a list of actions the same way; an item runs
its function and closes the menu. The entries take the parts of shadcn's
dropdown menu:

| Entry | Behavior |
| --- | --- |
| `ui.MenuItem(label, fn)` | An action. `.Icon(widget)` puts an icon before the label, `.Inset()` lines a row without one up with those that have one, `.Shortcut(chord)` shows a key hint, and `.Destructive()` draws it in the Destructive color. |
| `ui.MenuLabel(text)` | A small muted heading over the entries after it, which the keyboard passes over; `.Inset()` lines it up with inset rows. |
| `ui.MenuCheckbox(label, checked)` | A row that toggles a `Binding[bool]` and shows a check at its right while it is true; choosing it leaves the menu open. |
| `ui.MenuRadioGroup(value, options...)` | One row per option with a check on the one the binding holds; `.Format(fn)`, `.Inset()`, `.Items()`. |
| `ui.MenuSub(label, entries...)` | A row that opens its entries as a submenu beside it, on hover, a click, or Right, Enter or Space; Left or Escape closes it alone. |
| `ui.MenuDivider()` | A line between groups of entries. |

Menus, selects and comboboxes draw in the theme's `Menu()`, which a Config's
`MenuColor` can make the inverted dark one.

```go
ui.Menu("Account",
	ui.MenuLabel("My account"),
	ui.MenuItem("Profile", openProfile).Icon(ui.Icon(icons.File)).Shortcut("cmd+p"),
	ui.MenuSub("Invite users", ui.MenuItem("By email", byEmail), ui.MenuItem("By link", byLink)),
	ui.MenuDivider(),
	ui.MenuCheckbox("Status bar", showStatus),
	ui.MenuRadioGroup(sortBy, "Newest", "Oldest", "Name"),
	ui.MenuDivider(),
	ui.MenuItem("Log out", logOut).Destructive(),
)
```

`ui.ContextMenu(content, entries...)` attaches the same entries to a
secondary-click target. Right-click opens at the
pointer, with placement adjusted to fit the window. Left clicks and scrolling
continue to the wrapped content. Use `.Name("File actions")` for its accessible
name; `.Disabled(true)` disables the menu while keeping the content usable.

Tab focuses the wrapper, then Shift+F10, Enter or Space opens it. Up/Down wrap
through enabled items, Home/End jump to the first/last enabled item, and
Enter/Space runs the highlighted action. Escape or an outside click closes it.
Accessibility clients can expand/collapse the menu and press its items. Keep
the instance in a persistent tree or assign a stable `.Key(...)` across rebuilds.

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
control, not the macOS system menu bar.

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
The calendar includes six weeks of 28-pixel cells, with adjacent-month dates
muted but selectable and today marked. `.Months(n)` shows n consecutive months
side by side, wrapping onto further rows where the width runs out; the previous
and next buttons still move one month. `.CaptionDropdown(true)` replaces the
heading with a month and a year Select, as shadcn's `captionLayout="dropdown"`
does, offering the years `.DropdownYears(from, to)` names, or the bounds, or a
hundred years back to ten ahead; `.MonthNames(names)` localizes the month list.
Keep it persistent or give it a stable `Key` when rebuilding.

`ui.DatePicker(date)` opens the same calendar in a popup. `.Calendar()` exposes
its bounds, timezone, localization and disabled-date settings. `.Format(fn)`
formats the trigger value, and `.Placeholder(text)` replaces "Choose date" for
an empty value. Selecting a date closes the popup, including the current date;
Escape or an outside click cancels navigation without changing the value.
`.Disabled(true)` closes and disables the picker. DatePicker owns the disabled
state of the Calendar returned by `.Calendar()`; configure `Disabled` or
`BindDisabled` on the picker, not that internal calendar. `.Key(key)` preserves its
state across rebuilds. Editable date text is not included.

`ui.DateRangePicker(trip)` binds a `ui.DateRange` through a `CalendarRange`
showing two months. Its popup stays open while the range is picked; Escape or a
click outside closes it. An empty range reads "Choose dates", and
`.FormatRange(fn)` and `.OnRangeChange(fn)` take the place of `Format` and
`OnChange`. Configure the range's limits on `.Calendar()`.

```go
trip := ggui.State(ui.DateRange{})
picker := ui.DateRangePicker(trip).Name("Trip dates")
picker.Calendar().RangeNights(1, 14)
```

## Pagination

`ui.Pagination(page, pageCount)` binds a one-based page and reads the number of
pages. It shows at most five numbered buttons, Previous and Next, with standard
button keyboard support. `.OnChange(fn)` reports user changes; `.Disabled(v)`
disables navigation, and `.Labels(previous, next)` renames the Previous and Next
buttons for another language. Data slicing or fetching stays with the application.

Explore these controls in the [component gallery](../examples/gallery).

## Accordions and search

`ui.Accordion(openKeys, ui.AccordionItem(key, title, content), ...)` groups
keyed disclosures; `.Multiple()` allows several open sections. The group supports
Up/Down, Home/End and Enter/Space, and its open content remains tabbable.

`ui.Combobox(value).Options(options)` adds search to dropdown selection: its
trigger is an editable input with a chevron, and typing there filters the list
below it while focus stays in the input; the options are menu-style rows with a
check at the right of the chosen one, read as options by a screen reader.
`.ClearButton(true)` puts a button that clears the value in place of the chevron
while one is chosen, and `.Empty(text)` replaces "No items found." `ui.Command(query,
ui.CommandItem(label, action), ...)` provides an inline command search; put it in
`ui.Dialog` for a palette. Both keep IME input with the existing editor.

## Sidebar navigation

`ui.Sidebar(selected, entries...)` is a column of destinations down the side of
a window. `ui.SidebarItem(key, label)` is a destination, whose key the binding
holds while it is the current one, and `ui.SidebarSection(title)` is a heading
over the ones that follow; `ui.SidebarSeparator()`, `ui.SidebarSkeleton()` and
`ui.SidebarCustom(widget)` fill out a group. The column is 256 logical pixels
wide until `.Width(px)` says otherwise. `.Header(widget)` and `.Footer(widget)`,
or `.HeaderItems(entries...)` and `.FooterItems(entries...)`, stay put while the
groups between them scroll.

An item takes `.Icon(role)` or `.Leading(widget)` before its label, `.Badge(text)`
or `.Action(widget)` at its end (`.ActionOnHover()` shows the action only under
the pointer), `.Detail(text)` under the label on a large row, and `.Size(...)`
among `ui.SidebarSizeDefault`, `SidebarSizeSmall` and `SidebarSizeLarge`.
`.Sub(entries...)` nests items under it, and `.Collapsible()` folds them behind a
chevron, `.Open(true)` or `.BindOpen(binding)` saying whether they show. `.OnTap(fn)`
runs an action instead of choosing a destination, and `.Tooltip(text)` is what
the icon rail shows for it.

`.BindCollapsed(binding)` collapses the sidebar while it is true, as
`.Collapsible(mode)` says: `ui.SidebarCollapseOffcanvas`, the default, slides it
off its edge; `ui.SidebarCollapseIcon` narrows it to a 48-pixel rail of icons
whose items show their tooltips; `ui.SidebarCollapseNone` ignores it. The width
moves over the theme's MotionSlow. `ui.SidebarTrigger(binding)` is the panel
button that toggles the same binding, `.Rail(true)` adds a strip along the inner
edge that toggles it on a click, and `.Shortcut("cmd+b")` toggles it from the
keyboard. `.Variant(ui.SidebarVariantFloating)` raises it as a rounded panel and
`ui.SidebarVariantInset` sets the content beside it in a `ui.SidebarInset(child)`;
`.Side(...)` says which edge it stands at.

```go
page, collapsed := ggui.State("inbox"), ggui.State(false)
ggui.Row(
	ui.Sidebar(page,
		ui.SidebarSection("Mail"),
		ui.SidebarItem("inbox", "Inbox").Icon(icons.Download).Badge("12").Tooltip("Inbox"),
		ui.SidebarItem("sent", "Sent").Icon(icons.Redo).Tooltip("Sent"),
		ui.SidebarItem("projects", "Projects").Icon(icons.File).Collapsible().Open(true).
			Sub(ui.SidebarItem("design", "Design"), ui.SidebarItem("research", "Research")),
	).Header(ui.Title("Acme")).
		Collapsible(ui.SidebarCollapseIcon).BindCollapsed(collapsed).Rail(true).Shortcut("cmd+b"),
	ggui.Expanded(ggui.Column(ui.SidebarTrigger(collapsed), content)),
)
```

The column is one keyboard tab stop: Up and Down move the highlight over the
enabled entries and wrap, skipping headings and disabled items, Home and End go
to the ends, Space or Enter goes to the highlighted destination, and Right and
Left open and close a collapsible row. A click goes there directly. The
highlight follows its entry by key across a rebuild.

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

`MenuItem(...).Shortcut("cmd+n")` and `CommandItem(...).Shortcut("cmd+n")`
display right-aligned hints only, written the platform's way ("⌘N" on macOS,
"Ctrl+N" elsewhere); text that is not a chord, such as "⌘N", shows as it is.
Register the actual shortcut with `App.Shortcut`, or use `App.SetMenu`, whose
items register and label their chords together.

For a compact command palette, use `ui.CommandDialog(open, command)`. It supplies
an accessible "Commands" name without a visible heading, removes the surrounding
dialog padding, and holds the 384-pixel panel's top a third of the way down the
window, so it stays put while the results change. Any dialog does the same with
`Dialog.TopAt(share)`. Use
`.Name("...")` on the returned dialog to customize its accessible name.

```go
ui.CommandDialog(open, ui.Command(query,
    ui.CommandItem("Change theme", changeTheme).Group("Appearance"),
    ui.CommandItem("Show notification", notify).Group("Actions"),
).Height(176).StableHeight().Hints())
```

`CommandEntry.Group` labels consecutive entries, `CommandEntry.Icon(widget)` puts
an icon before one, and `ui.CommandSeparator()` draws a line between entries
while the search is empty. Filtering hides headings with no matching entries,
and keyboard navigation skips headings. The search field is always a rounded,
muted inset; `Command.InsetSearch()` is kept for code that asked for it.
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


## Tab orientation and activation

`Tabs` uses manual keyboard activation by default. Arrows, Home and End move
focus without replacing the panel; Space or Enter selects the focused tab.
Use `.ActivateOnFocus(true)` to restore automatic selection. `.Vertical()` places
the list beside the content and uses Up/Down navigation. `.Line()` works in both
orientations. `ui.Tab(...).Disabled(true)` is skipped by navigation and cannot be
activated. Horizontal geometry and arrows follow `ggui.Directionality`.

## Rich navigation and popovers

`ui.NavigationMenu` groups direct `NavigationMenuLink` callbacks and
`NavigationMenuItem(label, content)` dropdowns. Panel content is arbitrary Go
widgets, so link descriptions, columns and cards compose normally. Links expose
link semantics, and `.Active(true)` marks the current destination. Resting the
pointer on a trigger for 200ms opens its dropdown, as shadcn's does, and it
closes shortly after the pointer leaves both; one opened by a click or the
keyboard stays open until dismissed. `.OpenOnHover(false)` opens dropdowns by
activation alone.

`ui.Popover(label, content)` provides a themed interactive panel. `.Title(text)`
and `.Description(text)` head it, `.Side(ggui.PopupRight)` and the other sides
choose where it opens (below by default, flipping when that side has no room),
and `.Align(a)` and `.AlignOffset(px)` place it along that side. Use
`PopoverOf(face, content)` for a custom trigger, `.BindOpen(binding)` for
controlled visibility, and `Show`, `Hide` or `Toggle` for programmatic control.
`Trigger()` exposes button variants and `Popup()` the lower-level configuration.
Keep popovers mounted or give them a stable `Key` across rebuilds.

## Calendar selection modes

`Calendar` selects one date. `CalendarRange` binds `ui.DateRange{Start, End}`;
`CalendarMultiple` binds `[]time.Time`. All return `*CalendarWidget`, sharing
localization, bounds, disabled dates and keyboard navigation.

```go
trip := ggui.State(ui.DateRange{})
ui.CalendarRange(trip).RangeNights(1, 14).ExcludeDisabled(true).
    ResetOnSelect(true).OnRangeChange(updateTrip)

dates := ggui.State([]time.Time{})
ui.CalendarMultiple(dates).SelectionLimits(0, 4).OnMultipleChange(updateDates)
```

A zero `Start` means no range; a zero `End` with a start is unfinished. Nights
are civil-day differences, including across daylight saving changes. Range
endpoints cannot be disabled. `.ExcludeDisabled(true)` also rejects intervals
containing a disabled date. `.Required(true)` prevents clearing the last
selection; it does not invent an initial value. Multiple selections publish
sorted, deduplicated copies. Callbacks receive requests; a controlled binding's
getter remains authoritative if it rejects a request.
