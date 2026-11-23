# Auto-updating an app

[Documentation](README.md) · [The ggui command](cli.md) · [Releasing an app](releasing.md) · [Writing an installer in Go](installer.md)

Package `github.com/ironpark/ggui/updater` adds **Sparkle 2-compatible**
automatic updates to any GGUI application in pure Go with **zero cgo** — no
Cocoa `Sparkle.framework`, WinSparkle DLL, or platform-specific C toolchains.

- **Sparkle 2 wire format:** reads standard RSS 2.0 `appcast.xml` feeds using
  the `http://www.andymatuschak.org/xml-namespaces/sparkle` namespace (including
  Sparkle 1 enclosure attribute fallbacks), channels, minimum OS versions,
  phased rollouts, critical updates, and `<description>` release notes
  (`ParseAppcast` preserves Markdown or HTML text as-is, while the built-in UI
  renders Markdown; use `sparkle:releaseNotesLink` for full HTML pages).
- **Ed25519 cryptographic signing:** uses the exact same base64 32-byte public
  key (`SUPublicEDKey`), 32-byte private seed, and 64-byte raw-file
  `sparkle:edSignature` format as Sparkle 2's `generate_keys` and `sign_update`,
  so existing Sparkle 2 keys and feeds interoperate directly.
- **Cross-platform installation:** built-in platform appliers handle macOS
  (`.zip` and `.dmg`), Windows (`-setup.exe` silent upgrades and portable
  `.zip`), and Linux (`.tar.gz`) with backup and rollback on failure.
- **Built-in GGUI update UI:** drop `updater.Dialog(ctrl)`,
  `updater.Panel(ctrl)`, or `updater.ShowWindow(app, ctrl)` straight into your
  app, or drive `Updater` headlessly with your own UI.

## On this page

