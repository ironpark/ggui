// Package config reads and writes ggui.toml, the file that says how a ggui
// application is named, built, run during development and bundled.
//
// Every field has a default, so a project without the file still builds;
// the file only records what differs. A key the schema does not know is an
// error rather than something ignored, because a misspelt key that is
// silently dropped is a setting the author believes in and the build never
// applies.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// FileName is the name the file is looked for under.
const FileName = "ggui.toml"

// Config is the whole file. Paths in it are relative to the directory the
// file is in, which Project resolves.
type Config struct {
	App     App     `toml:"app"`
	Build   Build   `toml:"build"`
	Dev     Dev     `toml:"dev"`
	MacOS   MacOS   `toml:"macos"`
	Windows Windows `toml:"windows"`
	Linux   Linux   `toml:"linux"`
	Web     Web     `toml:"web"`
	I18n    I18n    `toml:"i18n"`
	Release Release `toml:"release"`
}

// Release is what ggui release builds and where it publishes it.
type Release struct {
	// Targets are the platforms built: "macos", "windows" and "linux".
	Targets []string `toml:"targets"`
	// Arches are the architectures each target is built for, where the
	// target has them: "amd64", "arm64", "386" and "riscv64".
	Arches []string `toml:"arches"`
	// GitHub is the repository released to, "owner/repo"; empty is the
	// one git's origin remote names.
	GitHub string `toml:"github"`
	// GitHubAPI is the REST API's root, another for GitHub Enterprise.
	GitHubAPI string `toml:"github_api"`
	// TagPrefix goes before the version to make the tag, v1.2.0.
	TagPrefix string `toml:"tag_prefix"`
	// Publish are the channels: "github", "homebrew" and "choco".
	Publish []string `toml:"publish"`
	// Notes is a Markdown file whose text is the release's description.
	Notes      string     `toml:"notes"`
	Homebrew   Homebrew   `toml:"homebrew"`
	Chocolatey Chocolatey `toml:"chocolatey"`
}

// Homebrew is the cask ggui release writes into a tap.
type Homebrew struct {
	// Tap is the tap's repository, "owner/homebrew-name".
	Tap string `toml:"tap"`
	// Cask is the cask's token; empty is made from app.name.
	Cask     string `toml:"cask"`
	Homepage string `toml:"homepage"`
	Desc     string `toml:"desc"`
}

// Chocolatey is the package ggui release makes for Chocolatey. The API key
// comes from $CHOCOLATEY_API_KEY only.
type Chocolatey struct {
	// ID is the package's ID; empty is app.name lowercased.
	ID          string   `toml:"id"`
	Authors     string   `toml:"authors"`
	ProjectURL  string   `toml:"project_url"`
	LicenseURL  string   `toml:"license_url"`
	IconURL     string   `toml:"icon_url"`
	Tags        []string `toml:"tags"`
	Summary     string   `toml:"summary"`
	Description string   `toml:"description"`
	// Source is the feed pushed to.
	Source string `toml:"source"`
}

// What [release] may name.
var (
	ReleaseTargets  = []string{"macos", "windows", "linux"}
	ReleaseArches   = []string{"amd64", "arm64", "386", "riscv64"}
	ReleaseChannels = []string{"github", "homebrew", "choco"}
)

// I18n names the app's own functions that pass a message on to the i18n
// package, for ggui i18n to read their calls as it reads i18n.T's and
// i18n.N's, in the syntax of its --func and --plural.
type I18n struct {
	// Funcs are like i18n.T: NAME or NAME:POS, POS the index of the
	// message's argument, 0 by default.
	Funcs []string `toml:"funcs"`
	// Plurals are like i18n.N: NAME or NAME:ONE,OTHER, the indexes of
	// the singular and the plural form, 1,2 by default.
	Plurals []string `toml:"plurals"`
}

