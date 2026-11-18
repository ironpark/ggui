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

	"github.com/BurntSushi/toml"
	"github.com/ironpark/zapp/pkg/dmg"
	"github.com/ironpark/zapp/pkg/udif"
)

// FileName is the name the file is looked for under.
const FileName = "ggui.toml"

// Config is the whole file. Paths in it are relative to the directory the
// file is in, which Project resolves.
type Config struct {
	App   App   `toml:"app"`
	Build Build `toml:"build"`
	Dev   Dev   `toml:"dev"`
	MacOS MacOS `toml:"macos"`
	Web   Web   `toml:"web"`
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
	// Icon is a square PNG, ideally 1024×1024, made into the bundle icon.
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
		Web: Web{Base: "/", WasmOpt: "-O2"},
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

// validate checks the values the decoder accepted as types but that the
// build would trip over later, further from where they were written.
func (c *Config) validate() []problem {
	var ps []problem
	add := func(msg string, key ...string) { ps = append(ps, problem{toml.Key(key), msg}) }

	if c.App.ID != "" && !bundleID.MatchString(c.App.ID) {
		add(fmt.Sprintf("app.id %q is not a bundle identifier: use reverse-DNS letters, digits, hyphens and dots, such as com.example.hello", c.App.ID), "app", "id")
	}
	if !versionRE.MatchString(c.App.Version) {
		add(fmt.Sprintf("app.version %q must be one to three numbers separated by dots, such as 1.2.0: macOS reads it as CFBundleShortVersionString", c.App.Version), "app", "version")
	}
	if c.App.Icon != "" && !strings.EqualFold(filepath.Ext(c.App.Icon), ".png") {
		add(fmt.Sprintf("app.icon %q must be a PNG file", c.App.Icon), "app", "icon")
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
	if !strings.HasPrefix(c.Web.Base, "/") {
		add(fmt.Sprintf("web.base %q must be an absolute path such as / or /app/", c.Web.Base), "web", "base")
	}
	for f := range strings.FieldsSeq(c.Web.WasmOpt) {
		if !strings.HasPrefix(f, "-") {
			add(fmt.Sprintf("web.wasm_opt %q must be wasm-opt flags such as -O2 or -Oz, or empty to skip it", c.Web.WasmOpt), "web", "wasm_opt")
			break
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
