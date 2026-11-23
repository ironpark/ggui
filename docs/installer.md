# Writing an installer in Go

[Documentation](README.md) · [The ggui command](cli.md) · [Releasing an app](releasing.md)

Package `github.com/ironpark/ggui/installer` lets you write your application's
installer as normal Go code instead of configuring TOML tables or external
script languages (Inno Setup, NSIS, WiX).

- On **Windows**, `ggui build --target windows` compiles your installer package
  into a self-contained `Name-Setup.exe` (and `Uninstall.exe`), injecting your
  compiled application (`Name.exe`, plus `ggfx_driver.dll` when using the Zig
  renderer), icon, version info, and UAC manifest automatically via `-overlay`.
- On **macOS**, `ggui build --target macos` reads the same Go code's
  `MacOS` configuration (`InstallLocation`, `License`, `Scripts`,
  `SigningIdentity`) to build `Name.pkg`.
- During development, `ggui dev ./cmd/installer` runs your installer directly
  in an interactive **preview sandbox** with live hot reload and no changes to
  your machine.

## Quick start

Add a `package main` at `./cmd/installer` (or `./installer`). `ggui build` and
`ggui release` discover it automatically — no `[installer]` table in `ggui.toml`
is needed.

```go
// cmd/installer/main.go
package main

import (
	_ "embed"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/installer"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

//go:embed LICENSE
var license string

func main() {
	installer.Main(&installer.App{
		License: license,
		Windows: installer.WindowsConfig{
			Scope:           installer.ScopeUser, // or installer.ScopeMachine (UAC)
			DesktopShortcut: installer.Bool(true),
		},
		MacOS: installer.MacOSConfig{
			InstallLocation: "/Applications",
		},
		Steps: []installer.Step{
			installer.LicenseStep(),
			installer.LocationStep(),
			installer.CustomStep("options", "Additional Options", func(ctx *installer.Context) ggui.Widget {
				return widget.Column(
					ui.Checkbox(ctx.Bool("add_path", true), "Add install folder to user PATH"),
				).Space(0.75)
			}),
			installer.ProgressStep(),
			installer.FinishStep(),
		},
		AfterInstall: func(ctx *installer.Context) error {
			if ctx.GetBool("add_path") {
				return ctx.AddToUserPath(ctx.Dir())
			}
			return nil
		},
		BeforeUninstall: func(ctx *installer.Context) error {
			return ctx.RemoveFromUserPath(ctx.Dir())
		},
	})
}
```

### Live preview while designing

```sh
ggui dev ./cmd/installer                 # preview install wizard
ggui dev ./cmd/installer -- --uninstall  # preview uninstall wizard
```

When no application payload has been embedded by `ggui build`, `installer.Main`
automatically enters preview mode (`ctx.IsPreview() == true`) backed by a
temporary `PreviewBackend` sandbox. Clicking **Install** or **Uninstall** runs
your steps and lifecycle hooks inside the sandbox without touching your real
filesystem, shortcuts, PATH, or registry.

### Building and releasing

```sh
GOARCH=amd64 ggui build --target windows # auto-discovers ./cmd/installer → Name-Setup.exe
ggui build --target macos                # auto-discovers ./cmd/installer → Name.pkg
ggui release                             # includes Name-Setup.exe and Name.pkg automatically
```

- Pass `--installer=false` to skip building an installer for a single build.
- Pass `--installer-pkg ./path/to/pkg` (or set `build.installer = "./path/to/pkg"`
  in `ggui.toml`) if your installer package lives outside `./cmd/installer` or
  `./installer`.
- Even without a Go installer package, `ggui build --target windows --installer`
  and `ggui build --target macos --installer` still build the default standard
  installer.

## How build-time metadata works

Before compiling target binaries, `ggui build` and `ggui release` compile your
installer package for the host and execute it with `GGUI_INSTALLER_DESCRIBE`
set to a temporary JSON path. `installer.Main` / `installer.Run` writes the
`App`'s resolved specification (Windows UAC `Scope`, license, shortcuts,
publisher, URL, and macOS `.pkg` options) and exits immediately without opening
a window. That keeps your Go code as the single source of truth — including
build-time Windows UAC manifests (`asInvoker` vs `requireAdministrator`) and
macOS `.pkg` licenses or scripts.

