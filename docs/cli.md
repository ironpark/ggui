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
behaves as one, but for what belongs to the window on the desktop:

- **One window.** `App.OpenWindow` returns an error while the app runs,
  and a window made before `Run` other than the first does not open; the
  app says so once on stderr. `ggui dev --display=window` opens them all.
- **Menus.** `NativeMenu`, `NativeContextMenu` and `TraySupported` report
  false: the menu bar is the viewer's, so `App.SetMenu`'s menus are drawn
  by `ui.AppMenubar`, as on Windows and Linux, and their chords work. A
  tray icon does not show, said once on stderr; `Window.ShowContextMenu`
  returns false. On Windows, too, `TraySupported` is false while the
  viewer shows the app.
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
- **The window's placement and state** are the viewer's and its user's:
  `SetSize`, `SetPosition`, maximize, minimize, fullscreen, show, hide,
  focus and attention requests do nothing, and the window reports itself
  at the screen's origin, visible and in none of those states. Its title,
  size limits, resizability, frame and always-on-top follow the app's
  `Config` and setters; a hidden title bar and transparency are the first
  app's of the session. `Screens()` lists one screen, the viewer's.
  Cursor shapes and text input, with input methods, work.
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
macOS, on the roots of the modules being edited; inotify on Linux, on
each watched directory and those made in them; `ReadDirectoryChangesW`
on Windows, on the module roots. A save is seen some 20 ms after it is
written, and the build starts 50 ms later if nothing else changes. A scan
also runs every 2 seconds whatever is said, for a notification that never
came, as on some network file systems. Where there are no notifications,
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
ggui build --identity "Developer ID Application" --notarize --dmg
```

Build makes a release build: without the development tags, with
`-trimpath`, and with `-ldflags "-s -w"` followed by `build.ldflags`. ggui
has no version variable of its own to stamp; to stamp yours, add
`-X main.version=1.2.0` to `build.ldflags`. Output goes to `build.output`,
`build/` by default, or to `--out`. The path of what was built is printed
on standard output, for scripts.

| Target | Writes |
| --- | --- |
| `macos` | `Name.app`: the program in `Contents/MacOS`, `Info.plist` from `[app]` and `[macos]`, and `Contents/Resources/AppIcon.icns` made from `app.icon`, signed with `macos.signing_identity`. With `--dmg` or `[macos.dmg]` also `Name.dmg`, and with `--pkg` or `[macos.pkg]` the installer `Name.pkg`; with `--notarize` each is notarized and stapled. See [Shipping to other Macs](#shipping-to-other-macs). |
| `web` | A directory, `build/web/` by default, for any static host: `index.html`, `main.wasm` and the Go toolchain's `wasm_exec.js`, each beside a gzipped `.gz` copy. |
| `native` | The program, for the `GOOS` and `GOARCH` the go command builds for. |

| Flag | Use |
| --- | --- |
| `--target macos\|web\|native` | What to build. |
| `--out dir` | The output directory, replacing `build.output`. |
| `--universal` | With `--target macos`, build for Apple silicon and Intel and join the two with `lipo`. Needs macOS with the Xcode command line tools. Same as `macos.universal`. |
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

The default signing identity, `-`, signs ad hoc with `codesign`, which
runs on the Mac that built it. Away from a Mac an ad hoc build is left
unsigned, with a notice.

Without `--universal` the program is built for the go command's `GOARCH`,
so `GOARCH=amd64 ggui build` makes an Intel bundle.

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
   file. ggui imports it into a temporary keychain for the build and
   deletes that afterwards. The variables' names can be changed in
   `ggui.toml`, which never holds a secret itself.
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

Away from a Mac, ggui signs with a `.p12` and notarizes with an API key
only through zapp's signing library, which needs cgo: a ggui installed
with `CGO_ENABLED=1 go install` and a C compiler (GCC on Linux, LLVM MinGW
on Windows) on Linux or Windows. The release binaries and the Homebrew
formula are built without cgo, and stop with a message saying so when
asked to sign there. Disk images and installers are made anywhere.

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
| remote display | Whether `ggui dev` can show the app in its viewer, and why not. |
| ggui.toml, icon | Whether the file is valid, and the icon a square PNG of at least 64 pixels, ideally 1024. |
| platform | What ggwd and ggfx load when an app starts. |
| tools | `wasm-opt`; on macOS `codesign`, and `lipo` for `--universal`. |
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
| `display` | `"remote"` on macOS, Linux and Windows | Where the app's window shows: `"remote"`, in [the viewer](#the-window-remote-display) that stays while the app restarts, or `"window"`, the app's own. |

### [macos]

| Key | Default | Use |
| --- | --- | --- |
| `minimum_system_version` | `"13.0"` | `LSMinimumSystemVersion`. Go 1.27 programs need macOS 13 whatever this says. |
| `category` | none | `LSApplicationCategoryType`, such as `"public.app-category.productivity"`. |
| `signing_identity` | `"-"` | The keychain identity to sign with: its name, part of it, or its hash. `-` signs ad hoc, or with a certificate picks its Developer ID Application identity. See [Shipping to other Macs](#shipping-to-other-macs). |
| `entitlements` | none | An entitlements plist to sign with. |
| `p12_file` | none | A `.p12` certificate and key to sign with instead of the keychain. |
| `p12_base64_env` | `"GGUI_P12_BASE64"` | The environment variable holding a `.p12` as base64, read when `p12_file` is not set. |
| `p12_password_file` | none | A file whose first line is the certificate's password. |
| `p12_password_env` | `"GGUI_P12_PASSWORD"` | The environment variable holding the password, read when `p12_password_file` is not set. |
| `notarize` | `false` | Notarize and staple; see `--notarize`. |
| `universal` | `false` | Build for Apple silicon and Intel; see `--universal`. |
| `[macos.plist]` | none | Extra `Info.plist` keys of any TOML type but dates, written over the generated ones. |

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

### [web]

| Key | Default | Use |
| --- | --- | --- |
| `title` | `app.name` | The page title. |
| `base` | `"/"` | The path the app is served below. An app using `router.Browser` passes the same path to it. |
| `wasm_opt` | `"-O2"` | `wasm-opt` flags for `ggui build --target web`; `""` skips it. Without `wasm-opt` on `PATH` the binary ships as linked. |