// App names the application and says where its main package is.
type App struct {
	// Name is the name people see: the bundle's name and its menu title.
	// Empty means the main package directory's name.
	Name string `toml:"name"`
	// ID is the reverse-DNS bundle identifier, such as com.example.hello.
	ID string `toml:"id"`
	// Version is the user-visible version, up to three numbers.
	Version string `toml:"version"`
	// Icon is a square PNG, ideally 1024×1024, made into the bundle icon
	// and the Windows program's.
	Icon string `toml:"icon"`
	// Package is the main package, as a directory relative to the file.
	Package string `toml:"package"`
}

// Build holds what every release build is given.
type Build struct {
	Tags    []string `toml:"tags"`
	LDFlags string   `toml:"ldflags"`
	Output  string   `toml:"output"`
}

// Dev holds what ggui dev builds and runs the app with.
type Dev struct {
	Tags  []string          `toml:"tags"`
	Args  []string          `toml:"args"`
	Env   map[string]string `toml:"env"`
	Watch []string          `toml:"watch"`
	// Display is where the app's window shows: "remote", in a viewer
	// that stays while the app restarts, or "window", the app's own.
	// Empty is remote where it is available, macOS for now.
	Display string `toml:"display"`
	// BundleDir is the directory where ggui dev creates Name.dev.app on
	// macOS. Empty is build.output.
	BundleDir string `toml:"bundle_dir"`
	// Identity names the macOS signing identity for Name.dev.app, or "-"
	// to sign ad hoc. Empty uses macos.signing_identity (when set and
	// not "-"), then the keychain's first Apple Development identity,
	// then ad hoc.
	Identity string `toml:"identity"`
}

// MacOS holds what only the .app bundle, and the disk image and installer
// made from it, use.
//
// The keys that sign and notarize follow the vocabulary of zapp's
// .zapp.yaml, which does the work, in TOML's spelling. Nothing here holds
// a secret: a password or a certificate's contents comes from an
// environment variable the file names, or from a file.
type MacOS struct {
	MinimumSystemVersion string `toml:"minimum_system_version"`
	Category             string `toml:"category"`
	// IconInset scales app.icon into the body of Apple's icon grid,
	// 824×824 centred in 1024×1024 on transparency, for the bundle's
	// icon, as for artwork drawn edge to edge for Windows. The rounded
	// square shape is the artwork's own.
	IconInset bool `toml:"icon_inset"`
	// SigningIdentity names a keychain identity, or "-" to sign ad hoc.
	SigningIdentity string `toml:"signing_identity"`
	Entitlements    string `toml:"entitlements"`
	// P12File is a PKCS#12 certificate and key to sign with instead of
	// the keychain's, as CI has one; P12Base64Env names a variable that
	// holds one as base64 text, read when there is no file.
	P12File         string `toml:"p12_file"`
	P12Base64Env    string `toml:"p12_base64_env"`
	P12PasswordFile string `toml:"p12_password_file"`
	P12PasswordEnv  string `toml:"p12_password_env"`
	// Notarize submits what is built to Apple's notary service, with the
	// credentials in Notary, and staples the ticket.
	Notarize  bool           `toml:"notarize"`
	Universal bool           `toml:"universal"`
	Plist     map[string]any `toml:"plist"`
	Notary    Notary         `toml:"notary"`
	DMG       DMG            `toml:"dmg"`
	PKG       PKG            `toml:"pkg"`
}

// Notary is how to reach Apple's notary service: a notarytool keychain
// profile, an App Store Connect API key, or an Apple ID with an
// app-specific password and its team. The first of them given is used.
type Notary struct {
	Profile     string `toml:"profile"`
	APIKeyFile  string `toml:"api_key_file"`
	AppleID     string `toml:"apple_id"`
	TeamID      string `toml:"team_id"`
	PasswordEnv string `toml:"password_env"`
	Staple      bool   `toml:"staple"`
	Timeout     string `toml:"timeout"`
}

// DMG is the disk image made beside the bundle: the app and a link to
// /Applications to drag it onto, in a Finder window laid out as given.
type DMG struct {
	// Make is whether the file has a [macos.dmg] table, which asks for
	// the disk image on every macOS build.
	Make       bool   `toml:"-"`
	Title      string `toml:"title"`
	Icon       string `toml:"icon"`
	Background string `toml:"background"`
	Window     Window `toml:"window"`
	IconSize   int    `toml:"icon_size"`
	LabelSize  int    `toml:"label_size"`
	// AppPosition and ApplicationsPosition are [x, y] of each icon's
	// centre; empty places them side by side across the window.
	AppPosition          []int  `toml:"app_position"`
	ApplicationsPosition []int  `toml:"applications_position"`
	FS                   string `toml:"fs"`
	Format               string `toml:"format"`
}

