# The ggui command

[Documentation](README.md) · [Project README](../README.md)

`ggui` creates, runs, checks and bundles ggui applications. It drives the
`go` command on your `PATH` rather than containing a toolchain, so what it
builds is what `go build` would build, with your Go version, workspace and
module cache.

```sh
brew install ironpark/tap/ggui
```

Homebrew builds it from source with Go, which it installs too. Without Homebrew, or for a version between
releases:

```sh
go install github.com/ironpark/ggui/cmd/ggui@latest
```

Releases are tagged `cmd/ggui/vX.Y.Z`, since the command is a module of its
own; `go install github.com/ironpark/ggui/cmd/ggui@vX.Y.Z` installs one.
Each release also carries the command built for macOS, Linux and Windows,
Intel and ARM, on the [releases page](https://github.com/ironpark/ggui/releases)
under `ggui command`: unpack the archive and put `ggui` on your `PATH`.
However it was installed, the command needs Go, which builds the
applications it runs. `ggui version` prints the version installed.

| Command | Does |
| --- | --- |
| [`ggui init [dir]`](#ggui-init) | Create a new application. |
| [`ggui dev [package]`](#ggui-dev) | Run it, rebuilding and restarting it whenever its code changes; `--web` serves it to a browser instead. |
| [`ggui build [package]`](#ggui-build) | Build a release: a macOS `.app`, a Windows `.exe`, a Linux install tree with its tarball and `.deb` and `.rpm` packages, a web bundle or a plain program. |
| [`ggui release [dir]`](#ggui-release) | Build the release files for macOS, Windows and Linux, and publish them: a GitHub release, a Homebrew cask, a Chocolatey package. |
| [`ggui doctor`](#ggui-doctor) | Check the machine and the project for what ggui needs. |
| [`ggui i18n check\|extract [dir]`](#ggui-i18n) | Check the translation catalogs against the messages the app passes, or add what they lack. |
| [`ggui record [name]`](#ggui-record) | Draw a tour the app registered into an animated GIF, or PNG frames. |

`ggui help <command>` prints a command's flags. Settings come from
[`ggui.toml`](#ggui-toml), and flags override them. Flags may come before
or after the package, as in `ggui dev ./cmd/app --web`.

On a terminal, ggui shows each step that takes a while as a line with a
spinner, the time so far and the last thing the `go` command printed,
ending in ✓, ! or ✗, in color. When the `go` command has been quiet for
20 seconds, as while a slow network or proxy resolves a module, the line
says so. Piped or redirected, ggui prints a plain line as each step starts
and ends instead, without colors or escape sequences. `NO_COLOR` set to
anything turns colors off; `CLICOLOR_FORCE` or `FORCE_COLOR` (other than
`0`) turns them on even off a terminal; `TERM=dumb` turns off both colors
and spinners. `NO_COLOR` wins over the other two.

## On this page

- [ggui init](#ggui-init)
- [ggui dev](#ggui-dev)
- [ggui build](#ggui-build)
- [ggui release](#ggui-release)
- [ggui doctor](#ggui-doctor)
- [ggui i18n](#ggui-i18n)
- [ggui record](#ggui-record)
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
| `.gitignore` | `build/`, the program `go build` leaves in the directory, and the resources a Windows build that was killed can leave. |

It then runs `go get github.com/ironpark/ggui@latest` and `go mod tidy`,
with `GOWORK=off`. If either fails, or is interrupted with Ctrl-C, the files
stay written and init prints what the `go` command said and the commands
that finish the job. It refuses to overwrite any of these files unless
given `--force`, and then writes none of them.

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
starts the new one, handing it the state the old one
[keeps](#keeping-state-across-a-restart).

- **A failed build** prints the compiler's errors, in the
  `file:line:col` form editors follow, and leaves the running app alone;
  so does a syntax error, found before the build.
- **An app that exits by itself**, or crashes, is reported, and the next
  change that builds starts it again.
- **Stopping the app** sends it a request to exit, `SIGTERM` on Unix and
  Ctrl-Break on Windows, which a Go program exits on by default and can
  catch to save first. After `--stop-timeout` (3s) it is killed.
- **Ctrl-C** stops the app as above and then ggui. A second Ctrl-C kills
  the app at once.
- **Timings**: each restart says how long it took from the change to the
  new app's first frame in the viewer, split into the build and the
  start.

The app runs in a process group of its own, a job object on Windows, so
the terminal's Ctrl-C reaches ggui rather than the app, and stopping the
app also stops anything it started. On Windows the job ends with ggui
however ggui ends, and on Linux the app is killed if ggui dies; on macOS
an app outlives a ggui that is killed with `SIGKILL`.

### Keeping state across a restart

A restart starts the app afresh, which after every save would mean
clicking back to the screen being worked on. State the app marks with
`Keep` is carried over to the new build instead:

```go
count := ggui.State(0).Keep("count")
form := ggui.State(Signup{}).Keep("signup.form")
r := router.New(routes...).Keep("router") // the page it was on
```

Before dev stops the running app, it asks it for the values it keeps,
which the app encodes as JSON, and holds them for the new build, which is
handed them as it starts. A state is restored when `Keep` is called, so one
kept in the app's setup shows its value in the first frame, and one kept in
a page the router mounts takes its value when that page mounts. The app
finds dev's channel in `GGUI_DEV_STATE`: a unix socket in a directory of
the temporary directory that only the user may enter, or on Windows, every
version of it, a named pipe with a random name that only the user may
open and no other machine may reach. Started any other way, or built for
release, the app keeps nothing, and `Keep` costs a check. See
[State and components](reactivity.md#keeping-state-while-developing) for
what is kept and when.

- **A type that changed** between builds is the normal case. A value is
  decoded over a copy of the state's initial value, so a field the new
  build added keeps its initial value and one it removed is ignored; a
  value that no longer decodes is dropped, said once on stderr, and its
  state starts from its initial value.
- **A failed build**, or an app that exits or crashes, leaves the state
  with dev: the next build that starts is handed the last state known.
- **An app that does not answer** within half a second is stopped all the
  same, which dev says in one line, and the new build starts with nothing
  kept: a state that may have hung one build is not handed to the next.
- **Both displays.** It works the same in the viewer and in a window of
  the app's own. With `--no-watch` there is no restart and nothing is
  kept. Unlike the viewer, it needs no unix sockets, so it works on a
  Windows older than 10 1803 too, where the app opens its own window.
- **In a browser**, with `--web`, the page carries the state across its
  reload onto the new build in its `sessionStorage`. A reload by hand
  starts afresh.

### The window: remote display

On macOS, Linux and Windows the app's window belongs to a viewer,
`ggviewer`, a process of its own that dev starts once and keeps for the
session. The app is built
with the `ggwd_remote` tag and started with the viewer's socket in
`GGWD_REMOTE_SOCKET`; it then opens no window and no GPU device: it draws
through ggfx's remote driver, whose command batches go to the viewer
through shared memory, and gets its input, its size and its scale from
the viewer's window. On a change only the app restarts: the window stays
where it was and as large as it was, keeps the last frame while the new
build starts, and the new app's first frame replaces it. The window notes
a running build in a corner, shows a failed build's errors over the last
frame, scrolled with the wheel, and says when the app exits and what it
last wrote to stderr. Closing the window asks the app, as on its own
window; when it closes, dev ends. The window's place and size are kept
per module, under the user's cache directory.

The viewer is built from the project's own module graph, with
`go build github.com/ironpark/ggui/cmd/ggviewer` in the project, so that
it replays the protocol of the ggfx and ggwd the app is built with. The
binary is kept under the user's cache directory, `ggui/viewer`
(`GGUI_CACHE_DIR` replaces the `ggui` there), under a key of what decides
it: the Go version, the target and build environment, and the version of
every module it is built from, or the contents of its files for a module
being edited. A later session starts it at once; a changed dependency
builds another, also when `go.mod` or `go.work` changes during a session,
when dev restarts the viewer and the window closes and opens again once.
The cache keeps the four used last. `GGUI_VIEWER` names a viewer binary to
run instead, for work on the viewer itself.

`--display=window`, or `display = "window"` under `[dev]`, gives each run
a window of its own instead, as a release build has. Dev falls back to it
by itself, saying why in one line, when the viewer cannot be had: on
another system than macOS, Linux and Windows; when the project's ggui has no
`cmd/ggviewer` or its ggwd or ggfx lacks the `remote` package, which dev
learns from `go list` in the project rather than from version numbers;
when the viewer does not build or start. If the viewer goes away during
the session, the app is started again in its own window and stays there.
`ggui doctor` says whether the project gets remote display, and why not.

An app shown by the viewer is the same program as a release build, and
behaves as one, but for what belongs to the window on the desktop. `ggui
dev` says so when the viewer starts, and when the app meets one of these
limits, which it reports on stderr in a line of its own beginning `ggui:`,
`ggui dev` takes that line out of the app's output and warns of it
instead, once a session, with `--display=window` as the way to see what
is missing:

- **One window.** `App.OpenWindow` returns an error while the app runs,
  and a window made before `Run` other than the first does not open, with
  a warning. `ggui dev --display=window` opens them all.
- **Menus.** On macOS, `App.SetMenu`'s native menu bar runs in the
  viewer (`NativeMenu` is true): reactive enabled and checked states stay
  in sync, clicks and shortcuts route back to the app, and the menu bar
  clears when the app detaches. `NativeContextMenu` and `TraySupported`
  report false: `Window.ShowContextMenu` returns false, and when the app
  adds a tray icon `ggui dev` warns once a session that the tray is not
  shown and that `--display=window` shows it. On Windows, too,
  `TraySupported` is false while the viewer shows the app.
- **Dialogs** open, from the app's process, standing alone in front of
  the viewer's window rather than as a sheet on it or owned by it. On
  macOS the viewer is active again once they close; on Windows the viewer
  lets the app's process come to the front each time the window is
  clicked or typed in, so that a dialog opened then is not left behind
  it; on Linux `zenity` or `kdialog` shows them, as in a release build.
- **Clipboard** text copies and pastes through the system clipboard. On
  macOS and Windows the app reaches it itself, as in a release build; on
  Linux, where X11 has the clipboard belong to a window, the app reads and
  writes it through the viewer's.
- **Window settings and state** follow the app live and reset cleanly
  when it detaches or restarts: title, icon, size and size limits,
  position, resizability, frame, always-on-top, hidden title bar
  (`Config.TitlebarHidden`), traffic light placement
  (`Config.TrafficLights`), maximize, minimize, fullscreen, show, hide,
  focus, attention, preferred color mode and Dock icon visibility all
  apply to the viewer's window. `Screens()` lists one screen, the
  viewer's. Cursor shapes and text input, with input methods, work.
- **No accessibility tree**: there is no native window to hang it on,
  so screen readers see the viewer's window as empty.
- The [inspector](inspector.md) and `ggui_debug` work as in a window of
  the app's own.

Remote display shows one window. It needs, on Linux, an X11 display, as
any ggui app does, and a window manager for the window to be minimized;
on Windows, Windows 10 1803 or later, the first with unix sockets, where
the shared memory is a named file mapping, since descriptors do not pass
over those sockets. On an older Windows the viewer does not start and the
app opens its own window. The socket lives in the temporary directory;
on Windows a temporary directory too long for a unix socket's path, as
under a very long user name, leaves the app in its own window too.

### The macOS development bundle

On macOS, `ggui dev` wraps the running program in a `Name.dev.app` bundle
under `build.output` (`build/` by default), or in the directory
`--bundle-dir` or `dev.bundle_dir` names (which the watcher also
ignores). Its `CFBundleIdentifier` is `<app.id>.dev`, so notifications,
settings and permissions stay separate from a release build installed on
the same Mac.

- **Both displays.** With `--display=remote` (the default), the viewer
  runs inside `Name.dev.app` so the Dock icon, menu bar app name and
  `Info.plist` belong to the project, while the app binary runs outside
  the bundle so rebuilding never invalidates the live viewer's code
  signature. With `--display=window`, the app itself runs inside
  `Name.dev.app`. In both modes ggui executes `Contents/MacOS/<exe>`
  directly rather than through `open`, keeping stdout/stderr pipes,
  `dev.env`, Ctrl-C and kept state intact.
- **Incremental updates.** `Info.plist`, `PkgInfo` and
  `Contents/Resources/AppIcon.icns` (including SVG icons) are written
  only when missing or when `ggui.toml`, the icon or entitlements change.
  Regular Go edits swap only the executable inside the bundle and re-sign
  it. In remote mode, a change to the bundle's icon, plist or
  entitlements restarts the viewer once so macOS picks it up.
- **Signing and TCC permissions.** macOS tracks privacy permissions
  (camera, microphone, screen recording, accessibility) by the bundle's
  designated requirement. An ad hoc signature (`-`) keys on the binary's
  hash, which changes on every rebuild and resets those permissions;
  signing with a keychain certificate keys on `<app.id>.dev` and the
  certificate, keeping permissions across rebuilds. `ggui dev` picks the
  signing identity in order:
  1. `--sign` or `dev.identity`
  2. `macos.signing_identity` (when set and not `"-"`)
  3. the first `Apple Development` identity found in the keychain
  4. ad hoc (`"-"`) when none of the above is available.

### What is watched

`go list -deps` names every package the app is built from, for the
platform and tags of the dev build. Dev watches the files of those that
live in a module being edited: the main module, the other modules of a
`go.work`, and modules replaced by a directory. The standard library and
the read-only module cache are left out. With them it watches each such
package's directory, so a new file is seen, the files the packages embed
with `//go:embed`, the `go.mod`, `go.sum` and `go.work` in effect,
`ggui.toml`, and anything `dev.watch` names. The list is made again after
each change, as an edit may have added an import, and whenever a watched
directory gains or loses a file.

What decides that something changed is a scan: a `stat` of every watched
file, compared with the last, which costs well under a millisecond for a
few thousand files and sees every way an editor saves, a write in place,
a new file renamed over the old one, or a delete and a create. The
operating system's notifications only wake the scan up: FSEvents on
macOS, on the roots of the modules being edited; inotify on Linux, once
on each watched directory; `ReadDirectoryChangesW` on Windows, on the
module roots. A save is seen some 20 ms after it is written, and the
build starts 50 ms later if nothing else changes. A scan also runs every
2 seconds whatever is said, for a notification that never came, as on
some network file systems; when the system says it lost notifications,
every watched directory is listed again as well. Editors' temporary
files, such as Vim's `4913` and swap files and Emacs's backups, are not
looked at. The watcher is
[`pkg/fswatch`](../pkg/fswatch/), which any Go program can use. Where there are no notifications,
or when inotify runs out of watches (`fs.inotify.max_user_watches`), dev
says so in one line and scans four times a second instead;
`GGUI_WATCH=poll` in the environment asks for that, for a file system
whose notifications are not to be trusted. ggui needs no cgo for any of
this.

### What rebuilds, and what does not

- **A real change** to a Go file, an embedded file, `go.mod`, `go.sum`,
  `go.work` or `ggui.toml`, or a Go file appearing or going, rebuilds
  once the sources have been quiet for 50 ms, 200 ms after a save-all or
  a branch switch that touched many files, and never later than 500 ms
  after the first change, so that a stream of changes cannot hold the
  build off. A formatter that rewrites a file just saved is in the same
  build.
- **A change to `ggui.toml`** is read again before that build:
  `dev.tags`, `dev.args`, `dev.env` and `dev.watch` take effect in it and
  in the app it is run as, the flags still replacing what they replace,
  and on macOS changes to `app.icon`, `[macos.plist]` or
  `macos.entitlements` update `Name.dev.app` (restarting the viewer once
  in remote mode).
  A file that is not valid is said, with its line, and the settings in
  use stay until it is. `dev.display`, `build.output` and `app.package`
  (and on macOS `dev.bundle_dir`, `dev.identity`, `app.name` and
  `app.id`) are held for the session; ggui says so when one of them
  changed, and starting `ggui dev` again takes it up. With `--web` the
  server holds every setting it started with, and says the same.
- **A change during a build** cancels it, the go command and its
  compilers with it, and builds again after the same wait; the build
  cache keeps what was compiled. Before the app is replaced, dev looks at
  the sources once more, and a build that is already out of date is
  thrown away rather than started.
- **Comments and layout.** An edit that leaves a Go file's tokens as they
  were, a comment, indentation, blank lines, spacing, rebuilds nothing,
  and dev says `comments only; not restarted — line numbers in traces now
  lag the source`: the running app was built from the file as it was, so
  the line numbers in its stack traces and logs point where the code used
  to be until the next real change rebuilds it. Comments that change a
  build count as code: `//go:build` and `// +build` lines, `//go:`
  directives such as `//go:embed`, `//go:linkname` and `//go:noinline`
  (but not `//go:generate`), `//line` directives, `//export`, and the C
  preamble above `import "C"`.
- **Tests and files outside the build.** `_test.go` files and files a
  build constraint leaves out are not in the build, so creating, editing
  or deleting one rebuilds nothing, silently. A Go file whose edited build
  constraint now takes it in is found by listing the packages again.
- **Ignored paths.** Changes under `.git`, under the `[build] output`
  directory (`build/` by default), and, in a git work tree with `git` on
  `PATH`, to anything `.gitignore` ignores, as `git check-ignore` says,
  do not wake the scan; without git, `node_modules` is ignored. A file the
  app is built from always counts, ignored or not, as a generated file
  the app embeds. Below the directories `dev.watch` names, ignored files
  are not watched.
- **A syntax error** in a changed Go file is shown at once, without
  running the go command, in the terminal and over the app in the viewer,
  and the app keeps running. A file that does not parse is read again
  100 ms later before its errors are shown, in case it was caught
  half-written. The check is skipped when the Go that built ggui is older
  than the project's toolchain, whose newer syntax it might reject; type
  errors are always the build's to report. With `--web` the build reports
  syntax errors too, over the page.

`--verbose`, or `GGUI_VERBOSE=1`, prints each of these decisions with the
time: what the system reported, what a scan found, what was passed over
and why, and builds started, cancelled and thrown away.

### Flags

| Flag | Use |
| --- | --- |
| `--tags a,b` | Build tags, replacing `dev.tags`. `--tags ""` builds with none. |
| `--no-watch` | Build and run once; dev exits when the app does. |
| `--stop-timeout 3s` | How long the app has to exit when asked before it is killed. |
| `--verbose` | Print the watcher's and the rebuild loop's decisions, timed. `GGUI_VERBOSE=1` does the same. |
| `--display remote` | Where the window shows: `remote`, in the viewer, the default on macOS, Linux and Windows, or `window`, the app's own. Replaces `dev.display`. |
| `--bundle-dir dir` | On macOS, the directory where `Name.dev.app` is created, replacing `dev.bundle_dir` (default: `build.output`). |
| `--sign identity` | On macOS, sign `Name.dev.app` as this keychain identity, replacing `dev.identity`; `-` signs ad hoc. |
| `--config file` | Read this file instead of the `ggui.toml` found from the package or the working directory. |
| `-- args` | Arguments for the app, after `dev.args`. |

### In a browser

`--web` builds the app for `js/wasm` and serves it, with a page that shows
the build, the download and the compile as they happen and steps aside
once the app has painted.

- **Every build is pushed to the page.** An open page follows the
  server's builds and reloads itself onto each one that succeeds, carrying
  over the state the app [keeps](#keeping-state-across-a-restart). One that
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
ggui build                    # macos on a Mac, windows on Windows, native elsewhere
ggui build --target web
ggui build --target macos --universal
GOARCH=amd64 ggui build --target windows
GOARCH=amd64 ggui build --target linux --format tar,deb,rpm
ggui build --identity "Developer ID Application" --notarize --dmg
```

Build makes a release build: without the development tags, with
`-trimpath`, and with `-ldflags "-s -w"` followed by `build.ldflags` and
then `-X github.com/ironpark/ggui.buildID=<app.id>`, which tells the
program its ID; on Linux it is the window class, which a desktop matches
to the desktop entry. ggui has no version variable of its own to stamp; to
stamp yours, add `-X main.version=1.2.0` to `build.ldflags`. Output goes to `build.output`,
`build/` by default, or to `--out`. The path of what was built is printed
on standard output, for scripts.

| Target | Writes |
| --- | --- |
| `macos` | `Name.app`: the program in `Contents/MacOS`, `Info.plist` from `[app]` and `[macos]`, and `Contents/Resources/AppIcon.icns` made from `app.icon`, signed with `macos.signing_identity`. With `--dmg` or `[macos.dmg]` also `Name.dmg`, and with `--pkg` or `[macos.pkg]` the installer `Name.pkg`; with `--notarize` each is notarized and stapled. See [Shipping to other Macs](#shipping-to-other-macs). |
| `windows` | `Name.exe`, for the go command's `GOARCH`: `amd64`, `arm64` or `386`. Its icon is made from `app.icon`, its version information from `[app]` and `[windows]`, and it has a manifest; it is a GUI program, which opens no console. See [The Windows program](#the-windows-program). |
| `linux` | `name-linux-arch/`, an install tree laid out as under `/`: the program in `usr/bin`, built for the go command's `GOARCH` (`amd64`, `arm64` or `riscv64`) without cgo, a desktop entry, the icon from `app.icon` in the hicolor sizes, and AppStream metadata. Beside it what `--format` or `linux.formats` asks for: `name-version-linux-arch.tar.gz` (the default), a `.deb` and an `.rpm`. See [The Linux packages](#the-linux-packages). |
| `web` | A directory, `build/web/` by default, for any static host: `index.html`, `main.wasm` and the Go toolchain's `wasm_exec.js`, each beside a gzipped `.gz` copy. |
| `native` | The program, for the `GOOS` and `GOARCH` the go command builds for, as `go build` makes it: on Windows without an icon or manifest, and opening a console. |

| Flag | Use |
| --- | --- |
| `--target macos\|windows\|linux\|web\|native` | What to build. |
| `--format tar,deb,rpm` | With `--target linux`, what to make of the install tree, replacing `linux.formats`; `--format ""` makes the tree alone. |
| `--out dir` | The output directory, replacing `build.output`. |
| `--universal` | With `--target macos`, build for Apple silicon and Intel and join the two into one universal program, on any host. Same as `macos.universal`. |
| `--console` | With `--target windows`, link the program for the console, so that Windows opens a console window for its output; `--console=false` makes a GUI program though `windows.console` is set. |
| `--identity name` | Sign as this keychain identity, replacing `macos.signing_identity`; `-` signs ad hoc. |
| `--p12-file file`, `--p12-password-file file` | Sign with this PKCS#12 certificate and its password, replacing `macos.p12_file` and `macos.p12_password_file`. |
| `--notarize` | Notarize and staple, replacing `macos.notarize`; `--notarize=false` turns it off. |
| `--notary-profile name`, `--api-key-file file`, `--apple-id id`, `--team-id id` | Notarize with these credentials, replacing those in `[macos.notary]`. |
| `--dmg`, `--pkg` | Also make the disk image or the installer; `--dmg=false` and `--pkg=false` skip the one `[macos.dmg]` or `[macos.pkg]` asks for. |
| `--tags a,b` | Build tags, replacing `build.tags`. |
| `--ldflags flags` | Linker flags after `-s -w`, replacing `build.ldflags`. |
| `--config file` | Read this file instead of the `ggui.toml` found. |

### The macOS bundle

The icon is written by ggui itself, without `iconutil` or `sips`, so a
bundle cross-built on Linux or Windows has it too. Every size from 16 to
1024 pixels is scaled down from the PNG by averaging in linear light. A
source smaller than 1024 pixels gives an icon without the larger sizes,
and macOS scales up the largest it has.

A macOS icon does not fill its canvas: Apple's grid puts its body, the
rounded square, in the middle 824×824 pixels of 1024, leaving 100 on each
side for the shadow, and an icon drawn to the edges looks larger than
every other in the Dock. Artwork made that way, as a Windows icon is, can
be scaled into the grid by setting `macos.icon_inset = true`: the bundle's
icon is then the PNG scaled to 824/1024 of its size, centred on
transparency. The Windows icon keeps the edges. ggui does not cut the
rounded square, or add the shadow: the artwork has its own shape. Without
`icon_inset`, a build whose icon has opaque pixels near the edges says so.

The icon is a PNG or an SVG. An `.svg` `app.icon` is drawn at 1024×1024
when the build starts, by the `svg` package of the ggfx in the project's
own module graph, at the version the app builds with: the ggui command
carries no SVG renderer of its own. The document's viewBox (or, without
one, its width and height) is scaled to fill the canvas, on
transparency, and must be square; the PNG drawn then goes the way a PNG
`app.icon` does, through `icon_inset` and into the Windows icon too. To
draw it, `ggui build` compiles a small program in the project's module,
given to the go command as an `-overlay` so no file is written into the
project, built for the machine running the build whatever `--target` is,
and with `-mod=readonly`, so `go.mod` and `go.sum` are left as they are.
That takes:

- a ggui project: a module that requires `github.com/ironpark/ggfx`,
  which requiring ggui does, `// indirect` or not;
- a ggfx with the `svg` package, from October 2026 on; an older one is
  named in the error, and updating ggui brings a later one;
- an SVG the package can draw: paths, shapes, groups, `use`, transforms,
  gradients, opacity, clip paths, masks and markers, but not `filter`,
  `image` or text. What it refuses is said with the file's name.

The compile is the go command's, so a build after the first takes it
from the build cache. A compressed `.svgz` is refused: name the SVG it
holds. Artwork that relies on more than the package draws is exported as
a 1024×1024 PNG from the drawing program instead.

macOS draws the app's standard menu items and dialogs in a language only
when `Info.plist` lists it in `CFBundleLocalizations`. When
`[macos.plist]` does not set that key, a project with translation
catalogs, found as [`ggui i18n check`](#ggui-i18n) finds them, gets it:
`en`, the development language, then the catalogs' languages, sorted,
said as the build starts. Set `CFBundleLocalizations` under
`[macos.plist]` to name them yourself.

The default signing identity, `-`, signs ad hoc, on any host: ggui writes
the signature itself, in Go, and it is the one `codesign --force --sign -`
makes, byte for byte. The program gets a code directory, with
`macos.entitlements` when set, that binds `Info.plist` and the seal of
the bundle's other files, `Contents/_CodeSignature/CodeResources`.
Programs in `Contents/MacOS` besides the app's own, and libraries in
`Contents/Frameworks`, are signed first, as `codesign --deep` would sign
them; a nested bundle, such as a `.framework`, is refused.

An ad hoc signature names no one, so it is for trying the app, or for
people who trust you, not for shipping. The Mac that built it, or one it
is copied to without a download, runs it. Downloaded, the app is marked
as quarantined, and macOS refuses it the first time, saying it cannot
check it for malicious software; the person who downloaded it can still
open it with **Open Anyway** in System Settings, Privacy & Security (or,
before macOS 15, Control-click and Open), or by removing the mark:
`xattr -d com.apple.quarantine Name.app`. Without the ad hoc signature,
with only the one the Go linker puts on the program, which does not
cover the bundle, macOS would call the downloaded app damaged and offer
no way to open it. To ship to others, sign with a Developer ID and
notarize; see below.

Without `--universal` the program is built for the go command's `GOARCH`,
so `GOARCH=amd64 ggui build` makes an Intel bundle. With it, the program
is built for both and the two are joined into one universal program, in
Go, as `lipo -create` joins them, on any host.

### Shipping to other Macs

Another Mac opens an app only when it is signed with a Developer ID and
notarized by Apple. ggui does both, and makes the disk image or installer
it ships in, with the packages of [zapp](https://github.com/ironpark/zapp);
its `.zapp.yaml` names the same settings much the same way.

```sh
ggui build --identity "Developer ID Application" --notarize --notary-profile ggui --dmg
```

1. **Signing.** `macos.signing_identity` or `--identity` names an identity
   in the keychain: all of its name, such as `"Developer ID Application:
   Example Ltd (ABCDE12345)"`, a part, such as `"Developer ID
   Application"` for the first one, or its SHA-1 hash. `security
   find-identity -v -p codesigning` lists them, and `ggui doctor` too. The
   app is signed with the hardened runtime and a secure timestamp, which
   notarization requires, and with `macos.entitlements` when it is set.
   In CI, sign with a certificate exported with its private key as `.p12`
   instead: `macos.p12_file` or `--p12-file`, or its base64 text in
   `$GGUI_P12_BASE64`, with its password in `$GGUI_P12_PASSWORD` or a
   file; a PEM file with the private key and certificate works as
   `macos.p12_file` too. ggui imports it into a temporary keychain for the
   build and deletes that afterwards. The variables' names can be changed
   in `ggui.toml`, which never holds a secret itself. On Linux and
   Windows a certificate is what signs; see
   [below](#signing-and-notarizing-on-linux-and-windows).
2. **Notarizing.** `--notarize` or `macos.notarize = true` submits the
   app, then the disk image and the installer, to Apple's notary service,
   waits for the verdict, which usually takes minutes, and staples the
   ticket to each. It needs one of: a notarytool keychain profile, made
   once with `xcrun notarytool store-credentials NAME`
   (`macos.notary.profile`, `--notary-profile`); an App Store Connect API
   key (`macos.notary.api_key_file`, `--api-key-file`); or an Apple ID,
   its team and an app-specific password (`apple_id`, `team_id`, and
   `$GGUI_NOTARY_PASSWORD`). A rejection prints the notary's log.
3. **Disk image.** `--dmg`, or a `[macos.dmg]` table, makes `Name.dmg`
   beside the app: the app and a link to `/Applications`, side by side in
   a 640×480 Finder window unless `[macos.dmg]` lays it out otherwise,
   with the app's icon as the volume's. It is made in Go, without
   `hdiutil`, and signed and notarized as the app is.
4. **Installer.** `--pkg`, or a `[macos.pkg]` table, makes `Name.pkg`,
   which installs the app into `/Applications`. An installer is signed
   with a Developer ID *Installer* identity, a certificate of its own:
   the keychain's or the `.p12`'s, or `macos.pkg.signing_identity`. With
   ad hoc signing it is left unsigned.
5. **Verifying.** A build signed with an identity ends by checking what
   it made: the bundle, the signature, Developer ID, timestamp, hardened
   runtime, architectures, minimum macOS, the stapled ticket, and
   Gatekeeper's verdict. A failed check fails the build. Gatekeeper
   rejects what is not notarized, which without `--notarize` is shown as a
   warning.

A build checks the identity, the certificate and the notary credentials
before it compiles anything, so a missing identity or password fails at
once, saying what to do. The paths of the app and of the disk image and
installer are printed on standard output, one a line.

ggui signs ad hoc on any host, with its own signer. Signing with an
identity, notarizing and verifying use Apple's tools on a Mac, and
rcodesign on Linux and Windows, as below.

#### Signing and notarizing on Linux and Windows

On Linux and Windows, amd64 and arm64, ggui signs and notarizes with
zapp's rcodesign library (Rust's
[apple-codesign](https://github.com/indygreg/apple-platform-rs), linked
statically, about 30 MB) in place of Apple's tools, so a Linux CI runner can
ship a macOS release. There is no keychain or `notarytool` there, so it
takes:

- **A certificate with its private key**: the Developer ID Application
  certificate exported as `.p12` (`macos.p12_file`, `--p12-file`, or its
  base64 text in `$GGUI_P12_BASE64`, with its password as on a Mac), or
  a PEM file holding the private key and the certificate, given the same
  way as a file. The certificate holds the one identity it signs as:
  `macos.signing_identity`, which may name a keychain identity for
  building on a Mac, is not used, and without a certificate a keychain
  identity is refused.
- **An App Store Connect API key, to notarize**: `macos.notary.api_key_file`
  or `--api-key-file`, as the JSON that
  [rcodesign](https://gregoryszorc.com/docs/apple-codesign/stable/apple_codesign_rcodesign.html)'s
  `rcodesign encode-app-store-connect-api-key -o key.json ISSUER_ID KEY_ID AuthKey_KEY_ID.p8`
  writes: `{"issuer_id", "key_id", "private_key"}`, the last the base64 of
  the `.p8` key, not its PEM. Make the key in App Store Connect, under
  Users and Access, Integrations, with the Developer role. A notarytool
  profile or an Apple ID works on a Mac only.

The app and the disk image are signed, notarized and stapled as on a Mac,
with the hardened runtime and Apple's timestamp service. An installer is
not: it takes a Developer ID Installer certificate of its own, and away
from a Mac ggui takes one certificate, so `--pkg` or `[macos.pkg]` with an
identity is refused there; make the installer on a Mac. The checks at the
end verify each binary's code digests and signature with rcodesign; a
disk image's or installer's signature, and Gatekeeper's verdict, are
checked on a Mac only, and are reported as skipped.

Only some builds of the ggui command link the library, since it takes cgo:

- **The release binaries** for Linux and Windows on
  [GitHub](https://github.com/ironpark/ggui/releases) do, cross-compiled
  with [zig](https://ziglang.org) as the C compiler; the Linux ones need
  glibc 2.28 or later. Their archives carry the library's notices under
  `rcodesign/`. Homebrew's formula builds with cgo on Linux too.
- **`go install` on Linux** does with cgo and a C compiler: `CGO_ENABLED=1
  go install github.com/ironpark/ggui/cmd/ggui@latest` with `gcc` or
  `clang` on `PATH` (cgo is on by default when one is), and the system's
  glibc 2.35 or later.
- **`go install` on Windows** does only with [LLVM
  MinGW](https://github.com/mstorsjo/llvm-mingw), the toolchain the
  library is built with, and the tag `ggui_rcodesign`: MinGW's GCC, which
  a Windows machine with a C compiler often has on `PATH` and cgo would
  then use, does not link it, so the tag keeps such a build from trying.

  ```powershell
  $env:CGO_ENABLED = '1'
  $env:CC = 'x86_64-w64-mingw32-clang'   # aarch64-w64-mingw32-clang on Arm
  go install -tags ggui_rcodesign github.com/ironpark/ggui/cmd/ggui@latest
  ```

A build without it, `CGO_ENABLED=0` or on another system, builds the
bundle, disk image and installer unsigned, and when asked to sign stops
with a message saying which of these to install. `ggui doctor` says
which kind of build it is, and checks the certificate and API key
`ggui.toml` gives.

Signing this way is tested in ggui's CI with a throwaway self-signed
certificate: on Linux, by a build with GCC and one built as the release
is, and checked by `codesign --verify` on a Mac; and on Windows by the
release build, checked by rcodesign. Notarizing this way is not tested
there, since CI has no Apple credentials.

### The Windows program

`ggui build --target windows` makes `Name.exe`, named for `app.name`,
which must therefore be a name Windows allows for a file. It carries:

- **An icon**, made from `app.icon` in the sizes Explorer, the taskbar
  and the Start menu ask for, from 16 to 256 pixels, scaled down in linear
  light as the macOS icon is; 256 is stored as PNG, the rest as bitmaps,
  as Visual Studio writes them. A source smaller than 256 pixels gives an
  icon without the larger sizes, which Windows scales up from the largest
  it has. This is the icon of the file; the window's own, in its title bar
  and taskbar button, is the one the app gives `Config.Icon`.
- **Version information**, which Explorer shows under Properties, Details,
  and Task Manager names the process by: `app.version` as the file and
  product version, `app.name` as the product name, and `[windows]`'s
  description, company and copyright. Windows keeps each part of a
  version in 16 bits, so `app.version` must keep each number at most
  65535 for this target.
- **A manifest**, which asks for per-monitor DPI awareness (v2, falling
  back to per-monitor where Windows does not know v2), the themed common
  controls 6 for the dialogs the app opens, Windows 10 and 11
  compatibility, the UTF-8 code page for the system's ANSI functions,
  paths past `MAX_PATH` where the system allows them, and running as the
  user who starts it. `windows.manifest` names one to link instead.

It is linked with `-H=windowsgui`, after `build.ldflags`, so Windows opens
no console window for it, and what it writes to standard output and
error goes nowhere unless redirected. `--console`, or `windows.console`,
keeps the console, for reading that output while debugging.

The program is built for the go command's `GOARCH` for Windows, as the
macOS bundle is: `GOARCH=amd64 ggui build --target windows` builds for
the Intel and AMD PCs most people have, and `GOARCH=arm64` for Windows on
Arm. Cross-building from an Apple silicon Mac, or from an Arm Linux
machine, the default is `arm64`, which ggui points out.

The resources are written by ggui itself, in Go, as a COFF object, so a
program cross-built on macOS or Linux has them too, with no `rc.exe`,
`windres` or `go generate` step. The go command links the `.syso` files
it finds in the directories of the packages it builds, and only those on
disk: an overlay (`go build -overlay`) adds a `.syso` to the package in
`go list`'s view, but the build then reads the file from its path on disk
and fails.
So for the build ggui writes `ggui_windows_<arch>.syso` beside the main
package's sources, a name only a build for that target links, and
removes it when the build ends, failed or interrupted with Ctrl-C too. A
file of that name, left by a build that was killed, is replaced. A
package that links resources of its own from a `.syso`, as from
go-winres, rsrc or windres, is built with those instead, since a program
has one resource section; ggui says so and adds none.

Signing the program and making an installer are not done here: both
start from `Name.exe` as built, with Microsoft's `signtool`, or
`osslsigncode` away from Windows, and with an installer tool such as
Inno Setup, WiX or MSIX.

### The Linux packages

`ggui build --target linux` builds the program for `linux/$GOARCH` with
`CGO_ENABLED=0`, from any host, and makes `name-linux-arch/` in the output
directory, where `name` is the package name: `app.name` in lowercase, with
each run of anything but letters, digits, `+` and `.` made one hyphen, so
`Hello World` is `hello-world`. It is also the program's name in
`/usr/bin`. The other files are named for `app.id`, the reverse-DNS ID:

```
hello-world-linux-amd64/usr/bin/hello-world
                        usr/share/applications/com.example.hello.desktop
                        usr/share/icons/hicolor/<N>x<N>/apps/com.example.hello.png   16 24 32 48 64 128 256 512
                        usr/share/icons/hicolor/scalable/apps/com.example.hello.svg  when app.icon is an SVG
                        usr/share/metainfo/com.example.hello.metainfo.xml
```

The program is mode 0755, everything else 0644. The icons are scaled from
`app.icon` as the macOS and Windows ones are, without `macos.icon_inset`;
a source smaller than 512 pixels leaves out the larger sizes. The desktop
entry has `Name`, `Comment` from `linux.description`, `Exec`, `Icon`,
`Categories` from `linux.categories`, `Terminal=false` and
`StartupWMClass` set to the ID, which the program sets as its X11 window
class, so that docks and task switchers group its windows under the entry.
With `linux.mime_types` it also has `MimeType`, and `Exec` passes the
files or URLs to open (`%U`). The AppStream metadata names the ID, name,
summary, `linux.license` and `linux.homepage`, and launches the desktop
entry.

| Format | File | Contents |
| --- | --- | --- |
| `tar` | `name-version-linux-arch.tar.gz` | The tree, paths from `usr/`, owned by root. |
| `deb` | `name_version_arch.deb` (`amd64`, `arm64`, `riscv64`) | The tree under `/usr`, depending on `linux.depends_deb`. |
| `rpm` | `name-version-1.arch.rpm` (`x86_64`, `aarch64`, `riscv64`) | The tree under `/usr`, requiring `linux.depends_rpm`. |

The packages are written in Go by [nfpm](https://nfpm.goreleaser.com/),
so any host makes them. After installing and removing they run
`update-desktop-database` and `gtk-update-icon-cache` where installed.
`linux.maintainer`, `linux.homepage`, `linux.license` and
`linux.description` fill their fields; a Debian package without a
maintainer names a placeholder, which ggui warns of.

The version is `app.version`. Should it be anything but numbers, the
packages get a version both dpkg and rpm accept and sort as meant: a
leading `v` is dropped, a pre-release after `-` becomes `~` (`1.2.0-rc.1`
is `1.2.0~rc.1`, before `1.2.0`), `+metadata` is kept, other characters
become `.`, and a version that does not start with a digit, such as `dev`,
becomes `0~dev`, before every release.

[Linux](linux.md) has what the packages depend on and why, installing the
tarball for one user, and what Linux apps lack so far.

### The web bundle

The page loads its files relative to `web.base`, which it writes as its
`<base>`. Serve the directory at that path, and have the host answer every
route below it with `index.html` if the app uses `router.Browser`. A host
that serves precompressed files, such as nginx with `gzip_static on` or
Caddy with `precompressed gzip`, sends the `.gz` copies, a little over a
quarter of the binary's size.

## ggui release

```sh
ggui release --dry-run          # build everything, print what would be sent
ggui release                    # publish app.version to GitHub
ggui release --publish github,homebrew,choco
```

`ggui release` builds `app.version` for each target and architecture,
collects what it publishes in `build/release/VERSION/`, writes
`SHA256SUMS` over it, prints a table of the files with their sizes and
checksums, and publishes them. [Releasing an app](releasing.md) walks
through it from the tag to the people installing the app, with the
credentials each channel needs and a GitHub Actions workflow.

| Target | Files, for each architecture |
| --- | --- |
| `macos` | `name-VERSION-macos-ARCH.dmg` when `[macos.dmg]` asks for a disk image, else `name-VERSION-macos-ARCH.zip` of `Name.app`, as `ditto -c -k --keepParent` lays it out, links and modes kept; `.pkg` too when `[macos.pkg]` asks. On a Mac one build, `universal`, joined with `lipo`; elsewhere one for each of `amd64` and `arm64`, unsigned. Signed and notarized as `[macos]` says. |
| `windows` | `name-VERSION-windows-ARCH.zip`, holding `Name.exe`: `amd64`, `arm64` and `386`. |
| `linux` | What `linux.formats` names: the tarball, `.deb` and `.rpm`: `amd64`, `arm64` and `riscv64`. |

`name` is `app.name` lowercased, with hyphens between words. An
architecture a target has no build for is passed over.

| Channel | Publishes |
| --- | --- |
| `github` | A release at the tag `v` + version, with every file. It is created as a draft with the tag and `target_commitish` set to the commit checked out, so GitHub makes the tag when it is published and nothing is pushed from here; the files are uploaded, a file of the same name replaced, a failed upload retried; and only then is the draft published, so a failed run leaves a draft no one sees. A run after a failure takes up that draft. |
| `homebrew` | The cask `Casks/TOKEN.rb`, committed to `release.homebrew.tap` through GitHub's contents API. |
| `choco` | The package `ID.VERSION.nupkg`, pushed to `release.chocolatey.source`. Its install script downloads the Windows zip from the GitHub release and checks its SHA-256. |

Before building, a run checks what it can: that the work tree has no
changes not committed, that the tag, if it exists here or on `origin`, is
at this commit, and, when publishing to GitHub, that the token works, the
commit is pushed and the release is not published already. A dry run
says what fails and carries on.

| Flag | Does |
| --- | --- |
| `--version X.Y.Z` | Release this version, replacing `app.version`, which the builds stamp in. |
| `--targets list` | Build these, from `macos`, `windows` and `linux`, replacing `release.targets`. |
| `--arches list` | For these architectures, replacing `release.arches`. |
| `--publish list` | Publish to these, from `github`, `homebrew` and `choco`, replacing `release.publish`; `none` publishes nothing. |
| `--draft` | Leave the GitHub release a draft. Not with `homebrew` or `choco`, which download from it. |
| `--prerelease` | Mark the GitHub release a prerelease. |
| `--notes file` | The release's description, Markdown, replacing `release.notes`. Without either it is a table of the files. |
| `--dry-run` | Build and write everything, the cask and the Chocolatey package too, publish nothing, and print the requests a real run would send. |
| `--allow-dirty` | Release from a work tree with changes not committed. |
| `--replace-assets` | Upload to a GitHub release already published, replacing its files of the same names. |
| `--out dir` | The output directory, replacing `build.output`. |
| `--config file` | Read this file instead of the `ggui.toml` found. |

| Variable | For |
| --- | --- |
| `GITHUB_TOKEN`, then `GH_TOKEN`, then `gh auth token` | The GitHub release, and the tap. |
| `HOMEBREW_TAP_TOKEN` | The tap, when the token above cannot write it. |
| `CHOCOLATEY_API_KEY` | The Chocolatey push. |

No token or key is ever printed: an error names where the token came
from, not what it is.

## ggui doctor

```sh
ggui doctor
ggui doctor --json
```

Doctor prints a line for each check: `ok`; `warn`, which works but lacks
something or is worth knowing; or `fail`, which will not work until fixed.
On a terminal each is also marked ✓, ! or ✗, in color.
It prints what to do under each line that is not ok, and exits with status
1 when a check failed. `--json` prints
`{"ok": bool, "checks": [{"name", "status", "detail", "fix"}]}`.

| Check | Looks at |
| --- | --- |
| go version | The toolchain against the `go` line of the go.mod of the ggui the project resolves. |
| target | Whether ggfx and ggwd support `GOOS/GOARCH`; whether cgo is on, and on macOS whether the Xcode command line tools that a cgo build calls are installed. ggui needs no cgo, so `CGO_ENABLED=0` avoids needing them. |
| ggui, ggfx, ggwd | The versions the project resolves, or the local directories that replace them, and whether ggfx and ggwd are the versions that ggui's go.mod requires. |
| remote display | Whether `ggui dev` can show the app in its viewer, and why not. |
| ggui.toml, icon | Whether the file is valid, and the icon a square PNG of at least 64 pixels, ideally 1024; under 256 it lacks Windows' largest size too. An SVG icon is only found: drawing it takes a compile, which the build does. |
| Windows build | What `--target windows` takes from the file and macOS does not check: each number of `app.version` at most 65535, `app.name` a name Windows allows for a file, and `windows.manifest`, when set, a well-formed `<assembly>`. |
| Linux build | What `--target linux` makes: the package name, version, desktop entry and formats, warning when `app.id` is not set, or, for packages, `linux.maintainer` or `linux.license`. |
| translations | In a project with [translation catalogs](#ggui-i18n) only: the errors `ggui i18n check` finds in each, as a warning, since the app still runs. |
| platform | What ggwd and ggfx load when an app starts. |
| tools | `wasm-opt`; on macOS `codesign`, for signing with an identity. Signing ad hoc and `--universal` need no tools. |
| signing | On macOS: the keychain's signing identities and whether `macos.signing_identity` is one; a certificate `ggui.toml` or `$GGUI_P12_BASE64` gives; `notarytool` and `stapler`, which come with the Xcode command line tools; and the notarization credentials. Elsewhere: whether this ggui can sign at all. |

ggwd and ggfx use no cgo: they load the system's libraries when the
program starts, so a missing one is not a build error but an app that
fails to open its window. Doctor looks for each library on the machine it
runs on:

| System | Required | Optional |
| --- | --- | --- |
| macOS | macOS 13 or later, which Go 1.27 programs need. The AppKit, Carbon, CoreFoundation, CoreGraphics, CoreVideo, Foundation, IOKit, Metal, OpenGL and QuartzCore frameworks: all load at start-up, OpenGL even when drawing with Metal. | |
| Linux, FreeBSD, NetBSD | An X11 display: there is no Wayland backend, so a Wayland session needs XWayland and `DISPLAY`. `libX11`; `libGLESv2` or `libGL`; GLX or `libEGL` to make the context; a driver offering OpenGL 3.3 or OpenGL ES 3.0, checked with `glxinfo` when it is installed. | `libXi`, `libXrandr`, `libXcursor`, `libXinerama`, `libXrender`, `libXext`; `fc-match` for system fonts; `zenity` or `kdialog` for file and message dialogs; `xdg-open`. |
| Windows | Direct3D 11 (`d3d11.dll`, `dxgi.dll` and a `d3dcompiler_4x.dll`), or OpenGL 3.3 through the GPU driver, which ggfx falls back to. | `uiautomationcore.dll`, for screen readers. |

## ggui i18n

```sh
ggui i18n check
ggui i18n check --allow-missing
ggui i18n extract
ggui i18n extract --lang ko --out locales/ko.json
```

`ggui i18n` compares the messages an app passes to the
`github.com/ironpark/ggui/i18n` package with the catalogs that translate
them. It reads the Go source, without building it: the project is the
directory given, else that of the `ggui.toml` found from the working
directory, else the working directory, and its Go files are read but for
`_test.go` files, `vendor` and `testdata` directories, directories whose
names begin with `.` or `_`, and other modules nested inside.

A message is the string literal, or a sum of string literals, that a call
passes:

| Call | Key |
| --- | --- |
| `i18n.T("Open file", args…)` | `Open file` |
| `i18n.N(n, "%d file", "%d files", args…)` | `%d file`, a plural message, keyed by its singular form |
| `loc.S("Saved")`, `loc.N(n, "%d file", "%d files")` on a `*i18n.Localizer` | as for `T` and `N` |
| any of them followed by `.Context("verb")` | `Open [verb]`: the text, a space and the context in brackets |

The package is recognised by its import path, whatever name the file gives
it, a dot import included. A Localizer is not: any method called `S` or
`N`, in a file that imports the package, whose message argument is a
string literal counts, so a method of another type with that name and
those arguments would be read as one. A constant of the call's package
declared with a string literal, `const saved = "Saved"` in any of its
files or in the function around the call, counts as that literal. A call
to `i18n.T` or `i18n.N` whose message is neither, such as `i18n.T(key)`
with `key` a variable or another package's constant, cannot be checked
or extracted, and is listed with its position.

Catalogs are of two forms. In Go, any composite literal of `i18n.Catalog`
in the project, with its fields named; a translation that is not a literal
counts as present and is not checked, and a key that is not a literal
turns off the report of unused keys for that catalog:

```go
var ko = &i18n.Catalog{
	Lang:     "ko",
	Messages: map[string]string{"Open [verb]": "열기"},
	Plurals:  map[string]i18n.Forms{"%d sandbox running": {Other: "샌드박스 %d개 실행 중"}},
}
```

In JSON, the files `--catalog glob` matches, relative to the project, `**`
standing for any number of directories; the flag may be repeated, and by
default it is `**/i18n/*.json` and `**/locales/*.json`. The plural forms
are `zero`, `one`, `two`, `few`, `many` and `other`:

```json
{"lang": "ko", "messages": {"Open [verb]": "열기"}, "plurals": {"%d sandbox running": {"other": "샌드박스 %d개 실행 중"}}}
```

`check` prints a line for each catalog and each problem in it, in
doctor's form, and exits with status 1 when there is an error:

| Problem | |
| --- | --- |
| missing | The catalog has no translation of a message, or an empty one, as `extract` adds. An error; `--allow-missing` makes it a warning. |
| verbs | The translation's format verbs differ from the source's: each argument must be formatted, with the same verb, as many times. `%[2]s` and `%[1]d` reorder the arguments, and text right after a verb, such as a Korean particle choice in `%s{을/를}`, is not part of it. A plural form other than `other` may leave verbs out, as "one file" leaves out the count, but not add any. An error. |
| no other, form | A plural translation without the `other` form, which every language's plural rules fall back to, or with a form that is not one of the six. An error. |
| whitespace | The translation begins or ends with other spaces than the source. A warning. |
| unused | A key that no call passes. A warning. |

`extract` adds each message a JSON catalog lacks with an empty
translation, `{"other": ""}` for a plural one, and writes the file again
with its keys sorted, keeping its entries and any other members. A Go
catalog is not rewritten: the entries it lacks are printed on standard
output, as lines to paste into its `Messages` and `Plurals`. With
`--lang ko` it makes a new JSON catalog of every message instead, at
`--out`, by default `locales/ko.json` in the project; a file already
there is left alone.

### Your own functions

```sh
ggui i18n check --func tr --plural trn
ggui i18n check --func ui.T --func m.tr --func say:1 --plural count:0,1
```

An app that wraps the package in functions of its own, such as

```go
func tr(key string, args ...any) string { return loc.S(key, args...) }
func trn(n int, one, other string, args ...any) string { return loc.N(n, one, other, args...) }
```

names them, so that their calls are read as `i18n.T`'s and `i18n.N`'s:
`--func` for one like `T`, `--plural` for one like `N`, each repeated for
more, or the same in [`ggui.toml`](#i18n), which the flags add to:

```toml
[i18n]
funcs = ["tr", "ui.T"]
plurals = ["trn"]
```

| Name | Matches |
| --- | --- |
| `tr` | a call of `tr`, and of any method called `tr`: `tr(…)`, `m.tr(…)`, `app.ui.tr(…)` |
| `ui.T` | a call of `T` qualified by an import of a package called `ui`, by the name the file gives the import (`u.T(…)` after `import u "example.com/app/ui"`) or by the package's own; by anything else called `ui`, such as a variable; and `T(…)` unqualified in package `ui` itself or in a file that dot-imports it |
| `NAME:POS` | with `--func`: the message is argument `POS`, from 0; 0 by default |
| `NAME:ONE,OTHER` | with `--plural`: the singular and the plural form are arguments `ONE` and `OTHER`; `1,2` by default, after the count, as `i18n.N` has them |

The source is read without its types, so a method is named by its name
alone: `(*Model).tr` is refused, and `tr` matches a method called `tr` of
any type. A package is matched by name, guessed from the last element of
its import path (`gopkg.in/yaml.v3` is `yaml`, `example.com/go-ui/v2` is
`ui`). A call of a wrapper counts in any file, not only those that import
the i18n package, and `.Context("verb")` after it counts as after
`i18n.T`. A wrapper's message that is not a literal or a constant is
listed as one of `i18n.T`'s is, with the wrapper's name; but not inside a
wrapper itself, where the message is one of its parameters passed on, as
`key` is to `loc.S` above. A wrapper may be a function, a method, or a
function literal assigned to a variable of its name.

## ggui record

```sh
ggui record
ggui record intro -o docs/intro.gif --fps 10 --width 800
ggui record --list
ggui record --frames /tmp/frames
ggui record --frame mac --backdrop '#1f2430'
ggui record --frame mac-unified
```

`ggui record` draws a tour, a scripted walk through the app that the app
registers with package `github.com/ironpark/ggui/record`, into an animated
GIF. Nothing is captured from the screen and no window opens: the app draws
each frame offscreen on a clock of its own. [Recording a tour](recording.md)
shows how to write one.

It builds the main package (`app.package`, or `--package`) with the release
tags, `build.tags`, to a temporary directory, and runs it in the project's
directory with `dev.env` added to its environment and `GGUI_RECORD` set to
the request, which `ggui.Run` reads: the app draws the tour and exits
instead of opening its window. What the app prints goes to the terminal.
At the end ggui says what was written, as `Wrote tour.gif (42.7 KB, 10
frames, 5.6s, 640×427)`, and prints its path on standard output.

The name selects the tour: with none, the one registered with no name, or
the only one. `--list` prints the names, the unnamed one as `(default)`.
The GIF is `name.gif`, or `tour.gif`, in the project's directory, unless
`-o` names a file.

| Flag | |
| --- | --- |
| `-o file` | Write the GIF to `file`. |
| `--frames dir` | Write PNG frames, `0000.png` and on, to `dir` instead of a GIF, for ffmpeg. |
| `--list` | Print the tours' names. |
| `--fps n` | Keep `n` frames a second; 0 keeps every frame the tour draws (15 a second unless the tour says otherwise). |
| `--width px` | Scale the frames down to `px` pixels wide. |
| `--colors n` | The GIF palette's size, 2 to 256 (256). |
| `--dither s` | Ordered dithering, 0 (none, the default) to 1; 0.3 smooths gradients that band. |
| `--loop n` | Play the GIF `n` times; 0 (the default) loops for ever. |
| `--lossy n` | Let the GIF's compression write a pixel in a color up to n/1000 from its own in Oklab where that compresses better, 0 (exact, the default) to 200; 20 to 40 is hard to see. |
| `--frame style` | Draw the frames in a window frame, in place of the tour's `Frame`: `mac`, a macOS window with a 28pt title bar, rounded corners and a shadow on a backdrop; the same window as `mac-titleless` (no title), `mac-unified` (a 52pt unified toolbar, the title and subtitle after the buttons), `mac-compact` (a 38pt compact toolbar), `mac-expanded` (the title row over a 38pt toolbar row) or `mac-hidden` (no bar, the buttons over the content); or `none`, the content alone. [The window frame](recording.md#window-styles) has the measures. |
| `--backdrop color` | What a framed window is drawn on: `#RRGGBB`, or `transparent`, which PNG frames keep and a GIF draws over white. |
| `--cursor=false` | Leave out the cursor the tour's pointer steps draw; the steps still drive the app. |
| `--package pkg` | The main package, replacing `app.package`. |
| `--tags tags` | Build tags, replacing `build.tags`. |
| `--config file` | Read `file` instead of the `ggui.toml` found. |

An app that does not import package `record`, has no tour of the name, or
exits before it reaches `ggui.Run` (or `record.Main`) is said to, and
`ggui record` exits with status 1.

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
notarize = true

[macos.plist]
NSCameraUsageDescription = "Scans documents."

[macos.notary]
profile = "example"

[macos.dmg]
background = "dmg-background.png"

[windows]
company = "Example Ltd"
copyright = "© 2026 Example Ltd"

[linux]
formats = ["tar", "deb", "rpm"]
description = "Says hello"
maintainer = "Example Ltd <hello@example.com>"
license = "MIT"

[web]
base = "/hello/"
```

### [app]

| Key | Default | Use |
| --- | --- | --- |
| `name` | the main package directory's name | The name people see: the `.app` name, its menu title, `Name.exe` and the page title. |
| `id` | made from the import path, such as `com.github.alice.hello` | The reverse-DNS bundle identifier. macOS keys settings, permissions and the Dock to it, so set it once and keep it. The Windows manifest names the program by it too, and on Linux it names the desktop entry, the icons and the AppStream component, and is the window class. |
| `version` | `"0.1.0"` | One to three numbers: `CFBundleShortVersionString` and `CFBundleVersion`, and the Windows file and product version, for which each is at most 65535. |
| `icon` | none | A square PNG, ideally 1024×1024, for the macOS and Windows icons, or an SVG with a square viewBox, which the build draws at 1024×1024 with the project's ggfx; see [The macOS bundle](#the-macos-bundle). See `macos.icon_inset` for the margin a macOS icon has. |
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
| `display` | `"remote"` on macOS, Linux and Windows | Where the app's window shows: `"remote"`, in [the viewer](#the-window-remote-display) that stays while the app restarts, or `"window"`, the app's own. |
| `bundle_dir` | `build.output` | On macOS, the directory where `Name.dev.app` is created; see [The macOS development bundle](#the-macos-development-bundle). Same as `--bundle-dir`. |
| `identity` | auto (`macos.signing_identity`, else `Apple Development`, else `"-"`) | On macOS, the keychain identity used to sign `Name.dev.app` so TCC permissions persist across rebuilds; `"-"` signs ad hoc. Same as `--sign`. |

### [macos]

| Key | Default | Use |
| --- | --- | --- |
| `minimum_system_version` | `"13.0"` | `LSMinimumSystemVersion`. Go 1.27 programs need macOS 13 whatever this says. |
| `category` | none | `LSApplicationCategoryType`, such as `"public.app-category.productivity"`. |
| `icon_inset` | `false` | Scale `app.icon` into the middle 824×824 of 1024, on transparency, as Apple's icon grid places a macOS icon, for artwork drawn edge to edge. The rounded square is the artwork's own; the Windows icon is unchanged. See [The macOS bundle](#the-macos-bundle). |
| `signing_identity` | `"-"` | The keychain identity to sign with: its name, part of it, or its hash. `-` signs ad hoc, or with a certificate picks its Developer ID Application identity. See [Shipping to other Macs](#shipping-to-other-macs). |
| `entitlements` | none | An entitlements plist to sign with. |
| `p12_file` | none | A `.p12` certificate and key to sign with instead of the keychain. |
| `p12_base64_env` | `"GGUI_P12_BASE64"` | The environment variable holding a `.p12` as base64, read when `p12_file` is not set. |
| `p12_password_file` | none | A file whose first line is the certificate's password. |
| `p12_password_env` | `"GGUI_P12_PASSWORD"` | The environment variable holding the password, read when `p12_password_file` is not set. |
| `notarize` | `false` | Notarize and staple; see `--notarize`. |
| `universal` | `false` | Build for Apple silicon and Intel; see `--universal`. |
| `[macos.plist]` | none | Extra `Info.plist` keys of any TOML type but dates, written over the generated ones. Without `CFBundleLocalizations` here, a project with [translation catalogs](#ggui-i18n) gets one naming their languages; see [The macOS bundle](#the-macos-bundle). |

### [macos.notary]

How to reach Apple's notary service; the first of a profile, an API key
and an Apple ID given is used.

| Key | Default | Use |
| --- | --- | --- |
| `profile` | none | A notarytool keychain profile, made with `xcrun notarytool store-credentials NAME`. |
| `api_key_file` | none | An App Store Connect API key as JSON: `{"issuer_id", "key_id", "private_key"}`, the last the `.p8` file's contents, as `rcodesign encode-app-store-connect-api-key` writes it. The only kind that works away from a Mac. |
| `apple_id` | none | An Apple ID, with `team_id` and an app-specific password. |
| `team_id` | none | The ten-character team ID, as in the identity's parentheses. |
| `password_env` | `"GGUI_NOTARY_PASSWORD"` | The environment variable holding the Apple ID's app-specific password. |
| `staple` | `true` | Attach the ticket, so Gatekeeper need not ask Apple when the app first opens. |
| `timeout` | `"1h"` | How long to wait for the verdict. |

### [macos.dmg]

The table asks for a disk image on every macOS build; `--dmg` makes one
without it, `--dmg=false` skips it.

| Key | Default | Use |
| --- | --- | --- |
| `title` | `app.name` | The volume's name. |
| `icon` | the app's icon | The volume's icon, a PNG or `.icns`. |
| `background` | none | A PNG, JPEG or TIFF drawn at the window's top left, in points. |
| `window` | `{ width = 640, height = 480 }` | The Finder window's size, in points. |
| `icon_size` | `128` | The icons' size, from 16 to 512. |
| `label_size` | `14` | The labels' text size, from 10 to 16. |
| `app_position`, `applications_position` | side by side across the middle | `[x, y]` of each icon's centre, from the window's top left. |
| `fs` | `"hfsplus"` | `"hfsplus"`, `"apfs"` or `"apfs-case-sensitive"`. |
| `format` | `"udzo"` | `"udzo"` (zlib), which every macOS opens, or `"ulfo"` (lzfse), smaller. |

### [macos.pkg]

The table asks for an installer package on every macOS build; `--pkg`
makes one without it, `--pkg=false` skips it.

| Key | Default | Use |
| --- | --- | --- |
| `signing_identity` | the Developer ID Installer identity | The identity the installer is signed with. |
| `install_location` | `"/Applications"` | Where the app is installed. |
| `scripts` | none | A directory with `preinstall` and `postinstall` scripts. |
| `license` | none | A plain-text license the installer shows. |

### [windows]

What `ggui build --target windows` puts in `Name.exe` beyond `[app]`; see
[The Windows program](#the-windows-program).

| Key | Default | Use |
| --- | --- | --- |
| `description` | `app.name` | The version information's `FileDescription`, which Task Manager and the taskbar's menus call the program by. |
| `company` | none | `CompanyName`. |
| `copyright` | none | `LegalCopyright`. |
| `console` | `false` | Link for the console, which Windows opens a window of for the program's output; see `--console`. |
| `manifest` | none | An application manifest to link instead of the generated one, as for a program that must run as an administrator. |

### [linux]

What `ggui build --target linux` writes beyond `[app]`; see
[The Linux packages](#the-linux-packages).

| Key | Default | Use |
| --- | --- | --- |
| `formats` | `["tar"]` | What to make of the install tree, which is always made: `"tar"`, `"deb"`, `"rpm"`; see `--format`. |
| `description` | `app.name` | One line on what the app does: the desktop entry's `Comment`, the AppStream summary and the packages' description. |
| `generic_name` | none | What kind of program it is, such as `"Image Viewer"`: the desktop entry's `GenericName`. |
| `categories` | `["Utility"]` | [Menu categories](https://specifications.freedesktop.org/menu-spec/latest/category-registry.html), such as `"Graphics"`, `"Office"` or `"Development"`. |
| `mime_types` | `[]` | MIME types the app opens, such as `"image/png"` or `"x-scheme-handler/myapp"`; with any, it is started with the files or URLs it is asked to open. |
| `maintainer` | none | `"Name <email>"`, the packages' maintainer, which a Debian package must name. |
| `homepage` | none | An `http` or `https` URL. |
| `license` | none | An SPDX license expression, such as `"MIT"` or `"MIT OR Apache-2.0"`. |
| `depends_deb` | `["libx11-6", "libxrandr2", "libxi6", "libxcursor1", "libxinerama1", "libxext6", "libxrender1", "libgl1", "libegl1"]` | The Debian package's dependencies, each in Debian's syntax, such as `"libgl1 \| libgles2"`. |
| `depends_rpm` | `["libX11", "libXrandr", "libXi", "libXcursor", "libXinerama", "libXext", "libXrender", "libglvnd-glx", "libglvnd-egl"]` | The RPM package's dependencies, as Fedora names them. |

### [web]

| Key | Default | Use |
| --- | --- | --- |
| `title` | `app.name` | The page title. |
| `base` | `"/"` | The path the app is served below. An app using `router.Browser` passes the same path to it. |
| `wasm_opt` | `"-O2"` | `wasm-opt` flags for `ggui build --target web`; `""` skips it. Without `wasm-opt` on `PATH` the binary ships as linked. |

### [i18n]

| Key | Default | Use |
| --- | --- | --- |
| `funcs` | none | The app's own functions that pass a message on as `i18n.T` does, for [`ggui i18n`](#your-own-functions) and `ggui doctor` to read: `"tr"`, `"ui.T"`, `"tr:1"`. As `--func`. |
| `plurals` | none | Those that pass one on as `i18n.N` does: `"trn"`, `"trn:0,1"`. As `--plural`. |

### [release]

What [`ggui release`](#ggui-release) builds and where it publishes it; see
[Releasing an app](releasing.md).

| Key | Default | Use |
| --- | --- | --- |
| `targets` | `["macos", "windows", "linux"]` | The targets built. As `--targets`. |
| `arches` | `["amd64", "arm64"]` | The architectures each target is built for where it has them: `"amd64"`, `"arm64"`, `"386"`, `"riscv64"`. As `--arches`. |
| `publish` | `["github"]` | The channels: `"github"`, `"homebrew"`, `"choco"`. As `--publish`. |
| `github` | `origin`'s repository | The GitHub repository, `"owner/repo"`. |
| `github_api` | `"https://api.github.com"` | The REST API, such as `"https://github.example.com/api/v3"` for GitHub Enterprise. |
| `tag_prefix` | `"v"` | What goes before the version in the tag. |
| `notes` | none | A Markdown file, the release's description. As `--notes`. |

### [release.homebrew]

| Key | Default | Use |
| --- | --- | --- |
| `tap` | none | The tap's repository, `"owner/homebrew-name"`, which publishing to Homebrew needs. |
| `cask` | `app.name` as a cask token, such as `hello-world` | The cask's token. |
| `homepage` | `linux.homepage`, else the repository | The cask's `homepage`. |
| `desc` | `linux.description` | The cask's `desc`: one line, which Homebrew wants without an article to begin it or a full stop to end it. |

### [release.chocolatey]

| Key | Default | Use |
| --- | --- | --- |
| `id` | `app.name` lowercased, with hyphens | The package ID. |
| `authors` | `windows.company`, else the repository's owner | The package's authors. |
| `project_url` | `linux.homepage`, else the repository | `projectUrl`. |
| `license_url` | none | `licenseUrl`. |
| `icon_url` | none | `iconUrl`, which the community repository asks for. |
| `tags` | `[]` | Tags, each its own string. |
| `summary` | `linux.description` | `summary`. |
| `description` | `summary` | `description`, Markdown. |
| `source` | `"https://push.chocolatey.org/"` | The feed pushed to; the package goes to `api/v2/package` below it. |
