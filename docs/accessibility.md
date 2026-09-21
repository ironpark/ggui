# Accessibility

[Documentation](README.md) · [Project README](../README.md)

Expose meaningful names and actions, and support text scaling and reduced motion.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

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
labels, and fields use a label or placeholder; custom content may need `.Name`.
Roles and names also help the inspector and `Probe` identify widgets.

Apply text scaling or reduced motion through inherited values:

```go
ggui.Provide(ggui.TextScaleKey, 1.5,
	ggui.Provide(ggui.ReducedMotionKey, true, tree),
)
```

Text scaling affects `Text` and `TextInput`. Reduced motion makes transitions and
control motion complete immediately. Custom controls can respect it through
`env.Motion(d)`; see [Styling and themes](styling.md#accessibility-preferences).

## Custom semantics and actions

Use `ggui.Node` to describe a role, name, value, state, and supported actions.
An interactive widget can implement `Describer` and `Actor`; register it with
`Canvas.Describe` or `DescribeNode` when painting. Noninteractive content can
use `Canvas.Leaf`, and `Canvas.Node` groups semantic children. Keep semantic
names descriptive and avoid exposing decorative content as extra controls.

`app.Semantics()` returns the last published `SemTree`. A snapshot may be read
from any goroutine and retained across frames, but its shared slices must not
be modified. `app.Perform(node.ID, action)` queues a supported action on the UI
thread; observe the result in a subsequent snapshot. See
[semantic action tests](testing.md#state-layout-and-semantic-actions) for a
headless example.

For status updates that should be spoken without moving focus, use
`app.Announce("Saved", ggui.Polite)`. `ggui.Assertive` requests an interrupting
announcement. Announcements may be queued from any goroutine; native delivery
requires the active macOS bridge.
