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
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/ironpark/ggui/cmd/ggui/internal/i18n"
	"github.com/ironpark/zapp/pkg/dmg"
	"github.com/ironpark/zapp/pkg/udif"
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

type problem struct {
	key toml.Key
	msg string
}

var (
	bundleID   = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
	versionRE  = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)
	macVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)
	tagRE      = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)
)

func validAppName(s string) bool {
	if s == "." || strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// validate checks the values the decoder accepted as types but that the
// build would trip over later, further from where they were written.
func (c *Config) validate() []problem {
	var ps []problem
	add := func(msg string, key ...string) { ps = append(ps, problem{toml.Key(key), msg}) }

	if c.App.Name != "" && !validAppName(c.App.Name) {
		add(fmt.Sprintf("app.name %q must not contain path separators, \"..\", or control characters", c.App.Name), "app", "name")
	}
	if c.App.ID != "" && !bundleID.MatchString(c.App.ID) {
		add(fmt.Sprintf("app.id %q is not a bundle identifier: use reverse-DNS letters, digits, hyphens and dots, such as com.example.hello", c.App.ID), "app", "id")
	}
	if !versionRE.MatchString(c.App.Version) {
		add(fmt.Sprintf("app.version %q must be one to three numbers separated by dots, such as 1.2.0: macOS reads it as CFBundleShortVersionString", c.App.Version), "app", "version")
	}
	switch ext := strings.ToLower(filepath.Ext(c.App.Icon)); {
	case c.App.Icon == "" || ext == ".png" || ext == ".svg":
		// An SVG is drawn at build time by the project's own ggfx.
	case ext == ".svgz":
		base := filepath.Base(c.App.Icon)
		add(fmt.Sprintf("app.icon %q is a compressed SVG; name the SVG it holds, as gunzip -c %s > %s writes it",
			c.App.Icon, base, strings.TrimSuffix(base, filepath.Ext(base))+".svg"), "app", "icon")
	default:
		add(fmt.Sprintf("app.icon %q must be a PNG file, square, ideally 1024×1024, or an SVG with a square viewBox", c.App.Icon), "app", "icon")
	}
	if c.App.Package == "" {
		add(`app.package must name the main package's directory; "." is the directory of this file`, "app", "package")
	}
	if c.Build.Output == "" {
		add(`build.output must name a directory, such as "build"`, "build", "output")
	}
	for _, list := range []struct {
		tags []string
		key  string
	}{{c.Build.Tags, "build"}, {c.Dev.Tags, "dev"}} {
		for _, t := range list.tags {
			if !tagRE.MatchString(t) {
				add(fmt.Sprintf("%s.tags: %q is not a build tag; give each tag as its own string", list.key, t), list.key, "tags")
			}
		}
	}
	switch c.Dev.Display {
	case "", "remote", "window":
	default:
		add(fmt.Sprintf(`dev.display %q must be "remote" or "window"`, c.Dev.Display), "dev", "display")
	}
	for k := range c.Dev.Env {
		if k == "" || strings.ContainsAny(k, "= \t") {
			add(fmt.Sprintf("dev.env: %q is not an environment variable name", k), "dev", "env")
		}
	}
	if c.MacOS.MinimumSystemVersion != "" && !macVersion.MatchString(c.MacOS.MinimumSystemVersion) {
		add(fmt.Sprintf("macos.minimum_system_version %q must be a version such as 11.0", c.MacOS.MinimumSystemVersion), "macos", "minimum_system_version")
	}
	if c.MacOS.Category != "" && !strings.HasPrefix(c.MacOS.Category, "public.app-category.") {
		add(fmt.Sprintf("macos.category %q must be one of Apple's public.app-category.* types, such as public.app-category.productivity", c.MacOS.Category), "macos", "category")
	}
	if c.MacOS.SigningIdentity == "" {
		add(`macos.signing_identity must not be empty: "-" signs ad hoc`, "macos", "signing_identity")
	}
	ps = append(ps, c.MacOS.validateSigning()...)
	for k, v := range c.MacOS.Plist {
		if err := plistValue(v); err != nil {
			add(fmt.Sprintf("macos.plist.%s: %v", k, err), "macos", "plist", k)
		}
	}
	ps = append(ps, c.Linux.validate()...)
	ps = append(ps, c.Release.validate()...)
	if !strings.HasPrefix(c.Web.Base, "/") {
		add(fmt.Sprintf("web.base %q must be an absolute path such as / or /app/", c.Web.Base), "web", "base")
	}
	for f := range strings.FieldsSeq(c.Web.WasmOpt) {
		if !strings.HasPrefix(f, "-") {
			add(fmt.Sprintf("web.wasm_opt %q must be wasm-opt flags such as -O2 or -Oz, or empty to skip it", c.Web.WasmOpt), "web", "wasm_opt")
			break
		}
	}
	for i, key := range []string{"funcs", "plurals"} {
		list := c.I18n.Funcs
		if i == 1 {
			list = c.I18n.Plurals
		}
		for _, spec := range list {
			if _, err := i18n.ParseFunc(spec, i == 1); err != nil {
				add(fmt.Sprintf("i18n.%s: %v", key, err), "i18n", key)
			}
		}
	}
	return ps
}

var (
	envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	teamID  = regexp.MustCompile(`^[A-Z0-9]{10}$`)
)

// validateSigning checks the keys that sign, notarize and package.
// Whether there is a certificate or a password to use depends on the
// environment of the build, so that is checked there.
func (m *MacOS) validateSigning() []problem {
	var ps []problem
	add := func(msg string, key ...string) { ps = append(ps, problem{toml.Key(key), msg}) }

	for _, e := range []struct {
		value string
		key   []string
	}{
		{m.P12Base64Env, []string{"macos", "p12_base64_env"}},
		{m.P12PasswordEnv, []string{"macos", "p12_password_env"}},
		{m.Notary.PasswordEnv, []string{"macos", "notary", "password_env"}},
	} {
		if e.value != "" && !envName.MatchString(e.value) {
			add(fmt.Sprintf("%s %q must name an environment variable, such as GGUI_P12_PASSWORD; the secret itself never goes in this file",
				strings.Join(e.key, "."), e.value), e.key...)
		}
	}

	n := m.Notary
	if n.AppleID != "" && n.TeamID == "" {
		add("macos.notary.apple_id needs macos.notary.team_id, the ten-character ID of the team the app is signed for", "macos", "notary", "apple_id")
	}
	if n.TeamID != "" && !teamID.MatchString(n.TeamID) {
		add(fmt.Sprintf("macos.notary.team_id %q must be ten capital letters and digits, as in the signing identity's parentheses", n.TeamID), "macos", "notary", "team_id")
	}
	if d, err := time.ParseDuration(n.Timeout); err != nil || d <= 0 {
		add(fmt.Sprintf("macos.notary.timeout %q must be a duration such as 30m or 2h", n.Timeout), "macos", "notary", "timeout")
	}

	d := m.DMG
	if d.Icon != "" && !hasExt(d.Icon, ".png", ".icns") {
		add(fmt.Sprintf("macos.dmg.icon %q must be a PNG or an .icns file", d.Icon), "macos", "dmg", "icon")
	}
	if d.Background != "" && !hasExt(d.Background, ".png", ".jpg", ".jpeg", ".tif", ".tiff") {
		add(fmt.Sprintf("macos.dmg.background %q must be a PNG, JPEG or TIFF image", d.Background), "macos", "dmg", "background")
	}
	if d.Window.Width <= 0 || d.Window.Height <= 0 {
		add(fmt.Sprintf("macos.dmg.window is %d×%d; give a width and height in points, such as { width = 640, height = 480 }", d.Window.Width, d.Window.Height), "macos", "dmg", "window")
	}
	if d.IconSize < dmg.MinIconSize || d.IconSize > dmg.MaxIconSize {
		add(fmt.Sprintf("macos.dmg.icon_size %d must be from %d to %d", d.IconSize, dmg.MinIconSize, dmg.MaxIconSize), "macos", "dmg", "icon_size")
	}
	if d.LabelSize < dmg.MinLabelSize || d.LabelSize > dmg.MaxLabelSize {
		add(fmt.Sprintf("macos.dmg.label_size %d must be from %d to %d", d.LabelSize, dmg.MinLabelSize, dmg.MaxLabelSize), "macos", "dmg", "label_size")
	}
	for _, p := range []struct {
		pos []int
		key string
	}{{d.AppPosition, "app_position"}, {d.ApplicationsPosition, "applications_position"}} {
		if p.pos != nil && (len(p.pos) != 2 || p.pos[0] < 0 || p.pos[1] < 0) {
			add(fmt.Sprintf("macos.dmg.%s must be [x, y], the icon's centre in points from the window's top left", p.key), "macos", "dmg", p.key)
		}
	}
	if _, err := dmg.ParseFileSystem(d.FS); err != nil {
		add(fmt.Sprintf("macos.dmg.fs %q must be one of %s", d.FS, strings.Join(dmg.FileSystemNames, ", ")), "macos", "dmg", "fs")
	}
	if _, err := udif.ParseFormat(d.Format); err != nil {
		add(fmt.Sprintf(`macos.dmg.format %q must be "udzo" (zlib, which every macOS reads) or "ulfo" (lzfse, smaller, macOS 10.11 and later)`, d.Format), "macos", "dmg", "format")
	}

	if !strings.HasPrefix(m.PKG.InstallLocation, "/") {
		add(fmt.Sprintf("macos.pkg.install_location %q must be an absolute path, such as /Applications", m.PKG.InstallLocation), "macos", "pkg", "install_location")
	}
	return ps
}

var (
	categoryRE   = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	mimeTypeRE   = regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]*/[A-Za-z0-9][A-Za-z0-9.+_-]*$`)
	maintainerRE = regexp.MustCompile(`^[^<>\n]*[^<>\s] <[^<>@\s]+@[^<>@\s]+>$`)
	licenseRE    = regexp.MustCompile(`^[A-Za-z0-9.+:() -]+$`)
)

// LinuxFormats are what [linux] formats may name.
var LinuxFormats = []string{"tar", "deb", "rpm"}

// validate checks [linux]: values that would make a desktop entry or a
// package that its readers refuse.
func (l *Linux) validate() []problem {
	var ps []problem
	add := func(msg string, key ...string) { ps = append(ps, problem{toml.Key(key), msg}) }

	for i, f := range l.Formats {
		switch {
		case !slices.Contains(LinuxFormats, f):
			add(fmt.Sprintf(`linux.formats: %q must be "tar", "deb" or "rpm"`, f), "linux", "formats")
		case slices.Index(l.Formats, f) != i:
			add(fmt.Sprintf("linux.formats names %q twice", f), "linux", "formats")
		}
	}
	for _, k := range []struct {
		value, key string
	}{{l.Description, "description"}, {l.GenericName, "generic_name"}} {
		if strings.ContainsAny(k.value, "\n\r") {
			add(fmt.Sprintf("linux.%s must be one line", k.key), "linux", k.key)
		}
	}
	for _, c := range l.Categories {
		if !categoryRE.MatchString(c) {
			add(fmt.Sprintf("linux.categories: %q is not a menu category; use the registered names, such as Utility, Graphics or Development, each as its own string", c), "linux", "categories")
		}
	}
	for _, m := range l.MIMETypes {
		if !mimeTypeRE.MatchString(m) {
			add(fmt.Sprintf("linux.mime_types: %q is not a MIME type such as image/png or x-scheme-handler/myapp", m), "linux", "mime_types")
		}
	}
	if l.Maintainer != "" && !maintainerRE.MatchString(l.Maintainer) {
		add(fmt.Sprintf(`linux.maintainer %q must be a name and an email address, such as "Jane Doe <jane@example.com>"`, l.Maintainer), "linux", "maintainer")
	}
	if l.Homepage != "" && !strings.HasPrefix(l.Homepage, "https://") && !strings.HasPrefix(l.Homepage, "http://") {
		add(fmt.Sprintf("linux.homepage %q must be an http or https URL", l.Homepage), "linux", "homepage")
	}
	if l.License != "" && !licenseRE.MatchString(l.License) {
		add(fmt.Sprintf("linux.license %q must be an SPDX license expression, such as MIT or Apache-2.0", l.License), "linux", "license")
	}
	for _, d := range []struct {
		list []string
		key  string
	}{{l.DependsDeb, "depends_deb"}, {l.DependsRPM, "depends_rpm"}} {
		for _, p := range d.list {
			if strings.TrimSpace(p) == "" || strings.ContainsAny(p, ",\n") {
				add(fmt.Sprintf("linux.%s: %q must name one dependency; give each as its own string", d.key, p), "linux", d.key)
			}
		}
	}
	return ps
}

var (
	// RepoRE matches a GitHub repository, owner/repo.
	RepoRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]+$`)
	// CaskTokenRE matches a Homebrew cask token.
	CaskTokenRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*(?:@[a-z0-9.-]+)?$`)
	// ChocoIDRE matches a Chocolatey package ID, which the community
	// repository wants lowercase.
	ChocoIDRE = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)
	// tagPrefixRE is what may go before the version in a git tag.
	tagPrefixRE = regexp.MustCompile(`^[A-Za-z0-9._/-]*$`)
)

// validate checks [release] and its tables.
func (r *Release) validate() []problem {
	var ps []problem
	add := func(msg string, key ...string) { ps = append(ps, problem{toml.Key(key), msg}) }
	list := func(values, allowed []string, key string) {
		if len(values) == 0 {
			add(fmt.Sprintf("release.%s must name at least one of %s", key, quoteList(allowed)), "release", key)
		}
		for i, v := range values {
			switch {
			case !slices.Contains(allowed, v):
				add(fmt.Sprintf("release.%s: %q must be one of %s", key, v, quoteList(allowed)), "release", key)
			case slices.Index(values, v) != i:
				add(fmt.Sprintf("release.%s names %q twice", key, v), "release", key)
			}
		}
	}
	list(r.Targets, ReleaseTargets, "targets")
	list(r.Arches, ReleaseArches, "arches")
	if r.GitHub != "" && !RepoRE.MatchString(r.GitHub) {
		add(fmt.Sprintf(`release.github %q must be a repository, "owner/repo"`, r.GitHub), "release", "github")
	}
	if !httpURL(r.GitHubAPI) {
		add(fmt.Sprintf("release.github_api %q must be an http or https URL, such as https://api.github.com or https://github.example.com/api/v3", r.GitHubAPI), "release", "github_api")
	}
	if !tagPrefixRE.MatchString(r.TagPrefix) || strings.Contains(r.TagPrefix, "..") || strings.HasPrefix(r.TagPrefix, "/") || strings.HasPrefix(r.TagPrefix, "-") {
		add(fmt.Sprintf(`release.tag_prefix %q cannot begin a git tag; use letters, digits, ".", "_", "-" and "/", such as "v"`, r.TagPrefix), "release", "tag_prefix")
	}
	for i, v := range r.Publish {
		switch {
		case !slices.Contains(ReleaseChannels, v):
			add(fmt.Sprintf("release.publish: %q must be one of %s", v, quoteList(ReleaseChannels)), "release", "publish")
		case slices.Index(r.Publish, v) != i:
			add(fmt.Sprintf("release.publish names %q twice", v), "release", "publish")
		}
	}
	if strings.ContainsAny(r.Notes, "\n\r") {
		add("release.notes must name one file", "release", "notes")
	}

	h := r.Homebrew
	if h.Tap != "" && !RepoRE.MatchString(h.Tap) {
		add(fmt.Sprintf(`release.homebrew.tap %q must be the tap's repository, "owner/homebrew-name"`, h.Tap), "release", "homebrew", "tap")
	}
	if h.Tap == "" && slices.Contains(r.Publish, "homebrew") {
		add(`release.publish names "homebrew", which needs release.homebrew.tap, "owner/homebrew-name"`, "release", "publish")
	}
	if h.Cask != "" && !CaskTokenRE.MatchString(h.Cask) {
		add(fmt.Sprintf("release.homebrew.cask %q is not a cask token: lowercase letters, digits and single hyphens, such as my-app", h.Cask), "release", "homebrew", "cask")
	}
	if h.Homepage != "" && !httpURL(h.Homepage) {
		add(fmt.Sprintf("release.homebrew.homepage %q must be an http or https URL", h.Homepage), "release", "homebrew", "homepage")
	}
	if strings.ContainsAny(h.Desc, "\n\r") {
		add("release.homebrew.desc must be one line", "release", "homebrew", "desc")
	}

	c := r.Chocolatey
	if c.ID != "" && !ChocoIDRE.MatchString(c.ID) {
		add(fmt.Sprintf("release.chocolatey.id %q is not a package ID: lowercase letters and digits, joined by single dots or hyphens, such as my-app", c.ID), "release", "chocolatey", "id")
	}
	for _, u := range []struct{ value, key string }{
		{c.ProjectURL, "project_url"}, {c.LicenseURL, "license_url"}, {c.IconURL, "icon_url"},
	} {
		if u.value != "" && !httpURL(u.value) {
			add(fmt.Sprintf("release.chocolatey.%s %q must be an http or https URL", u.key, u.value), "release", "chocolatey", u.key)
		}
	}
	if !httpURL(c.Source) {
		add(fmt.Sprintf("release.chocolatey.source %q must be an http or https URL, such as https://push.chocolatey.org/", c.Source), "release", "chocolatey", "source")
	}
	for _, t := range c.Tags {
		if t == "" || strings.ContainsAny(t, " \t,\n") {
			add(fmt.Sprintf("release.chocolatey.tags: %q must be one tag without spaces or commas; give each as its own string", t), "release", "chocolatey", "tags")
		}
	}
	if strings.ContainsAny(c.Summary, "\n\r") {
		add("release.chocolatey.summary must be one line", "release", "chocolatey", "summary")
	}
	return ps
}

