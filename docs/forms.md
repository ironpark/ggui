# Forms and controls

[Documentation](README.md) · [Project README](../README.md)

Bind inputs to application state, name controls, and show validation feedback.

Examples use `ggui`, `ui` and `ui/icons` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## On this page

- [Controls](#controls)
- [A small form](#a-small-form)
- [Buttons and shared options](#buttons-and-shared-options)
- [Text input and validation](#text-input-and-validation)

## Controls

Import `github.com/ironpark/ggui/ui` for themed controls. They use the core
layout, reactivity, and input APIs, so custom controls can follow the same model.
A bound control writes its value to the binding; writing to the binding updates
the control.

## A small form

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
	ui.Radios(plan).Options([]string{"free", "pro"}).Format(strings.ToTitle),
	ggui.Row(ui.Button("Save", save), ui.Button("Cancel", cancel).Outline()).Gap(8),
	ui.Divider(),
).Gap(12)
```

## Buttons and shared options

`ui.Button(label, onTap)` creates a primary button. Use `.Outline()`,
`.Secondary()`, `.Ghost()`, `.Destructive()` or `.Link()` for the other
shadcn/ui variants, `.Size(ui.ButtonSmall)`, `ui.ButtonLarge` or
`ui.ButtonIcon` for the sizes, and `ui.ButtonOf(child, onTap)` for custom
content. `.Icon(ui.Icon(role))` puts an icon before the label and
`.TrailingIcon(...)` one after it, at the size's icon size and gap; a button
holding nothing but an icon is `ui.ButtonOf(ui.Icon(role), onTap)` with one of
the `ui.ButtonIcon` sizes and a `.Name`.
[Component variants](styling.md#component-variants) covers variants
of your own and restyling the built-in ones.

```go
ui.Button("New branch", create).Outline().Icon(ui.Icon(icons.Plus))
ui.Button("Next", next).TrailingIcon(ui.Icon(icons.ChevronRight))
ui.ButtonOf(ui.Icon(icons.Search), search).Name("Search").Size(ui.ButtonIconSmall).Ghost()
```

`ui.Radio(plan, value, label)` creates one radio option; `ui.Radios` builds a
group, stacked with 8 logical pixels between options as shadcn's RadioGroup is.
`.Horizontal()` lays the options out in a row instead. `ui.Switch(on, label)`
takes `.Small()` for shadcn's small switch.

`Checkbox`, `Radio`, `Radios`, `Switch` and `Select` take `.Invalid(true)` for
a value that failed validation: a destructive border and ring. `Checkbox`, `Radio` and `Switch` also take
`.BindInvalid(reader)`. A control inside a `Field` that holds an error is
invalid without either.

Common options on interactive controls include:

| Option | Purpose |
| --- | --- |
| `.Disabled(v)` | Disable interaction. |
| `.BindDisabled(reader)` | Follow a reactive disabled state without rebuilding. |
| `.OnChange(fn)` | Observe a user-selected value on controls that expose this callback. |
| `.OnCommit(fn)` | Observe completed editing on sliders and text fields. |
| `.Pad(...)` | Override padding on controls such as buttons. |

Every control widget with `Disabled` also supports `BindDisabled`. The last
setting wins: `Disabled(false)` removes a previous `BindDisabled` binding;
`BindDisabled(busy)` replaces the static setting. Composite controls evaluate
their own binding and apply the result to their internal controls. Disabling a
popup control also closes its popup. Item configuration values such as
`CommandEntry` keep their static `Disabled` option.

Public control `.Key(id)` setters return the concrete control, so identity can
be assigned inline: `ui.Button("Save", save).Key("save").Outline()`. Configure
keys before mount and use stable, comparable values. A key identifies the
control's input state; it does not automatically key all children of a group.
Combobox keys its trigger, popup and search editor separately; keep it mounted
to preserve the search query.

Use `.Name(name)` for a control's accessible and Probe name. `Field` supplies
its label only when the control has no explicit name, taking precedence over a
placeholder or built-in fallback. Labels passed to constructors such as
`Button("Save", save)` are explicit names. Custom controls can participate by
implementing `ui.Named` (`SetName(string)` and `HasName() bool`); `HasName` must
exclude placeholders and fallback names. `Semantics` reports the resolved name.
Use `.Format(fn)` for option text on Select, Combobox, Radios, and ToggleGroup,
and `.RowName(fn)` for a table row's accessible name.

Controls obtain colors and spacing from the inherited theme. Hover and press
state stay in the widget; animated details retain their motion across rebuilds.
Use `ggui.Now()` in `Paint` for animation timing. It is sampled once per frame
and advances by at most 100ms, so a hidden window resumes without a large jump.
`ggui.SetClock` replaces the source in tests, for the probes the test's
goroutine makes; `Probe.Advance(d)` advances one probe's clock, and its
headless frame, by exactly `d`.

## Text input and validation

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
| `.Filter(fn)` | Pass every edit through a prefix-preserving function. |
| `.Style(ts)` | Merge a text style onto the editor's. |
| `.Key(k)` | Give the editor an identity that survives a rebuild that moved it. |

Every chainable setter on `ggui.TextInput` has a namesake on `ui.TextField`;
a test in the `ui` package keeps them in step.

Editors support IME composition, with preedit text underlined in place.
Caret movement and Backspace respect grapheme clusters as Unicode defines them
(UAX #29), including combined letters, Indic conjuncts, Hangul written in jamo
and multi-code-point emoji. Caret and selection state survive rebuilds.
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
	BindError(problem)
```

A nonempty error is shown under the help text in the theme's Destructive
color, the label turns destructive too, and the control inside is marked
invalid. The field label names the control for `Probe.Find` unless the control
already has an explicit name.

`.Horizontal()` lays a field out as a row, shadcn's horizontal orientation: the
label, with its help and error under it, takes the width that is left and the
control sits at the end, as a switch does in a list of settings.
`.ControlFirst()` puts the control at the start instead, as a checkbox sits
before what it chooses.

`ui.FieldGroup(children...)` stacks fields, and whatever a form holds between
them, two and a half Space apart. `ui.FieldSet(children...)` groups related
fields under `.Legend(text)` and `.Description(text)`, and is read as a group
named by its legend. `ui.FieldSeparator()` draws a hairline between parts of a
group; `.Label(text)` breaks it around a word.

```go
ui.FieldSet(ui.FieldGroup(
	ui.Field("Time zone", ui.Select(zone).Options(zones).Placeholder("Select a time zone")),
	ui.FieldSeparator().Label("Notifications"),
	ui.Field("Marketing emails", ui.Switch(marketing, "")).Help("News about products.").Horizontal(),
	ui.Field("Accept the terms", ui.Checkbox(terms, "")).ControlFirst(),
)).Legend("Preferences").Description("How the workspace reaches you.")
```

### Committing an edit

A field that should change the model only once the user is done with it,
and refuse what the model will not take, binds a `ggui.Draft`. The field
shows the model's value until edited, then the edit; leaving the field or
pressing Enter hands the edit to the commit function, and a commit that
returns an error keeps the edit and shows the error, for the user to repair.
Escape drops the edit and goes back to the model's value, and while there is
nothing to drop it passes on, so a dialog around the field still closes.

```go
name := ggui.Draft(ggui.Select(store, func(p *Project) string { return p.Name }),
	func(v string) error { return project.Rename(v) })
ui.Field("Name", ui.TextField(name)).BindError(name.Error())
```

`Commit` and `Revert` do the same from code, as a Save button committing
every field first does; `Fail(msg)` shows an error found elsewhere, and
`Dirty()` follows whether an edit is waiting. A Draft made in a `View`'s
build is lost with that build; make it with `ggui.Remember` to keep it, its
edit and error included, while the page rebuilds around it. Any `Binding`
with `Commit() bool` and `Revert() bool` gets the same treatment from a text
input; that is the `ggui.Committer` interface.

### Numbers

`ui.NumberInput(value)` edits a `Binding[float64]` as text with − and +
steppers. Typing is committed on Enter or blur: the text is parsed, rounded to
the decimals shown and clamped to the range; text that is not a number
reverts. ArrowUp and ArrowDown step (Shift for ten steps), and a stepper is
disabled at its end of the range. Screen readers see one spin button with its
range, and can increment, decrement or set it. The spin button is the editor
itself, so its caret and characters read as in any text field.

```go
ui.Field("Quantity", ui.NumberInput(qty).Range(0, 99))
ui.NumberInput(opacity).Range(0, 1).Step(0.05) // shows two decimals
```

| Option | Behavior |
| --- | --- |
| `.Range(lo, hi)` | Clamp the value; unbounded by default. |
| `.Step(s)` | The step, 1 by default; also sets the decimals shown. |
| `.Precision(n)` | Show and round to `n` decimals instead. |
| `.OnChange(fn)` | Handle a committed or stepped change. |
| `.Name`, `.Disabled`, `.Key` | As on other controls, with `Bind` forms. |

For segmented verification codes, see [Input OTP](carousel-and-otp.md#input-otp).
For font fallbacks, IME-related text coverage, and emoji, see
[Fonts, emoji, and icons](fonts.md).

## Pressed buttons, labels and ranges

`ui.Toggle(pressed, "Bold")` is a two-state button with `.Outline()`, button size
variants, `.Icon(...)` and `.TrailingIcon(...)`, and `.OnChange(fn)`.
`ui.ToggleOf(pressed, ui.Icon(role)).Name("Bold")` holds an icon or other
content instead of a label. `ui.ToggleGroupMultiple(values).Options(options)`
binds a slice of independent selections. Its arrows move focus without changing
the selection; Space and Enter toggle the focused option. `.OnValuesChange(fn)`
receives an owned snapshot.

`ui.Label("Email").For(editor)` associates a separate label with a control.
Clicking focuses the editor or activates an associated choice control, without
adding a Tab stop. `Field` uses this association automatically.

```go
budget := ggui.State([]float64{25, 75})
ui.RangeSlider(budget, 0, 100).Step(5).MinGap(10).
    ThumbNames("Minimum budget", "Maximum budget")
```

`RangeSlider` supports any number of independently focusable thumbs. `MinGap`
is measured in value units; thumbs never cross. Each thumb exposes its permitted
minimum and maximum to accessibility. `Slider` and `RangeSlider` support
`.Vertical()`, `.LargeStep(n)`, `.OnChange(fn)` and `.OnCommit(fn)`. Defaults are
step 1 and large step 10; use `.Step(0)` for continuous pointer movement. Page
keys and Shift+arrows use the large step. Home/End respect neighboring thumbs.
Horizontal pointer position and Left/Right keys mirror under `DirRTL`.

## Multiple comboboxes and native selects

```go
languages := ggui.State([]string{"Go"})
ui.ComboboxMultiple(languages).Options([]string{"Go", "Rust", "TypeScript"}).
    Name("Languages").OnValuesChange(updateLanguages)
```

The popup filters options, marks selections and stays open while toggling them.
Selected values appear as removable chips. `Options` / `BindOptions`, `Format`,
`Placeholder`, `Disabled` / `BindDisabled` and `Key` follow the single-combobox
conventions. Callbacks receive separate slice snapshots. An empty search accepts
Backspace to remove the last selected value.

`ui.Select(value).Options(options)` lists its options in a menu-style popup with
a check at the right of the chosen one, opening with that row over the field.
`.Placeholder(text)` shows muted text while the value is not one of the
options, `.GroupBy(fn)` puts runs of options under the heading `fn` names with a
separator between groups, `.DisabledOption(fn)` greys out the options it reports,
and `.Small()` makes the field 28 logical pixels tall instead of 32.

```go
ui.Select(zone).Options([]string{"Eastern", "Pacific", "London", "Tokyo"}).
	Placeholder("Select a time zone").
	GroupBy(func(z string) string { return regionOf[z] }).
	DisabledOption(func(z string) bool { return z == "Tokyo" })
```

`ui.NativeSelect(value).Options(options)` attempts the platform's checked native
menu. It uses the regular themed Select popup when the platform cannot provide
one, including in headless tests. `ui.Select(value).Native(true)` is equivalent.
