# Public API conventions

[Documentation](README.md) · [Forms](forms.md) · [Layout](layout.md)

Set a literal with `X(value)`, follow a source with `BindX(reader)`, and pass a
`Binding[T]` when a control edits a value. Both forms return the concrete widget
so expressions chain. Widgets expose setters, never a property type; a custom
widget writes setters that keep this contract with the
[`property`](../property/property.go) package, as `widget`, `ui`, `ui/chart`, and `ui/chat` do.

```go
ui.TextField(name).Name("Display name").BindDisabled(saving)
ui.Select(selected).BindOptions(available)
widget.Box(widget.TextOf(status)).Pad(24).BindWidth(width)
```

| Area | Contract |
| --- | --- |
| Literal vs reader | Last assignment wins. X removes a reader even when the current values are equal. BindX requires a non-nil reader. |
| Value setters | Work after mount, independently of whether BindX is available. Equal assignments are no-ops. |
| Structural setup | Configure children, identity, orientation, input modes and formatter/renderer/event callbacks before mount; rebuild for changes. |
| Compound setters | Size writes Width and Height; Pad writes Padding; Style writes only specified fields. |
| Selection lists | Select, Combobox, Radios and ToggleGroup take the selected binding; Options or BindOptions provides an owned shallow snapshot. |
| Text | Content and BindContent replace any source. TextOf and Textf pull during layout and own no computation. Sprintf explicitly creates a derived string. |
| Names and disabled state | Use Name/BindName and Disabled/BindDisabled. Custom controls use Interactive.SetName/BindName and SetInert/BindInert/IsInert. |
| Identity | Fluent Key returns the control. Custom controls use Interactive.SetKey. Configure keys before mount. |

Readers are borrowed, never disposed by a property. Keep their owners alive as
long as the widgets using them. A custom Get that reads a signal participates in
tracking; plain external storage still needs explicit invalidation. Readers do
not need to be comparable. See [Layout](layout.md) and [Reactivity](reactivity.md)
for lifecycle and cache details.

Bindings to writable view state include `widget.Scroll.BindOffset`,
`widget.Popup.BindOpen`, `ui.Table.BindSelected`, and
`chat.Questionnaire.BindActive`. Their literal counterparts switch to local
view state; user input can still change it.

Description values such as CommandEntry and AccordionSection are configuration
values, not mounted widgets. Their setters keep their value semantics.

`*theme.Theme` pointers follow a read-only vs. fresh-copy split. Constructors
and derivations (`theme.Default`, `theme.Dark`, `theme.From`, `Preset.*`,
`Config.*`, `FromCSS`, and `Clone`) return fresh mutable `*theme.Theme`
pointers that callers may edit before applying. `theme.Use()` and
`theme.Of(env)` return shared read-only `*theme.Theme` pointers without
copying; call `.Clone()` or use `theme.Override` before modifying one.
Compare themes structurally with `a.Equal(b)` rather than `==` (which compares
pointers). See [Styling and themes](styling.md) and the
[ui/theme README](../ui/theme/README.md).
