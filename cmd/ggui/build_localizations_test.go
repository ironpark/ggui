package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/macos"
)

// A macOS bundle names the languages of the project's catalogs in
// CFBundleLocalizations, en first, unless [macos.plist] names them.
func TestPlistLocalizations(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/app\n",
		"main.go": `package main

import "github.com/ironpark/ggui/i18n"

var ko = i18n.Catalog{Lang: "ko"}

func main() { _ = i18n.T("Hello") }
`,
		"locales/ja.json": `{"lang": "ja"}`,
		"locales/en.json": `{"lang": "en"}`,
	}
	for name, src := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	given := map[string]any{"LSUIElement": true}
	extra, langs := plistExtra(dir, given)
	if !slices.Equal(langs, []string{"en", "ja", "ko"}) {
		t.Errorf("langs = %q", langs)
	}
	if _, ok := given["CFBundleLocalizations"]; ok {
		t.Error("the configured keys were changed in place")
	}
	if extra["LSUIElement"] != true {
		t.Error("lost a configured key")
	}

	bin := filepath.Join(dir, "prog")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	app, _, err := macos.Assemble(macos.Bundle{
		Name: "App", Executable: "app", ID: "com.example.app", Version: "1.0", Extra: extra,
		Binary: bin, OutDir: filepath.Join(dir, "build"),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	want := "<key>CFBundleLocalizations</key>\n\t<array>\n\t\t<string>en</string>\n\t\t<string>ja</string>\n\t\t<string>ko</string>\n\t</array>"
	if !strings.Contains(string(data), want) {
		t.Errorf("Info.plist lacks %q:\n%s", want, data)
	}

	// Set by hand, it wins.
	mine := map[string]any{"CFBundleLocalizations": []any{"en", "de"}}
	if extra, langs := plistExtra(dir, mine); langs != nil || len(extra["CFBundleLocalizations"].([]any)) != 2 {
		t.Errorf("an explicit value was replaced: %v", extra)
	}
	// No catalogs, no key.
	if extra, langs := plistExtra(t.TempDir(), nil); langs != nil || extra != nil {
		t.Errorf("without catalogs: %v, %v", extra, langs)
	}
}
