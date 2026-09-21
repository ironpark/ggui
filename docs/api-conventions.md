# Public API conventions

[Documentation](README.md) · [Forms](forms.md) · [Layout](layout.md)

Set a literal with `X(value)`, follow a source with `BindX(reader)`, and pass a
`Binding[T]` when a control edits a value. Both forms return the concrete widget
so expressions chain. There is no public property wrapper.

```go
ui.TextField(name).Name("Display name").BindDisabled(saving)
ui.Select(selected).BindOptions(available)
ggui.Box(ggui.TextOf(status)).Pad(24).BindWidth(width)
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

Bindings to writable view state include Scroll.BindOffset, Popup.BindOpen,
Table.BindSelected and Questionnaire.BindActive. Their literal counterparts
switch to local view state; user input can still change it.

Description values such as CommandEntry and AccordionSection are configuration
values, not mounted widgets. Their setters keep their value semantics.