## Customizing the wizard

### Built-in steps

| Constructor | Behavior |
| --- | --- |
| `installer.WelcomeStep(title, body)` | Optional welcome page (empty strings use defaults from metadata). |
| `installer.LicenseStep()` | Scrollable license viewer + acceptance checkbox; automatically skipped when license text is empty. |
| `installer.LocationStep()` | Install folder field with `Browse…` picker, scope hint, upgrade detection, and Start Menu / Desktop shortcut checkboxes. |
| `installer.CustomStep(id, title, build)` | Custom navigable step with automatic `Back`, `Cancel`, and `Next` (or `Install`/`Update` right before `ProgressStep`) footer. |
| `installer.UninstallConfirmStep()` | Uninstall confirmation showing installed location and destructive `Uninstall` button. |
| `installer.ProgressStep()` | Progress bar and reactive status caption; entering it starts `Install` or `Uninstall`. |
| `installer.FinishStep()` | Completion screen with optional `Launch <Name>` checkbox and `Finish` button. |

Every `Step` supports value-semantic fluent setters: `.WithTitle(...)`,
`.WithFooter(...)`, `.WithCanContinue(...)`, `.WithNextLabel(...)`,
`.WithOnEnter(...)`, and `.SkipWhen(...)`.

### Themes, headers, and full custom UI

- Set `Theme` and `DarkTheme` on `installer.App` to style the wizard, and
  `Title`, `Width`, `Height`, `Resizable` for the window.
- Set `Header` to replace the top icon/title banner while keeping the step
  wizard below.
- Set `BuildUI` to replace the entire window with your own `ggui` widget tree
  (for example a one-click launcher installer) driven by `ctx.StartInstall()`,
  `ctx.StartUninstall()`, `ctx.Finish()`, `ctx.Progress`, `ctx.Status`, and
  `ctx.Failed`.

### Lifecycle hooks and thread safety

`BeforeInstall`, `AfterInstall`, `BeforeUninstall`, and `AfterUninstall` run
during both GUI and `--silent` installs/uninstalls. Because GUI installs run
on a worker goroutine:

- Use `ctx.Bool(key, defaultVal)` / `ctx.String(key, defaultVal)` in steps and
  read their snapshotted values with `ctx.GetBool(key)` / `ctx.GetString(key)`
  in hooks — or read external signals safely with `installer.Value(ctx, sig)`.
- Use `ctx.WriteFile(relPath, data, mode)`, `ctx.TrackFile(relPath)`,
  `ctx.CreateShortcut(...)`, and `ctx.TrackShortcut(...)` inside `AfterInstall`
  so extra files and shortcuts are recorded in `install-manifest.json`, rolled
  back automatically on failure, and removed cleanly on uninstall.
- Report custom progress via `ctx.ReportProgress(fraction, message)`.

## Registry, environment variables, and PATH

Both **declarative** (`WindowsConfig`) and **imperative** (`*installer.Context`
inside hooks) APIs are supported. Entries written through either API are:

- **Broadcast automatically** via `WM_SETTINGCHANGE` (`"Environment"`) on Windows
  when environment variables or `PATH` change.
- **Rolled back automatically** if installation fails partway (restoring any
  prior value that existed before install).
- **Recorded in `install-manifest.json`** and removed automatically on
  uninstall — shared registry keys (such as `CurrentVersion\Run`) keep all
  unrelated values intact, while newly created keys are deleted once empty.

### Declarative (on `WindowsConfig`)

String values in `Env` and `Registry` expand `%INSTALL_DIR%`, `%EXE_PATH%`,
`%APP_NAME%`, `%APP_ID%`, and `%VERSION%` (leaving standard Windows variables
such as `%LOCALAPPDATA%` untouched for `REG_EXPAND_SZ`).

