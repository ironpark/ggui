# Accessibility

[Documentation](README.md) · [Project README](../README.md)

Expose meaningful names and actions, support text scaling and reduced motion,
and word the controls in the user's language.

Examples use `ggui`, `widget` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## On this page

- [Where the code lives](#where-the-code-lives)
- [Custom semantics and actions](#custom-semantics-and-actions)
- [Localization](#localization)

**The native accessibility bridge covers macOS and Windows.** On macOS it
exposes widget semantics to VoiceOver and the other Cocoa assistive tools. On
Windows it implements a UI Automation provider, which also reaches the older
MSAA clients through the bridge Windows supplies. A Linux bridge is not yet
implemented, and on any platform without one the bridge costs nothing.

> [!WARNING]
> The Windows bridge has not yet been exercised on Windows hardware. It builds
> and type-checks from any host, but its runtime behaviour is unverified;
> please report what Narrator, NVDA and Accessibility Insights make of it.

The Windows bridge reports names, roles, values and bounds, and implements the
invoke, value, range value, toggle, expand/collapse and selection item patterns.
It does not implement the text pattern yet, so a screen reader reads a text
field's contents as a value rather than walking it character by character; on
macOS that walk already works.

On `amd64` and `arm64` the bridge is complete. On any other Windows
architecture two calls that pass their arguments as floating-point numbers
cannot be received, so pointer hit testing falls back to the window and a
slider reports itself read-only; everything else behaves the same.

## Where the code lives

The accessibility vocabulary and `SemTree` live in the `a11y` package, while
the native bridges live in `internal/platform/a11ybridge`. The `ggui` package
re-exports every name from `a11y`, so `ggui.Node` and `a11y.Node` are the same
type and normal code never imports `a11y` directly. Geometry sits below both,
in `layout`, for the same reason.

The split is what lets a bridge read everything a widget published without a
copy of `Node` that falls behind the original. A new platform is a new file in
`internal/platform/a11ybridge`, implementing the `axPlatform` interface that
knows an operating system exists.

Set `Config.Accessibility` when creating the app:

| Mode | Behavior |
| --- | --- |
| `AccessibilityAuto` | Default: activate while assistive technology is attached. |
| `AccessibilityAlways` | Keep the bridge active, including for Accessibility Inspector. |
| `AccessibilityOff` | Disable the native bridge. |

Give controls meaningful labels. Buttons use their text, checkboxes use their
labels, and fields use a label or placeholder; custom content may need `.Name`.
Roles and names also help the inspector and `Probe` identify widgets.

Apply text scaling or reduced motion through inherited values:

```go
ggui.Provide(ggui.TextScaleKey, 1.5,
	ggui.Provide(ggui.ReducedMotionKey, true, tree),
)
```

Text scaling affects `Text` and `widget.TextInput`. Reduced motion makes transitions and
control motion complete immediately. Custom controls can respect it through
`env.Motion(d)`; see [Styling and themes](styling.md#accessibility-preferences).

## Custom semantics and actions

Use `ggui.Node` to describe a role, name, value, state, and supported actions.
An interactive widget can implement `Describer` and `Actor`; register it with
`Canvas.Describe` or `DescribeNode` when painting. Noninteractive content can
use `Canvas.Leaf`, and `Canvas.Node` groups semantic children. Keep semantic
names descriptive and avoid exposing decorative content as extra controls.
A widget that paints text, and would describe it with `Leaf`, asks
`dst.Named(r)` first: it is true where the element described just before is
a control covering `r`, a button whose label the text is, and the text is
then part of that control's name rather than an element of its own, which is
how `Text` stays quiet inside a button. A container whose children hang
under a node of its own that takes actions, as a reorderable list's rows do,
implements `Describer` and `Actor` on the row and paints the row's content
through `DescribeNode`.

A control built around a `widget.TextInput` keeps the editor as the element rather
than describing itself and forwarding the editor's input. The editor then
brings its caret, selection and character positions to the screen reader.
`OnDescribe` amends the editor's node, and `OnAction` takes the actions the
control adds, as `ui.NumberInput` makes its editor a spin button that steps:

```go
input := widget.TextInput(text).
	OnDescribe(func(n *ggui.Node) { n.Role = ggui.RoleSpinButton; n.Actions |= ggui.ActionIncrement }).
	OnAction(func(a ggui.Action) bool {
		if a.Kind != ggui.ActionIncrement {
			return false // the editor's own, such as moving the caret
		}
		increment()
		return true
	})
```

`app.Semantics()` returns the last published `SemTree`. A snapshot may be read
from any goroutine and retained across frames, but its shared slices must not
be modified. A window builds the tree only while something reads it: an
assistive technology, the inspector, a caller of `Semantics`, or a `Perform`
waiting to be carried out. The first call to `Semantics` turns it on and asks
for a frame, so the tree it returns may be the last one built, empty before
any was. A `Perform` keeps the tree on only until it has run.
`app.Perform(node.ID, action)` queues a supported action on the UI thread;
observe the result in a subsequent snapshot. See
[semantic action tests](testing.md#state-layout-and-semantic-actions) for a
headless example.

For status updates that should be spoken without moving focus, use
`app.Announce("Saved", ggui.Polite)`. `ggui.Assertive` requests an interrupting
announcement. Announcements may be queued from any goroutine; native delivery
requires an active macOS or Windows bridge.

## Localization

The words `ui`'s controls produce by themselves, a dialog's "Close", a pager's
"Previous" and "Next", a command list's "No results", the calendar's month and
weekday names, the names a screen reader hears for an icon button, are a
`ui.Strings`. `ui.ProvideStrings` gives a subtree its own, in any language;
put it around the root for the whole app, since popups, dialogs and toasts take
the Strings of where they are placed. A field left empty keeps the English of
`ui.DefaultStrings()`, so an app can replace a few words and leave the rest:

```go
ko := ui.Strings{
	Close: "닫기", Previous: "이전", Next: "다음",
	NoResults: "결과 없음", CopyCode: "코드 복사",
	Weekdays: [7]string{"일", "월", "화", "수", "목", "금", "토"},
	Months: [12]string{"1월", "2월", "3월", "4월", "5월", "6월",
		"7월", "8월", "9월", "10월", "11월", "12월"},
	MonthTitleFormat: "%[2]d년 %[1]s", // 2026년 11월
	DismissFormat:    "%s 닫기",
}
root := ggui.Provide(ggui.LanguageKey, "ko", ui.ProvideStrings(ko, app))
```

Fields ending in `Format` are `fmt` formats whose arguments each field's
comment lists; an explicit index such as `%[2]d` puts them in the order a
language needs. A control reads its Strings from its Env as it lays out, so
`ui.BindStrings(lang, root)` switches the language of a mounted tree when the
`lang` signal changes, with nothing rebuilt. `ui.Strings` also covers the
controls in `ui/chart` and `ui/chat`. A custom widget words itself the same
way in `Layout`, reading a shared read-only `*ui.Strings` with
`ui.StringsOf(env)` or a mutable copy with `ui.StringsFrom(env)`.

A setter on a control still wins over the Strings around it: `Name`,
`Placeholder`, `Pagination.Labels`, `Command.Empty` and `HintLabels`,
`Calendar.WeekdayLabels`, `MonthLabel` and `MonthNames`,
`chat.Questionnaire.SubmitLabel` (`ui/chat`), `AlertDialog.Confirm` and `Cancel`,
`EditorTab.CloseName`, `DatePicker.Format`.

Strings are the controls' words. An app's own text is translated by package
`i18n`, which works the same way and can follow the same language: see
[Translating an app](i18n.md#with-uistrings).

Strings carry words only. `ggui.LanguageKey` is the language text is shaped
in, which picks a script's forms, and `ggui.Directionality` the direction it
reads in; see [right-to-left text](fonts.md#right-to-left-text). An app in
Arabic provides all three. Numbers, a calendar day's ISO date name and the
glyphs between parts (`·`, `–`, `↑↓`) are not words and stay as they are.
