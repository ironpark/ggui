# Feedback and composition

[Documentation](README.md) · [Project README](../README.md)

Compose notifications, modal panels, grouped actions, and reusable content surfaces.

Examples use `ggui`, `ui` and `ui/icons` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## On this page

- [Notices and loading states](#notices-and-loading-states)
- [Resizable panes](#resizable-panes)
- [Toast notifications](#toast-notifications)
- [Sheets and drawers](#sheets-and-drawers)
- [Grouped actions and choices](#grouped-actions-and-choices)
- [Confirmations](#confirmations)
- [Composition helpers](#composition-helpers)

## Notices and loading states

`ui.Alert(title, description)` provides an inline notice, with `.Destructive()`,
`.Icon(role)` leading it in the title's color, and `.Action(widget)` at its top
right. `ui.Empty(title, description)` presents an empty state, with optional
`.Media(widget)` or `.MediaIcon(role)` above the title and `.Action(widget)`
below it. It has no border of its own; `.Outline()` draws shadcn's dashed one.

```go
ui.Alert("Connection interrupted", "You can retry without losing your work.").
	Destructive().Icon(icons.Alert).Action(ui.Button("Retry", retry).Size(ui.ButtonExtraSmall).Outline())
ui.Empty("No attachments", "Add a file to get started.").
	MediaIcon(icons.File).Action(ui.Button("Add file", add)).Outline()
```

`ui.Kbd("Ctrl")` displays a key cap, and `ui.KbdGroup(ui.Kbd("⌘"), ui.Kbd("K"))`
lines up the caps of a chord, with any text between them. `ui.Skeleton(180, 16)`
reserves loading space (`.Circle()` rounds it), and `ui.Spinner()` indicates
ongoing work at the theme's icon size, 16 logical pixels, in the color of the
text around it; `.Size(px)` and `.Color(c)` change either. Both loading
indicators respect reduced motion.

## Resizable panes

`ui.Resizable(fraction, first, second)` splits space into two panes with a draggable,
keyboard-operable divider; `.Vertical()` and `.MinSizes(a, b)` configure it.
The divider is a hairline that takes no room of its own, as shadcn's is;
`.WithHandle()` adds the grip over its middle.
Each pane is clipped to its share and is a layout boundary, as `ggui.Contain`
makes one: a disclosure easing open in a pane repaints that pane alone.

## Toast notifications

`ui.NewToaster()` owns a bounded notification queue. Create it once in app or
component setup, and call it on the UI thread. Include the host once in the
tree, register `ggui.OnCleanup(toaster.Close)` in setup, then call
`toaster.Push(ui.Toast(title, description).Action("Undo", undo))` from UI callbacks.
The toaster itself is the host widget. For example, include it beside the page
in `ggui.Stack(page, toaster).Expand()`. Configure `.Limit(n)` to change the
default maximum of three notices, and `.Position(p)` to stack them somewhere
other than `ui.ToastBottomRight`: the other corners and the middle of the top
or bottom edge. `Push` returns a `ToastID` for later dismissal or replacement;
`Clear()` dismisses all notices and `Close()` also rejects future pushes.

A notice's type picks its leading icon: `.Success()`, `.Info()`, `.Warning()`
and `.Error()` (`.Destructive()` is the same), or `.Type(t)`. `.Loading()` leads
with a spinner and stays until it is dismissed or replaced; `Update(id, message)`
turns it into the result it waited for, in place, and starts that one's lifetime:

```go
id := toaster.Push(ui.Toast("Uploading", "Sending the file…").Loading())
// later, on the UI goroutine:
toaster.Update(id, ui.Toast("Uploaded", "The file is stored.").Success())
```

Notifications do not steal focus and pause their timeout while hovered or focused.
They stack with the newest in front and the others tucked behind it, and spread
out while the pointer is over them. They come and go over 500ms and the stack
moves smoothly when notices change; reduced motion skips these animations.
`.Countdown()` draws a bar that empties as a timed notice's lifetime runs out,
which no notice has by default. `.Duration(0)` keeps a notice until dismissed. `Dismiss` immediately removes its
interaction and excludes it from `Len`, while its exit animation finishes.

## Sheets and drawers

`ui.Sheet(open, content)` is a modal panel anchored to an edge of the window,
for filters, details and settings that deserve more room than a popup. It
behaves as `ui.Dialog` does — a scrim takes the clicks, Tab stays inside,
Escape or a click outside closes it, focus returns to where it was — but it
fills one edge instead of floating in the middle. `.Left()`, `.Right()`
(the default), `.Top()` and `.Bottom()` choose the edge, `.Size(px)` sets
how far it reaches from it, `.Title(text)` adds a heading, `.Description(text)`
a muted line under it, `.Footer(actions...)` stacks the answering buttons at
its foot at full width, and `.Compact()` drops the padding.

```go
filters := ggui.State(false)
ui.Sheet(filters, filterForm).Title("Filters").Description("Narrow the list.").Left().Size(320).
	Footer(ui.Button("Apply", apply), ui.Button("Cancel", cancel).Outline())
```

`ui.Drawer(open, content)` is the same panel rising from the bottom with its top
corners rounded. It is as tall as its content, up to 96 logical pixels short of
the window, until `.Size(px)` or `SnapPoints` say otherwise. It drags by the
middle of its top edge either way; `.Handle(true)` draws the short bar that says
so, and `.CloseButton(true)` gives it the close button a sheet has. Sheets slide in and out over the theme's `MotionSlow` and drawers over
`MotionDrawer`, 450ms, and both land at once under reduced motion. The motion lives in the widget, so keep
the sheet across rebuilds — construct it beside the signal it binds, or inside
a `Component` — and one rebuilt every frame simply appears and goes, as
`ui.Dialog` does.

## Grouped actions and choices

`ui.ButtonGroup(children...)` joins controls into one strip, as shadcn's
ButtonGroup does: the corners between neighbours are squared, neighbouring
borders are shared, and the focused child's ring is drawn over the others;
`.Vertical()` stacks them. It is a container and nothing more, so every child
keeps its own tab stop and its own action. Outline buttons suit it best.
`ui.ButtonGroupText(label)` (or `ButtonGroupTextOf(widget)`) puts a caption on
the muted surface among the controls, and `ui.ButtonGroupSeparator()` draws a
line between two children that have no border between them, such as the halves
of a split button. A `TextField`, `NumberInput` or `InputGroup` joins too and
takes the width the buttons leave, which makes a search field with its button.
A group of groups keeps them a Space apart instead of joining them.

`ui.ToggleGroup(value).Options(options)` is a segmented single choice: one option of
several, bound the way `ui.Select` and `ui.Radios` are. Its segments are toggles
with no track behind them: `.Outline()` borders them, `.Size(...)` takes a button
size, and `.Spacing(units)` sets the gap in shadcn's spacing units, two by
default, where 0 joins them into one strip that shares its borders. `.Format(fn)`
sets how an option is shown, `.Vertical()` stacks the segments, and `.OnChange(fn)`
reports user changes. The group is one tab stop, as a set of radio buttons is:
Left and Right (Up and Down when vertical) move the choice and wrap, Home and
End go to the ends, and Space or Enter re-picks where the choice already is.
Each segment is announced as a radio that says whether it is the chosen one.

```go
ui.ButtonGroup(ui.Button("Copy", copyIt).Outline(), ui.Button("Paste", pasteIt).Outline())
ui.ButtonGroup(ui.TextField(query).Placeholder("Search…"),
	ui.ButtonOf(ui.Icon(icons.Search), search).Name("Search").Size(ui.ButtonIcon).Outline())
ui.ToggleGroup(align).Options([]string{"left", "center", "right"})
ui.ToggleGroupMultiple(styles).Options([]string{"Bold", "Italic"}).Outline().Spacing(0)
```

A single `ui.Toggle(pressed, label)` is a two-state button; `ui.ToggleOf(pressed,
widget)` holds an icon or other content instead of text and needs a `.Name`.

## Confirmations

`ui.AlertDialog(open, title, description)` is a question that has to be
answered: the scrim takes the clicks but does not close it, so the only ways
out are its two buttons and Escape, which cancels. `.Confirm(label, fn)` names
the action and what it does, `.Cancel(label)` renames the other button,
`.OnCancel(fn)` hears about every cancellation however it happened, and
`.Destructive()` colors the confirming button for an answer that cannot be
taken back. `.Media(widget)` shows an icon in a muted tile beside the text, and
`.Small()` makes the dialog 320 logical pixels wide with its text centered, the
media above it and the two buttons sharing the footer.

```go
del := ggui.State(false)
ui.AlertDialog(del, "Delete the file?", "This cannot be undone.").
	Confirm("Delete", remove).Destructive().Media(ui.Icon(icons.Alert))
```

A plain `ui.Dialog` takes the same parts as a sheet: `.Title`, `.Description`,
and `.Footer(actions...)`, which sets the answering buttons in a tinted band
along its bottom edge.

```go
ui.Dialog(open, form).Title("Edit profile").Description("Make changes to your profile.").
	Footer(ui.Button("Cancel", cancel).Outline(), ui.Button("Save", save))
```

The policy itself is `ui.Dialog`'s: `.Dismissible(false)` on any dialog stops
a click on the scrim from closing it, while leaving Escape alone so the
keyboard is never shut in.

## Composition helpers

These are small widgets with no state of their own, for the shapes that
otherwise get rebuilt by hand in every application.

| Widget | Behavior |
| --- | --- |
| `ui.Avatar(name)` | A round portrait, 32 logical pixels across: the initials of the name on the muted surface until `.Image(img)` gives it one, cropped to cover the circle. `.Small()` (24), `.Large()` (40) or `.Size(px)` set the diameter, `.Square()` rounds it to the theme's radius instead, and `.Badge(color)` or `.BadgeIcon(role)` add a status dot at its lower right. |
| `ui.AvatarGroup(avatars...)` | Avatars overlapping in a row, each ringed in the page's background. `.Max(n)` shows the first n and counts the rest in a bubble at the end; `.Count(n)` adds people with no avatar to that count. |
| `ui.AspectRatio(ratio, child)` | Sizes the child to a width-over-height ratio inside the space it is offered: the width leads, unless the height it implies would not fit. |
| `ui.Item(title, description)` | One row of a list: `.Media(widget)` or `.MediaIcon(role)` before the text, `.Action(widget)` after it, `.Header(widget)` and `.Footer(widget)` above and below, `.Outline()` for a bordered card or `.Muted()` for a tinted one, and `.Small()` or `.ExtraSmall()` for denser rows. It takes no input, so the action keeps its own. `ui.ItemGroup(items...)` stacks items as a list, with `ui.ItemSeparator()` drawing a line between two of them. |
| `ui.Card(children...)` | A bordered surface. Given `ui.CardHeader(title, description)` (with `.Action(widget)` at its top right), `ui.CardContent(widget)` and `ui.CardFooter(widgets...)` (with `.Justify(j)`), it stacks them a CardPad apart and runs the footer to the card's bottom edge on a muted band; `.Small()` tightens it. |
| `ui.Breadcrumb(crumbs...)` | The path to the page the user is on, built from `ui.Crumb(label, onTap)`. Every step but the last is a link with its own tab stop; a chevron separates them, `.Separator(s)` or `.SeparatorOf(widget)` replace it, and `.Max(n)` elides the middle behind an ellipsis. |
| `ui.InputGroup(input)` | One field chrome around a bare `ggui.TextInput` and the widgets that flank it: `.Leading(widget)`, `.Trailing(widget)`, and `.Top(widget)` and `.Bottom(widget)` on rows of their own above and below the editor. `.Multiline()` makes it a text area that grows by the line. The group owns the border, fill, padding and focus ring, so it takes the editor rather than `ui.TextField`, which draws a box of its own. A Button handed over in its default look becomes a ghost, extra-small addon button. |
| `ui.Tooltip(child, text)` | A line of help shown above the child immediately on hover (customize with `.Delay(d)`), or while focus moved there with the keyboard is within it. `.Side(ggui.PopupRight)` and the other sides, `.Align(a)` and `.AlignOffset(px)` place it; it flips when its side has no room. `.Shortcut(chord)` adds a key cap. A press closes it until the cursor leaves, and the focus a click gives does not open it. It registers no hit region, so the child gets every event, and paints through `Canvas.Overlay` above everything else. |
| `ui.HoverCard(anchor, content)` | A panel of content shown near the anchor once the cursor has rested on it, and kept up while the cursor is on either one. `.Delay(d)`, `.Width(px)`, `.Side(s)`, `.Align(a)` and `.AlignOffset(px)` tune it. Nothing about it takes focus: a hover card is an aside, and the keyboard never has to visit it. |

```go
ggui.Row(
	ui.Avatar("Ada Lovelace").Image(portrait),
	ui.Item("Backups", "Last run 2 hours ago").Action(ui.Button("Run", run).Outline()),
).Space(1)

ui.Card(
	ui.CardHeader("Create project", "Deploy your new project in one click."),
	ui.CardContent(form),
	ui.CardFooter(ui.Button("Cancel", cancel).Outline(), ui.Button("Deploy", deploy)).Justify(ggui.SpaceBetween),
)

ui.InputGroup(ggui.TextInput(prompt).Placeholder("Ask anything…")).Multiline().
	Bottom(ggui.Row(ui.Caption("52% used"), ggui.Spacer(), ui.Button("Send", send).Size(ui.ButtonExtraSmall)).Gap(8))
```

`InputGroup.Disabled` and `BindDisabled` disable the editor as well as its
chrome. Leading and trailing addon controls remain independent. The editor's own
settings are preserved: it is disabled if either it or its group is disabled.
Custom containers can pass `ggui.InputDisabledKey` through `Env` or `Provide`;
TextInput combines that inherited value with its own settings. A descendant
`false` cannot clear an ancestor's `true`. Disabled editors remain in the
accessibility tree and reject pointer, keyboard, and accessibility edits.

## Draggable drawers

A bottom `Drawer` supports dragging by its handle, drawn or not. Use `.Draggable(false)` to
turn this off. Snap heights are positive, increasing logical pixels:

```go
open := ggui.State(false)
snap := ggui.State(1)
ui.Drawer(open, content).SnapPoints(160, 280, 420).BindSnapPoint(snap)
```

Releasing settles to the nearest point; dragging below half the smallest point
requests dismissal when `.Dismissible(true)`. `OnSnapChange` reports the requested
index. Controlled bindings may reject a request. This native handle gesture does
not implement browser scroll handoff, velocity snapping or nested drawer scaling.
The handle also offers keyboard and accessibility resizing.

## Code and execution traces

```go
ui.CodeBlock(source).Header("main.go").Language("go").LineNumbers(true).
    StartLine(1).Wrap(false).Height(320)

ui.Trace(
    ui.TraceStep{ID: "fetch", Title: "Fetch source", Status: ui.TraceCompleted},
    ui.TraceStep{ID: "build", Title: "Build", Status: ui.TraceRunning,
        Details: ui.CodeBlock(buildLog)},
)
```

Code remains selectable and its copy action preserves the source, excluding
line numbers and decoration. It stays LTR inside an RTL interface. `CodeBlockOf`
or `BindCode` follows streaming source. Language labels do not install a syntax
highlighter. Trace IDs must be globally unique within the hierarchy; they retain
disclosure identity during updates. `Children` nests steps, `Description` and
`Duration` add context, and `OnActivate` makes a leaf an action. `BindSteps` follows
execution state; `BindExpanded` controls the map of open step IDs. Plain rows do
not add tab stops.
