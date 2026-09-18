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
| Customize appearance and motion | [STYLING.md](STYLING.md) · [Animation](#animation) |
| Handle keys, pointer input, and focus | [Input](#input) |
| Support assistive technology | [Accessibility](#accessibility) |
| Update state from a goroutine | [Threads](#threads) |
| Verify behavior without a window | [Testing](#testing) |
| Extend or debug the renderer | [Custom widgets](#custom-widgets) · [HiDPI](#hidpi) · [Frames](#frames) · [Inspector](#inspector) |
| Navigate the implementation | [Repository layout](#repository-layout) |

> [!IMPORTANT]
> APIs are under active development. Native accessibility support is currently
> **macOS only**; Windows and Linux accessibility bridges are not implemented.

## Concepts

### Signals

`State(v)` creates a `*StateValue[T]`. `Get()` tracks a read inside a
reactive computation; `Set(v)` and `Update(func(T) T)` publish changes.
`Untrack(value.Get)` reads the current value without subscribing.

```go
count := ggui.State(0)
count.Update(func(n int) int { return n + 1 })
label := ggui.Textf("Count: %d", count)
```

State is shallow: assigning `items.Get()[0].Name` does not publish a change.
Copy slices/maps before changing them, then call `Set` or `Update`.
`Append`, `Remove`, `Add`, and `Toggle` work with `Writable` values.

Equality uses a type's `Equal(T) bool` method, otherwise `==` for comparable
non-interface types. Other types notify on every write. `WithEqual(fn)`
customizes comparison; `WithEqual(nil)` always notifies. Equality functions
must be pure. Values with mutable references need immutable snapshots for
meaningful comparisons.

### Derived values

`Derived(func() T)` covers both Svelte `$derived` and `$derived.by`:

```go
doubled := ggui.Derived(func() int { return count.Get() * 2 })
total := ggui.Combine(price, qty, func(p float64, n int) float64 {
    return p * float64(n)
})
```

A `*DerivedValue[T]` computes on its first read and the next read after an
input changes. Unread values do not run during a frame flush. Dependencies
are recollected on each computation; `Get()` settles upstream values before
returning. `Untrack(total.Get)` also returns the latest value.

Derived calculations must be pure: state writes and recursive derived cycles
panic. `WithEqual` controls downstream notification, including slice results.
`Map`, `StateValue.Map`, and `DerivedValue.Map` are convenience derivations.
A derived value belongs to its creation owner; disposing that owner stops it.
Unowned derivations must be explicitly disposed. A disposed value retains its
last computed result (the zero value if it was never read).

### Ownership and cleanup

Create local state and work in `Component` setup. `Effect` requires an owner
and runs after mount and layout, before paint. It returns a disposer and its
callback returns a cleanup, or `nil`:

```go
ggui.Effect(func() ggui.Cleanup {
    room := roomID.Get()
    return subscribe(room)
})
```

Cleanup runs untracked before another execution and on disposal. Multiple
writes before the next update are coalesced. An effect disposed before its
first execution never runs. `Watch(source, fn)` has the same deferred timing.
Use `Derived` for computed state rather than copying it through effects.

`OnCleanup(fn)` attaches cleanup directly to the current owner. `Root(fn)`
creates a scope that survives its enclosing computation's reruns; it ends
when its disposer is called or its parent is disposed. Disposers are
idempotent. For app-wide effects, use `App.Setup` or `Probe.Setup`:

```go
app.Setup(func() {
    ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme())
})
```

### Threads

State reads/writes, derived calculations, effects, construction and layout
belong to the UI goroutine. `Untrack` changes dependency collection, not
thread safety. A worker receives an immutable input snapshot and posts its
result with `App.Post`. Inside a mounted component or app setup,
`UIThread()` returns the owner's dispatcher. Closing an app drops queued
work; component-specific work must additionally check its lifetime, as
`Resource` does automatically. Debug builds (`-tags ggui_debug`) diagnose
reactive operations from the wrong goroutine while the app is running.

### Readers, bindings, and lenses

| Interface | Methods | Use |
| --- | --- | --- |
| `Readable[T]` | `Get()` | Display-only data, including derived values. |
| `Binding[T]` | `Get()`, `Set(T)` | Two-way controls and animated values. |
| `Writable[T]` | `Binding[T]`, `Update(func(T) T)` | Immediate read-modify-write helpers. |

`Field` and `Lens` bind controls to parts of a state value:

```go
form := ggui.State(Form{Name: "Ada"})
name := form.Field(func(f *Form) *string { return &f.Name })
field := ui.TextField(name)
```

`Field` copies the struct, not the nested objects it points to. Use `Lens`
with explicit copy logic for nested mutable containers. Tweens and springs
implement `Binding`, not `Writable`; use `Target()` when updating a motion's
destination rather than its current animated value.

### Builders and components

`Component(func() Widget)` runs its setup once per mount, without tracking
setup reads. The app's root constructor also runs once. Bind values to
widgets or use explicit reactive blocks for subsequent changes:

```go
func Counter() ggui.Widget {
    return ggui.Component(func() ggui.Widget {
        count := ggui.State(0)
        return ggui.Column(
            ggui.Textf("Count: %d", count),
            ui.Button("Increment", func() { ggui.Add(count, 1) }),
        )
    })
}
```

`Text(count.Get())`-style snapshots do not subscribe setup. `TextOf`, `Textf`
and reactive control bindings do. `View(source, build)` and `Reactive(build)`
are explicit subtree replacement boundaries: their callbacks rerun and
replace locally created state/work. Keep state outside these callbacks if
it must survive their updates. Svelte's snippet `{@render}` has no special
runtime counterpart; use ordinary Go functions to compose reusable widgets.

### Conditional and key blocks

```go
ggui.If(signedIn, func() ggui.Widget {
    return ProfileScreen()
}).Else(func() ggui.Widget {
    return LoginScreen()
})

ggui.Key(documentID, func(id string) ggui.Widget {
    return Editor(id)
})
```

`If` creates only the active branch; `.ElseIf` and `.Else` add branches.
Leaving a branch disposes its state, effects and resources. Returning creates
a fresh instance. Reads in branch factories are untracked; use bindings or
`View` inside them. `Key` recreates its subtree when its comparable key changes.
Configure fluent blocks before mount; later configuration panics.

The keyboard key type is `KeyboardKey`; constants such as `KeyEnter` and the
`KeyEvent.Key` field retain their names. Widget `.Key(id)` setters still assign
input identity and are distinct from the `Key` control-flow block.

### Keyed lists

`Each(items, row)` reuses rows by position and permits duplicate values.
`EachKeyed(items, key, row)` preserves row identity across reordering:

```go
ggui.EachKeyed(todos, func(t Todo) int { return t.ID },
    func(row ggui.EachItem[Todo]) ggui.Widget {
        return ggui.View(row.Value, func(t Todo) ggui.Widget {
            return ggui.Text(t.Title)
        })
    },
).Else(func() ggui.Widget { return ggui.Text("No items") })
```

Each row gets reactive `Value` and `Index`. Its factory runs once per row
instance. Duplicate keys panic before any row update is applied. Empty-list
branches have their own lifetime. These blocks are layout widgets (vertical
by default), not fragments spliced into their parent's children.

`Gap`, `Space`, `Align`, and `Horizontal` configure layout. `ItemExtent` inside
`Scroll` virtualizes fixed-size rows; `Retain(n)` bounds offscreen instances.
Evicted rows lose local state and are recreated when visible; focused or
captured rows are retained. `Transition` preserves exiting rows until their
animation completes; virtualization removes rows immediately.

### Resources and await blocks

`Resource(input, load, options...)` tracks input on the UI goroutine and runs
`load(context.Context, input)` on a worker. Create it in app setup or a mounted
component. Copy mutable input references before returning them from `input`.

```go
users := ggui.Resource(
    func() string { return query.Get() },
    func(ctx context.Context, q string) ([]User, error) {
        return api.SearchUsers(ctx, q)
    },
)

view := ggui.Await(users).
    Pending(func() ggui.Widget { return ggui.Text("Searching…") }).
    Then(func(value ggui.Readable[[]User]) ggui.Widget {
        return ggui.EachKeyed(value, userID, userRow)
    }).
    Catch(func(err ggui.Readable[error]) ggui.Widget {
        return ggui.Textf("Search failed: %v", err)
    })
```

Input changes cancel the prior context. Request IDs and disposal checks
prevent old completions from committing, even when a worker ignores
cancellation. `Reload()` retries with the current input. `ResourceEqual(fn)`
customizes input equality; `nil` restarts on every input notification.
Resources execute independently of consumers; multiple `Await` blocks share
one task. Unmounting a consumer does not cancel a resource owned elsewhere.

`Await` accepts any `Readable[AsyncState[T]]`. A snapshot has `RequestID`,
`Status` (`Pending`, `Ready`, `Failed`), `Value`, and `Err`. Zero values are
valid successful results. Internally canceled requests do not display errors.
Omitted branches are empty. Within one request and status, the branch and its
reactive value are preserved; a new request creates a new branch even when
its intermediate pending state is not painted. Retaining stale results while
refreshing is not part of this API.


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
`StateValue[bool]`; `.Keys(h)` keeps keyboard focus on the widget that opened it
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
`.NoWrap()` adjust it; what is not set is inherited (see [STYLING.md](STYLING.md)). The
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
	ui.Radios(plan, []string{"free", "pro"}).Format(strings.ToTitle),
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

Every control widget with `Disabled` also supports `DisabledWhen`. The last
setting wins: `Disabled(false)` removes a previous `DisabledWhen` binding;
`DisabledWhen(busy)` replaces the static setting. Composite controls evaluate
their own binding and apply the result to their internal controls. Disabling a
popup control also closes its popup. Item configuration values such as
`CommandEntry` keep their static `Disabled` option.

Use `.Named(name)` for a control's accessible and Probe name. `Field` supplies
its label only when the control has no explicit name, taking precedence over a
placeholder or built-in fallback. Labels passed to constructors such as
`Button("Save", save)` are explicit names. Custom controls can participate by
implementing `ui.Named` (`SetName(string)` and `HasName() bool`); `HasName` must
exclude placeholders and fallback names. `Semantics` reports the resolved name.
Use `.Format(fn)` for option text on Select, Combobox, Radios, and ToggleGroup,
and `.RowName(fn)` for a table row's accessible name.

| `.OnCommit(fn)` | Observe completed editing on sliders and text fields. |
| `.Pad(...)` | Override padding on controls such as buttons. |

Controls obtain colors and spacing from the inherited theme. Hover and press
state stay in the widget; animated details retain their motion across rebuilds.
Animations use `ggui.Now()`: the frame's instant, sampled once per frame so
everything animating agrees on the time, and moved on by at most 100ms per
frame so a window that was hidden resumes instead of jumping. Read it rather
than `time.Now` in a `Paint`. `ggui.SetClock` replaces the source in tests,
and `Probe.Advance(d)` steps a headless frame by exactly `d`.

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

`Text` and `TextInput` render color emoji as part of ordinary strings. Emoji
presentation selectors, skin tones, regional-indicator flags, keycaps and ZWJ
sequences stay together when shaping, wrapping, moving the caret or deleting.
Native apps use `SystemEmojiFont()` when available. For a predictable offline
font, including WebAssembly:

```go
import "github.com/ironpark/ggui/fonts/notoemoji"

notoemoji.Enable() // before building the app
label := ggui.Text("Hello 👋🏽 · 🇰🇷 · 👩🏽‍💻 · 1️⃣")
```

The optional Noto package embeds approximately 10.7 MB of font data; apps that
do not import it do not embed it. `SetEmojiFont(font)` selects a caller-owned
CBDT/CBLC, sbix, COLRv0 or OpenType SVG font without replacing the text font.
`SetEmojiFont(nil)` disables substitution; `SetEmojiFont(SystemEmojiFont())`
restores the platform font. `Font.NoFallback()` opts that font out as well.
Actual glyph coverage depends on the selected emoji font. Explicit text
presentation (VS15) stays in the ordinary font; emoji presentation (VS16) uses
the emoji font. Color glyphs retain their colors when text color changes.

### Containers and feedback

| Control | Behavior |
| --- | --- |
| `ui.Tabs(selected, ui.Tab("One", page), ...)` | Displays the selected index, with an animated segmented selection and Left/Right navigation; `.Line()` uses an underline. Only the active page is laid out. |
| `ui.Collapsible(open, "Title", content)` | Animates an expandable section with `Presence`. |
| `ui.Card(child)` | Adds a surface, border, radius, padding and subtle shadow. |
| `ui.Badge("new")` | Displays a small label; `.Accent()` emphasizes it. |
| `ui.Progress(value)` | Eases toward a fraction from a `Readable[float64]`. |
| `ui.Dialog(open, content)` | Shows a modal while the binding is true; see [focus scopes](#focus-scopes). |

### Tables

`ui.Table(rows, key, cols...)` is a keyed list of rows under a
heading row: `ui.TextCol(title, func(T) string)` is a text column that
follows its item, `ui.Col(title, func(Readable[T]) Widget)` holds any
widget, and `.W(px)`, `.Grow(flex)` and `.Right()` size and align a
column. `.Selected(binding)` highlights the row whose key the binding
holds and sets it on a click or Space, `.OnSelect(fn)` gets the item,
`.Height(h)` scrolls the body under a fixed heading and lays out only the
rows in view, and `.RowName(fn)` names rows for `Probe.Find`.

```go
ui.Table(people, func(p Person) int { return p.ID },
	ui.TextCol("Name", func(p Person) string { return p.Name }),
	ui.TextCol("Age", func(p Person) string { return strconv.Itoa(p.Age) }).W(60).Right(),
).Selected(chosen).Height(240)
```

### Select and menu

`ui.Select(value, options)` is a dropdown bound to a
signal, labelled through `fmt.Sprint` or `.Format(fn)`: a click
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
`.Disabled(true)` closes and disables the picker. DatePicker owns the disabled
state of the Calendar returned by `.Calendar()`; configure `Disabled` or
`DisabledWhen` on the picker, not that internal calendar. `.Key(key)` preserves its
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
).Header(ggui.Title("Acme")).Collapsed(narrow) // narrow is a Readable[bool]
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
several, bound the way `ui.Select` and `ui.Radios` are. `.Format(fn)` sets how
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

`InputGroup.Disabled` and `DisabledWhen` disable the editor as well as its
chrome. Leading and trailing addon controls remain independent. The editor's own
settings are preserved: it is disabled if either it or its group is disabled.
Custom containers can pass `ggui.InputDisabled` through `Env` or `Provide`;
TextInput combines that inherited value with its own settings. A descendant
`false` cannot clear an ancestor's `true`. Disabled editors remain in the
accessibility tree and reject pointer, keyboard, and accessibility edits.

## Animation

`Tween(v, d)` and `Spring(v)` are values that move toward their target over
time instead of jumping, after Svelte's `tweened` and `spring`. Both are
`Readable`s, so an island that reads one rebuilds every frame the value moves:

```go
width := ggui.Tween(0.0, 200*time.Millisecond).Easing(ggui.EaseOut)
bar := ggui.Reactive(func() ggui.Widget {
	return ggui.Box().Size(width.Get(), 4).Fill(accent)
})
width.Set(120) // slides there over 200ms; Jump(v) skips the motion
```

Animations created inside an owner stop when it is disposed and keep their
last value; `Set` and `Jump` on those disposed values do nothing. Create them
in component setup or `App.Setup` to keep them across builder reruns. An
animation created without an owner runs until it settles or `Jump` stops it.

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

Styling has three layers: a widget's own setters, values inherited through the
`Env`, and the theme's tokens. A widget's setters win over what it inherited,
and inheritance is resolved at layout time, so a theme swap reaches widgets
built long before it.

```go
ggui.Text("Heading").Style(t.Title).Color(brand)   // local
ggui.Styled(page).Color(t.MutedFg).Size(12)        // inherited
ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme())
```

Use `ui.ThemeSwitch(dark)` for a compact day/night control: the large sun thumb turns into
a softly shaded full moon, with clouds fading into stars. `true` means dark
mode. Bind the same signal with `BindTheme` as above to apply the theme. The
control supports `.Named("Appearance")`, `.OnChange(fn)`, `.Disabled(v)`, and
`.DisabledWhen(reader)`, plus Space/Enter and reduced-motion preferences.

**[STYLING.md](STYLING.md) is the full reference**: every theme token with its
shadcn/ui variable and its light and dark value, how a `TextStyle` resolves,
deriving a theme without the zero-field trap, tokens of your own, styling a
custom widget, and the two accessibility preferences.

## Icons

`ui.Icon(icons.Search)` uses a shared semantic placeholder backed by embedded
Lucide SVGs. Use `lucide.Icon("download")` to request an explicit bundled icon.
Both support `.Size(20)`, `.Color(col)`, and `.Alt("Download")`.

Provide `icons.SetKey` through `ggui.Provide` for a subtree, or store it in a
`Theme` with `.Set(icons.SetKey, set)` to replace built-in control icons. Sets
map roles such as `Check`, `Close`, and `ChevronDown` to parsed SVGs. Missing
roles retain the Lucide defaults. Decorative icons stay out of accessibility;
name the button that contains them, or use `Alt` for a standalone image.

See [icons/README.md](icons/README.md) for custom SVG sets, precedence and caching.

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
a rebuild; `.Offset(sig)` binds it to a `StateValue[float64]` for programmatic
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

Keys, cursor shapes and mouse buttons carry ggui's own names: `ggui.KeyTab`,
`ggui.CursorShapePointer`, `ggui.MouseButtonRight`. They are aliases for
Ebitengine's, so they are the same values of the same types and an
`ebiten.Key` still works wherever one is wanted; what they buy is that a
widget, or an app, imports `ggui` alone.

### Focus scopes

An open `Popup` and a `ui.Dialog` paint their content
through `dst.FocusTrap`: while it shows, Tab cycles inside it, focus is
moved in when it opens and returned to the opener when it closes, and an
Escape the focused widget did not consume closes it. `ui.Dialog(open,
content)` is a centered modal on a scrim that takes the clicks, with
`.Title`, `.Width` and `.OnClose`.

### Roles and labels

Every control carries a `Role` and a name: a button's text, a
checkbox's label, a field's `Named` or placeholder; `ButtonOf`, `Slider`
and `Select` take one through `.Named`. The inspector shows
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
moved in the same frame keeps its state. Inside a `Component`, keyed row, or `Key` branch every one of those gets an identity for free, the component's
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
labels, and fields use a label or placeholder; custom content may need `.Named`.
Roles and names also help the inspector and `Probe` identify widgets.

Apply text scaling or reduced motion through inherited values:

```go
ggui.Provide(ggui.TextScaleKey, 1.5,
	ggui.Provide(ggui.ReducedMotionKey, true, tree),
)
```

Text scaling affects `Text` and `TextInput`. Reduced motion makes transitions and
control motion complete immediately. Custom controls can respect it through
`env.Motion(d)`; see [STYLING.md](STYLING.md#accessibility-preferences).

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
	if !ggui.Untrack(on.Get) {
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

Every frame the runtime routes input and steps animations, settles internal
bindings and mounts, stabilizes layout, runs user effects, and then paints.
Effect writes trigger another stabilization before paint. It lays the tree out only when something could have moved: the root
was rebuilt, the window changed size, a `StateValue` was written, or
`Invalidate` was called. Hover and press live outside signals and only
change how a widget paints, so a still frame costs a paint and nothing
else.

A custom widget that keeps size-affecting state outside signals calls
`Invalidate(env)` when that state changes; `Scroll` does for its offset. A
`Scroll` also tells its subtree the window it shows through the `Env`
(`ScrollViewport(env)`), which is how `EachKeyed` virtualizes.

Effects are flushed until they are quiet. An effect that writes a signal it
reads, directly or through other effects and memos, never is: the frame gives
up, `App.Run` returns `ErrCycle` and `Probe` panics with it. Match it with
`errors.Is(err, ggui.ErrCycle)` and print the error for the detail, which says
how many effects the cycle turns and how many there were in all. Build with
`-tags ggui_debug` and each is named by the line that created it:

```
ggui: effects did not settle after 16 passes; an Effect is writing a StateValue it reads
  2 of 15 effects never settled
    - /src/app/total.go:31 (a derived value)
    - /src/app/cart.go:64
```

Break the loop with `Untrack` on the read that should not
subscribe.

### Layout caching

A rebuild boundary is a layout boundary. `Component`, `Reactive`, and `Key` each cache their subtree's size: they return it unchanged while
the constraints and everything inherited through the `Env` are the same as
last time, layout input versions match, and nothing inside asked for a layout.
Measurement dependencies include untracked reads without adding reactive
subscriptions. A signal write in one panel
therefore measures that panel, not the whole window, even though the runtime
lays out from the root whenever anything was written.

`Cached(child)` is the same cache without a rebuild boundary, for a static
subtree that sits under something that does rebuild.

`EachKeyed`, `Scroll` and `TextInput` ask for a layout when they change. A custom
widget whose size depends on state outside a signal **must** call
`Invalidate(env)` with the Env it was laid out under; without it the widget
keeps the size it was last measured at, since the cache above it has no
reason to measure again.

### Retained paint state

State that has no signal and must outlive a rebuild can be kept on the
Canvas under a typed `Slot`: `dst.Retain(anchor, slot, v)` stores a value
for the next frame and `dst.Retained(anchor, slot)` reads what was stored
last frame, where the `Anchor` is the widget's ID or its Rect. `dst.Ease`
is a `Motion` kept that way. Tooltip keeps its hover timer and Transition
its start time in slots.

## Inspector

`Config{Inspector: ggui.KeyF1}` binds a key that toggles a development
overlay in the shape of a browser's element panel; `App.Inspector(on)` does
the same from code. It is the quickest way to see why something sits where
it does.

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
Click outside the panel to return keyboard focus to the app; application
drags that started outside the panel continue across it.

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
| Styling and animation | [style.go](style.go), [anim.go](anim.go), [transition.go](transition.go) — see [STYLING.md](STYLING.md) |
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

## Chat components and questionnaires

The chat controls follow the [shadcn/ui chat components](https://ui.shadcn.com/docs/changelog/2026-06-chat-components)
and [Questionnaire](https://ui.shadcn.com/docs/components/base/questionnaire)
contracts with native Go widgets, bindings and callbacks. Surface colors come
from the current theme; typography uses the application's native font. The
gallery has separate Attachment, Bubble, Message, Marker, Message Scroller and
Questionnaire previews in both themes. The chat surfaces follow the official
Rhea demos (24px bubble corners, 16px attachment corners); the questionnaire
follows the Nova demo. Bubble groups use 8px spacing, separate turns use 24–32px,
and reaction rings sit outside the pill. Native text metrics are accounted for
in the bubble padding.

The gallery's reference scenes bundle Geist, the original demo photos/avatars,
and the optional Noto Color Emoji font for consistent offline/native/WASM
rendering. The gallery uses ThemePreset for its palette and geometry; library
widgets continue to use the host theme and font. Sources and licenses are listed in
[the asset notes](examples/gallery/assets/chat/README.md).

### Attachments

```go
state := ggui.State(ui.AttachmentIdle)
file := ui.Attachment("report.pdf", "PDF · 2.4 MB").
    Media(ggui.Text("PDF").Size(11)).StateOf(state).
    Trigger("Preview report.pdf", previewReport).
    Actions(ui.AttachmentAction("Remove report.pdf", ggui.Text("×"), removeReport))
```

`State` sets a fixed state and replaces `StateOf`. The five states are `AttachmentIdle`
(dashed border), `AttachmentUploading` and `AttachmentProcessing` (title shimmer),
`AttachmentError` (destructive border/media/metadata), and `AttachmentDone` (default).
Put an explicit failure reason in the description. Uploading is presentation
only: the host owns files, progress, retries, transport and image lifetimes.

`Image(img, alt)` displays a rounded cover crop; unfinished previews are dimmed.
`Vertical()` moves media above the metadata and overlays actions at the top right.
`Size(AttachmentSmall)` and `Size(AttachmentExtraSmall)` select compact geometry;
`Width(px)` sets the requested width within parent constraints. Long filenames
and metadata truncate visually but retain their complete accessible text.
Card triggers and actions remain separate keyboard and pointer targets.

`AttachmentGroup(files...)` supplies horizontal scrolling, 12px gaps, settling
snap points and edge fades. Its named group supports Left/Right, Home/End and
PageUp/PageDown. Tab brings offscreen actions into view. Keep the group instance
or assign a stable `Key` when preserving scroll across rebuilds.

### Conversation surfaces

`Bubble(content)` is a primary surface, limited to 80% of its available width.
`Secondary`, `Muted`, `Tinted`, `Outline`, `Ghost` and `Destructive` select the
other treatments. Ghost has no frame or padding and can use the full width.
`End()` aligns a bubble to the trailing side. `Action(name, fn)` or `Link(name, fn)`
provides a focusable button/link surface; the host owns opening a URL. Both
support `Disabled` and `DisabledWhen`.

`Reactions(widget)` overlaps the bottom end edge. `ReactionsTop()` and
`ReactionsStart()` move it. Leave vertical space between rows; reactions can
contain independently named buttons. Compose `ui.Collapsible` inside the bubble
for show-more behavior. `BubbleGroup` stacks consecutive bubbles with an 8px gap.

`Message(content).Avatar(widget).Header(widget).Footer(widget)` arranges a
conversation row. `End()` reverses the avatar side and aligns its metadata and
nested bubbles. The avatar sits above the footer. `MessageGroup` stacks rows;
avatars, links, attachments and content are supplied by the caller.

`Marker(content)` displays muted status content. `Icon(widget)` supplies a
16px decorative slot; `Separator()` adds rules around a centered label and
`Border()` adds a bottom rule. Compose a spinner, text or links as needed.

### Transcript scrolling

```go
rows := ggui.State([]ui.MessageEntry{
    {ID: "question", Content: ui.Message(ui.Bubble(ggui.Text("Review this?"))).End(), Anchor: true},
    {ID: "reply", Content: ui.Message(ui.Bubble(ggui.Text("Reviewing…")).Secondary())},
})
transcript := ui.MessageScroller(rows).Height(320).AutoScroll(true)
```

Each row needs a unique, nonempty ID and nonnil content. Keep the scroller alive
while updating its bound rows. Replace a row's content for streamed updates, or
bind its text directly. The scroller does not own transport or message storage.

- `Opening(ScrollStart|ScrollEnd|ScrollLastAnchor)` chooses the first nonempty
  transcript's position. End is the default; last-anchor falls back to end when
  that turn fits. Opening is applied before the first paint.
- `AutoScroll` defaults to false. When enabled, it follows output at the live
  edge; wheel, pointer presses and keyboard input inside the viewport pause it,
  including events consumed by child controls. `ScrollToEnd()` resumes following.
- Appending an `Anchor` row starts a turn near the top, preserving a 64px peek
  of its predecessor. `PreviousPeek`, `ScrollMargin` and `Gap` configure spacing.
- Prepending history or changing measured row heights preserves the first visible
  stable row and the offset within it. `Save()`/`Restore()` persist that reading
  position. Restore waits if its row has not been loaded yet.
- `ScrollToMessage(id, alignment)` supports start, center, end and nearest;
  missing rows remain queued until available. Later jump commands replace them.
  `ScrollToStart()` and `ScrollToEnd()` target the edges.
- `Animation(duration)` controls programmatic scrolling; reduced motion jumps
  immediately. `Visibility()` and `OnVisibility` expose visible IDs, the current
  anchor and scrollable edges. Callbacks run on the UI thread and should avoid
  changing layout in a feedback loop.

The viewport is keyboard scrollable, reveals focused descendants, clips input
along with content and shows start/end controls only when useful. Large histories
are measured, not virtualized; paginate the bound collection for very long chats.

### Questionnaires

```go
answers := ggui.State(ui.QuestionAnswers{})
form := ui.Questionnaire(answers,
    ui.Question{
        Name: "scope", Title: "What should we build?", Required: true,
        Choices: []ui.QuestionOption{
            {Value: "small", Label: "Small change", Description: "Keep the scope focused."},
            {Value: "full", Label: "Complete feature"},
        },
        InputLabel: "Another answer", Placeholder: "Describe your idea…",
    },
    ui.Question{Name: "notes", Title: "Any constraints?", InputLabel: "Notes"},
).Shortcuts(ui.QuestionNumbers).OnSubmit(func(values ui.QuestionAnswers) {
    // Persist or send the validated answers here.
})
```

Question names and option values must be unique and nonempty. Questions are
optional by default, but moving forward requires either an answer or explicit
Skip. `Required` prevents skipping; `Multiple` permits several fixed answers.
Freeform input replaces a fixed answer on single-choice steps and can accompany
fixed answers on multiple-choice steps. Whitespace alone is not an answer.
Disabled steps are excluded from progress/navigation/submission and disabled
choices are not selectable or serialized.

The answer binding stores `Values`, `Text` and `Skipped` per question.
`OnSubmit` receives a deep copy containing enabled answered questions; skipped
questions are omitted. `Status(name)` and `OnStatusChange` distinguish unanswered,
answered and skipped. `Active(binding)` controls the active question by name;
`OnItemChange` observes navigation. `SetItems` supports conditional collections.
`Reset()` restores initial answers and navigation, clearing skips and errors.

`Question.Validate` returns an error string for custom validation. `SetError`
returns to a step with a host-provided error; editing clears it. Forward navigation
validates the active question, while Submit validates all enabled questions and
focuses the first invalid answer. Previous does not validate or discard answers.
Buttons remain enabled so an attempted action can explain what is missing.

Tab visits answers and visible actions. Arrows move between fixed answers;
radio movement also selects. Enter on a selected answer continues; Cmd/Ctrl+Enter
validates and continues from any answer or action. Letter or number shortcuts
select enabled answers without advancing and never intercept text editing or
IME composition. Progress and validation are exposed to assistive technology.
The surrounding app/card/dialog owns cancellation, persistence and branching.
