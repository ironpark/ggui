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
	ui.Radios(plan, []string{"free", "pro"}).Format(strings.ToTitle),
	ggui.Row(ui.Button("Save", save), ui.Button("Cancel", cancel).Outline()).Gap(8),
	ui.Divider(),
).Gap(12)
```

## Buttons and shared options

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

Every control widget with `Disabled` also supports `DisabledWhen`. The last
setting wins: `Disabled(false)` removes a previous `DisabledWhen` binding;
`DisabledWhen(busy)` replaces the static setting. Composite controls evaluate
their own binding and apply the result to their internal controls. Disabling a
popup control also closes its popup. Item configuration values such as
`CommandEntry` keep their static `Disabled` option.

Public control `.Key(id)` setters return the concrete control, so identity can
be assigned inline: `ui.Button("Save", save).Key("save").Outline()`. Configure
keys before mount and use stable, comparable values. A key identifies the
control's input state; it does not automatically key all children of a group.
Combobox keys its trigger, popup and search editor separately; keep it mounted
to preserve the search query.

Use `.Named(name)` for a control's accessible and Probe name. `Field` supplies
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
`ggui.SetClock` replaces the source in tests; `Probe.Advance(d)` advances a
headless frame by exactly `d`.

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
	Error(problem)
```

A nonempty error replaces the help text. The field label names the control
for `Probe.Find` unless the control already has an explicit name.

For segmented verification codes, see [Input OTP](carousel-and-otp.md#input-otp).
For font fallbacks, IME-related text coverage, and emoji, see
[Fonts, emoji, and icons](fonts.md).
