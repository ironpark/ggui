# Public API conventions

[Documentation](README.md) · [Forms](forms.md) · [Layout](layout.md)

The public API separates construction, reactive bindings and explicit runtime
operations. These decisions apply to both built-in and custom controls.

| Area | Decision | Reason |
| --- | --- | --- |
| Disabled state | Keep `Disabled` and `DisabledWhen` together on controls, including wrappers. | Consumers should not need to rebuild a control to disable it reactively. |
| Input identity | Public control `Key` setters return their concrete pointer type. | Identity belongs inside normal widget expressions and fluent chains. |
| Selection options | Own option snapshots; Select and Combobox offer `Options` and `OptionsWhen`. | External slice mutation must not separate displayed labels from selected values; list updates should preserve the mounted control. |
| Formatted reactive text | Keep `GetAny` dispatch and adapt custom readers with `Derived(reader.Get)`. | Preserve the small `Readable` interface and avoid reflection or a redundant formatting adapter. |
| Setter lifecycle | Configure before layout by default; document runtime operations explicitly. | Layout itself configures child widgets. Invalidating from every setter would cause unnecessary layouts on still frames. |

Combobox already supported `DisabledWhen`; the convention check now covers
exported pointer-receiver setters rather than only direct `Interactive` embeds.
Description values such as `CommandEntry` intentionally keep static options.

See [selection options](data-and-navigation.md#select-and-menu) for the complete
replacement, missing-value, query and highlight rules. Snapshot ownership is
shallow: nested referenced objects remain the application's responsibility.
Static lists such as Radios can still be rebuilt with `View` when needed.

Text layout caches include the resolved line height, so inherited line-spacing
changes also update the measured height. This is required even when a widget
keeps the same text, font, size and wrapping width.

## Current API

Controls expose fluent `Key(id)` methods. Custom control implementations set
input identity through `Interactive.SetKey(id)`; `Interactive` itself does not
provide a `Key` method. There are no aliases for the previous signatures.

Use `Button.Secondary` for the secondary variant and
`InspectorOptions.ShowOutlines` to enable outlines. Outlines are off by default.

## Verification

Convention tests cover disabled-state pairs and fluent keys, including inherited
methods. Probe tests cover identity across moving rebuilds, popup state, keyboard
navigation, option replacement under caches, search preservation and binding
replacement. Text tests cover line-height changes and custom-reader formatting.
An idle-frame test guards against configuration causing repeated layouts.
