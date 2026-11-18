package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/i18n"
)

func i18nProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/app\n",
		"main.go": `package main

import "github.com/ironpark/ggui/i18n"

var ko = i18n.Catalog{Lang: "ko", Messages: map[string]string{"Hello": "안녕"}}

func main() {
	_ = i18n.T("Hello")
	_ = i18n.T("Open").Context("verb")
	_ = i18n.N(2, "%d file", "%d files", 2)
}
`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestI18nCheckAndExtract(t *testing.T) {
	dir := i18nProject(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"i18n", "check", dir}, &out, &errOut); code != 1 {
		t.Errorf("check with missing translations exited %d, want 1: %s", code, errOut.String())
	}
	if code := run([]string{"i18n", "check", dir, "--allow-missing"}, &out, &errOut); code != 0 {
		t.Errorf("check --allow-missing exited %d, want 0: %s", code, errOut.String())
	}

	ja := filepath.Join(dir, "locales", "ja.json")
	if code := run([]string{"i18n", "extract", dir, "--lang", "ja"}, &out, &errOut); code != 0 {
		t.Fatalf("extract --lang exited %d: %s", code, errOut.String())
	}
	c, err := i18n.ReadJSON(ja)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Messages) != 2 || c.Messages["Open [verb]"] != "" || len(c.Plurals) != 1 {
		t.Errorf("new catalog = %+v", c)
	}
	if code := run([]string{"i18n", "extract", dir, "--lang", "ja"}, &out, &errOut); code != 1 {
		t.Errorf("extract --lang over an existing catalog exited %d, want 1", code)
	}

	for _, args := range [][]string{
		{"i18n"},
		{"i18n", "translate"},
		{"i18n", "check", "--lang", "ko"},
		{"i18n", "extract", "--out", "x.json"},
		{"i18n", "check", "--catalog", "[a"},
	} {
		code := run(args, &out, &errOut)
		if code == 0 {
			t.Errorf("%q succeeded", args)
		}
	}
}

// The entries a Go catalog lacks are printed as Go, ready to paste.
func TestExtractGoCatalog(t *testing.T) {
	src, err := i18n.Find(i18nProject(t), i18n.DefaultGlobs)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := extract(&out, src, "", ""); err != nil {
		t.Fatal(err)
	}
	want := "// ko (main.go:5)\n// Messages:\n\t\"Open [verb]\": \"\",\n// Plurals:\n\t\"%d file\": {Other: \"\"},\n"
	if out.String() != want {
		t.Errorf("printed:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestPrintCheck(t *testing.T) {
	src, err := i18n.Find(i18nProject(t), i18n.DefaultGlobs)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if !printCheck(&out, src, i18n.Options{}) {
		t.Error("missing translations did not fail")
	}
	text := out.String()
	for _, s := range []string{
		`fail  ko (main.go:5)  missing "Open [verb]", used at main.go:9:6`,
		"→ ggui i18n extract prints the entries to add",
		"2 errors, 0 warnings.",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("report lacks %q:\n%s", s, text)
		}
	}
}
