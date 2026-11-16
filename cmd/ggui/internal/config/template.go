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
# The name people see: the .app bundle's name and the macOS menu title.
# Default: the name of the main package's directory.
name = {{.Name}}
# Reverse-DNS bundle identifier. macOS keys preferences, permissions and
# the Dock to it, so pick it once and keep it.
id = {{.ID}}
# User-visible version, one to three numbers: Info.plist's
# CFBundleShortVersionString and CFBundleVersion.
version = {{.Version}}
# A square PNG, ideally 1024x1024, turned into the macOS app icon.
# Default: none, and the bundle has the generic icon.
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
# macOS; ggui dev --display overrides this.
# display = "remote"

[macos]
# The oldest macOS the app runs on: LSMinimumSystemVersion. Programs
# built by Go 1.27 need macOS 13 whatever this says.
# minimum_system_version = "13.0"
# The App Store category, such as "public.app-category.productivity".
# category = ""
# The code signing identity, such as "Developer ID Application: Name
# (TEAMID)". "-" signs ad hoc, which runs on this Mac but not on others
# without a Gatekeeper prompt. Signing happens only on macOS.
# signing_identity = "-"
# An entitlements plist to sign with.
# entitlements = ""
# Build for both Apple silicon and Intel and join the two with lipo.
# Needs macOS with the Xcode command line tools. Same as --universal.
# universal = false

# Extra Info.plist keys, written over the generated ones.
# [macos.plist]
# NSCameraUsageDescription = "Scans documents."

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
