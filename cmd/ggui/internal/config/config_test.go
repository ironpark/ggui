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

[windows]
description = "Says hello"
company = "Example Ltd"
copyright = "© 2026 Example Ltd"
console = true
manifest = "app.manifest"

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
	want := Windows{Description: "Says hello", Company: "Example Ltd", Copyright: "© 2026 Example Ltd", Console: true, Manifest: "app.manifest"}
	if cfg.Windows != want {
		t.Errorf("windows = %+v, want %+v", cfg.Windows, want)
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
// An SVG icon is refused with what to do instead, since ggui carries no
// SVG renderer.
func TestSVGIcon(t *testing.T) {
	es := errorsOf(t, "[app]\nicon = \"art/icon.svg\"\n")
	if len(es) != 1 || es[0].Line != 2 || !strings.Contains(es[0].Msg, "export it at 1024×1024") || !strings.Contains(es[0].Msg, "-o icon.png") {
		t.Errorf("errors = %v", es)
	}
	cfg, err := Parse("ggui.toml", []byte("[macos]\nicon_inset = true\n"))
	if err != nil || !cfg.MacOS.IconInset || Defaults().MacOS.IconInset {
		t.Errorf("icon_inset: %v, %v", cfg.MacOS.IconInset, err)
	}
}

func errorsOf(t *testing.T, src string) Errors {
	t.Helper()
	_, err := Parse("ggui.toml", []byte(src))
	if err == nil {
		t.Fatalf("no error for:\n%s", src)
	}
	if es, ok := errors.AsType[Errors](err); ok {
		return es
	}
	if e, ok := errors.AsType[*Error](err); ok {
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
		{"unknown top-level key", "titel = \"x\"\n", 1, `unknown key "titel"; the tables are [app], [build], [dev], [macos], [windows], [web]`},
		{"misspelt Windows key", "[windows]\nconsle = true\n", 2, `unknown key "windows.consle" (did you mean "windows.console"?)`},
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
display = "viewer"

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
		8:  "dev.display",
		11: "macos.category",
		14: "web.base",
		15: "web.wasm_opt",
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

func TestMacOSDistribution(t *testing.T) {
	cfg, err := Parse("ggui.toml", []byte(`[macos]
signing_identity = "Developer ID Application"
notarize = true
p12_base64_env = "CI_P12"

[macos.notary]
profile = "ggui"

[macos.dmg]

[macos.dmg.window]
width = 700
`))
	if err != nil {
		t.Fatal(err)
	}
	m := cfg.MacOS
	if !m.Notarize || m.Notary.Profile != "ggui" || !m.Notary.Staple || m.Notary.Timeout != "1h" {
		t.Errorf("notarization = %v %+v", m.Notarize, m.Notary)
	}
	if m.P12Base64Env != "CI_P12" || m.P12PasswordEnv != "GGUI_P12_PASSWORD" {
		t.Errorf("certificate variables = %q, %q", m.P12Base64Env, m.P12PasswordEnv)
	}
	// An empty table asks for a disk image; the window keeps the default
	// height when only its width is set.
	if !m.DMG.Make || m.DMG.Window != (Window{700, DefaultDMGHeight}) || m.DMG.IconSize != DefaultDMGIconSize {
		t.Errorf("dmg = %+v", m.DMG)
	}
	if m.PKG.Make {
		t.Error("an installer was asked for without [macos.pkg]")
	}
	if d := Defaults().MacOS; d.DMG.Make || d.PKG.Make || d.Notarize {
		t.Error("the defaults ask for a disk image, an installer or notarization")
	}
}

func TestMacOSValidation(t *testing.T) {
	src := `[macos]
p12_password_env = "MY PASSWORD"

[macos.notary]
apple_id = "me@example.com"
timeout = "soon"

[macos.dmg]
background = "bg.gif"
icon_size = 8
app_position = [10]
fs = "fat32"
format = "zip"

[macos.pkg]
install_location = "Applications"
`
	es := errorsOf(t, src)
	want := map[int]string{
		2:  "macos.p12_password_env",
		5:  "macos.notary.apple_id",
		6:  "macos.notary.timeout",
		9:  "macos.dmg.background",
		10: "macos.dmg.icon_size",
		11: "macos.dmg.app_position",
		12: "macos.dmg.fs",
		13: "macos.dmg.format",
		16: "macos.pkg.install_location",
	}
	if len(es) != len(want) {
		t.Fatalf("got %d errors, want %d:\n%v", len(es), len(want), es)
	}
	for _, e := range es {
		if !strings.HasPrefix(e.Msg, want[e.Line]) {
			t.Errorf("line %d: %q, want a message about %s", e.Line, e.Msg, want[e.Line])
		}
	}

	es = errorsOf(t, "[macos.notary]\napple_id = \"me@example.com\"\nteam_id = \"abc\"\n")
	if len(es) != 1 || es[0].Line != 3 || !strings.HasPrefix(es[0].Msg, "macos.notary.team_id") {
		t.Errorf("bad team ID: %v", es)
	}
	es = errorsOf(t, "[macos.dmg]\nbackgrund = \"bg.png\"\n")
	if len(es) != 1 || es[0].Line != 2 || !strings.Contains(es[0].Msg, `did you mean "macos.dmg.background"`) {
		t.Errorf("misspelt dmg key: %v", es)
	}
	es = errorsOf(t, "[macos.plist]\nRatio = nan\n")
	if len(es) != 1 || es[0].Line != 2 {
		t.Errorf("NaN in Info.plist: %v", es)
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
	for _, table := range []string{"app", "build", "dev", "macos", "macos.notary", "macos.dmg", "macos.pkg", "windows", "web"} {
		for _, key := range knownKeys(strings.Split(table, ".")) {
			if !strings.Contains(string(data), "\n"+key+" =") && !strings.Contains(string(data), "\n# "+key+" =") &&
				!strings.Contains(string(data), "["+table+"."+key+"]") {
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