// Window is the size of the Finder window a disk image opens in.
type Window struct {
	Width  int `toml:"width"`
	Height int `toml:"height"`
}

// PKG is the installer package made beside the bundle, which installs
// the app into InstallLocation.
type PKG struct {
	// Make is whether the file has a [macos.pkg] table.
	Make bool `toml:"-"`
	// SigningIdentity names the installer identity; empty is the
	// keychain's or the certificate's Developer ID Installer.
	SigningIdentity string `toml:"signing_identity"`
	InstallLocation string `toml:"install_location"`
	Scripts         string `toml:"scripts"`
	License         string `toml:"license"`
}

// Defaults for a disk image, which are zapp's.
const (
	DefaultDMGWidth     = 640
	DefaultDMGHeight    = 480
	DefaultDMGIconSize  = 128
	DefaultDMGLabelSize = 14
)

// Windows holds what only the Windows program uses: what its version
// information says beyond [app], and how it is linked. Its icon is
// app.icon's, its version app.version.
type Windows struct {
	// Description is the version information's FileDescription, which
	// Task Manager and the taskbar's menus call the program by; empty is
	// the app's name.
	Description string `toml:"description"`
	// Company and Copyright are its CompanyName and LegalCopyright, left
	// out when empty.
	Company   string `toml:"company"`
	Copyright string `toml:"copyright"`
	// Console links the program for the console, which Windows opens a
	// window of for it, as for a build whose output is to be read; by
	// default it is a GUI program and opens none.
	Console bool `toml:"console"`
	// Manifest is an application manifest to link instead of the one
	// ggui writes, as for a program that must run as an administrator.
	Manifest string `toml:"manifest"`
}

// Linux holds what only the Linux install tree and the packages made of
// it use. Its name, ID, version and icon are app's.
type Linux struct {
	// Formats are what ggui build --target linux makes of the tree:
	// "tar", "deb" and "rpm".
	Formats []string `toml:"formats"`
	// Description is one line on what the app does: the desktop entry's
	// Comment, the AppStream summary and the packages' description.
	// Empty is app.name.
	Description string `toml:"description"`
	// GenericName is what kind of program it is, such as "Image Viewer".
	GenericName string `toml:"generic_name"`
	// Categories are the desktop menu categories, from the freedesktop.org
	// menu specification's registered ones.
	Categories []string `toml:"categories"`
	// MIMETypes are the types the app opens; with any, it is given the
	// files or URLs it is opened with.
	MIMETypes []string `toml:"mime_types"`
	// Maintainer is "Name <email>", which a Debian package must name.
	Maintainer string `toml:"maintainer"`
	Homepage   string `toml:"homepage"`
	// License is an SPDX license expression, such as MIT.
	License string `toml:"license"`
	// DependsDeb and DependsRPM are the packages' dependencies, in each
	// format's syntax, replacing the libraries ggui's own need.
	DependsDeb []string `toml:"depends_deb"`
	DependsRPM []string `toml:"depends_rpm"`
}

// Default Linux package dependencies: the libraries ggwd and ggfx open at
// run time, which a package manager cannot see, as each family names
// their packages. libgl1 and libglvnd-glx hold libGL and GLX, libegl1 and
// libglvnd-egl EGL, and each requires Mesa's drivers.
var (
	DefaultDependsDeb = []string{
		"libx11-6", "libxrandr2", "libxi6", "libxcursor1", "libxinerama1", "libxext6", "libxrender1",
		"libgl1", "libegl1",
	}
	DefaultDependsRPM = []string{
		"libX11", "libXrandr", "libXi", "libXcursor", "libXinerama", "libXext", "libXrender",
		"libglvnd-glx", "libglvnd-egl",
	}
)

