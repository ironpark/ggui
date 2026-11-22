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
		{"i18n", "extract", dir, "--lang", "../evil"},
		{"i18n", "extract", dir, "--lang", "en/us"},
		{"i18n", "extract", dir, "--lang", `en\us`},
		{"i18n", "extract", dir, "--lang", ".."},
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

// The app's own functions are named in ggui.toml and with flags, which
// add to it.
func TestI18nFuncs(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"go.mod":    "module example.com/app\n",
		"ggui.toml": "[i18n]\nfuncs = [\"tr\"]\n",
		"main.go": `package main

func tr(key string, args ...any) string { return key }

func trn(n int, one, other string) string { return one }

var k = "dynamic"

var _, _, _ = tr("Hello"), trn(2, "%d file", "%d files"), tr(k)
`,
		"locales/ko.json": `{"lang": "ko", "messages": {"Hello": "안녕"}}`,
	} {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"i18n", "check", dir}, &out, &errOut); code != 0 {
		t.Errorf("check with ggui.toml's tr exited %d, want 0: %s", code, errOut.String())
	}
	if code := run([]string{"i18n", "check", dir, "--plural", "trn"}, &out, &errOut); code != 1 {
		t.Errorf("check --plural trn exited %d, want 1 for the missing plural", code)
	}

	funcs, _ := i18n.ParseFuncs([]string{"tr"}, []string{"trn"})
	src, err := i18n.Find(dir, i18n.DefaultGlobs, funcs...)
	if err != nil {
		t.Fatal(err)
	}
	var report bytes.Buffer
	printCheck(&report, src, i18n.Options{})
	for _, s := range []string{
		"warn  sources",
		"tr at main.go:9:59: the message is not a string literal",
		`fail  ko (locales/ko.json)  missing plural "%d file", used at main.go:9:28`,
	} {
		if !strings.Contains(report.String(), s) {
			t.Errorf("report lacks %q:\n%s", s, report.String())
		}
	}

	if code := run([]string{"i18n", "extract", dir, "--plural", "trn"}, &out, &errOut); code != 0 {
		t.Fatalf("extract exited %d: %s", code, errOut.String())
	}
	c, err := i18n.ReadJSON(filepath.Join(dir, "locales", "ko.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Plurals["%d file"]; !ok || c.Messages["Hello"] != "안녕" || len(c.Messages) != 1 {
		t.Errorf("extracted catalog = %+v", c)
	}

	for _, args := range [][]string{
		{"i18n", "check", dir, "--func", "(*M).tr"},
		{"i18n", "check", dir, "--plural", "trn:1"},
		{"i18n", "extract", dir, "--func", "tr:x"},
	} {
		if code := run(args, &out, &errOut); code == 0 {
			t.Errorf("%q succeeded", args)
		}
	}
	os.WriteFile(filepath.Join(dir, "ggui.toml"), []byte("[i18n]\nplurals = [\"trn:2,2\"]\n"), 0o644)
	errOut.Reset()
	if code := run([]string{"i18n", "check", dir}, &out, &errOut); code == 0 || !strings.Contains(errOut.String(), "i18n.plurals") {
		t.Errorf("a bad i18n.plurals exited %d: %s", code, errOut.String())
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