```go
installer.Main(&installer.App{
	Windows: installer.WindowsConfig{
		Scope:     installer.ScopeUser, // ScopeUser → HKCU; ScopeMachine → HKLM
		AddToPath: true,
		Env: map[string]string{
			"MYAPP_HOME": "%INSTALL_DIR%",
			"MYAPP_ENV":  "production",
		},
		Registry: []installer.RegistryEntry{
			// Empty Root follows Scope (HKCU for ScopeUser, HKLM for ScopeMachine).
			{Key: `Software\Classes\.myext`, Value: installer.StringValue("MyApp.Document")},
			{Key: `Software\Classes\MyApp.Document\shell\open\command`, Value: installer.StringValue(`"%EXE_PATH%" "%1"`)},
			{Key: `Software\Example\MyApp`, Name: "InstallDir", Value: installer.StringValue("%INSTALL_DIR%")},
			{Key: `Software\Example\MyApp`, Name: "AutoUpdate", Value: installer.DWORDValue(1)},
		},
	},
})
```

### Imperative (inside `AfterInstall` / `BeforeUninstall`)

Use `ctx` methods when values depend on user choices in custom wizard steps:

```go
AfterInstall: func(ctx *installer.Context) error {
	if ctx.GetBool("add_path") {
		if err := ctx.AddToPath(ctx.Dir()); err != nil { // or AddToUserPath / AddToMachinePath
			return err
		}
	}
	if err := ctx.SetEnv("MYAPP_API", ctx.GetString("api_url")); err != nil {
		return err
	}
	return ctx.SetRegistryDWORD(ctx.RegistryRoot(), `Software\Example\MyApp`, "Telemetry", 0)
}
```

Available helpers on `*installer.Context`:

| Category | Methods |
| --- | --- |
| Environment | `SetEnv`, `GetEnv`, `UnsetEnv`, `SetEnvForScope`, `GetEnvForScope`, `UnsetEnvForScope` |
| `PATH` | `AddToPath`, `RemoveFromPath`, `AddToUserPath`, `RemoveFromUserPath`, `AddToMachinePath`, `RemoveFromMachinePath` |
| Registry write | `SetRegistryString` (`REG_SZ`), `SetRegistryExpandString` (`REG_EXPAND_SZ`), `SetRegistryDWORD` (`REG_DWORD`), `SetRegistryQWORD` (`REG_QWORD`), `SetRegistryBinary` (`REG_BINARY`), `SetRegistryMultiString` (`REG_MULTI_SZ`), `SetRegistryValue` |
| Registry read/delete | `GetRegistryValue`, `DeleteRegistryValue`, `DeleteRegistryKey`, `RegistryRoot` (`RegistryHKCU` / `RegistryHKLM` / `RegistryHKCR`) |

## Silent / unattended CLI flags

Compiled Windows installers (`Name-Setup.exe` and `Uninstall.exe`) accept both
POSIX and classic Windows flags:

| Flag | Effect |
| --- | --- |
| `--silent`, `-s`, `/S`, `/quiet`, `/qn` | Install or uninstall without opening a window |
| `--dir PATH`, `-d PATH`, `--dir=PATH`, `/D=PATH` | Install into `PATH` |
| `--uninstall`, `/uninstall` | Uninstall the application |
| `--no-start-menu`, `--no-startmenu` | Do not create a Start Menu shortcut |
| `--no-desktop-shortcut`, `--no-desktop` | Do not create a Desktop shortcut |
| `--no-launch` | Do not launch the app after installing |
| `-h`, `--help`, `/?` | Print usage |

## Headless UI testing

Use `installer.NewContext` and `ctx.BuildWidget` with `ggui.Probe` to test your
custom steps and hooks headlessly on any OS:

```go
ctx, err := installer.NewContext(myApp, nil, installer.NewPreviewBackend())
probe := ggui.ProbeBuilder(func() ggui.Widget {
	return ctx.BuildWidget(probe.Post, probe.Dialogs(), func() {})
}, ggui.Sz(540, 390))
```
