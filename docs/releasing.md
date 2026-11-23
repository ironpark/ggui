# Releasing an app

[Documentation](README.md) · [The ggui command](cli.md)

`ggui release` takes a ggui app from a commit to the people who install
it: it builds the macOS, Windows and Linux files, checksums them, and
publishes them as a GitHub release, a Homebrew cask and a Chocolatey
package. This page goes through it in order; [the command's
reference](cli.md#ggui-release) lists every flag and key.

- [From a tag to users](#from-a-tag-to-users)
- [Credentials](#credentials)
- [Homebrew and Gatekeeper](#homebrew-and-gatekeeper)
- [Chocolatey](#chocolatey)
- [Releasing from GitHub Actions](#releasing-from-github-actions)
- [Trying it first: the dry run](#trying-it-first-the-dry-run)
- [Troubleshooting](#troubleshooting)

## From a tag to users

```toml
[app]
name = "Hello World"
id = "com.example.hello"
version = "1.2.0"

[macos]
signing_identity = "Developer ID Application"
notarize = true

[macos.notary]
profile = "hello"

[linux]
formats = ["tar", "deb", "rpm"]
description = "Says hello"

[release]
publish = ["github", "homebrew", "choco"]

[release.homebrew]
tap = "alice/homebrew-tap"

[release.chocolatey]
authors = "Alice"
license_url = "https://github.com/alice/hello/blob/main/LICENSE"
icon_url = "https://raw.githubusercontent.com/alice/hello/main/icon.png"
tags = ["hello", "gui"]
```

1. **The version.** Set `app.version` and commit it; `--version` releases
   another without editing the file. The tag is `v` and the version,
   `v1.2.0`, or `release.tag_prefix` and the version.
2. **Checks.** The work tree must have nothing left to commit (or
   `--allow-dirty`), and the tag, if it exists already, here or on
   `origin`, must be at the commit checked out. Publishing to GitHub
   also needs the commit pushed, a token that works, and no published
   release at the tag. All of this is checked before anything is built.
3. **Builds.** Each of `release.targets` for each of `release.arches`
   it has: macOS as one `universal` build on any host whenever both `amd64`
   and `arm64` are enabled (joined in pure Go), signed and notarized as
   `[macos]` says; Windows and Linux cross-compiled. The files go in
   `build/release/1.2.0/`:

   ```text
   hello-world-1.2.0-macos-universal.dmg     (or .zip of Hello World.app; plus .pkg with [installer] / [macos.pkg])
   hello-world-1.2.0-windows-amd64.zip       (Hello World.exe; plus ggfx_driver.dll with renderer = "zig")
   hello-world-1.2.0-windows-amd64-setup.exe (with [installer] or [installer.windows])
   hello-world-1.2.0-windows-arm64.zip
   hello-world-1.2.0-windows-arm64-setup.exe
   hello-world-1.2.0-linux-amd64.tar.gz
   hello-world_1.2.0_amd64.deb
   hello-world-1.2.0-1.x86_64.rpm
   ...
   SHA256SUMS
   homebrew/Casks/hello-world.rb
   chocolatey/hello-world.nuspec, tools/, hello-world.1.2.0.nupkg
   ```

   Without `[macos.dmg]` the app ships as a zip, laid out as `ditto -c -k
   --keepParent` makes one, which unpacks to `Hello World.app` with its
   signature intact.
4. **GitHub.** The release is created as a draft, with `tag_name` and
   `target_commitish` set to the commit checked out. GitHub makes the tag
   when the release is published, so ggui pushes nothing with git; tag
   the commit yourself first if you prefer, and the release uses that
   tag. The files are uploaded, and only when all of them are is the
   draft published, so a failed run leaves a draft rather than a release
   with some files; running again takes it up.
5. **Homebrew.** The cask is committed to the tap as `Casks/TOKEN.rb`,
   with the message `Update TOKEN to 1.2.0`. People install with
   `brew install --cask alice/tap/hello-world`.
6. **Chocolatey.** The package is pushed; once moderated, people install
   with `choco install hello-world`.

## Credentials

| Channel | Needs | From |
| --- | --- | --- |
| GitHub | A token that can write the repository's contents (releases): a classic token with `repo`, a fine-grained one with *Contents: read and write*, or Actions' own `GITHUB_TOKEN` with `contents: write`. | `$GITHUB_TOKEN`, else `$GH_TOKEN`, else `gh auth token` when [gh](https://cli.github.com) is signed in. |
| Homebrew | A token that can write the *tap's* contents. Actions' `GITHUB_TOKEN` cannot write another repository, so give a fine-grained token for the tap. | `$HOMEBREW_TAP_TOKEN`, else the GitHub token. |
| Chocolatey | The API key of your [chocolatey.org account](https://community.chocolatey.org/account). | `$CHOCOLATEY_API_KEY` only; never `ggui.toml`. |
| macOS signing | A Developer ID certificate and notary credentials. | See [Shipping to other Macs](cli.md#shipping-to-other-macs). |

No token or key is printed, in a log or an error: errors say where a
token came from, such as `token from $GITHUB_TOKEN`, never what it is.

For GitHub Enterprise, set `release.github_api`, such as
`"https://github.example.com/api/v3"`; downloads are then on
`https://github.example.com`, and `gh auth token --hostname` is asked for
that host.

## Homebrew and Gatekeeper

The cask downloads the disk image or zip from the GitHub release and
checks its SHA-256. It names the app, `depends_on macos:` from
`macos.minimum_system_version` (or an `LSMinimumSystemVersion` in
`[macos.plist]`), a `livecheck` that follows the repository's latest
release, and a `zap` that removes the app's files in
`~/Library/Application Support`, `Preferences` and `Saved Application
State`, all named for `app.id`. When macOS is built as a `universal`
binary (the default whenever both `amd64` and `arm64` are enabled), the
cask points directly to that universal asset; a per-architecture release
gets an `arch` stanza with a checksum for each architecture built.

**Notarize what you put in a cask.** Homebrew sets the quarantine
attribute on what it downloads, as a browser does, so macOS checks the
app with Gatekeeper the first time it opens. An app signed ad hoc, or not
at all, installs, but macOS refuses to open it, and the user must allow
it in System Settings > Privacy & Security, or remove the attribute with
`xattr -dr com.apple.quarantine "/Applications/Hello World.app"`.
Homebrew used to offer `brew install --no-quarantine` for this; Homebrew
5.0 (November 2025) deprecated the flag on the way to removing it, and
announced that the official `homebrew/cask` tap would drop casks whose
apps fail Gatekeeper. A tap of your own is not reviewed, but macOS
behaves the same, so do not point users at `--no-quarantine`, which a
current Homebrew may no longer have. Sign with a Developer ID and
notarize, with `macos.notarize = true`, as [Shipping to other
Macs](cli.md#shipping-to-other-macs) explains; ggui release warns when the
cask's app is not notarized.

The tap is a GitHub repository named `homebrew-something`; create it once,
empty, and `release.homebrew.tap` names it, `"alice/homebrew-tap"`.

## Chocolatey

The package is made in Go, as `choco pack` would make it: `ID.nuspec`,
`tools/chocolateyinstall.ps1` and `tools/chocolateyuninstall.ps1`. The
install script downloads the Windows zip from the GitHub release with
`Install-ChocolateyZipPackage`, checking its SHA-256, marks the program a
GUI program for Chocolatey's shim (an empty `Name.exe.gui` beside it), and
adds a Start menu shortcut; uninstalling removes the shortcut. Since the
package embeds no program, it has no `VERIFICATION.txt` or `LICENSE.txt`,
which Chocolatey asks for of packages that do.

Chocolatey's helpers take a 32-bit and a 64-bit URL and no other, so the
package installs the `amd64` zip, or the `386` one on 32-bit Windows when
`release.arches` has `386`. Windows on Arm runs the `amd64` program; the
`arm64` zip is on the GitHub release for those who want it.

The source written beside the `.nupkg` in `build/release/VERSION/chocolatey`
packs to the same package with `choco pack`, if you want to check it on
Windows.

Things to know before the first push:

- **An account first.** Register at community.chocolatey.org, and take
  the API key from your account page. The first version of a package
  establishes its ID as yours; push it by hand the first time
  (`choco push hello-world.1.2.0.nupkg --source https://push.chocolatey.org/
  --api-key ...` from the `chocolatey` directory, or `ggui release
  --publish choco`), and check it appears under your account.
- **Moderation.** Every version of a community package is checked by
  automated validation and, until a package is trusted, by a human
  moderator before it is listed; that takes from hours to days. The
  validator wants a `description`, `project_url`, `license_url`,
  `icon_url` and `tags`; set them in `[release.chocolatey]`.
- **One version once.** A version cannot be pushed twice; the feed answers
  409. Bump the version.
- **Another feed.** `release.chocolatey.source` names any NuGet v2 feed,
  such as an internal repository; the package is sent as `choco push`
  sends it, a `PUT` of `multipart/form-data` to `api/v2/package/` below the
  source, with the key in `X-NuGet-ApiKey`.

## Releasing from GitHub Actions

A workflow that releases each tag pushed. It runs on macOS, which signs
and notarizes with Apple's tools, and cross-compiles the Windows and
Linux files there too.

```yaml
name: Release
on:
  push:
    tags: ["v*"]

permissions:
  contents: write # the release

jobs:
  release:
    runs-on: macos-26
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: stable
      - run: go install github.com/ironpark/ggui/cmd/ggui@latest
      # The version is the tag's, v1.2.0 releasing 1.2.0.
      - run: ggui release --version "${GITHUB_REF_NAME#v}"
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
          CHOCOLATEY_API_KEY: ${{ secrets.CHOCOLATEY_API_KEY }}
          GGUI_P12_BASE64: ${{ secrets.DEVELOPER_ID_P12_BASE64 }}
          GGUI_P12_PASSWORD: ${{ secrets.DEVELOPER_ID_P12_PASSWORD }}
          GGUI_NOTARY_PASSWORD: ${{ secrets.NOTARY_PASSWORD }}
```

`--version` releases the tag that started the workflow even when
`app.version` was not bumped with it, and stamps the builds with it.
Without it, the run releases `v` + `app.version`, which must then be the
tag pushed: a tag of that name at another commit stops it. Notarizing with an
Apple ID needs `macos.notary.apple_id` and `team_id` in `ggui.toml`; with
an App Store Connect API key, write the key's JSON to a file in a step
before and name it in `macos.notary.api_key_file`.

A Linux runner is cheaper and starts faster, and still builds a `universal`
macOS binary whenever both `amd64` and `arm64` are enabled. The released
Linux binary on GitHub (as well as Homebrew on Linux and `CGO_ENABLED=1 go
install`) already links zapp's `rcodesign`, so a Linux CI runner can sign
(with a `.p12` or PEM certificate) and notarize (with an App Store Connect
API key) macOS `.app` and `.dmg` releases for both GitHub and Homebrew
casks; only `.pkg` installers require a Mac. See [Signing and notarizing on
Linux and Windows](cli.md#signing-and-notarizing-on-linux-and-windows).

## Trying it first: the dry run

```sh
ggui release --dry-run
```

builds everything and writes `build/release/VERSION/` as a real run
would, cask and Chocolatey package included, then prints the requests a
real run would send instead of sending them:

```text
asset                                     size  sha256
hello-world-1.2.0-macos-universal.dmg   9.1 MB  c1ed2606f5db
...
Dry run: nothing was published. A real run would send:

# GitHub release v1.2.0 of alice/hello
GET    https://api.github.com/repos/alice/hello/releases?per_page=100  (look for a release or draft at v1.2.0)
POST   https://api.github.com/repos/alice/hello/releases  (draft, tag_name v1.2.0, ...)
POST   https://uploads.github.com/repos/alice/hello/releases/{id}/assets?name=...
PATCH  https://api.github.com/repos/alice/hello/releases/{id}  (draft false: publish)

# Homebrew cask hello-world in alice/homebrew-tap, from build/release/1.2.0/homebrew/Casks/hello-world.rb
GET    https://api.github.com/repos/alice/homebrew-tap/contents/Casks/hello-world.rb
PUT    https://api.github.com/repos/alice/homebrew-tap/contents/Casks/hello-world.rb

# Chocolatey package hello-world 1.2.0
PUT    https://push.chocolatey.org/api/v2/package/
```

A dry run needs no token and no key, and says what a real run would stop
at, a dirty tree or a tag elsewhere, without stopping. Read the cask and
the scripts it wrote before the first real release; `brew style --cask`
on the cask in a tap checks it as Homebrew's maintainers would.

## Troubleshooting

- **"the work tree has changes not committed"**: commit them, or add
  `build/` to `.gitignore` if it is the build's own output; `--allow-dirty`
  releases anyway, from a commit that is not what was built.
- **"the tag v1.2.0 is at ... on origin, not at this commit"**: the version
  was released from another commit. Bump `app.version`, or check out the
  tagged commit.
- **"alice/hello has no commit ..."**: push the commit first; GitHub makes
  the tag at a commit it has.
- **"the release v1.2.0 ... is published already"**: a version is
  released once. `--replace-assets` uploads to it anyway, replacing files
  of the same names, as to fix a broken file. The cask is committed again;
  Chocolatey refuses a version pushed already, so leave `choco` out with
  `--publish github,homebrew`.
- **A run failed half way.** The release is still a draft, with the files
  uploaded so far; run again and it is taken up, files of the same names
  replaced. Delete the draft on GitHub to start over.
- **401 or 403 from GitHub**: the token has expired or lacks the
  permission: *Contents: write* on the repository, and on the tap for
  Homebrew. An `upload` refused with 403 by Actions' token means the
  workflow lacks `permissions: contents: write`.
- **409 or 422 from the contents API for the tap**: the tap's default
  branch may be protected; let the token's owner push to it, or publish
  the cask from `build/release/VERSION/homebrew` by a pull request.
- **409 from Chocolatey**: that version was pushed already.
- **The app will not open after `brew install`**: it is not notarized; see
  [Homebrew and Gatekeeper](#homebrew-and-gatekeeper).
