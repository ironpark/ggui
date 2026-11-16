package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseEmptyIsDefaults(t *testing.T) {
	cfg, err := Parse("ggui.toml", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, Defaults()) {
		t.Fatalf("empty file = %+v, want the defaults %+v", cfg, Defaults())
	}
}

func TestParseFull(t *testing.T) {
	src := `
[app]
name = "Hello World"
id = "com.example.hello"
version = "1.2.3"
icon = "assets/icon.png"
package = "./cmd/hello"

[build]
tags = ["prod"]
ldflags = "-X main.version=1.2.3"
output = "dist"

[dev]
tags = []
args = ["--verbose"]
env = { LOG_LEVEL = "debug" }
watch = ["assets"]

[macos]
minimum_system_version = "14.0"
category = "public.app-category.productivity"
signing_identity = "Developer ID Application: Example (ABCDE12345)"
entitlements = "app.entitlements"
universal = true

[macos.plist]
NSCameraUsageDescription = "Scans documents."
LSUIElement = true
CFBundleURLTypes = [{ CFBundleURLSchemes = ["hello"] }]

[web]
title = "Hello"
base = "/app/"
wasm_opt = "-Oz"
`
	cfg, err := Parse("ggui.toml", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.Name != "Hello World" || cfg.App.Package != "./cmd/hello" || cfg.Build.Output != "dist" {
		t.Errorf("app/build = %+v %+v", cfg.App, cfg.Build)
	}
	// tags = [] replaces the default rather than leaving it.
	if cfg.Dev.Tags == nil || len(cfg.Dev.Tags) != 0 {
		t.Errorf("dev.tags = %#v, want empty", cfg.Dev.Tags)
	}
	if cfg.Dev.Env["LOG_LEVEL"] != "debug" || !cfg.MacOS.Universal || cfg.Web.Base != "/app/" {
		t.Errorf("dev/macos/web = %+v %+v %+v", cfg.Dev, cfg.MacOS, cfg.Web)
	}
	if cfg.MacOS.Plist["LSUIElement"] != true {
		t.Errorf("plist = %#v", cfg.MacOS.Plist)
	}
}

func TestDefaultsKeptForUnsetKeys(t *testing.T) {
	cfg, err := Parse("ggui.toml", []byte("[app]\nname = \"x\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Dev.Tags; !reflect.DeepEqual(got, defaultDevTags) {
		t.Errorf("dev.tags = %v, want the default %v", got, defaultDevTags)
	}
	if cfg.MacOS.SigningIdentity != "-" || cfg.Web.WasmOpt != "-O2" {
		t.Errorf("defaults lost: %+v %+v", cfg.MacOS, cfg.Web)
	}
}

// errorsOf returns the problems Parse reported.
func errorsOf(t *testing.T, src string) Errors {
	t.Helper()
	_, err := Parse("ggui.toml", []byte(src))
	if err == nil {
		t.Fatalf("no error for:\n%s", src)
	}
	var es Errors
	if errors.As(err, &es) {
		return es
	}
	var e *Error
	if errors.As(err, &e) {
		return Errors{e}
	}
	t.Fatalf("error %T %v is not a config error", err, err)
	return nil
}

func TestUnknownKeys(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		line      int
		want      string
	}{
		{"misspelt key", "[app]\nname = \"a\"\nnmae = \"b\"\n", 3, `unknown key "app.nmae" (did you mean "app.name"?)`},
		{"key in the wrong table", "[app]\nwasm_opt = \"-O2\"\n", 2, `unknown key "app.wasm_opt"; [app] takes name, id, version, icon, package`},
		{"unknown table, reported once", "[app]\n\n[mac]\nfoo = 1\nbar = 2\n", 3, `unknown key "mac" (did you mean "macos"?)`},
		{"unknown top-level key", "titel = \"x\"\n", 1, `unknown key "titel"; the tables are [app], [build], [dev], [macos], [web]`},
		{"dotted key", "web.titel = \"x\"\n", 1, `unknown key "web.titel" (did you mean "web.title"?)`},
		{"quoted key", "[dev]\n\"tag\" = [\"a\"]\n", 2, `unknown key "dev.tag" (did you mean "dev.tags"?)`},
		{"after a multi-line array", "[dev]\nargs = [\n  \"a=b\",\n]\nwach = []\n", 5, `unknown key "dev.wach"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			es := errorsOf(t, tc.src)
			if len(es) != 1 {
				t.Fatalf("got %d errors, want 1: %v", len(es), es)
			}
			if es[0].Line != tc.line {
				t.Errorf("line = %d, want %d (%v)", es[0].Line, tc.line, es[0])
			}
			if !strings.HasPrefix(es[0].Msg, tc.want) {
				t.Errorf("message = %q, want it to start with %q", es[0].Msg, tc.want)
			}
		})
	}
}

func TestSyntaxAndTypeErrorsHaveLines(t *testing.T) {
	es := errorsOf(t, "[app]\nname = \"a\"\nversion = 1\n")
	if es[0].Line != 3 {
		t.Errorf("type error at line %d, want 3: %v", es[0].Line, es[0])
	}
	es = errorsOf(t, "[app]\nname = \"a\n")
	if es[0].Line != 2 {
		t.Errorf("syntax error at line %d, want 2: %v", es[0].Line, es[0])
	}
	if !strings.HasPrefix(es[0].Error(), "ggui.toml:2: ") {
		t.Errorf("error %q does not lead with file:line", es[0].Error())
	}
}

func TestValidation(t *testing.T) {
	src := `[app]
id = "not an id"
version = "1.0-beta"
icon = "icon.jpg"

[dev]
tags = ["a b"]

[macos]
category = "games"

[web]
base = "app"
wasm_opt = "O2"
`
	es := errorsOf(t, src)
	want := map[int]string{
		2:  "app.id",
		3:  "app.version",
		4:  "app.icon",
		7:  "dev.tags",
		10: "macos.category",
		13: "web.base",
		14: "web.wasm_opt",
	}
	if len(es) != len(want) {
		t.Fatalf("got %d errors, want %d:\n%v", len(es), len(want), es)
	}
	for _, e := range es {
		if !strings.HasPrefix(e.Msg, want[e.Line]) {
			t.Errorf("line %d: %q, want a message about %s", e.Line, e.Msg, want[e.Line])
		}
	}
}

// The file init writes must parse, to the values it was given and the
// defaults otherwise, and must mention every key of the schema.
func TestTemplate(t *testing.T) {
	data := Template(`Say "Hi"`, "com.example.hi", "0.1.0")
	cfg, err := Parse("ggui.toml", data)
	if err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	want := Defaults()
	want.App.Name, want.App.ID = `Say "Hi"`, "com.example.hi"
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("template parses to %+v, want %+v", cfg, want)
	}
	for _, table := range []string{"app", "build", "dev", "macos", "web"} {
		for _, key := range knownKeys([]string{table}) {
			if !strings.Contains(string(data), "\n"+key+" =") && !strings.Contains(string(data), "\n# "+key+" =") &&
				!strings.Contains(string(data), "[macos."+key+"]") {
				t.Errorf("template does not document %s.%s", table, key)
			}
		}
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "cmd", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644)
	if got, err := Find(sub); err != nil || got != "" {
		t.Fatalf("Find without a file = %q, %v", got, err)
	}
	file := filepath.Join(root, FileName)
	os.WriteFile(file, nil, 0o644)
	if got, _ := Find(sub); got != file {
		t.Errorf("Find from below = %q, want %q", got, file)
	}
	// The search stops at the module root: a file above it is another
	// project's.
	inner := filepath.Join(root, "inner")
	os.MkdirAll(inner, 0o755)
	os.WriteFile(filepath.Join(inner, "go.mod"), []byte("module y\n"), 0o644)
	if got, _ := Find(inner); got != "" {
		t.Errorf("Find crossed a module root: %q", got)
	}
}
