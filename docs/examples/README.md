# Examples

[Documentation](../README.md) · [Project README](../../README.md)

Runnable GGUI applications covering reactive state, forms, layouts, charts,
chat interfaces, routing, multi-window desktop integration, and SQLite.

Run any example from the repository root with `go run`:

```sh
go run ./docs/examples/counter
```

On macOS, `task run`, `task run-todo`, `task run-workspace`, `task run-gallery`,
and `task run-create` build and launch local `.app` bundles. To run an example
in a browser with live reloads, use `task serve EXAMPLE=<name>` (for example
`task serve-gallery` or `task serve EXAMPLE=router`).

## Catalog

| Example | What it demonstrates | Run |
| --- | --- | --- |
| [Counter](counter) | Reactive state, buttons, keyboard shortcuts, and theme switching | `go run ./docs/examples/counter` (`task run`) |
| [Todo](todo) | IME text input, validation, keyed lists with inline editing, transitions, empty state, and `AlertDialog` | `go run ./docs/examples/todo` (`task run-todo`) |
| [Settings](settings) | Struct state with `Field` lenses, derived dirty flag, styled text, and background worker updates | `go run ./docs/examples/settings` |
| [Router](router) | Nested persistent layouts, path parameters, redirects, scoped not-found screens, and history navigation | `go run ./docs/examples/router` |
| [Custom widgets](custom) | Custom dial on `Interactive` + `Spring`, disclosure using `Invalidate`, and a 5,000-row virtualized list | `go run ./docs/examples/custom` |
| [Charts](charts) | All 70 shadcn chart variants (`ui/chart`), interactive tooltips, theme tokens, and animation replay | `go run ./docs/examples/charts` |
| [Carousel & Input OTP](controls) | Swipeable carousels and segmented OTP inputs across variants and states | `go run ./docs/examples/controls` |
| [Windows](windows) | Multiple windows sharing state, window signals, native menus, and system tray icons | `go run ./docs/examples/windows` |
| [Files](files) | Desktop drag-and-drop and native Open, Save, and folder dialogs via `Host.Dialogs` | `go run ./docs/examples/files` |
| [Workspace](workspace) | Project dashboard with searchable tables, form lenses, live options, dialogs, charts, progress, and toasts | `go run ./docs/examples/workspace` (`task run-workspace`) |
| [SQLite](sqlite) | Standalone-module SQLite browser with filtering, sorting, paging, typed edits, SQL runner, and CSV export | `go run ./docs/examples/sqlite` |
| [Gallery](gallery) | Searchable preview catalog across all `ui`, `ui/chart`, and `ui/chat` controls with live theme customizer | `go run ./docs/examples/gallery` (`task run-gallery`) |
| [Create](create) | Live theme editor with interactive card collage, palettes, typography, radius, JSON presets, and Go code export | `go run ./docs/examples/create` (`task run-create`) |
| [Installer](installer) | Go-authored setup wizard preview with welcome, license, location, custom options, env/PATH, and Windows Registry hooks | `go run ./docs/examples/installer` |
| [Updater](updater) | Sparkle 2-compatible auto-updater demo with local appcast server, Ed25519 signing/verification, `stable`/`beta` channel switching, manual & silent checks, skipping/clearing versions, simulated live `Applier`, and `updater.Dialog(ctrl)` | `go run ./docs/examples/updater` |

## Headless tests

Most examples include headless `Probe` tests you can run without opening a window:

```sh
go test ./docs/examples/...
go test -C docs/examples/sqlite ./...
```

See [Testing](../testing.md) for how `Probe` drives the same UI code in tests.
