# The ggui command

[Documentation](README.md) · [Project README](../README.md)

`ggui` creates, runs, checks and bundles ggui applications. It drives the
`go` command on your `PATH` rather than containing a toolchain, so what it
builds is what `go build` would build, with your Go version, workspace and
module cache.

```sh
go install github.com/ironpark/ggui/cmd/ggui@latest
```

| Command | Does |
| --- | --- |
| [`ggui init [dir]`](#ggui-init) | Create a new application. |
| [`ggui dev [package]`](#ggui-dev) | Run it, rebuilding and restarting it whenever its code changes; `--web` serves it to a browser instead. |
| [`ggui build [package]`](#ggui-build) | Build a release: a macOS `.app`, a web bundle or a plain program. |
| [`ggui doctor`](#ggui-doctor) | Check the machine and the project for what ggui needs. |

`ggui help <command>` prints a command's flags. Settings come from
[`ggui.toml`](#ggui-toml), and flags override them. Flags may come before
or after the package, as in `ggui dev ./cmd/app --web`.

## On this page

- [ggui init](#ggui-init)
- [ggui dev](#ggui-dev)
- [ggui build](#ggui-build)
- [ggui doctor](#ggui-doctor)
- [ggui.toml](#ggui-toml)

## ggui init

```sh
ggui init hello
cd hello
ggui dev
```

Init writes a new application into the directory, the current one by
default, creating it if needed:

| File | Holds |
| --- | --- |
| `go.mod` | The module, `--module` or one made from the directory's name, requiring the latest ggui. |
| `main.go` | A small counter using `ggui`, `widget` and `ui`, with the inspector on F1 in development builds. |
| `ggui.toml` | Every setting, each documented, the defaults commented out. |
| `.gitignore` | `build/` and the program `go build` leaves in the directory. |

It then runs `go get github.com/ironpark/ggui@latest` and `go mod tidy`.
It refuses to overwrite any of these files unless given `--force`, and then
writes none of them.

| Flag | Use |
| --- | --- |
| `--module path` | The module path, such as `example.com/hello`. |
| `--name name` | The name people see, the window title and the `.app` name. Default: the directory's name. |
| `--replace dir` | Build against the ggui checkout in `dir` through a `replace` directive, instead of a published version. |
| `--offline` | Write the files only; run `go get` and `go mod tidy` yourself. |
| `--force` | Overwrite the files if they exist. |

If a `go.work` in a directory above the new one is in effect, init says
so: the go command refuses to build a module the workspace does not list,
so add it with `go work use` or set `GOWORK=off`.

## ggui dev

```sh
ggui dev                     # the package app.package names, "." by default
ggui dev ./cmd/app -- --verbose
ggui dev --web --port 3000
```

Dev builds the application with the development tags and runs it, then
watches what it was built from. On a change it waits for the files to go
quiet, rebuilds, and when the build succeeds stops the running app and
starts the new one.

- **A failed build** prints the compiler's errors, in the
  `file:line:col` form editors follow, and leaves the running app alone.
- **An app that exits by itself**, or crashes, is reported, and the next
  change that builds starts it again.
- **Stopping the app** sends it a request to exit, `SIGTERM` on Unix and
  Ctrl-Break on Windows, which a Go program exits on by default and can
  catch to save first. After `--stop-timeout` (3s) it is killed.
- **Ctrl-C** stops the app as above and then ggui. A second Ctrl-C kills
  the app at once.

The app runs in a process group of its own, a job object on Windows, so
the terminal's Ctrl-C reaches ggui rather than the app, and stopping the
app also stops anything it started. On Windows the job ends with ggui
however ggui ends, and on Linux the app is killed if ggui dies; on macOS
an app outlives a ggui that is killed with `SIGKILL`.

### What is watched

`go list -deps` names every package the app is built from, for the
platform and tags of the dev build. Dev watches the files of those that
live in a module being edited: the main module, the other modules of a
`go.work`, and modules replaced by a directory. The standard library and
the read-only module cache are left out. With them it watches each such
package's directory, so a new file is seen, the files the packages embed
with `//go:embed`, the `go.mod`, `go.sum` and `go.work` in effect,
`ggui.toml`, and anything `dev.watch` names. The list is made again after
each change, as an edit may have added an import.

Watching polls modification times, a few times a second. A program is
built from a few hundred to a few thousand files, and checking them costs
far less than a millisecond, so polling needs no dependency and no
per-platform code, holds no file descriptor per file, and sees every way
an editor saves, including writing a new file and renaming it over the
old one.

### Flags

| Flag | Use |
| --- | --- |
| `--tags a,b` | Build tags, replacing `dev.tags`. `--tags ""` builds with none. |
| `--no-watch` | Build and run once; dev exits when the app does. |
| `--stop-timeout 3s` | How long the app has to exit when asked before it is killed. |
| `--config file` | Read this file instead of the `ggui.toml` found from the package or the working directory. |
| `-- args` | Arguments for the app, after `dev.args`. |

### In a browser

`--web` builds the app for `js/wasm` and serves it, with a page that shows
the build, the download and the compile as they happen and steps aside
once the app has painted.

- **Every build is pushed to the page.** An open page follows the
  server's builds and reloads itself onto each one that succeeds. One that
  fails is shown over the page, which keeps running the build before it.
  A page whose own load failed reloads once a build succeeds.
- **The binary is gzipped** once per build and sent compressed to a
  browser that accepts it, with an `ETag` naming the build, so a reload
  without a rebuild comes from the browser's cache.
- **Routes reload.** Any request for HTML that names no file is answered
  with the page, so an app using `router.Browser` can be reloaded on a
  route. Other files are served from the package's directory, so the app
  can fetch an asset it ships beside its code.

| Flag | Use |
| --- | --- |
| `--port 8080` | The port to serve on. |
| `--host localhost` | The address to listen on. `--host 0.0.0.0` makes the server reachable from other devices, such as a phone on the same network; it also serves them the package's directory. |
| `--base /app/` | Serve below a path, replacing `web.base`, as a host serving the app from a subdirectory would. |
| `--wasm-opt -O2` | Run binaryen's `wasm-opt` on each build. Off by default here: it adds seconds to every rebuild and takes about 2% off the binary. |

With `--no-watch` the server builds once and keeps the build, and
`<base>/_rebuild`, or reloading the page after a failed build, builds
again.

## ggui build

```sh
ggui build                    # macos on a Mac, native elsewhere
ggui build --target web
ggui build --target macos --universal
```

Build makes a release build: without the development tags, with
`-trimpath`, and with `-ldflags "-s -w"` followed by `build.ldflags`. ggui
has no version variable of its own to stamp; to stamp yours, add
`-X main.version=1.2.0` to `build.ldflags`. Output goes to `build.output`,
`build/` by default, or to `--out`. The path of what was built is printed
on standard output, for scripts.

| Target | Writes |
| --- | --- |
| `macos` | `Name.app`: the program in `Contents/MacOS`, `Info.plist` from `[app]` and `[macos]`, and `Contents/Resources/AppIcon.icns` made from `app.icon`. The bundle is signed with `macos.signing_identity`. |
| `web` | A directory, `build/web/` by default, for any static host: `index.html`, `main.wasm` and the Go toolchain's `wasm_exec.js`, each beside a gzipped `.gz` copy. |
| `native` | The program, for the `GOOS` and `GOARCH` the go command builds for. |

| Flag | Use |
| --- | --- |
| `--target macos\|web\|native` | What to build. |
| `--out dir` | The output directory, replacing `build.output`. |
| `--universal` | With `--target macos`, build for Apple silicon and Intel and join the two with `lipo`. Needs macOS with the Xcode command line tools. Same as `macos.universal`. |
| `--tags a,b` | Build tags, replacing `build.tags`. |
| `--ldflags flags` | Linker flags after `-s -w`, replacing `build.ldflags`. |
| `--config file` | Read this file instead of the `ggui.toml` found. |

### The macOS bundle

The icon is written by ggui itself, without `iconutil` or `sips`, so a
bundle cross-built on Linux or Windows has it too. Every size from 16 to
1024 pixels is scaled down from the PNG by averaging in linear light. A
source smaller than 1024 pixels gives an icon without the larger sizes,
and macOS scales up the largest it has.

On macOS the bundle is signed with `codesign`. The default identity, `-`,
signs ad hoc, which runs on the Mac that built it. A Developer ID identity
also gets the hardened runtime and a secure timestamp, which notarization
needs; ggui does not notarize. Elsewhere the bundle is left unsigned, with
a notice: sign it on a Mac before shipping it.

Without `--universal` the program is built for the go command's `GOARCH`,
so `GOARCH=amd64 ggui build` makes an Intel bundle.

### The web bundle

The page loads its files relative to `web.base`, which it writes as its
`<base>`. Serve the directory at that path, and have the host answer every
route below it with `index.html` if the app uses `router.Browser`. A host
that serves precompressed files, such as nginx with `gzip_static on` or
Caddy with `precompressed gzip`, sends the `.gz` copies, a little over a
quarter of the binary's size.

## ggui doctor

```sh
ggui doctor
ggui doctor --json
```

Doctor prints a line for each check: `ok`; `warn`, which works but lacks
something or is worth knowing; or `fail`, which will not work until fixed.
It prints what to do under each line that is not ok, and exits with status
1 when a check failed. `--json` prints
`{"ok": bool, "checks": [{"name", "status", "detail", "fix"}]}`.

| Check | Looks at |
| --- | --- |
| go version | The toolchain against the `go` line of the go.mod of the ggui the project resolves. |
| target | Whether ggfx and ggwd support `GOOS/GOARCH`; whether cgo is on, and on macOS whether the Xcode command line tools that a cgo build calls are installed. ggui needs no cgo, so `CGO_ENABLED=0` avoids needing them. |
| ggui, ggfx, ggwd | The versions the project resolves, or the local directories that replace them, and whether ggfx and ggwd are the versions that ggui's go.mod requires. |
| ggui.toml, icon | Whether the file is valid, and the icon a square PNG of at least 64 pixels, ideally 1024. |
| platform | What ggwd and ggfx load when an app starts. |
| tools | `wasm-opt`; on macOS `codesign`, and `lipo` for `--universal`. |

ggwd and ggfx use no cgo: they load the system's libraries when the
program starts, so a missing one is not a build error but an app that
fails to open its window. Doctor looks for each library on the machine it
runs on:

| System | Required | Optional |
| --- | --- | --- |
| macOS | macOS 13 or later, which Go 1.27 programs need. The AppKit, Carbon, CoreFoundation, CoreGraphics, CoreVideo, Foundation, IOKit, Metal, OpenGL and QuartzCore frameworks: all load at start-up, OpenGL even when drawing with Metal. | |
| Linux, FreeBSD, NetBSD | An X11 display: there is no Wayland backend, so a Wayland session needs XWayland and `DISPLAY`. `libX11`; `libGLESv2` or `libGL`; GLX or `libEGL` to make the context; a driver offering OpenGL 3.3 or OpenGL ES 3.0, checked with `glxinfo` when it is installed. | `libXi`, `libXrandr`, `libXcursor`, `libXinerama`, `libXrender`, `libXext`; `fc-match` for system fonts; `zenity` or `kdialog` for file and message dialogs; `xdg-open`. |
| Windows | Direct3D 11 (`d3d11.dll`, `dxgi.dll` and a `d3dcompiler_4x.dll`), or OpenGL 3.3 through the GPU driver, which ggfx falls back to. | `uiautomationcore.dll`, for screen readers. |

## ggui.toml

`ggui.toml` sits at the root of an application, beside its `go.mod`, or
in the directory of its main package. A command given a package looks for
the file from the package's directory up to the module root, and otherwise
from the working directory. Paths in the file are relative to it.

Every key has a default, so a project without the file works, and a key
set in the file replaces its default whole: `tags = []` means no tags. A
key the schema does not know is an error, with its line and the key most
likely meant, rather than a setting silently ignored.

```toml
[app]
name = "Hello"
id = "com.example.hello"
version = "1.0.0"
icon = "icon.png"

[macos]
signing_identity = "Developer ID Application: Example Ltd (ABCDE12345)"

[macos.plist]
NSCameraUsageDescription = "Scans documents."

[web]
base = "/hello/"
```

### [app]

| Key | Default | Use |
| --- | --- | --- |
| `name` | the main package directory's name | The name people see: the `.app` name, its menu title and the page title. |
| `id` | made from the import path, such as `com.github.alice.hello` | The reverse-DNS bundle identifier. macOS keys settings, permissions and the Dock to it, so set it once and keep it. |
| `version` | `"0.1.0"` | One to three numbers: `CFBundleShortVersionString` and `CFBundleVersion`. |
| `icon` | none | A square PNG, ideally 1024×1024, for the macOS icon. |
| `package` | `"."` | The main package's directory. A package given on the command line replaces it. |

### [build]

| Key | Default | Use |
| --- | --- | --- |
| `tags` | `[]` | Build tags for `ggui build`. |
| `ldflags` | `""` | Linker flags added after `-s -w`. |
| `output` | `"build"` | The directory `ggui build` writes to. |

### [dev]

| Key | Default | Use |
| --- | --- | --- |
| `tags` | `["ggui_inspector", "ggui_debug"]` | Build tags for `ggui dev`. `ggui_inspector` builds in the [inspector](inspector.md); `ggui_debug` panics on a state write from the wrong goroutine and names where effects were made. A release build has neither. |
| `args` | `[]` | Arguments the app is run with. |
| `env` | `{}` | Environment variables set for the app, such as `{ LOG_LEVEL = "debug" }`. |
| `watch` | `[]` | More files or directories to watch, such as assets the app reads at run time. |

### [macos]

| Key | Default | Use |
| --- | --- | --- |
| `minimum_system_version` | `"13.0"` | `LSMinimumSystemVersion`. Go 1.27 programs need macOS 13 whatever this says. |
| `category` | none | `LSApplicationCategoryType`, such as `"public.app-category.productivity"`. |
| `signing_identity` | `"-"` | The `codesign` identity. `-` signs ad hoc; list your identities with `security find-identity -v -p codesigning`. |
| `entitlements` | none | An entitlements plist to sign with. |
| `universal` | `false` | Build for Apple silicon and Intel; see `--universal`. |
| `[macos.plist]` | none | Extra `Info.plist` keys of any TOML type but dates, written over the generated ones. |

### [web]

| Key | Default | Use |
| --- | --- | --- |
| `title` | `app.name` | The page title. |
| `base` | `"/"` | The path the app is served below. An app using `router.Browser` passes the same path to it. |
| `wasm_opt` | `"-O2"` | `wasm-opt` flags for `ggui build --target web`; `""` skips it. Without `wasm-opt` on `PATH` the binary ships as linked. |
