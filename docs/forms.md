# Forms and controls

[Documentation](README.md) · [Project README](../README.md)

Bind inputs to application state, name controls, and show validation feedback.

Examples use `ggui` and `ui` imports and application-defined placeholders.
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
content. [Component variants](styling.md#component-variants) covers variants
of your own and restyling the built-in ones.
`ui.Radio(plan, value, label)` creates one radio option; `ui.Radios` builds a group.

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
	BindError(problem)
```

A nonempty error replaces the help text. The field label names the control
for `Probe.Find` unless the control already has an explicit name.

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