// Web holds what only the js/wasm bundle and its page use.
type Web struct {
	Title   string `toml:"title"`
	Base    string `toml:"base"`
	WasmOpt string `toml:"wasm_opt"`
}

// Default dev tags. The inspector is the dev loop's reason to exist, as the
// repository's own run and serve tasks have it; ggui_debug turns a Set from
// the wrong goroutine into a panic at the write, and names where each
// effect in a cycle was made, which is what a program being written needs
// and a release must not pay for.
var defaultDevTags = []string{"ggui_inspector", "ggui_debug"}

// Defaults returns the configuration a project without a file gets.
func Defaults() Config {
	return Config{
		App:   App{Version: "0.1.0", Package: "."},
		Build: Build{Output: "build"},
		Dev:   Dev{Tags: slices.Clone(defaultDevTags)},
		MacOS: MacOS{
			// What the Go 1.27 linker records as the binary's minimum,
			// which macOS enforces whatever Info.plist says; a lower
			// value would only let Finder offer an app that cannot start.
			MinimumSystemVersion: "13.0",
			SigningIdentity:      "-",
			P12Base64Env:         "GGUI_P12_BASE64",
			P12PasswordEnv:       "GGUI_P12_PASSWORD",
			Notary: Notary{
				PasswordEnv: "GGUI_NOTARY_PASSWORD",
				Staple:      true,
				// zapp's default: a submission usually takes minutes,
				// but Apple's queue has taken hours.
				Timeout: "1h",
			},
			DMG: DMG{
				Window:    Window{Width: DefaultDMGWidth, Height: DefaultDMGHeight},
				IconSize:  DefaultDMGIconSize,
				LabelSize: DefaultDMGLabelSize,
				FS:        "hfsplus",
				Format:    "udzo",
			},
			PKG: PKG{InstallLocation: "/Applications"},
		},
		Linux: Linux{
			Formats:    []string{"tar"},
			Categories: []string{"Utility"},
			DependsDeb: slices.Clone(DefaultDependsDeb),
			DependsRPM: slices.Clone(DefaultDependsRPM),
		},
		Web: Web{Base: "/", WasmOpt: "-O2"},
		Release: Release{
			Targets:    slices.Clone(ReleaseTargets),
			Arches:     []string{"amd64", "arm64"},
			GitHubAPI:  "https://api.github.com",
			TagPrefix:  "v",
			Publish:    []string{"github"},
			Chocolatey: Chocolatey{Source: "https://push.chocolatey.org/"},
		},
	}
}

// Error is a problem with the file, at a line when one can be named.
type Error struct {
	File string
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
	}
	return e.File + ": " + e.Msg
}

// Errors is every problem found in one file, so that fixing them is one
// round rather than one per run.
type Errors []*Error

func (es Errors) Error() string {
	msgs := make([]string, len(es))
	for i, e := range es {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "\n")
}

// Find looks for the file in dir and then its parents, stopping at the
// first directory that holds go.mod or go.work: a project's file sits at
// or below its module root, and one above it belongs to someone else. It
// returns "" with no error when there is none.
func Find(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, FileName)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if exists(filepath.Join(dir, "go.mod")) || exists(filepath.Join(dir, "go.work")) {
			return "", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Load reads the file at path over the defaults.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return Parse(path, data)
}

