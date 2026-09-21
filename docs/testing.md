# Testing

[Documentation](README.md) · [Project README](../README.md)

Exercise layout, state, and interaction without opening an application window.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

`Probe` drives layout and input without opening a window. It uses the app's frame
steps and hit regions, so tests exercise the same interaction routing.

This complete test checks a checkbox by its accessible label:

```go
package example_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestCheckbox(t *testing.T) {
	on := ggui.State(false)
	p := ggui.NewProbe(ui.Checkbox(on, "Enable alerts"), ggui.Sz(240, 40))
	defer p.Close()

	p.Tap("Enable alerts")
	if !ggui.Untrack(on.Get) {
		t.Fatal("expected alerts to be enabled after tapping the checkbox")
	}
}
```

| API | Use it for |
| --- | --- |
| `NewProbe(widget, size)` | Testing a widget constructed in advance. |
| `ProbeBuilder(build, size)` | Testing app-style setup: the root builder runs once; use explicit reactive blocks for updates. |
| `Host` | The surface `App` and `Probe` share (`Shortcut`, `OnKey`, `OnFrame`, `Post`, `Perform`, `Announce`, `Semantics`, `Close`), so the same setup function drives both. |
| `Tap(label)`, `Find(label)`, `FindRole(role, label)` | Locating controls by semantics. |
| `Click`, `Press`, `Move`, `Release`, `Scroll` | Pointer interaction. |
| `Type`, `Text` | Key events and text entry. |
| `Advance(duration)` | Advancing the test clock for animation. |

> [!IMPORTANT]
> Probes share the reactive runtime, theme, and clock. Run probe tests serially
> (do not call `t.Parallel`) and always call `Close`, usually with `defer`.

## State, layout, and semantic actions

Call `Setup` before the first frame to register owned effects or theme bindings.
Use `Frame()` after changing state directly, or `Resize(size)` followed by
`Frame()` to test another viewport. Input helpers already drive frames.
`Flush()` settles reactive work and layout without painting; use `Frame()`
when you need fresh hit regions or semantics.

`Probe.Semantics()` runs a frame and returns its semantic snapshot. `FindAll(role)` finds all
controls with a role. To exercise the same action path as an accessibility client:

```go
node, ok := p.Semantics().Find(ggui.RoleCheckbox, "Enable alerts")
if !ok {
    t.Fatal("checkbox is missing from the semantic tree")
}
p.Perform(node.ID, ggui.Action{Kind: ggui.ActionPress})
```

`Perform` queues the action and runs a frame. Test the resulting binding or
semantic state. Headless paint does not validate GPU pixels; use the
[chart](charts.md#complete-reference-catalog-and-visual-validation) and
[control](carousel-and-otp.md#examples-and-validation) render catalogs for
visual checks.

See the [counter tests](../examples/counter/main_test.go) for a full app
example, the [todo tests](../examples/todo/main_test.go) for keyed rows and a
dialog, and the [settings tests](../examples/settings/main_test.go) for
waiting on work posted from a worker.
