# Development

[Documentation](README.md) · [Project README](../README.md)

Run these commands from the repository root with Go 1.27+.
[Task](https://taskfile.dev) v3 provides shortcuts for the repository workflows.
On macOS, install it with `brew install go-task`.

## On this page

- [Build and check](#build-and-check)
- [Run native examples](#run-native-examples)
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
| `task fmt` | Format Go sources. |
| `task tidy` | Update module dependency metadata. |
| `task bench COUNT=6` | Run package benchmarks repeatedly for comparison. |
| `task clean` | Clean Go artifacts and remove `bin/`, including app bundles. |

Override the Go executable with `task build GO=/path/to/go` when needed.
To enable reactive-thread diagnostics and source locations for runtime errors:

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

On macOS, the following commands build and launch application bundles:

```sh
task run
task run-todo
task run-gallery
task bundle EXAMPLE=gallery
```

`task bundle` accepts `counter`, `todo`, or `gallery` and builds
`bin/ggui-<example>.app` with its executable and `Info.plist`. The run tasks launch
it with `open -n`, so each invocation starts a new instance. These local builds
are not distribution-signed or notarized. Other platforms use the `go run`
commands above; chart and control examples also run directly on macOS.

## Run in a browser

```sh
task serve-gallery
task serve EXAMPLE=todo PORT=3001
```

The server builds the example for `js/wasm` and serves a loading page. Open
`http://localhost:3000` for `serve-gallery`, or the port you selected. The Task
entry accepts `counter`, `todo`, and `gallery`.

Build output is cached for the lifetime of the server. After editing code,
request `/_rebuild` on the same server to invalidate the build, then reload the
app page. The endpoint returns HTTP 204; it does not navigate or rebuild by itself. Reloading after a failed build retries it; a normal reload of a successful
build uses the cache.

If Binaryen's `wasm-opt` is installed, the server uses `-O2` by default; without
it, the linked binary is served. To explicitly skip optimization, run the server
directly (the Task default substitutes `-O2` for an empty option):

```sh
go run ./internal/tools/serve -http :3000 -wasm-opt="" ./examples/gallery
```

The server uses `go` from `PATH` for its child build, even when the Task launcher
uses `GO=/path/to/go`. Put the intended toolchain on `PATH` for WASM builds.
This is a development server; stop it when finished.

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
| Reactivity and component ownership | [signal.go](../signal.go), [widget.go](../widget.go), [for.go](../for.go) |
| Layout, geometry, and drawing | [widgets.go](../widgets.go), [geometry.go](../geometry.go), [canvas.go](../canvas.go) |
| Themed controls | [ui/](../ui/) |
| Text editing and fonts | [editor.go](../editor.go), [font.go](../font.go), [internal/textinput/](../internal/textinput/) |
| Input and shortcuts | [input.go](../input.go), [chord.go](../chord.go), [clipboard.go](../clipboard.go) |
| Accessibility and semantics | [a11y.go](../a11y.go), [a11y_darwin.go](../a11y_darwin.go), [semantics.go](../semantics.go) |
| Styling and animation | [style.go](../style.go), [anim.go](../anim.go), [transition.go](../transition.go) — see [Styling and themes](styling.md) |
| Overlays and images | [popup.go](../popup.go), [tooltip.go](../tooltip.go), [image.go](../image.go) |
| Testing and diagnostics | [probe.go](../probe.go), [inspector.go](../inspector.go) (build tag `ggui_inspector`; [inspector_stub.go](../inspector_stub.go) otherwise), [cache.go](../cache.go) |
| Runnable applications | [examples/](../examples/) |