// Parse reads data, named name in errors, over the defaults. A key the
// file sets replaces the default whole, so tags = [] means no tags rather
// than the default ones.
func Parse(name string, data []byte) (Config, error) {
	cfg := Defaults()
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&cfg)
	if err != nil {
		if pe, ok := errors.AsType[toml.ParseError](err); ok {
			return Config{}, &Error{File: name, Line: pe.Position.Line, Msg: pe.Message}
		}
		// A value of the wrong type comes as text, naming the key.
		if m := typeErr.FindStringSubmatch(err.Error()); m != nil {
			line, _ := strconv.Atoi(m[1])
			if line == 0 {
				line = keyLine(data, strings.Split(m[2], "."))
			}
			msg := strings.Replace(m[3], "incompatible types: TOML value has type", "has type", 1)
			return Config{}, &Error{File: name, Line: line, Msg: m[2] + " " + msg}
		}
		return Config{}, &Error{File: name, Msg: strings.TrimPrefix(err.Error(), "toml: ")}
	}

	// A table asks for what it describes even when it sets nothing.
	cfg.MacOS.DMG.Make = md.IsDefined("macos", "dmg")
	cfg.MacOS.PKG.Make = md.IsDefined("macos", "pkg")

	var errs Errors
	for _, k := range undecoded(md) {
		errs = append(errs, &Error{File: name, Line: keyLine(data, k), Msg: unknownKey(k)})
	}
	for _, p := range cfg.validate() {
		errs = append(errs, &Error{File: name, Line: keyLine(data, p.key), Msg: p.msg})
	}
	if len(errs) > 0 {
		sort.SliceStable(errs, func(i, j int) bool { return errs[i].Line < errs[j].Line })
		return Config{}, errs
	}
	return cfg, nil
}

// typeErr matches the decoder's error for a value of the wrong type.
var typeErr = regexp.MustCompile(`^toml: (?:line (\d+) )?\(last key "([^"]*)"\): (.*)$`)

// undecoded returns the keys the schema has no field for, leaving out
// those below a table that is unknown itself: [foo] with three keys is one
// mistake, not four.
func undecoded(md toml.MetaData) []toml.Key {
	var out []toml.Key
	seen := map[string]bool{}
	for _, k := range md.Undecoded() {
		if inMap(k) {
			continue
		}
		parentUnknown := false
		for i := 1; i < len(k); i++ {
			if seen[toml.Key(k[:i]).String()] {
				parentUnknown = true
				break
			}
		}
		seen[k.String()] = true
		if !parentUnknown {
			out = append(out, k)
		}
	}
	return out
}

// inMap reports whether k lies below a field that is a map, such as
// macos.plist, which takes any key: the decoder reports the keys of a
// table inside an array inside one as undecoded, though the map holds
// them.
func inMap(k toml.Key) bool {
	t := reflect.TypeFor[Config]()
	for _, name := range k {
		if t.Kind() == reflect.Map {
			return true
		}
		if t.Kind() != reflect.Struct {
			return false
		}
		f, ok := fieldByTag(t, name)
		if !ok {
			return false
		}
		t = f.Type
	}
	return false
}

// unknownKey says a key is unknown and, when one is close, which known key
// was probably meant.
func unknownKey(k toml.Key) string {
	msg := fmt.Sprintf("unknown key %q", k.String())
	known := knownKeys(k[:len(k)-1])
	if len(known) == 0 {
		return msg
	}
	if best := closest(k[len(k)-1], known); best != "" {
		return msg + fmt.Sprintf(" (did you mean %q?)", strings.Join(append(slices.Clone(k[:len(k)-1]), best), "."))
	}
	if len(k) == 1 {
		return msg + "; the tables are [" + strings.Join(known, "], [") + "]"
	}
	return msg + "; [" + strings.Join(k[:len(k)-1], ".") + "] takes " + strings.Join(known, ", ")
}

// knownKeys lists the keys the schema has below table, from the struct
// tags, so the suggestion can never fall out of step with the schema.
func knownKeys(table []string) []string {
	t := reflect.TypeFor[Config]()
	for _, name := range table {
		f, ok := fieldByTag(t, name)
		if !ok || f.Type.Kind() != reflect.Struct {
			return nil
		}
		t = f.Type
	}
	var keys []string
	for field := range t.Fields() {
		if tag := field.Tag.Get("toml"); tag != "-" {
			keys = append(keys, tag)
		}
	}
	return keys
}

func fieldByTag(t reflect.Type, tag string) (reflect.StructField, bool) {
	for field := range t.Fields() {
		if field.Tag.Get("toml") == tag {
			return field, true
		}
	}
	return reflect.StructField{}, false
}

// closest returns the candidate within two edits of s, or "".
func closest(s string, candidates []string) string {
	best, bestD := "", 3
	for _, c := range candidates {
		if d := distance(strings.ToLower(s), c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

// distance is the Levenshtein distance between a and b.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
