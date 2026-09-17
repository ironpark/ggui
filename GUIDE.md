# ggui guide

[← README](README.md) · [Counter example](examples/counter) · [Todo example](examples/todo) · [Component gallery](examples/gallery)

Learn how to model state, compose widgets, and build interactive desktop apps
with ggui. Start with the [runnable counter](README.md#quick-start) if you have
not created an app yet. This guide assumes Go 1.27+.

Examples below are focused snippets unless they include a package declaration.
They use `ggui` and `ui` from `github.com/ironpark/ggui` and
`github.com/ironpark/ggui/ui`; names such as `save`, `todos`, and `accent`
represent application callbacks, data, or colors.

## Find your way

| I want to… | Start here |
| --- | --- |
| Understand reactive updates | [Signals and derived values](#concepts) |
| Keep component state across rebuilds | [Builders and components](#builders-and-components) |
| Render a changing or large collection | [Keyed lists](#keyed-lists) |
| Arrange and display content | [Widgets and layout](#widgets-and-layout) |
| Build forms and interactive screens | [Controls](#controls) · [Additional UI components](#additional-ui-components) |
| Customize appearance and motion | [Styling](#styling) · [Animation](#animation) |
| Handle keys, pointer input, and focus | [Input](#input) |
| Support assistive technology | [Accessibility](#accessibility) |
| Verify behavior without a window | [Testing](#testing) |
| Extend or debug the renderer | [Custom widgets](#custom-widgets) · [HiDPI](#hidpi) · [Frames](#frames) · [Inspector](#inspector) |
| Navigate the implementation | [Repository layout](#repository-layout) |

> [!IMPORTANT]
> APIs are under active development. Native accessibility support is currently
> **macOS only**; Windows and Linux accessibility bridges are not implemented.

## Concepts

### Signals

`State(v)` creates a `*Signal[T]`, with `T` inferred from the initial value.
Reads inside an effect or builder are tracked automatically. Writing a new
value schedules updates only for computations that depend on it.

```go
count := ggui.State(0)
count.Set(2)
count.Update(func(n int) int { return n + 1 })

label := ggui.Textf("Count: %d", count) // follows future changes
```

| Operation | Use it to… |
| --- | --- |
| `Get()` | Read a value and subscribe the current reactive computation. |
| `Peek()` | Read the current value without subscribing. |
| `Set(v)` | Replace the value. Equal values do not notify readers. |
| `Update(fn)` | Compute the next value from the current one. |
| `WithEqual(fn)` | Supply equality for values such as slices. |
| `Untrack(fn)` | Run a block without collecting dependencies. |

Keep event callbacks focused on writes. Use tracked reads in the builder or
computed value that should respond to a change.

### Derived values

`Derived(fn)` caches a computed value and refreshes it when
one of the signals `fn` read changes. `Signal.Map` is the common case, and
`Combine` folds two sources into one:

```go
total := ggui.Combine(price, qty, func(p float64, n int) float64 { return p * float64(n) })
pretty := total.Map(func(v float64) string { return fmt.Sprintf("$%.2f", v) })
```

A `Memo` notifies its readers only when the result actually differs, and a chain
of them settles within a single frame.

### Ownership and cleanup

Effects and derived values created while an effect runs belong to that owner.
They are disposed before the owner re-runs or when it is disposed. Dependencies
are collected again on every run, so a computation follows only the signals it
read most recently.

| API | Lifetime behavior |
| --- | --- |
| `app.Setup(fn)` | Runs under the app's root owner before the first build. Use it for app-wide watchers and theme bindings. |
| `OnCleanup(fn)` | Runs before the owning effect re-runs and when it is disposed. |
| `app.Close()` | Disposes the app's owned computations and ends `Run`. |
| `Root(fn)` | Creates an owner that does not re-run, useful for custom containers that retain children. |

For example, bind an app-wide theme before starting the app:

```go
dark := ggui.State(false)
app.Setup(func() {
	ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme())
})
```

Create component-local state and cleanup in component setup; see
[builders and components](#builders-and-components).

### State as a struct of signals

Keep one signal per piece of state and
group them in a plain struct. Each field is its own reactive cell, so a change
to one re-runs only what read it, and components take exactly the signals they
need:

```go
type model struct {
	Count *ggui.Signal[int]
	Step  *ggui.Signal[int]
}
state := model{Count: ggui.State(0), Step: ggui.State(1)}
ggui.Add(state.Count, state.Step.Get())
```

Use `Toggle`, `Add`, `Append`, and `Remove` for common updates.
`Remove(items, predicate)` creates a new slice without matching items and
notifies readers only when something was removed.

### Readers, bindings, and lenses

| Interface | Methods | Typical values |
| --- | --- | --- |
| `Reader[T]` | `Get()` | Signals, memos, and animated values. |
| `Binding[T]` | `Get()`, `Peek()`, `Set(T)` | Signals, lenses, tweens, and springs. |

Use a `Reader` for display-only data and a `Binding` when a control needs to
write back. `Watch` and `Combine` accept readers, so computed values work as
inputs too. A slider can bind to a spring just as it binds to a signal.

A lens exposes one field of a struct signal as a binding:

```go
type Form struct {
	Name string
}

form := ggui.State(Form{})
name := form.Lens(
	func(f Form) string { return f.Name },
	func(f Form, v string) Form {
		f.Name = v
		return f
	},
)
field := ui.TextField(name).Placeholder("Your name")
```

### Builders and components

A `Builder` is a `func() ggui.Widget`. It runs inside an effect and rebuilds
its tree when the signals it reads change. Give a subtree its own builder
when it should update independently.

`Component` separates setup, which runs once at first layout, from building,
which re-runs when its tracked inputs change:

```go
func hoverLabel(label string) ggui.Widget {
	return ggui.Component(func() ggui.Builder {
		hovered := ggui.State(false) // setup: local state
		return func() ggui.Widget {
			text := label
			if hovered.Get() { // tracked read: rebuild on hover changes
				text += " · hovered"
			}
			return ggui.Pointer(ggui.Text(text)).OnHover(hovered.Set)
		}
	})
}
```

Choose the smallest boundary that fits the job:

| API | Use when… |
| --- | --- |
| `Component(setup)` | A subtree needs local state and cleanup. |
| `Reactive(build)` | A subtree needs independent updates without setup. |
| `View(reader, build)` | A subtree depends on one reactive value, e.g. `ggui.View(name, ggui.Text)`. |
| `Keyed(key, setup)` | A component must survive its enclosing builder's rebuilds. |
| `Mount(key, props, setup)` | A keyed component also needs updated props, passed to setup as a signal. |
| `When(cond, then, else)` | A condition selects between widgets constructed once. |
| `TextOf(reader)` / `Textf(format, readers...)` | Text should follow reactive values and retain chainable text setters. |

`Sprintf` provides the memo behind reactive formatted text.

> [!TIP]
> A parent rebuild creates a new ordinary `Component`. Keep the parent's
> builder free of tracked reads and put changing content in smaller reactive
> subtrees, or use `Keyed` / `Mount` when local state must survive parent rebuilds.

### Keyed lists

`For(items, key, build)` watches a `Reader[[]T]` and keeps one child per key.
Reordering the list reuses each child's state. The child receives a `Reader[T]`
that follows its current item; read it reactively and make edits through the model.

```go
ggui.Scroll(
	ggui.For(todos, func(t Todo) int { return t.ID },
		func(item ggui.Reader[Todo]) ggui.Widget {
			return ggui.View(item, func(t Todo) *ggui.TextWidget {
				return ggui.Text(t.Title)
			})
		},
	).Gap(4),
)
```

| Collection API | Identity and updates |
| --- | --- |
| `For(items, key, build)` | Reactive collection with an explicit key per item. |
| `Each(items, build)` | Reactive collection of comparable items, keyed by their value. |
| `List(items, build)` | Plain slice, rebuilt with its parent. |
| `Children(items, build)` | Turns a plain slice into children for containers such as `Row`. |

A child is built on its first layout. For large lists inside `Scroll`, use
`.ItemExtent(h)` to give every row a fixed height; with `.Horizontal()`, it sets
width instead. Only visible rows need to be built, laid out, and painted.

- `.Retain(n)` limits how many offscreen rows remain mounted. Rows holding focus
  or pointer capture are never evicted; evicted rows rebuild when they return.
- `.Transition(wrap)` wraps each row in an enter/leave transition. Removed rows
  play the animation backwards and stop accepting input before disappearing.

## Widgets and layout

A `Widget` is asked for a size under `Constraints` and an `Env`
of inherited values, then asked to paint into the `Rect` its parent assigned
on a `Canvas`. Constraints flow down, sizes flow up.
Every built-in follows one shape: a constructor takes what the widget cannot
do without, chainable setters take the rest, so the code reads as the tree it
builds:

```go
ggui.Center(
	ggui.Box(
		ggui.Column(
			ggui.Text(title).Color(fg),
			ggui.Text(body).Color(dim),
		).Gap(8),
	).Pad(24).Fill(panel),
)
```

### Choosing a layout

| Need | Widgets |
| --- | --- |
| A line of children | `Row`, `Column` |
| Share remaining space | `Flex`, `Expanded`, `Spacer` |
| Flow onto multiple lines | `Wrap` |
| Equal-width columns | `Grid` |
| Overlap children | `Stack` |
| Position one child | `Align`, `Center` |
| Decorate or inset content | `Box`, `Padding` |
| Show overflowing content | `Scroll` |

Build children from a plain slice with `List` or `Children`:

```go
ggui.List(rows, func(r Row) ggui.Widget {
	return ggui.Text(r.Title)
}).Gap(4)

ggui.Row(ggui.Children(rows, func(r Row) ggui.Widget {
	return ggui.Text(r.Title)
})...)
```

Padding uses CSS order: `Padding(w, 8)` sets every side, `Padding(w, 4, 12)`
sets vertical/horizontal padding, and `Box(w).Pad(1, 2, 3, 4)` sets
top/right/bottom/left.

`Stack` layers children at the top-left and takes its largest child's size
unless `.Expand()` is set. `Align` fills available space and positions its child
by fraction (`.At(0.25, 1)`) or edge (`.Bottom().Right()`). `Center` places it at
`(0.5, 0.5)`.

`ggui.Sz(w, h)` and `ggui.Pt(x, y)` accept ints or floats without casts.

### Rows, columns, and flex

`Row` and `Column` hug their children by default. `Expanded(child)` and
`Flex(child, weight)` share whatever main-axis space the rigid children leave,
`Spacer()` is an empty `Expanded`, and `.Justify(...)` distributes slack
(`JustifyCenter`, `JustifyEnd`, `SpaceBetween`, `SpaceAround`, `SpaceEvenly`).
Any of those makes the widget fill its main axis. `.Align(...)` places
children across the axis (`AlignStart`, `AlignCenter`, `AlignEnd`,
`AlignStretch`); a Row centers by default, a Column starts at the left.

```go
ggui.Row(ggui.Text("Title"), ggui.Spacer(), ggui.Text("3 items")) // centered on its height
ggui.Column(ggui.Text("a"), ggui.Text("b")).Align(ggui.AlignStretch)
```

### Wrap and grid

`Wrap` and `Grid` cover the two other common arrangements. `Wrap` flows
children left to right and starts a new line where the next one would not
fit, for tags and toolbars; `.Gap(v)` spaces both axes, `.RunGap(v)` the
lines alone, `.Align(...)` places children within their line. `Grid(cols,
...)` deals children into equal-width columns, each given its cell width
tight so columns line up, with rows as tall as their tallest cell.

### Tooltips and popups

`Tooltip(child, text)` shows text below the child once the cursor has
rested on it for half a second (`.Delay(d)`). It registers no hit region, so
the child gets every event, and it paints through `Canvas.Overlay`, above
everything else.

`Popup(anchor, content)` floats content below its anchor (above it when
there is no room), painted through `Canvas.Overlay` over a scrim: a press
anywhere outside closes it and reaches nothing underneath. `Show`, `Hide`,
`Toggle` and `IsOpen` drive it, or `.Bind(sig)` keeps the state in a
`Signal[bool]`; `.Keys(h)` keeps keyboard focus on the widget that opened it
while the pointer is in the content, and a widget inside can find its popup
with `PopupOf(env)` to close it after acting. `ui.Select` and `ui.Menu` are
built on it.

### Images

`Image(img)` draws an `*ebiten.Image` at its natural size, shrinking to
the room it gets with its aspect ratio kept; `.Size`, `.Width` or `.Height`
fix it, and `.Fit(FitContain | FitCover | FitFill | FitNone)` says how it
sits in a box of another shape. `DecodeImage(bytes)` and
`LoadImageFile(path)` read PNG, JPEG and GIF.

### Text

`Text` wraps at spaces to the width it is given, and between runes when a
word is wider than the line, so scripts without spaces wrap too. `.Size(px)`,
`.Color(c)`, `.Font(f)`, `.LineHeight(mult)`, `.Style(ts)`, `.Align(0.5)` and
`.NoWrap()` adjust it; what is not set is inherited (see Styling). The
built-in font is Go Regular; `LoadFont(ttf)` or `LoadFontFile(path)` load your
own, and `SetDefaultFont` makes one the default.

## Controls

Import `github.com/ironpark/ggui/ui` for themed controls. They use the core
layout, reactivity, and input APIs, so custom controls can follow the same model.
A bound control writes its value to the binding; writing to the binding updates
the control.

### A small form

Here `save` and `cancel` are application callbacks:

```go
name := ggui.State("")
agree := ggui.State(false)
dark := ggui.State(false)
size := ggui.State(0.5)
plan := ggui.State("free")

ggui.Column(
	ui.TextField(name).Placeholder("Your name").OnSubmit(func(_ string) { save() }),
	ui.Checkbox(agree, "I agree"),
	ui.Switch(dark, "Dark mode"),
	ui.Slider(size, 0, 1).Step(0.1),
	ui.Radios(plan, []string{"free", "pro"}).Label(strings.ToTitle),
	ggui.Row(ui.Button("Save", save), ui.Button("Cancel", cancel).Outline()).Gap(8),
	ui.Divider(),
).Gap(12)
```

### Buttons and shared options

`ui.Button(label, onTap)` creates a primary button. Use `.Outline()` for a
secondary action or `ui.ButtonOf(child, onTap)` for custom content.
`ui.Radio(plan, value, label)` creates one radio option; `ui.Radios` builds a group.

Common options on interactive controls include:

| Option | Purpose |
| --- | --- |
| `.Disabled(v)` | Disable interaction. |
| `.DisabledWhen(reader)` | Follow a reactive disabled state without rebuilding. |
| `.OnChange(fn)` | Observe a user-selected value on controls that expose this callback. |
| `.OnCommit(fn)` | Observe completed editing on sliders and text fields. |
| `.Pad(...)` | Override padding on controls such as buttons. |

Controls obtain colors and spacing from the inherited theme. Hover and press
state stay in the widget; animated details retain their motion across rebuilds.
Animations use `ggui.Now()`, which `ggui.SetClock` can replace in tests.

### Text input and validation

Use `ui.TextField(value)` for a themed editor, or `ggui.TextInput(value)` to
supply your own decoration. Both bind to a string value.

| Option | Behavior |
| --- | --- |
| `.Placeholder(s)` | Show a hint when empty. |
| `.Password()` | Mask the displayed value. |
| `.MinWidth(w)` | Set a minimum editor width. |
| `.Multiline()` | Wrap text and accept line breaks. |
| `.Lines(n)` | Enable multiline editing with at least `n` lines of height. |
| `.OnChange(fn)` | Handle edits. |
| `.OnSubmit(fn)` | Handle Enter in a single-line field or ⌘/Ctrl+Enter in a multiline field. |

Editors support IME composition, with preedit text underlined in place.
Caret movement and Backspace respect grapheme clusters, including combined
letters and multi-code-point emoji. Caret and selection state survive rebuilds.
Use `.Input().Focused()` on a `ui.TextField` to inspect focus.

| Interaction | Result |
| --- | --- |
| Click / drag | Place the caret / select text. |
| Double-click / triple-click | Select a word / all text. |
| Arrows / Shift+arrows | Move / extend selection. |
| Alt or Ctrl with arrows | Move by word. |
| Home / End | Move to line boundaries; macOS also supports ⌘ navigation. |
| ⌘/Ctrl+A, C, X, V | Select all, copy, cut, paste. |
| ⌘/Ctrl+Z | Undo; consecutive typing is grouped. |
| ⌘+Shift+Z / Ctrl+Y | Redo. |

Wrap an input with `ui.Field` for a label, help, and reactive validation feedback:

```go
name := ggui.State("")
problem := ggui.State("")
field := ui.Field("Name", ui.TextField(name)).
	Help("Shown on your profile").
	Error(problem)
```

A nonempty error replaces the help text. The field label also names the control
for `Probe.Find`.

### Fonts and international text

The built-in font covers Latin, Greek and Cyrillic. Glyphs a font lacks are
drawn from its fallbacks: `SystemFonts()`, the CJK and wide-coverage fonts
found at well-known paths on macOS, Windows and Linux (plus any files in
the `GGUI_FONTS` environment variable), so Korean, Japanese and Chinese
render out of the box on a machine that has such a font. `Fallback(fonts...)`
on a `Font` chooses a chain of your own and `NoFallback()` turns it off.
`LoadFontFile` also reads the first face of a `.ttc`; `LoadFontCollection`
returns them all.

### Containers and feedback

| Control | Behavior |
| --- | --- |
| `ui.Tabs(selected, ui.Tab("One", page), ...)` | Displays the selected index, with an animated segmented selection and Left/Right navigation; `.Line()` uses an underline. Only the active page is laid out. |
| `ui.Collapsible(open, "Title", content)` | Animates an expandable section with `Presence`. |
| `ui.Card(child)` | Adds a surface, border, radius, padding and subtle shadow. |
| `ui.Badge("new")` | Displays a small label; `.Accent()` emphasizes it. |
| `ui.Progress(value)` | Eases toward a fraction from a `Reader[float64]`. |
| `ui.Dialog(open, content)` | Shows a modal while the binding is true; see [focus scopes](#focus-scopes). |

### Tables

`ui.Table(rows, key, cols...)` is a keyed list of rows under a
heading row: `ui.TextCol(title, func(T) string)` is a text column that
follows its item, `ui.Col(title, func(Reader[T]) Widget)` holds any
widget, and `.W(px)`, `.Grow(flex)` and `.Right()` size and align a
column. `.Selected(binding)` highlights the row whose key the binding
holds and sets it on a click or Space, `.OnSelect(fn)` gets the item,
`.Height(h)` scrolls the body under a fixed heading and lays out only the
rows in view, and `.Label(fn)` names rows for `Probe.Find`.

```go
ui.Table(people, func(p Person) int { return p.ID },
	ui.TextCol("Name", func(p Person) string { return p.Name }),
	ui.TextCol("Age", func(p Person) string { return strconv.Itoa(p.Age) }).W(60).Right(),
).Selected(chosen).Height(240)
```

### Select and menu

`ui.Select(value, options)` is a dropdown bound to a
signal, labelled through `fmt.Sprint` or `.Label(fn)`: a click
or Space opens the list in a `Popup`, the arrow keys move through it (or
step the value while it is closed), Enter picks, Escape closes.
`ui.Menu("File", ui.MenuItem("New", fn), ui.MenuDivider(), ...)` is a
secondary button that opens a list of actions the same way; an item runs
its function and closes the menu.

`ui.ContextMenu(content, entries...)` attaches the same `MenuItem` and
`MenuDivider` entries to a secondary-click target. Right-click opens at the
pointer, with placement adjusted to fit the window. Left clicks and scrolling
continue to the wrapped content. Use `.Named("File actions")` for its accessible
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
).Named("Project notes actions")
```

## Additional UI components

### Menubar

`ui.Menubar(ui.Menu("File", entries...), ui.Menu("Edit", entries...))` creates
an in-window menu strip with one keyboard tab stop and one shared popup. Menus
passed to a bar belong to it and should not also be painted independently.
`.Named(name)` names the strip; `Menu.Disabled(true)` disables a top-level menu.

Left/Right wrap across enabled menus. Enter/Space or Down opens the first enabled
action; Up opens the last. Within an open menu, Up/Down move through enabled
actions and Home/End go to the first/last. Left/Right switch menus without closing
the popup. Hovering another trigger also switches menus. Escape and outside
clicks dismiss it; selection runs the action and restores focus to the bar.
Use a persistent instance or a stable `Key` across rebuilds. This is an in-window
control, not the macOS system menu bar; nested submenus are not included.

### Calendar and date picker

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
picker := ui.DatePicker(date).Named("Due date").OnChange(saveDate)
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
`.Disabled(true)` closes and disables the picker. `.Key(key)` preserves its
state across rebuilds. Date ranges and editable date text are not included.

### Notices and loading states

`ui.Alert(title, description)` provides an inline notice, with `.Destructive()`
and `.Action(widget)`. `ui.Empty(title, description)` presents an empty state,
with optional `.Media(widget)` and `.Action(widget)`.

`ui.Kbd("Ctrl")` displays a key cap. `ui.Skeleton(180, 16)` reserves loading
space (`.Circle()` rounds it), and `ui.Spinner().Size(20)` indicates ongoing
work. Both loading indicators respect reduced motion.

### Pagination

`ui.Pagination(page, pageCount)` binds a one-based page and reads the number of
pages. It shows at most five numbered buttons, Previous and Next, with standard
button keyboard support. `.OnChange(fn)` reports user changes; `.Disabled(v)`
disables navigation. Data slicing or fetching stays with the application.

Explore these controls in the [component gallery](examples/gallery).

### Accordions and search

`ui.Accordion(openKeys, ui.AccordionItem(key, title, content), ...)` groups
keyed disclosures; `.Multiple()` allows several open sections. The group supports
Up/Down, Home/End and Enter/Space, and its open content remains tabbable.

`ui.Combobox(value, options)` adds search to dropdown selection. `ui.Command(query,
ui.CommandItem(label, action), ...)` provides an inline command search; put it in
`ui.Dialog` for a palette. Both keep IME input with the existing editor.

### Resizable panes

`ui.Resizable(fraction, first, second)` splits space into two panes with a draggable,
keyboard-operable divider; `.Vertical()` and `.MinSizes(a, b)` configure it.

### Toast notifications

`ui.NewToaster()` owns a bounded notification queue. Create it once in app or
component setup, and call it on the UI thread. Include the host once in the
tree, register `ggui.OnCleanup(toaster.Close)` in setup, then call
`toaster.Push(ui.Toast(title, description).Action("Undo", undo))` from UI callbacks.
Notifications do not steal focus and pause their timeout while hovered or focused.
They fade and slide in and out, and the stack moves smoothly when notices change.
Reduced motion skips these animations. Timed notices show their remaining lifetime;
`.Duration(0)` keeps a notice until dismissed. `Dismiss` immediately removes its
interaction and excludes it from `Len`, while its exit animation finishes.

### Sheets and drawers

`ui.Sheet(open, content)` is a modal panel anchored to an edge of the window,
for filters, details and settings that deserve more room than a popup. It
behaves as `ui.Dialog` does — a scrim takes the clicks, Tab stays inside,
Escape or a click outside closes it, focus returns to where it was — but it
fills one edge instead of floating in the middle. `.Left()`, `.Right()`
(the default), `.Top()` and `.Bottom()` choose the edge, `.Size(px)` sets
how far it reaches from it, `.Title(text)` adds a heading and `.Compact()`
drops the padding.

```go
filters := ggui.State(false)
ui.Sheet(filters, filterForm).Title("Filters").Left().Size(280)
```

`ui.Drawer(open, content)` is the same panel rising from the bottom, with the
grab handle that says so. Both slide in and out over the theme's `MotionSlow`,
and land at once under reduced motion. The motion lives in the widget, so keep
the sheet across rebuilds — construct it beside the signal it binds, or inside
a `Component` — and one rebuilt every frame simply appears and goes, as
`ui.Dialog` does.

### Sidebar navigation

`ui.Sidebar(selected, entries...)` is a column of destinations down the side of
a window. `ui.SidebarItem(key, label)` is a destination, whose key the binding
holds while it is the current one, and `ui.SidebarSection(title)` is a heading
over the ones that follow. `.Header(widget)` and `.Footer(widget)` frame the
list, `.Width(px)` sets the column width, and `.Collapsed(reader)` takes the
sidebar off the page while the reader is true, which is what a narrow window
wants. There is no icon rail: an entry is named by its label alone.

```go
page := ggui.State("inbox")
ui.Sidebar(page,
	ui.SidebarSection("Mail"),
	ui.SidebarItem("inbox", "Inbox"),
	ui.SidebarItem("sent", "Sent"),
	ui.SidebarItem("spam", "Spam").Disabled(true),
).Header(ggui.Title("Acme")).Collapsed(narrow) // narrow is a Reader[bool]
```

The column is one keyboard tab stop: Up and Down move the highlight over the
enabled entries and wrap, skipping headings and disabled items, Home and End go
to the ends, and Space or Enter goes to the highlighted destination. A click
goes there directly. The highlight follows its entry by key across a rebuild.

### Grouped actions and choices

`ui.ButtonGroup(children...)` joins controls into one bordered strip with a
hairline between each pair; `.Vertical()` stacks them. It is a container and
nothing more, so every child keeps its own tab stop and its own action. Ghost
buttons suit it, since the strip draws the border they would each draw.

`ui.ToggleGroup(value, options)` is a segmented single choice: one option of
several, bound the way `ui.Select` and `ui.Radios` are. `.Label(fn)` sets how
an option is shown, `.Vertical()` stacks the segments, and `.OnChange(fn)`
reports user changes. The group is one tab stop, as a set of radio buttons is:
Left and Right (Up and Down when vertical) move the choice and wrap, Home and
End go to the ends, and Space or Enter re-picks where the choice already is.
Each segment is announced as a radio that says whether it is the chosen one.

```go
ui.ButtonGroup(ui.Button("Copy", copyIt).Ghost(), ui.Button("Paste", pasteIt).Ghost())
ui.ToggleGroup(align, []string{"left", "center", "right"})
```

### Confirmations

`ui.AlertDialog(open, title, description)` is a question that has to be
answered: the scrim takes the clicks but does not close it, so the only ways
out are its two buttons and Escape, which cancels. `.Confirm(label, fn)` names
the action and what it does, `.Cancel(label)` renames the other button,
`.OnCancel(fn)` hears about every cancellation however it happened, and
`.Destructive()` colors the confirming button for an answer that cannot be
taken back.

```go
del := ggui.State(false)
ui.AlertDialog(del, "Delete the file?", "This cannot be undone.").
	Confirm("Delete", remove).Destructive()
```

The policy itself is `ui.Dialog`'s: `.Dismissible(false)` on any dialog stops
a click on the scrim from closing it, while leaving Escape alone so the
keyboard is never shut in.

### Composition helpers

These are small widgets with no state of their own, for the shapes that
otherwise get rebuilt by hand in every application.

| Widget | Behavior |
| --- | --- |
| `ui.Avatar(name)` | A round portrait: the initials of the name on the muted surface until `.Image(img)` gives it one, cropped to cover the circle. `.Size(px)` sets the diameter and `.Square()` rounds it to the theme's radius instead. |
| `ui.AspectRatio(ratio, child)` | Sizes the child to a width-over-height ratio inside the space it is offered: the width leads, unless the height it implies would not fit. |
| `ui.Item(title, description)` | One row of a list: `.Media(widget)` before the text, `.Action(widget)` after it, `.Outline()` for a bordered card. It takes no input, so the action keeps its own. |
| `ui.Breadcrumb(crumbs...)` | The path to the page the user is on, built from `ui.Crumb(label, onTap)`. Every step but the last is a link with its own tab stop; `.Separator(s)` replaces the "/", and `.Max(n)` elides the middle behind an ellipsis. |
| `ui.InputGroup(input)` | One field chrome around a bare `ggui.TextInput` and the widgets that flank it: `.Leading(widget)`, `.Trailing(widget)`. The group owns the border, fill, padding and focus ring, so it takes the editor rather than `ui.TextField`, which draws a box of its own. |
| `ui.HoverCard(anchor, content)` | A panel of content shown near the anchor once the cursor has rested on it, and kept up while the cursor is on either one. `.Delay(d)` and `.Width(px)` tune it. Nothing about it takes focus: a hover card is an aside, and the keyboard never has to visit it. |

```go
ggui.Row(
	ui.Avatar("Ada Lovelace").Image(portrait).Size(32),
	ui.Item("Backups", "Last run 2 hours ago").Action(ui.Button("Run", run).Outline()),
).Space(1)
```

## Animation

`Tween(v, d)` and `Spring(v)` are values that move toward their target over
time instead of jumping, after Svelte's `tweened` and `spring`. Both are
`Reader`s, so an island that reads one rebuilds every frame the value moves:

```go
width := ggui.Tween(0.0, 200*time.Millisecond).Easing(ggui.EaseOut)
bar := ggui.Reactive(func() ggui.Widget {
	return ggui.Box().Size(width.Get(), 4).Fill(accent)
})
width.Set(120) // slides there over 200ms; Jump(v) skips the motion
```

A tween restarts from wherever it is when retargeted; a spring keeps its
momentum, overshoots a little and settles (`.Stiffness`, `.Damping`). Easings:
`EaseLinear`, `EaseIn`, `EaseOut`, `EaseInOut`. The runtime steps running
animations once per frame, before effects are flushed. For a look that moves
inside one widget, `Motion` is the same tween driven from `Paint` with the
current time and no signal; `ui.Switch` slides its knob with one.

### Enter and leave transitions

`Transition(child).Fade().Slide(dx, dy).Scale(from)` plays
an enter animation when the child first appears, over `.Duration(d)` with
`.Easing(e)`. Whether it is new is judged against the previous frame by the
identity a keyed component gives it, else by Rect, so a Builder that
rebuilds every frame does not restart it.
`Presence(show, child)` keeps the child on screen when `show` turns false,
inert to input, and runs the same animation backwards before removing it:

```go
ggui.Presence(open, ggui.Transition(panel).Slide(0, -8).Fade())
```

## Styling

Styling has three layers: local widget styles, inherited values in `Env`, and
shared theme tokens. A widget's explicit setters override inherited text styles.

### Local styles

`TextStyle{Font, Size, Color, LineHeight}` is what
`Text`'s setters write into; a zero field means "inherit". `a.Merge(b)` lays
the set fields of `b` over `a`, so a heading is `Text(s).Style(t.Title)` and a
one-off tweak is `Text(s).Style(t.Title).Color(red)`.

### Inherited styles

Every widget lays out under an `Env`
that flows down from the root, like CSS inheritance. `Styled(child)` sets the
text style everything below starts from, and a `Text`'s own setters still win:

```go
ggui.Styled(ggui.Column(ggui.Text("a"), ggui.Text("b").Size(18))).Color(t.Muted).Size(12)
```

The root `Env` starts from the theme's `Text`, so a bare `Text(s)` already
looks right; `Title(s)` and `Caption(s)` take the theme's named styles from
the Env at layout.

`.Space(n)` on Column, Row, Wrap, Grid and For is n times
the theme's `Space`, and `Themed(t, child)` gives a subtree its own theme,
so a Builder rarely needs `UseTheme` at all. Inheritance happens at layout time, so it works with the eager
construction of Go: no closures around subtrees. Your own inherited values go
the same way: `Provide(key, v, child)` stores `v` under a `Key[T]` from
`NewKey`, and a widget reads it back with `env.Get(key)` in `Layout`.

### Theme tokens

`Theme`'s colors follow shadcn/ui's semantic tokens, so a palette written
for shadcn ports across a variable at a time: `Bg`/`Fg`, `Card`, `Popover`,
`Primary`/`PrimaryFg` (with `PrimaryHover`), `Secondary`/`SecondaryFg`,
`Muted`/`MutedFg`, `Destructive`/`DestructiveFg`, `Border`, `Input`, `Ring`,
plus `Selection` and the modal `Scrim`. Each pair is a surface and the
foreground drawn on it: `Muted` is the quiet surface behind a hover, and
`MutedFg` the grey of secondary text.

Elevation is three tokens in the order a surface rises off the page:
`CardShadow`, `PanelShadow` and `OverlayShadow`. Sizes are `Radius` with
`RadiusSm` for rows and pills and `RadiusLg` for cards, dialogs and toasts,
plus `Space`, `BorderWidth` and `MenuWidth`. The controls' paddings are
`ButtonPad`, `FieldPad`, `ItemPad`, `CardPad`, `PanelPad` and `TabPad`, and
the state tints are `HoverMix`, `PressMix` and `DisabledMix`, so a custom
control can match the built-in ones.

`t.Set(key, v)` still adds a token of your own under a `Key`, without
changing `t`, and `t.Get(key)` reads it back.

Two `Env` keys support accessibility preferences: `Provide(TextScaleKey, 1.5, tree)` scales every
`Text` and `TextInput`, and `Provide(ReducedMotionKey, true, tree)` lands
transitions at once and makes the controls' eased motions jump, through
`env.Motion(d)`.

`UseTheme()` reads it at build time and subscribes the enclosing Builder;
`SetTheme(t)` swaps it and rebuilds only what read it. `DefaultTheme()` is
light, `DarkTheme()` dark, and a window with no `Background` follows the
theme's `Bg`. The theme also travels in the `Env`, where `env.Theme()` gives a
custom widget the tokens at layout time, the way the built-in controls get
theirs. `Box` decorates with `.Fill`, `.Radius(r)` and `.Border(w, c)`.

## Input

Interactive widgets register the `Rect` they painted as a hit region on the
`Canvas`. Each frame the runtime routes the pointer to the topmost region
under it, and a region that does not handle an event lets it fall through to
the one beneath, so a tap target does not block scrolling.

```go
hovered := ggui.State(false)
button := ggui.Pointer(ggui.Box(ggui.Text("+")).Pad(6, 16)).
	OnTap(func() { ggui.Add(count, 1) }).
	OnHover(hovered.Set)
```

### Scrolling

`Scroll(child)` gives its child `Unbounded` height (or width, with
`.Horizontal()`), shows a window onto it, moves that window with the wheel and
clips both drawing and hit regions to the window. The offset carries across
a rebuild; `.Offset(sig)` binds it to a `Signal[float64]` for programmatic
scrolling; `.Speed(px)` and `.Bar(color)` tune it. Widgets that fill their space
fall back to their content size on an unbounded axis, so `Center`, `Expanded`
and `.Justify` inside a `Scroll` do not blow up.

### Pointer and keyboard events

`Pointer` has `OnTap`, `OnDown`, `OnUp`, `OnMove`, `OnDrag`, `OnEnter`,
`OnExit`, `OnHover` and `OnScroll`, and `.Cursor(shape)` sets the mouse
cursor over it; `Tap(child, fn)` is the one-callback shortcut. A tap is a
press and a release inside the same region, matched by `Rect`, so a tree
rebuilt in between still completes it. The region that took a press captures
the pointer: it gets `OnDrag` every frame until the release, wherever the
cursor went, which is what a slider or a text selection needs.

`Focus(child)`
takes keyboard focus when clicked and delivers `OnKey`, `OnText` and
`OnFocus`; a held key repeats, and `KeyEvent.Mods` carries Shift, Ctrl, Alt
and Meta (`Mods.Cmd()` is ⌘ on macOS and Ctrl elsewhere). Tab and Shift+Tab
move focus through the key regions in paint order; focus that arrived that
way is reported with `Key` set to `KeyTab`, which is when the controls draw
a focus ring, and a `Scroll` around the new target scrolls it into view.
Buttons press on Space or Enter, toggles flip, sliders step with the
arrows.

### Shortcuts

Shortcuts are chords: `app.Shortcut("cmd+s", save)` runs before the
focused widget and takes the key from it, as every chord with a modifier
does. A bare key such as `"space"` reaches the focused widget first and runs
the shortcut only when the widget did not consume it, so Space on a focused
button presses the button and a text field keeps every key but Escape;
`.Exclusive()` on the handle makes a bare key run first too. A widget says
what it consumes through `KeyConsumer`; `Interactive` claims Space and
Enter and the controls with more keys claim those. `ParseChord` reads the
names, `KeyEvent.Is(chord)` matches one in a handler, and `App.OnKey` stays
for what a chord cannot say.

### Focus scopes

An open `Popup` and a `ui.Dialog` paint their content
through `dst.FocusTrap`: while it shows, Tab cycles inside it, focus is
moved in when it opens and returned to the opener when it closes, and an
Escape the focused widget did not consume closes it. `ui.Dialog(open,
content)` is a centered modal on a scrim that takes the clicks, with
`.Title`, `.Width` and `.OnClose`.

### Roles and labels

Every control carries a `Role` and a name: a button's text, a
checkbox's label, a field's `Label` or placeholder; `ButtonOf`, `Slider`
and `Select` take one through `.Label` or `.Named`. The inspector shows
them, and tests find controls by them.

### Custom input handlers

To make your own widget interactive, implement `PointerHandler` or
`KeyHandler` and call `dst.HitPointer(r, w)`, `dst.HitKey(r, w)` or
`dst.HitCursor(r, shape)` from `Paint`; calls for the same `Rect` merge into
one region. A `KeyHandler` that also implements `TickHandler` runs once per
frame while focused, which is how `TextInput` drives the IME. `dst.Clip(r)`
returns a Canvas that draws and registers regions only inside `r`,
`dst.Pointer()` is where the cursor is, and `dst.Overlay(fn)` paints above
the tree once it is done.

### Keeping interaction state across rebuilds

A rebuild replaces widgets, and with them the state they hold. A handler that
implements `Adopter` is handed the handler that held the same region in the
previous frame as it registers its own, so it can copy hover, press, a caret
or an animation in flight: the built-in controls, `TextInput`, `Scroll` and
`Popup` all do, which is why flipping a `ui.Switch` that rebuilds its own
subtree still slides the knob. The region is matched by the handler's
identity when it implements `Identified`, else by `Rect`; `.Key(k)` on a
control, `TextInput`, `Scroll` or `Popup` sets one, so a widget rebuilt and
moved in the same frame keeps its state. Inside a `Keyed` or `Mount`
component every one of those gets an identity for free, the component's
mount instance plus its place in construction order, so a keyed form keeps focus and
carets through its own rebuilds with no keys on the fields.

`ggui.Interactive` is the shared body of a
control: embed it, call `Hit` from `Paint` and `Pointer` and `Keyboard` from
the handlers, and hover, press, focus, the focus ring and adoption come with
it; `ui` is built on it.

## Accessibility

**The native accessibility bridge currently supports macOS only.** It exposes
widget semantics to VoiceOver and other macOS assistive tools. Windows and Linux
bridges are not yet implemented.

Set `Config.Accessibility` when creating the app:

| Mode | Behavior |
| --- | --- |
| `AccessibilityAuto` | Default: activate while assistive technology is attached. |
| `AccessibilityAlways` | Keep the bridge active, including for Accessibility Inspector. |
| `AccessibilityOff` | Disable the native bridge. |

Give controls meaningful labels. Buttons use their text, checkboxes use their
labels, and fields use a label or placeholder; custom content may need `.Named`
or `.Label`. Roles and names also help the inspector and `Probe` identify widgets.

Apply text scaling or reduced motion through inherited values:

```go
ggui.Provide(ggui.TextScaleKey, 1.5,
	ggui.Provide(ggui.ReducedMotionKey, true, tree),
)
```

Text scaling affects `Text` and `TextInput`. Reduced motion makes transitions and
control motion complete immediately. Custom controls can respect it through
`env.Motion(d)`.

## Testing

`Probe` drives layout and input without opening a window. It uses the app's frame
steps and hit regions, so tests exercise the same interaction routing.

This complete test checks a checkbox by its accessible label:

```go
package example_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestCheckbox(t *testing.T) {
	on := ggui.State(false)
	p := ggui.NewProbe(ui.Checkbox(on, "Enable alerts"), ggui.Sz(240, 40))
	defer p.Close()

	p.Tap("Enable alerts")
	if !on.Peek() {
		t.Fatal("expected alerts to be enabled after tapping the checkbox")
	}
}
```

| API | Use it for |
| --- | --- |
| `NewProbe(widget, size)` | Testing a widget constructed in advance. |
| `ProbeBuilder(build, size)` | Testing a reactive builder, as with an app. |
| `Tap(label)`, `Find(label)`, `FindRole(role, label)` | Locating controls by semantics. |
| `Click`, `Press`, `Move`, `Release`, `Scroll` | Pointer interaction. |
| `Type`, `Text` | Key events and text entry. |
| `Advance(duration)` | Advancing the test clock for animation. |

> [!IMPORTANT]
> Probes share the reactive runtime, theme, and clock. Run probe tests serially
> (do not call `t.Parallel`) and always call `Close`, usually with `defer`.

See the [counter tests](examples/counter/main_test.go) for a full app example.

## HiDPI

Widgets work in logical pixels; the screen is allocated at the monitor's
device scale factor so a Retina display gets a sharp image. `dst.Scale()`
returns the factor, and drawing goes through it: `dst.FillRect`,
`dst.FillRoundRect`, `dst.StrokeRoundRect`, `dst.FillCircle` and
`dst.StrokeLine` take logical geometry, `dst.Px(v)` converts a length,
`dst.Geo(at)` is the transform for `DrawImageOptions`, and text rasterizes
its face at the scaled size rather than scaling the pixels. A custom widget
that draws with Ebitengine directly should do the same.

## Custom widgets

Implement `Layout` and `Paint`. `Layout` receives the
`Env` to pass on to children unchanged; `Paint` receives the `Rect` to draw
in, so a leaf widget stores nothing between the two calls and a container
remembers only where its children go. A container paints its children with
`dst.Paint(child, r)` rather than `child.Paint(dst, r)`, so the inspector
sees them. `FromFuncs` wraps two closures when a named type is overkill:

```go
dot := ggui.FromFuncs(
	func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(8, 8)) },
	func(dst *ggui.Canvas, r ggui.Rect) {
		vector.DrawFilledCircle(dst.Image, dst.Px(r.Origin.X+4), dst.Px(r.Origin.Y+4), dst.Px(4), fg, true)
	},
)
```

## Frames

### Frame lifecycle

Every frame the runtime routes input, steps animations, flushes effects and
paints. It lays the tree out only when something could have moved: the root
was rebuilt, the window changed size, a `Signal` was written, or
`Invalidate` was called. Hover and press live outside signals and only
change how a widget paints, so a still frame costs a paint and nothing
else.

A custom widget that keeps size-affecting state outside signals calls
`Invalidate(env)` when that state changes; `Scroll` does for its offset. A
`Scroll` also tells its subtree the window it shows through the `Env`
(`ScrollViewport(env)`), which is how `For` virtualizes.

### Layout caching

`Cached(child)` narrows the skip to a subtree: it returns its last size
while the constraints and everything inherited through the `Env` are
unchanged and nothing inside asked for a layout. `Reactive`, `For`, `Scroll`
and `TextInput` ask when they change; a custom widget whose size depends on
state outside a signal calls `Invalidate(env)` with the Env it was laid out
under. Wrap the panels that do not change together in it.

### Retained paint state

State that has no signal and must outlive a rebuild can be kept on the
Canvas under a typed `Slot`: `dst.Retain(anchor, slot, v)` stores a value
for the next frame and `dst.Retained(anchor, slot)` reads what was stored
last frame, where the `Anchor` is the widget's ID or its Rect. `dst.Ease`
is a `Motion` kept that way. Tooltip keeps its hover timer and Transition
its start time in slots.

## Inspector

`Config{Inspector: ebiten.KeyF1}` binds a key that toggles a development
overlay in the shape of a browser's element panel; `App.Inspector(on)` does
the same from code. It is the quickest way to see why something sits where
it does.

Every widget painted through `Canvas.Paint` is outlined and colored by
depth, and the tree of them is listed down the right-hand side with each
one's size. Moving the pointer selects the innermost widget under it, scrolls
the tree to that row, and describes it underneath: its type, its size and
position, its depth, and the accessibility node there with everything it sits
inside of.

Clicking a row pins the selection, so it survives moving the pointer away —
the only way to read anything about a widget that exists only while hovered.
Clicking the header goes back to following the pointer, and the wheel scrolls
the tree. The panel takes only the pointer events that land on it, so the app
underneath keeps working while the inspector is open.

## Repository layout

| Area | Source |
| --- | --- |
| Application and frame loop | [app.go](app.go), [loop.go](loop.go) |
| Reactivity and component ownership | [signal.go](signal.go), [widget.go](widget.go), [for.go](for.go) |
| Layout, geometry, and drawing | [widgets.go](widgets.go), [geometry.go](geometry.go), [canvas.go](canvas.go) |
| Themed controls | [ui/](ui/) |
| Text editing and fonts | [editor.go](editor.go), [font.go](font.go), [internal/textinput/](internal/textinput/) |
| Input and shortcuts | [input.go](input.go), [chord.go](chord.go), [clipboard.go](clipboard.go) |
| Accessibility and semantics | [a11y.go](a11y.go), [a11y_darwin.go](a11y_darwin.go), [semantics.go](semantics.go) |
| Styling and animation | [style.go](style.go), [anim.go](anim.go), [transition.go](transition.go) |
| Overlays and images | [popup.go](popup.go), [tooltip.go](tooltip.go), [image.go](image.go) |
| Testing and diagnostics | [probe.go](probe.go), [inspector.go](inspector.go), [cache.go](cache.go) |
| Runnable applications | [examples/](examples/) |

---

[Back to top](#ggui-guide) · [README](README.md) · [Development commands](README.md#development)


### GPU shadows

`Box.Shadow(styles...)` and `ui.Card(...).Shadow(styles...)` paint outer shadows
before the surface. Multiple styles form layers; calling `.Shadow()` clears them.
For custom drawing, use `canvas.Shadow(rect, cornerRadius, style)`.

```go
ui.Card(content).Shadow(ggui.ShadowStyle{
    Offset: ggui.Pt(0, 6),
    Blur:   12,
    Spread: 0,
    Color:  color.NRGBA{A: 50},
})
```

All distances are logical pixels and scale with the display. Positive spread
expands the silhouette; negative spread contracts it. Nil or transparent colors
skip rendering. Blur is a smooth feather distance on both sides of the edge;
zero gives a sharp, antialiased shadow. This is a rounded-rectangle distance-field
approximation, not a Gaussian blur of the content or image alpha.

The renderer lazily shares one Ebitengine Kage shader. Each visible shadow layer
uses one `DrawRectShader` call, with no intermediate textures, blur passes or CPU
rasterization. Draw bounds are intersected with the target before rendering.
Cost still grows with visible pixel area and overlapping layers.

Shadows do not reserve layout space or create hit regions. Add padding/gaps when
needed; parent clipping and window bounds still clip them. The gallery's Shadows
preview compares subtle, floating and colored treatments. Toast uses this same
renderer, including its existing fade animation.


### Component appearance

The control set follows the visual hierarchy of [shadcn/ui's semantic theme
colors](https://ui.shadcn.com/docs/theming), [segmented tabs](https://ui.shadcn.com/docs/components/tabs)
and [cards](https://ui.shadcn.com/docs/components/card), adapted to native drawing
and ggui's existing APIs.

| Element | Appearance and configuration |
| --- | --- |
| Buttons | Primary by default; `Outline()` draws a border around the background. `Secondary()` adds a subdued fill, `Ghost()` removes the resting surface, and `Destructive()` uses the theme's `Destructive` color. Variants preserve pointer, keyboard and accessibility behavior. |
| Focus | Controls use a separate, softer focus color. Text fields add an outer halo while editing. |
| Tabs | A muted rounded strip with an animated raised selection; `.Line()` opts into the underline treatment. Reduced-motion settings still apply. |
| Cards and floating panels | Cards receive a subtle shadow; menus, select lists, comboboxes and date pickers use a stronger shared panel shadow. Dialogs use a larger radius and deeper elevation. |
| Labels and notices | Field labels use the body size; help text remains smaller. Alerts use body-size descriptions and tighter title spacing. |
| Badges and calendar | Badges use small rounded corners. Calendar month navigation uses ghost buttons, with a centered month heading and contrasting selected-date text. |

These choices follow a brand by changing the theme rather than the widgets.
The tokens the control set reads are ordinary `Theme` fields, so a derived
theme is a struct literal away.

```go
theme := ggui.DefaultTheme()
theme.Ring = color.NRGBA{R: 140, G: 165, B: 230, A: 255}
theme.PanelShadow = ggui.ShadowStyle{Offset: ggui.Pt(0, 4), Blur: 12, Color: color.NRGBA{A: 45}}
// Disable default card elevation globally, or call Card(...).Shadow() locally.
theme.CardShadow = ggui.ShadowStyle{}
ggui.SetTheme(theme)
```


### Command and menu presentation

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
`.Named("...")` on the returned dialog to customize its accessible name.

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

Navigation uses restrained selection styles: Pagination outlines only the current
page and uses ghost buttons for other pages, with ellipses indicating hidden
ranges. Its current page is also exposed as selected to accessibility clients.
Accordion places its chevron on the right, aligns header and body text, and adds
space beneath expanded content. Tabs uses a compact segmented strip; wrap the
content in a Card only when a separate content surface is needed. Menubar and
ContextMenu share the same subdued floating-panel shadow.