- [Quick start](#quick-start)
- [Sparkle 2 appcast features](#sparkle-2-appcast-features)
- [Custom UI and headless API](#custom-ui-and-headless-api)
- [Platform installation behavior](#platform-installation-behavior)
- [Security model](#security-model)
- [CLI reference](#cli-reference)
- [Runnable example](#runnable-example)

## Quick start

### 1. Generate an Ed25519 key pair

Run `ggui update keygen` once per application:

```sh
ggui update keygen --out-private ~/.config/myapp/update.key
```

This writes the 32-byte base64 private key seed to `~/.config/myapp/update.key`
with mode `0600` (refusing to overwrite an existing file) and prints the public
key to standard output:

```text
PublicKey (SUPublicEDKey): <base64-32-byte-public-key>
```

Without `--out-private`, both `PublicKey (SUPublicEDKey)` and
`PrivateKey (GGUI_UPDATE_PRIVATE_KEY)` are printed.

- **Embed the public key** in your Go source (`updater.Config.PublicKey`).
- **Keep the private key secret** in `$GGUI_UPDATE_PRIVATE_KEY` locally or as a
  CI secret — never put it in `ggui.toml` or commit it to git.

### 2. Configure `[release.appcast]` in `ggui.toml`

Add a `[release.appcast]` table to `ggui.toml` so `ggui release` builds, signs,
merges, and uploads `appcast.xml` alongside your release archives:

```toml
[app]
name = "Hello World"
id = "com.example.hello"
version = "1.0.0"

[release]
publish = ["github"]

[release.appcast]
merge_from = "https://github.com/alice/hello/releases/latest/download/appcast.xml"
max_items = 20
```

Export your signing key (or add `GGUI_UPDATE_PRIVATE_KEY` to your GitHub Actions
secrets; see [Releasing an app](releasing.md#sparkle-2-appcast-feeds)) and cut a
release:

```sh
export GGUI_UPDATE_PRIVATE_KEY="$(cat ~/.config/myapp/update.key)"
ggui release
```

On the very first release, `merge_from` returns HTTP 404 and `ggui release`
warns and starts a new `appcast.xml`. On subsequent releases, prior versions are
merged automatically so older installations and multiple channels stay intact.

### 3. Embed the updater in your app

Create an `*updater.Updater`, wrap it in an `*updater.Controller`, attach it to
your `ggui.App`, mount `updater.Dialog(ctrl)` (or open a standalone window with
`updater.ShowWindow(app, ctrl)`), and wire a **Check for Updates…** menu item:

```go
package main

import (
	"context"
	"log"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/updater"
	"github.com/ironpark/ggui/widget"
)

const (
	appVersion = "1.0.0"
	feedURL    = "https://github.com/alice/hello/releases/latest/download/appcast.xml"
	publicKey  = "<PublicKey printed by ggui update keygen>"
)

func main() {
	u, err := updater.New(updater.Config{
		FeedURL:        feedURL,
		PublicKey:      publicKey,
		CurrentVersion: appVersion,
		AppID:          ggui.AppID(),
		AppName:        "Hello World",
	})
	if err != nil {
		log.Fatal(err)
	}

	ctrl := updater.NewController(u)

	var app *ggui.App
	app = ggui.New(ggui.Config{Title: "Hello World", Width: 800, Height: 600, Resizable: true}, func() ggui.Widget {
		page := widget.Column(
			ui.AppMenubar(app.Menu()),
			widget.Expanded(
				widget.Center(widget.Text("Hello World " + appVersion)),
			),
		).Stretch()

		// Mount the built-in modal update dialog above the main UI.
		return widget.Stack(page, updater.Dialog(ctrl)).Expand()
	})

	ctrl.Attach(app)
	app.SetMenu(
		ggui.Menu("Help",
			ggui.MenuAction("Check for Updates…", "", ctrl.CheckNow),
		),
	)

	app.Setup(func() {
		// Run periodic silent background checks respecting CheckInterval and
		// the user's AutoCheckEnabled preference.
		bgCtx, cancel := context.WithCancel(context.Background())
		stop := ctrl.StartBackground(bgCtx)
		ggui.OnCleanup(func() {
			cancel()
			stop()
		})
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

> [!TIP]
> Prefer a dedicated Sparkle-style update window instead of an in-window modal?
> Omit `updater.Dialog(ctrl)` from `widget.Stack` and call
> `updater.ShowWindow(app, ctrl)` (for example from your menu item or when a
> background check discovers an update). To embed update status and controls
> inline inside a Settings screen, render `updater.Panel(ctrl)`.

On macOS, `app.SetMenu` places the menu in the native menu bar and
`ui.AppMenubar` draws nothing; on Windows and Linux, `ui.AppMenubar` draws the
menu inside the window so the same tree works everywhere.

## Sparkle 2 appcast features

A generated `appcast.xml` is a standard RSS 2.0 document with Sparkle 2
elements:

```xml
<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"
     xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle"
     xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Hello World</title>
    <link>https://github.com/alice/hello</link>
    <description>Updates for Hello World</description>
    <language>en</language>
    <item>
      <title>Version 1.2.0</title>
      <sparkle:version>1.2.0</sparkle:version>
      <sparkle:shortVersionString>1.2.0</sparkle:shortVersionString>
      <pubDate>Mon, 24 Nov 2026 09:00:00 +0000</pubDate>
      <sparkle:minimumSystemVersion>13.0</sparkle:minimumSystemVersion>
      <sparkle:phasedRolloutInterval>86400</sparkle:phasedRolloutInterval>
      <description>## What's new
- Faster startup and lower memory usage.</description>
      <enclosure
        url="https://github.com/alice/hello/releases/download/v1.2.0/hello-world-1.2.0-macos-universal.dmg"
        length="9542041"
        type="application/octet-stream"
        sparkle:edSignature="..."
        sparkle:sha256="..."
        sparkle:os="macos"
        sparkle:arch="universal"/>
    </item>
  </channel>
</rss>
```

`ParseAppcast` accepts both Sparkle 2 child elements (`<sparkle:version>`,
`<sparkle:shortVersionString>`) and Sparkle 1 attributes on `<enclosure>`,
multiple `<enclosure>` entries inside a single `<item>` or one `<enclosure>`
per `<item>`, and both XML-escaped text and `<![CDATA[...]]>` inside
`<description>`.

### Release channels (`stable`, `beta`, `nightly`)

Set `Config.Channel` on the client and `channel` in `[release.appcast]` (or
`--channel` on `ggui update appcast`). Channel filtering follows Sparkle 2
inheritance rules:

| Client `Config.Channel` | Eligible `<item>` entries |
| --- | --- |
| `""`, `"stable"`, or `"default"` | Default-channel items only (items with no `<sparkle:channel>`, `stable`, or `default`). |
| Non-default (e.g. `"beta"`, `"nightly"`) | Both default-channel items **and** items whose `<sparkle:channel>` matches (case-insensitively), picking whichever eligible release has the highest version. |

This means a user on the `beta` channel automatically receives `2.0.0` (stable)
once it supersedes `2.0.0-beta.3` without switching channels. Note that
hyphenated prerelease versions such as `2.0.0-beta.3` are published with
`ggui update appcast --version` (since `ggui.toml` `app.version` uses numeric
`MAJOR.MINOR.PATCH`).

> [!NOTE]
> GitHub's `/releases/latest/download/appcast.xml` redirect only resolves to the
> latest **non-prerelease** release. If you publish `beta` or `nightly`
> prereleases into the same feed, host `appcast.xml` at a fixed HTTPS URL (such
> as GitHub Pages, Cloudflare R2, or S3) and merge new releases into that URL.

### Minimum OS version (`sparkle:minimumSystemVersion`)

If an item sets `<sparkle:minimumSystemVersion>`, `Updater` compares it against
the host OS version using `CompareVersion` and skips releases that require a
newer operating system:

- **macOS:** reads `ProductVersion` from
  `/System/Library/CoreServices/SystemVersion.plist` (e.g. `"14.5"`).
- **Windows:** reads `CurrentMajorVersionNumber`, `CurrentMinorVersionNumber`,
  and `CurrentBuildNumber` from
  `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion` (e.g. `"10.0.22631"`).
- **Override:** set `Config.OSVersion` explicitly if needed (for example in
  tests or on Linux).

During `ggui release`, macOS items automatically get
`<sparkle:minimumSystemVersion>` from `macos.minimum_system_version` (or
`macos.plist.LSMinimumSystemVersion`). When running `ggui update appcast
--min-os` over a directory containing macOS alongside Windows or Linux
artifacts, `--min-os` applies only to the macOS items so a macOS version like
`13.0` does not filter out Windows or Linux users.

### Phased rollout (`sparkle:phasedRolloutInterval`)

Gradually roll out an update over time by setting `phased_rollout = "168h"` in
`[release.appcast]` (or `--rollout 168h` on `ggui update appcast`), which writes
`<sparkle:phasedRolloutInterval>` in seconds:

- On first launch, `Updater` generates a cryptographically random `rollout_seed`
  in `[0, 1)` and persists it in `updater-state.json`, giving each installation
  a stable rollout bucket across restarts.
- During automatic checks, a release becomes eligible once
  `now >= pubDate + rollout_seed * interval`.
- **Manual bypass:** user-initiated checks (`ctrl.CheckNow()` or
  `CheckOptions{Manual: true}`) always bypass phased rollout delays so users
  who click **Check for Updates…** get the release immediately. Critical updates
  also bypass phased rollouts.

### Critical updates (`sparkle:criticalUpdate`)

Mark urgent security or compatibility releases with `critical = true` in
`[release.appcast]` (or `--critical` on `ggui update appcast`), which emits
`<sparkle:criticalUpdate/>`.

`ParseAppcast` also supports Sparkle 2's optional threshold attribute:

```xml
<sparkle:criticalUpdate sparkle:version="2.0.0"/>
```

When `sparkle:version` is present on `<sparkle:criticalUpdate>`, the update is
treated as critical only when the running `CurrentVersion` is strictly lower
than that threshold (`rel.IsCriticalFor(currentVersion)`).

Critical updates automatically bypass:
- Phased rollout delays (`sparkle:phasedRolloutInterval`)
- Previously skipped versions (`SkipVersion`)

### Skip This Version, check intervals, and `updater-state.json`

`Updater` persists per-installation preferences atomically (via temp file +
rename) in `updater-state.json` under `Config.StateDir` (default
`<os.UserConfigDir()>/<sanitized AppID>`):

```json
{
  "skipped_version": "1.2.0",
  "last_check": "2026-11-24T09:00:00Z",
  "auto_check": true,
  "rollout_seed": 0.4271938412,
  "has_rollout_seed": true
}
```

- **Skip This Version:** `ctrl.Skip()` (or `u.SkipVersion(rel.CompareVersion())`)
  suppresses automatic prompts for that version and older. Manual checks
  (`ctrl.CheckNow()` / `CheckOptions{Manual: true}`), `CheckOptions{IncludeSkipped: true}`,
  newer versions, and critical updates still surface. Call
  `u.ClearSkippedVersion()` to reset.
- **Automatic check cadence:** `Config.CheckInterval` defaults to
  `updater.DefaultCheckInterval` (24 hours). `u.AutoCheckEnabled()` defaults to
  `true` and can be toggled in a Settings screen via
  `u.SetAutoCheckEnabled(enabled)`. `u.ShouldAutoCheck()` and `StartBackground`
  both honor `AutoCheckEnabled()`, `LastCheckTime()`, and `CheckInterval`.

### Version comparison (`CompareVersion`)

`updater.CompareVersion(a, b)` implements Sparkle 2 / SemVer-compatible
ordering:

- Trims whitespace, strips an optional leading `v`/`V` before a digit, and
  ignores `+build` metadata.
- Compares numeric segments numerically without integer overflow, treating
  trailing `.0` components as equal (`1.2` == `1.2.0`).
- Supports both SemVer prereleases (`1.2.0-rc.1`, `1.2.0-beta.2`) and
  Sparkle-style inline prereleases (`1.2a2`, `1.2b1`, `1.2rc1`), sorting every
  prerelease below its corresponding release (`1.2.0-rc.1` < `1.2.0`).

## Custom UI and headless API

### Using `updater.Controller`

`updater.Controller` bridges `*updater.Updater` to GGUI's UI thread and drives
the built-in views:

| Function / Method | Purpose |
| --- | --- |
| `updater.NewController(u, cfg...)` | Create a controller managing check, download, install, and error states for `u`. |
| `ctrl.Attach(app)` | Bind the controller to `app` for thread-safe UI updates and graceful quit during install. |
| `ctrl.SetPost(fn)` / `ctrl.SetQuit(fn)` | Wire a custom UI-thread marshaler (`Post`) or application quit callback (`Quit`) without calling `Attach`. |
| `ctrl.Updater()` | Return the underlying `*updater.Updater`. |
| `ctrl.CheckNow()` | Run a manual check (bypasses phased rollout and skipped versions) and surface results to the user. |
| `ctrl.CheckSilent()` | Run an automatic check silently; only surfaces UI if an eligible update is found. |
| `ctrl.StartBackground(ctx)` | Start periodic background checks respecting `CheckInterval` and `AutoCheckEnabled()`; returns a `stop` function. |
| `ctrl.Download()` | Download and verify the discovered release, updating download progress. |
| `ctrl.DownloadAndInstall()` | Download, verify, and immediately begin installing the discovered release. |
| `ctrl.CancelDownload()` | Abort an active download and return to `PhaseAvailable` (or `PhaseIdle`). |
| `ctrl.Install()` | Apply the verified download via the platform applier and relaunch. |
| `ctrl.Skip()` | Record the offered release in `updater-state.json` and dismiss. |
| `ctrl.RemindLater()` | Dismiss the prompt without skipping the version. |
| `ctrl.Dismiss()` | Cancel any active check/download and close the update UI. |
| `ctrl.ToggleAutoCheck(enabled)` | Persist the automatic-check preference and update `ctrl.AutoCheck`. |
| `ctrl.Retry()` | Retry the most recently failed check, download, or install operation. |
| `updater.Dialog(ctrl)` | Built-in modal overlay dialog bound to `ctrl`. |
| `updater.Panel(ctrl)` | Embeddable update status/progress/release-notes panel (e.g. inside a Settings page). |
| `updater.ShowWindow(app, ctrl)` | Open a standalone 520×420 update window bound to `ctrl`. |

Optional `updater.ControllerConfig` fields passed to `NewController`:
`Post` (UI goroutine marshaler), `Quit` (shutdown callback for `ApplyOptions.QuitFunc`),
`Relaunch` (`*bool`, defaults to `true`), `SilentInstall` (`*bool`, defaults to `true`),
and `OnLink` (custom handler for links in Markdown release notes).

`Controller` exposes reactive `*ggui.StateValue` fields updated on the UI thread
for building custom views: `Open` (`bool`), `Phase` (`updater.Phase`),
`Release` (`*updater.Release`), `Downloaded` (`*updater.DownloadedUpdate`),
`Progress` (`float64` in `[0, 1]`), `ProgressText` (`string`),
`ErrText` (`string`), `AutoCheck` (`bool`), and `ManualCheck` (`bool`).
`updater.Phase` constants are `PhaseIdle`, `PhaseChecking`, `PhaseUpToDate`,
`PhaseAvailable`, `PhaseDownloading`, `PhaseReadyToInstall`, `PhaseInstalling`,
and `PhaseError`.

You can compose `updater.Panel(ctrl)` inside your own layout or bind custom
widgets directly to these reactive state signals while calling the methods above
for actions.

### Headless `Updater` API

For CLI tools, custom wizards, or full control over every step, use `*updater.Updater`
directly:

```go
u, err := updater.New(updater.Config{
	FeedURL:        "https://github.com/alice/hello/releases/latest/download/appcast.xml",
	PublicKey:      publicKey,
	CurrentVersion: "1.0.0",
	Channel:        "stable",
})
if err != nil {
	return err
}

// 1. Check the appcast (use Check(ctx) for automatic checks).
rel, ok, err := u.CheckWithOptions(ctx, updater.CheckOptions{Manual: true})
if err != nil || !ok {
	return err
}

// 2. Stream download into CacheDir, checking size, SHA-256, and Ed25519 signature.
upd, err := u.Download(ctx, rel, func(p updater.DownloadProgress) {
	if p.Total > 0 {
		log.Printf("Downloading %s: %.0f%% (%d/%d bytes)",
			rel.DisplayVersion(), p.Fraction*100, p.Downloaded, p.Total)
	}
})
if err != nil {
	return err
}

// 3. Apply the verified archive using the platform's default applier and relaunch.
return u.Apply(ctx, upd, updater.ApplyOptions{
	Relaunch: true,
	Silent:   true,
	QuitFunc: app.Quit,
})
```

To run automatic checks headlessly without `Controller`, use
`stop := u.StartBackground(ctx, func(rel *updater.Release) { ... })`. To
customize installation, implement `updater.Applier` (or wrap
`updater.DefaultApplier()`) and assign it to `updater.Config.Applier`.

### `updater.Config` reference

| Field | Default | Use |
| --- | --- | --- |
| `FeedURL` | *(required)* | Sparkle 2 `appcast.xml` URL. Must be `https://` unless `AllowInsecureHTTP` is true. |
| `PublicKey` | *(required)* | Base64 32-byte Ed25519 public key (`SUPublicEDKey`). Required unless `AllowUnsigned` is true. |
| `CurrentVersion` | `""` | Currently running app version (e.g. `"1.2.0"`). |
| `Channel` | `""` (`stable`) | Update channel (e.g. `"beta"`, `"nightly"`). Empty, `"stable"`, or `"default"` receives default-channel items only. |
| `AppID` | `ggui.AppID()` | Reverse-DNS identifier used for default `StateDir` and `CacheDir`. |
| `AppName` | `AppID` | Display name used in UI and default HTTP `User-Agent`. |
| `OS`, `Arch` | `runtime.GOOS` / `GOARCH` | Normalized host OS (`"macos"`, `"windows"`, `"linux"`) and architecture (`"amd64"`, `"arm64"`, `"386"`, `"riscv64"`). |
| `OSVersion` | host OS version | Host OS version for `sparkle:minimumSystemVersion` checks. |
| `CheckInterval` | `24 * time.Hour` | Minimum duration between automatic checks (`DefaultCheckInterval`). |
| `MaxDownloadBytes` | `512 MiB` | Maximum permitted archive download size (`DefaultMaxDownloadBytes`). |
| `StateDir` | `<UserConfigDir>/<AppID>` | Directory where `updater-state.json` is stored. |
| `CacheDir` | `<UserCacheDir>/<AppID>/updates` | Directory where verified update archives are downloaded. |
| `UserAgent` | `<AppName>/<Ver> (<OS>; <Arch>)` | HTTP `User-Agent` header for feed and archive requests. |
| `HTTPClient` | default client | Optional `*http.Client`; redirect validation always enforces HTTPS unless `AllowInsecureHTTP` is set. |
| `Applier` | `DefaultApplier()` | Custom platform `Applier` implementation. |
| `PreferWindowsSetup` | `false` (auto-detect) | On Windows, force preferring `-setup.exe` over portable `.zip`. When `false`, auto-detects `install-manifest.json` beside the executable. |
| `AllowInsecureHTTP` | `false` | Allow `http://` URLs — **local development and tests only**. |
| `AllowUnsigned` | `false` | Allow missing Ed25519 keys/signatures — **local development and tests only**. |
| `Now` | `time.Now` | Clock override for deterministic tests. |

## Platform installation behavior

When selecting an `<enclosure>`, `Updater` normalizes `sparkle:os` (an omitted
`sparkle:os` defaults to `"macos"` per Sparkle 2 convention; compound tags like
`windows-x64` or `windows-arm64` are also understood) and prefers an exact
`sparkle:arch` match, then macOS `universal`, then an unspecified architecture.

Once downloaded and cryptographically verified, `updater.DefaultApplier()`
handles installation per platform:

### macOS (`.zip` and `.dmg`)

- Extracts `.zip` archives or mounts `.dmg` disk images into an isolated staging
  directory.
- Verifies that the incoming `.app` bundle's `CFBundleIdentifier` and signing
  identity (`TeamIdentifier`/`Authority`) match the currently running
  application bundle and validates its code signature before replacing anything.
- Atomically swaps the running `.app` bundle with a backup copy, rolling back
  automatically if replacement fails.
- Calls `QuitFunc` (if provided) and relaunches the updated bundle via
  `open -n` when `ApplyOptions.Relaunch` is true.

### Windows (`-setup.exe` and portable `.zip`)

`Updater` automatically detects how the app was installed:

- **Installed via a GGUI Windows installer** (`install-manifest.json` exists
  beside the running `.exe`, or `Config.PreferWindowsSetup` is true): selects
  the `-setup.exe` enclosure and launches it silently with
  `--silent --dir <installDir>` (plus any allowlisted boolean flags from
  `sparkle:installerArguments` and caller-provided `ApplyOptions.ExtraArgs`).
  Untrusted `sparkle:installerArguments` from appcast XML is restricted to a
  strict allowlist of benign boolean flags so feeds cannot inject arbitrary
  paths or arguments. Your Go installer's hooks, shortcuts, PATH, and registry
  entries run cleanly as an in-place upgrade, and the app relaunches once setup
  completes. See [Writing an installer in Go](installer.md).
- **Portable install** (no `install-manifest.json` beside the executable):
  selects the portable `.zip` enclosure, stages extraction, renames the active
  files to `.old` (working around Windows' running-executable lock) with
  rollback on failure, moves the new files into place, and relaunches.

If only one package format is published in the appcast, `Updater` falls back to
whichever matches the host architecture.

### Linux (`.tar.gz`)

- Extracts the `.tar.gz` archive (matching `ggui build --target linux` /
  `ggui release` tarball output) into a staging directory.
- Atomically swaps the running executable with backup and rollback on error,
  then relaunches.
- `.deb` and `.rpm` packages are intentionally excluded from appcast feeds so
  APT, DNF, and other system package managers remain authoritative for
  packaged installs.

## Security model

Every step of the update pipeline is hardened by default:

1. **HTTPS enforced across redirects:** `FeedURL`, enclosure `url`, and every
   HTTP redirect target (up to 10 hops) must use `https://` (`ErrInsecureURL`).
   Even if you supply a custom `Config.HTTPClient`, `Updater` wraps its
   `CheckRedirect` callback to block HTTPS-to-HTTP downgrades unless
   `AllowInsecureHTTP: true` is explicitly set.
2. **Mandatory Ed25519 verification before extraction or execution:**
   - `updater.New` refuses to construct an `Updater` without a valid 32-byte
     Ed25519 `PublicKey` (`ErrMissingSignature`).
   - `Download` rejects missing or malformed `sparkle:edSignature` attributes
     before opening a network connection, streams into a mode-`0600` `.part`
     file, and verifies the Ed25519 signature over the exact raw archive bytes
     before atomically renaming the file into `CacheDir`. Failed downloads
     delete the `.part` file automatically.
   - `Apply` refuses any `*DownloadedUpdate` unless `Verified` is `true` and
     re-verifies the Ed25519 signature (and `sparkle:sha256` when present) on
     disk before installing (`ErrInvalidSignature`), unless
     `AllowUnsigned: true` is set for local testing.
3. **Optional SHA-256 digest verification:** whenever `sparkle:sha256` is
   present on an enclosure (always emitted by `ggui release` and
   `ggui update appcast`), `Download` compares the streamed digest in constant
   time (`ErrHashMismatch`).
4. **Bounded memory and disk usage:** `ParseAppcast` caps feed XML at 4 MiB;
   `Download` enforces `Config.MaxDownloadBytes` (default 512 MiB,
   `ErrDownloadTooLarge`) against both `Content-Length` and the live stream via
   `io.LimitReader`, and sanitizes download filenames.
5. **Archive extraction, bundle integrity, and installer argument hardening:**
   archive extractors guard against zip-slip path traversal and symlink escapes
   outside the staging directory; on macOS, `CFBundleIdentifier`, code signature
   validity, and matching signing identity (`TeamIdentifier`/`Authority`) are
   verified before swapping the `.app` bundle; on Windows, untrusted
   `sparkle:installerArguments` from appcast XML is restricted to a strict
   allowlist of benign boolean flags.
6. **Private key hygiene:** the Ed25519 private key is never allowed in
   `ggui.toml`, is never printed by `ggui release` errors, and
   `ggui update keygen --out-private` writes the key with atomic `O_EXCL` and
   permissions `0600`.

### Sentinel errors

Inspect failures with `errors.Is`:

| Error | Meaning |
| --- | --- |
| `updater.ErrMissingSignature` | `PublicKey` or enclosure `sparkle:edSignature` is missing while signing is required. |
| `updater.ErrInvalidSignature` | Ed25519 signature failed verification or had invalid base64 / byte length. |
| `updater.ErrInsecureURL` | Feed URL, enclosure URL, or a redirect target used a non-HTTPS scheme. |
| `updater.ErrHashMismatch` | Downloaded archive did not match enclosure `sparkle:sha256`. |
| `updater.ErrDownloadTooLarge` | Archive exceeded `Config.MaxDownloadBytes`. |
| `updater.ErrUnsupportedPlatform` | No platform `Applier` is registered for the current OS. |

## CLI reference

`ggui update` exposes four subcommands for managing keys, signatures, and
standalone appcasts outside `ggui release` (see [The ggui command](cli.md#ggui-update)
for the complete flag table):

```sh
# Generate a Sparkle 2-compatible Ed25519 key pair (mode 0600 key file)
ggui update keygen --out-private ~/.config/myapp/update.key

# Sign a single release file (reads $GGUI_UPDATE_PRIVATE_KEY or --key-file)
ggui update sign --key-file ~/.config/myapp/update.key dist/myapp-1.2.0-macos-universal.dmg

# Verify a release file against a signature and public key
ggui update verify \
  --signature "<sparkle:edSignature>" \
  --public-key "<SUPublicEDKey>" \
  dist/myapp-1.2.0-macos-universal.dmg

# Scan a directory of release artifacts, sign them, merge history, and write appcast.xml
ggui update appcast \
  --dir build/release/1.2.0 \
  --base-url https://github.com/alice/myapp/releases/download/v1.2.0 \
  --version 1.2.0 \
  --channel beta \
  --min-os 13.0 \
  --rollout 24h \
  --notes RELEASE_NOTES.md \
  --merge https://github.com/alice/myapp/releases/latest/download/appcast.xml \
  --out build/release/1.2.0/appcast.xml
```

For automated multi-platform builds and publishing in CI, prefer
[`[release.appcast]`](cli.md#releaseappcast) with `ggui release` — see
[Releasing an app](releasing.md#sparkle-2-appcast-feeds).

## Runnable example

Explore an end-to-end interactive demo with a local Sparkle 2 appcast server,
real Ed25519 signing and verification, `stable`/`beta` channel switching, manual
and silent background checks, skipping and clearing versions, a simulated live
`Applier`, and `updater.Dialog(ctrl)` without touching your system:

```sh
go run ./docs/examples/updater
```

Source: [docs/examples/updater](examples/updater) · [Examples catalog](examples/README.md).