func httpURL(s string) bool {
	return strings.HasPrefix(s, "https://") && len(s) > len("https://") ||
		strings.HasPrefix(s, "http://") && len(s) > len("http://")
}

// quoteList writes a, b and c as "a", "b" or "c".
func quoteList(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = strconv.Quote(v)
	}
	if len(q) == 1 {
		return q[0]
	}
	return strings.Join(q[:len(q)-1], ", ") + " or " + q[len(q)-1]
}

func hasExt(path string, exts ...string) bool {
	return slices.Contains(exts, strings.ToLower(filepath.Ext(path)))
}

// plistValue reports whether v can go in an Info.plist. Every TOML value
// can but dates and the floats a plist has no form for: a table inside an
// array of tables is fine, and a value mixing kinds is still a single
// plist type per element.
func plistValue(v any) error {
	switch v := v.(type) {
	case string, bool, int64:
		return nil
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return fmt.Errorf("%v has no Info.plist form", v)
		}
		return nil
	case []any:
		for _, e := range v {
			if err := plistValue(e); err != nil {
				return err
			}
		}
		return nil
	case []map[string]any:
		for _, e := range v {
			if err := plistValue(e); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for _, e := range v {
			if err := plistValue(e); err != nil {
				return err
			}
		}
		return nil
	default:
		// TOML's dates and times; Info.plist keys never take them.
		return fmt.Errorf("a %T cannot be written to Info.plist", v)
	}
}
