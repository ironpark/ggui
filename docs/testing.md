# Testing

[Documentation](README.md) · [Project README](../README.md)

Exercise layout, state, and interaction without opening an application window.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## On this page

- [State, layout, and semantic actions](#state-layout-and-semantic-actions)
- [Writes from the wrong goroutine](#writes-from-the-wrong-goroutine)
- [Pixels](#pixels)

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
	if !ggui.Peek(on) {
		t.Fatal("expected alerts to be enabled after tapping the checkbox")
	}
}
```

| API | Use it for |
| --- | --- |
| `NewProbe(widget, size)` | Testing a widget constructed in advance. |
| `ProbeBuilder(build, size)` | Testing app-style setup: the root builder runs once; use explicit reactive blocks for updates. |
| `Host` | The surface `App`, `Window`, and `Probe` share (`Post`, `OnFrame`, `OnKey`, `Shortcut`, `OnDrop`, `Dialogs`, `OnCloseRequest`, `Clipboard`, `PerformAction`, `Announce`, `Semantics`, `Notify`, `RemoveNotification`, `OnNotification`, `Close`), so the same setup function drives both app and tests. |
| `Tap(label)`, `Find(label)`, `FindRole(role, label)` | Locating controls by semantics. |
| `Click`, `Press`, `Move`, `Release`, `Scroll` | Pointer interaction. `Move` hovers the region under the pointer and the regions it is painted inside, as a window does. |
| `Key("cmd+v")`, `Type`, `Text` | Shortcuts, written as `ParseChord` reads them so `cmd` is ⌘ on macOS and Ctrl elsewhere; other key events; text entry. |
| `Advance(duration)` | Advancing this probe's clock for animation. |
| `SetIME(fn)` | Giving the text inputs the test builds a fake input method, which composes and commits through the `IMEHandler` it is handed. |

> [!IMPORTANT]
> Each probe has its own reactive runtime, animations, clock and
> environment, so probe tests may call `t.Parallel`: one probe's frames never
> run another's effects, and `Advance` moves only its own time. `SetEnv`, a
> theme set with `ui/theme`, `SetClock`, `SetDefaultFont`, `SetDefaultMonoFont`,
> `SetEmojiFont` and `SetIME`
> change the probe whose frames the test runs, or before its first frame
> what every probe the test makes starts from; either way they stay on the
> test's goroutine. A subtest runs on a goroutine of its own, so a setting
> its probes need is made in the subtest. Keep each parallel test to its
> own signals, and call `Close`, usually with `defer`.
>
> Three settings remain process-wide, and tests that change them must not be
> parallel: `RegisterInspector`, `a11y.SetWantsDetail` (which says whether an
> assistive technology is reading), and `widget.SetImageCacheLimit`.

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
p.PerformAction(node.ID, ggui.Action{Kind: ggui.ActionPress})
```

A `WindowProbe` drives a real window's frames, damage and all, and reads
the platform's events as a window does: `Wheel` turns a wheel by notches,
`Trackpad` scrolls by pixels, and `TrackpadGesture` scrolls in a phase of a
trackpad's gesture, which stays with the view that took it.

`PerformAction` queues the action and runs a frame. Test the resulting binding or
semantic state.

## Writes from the wrong goroutine

Run an app's tests with `-tags ggui_debug` as well:

```sh
go test -tags ggui_debug ./...
```

In that build a `Probe` or `WindowProbe` checks, as an app's window does,
that its state is touched only on the goroutine running its frames. A worker
that writes a signal directly, `go func() { m.Results.Set(rows) }()`, panics at
the `Set`, naming both goroutines, instead of racing the next frame; hand the
result back with `p.Post` (`App.Post`, `ggui.Async` or `ggui.Resource` in the
app) and it runs in the frame, where nothing is reported.

What is checked is every `Get` and `Set` of a `State` and every `Get` of a
`Derived`, and the creation of an `Effect`, a `Resource` or a `Debounced`, a
`Resource.Reload`, `UIThread`, the font settings and router navigation. Each
probe's frames mark its own reactive runtime, and the base runtime of the
goroutine that made the probe, with the goroutine running them. A signal
belongs to the runtime it was made in: its owner's, or with none its
goroutine's base, as state a test makes before building its probe does. An
access is reported only when that runtime's frames run on another goroutine.
So:

- Parallel tests, each with its own probe, never trip each other.
- A test may make and set state before its probe's first frame, and between
  frames, on its own goroutine.
- Work posted from a worker runs on the frame's goroutine and passes; state
  a worker makes for itself and hands over through `Post` is the worker's,
  and is not checked.
- Closing a probe ends the check of its runtime, and the next probe's first
  frame starts it again on the goroutine that runs that probe.

The release build carries none of this.

## Pixels

A probe on its own paints with no renderer, so a test cannot read what it
drew. The `ggtest` package gives a package's tests a frame of the renderer
with no window: a headless OpenGL context on macOS, a surfaceless EGL one on
Linux. `Render` then paints a probe's frame and reads it back.

```go
func TestMain(m *testing.M) { ggtest.Main(m) }

func TestTheBadgeIsRed(t *testing.T) {
	p := ggui.NewProbe(badge(), ggui.Sz(80, 24))
	defer p.Close()
	img := ggtest.Render(p, ggui.Sz(80, 24), 2) // 160×48 device pixels
	_ = img.RGBAAt(80, 24)
}
```

Every test under `ggtest.Main` runs in that one frame on the main thread, so
none of them calls `t.Parallel`. `RenderWindow` reads a `WindowProbe`'s
screen the same way. Pixels can differ slightly between GPUs: compare with a
tolerance. For whole catalogs of charts and controls, see the
[chart](charts.md#complete-reference-catalog-and-visual-validation) and
[control](carousel-and-otp.md#examples-and-validation) render catalogs.

See the [counter tests](examples/counter/main_test.go) for a full app
example, the [todo tests](examples/todo/main_test.go) for keyed rows and a
dialog, and the [settings tests](examples/settings/main_test.go) for
waiting on work posted from a worker.
