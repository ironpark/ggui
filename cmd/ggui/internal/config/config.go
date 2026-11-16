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
}

// MacOS holds what only the .app bundle uses.
type MacOS struct {
	MinimumSystemVersion string         `toml:"minimum_system_version"`
	Category             string         `toml:"category"`
	SigningIdentity      string         `toml:"signing_identity"`
	Entitlements         string         `toml:"entitlements"`
	Universal            bool           `toml:"universal"`
	Plist                map[string]any `toml:"plist"`
}

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
		var pe toml.ParseError
		if errors.As(err, &pe) {
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
	t := reflect.TypeOf(Config{})
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
	t := reflect.TypeOf(Config{})
	for _, name := range table {
		f, ok := fieldByTag(t, name)
		if !ok || f.Type.Kind() != reflect.Struct {
			return nil
		}
		t = f.Type
	}
	var keys []string
	for i := range t.NumField() {
		keys = append(keys, t.Field(i).Tag.Get("toml"))
	}
	return keys
}

func fieldByTag(t reflect.Type, tag string) (reflect.StructField, bool) {
	for i := range t.NumField() {
		if t.Field(i).Tag.Get("toml") == tag {
			return t.Field(i), true
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
	for k, v := range c.MacOS.Plist {
		if err := plistValue(v); err != nil {
			add(fmt.Sprintf("macos.plist.%s: %v", k, err), "macos", "plist", k)
		}
	}
	if !strings.HasPrefix(c.Web.Base, "/") {
		add(fmt.Sprintf("web.base %q must be an absolute path such as / or /app/", c.Web.Base), "web", "base")
	}
	for _, f := range strings.Fields(c.Web.WasmOpt) {
		if !strings.HasPrefix(f, "-") {
			add(fmt.Sprintf("web.wasm_opt %q must be wasm-opt flags such as -O2 or -Oz, or empty to skip it", c.Web.WasmOpt), "web", "wasm_opt")
			break
		}
	}
	return ps
}

// plistValue reports whether v can go in an Info.plist. Every TOML value
// can but one: a table inside an array of tables is fine, but a value
// mixing kinds is still a single plist type per element, so all that is
// refused is what TOML itself cannot hold anyway.
func plistValue(v any) error {
	switch v := v.(type) {
	case string, bool, int64, float64:
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
