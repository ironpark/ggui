# Development

[Documentation](README.md) · [Project README](../README.md)

Run these commands from the repository root with Go 1.27+.
[Task](https://taskfile.dev) v3 provides shortcuts for the repository workflows.
On macOS, install it with `brew install go-task`.

## On this page

- [Build and check](#build-and-check)
- [Run native examples](#run-native-examples)
- [The dev viewer](#the-dev-viewer)
- [Run in a browser](#run-in-a-browser)
- [Visual checks](#visual-checks)
- [Repository layout](#repository-layout)

## Build and check

| Command | Purpose |
| --- | --- |
| `task` or `task all` | Format, vet, and test all packages. Formatting may modify Go sources. |
| `task build` | Build all packages. |
| `task test` | Run `go test ./...`. |
| `task vet` | Run `go vet ./...`. |
| `task lint` | Run `staticcheck` once per platform, since code one platform does not compile looks unused on it. |
| `task check` | Format, vet for this system, Windows and Linux, and lint: what CI most often fails a push for, without running the tests. |
| `task hooks` | Install a git `pre-push` hook that runs `task check`; `git push --no-verify` skips it once. |
| `task fuzz FUZZTIME=1m` | Run every fuzz target for `FUZZTIME` each, 10s by default. |
| `task fmt` | Format Go sources. |
| `task tidy` | Update module dependency metadata. |
| `task bench COUNT=6` | Run package benchmarks repeatedly for comparison. |
| `task bench:save NAME=base` | Save the engine packages' benchmarks (reactivity, fonts, text layout, motion, virtual lists, repaint regions, geometry, the image cache, layer images, Markdown parsing) to `.bench/NAME.txt`. `PKGS`, `BENCH` and `COUNT` narrow or widen the run. |
| `task bench:compare REF=main` | Run the same benchmarks now and compare them by `benchstat` with a baseline: `.bench/BASE.txt`, or `REF` measured in a temporary worktree of that commit. |
| `task clean` | Clean Go artifacts and remove `bin/`, including app bundles. |

Override the Go executable with `task build GO=/path/to/go` when needed.
To enable reactive-thread diagnostics (see
[Testing](testing.md#writes-from-the-wrong-goroutine)) and source locations for
runtime errors:

```sh
go test -tags ggui_debug ./...
go run -tags ggui_debug ./examples/counter
```

The [widget inspector](inspector.md) is behind its own tag, so that a
release binary carries none of it:

```sh
go run -tags ggui_inspector ./examples/gallery
```

`task test`, `task build` and `task vet` each run both configurations, and
the `task run-*` and `task serve` loops set the tag. `task bundle` does not.

See [Testing](testing.md) for `Probe`, and [Rendering](rendering.md#frame-lifecycle)
for diagnosing effects that fail to settle.

### Continuous integration

`.github/workflows/ci.yml` runs on every push to `main` and every pull
request:

- **test**, on Linux, macOS and Windows: `go test` plain, with
  `ggui_inspector` and with `ggui_debug`, and with `-race` everywhere but
  Windows, whose runner has no C toolchain for it. It also tests the
  `ggwd_remote` build that `ggui dev` shows in its viewer, and builds
  `examples/counter` with it.
- **remote**, on Linux, macOS and Windows: runs the viewer and
  `examples/counter` with the remote backend end to end, on the runner's
  window system and GPU (`cmd/ggviewer`'s `TestEndToEnd`, which
  `GGUI_E2E=1` turns on): the app attaches and shows its first frame,
  takes input, shows a failed build's errors over its frame, follows a
  resize, restarts into the same window, and gets its frames through
  while the window is hidden or minimized. Linux runs it under Xvfb with
  openbox and Mesa's software OpenGL, Windows on WARP, Direct3D's software
  rasterizer; the window's pictures are kept as the job's artifact. `examples/sqlite`, the
  other nested modules and `cmd/ggui` are tested through a `go work init`
  workspace, so they build against the checkout rather than a published
  ggui; `cmd/ggui`'s tests include building the app `ggui init` writes.
- **wasm**: builds everything for `js/wasm` and runs the packages that do
  not start ggfx under node, `internal/reactive/goid` among them.
- **lint**: `gofmt`, `go vet`, the Windows vet of `task vet:windows`,
  `task lint`'s staticcheck for every platform, and builds for
  linux/riscv64 and windows/arm64.
- **fuzz**: every fuzz target for 5s. `go test` already runs their seed
  inputs; this goes a little past them. A failing input lands in the
  package's `testdata/fuzz`: commit it with the fix, and it stays a test.

A test that presses a shortcut should use `Probe.Key("cmd+k")` rather
than a `Mods` literal: `cmd` is ⌘ on macOS and Ctrl elsewhere, and the
Linux and Windows jobs press Ctrl.

## Run native examples

```sh
go run ./examples/counter
go run ./examples/todo
go run ./examples/gallery
go run ./examples/charts
go run ./examples/controls
go run ./examples/settings
go run ./examples/custom
```

The [ggui command](cli.md) in [cmd/ggui](../cmd/ggui) runs any of them
with live rebuilds, restarting the app on every change:

```sh
go build -C cmd/ggui -o ../../bin/ggui .
bin/ggui dev ./examples/gallery
bin/ggui dev --web ./examples/gallery
```

It is a module of its own, so it is built from its directory and run from
here; the Task entries below do the same.

### The dev viewer

On macOS, Linux and Windows `ggui dev` shows the app in
[`cmd/ggviewer`](../cmd/ggviewer/),
a long-lived process that owns the OS window, rather than in a window of
the app's own. The app is an ordinary native build with the `ggwd_remote`
tag, started with `GGWD_REMOTE_SOCKET` naming the viewer's unix socket.
Under that tag and variable ggwd opens no window and ggfx no GPU device:
ggfx's remote driver records the frame's driver calls, images, shaders,
draws and presents, as a command stream, which goes to the viewer in
batches through shared memory, and `ggfx/remote`'s Replayer runs them
against the viewer's own device into its window. Input, the window's
size and scale and the system's appearance come back over the socket,
in the protocol of `ggwd/remote`. The viewer acknowledges each batch
once replayed, which paces the app to what is shown.

The transport is a unix socket everywhere, Windows' included (Windows 10
1803 and later). On macOS and Linux the shared memory's descriptor goes
over the socket with `SCM_RIGHTS`: a memfd on Linux, an unlinked
temporary file on macOS. Windows passes no handles over a unix socket,
so there it is a file mapping with a random name in the session's
`Local` namespace, which the viewer opens by name; either way it goes
with the last process that holds it. The replay is checked against a
native draw, pixel for pixel, on Metal, Direct3D 11 and 12, and OpenGL
(ggfx's `internal/rendertest`).

What differs by system is what the window system gives a process without
a window: X11's clipboard belongs to a window, so on Linux the app's
clipboard goes through the viewer's (`ClipboardRequest` and
`SetClipboard` in `ggwd/remote`); Windows lets only the process the user
works in bring a window to the front, so its viewer allows the app's
process to with each click and key press, for the app's dialogs; and
macOS turns the app into an accessory application while a dialog is up
(`runtime/remote_darwin.go`). The tray is off on macOS and Windows.

The point is a restart that keeps the window: a code change rebuilds and
restarts only the app, while the window stays where it was, keeps the
last frame until the new app's first, and shows the build's state and
errors itself. Replaying driver calls rather than pixels keeps the
traffic small and the app's rendering the same code a release build
runs, down to the shaders; the price is that the viewer must speak the
app's ggfx and ggwd exactly, which is why `ggui dev` builds it from the
project's own module graph. What the app gives up in the viewer, one
window, native menus, the accessibility tree, is listed in
[the ggui command](cli.md#the-window-remote-display).

On macOS, the following commands build and launch application bundles:

```sh
task run
task run-todo
task run-gallery
task bundle EXAMPLE=gallery
```

`task bundle` accepts `counter`, `todo`, `workspace`, `gallery`, `create` or
`router` and builds `bin/ggui-<example>.app` with `ggui build --target macos`,
which reads the bundle's name and identifier from the example's
`ggui.toml`. The run tasks launch it with `open -n`, so each invocation
starts a new instance. These local builds are signed ad hoc, not with a
Developer ID, and are not notarized. Other platforms use the `go run`
commands above; chart and control examples also run directly on macOS.

## Run in a browser

```sh
task serve-gallery
task serve EXAMPLE=todo PORT=3001
```

`task serve` runs `ggui dev --web`, which builds the example for `js/wasm`
and serves a loading page. Open `http://localhost:3000` for
`serve-gallery`, or the port you selected. The Task entry accepts the same
examples as `task bundle`.

The server rebuilds when the example's code changes, and an open page
reloads itself onto each build; a build that fails is shown over the page,
which keeps running the build before it. Each rebuild skips Binaryen's
`wasm-opt`, which takes seconds for 2% of the binary; pass `WASM_OPT=-O2`
to run it. `ggui build --target web` runs it by default.

ggui runs `go` from `PATH`, even when the Task launcher uses
`GO=/path/to/go`, so put the intended toolchain on `PATH`. This is a
development server, bound to localhost; stop it when finished.

## Visual checks

The [chart catalog](charts.md#complete-reference-catalog-and-visual-validation)
and [carousel/OTP catalog](carousel-and-otp.md#examples-and-validation) document
GPU render commands and their expected output. These complement headless tests,
which validate behavior but do not draw GPU pixels. Treat benchmark figures in
those guides as historical measurements; rerun them on your target hardware.

## Repository layout

| Area | Source |
| --- | --- |
| Application and frame loop | [app.go](../app.go), [loop.go](../loop.go) |
| Reactivity and component ownership | [reactivity.go](../reactivity.go), [widget.go](../widget.go), [widget/for.go](../widget/for.go) (`Each` and `EachKeyed`), [pkg/virtual/](../pkg/virtual/) (which rows a virtual list lays out, and where) |
| Layout, geometry, and drawing | [boundary.go](../boundary.go) (`LayoutBoundary`), [widget/layout.go](../widget/layout.go) (boxes, rows, stacks, grids and `Responsive`), [widget/scroll.go](../widget/scroll.go), [canvas.go](../canvas.go) (the frame state, retained values, clips and hit-region registration), [paint.go](../paint.go) (shapes, paths, gradients and layers), [damage.go](../damage.go) and [internal/region/](../internal/region/) (the regions a partial frame repaints), [pkg/layout/](../pkg/layout/) (box layout and rectangles), [pkg/geom/](../pkg/geom/) (affine transforms and path outlines), [internal/render/](../internal/render/) (the shaders behind the Canvas, and the layer images it lends) |
| Unstyled widgets | [widget/](../widget/): layout, text, scrolling, lists, popups, transitions, compositing, pointer and drag and drop, the text input, images and Reveal, written on ggui's public API and the internal packages and importing nothing of `ui`, its theme, icons or fonts |
| UI controls | [ui/](../ui/) (themed components, built on `ggui` and `widget`), [pkg/markdown/](../pkg/markdown/) (the Markdown parser `ui.Markdown` renders) |
| Text editing and fonts | [widget/textinput.go](../widget/textinput.go), [font.go](../font.go), [widget/text.go](../widget/text.go) and [widget/richtext.go](../widget/richtext.go), [internal/fontsys/](../internal/fontsys/) (fonts, fallback chains and emoji), [internal/textlayout/](../internal/textlayout/) (measuring and wrapping), [internal/paragraph/](../internal/paragraph/) (bidi lines, styled paragraphs and CPU rendering), [internal/textedit/](../internal/textedit/) (the buffer, selection, undo, click selection and case-blind find), [internal/editlayout/](../internal/editlayout/) (TextInput's wrap and bidi lines, cached by hard line), [input.go](../input.go) (`IME`, `NewIME`, `SetIME`: the IME session a text widget is given), [internal/platform/textinput/](../internal/platform/textinput/) (the platform IME) |
| Input and shortcuts | [input.go](../input.go) (hit regions, pointer and focus routing, touch), [widget/interact.go](../widget/interact.go) (`Pointer`, `Tap` and `Focus`), [drag.go](../drag.go) (the drag session: a drag inside the app, and files dropped from the desktop), [widget/dragdrop.go](../widget/dragdrop.go) (`DragSource`, `DropZone` and `Reorderable`), [keys.go](../keys.go) (keys, chords and the shortcuts taken before and after the focused widget), [pkg/kinetics/](../pkg/kinetics/) (a touch's pan, velocity and fling) |
| Platform services | [runtime/](../runtime/) (file dialogs and the clipboard, reached through `Host`) |
| Accessibility and semantics | [a11y/](../a11y/) (the vocabulary), [internal/platform/a11ybridge/](../internal/platform/a11ybridge/) (the platform bridges), [semantics.go](../semantics.go) |
| Styling and animation | [environment.go](../environment.go) (styles), [anim.go](../anim.go), [widget/transition.go](../widget/transition.go), [effects.go](../effects.go) (the Canvas's blurs, shadows, edge fades and transforms), [widget/composite.go](../widget/composite.go) (`Opacity`, `Blur`, `BackdropBlur`, `Transform` and `ClipPath` on them), [pkg/kinetics/](../pkg/kinetics/) (easing curves, springs, and scroll and touch physics) — see [Styling and themes](styling.md) |
| Overlays and images | [widget/popup.go](../widget/popup.go) (`Popup`), [overlay.go](../overlay.go) (the window's `Overlay`), [paint.go](../paint.go) (`Canvas.DrawImage`), [widget/image.go](../widget/image.go), [internal/imgcache/](../internal/imgcache/) (the cache `widget.AsyncImage` loads into) |
| Testing and diagnostics | [probe.go](../probe.go), [inspect/panel/](../inspect/panel) (the panel, build tag `ggui_inspector`), [inspector.go](../inspector.go) (its registration, and the frame built from the trace), [inspect/](../inspect) (the frame model the panel and `App.OnInspect` share, and `ReadFrameStats` and `ReadMotionReport`), [internal/framestats/](../internal/framestats/) (the process's frame counters those read), [overlay.go](../overlay.go) (the Overlay the panel draws as), [widget.go](../widget.go) (Cached) |
| Runnable applications | [examples/](../examples/) |
| The ggui command | [cmd/ggui/](../cmd/ggui/): `init`, `dev`, `build` and `doctor`, a module of its own that drives the go command and imports nothing of ggui, and signs, notarizes and packages macOS builds with the packages of [zapp](https://github.com/ironpark/zapp); its [internal/](../cmd/ggui/internal/) packages keep the watcher, the process runner, the builders, the viewer's launcher and the bundle writers apart |
| The dev viewer | [cmd/ggviewer/](../cmd/ggviewer/): the window `ggui dev` shows an app in while it restarts, replaying the app's command stream; [internal/platform/remotedisplay/](../internal/platform/remotedisplay/) is what the app gives up in it |

The module is built in three layers, each importing only those below it:

- **`ggui`**, the root package, is the kernel: the app and its windows, the
  frame loop, the canvas and painting, input and shortcuts, the environment,
  reactivity and component ownership, the widget protocol (`Widget`,
  `Scope`, `LayoutBoundary`, `Invalidate`, `Cached`) and the `Probe` and
  `WindowProbe` that drive a tree in tests. It holds no widget of its own.
- **[`widget`](../widget/)** holds the unstyled building blocks written on
  the kernel's public API: `Text` and `RichText`, `Box`, `Row`, `Column`,
  `Stack` and the other layouts, `Scroll`, `Each`, `Popup`, `Transition`,
  `Opacity` and the other composites, the pointer and drag and drop
  widgets, the text input, images and `Reveal`. They take their looks from
  the `Env` and what they are given.
- **[`ui`](../ui/)** is the themed component library, built on both.

The import graph keeps the layers apart, with no test to guard it: `widget`
cannot reach a name the kernel keeps to itself, and the kernel, tests
included, cannot import a widget back without a cycle. A widget that needs
more of the kernel than it exports gets it exported, as one of an
application's own package would; [internal/apitest](../internal/apitest/)
shows such a package mounting keyed subtrees with `Scope`, holding changes
of size with `LayoutBoundary` and asking for layouts with `Invalidate` and
`Relayout`. The kernel's tests stand on the small widgets of
[fixtures_test.go](../fixtures_test.go), built on the widget protocol, and
`widget`'s tests drive the widgets with a `Probe` or a `WindowProbe`.
`widget` imports none of `ui`, its theme, icons or fonts, which
[widget/imports_test.go](../widget/imports_test.go) checks.

## Release the ggui command

The command is a module of its own, released by a tag on its directory:

```sh
git tag cmd/ggui/v0.2.0
git push origin cmd/ggui/v0.2.0
```

The tag runs `.github/workflows/release-cli.yml`: GoReleaser builds the
command for macOS, Linux and Windows (`cmd/ggui/.goreleaser.yaml`), the
archives become the tag's GitHub release, and a formula goes to the
Homebrew tap [ironpark/homebrew-tap](https://github.com/ironpark/homebrew-tap),
from which `brew install ironpark/tap/ggui` installs it. The formula
(`cmd/ggui/release/formula.sh` writes it) builds the command from its module
zip on the Go module proxy, as formulae are meant to build from source; the
tap's own workflow installs and tests every formula pushed to it on macOS
and Linux. Writing to the tap
takes the repository secret `TAP_GITHUB_TOKEN`, a fine-grained token with
Contents read and write on that repository only.

The command signs and packages macOS builds with zapp's `pkg/` packages,
and two things follow from that dependency. zapp requires ggui and ggfx itself, for its own window, so an
older ggui and ggfx stand in the command's module graph without being
built; in a workspace with `.` they resolve to the checkout, and with
`GOWORK=off` nothing of them is downloaded. And the command imports
zapp's signing and verification packages on macOS only: on Linux and
Windows they would import its rcodesign binding, which a cgo build links
against 30 MB static libraries, and MinGW's GCC on Windows cannot link at
all. To try a release without publishing it:

```sh
GORELEASER_CURRENT_TAG=v0.2.0 goreleaser release --clean --skip=publish,validate -f cmd/ggui/.goreleaser.yaml
```
