# Feedback and composition

[Documentation](README.md) · [Project README](../README.md)

Compose notifications, modal panels, grouped actions, and reusable content surfaces.

Examples use `ggui` and `ui` imports and application-defined placeholders.
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

`ui.Alert(title, description)` provides an inline notice, with `.Destructive()`
and `.Action(widget)`. `ui.Empty(title, description)` presents an empty state,
with optional `.Media(widget)` and `.Action(widget)`.

`ui.Kbd("Ctrl")` displays a key cap. `ui.Skeleton(180, 16)` reserves loading
space (`.Circle()` rounds it), and `ui.Spinner().Size(20)` indicates ongoing
work. Both loading indicators respect reduced motion.

## Resizable panes

`ui.Resizable(fraction, first, second)` splits space into two panes with a draggable,
keyboard-operable divider; `.Vertical()` and `.MinSizes(a, b)` configure it.

## Toast notifications

`ui.NewToaster()` owns a bounded notification queue. Create it once in app or
component setup, and call it on the UI thread. Include the host once in the
tree, register `ggui.OnCleanup(toaster.Close)` in setup, then call
`toaster.Push(ui.Toast(title, description).Action("Undo", undo))` from UI callbacks.
The toaster itself is the host widget. For example, include it beside the page
in `ggui.Stack(page, toaster).Expand()`. Configure `.Limit(n)` to change the
default maximum of three notices. `Push` returns a `ToastID` for later dismissal;
`Clear()` dismisses all notices and `Close()` also rejects future pushes.

Notifications do not steal focus and pause their timeout while hovered or focused.
They fade and slide in and out, and the stack moves smoothly when notices change.
Reduced motion skips these animations. Timed notices show their remaining lifetime;
`.Duration(0)` keeps a notice until dismissed. `Dismiss` immediately removes its
interaction and excludes it from `Len`, while its exit animation finishes.

## Sheets and drawers

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

## Grouped actions and choices

`ui.ButtonGroup(children...)` joins controls into one bordered strip with a
hairline between each pair; `.Vertical()` stacks them. It is a container and
nothing more, so every child keeps its own tab stop and its own action. Ghost
buttons suit it, since the strip draws the border they would each draw.

`ui.ToggleGroup(value).Options(options)` is a segmented single choice: one option of
several, bound the way `ui.Select` and `ui.Radios` are. `.Format(fn)` sets how
an option is shown, `.Vertical()` stacks the segments, and `.OnChange(fn)`
reports user changes. The group is one tab stop, as a set of radio buttons is:
Left and Right (Up and Down when vertical) move the choice and wrap, Home and
End go to the ends, and Space or Enter re-picks where the choice already is.
Each segment is announced as a radio that says whether it is the chosen one.

```go
ui.ButtonGroup(ui.Button("Copy", copyIt).Ghost(), ui.Button("Paste", pasteIt).Ghost())
ui.ToggleGroup(align).Options([]string{"left", "center", "right"})
```

## Confirmations

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

## Composition helpers

These are small widgets with no state of their own, for the shapes that
otherwise get rebuilt by hand in every application.

| Widget | Behavior |
| --- | --- |
| `ui.Avatar(name)` | A round portrait: the initials of the name on the muted surface until `.Image(img)` gives it one, cropped to cover the circle. `.Size(px)` sets the diameter and `.Square()` rounds it to the theme's radius instead. |
| `ui.AspectRatio(ratio, child)` | Sizes the child to a width-over-height ratio inside the space it is offered: the width leads, unless the height it implies would not fit. |
| `ui.Item(title, description)` | One row of a list: `.Media(widget)` before the text, `.Action(widget)` after it, `.Outline()` for a bordered card. It takes no input, so the action keeps its own. |
| `ui.Breadcrumb(crumbs...)` | The path to the page the user is on, built from `ui.Crumb(label, onTap)`. Every step but the last is a link with its own tab stop; `.Separator(s)` replaces the "/", and `.Max(n)` elides the middle behind an ellipsis. |
| `ui.InputGroup(input)` | One field chrome around a bare `ggui.TextInput` and the widgets that flank it: `.Leading(widget)`, `.Trailing(widget)`. The group owns the border, fill, padding and focus ring, so it takes the editor rather than `ui.TextField`, which draws a box of its own. |
| `ui.Tooltip(child, text)` | A line of help shown below the child once the cursor has rested on it for half a second (`.Delay(d)`), or while keyboard focus is within it. It registers no hit region, so the child gets every event, and paints through `Canvas.Overlay` above everything else. |
| `ui.HoverCard(anchor, content)` | A panel of content shown near the anchor once the cursor has rested on it, and kept up while the cursor is on either one. `.Delay(d)` and `.Width(px)` tune it. Nothing about it takes focus: a hover card is an aside, and the keyboard never has to visit it. |

```go
ggui.Row(
	ui.Avatar("Ada Lovelace").Image(portrait).Size(32),
	ui.Item("Backups", "Last run 2 hours ago").Action(ui.Button("Run", run).Outline()),
).Space(1)
```

`InputGroup.Disabled` and `BindDisabled` disable the editor as well as its
chrome. Leading and trailing addon controls remain independent. The editor's own
settings are preserved: it is disabled if either it or its group is disabled.
Custom containers can pass `ggui.InputDisabled` through `Env` or `Provide`;
TextInput combines that inherited value with its own settings. A descendant
`false` cannot clear an ancestor's `true`. Disabled editors remain in the
accessibility tree and reject pointer, keyboard, and accessibility edits.
