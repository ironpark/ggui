# Accessibility

[Documentation](README.md) · [Project README](../README.md)

Expose meaningful names and actions, and support text scaling and reduced motion.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

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

The accessibility layer is the `a11y` package: the vocabulary a widget
describes itself in, the tree one frame publishes, and the platform bridges.
The `ggui` package re-exports every name from it, so `ggui.Node` and
`a11y.Node` are the same type and normal code never imports `a11y` directly.
Geometry sits below both, in `geom`, for the same reason.

The split is what lets a bridge read everything a widget published without a
copy of `Node` that falls behind the original. A new platform is a new file
in `a11y`, implementing the one interface that knows an operating system
exists.

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
