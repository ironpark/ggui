package config

import (
	"bytes"
	"strconv"
	"text/template"
)

// fileTemplate is the file ggui init writes: every key, each with what it
// does and its default, so the file is its own reference. Keys left at
// their default are commented out, which keeps the defaults in one place,
// this program, rather than frozen into every project at the version it
// was made with.
var fileTemplate = template.Must(template.New(FileName).Parse(`# ggui.toml configures how the ggui command builds, runs and bundles this
# application. Paths are relative to this file. Flags given to the ggui
# command take precedence over what is written here. An unknown key is an
# error, so a misspelling is caught rather than ignored.
# Reference: https://github.com/ironpark/ggui/blob/main/docs/cli.md

[app]
# The name people see: the .app bundle's name and the macOS menu title,
# and the Windows program's, Name.exe.
# Default: the name of the main package's directory.
name = {{.Name}}
# Reverse-DNS bundle identifier. macOS keys preferences, permissions and
# the Dock to it, so pick it once and keep it.
id = {{.ID}}
# User-visible version, one to three numbers: Info.plist's
# CFBundleShortVersionString and CFBundleVersion, and the Windows
# program's file and product version, where each number is at most 65535.
version = {{.Version}}
# A square PNG, ideally 1024x1024, turned into the macOS app icon and the
# Windows program's icon; or an SVG with a square viewBox, which the build
# draws at 1024x1024 with this module's ggfx.
# Default: none, and the app has the generic icon.
# icon = "icon.png"
# The directory of the main package.
# package = "."

[build]
# Build tags for release builds (ggui build).
# tags = []
# Linker flags added after -s -w, such as "-X main.version=1.2.0".
# ldflags = ""
# Where ggui build writes its output.
# output = "build"
# Graphics driver implementation: "go" (default pure-Go driver) or "zig"
# (native Zig driver: Metal on macOS, Direct3D 12 on Windows, Vulkan on
# Linux). Release bundles ship the shared library beside the app; dev,
# native and record builds embed it. Same as --renderer.
# renderer = "go"

[dev]
# Build tags for ggui dev. ggui_inspector puts the widget inspector in the
# build (bind it with Config.Inspector); ggui_debug panics on a state write
# from the wrong goroutine and names where effects were made. Neither
# reaches a release build.
# tags = ["ggui_inspector", "ggui_debug"]
# Arguments given to the app on every run; ggui dev -- args adds more.
# args = []
# Environment variables set for the app, such as { LOG_LEVEL = "debug" }.
# env = {}
# Files or directories to watch besides the Go sources the app is built
# from, the files it embeds and this file: assets read at run time.
# watch = []
# Where the app's window shows: "remote" keeps one window, the viewer's,
# open while the app is rebuilt and restarts; "window" gives each run a
# window of its own, as a release build has. Remote is the default on
# macOS, Linux and Windows; ggui dev --display overrides this.
# display = "remote"
# Directory where ggui dev creates Name.dev.app on macOS.
# Default: build.output. Same as --bundle-dir.
# bundle_dir = ""
# macOS signing identity for Name.dev.app, so TCC permissions (camera,
# screen recording, notifications) survive rebuilds. "-" signs ad hoc.
# Default: macos.signing_identity when set, else an Apple Development
# identity from the keychain, else ad hoc. Same as --sign.
# identity = ""

[macos]
# The oldest macOS the app runs on: LSMinimumSystemVersion. Programs
# built by Go 1.27 need macOS 13 whatever this says.
# minimum_system_version = "13.0"
# The App Store category, such as "public.app-category.productivity".
# category = ""
# Scale app.icon into the middle 824x824 of 1024 on transparency, as
# Apple's icon grid places a macOS icon, for artwork drawn edge to edge.
# The rounded square shape is the artwork's to have. Windows' icon keeps
# the edges.
# icon_inset = false
# The keychain identity to sign with, such as "Developer ID Application"
# (the first one of that kind) or "Developer ID Application: Name
# (TEAMID)"; security find-identity -v -p codesigning lists them. "-"
# signs ad hoc, which runs on this Mac but not on others. An identity
# also gets the hardened runtime and a secure timestamp, as notarization
# requires. Same as --identity.
# signing_identity = "-"
# An entitlements plist to sign with.
# entitlements = ""
# A PKCS#12 (.p12) certificate and key to sign with instead of the
# keychain, as CI has: a file, or else base64 text in the environment
# variable p12_base64_env names. Its password is read from
# p12_password_file, or else from the variable p12_password_env names.
# signing_identity "-" then means the certificate's Developer ID
# Application identity. Same as --p12-file and --p12-password-file.
# p12_file = ""
# p12_base64_env = "GGUI_P12_BASE64"
# p12_password_file = ""
# p12_password_env = "GGUI_P12_PASSWORD"
# Submit the app, and the disk image and installer, to Apple's notary
# service with the credentials in [macos.notary], and staple the ticket,
# so Gatekeeper opens them on any Mac. It takes minutes and a Developer
# ID identity. Same as --notarize.
# notarize = false
# Build for both Apple silicon and Intel and join the two into one
# universal program, on any host. Same as --universal.
# universal = false

# Extra Info.plist keys, written over the generated ones.
# [macos.plist]
# NSCameraUsageDescription = "Scans documents."

# How to reach the notary service: a notarytool keychain profile (made
# once with xcrun notarytool store-credentials NAME), an App Store
# Connect API key, or an Apple ID with an app-specific password, which is
# read from the variable password_env names. The first given is used.
# Same as --notary-profile, --api-key-file, --apple-id and --team-id.
# [macos.notary]
# profile = ""
# The key as JSON: {"issuer_id", "key_id", "private_key"}, the last the
# .p8 file's contents, as rcodesign encode-app-store-connect-api-key
# writes it.
# api_key_file = ""
# apple_id = ""
# team_id = ""
# password_env = "GGUI_NOTARY_PASSWORD"
# Attach the ticket, so Gatekeeper need not reach Apple to check it.
# staple = true
# How long to wait for Apple's verdict.
# timeout = "1h"

# The table asks for a disk image, Name.dmg, beside the bundle on every
# macOS build: the app and a link to /Applications to drag it onto.
# --dmg makes one without the table, --dmg=false skips it.
# [macos.dmg]
# The volume's name. Default: app.name.
# title = ""
# The volume's icon, a PNG or .icns. Default: the app's icon.
# icon = ""
# The window's background image, drawn at its top left in points.
# background = ""
# window = { width = 640, height = 480 }
# icon_size = 128
# label_size = 14
# The centre of each icon, [x, y] in points from the window's top left.
# Default: side by side across the middle of the window.
# app_position = []
# applications_position = []
# The volume's file system: "hfsplus", "apfs" or "apfs-case-sensitive".
# fs = "hfsplus"
# "udzo" (zlib), which every macOS opens, or "ulfo" (lzfse), smaller.
# format = "udzo"

# The table asks for an installer package, Name.pkg, beside the bundle on
# every macOS build. --pkg makes one without the table.
# [macos.pkg]
# The installer identity, which differs from the app's. Default: the
# Developer ID Installer identity of the keychain or the certificate.
# The package is left unsigned when the app is signed ad hoc.
# signing_identity = ""
# install_location = "/Applications"
# A directory with preinstall and postinstall scripts.
# scripts = ""
# A plain-text license the installer shows and asks to accept.
# license = ""

[windows]
# ggui build --target windows makes Name.exe with app.icon as its icon, a
# manifest, and version information from [app] and the keys below, which
# Explorer shows under Properties > Details.
# The description Task Manager and the taskbar's menus call the program
# by. Default: app.name.
# description = ""
# The company and the copyright notice; left out when empty.
# company = ""
# copyright = ""
# Link the program for the console, so that Windows opens a console window
# for its output, as for debugging. By default it is a GUI program that
# opens none. Same as --console.
# console = false
# An application manifest to link instead of the one ggui writes, which
# asks for per-monitor DPI awareness, the themed common controls, the
# UTF-8 code page and long paths, and runs as the user who starts it.
# manifest = ""

[linux]
# ggui build --target linux makes an install tree, name-linux-arch: the
# program in usr/bin, a desktop entry named for app.id, app.icon in the
# hicolor sizes, and AppStream metadata. What to make of it besides:
# "tar" (name-version-linux-arch.tar.gz), "deb" and "rpm". Same as
# --format.
# formats = ["tar"]
# One line on what the app does: the menu entry's comment, the software
# centre's summary and the packages' description. Default: app.name.
# description = ""
# What kind of program it is, such as "Image Viewer".
# generic_name = ""
# Menu categories, from https://specifications.freedesktop.org/menu-spec/
# such as "Graphics", "Office" or "Development".
# categories = ["Utility"]
# MIME types the app opens, such as "image/png" or
# "x-scheme-handler/myapp"; with any, it is started with the files or
# URLs it is asked to open.
# mime_types = []
# Who maintains the packages, "Name <email>"; a Debian package must say.
# maintainer = ""
# homepage = ""
# The app's license, as an SPDX expression such as "MIT".
# license = ""
# What the packages depend on: the libraries the app opens at run time,
# X11 and GL, which a package manager cannot see for itself.
# depends_deb = ["libx11-6", "libxrandr2", "libxi6", "libxcursor1", "libxinerama1", "libxext6", "libxrender1", "libgl1", "libegl1"]
# depends_rpm = ["libX11", "libXrandr", "libXi", "libXcursor", "libXinerama", "libXext", "libXrender", "libglvnd-glx", "libglvnd-egl"]

[web]
# The page title. Default: app.name.
# title = ""
# The path the app is served below, such as "/app/". The page loads its
# files relative to it, and an app using router.Browser should pass the
# same path there.
# base = "/"
# binaryen wasm-opt flags for ggui build --target web; "" skips the pass.
# ggui dev --web skips it unless --wasm-opt is given, to keep rebuilds
# short. Without wasm-opt on PATH the binary ships as linked.
# wasm_opt = "-O2"

[i18n]
# The app's own functions that pass a message on to the i18n package, for
# ggui i18n check and extract to read their calls as i18n.T's: NAME, a
# function or a method of any type called so, or pkg.Name, followed by
# :POS when the message is not the first argument. As --func.
# funcs = ["tr", "ui.T"]
# Those like i18n.N: NAME, or NAME:ONE,OTHER when the singular and plural
# forms are not the second and third arguments. As --plural.
# plurals = ["trn"]

[release]
# ggui release builds the targets below for app.version, collects what
# they make in build/release/VERSION with SHA256SUMS, and publishes it.
# See docs/releasing.md.
# The platforms to build: "macos", "windows" and "linux". Same as --targets.
# targets = ["macos", "windows", "linux"]
# The architectures each is built for where the target has them: "amd64",
# "arm64", "386" and "riscv64". macOS builds one universal binary when both
# amd64 and arm64 are built.
# arches = ["amd64", "arm64"]
# Where to publish: "github", "homebrew" and "choco". Same as --publish.
# publish = ["github"]
# The GitHub repository, "owner/repo". Default: the one git's origin names.
# github = ""
# The GitHub REST API, another for GitHub Enterprise.
# github_api = "https://api.github.com"
# What goes before app.version in the tag the release is made at.
# tag_prefix = "v"
# A Markdown file whose text describes the release. Same as --notes.
# Default: a list of the files.
# notes = ""

# The Homebrew cask written into a tap for the macOS build.
# [release.homebrew]
# The tap's repository, which the token must be able to write.
# tap = "owner/homebrew-tap"
# The cask's token. Default: app.name, lowercased, with hyphens.
# cask = ""
# homepage = ""
# One line on what the app does, not starting with "A" or the app's name.
# desc = ""

# The Chocolatey package made for the Windows build. The API key is read
# from $CHOCOLATEY_API_KEY.
# [release.chocolatey]
# The package ID. Default: app.name, lowercased, with hyphens.
# id = ""
# authors = ""
# project_url = ""
# license_url = ""
# icon_url = ""
# tags = []
# summary = ""
# description = ""
# The feed pushed to.
# source = "https://push.chocolatey.org/"

# Platform installers built beside the app: Name.pkg on macOS and the
# ggui-based Name-Setup.exe on Windows. Adding [installer] enables both;
# adding only [installer.macos] or [installer.windows] enables that
# target alone. --installer makes one without the table, --installer=false
# skips it.
# [installer]
# A plain-text license file shown and accepted during installation.
# license = ""

# macOS .pkg installer options (keys in [macos.pkg] also work and take
# precedence when set).
# [installer.macos]
# enabled = true
# signing_identity = ""
# install_location = "/Applications"
# scripts = ""
# license = ""

# Windows Name-Setup.exe (ggui-based setup wizard and uninstaller) options.
# [installer.windows]
# enabled = true
# "user" (%LOCALAPPDATA%\Programs\Name, no UAC prompt) or "machine"
# (%ProgramFiles%\Name, asks for administrator elevation).
# scope = "user"
# Default install folder; may use %LOCALAPPDATA% or %ProgramFiles%.
# install_location = ""
# Plain-text license file, overriding installer.license.
# license = ""
# Publisher in Add/Remove Programs. Default: windows.company.
# publisher = ""
# Info URL in Add/Remove Programs. Default: linux.homepage.
# url = ""
# start_menu_shortcut = true
# desktop_shortcut = false
# Offer to launch the app on the final wizard step.
# launch_after = true
`))

// Template returns the commented file ggui init writes.
func Template(name, id, version string) []byte {
	var b bytes.Buffer
	err := fileTemplate.Execute(&b, map[string]string{
		"Name":    strconv.Quote(name),
		"ID":      strconv.Quote(id),
		"Version": strconv.Quote(version),
	})
	if err != nil {
		panic(err) // the template is fixed, and its data always strings
	}
	return b.Bytes()
}
