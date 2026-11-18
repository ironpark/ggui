package i18n

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// write makes the files of a project below a temporary directory.
func write(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const app = `package main

import (
	"fmt"

	"github.com/ironpark/ggui/i18n"
)

type other struct{}

func (other) S(string) string { return "" }

func main() {
	var loc *i18n.Localizer
	key := "dynamic"
	fmt.Println(
		i18n.T("Hello, %s", "you"),
		i18n.T("Open").Context("verb"),
		i18n.T("Open"),
		i18n.T("Long " + "message"),
		i18n.T(key),
		i18n.N(2, "%d sandbox running", "%d sandboxes running", 2),
		loc.S("Saved"),
		loc.N(1, "%d file", "%d files", 1),
		loc.S(key), // a method of anything: not counted, not reported
	)
}
`

func TestScan(t *testing.T) {
	dir := write(t, map[string]string{
		"go.mod":  "module example.com/app\n",
		"main.go": app,
		"aliased/a.go": `package aliased
import tr "github.com/ironpark/ggui/i18n"
var _ = tr.T("Aliased")
`,
		"dot/d.go": `package dot
import . "github.com/ironpark/ggui/i18n"
var _ = T("Dotted")
`,
		"none/n.go": `package none
type x struct{}
func (x) S(s string) string { return s }
var _ = x{}.S("Not a message: no import")
`,
		"main_test.go":         "package main\nimport \"github.com/ironpark/ggui/i18n\"\nvar _ = i18n.T(\"In a test\")\n",
		"vendor/v/v.go":        "package v\nimport \"github.com/ironpark/ggui/i18n\"\nvar _ = i18n.T(\"Vendored\")\n",
		"testdata/t.go":        "package t\nimport \"github.com/ironpark/ggui/i18n\"\nvar _ = i18n.T(\"Test data\")\n",
		"nested/go.mod":        "module example.com/nested\n",
		"nested/n.go":          "package n\nimport \"github.com/ironpark/ggui/i18n\"\nvar _ = i18n.T(\"Nested module\")\n",
		"_hidden/h.go":         "package h\nimport \"github.com/ironpark/ggui/i18n\"\nvar _ = i18n.T(\"Hidden\")\n",
		"locales/notes.go.txt": "",
	})
	s, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	keys, pkeys := s.Keys()
	want := []string{"Aliased", "Dotted", "Hello, %s", "Long message", "Open", "Open [verb]", "Saved"}
	if !slices.Equal(keys, want) {
		t.Errorf("messages = %q, want %q", keys, want)
	}
	if want := []string{"%d file", "%d sandbox running"}; !slices.Equal(pkeys, want) {
		t.Errorf("plurals = %q, want %q", pkeys, want)
	}
	if m := s.Plurals["%d sandbox running"]; m.Other != "%d sandboxes running" || !m.Plural {
		t.Errorf("plural = %+v", m)
	}
	if m := s.Messages["Open [verb]"]; m.Source != "Open" || m.Uses[0].Filename != "main.go" || m.Uses[0].Line != 18 {
		t.Errorf("message with context = %+v", m)
	}
	if len(s.Skipped) != 1 || s.Skipped[0].Call != "i18n.T" || s.Skipped[0].Pos.Line != 21 {
		t.Errorf("skipped = %+v", s.Skipped)
	}
}

func TestScanNotParsing(t *testing.T) {
	dir := write(t, map[string]string{
		"a.go": "package a\nimport \"github.com/ironpark/ggui/i18n\"\nvar _ = i18n.T(\"Kept\")\n",
		"b.go": "package a\nfunc {\n",
	})
	s, err := Scan(dir)
	if err == nil || !strings.Contains(err.Error(), "b.go") {
		t.Errorf("err = %v", err)
	}
	if s == nil || s.Messages["Kept"] == nil {
		t.Error("the files that parse were not read")
	}
}

const goCatalogs = `package main

import "github.com/ironpark/ggui/i18n"

var ko = &i18n.Catalog{
	Lang: "ko",
	Messages: map[string]string{
		"Hello, %s":   "안녕하세요, %s{을/를}",
		"Open [verb]": " 열기",
		"Open":        dynamic(),
		"Stale":       "낡은",
	},
	Plurals: map[string]i18n.Forms{
		"%d sandbox running": {One: "샌드박스 하나", Other: "샌드박스 %d개"},
		"%d file":            {One: "파일 %d개"},
	},
}

func ja() i18n.Catalog {
	return i18n.Catalog{Lang: "ja", Messages: map[string]string{"Hello, %s": "こんにちは、%d"}}
}

func dynamic() string { return "" }
`

func TestCheck(t *testing.T) {
	dir := write(t, map[string]string{
		"main.go":     app,
		"catalogs.go": goCatalogs,
		"locales/de.json": `{"lang": "de", "messages": {"Hello, %s": "Hallo, %s", "Saved": "", "Moved %[2]s": "x"},
			"plurals": {"%d file": {"one": "%d Datei", "other": "%d Dateien", "several": "x"}}}`,
		"locales/broken.json":    `{"messages": {}}`,
		"elsewhere/i18n/fr.json": `{"lang": "fr"}`,
		"elsewhere/fr.json":      `{"lang": "not matched"}`,
	})
	s, err := Find(dir, DefaultGlobs)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range s.Catalogs {
		names = append(names, c.String())
	}
	want := []string{"ko (catalogs.go:5)", "ja (catalogs.go:20)", "fr (elsewhere/i18n/fr.json)", "locales/broken.json", "de (locales/de.json)"}
	if !slices.Equal(names, want) {
		t.Fatalf("catalogs = %q, want %q", names, want)
	}
	if got := Langs(s.Catalogs); !slices.Equal(got, []string{"ko", "ja", "fr", "de"}) {
		t.Errorf("Langs = %q", got)
	}

	// problems lists a catalog's problems as kind key, errors marked !.
	problems := func(c *Catalog, o Options) []string {
		var out []string
		for _, p := range Check(c, s, o) {
			mark := ""
			if p.Error {
				mark = "!"
			}
			out = append(out, mark+string(p.Kind)+" "+p.Key)
		}
		return out
	}
	ko := problems(s.Catalogs[0], Options{})
	// Open's translation is not a literal: present, and not checked.
	wantKo := []string{
		"!missing Long message", "!missing Saved",
		"!no other %d file",
		"whitespace Open [verb]",
		"unused Stale",
	}
	if !slices.Equal(ko, wantKo) {
		t.Errorf("ko:\n got %q\nwant %q", ko, wantKo)
	}
	if ja := problems(s.Catalogs[1], Options{AllowMissing: true}); !slices.Contains(ja, "!verbs Hello, %s") || !slices.Contains(ja, "missing Saved") {
		t.Errorf("ja: %q", ja)
	}
	de := problems(s.Catalogs[4], Options{})
	for _, p := range []string{"!missing Saved", "!form %d file", "unused Moved %[2]s", "!missing %d sandbox running"} {
		if !slices.Contains(de, p) {
			t.Errorf("de lacks %q: %q", p, de)
		}
	}
	if b := problems(s.Catalogs[3], Options{}); !slices.Equal(b, []string{"!invalid "}) {
		t.Errorf("broken: %q", b)
	}
}

func TestParseVerbs(t *testing.T) {
	for _, tc := range []struct {
		format string
		want   string // arg:verb ...
	}{
		{"no verbs, 100%% sure", ""},
		{"%d files in %s", "1d 2s"},
		{"%[2]s에 %[1]d개", "2s 1d"},
		{"%s{을/를} 열까요?", "1s"},
		{"%[1]s{이/가} %[2]d개", "1s 2d"},
		{"%-8.3f|%+q|%#x", "1f 2q 3x"},
		{"%*d", "1* 2d"},
		{"%.*f", "1* 2f"},
		{"%[3]*.[2]*[1]f", "3* 2* 1f"},
		{"100%", "1!"},
		{"%{을/를}", "1!"},
	} {
		var got []string
		for _, v := range ParseVerbs(tc.format) {
			got = append(got, strings.TrimSpace(string(rune('0'+v.Arg))+string(v.Verb)))
		}
		if s := strings.Join(got, " "); s != tc.want {
			t.Errorf("ParseVerbs(%q) = %q, want %q", tc.format, s, tc.want)
		}
	}
	if !sameVerbs(ParseVerbs("%d files in %s"), ParseVerbs("%[2]s에 %[1]d개 파일"), false) {
		t.Error("reordering with indexes counted as a difference")
	}
	if sameVerbs(ParseVerbs("%d files"), ParseVerbs("%s files"), false) {
		t.Error("a different verb counted as the same")
	}
	if sameVerbs(ParseVerbs("%d of %d"), ParseVerbs("%d"), false) || !sameVerbs(ParseVerbs("%d of %d"), ParseVerbs("%d"), true) {
		t.Error("subset is wrong")
	}
}

func TestMatchGlob(t *testing.T) {
	for _, tc := range []struct {
		pattern, name string
		want          bool
	}{
		{"**/i18n/*.json", "i18n/ko.json", true},
		{"**/i18n/*.json", "app/ui/i18n/ko.json", true},
		{"**/i18n/*.json", "app/i18n/sub/ko.json", false},
		{"**/locales/*.json", "locales.json", false},
		{"lang/*.json", "lang/ko.json", true},
		{"lang/*.json", "x/lang/ko.json", false},
		{"**", "a/b/c", true},
	} {
		if got := matchGlob(tc.pattern, tc.name); got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v", tc.pattern, tc.name, got)
		}
	}
	if ValidGlob("[a") == nil {
		t.Error("a malformed pattern passed")
	}
}

// A JSON catalog is written with its keys sorted, its other members kept,
// and what it had untouched.
func TestAddAndWriteJSON(t *testing.T) {
	dir := write(t, map[string]string{
		"main.go":         app,
		"locales/ko.json": `{"lang":"ko","note":"by hand","messages":{"Saved":"저장됨"},"plurals":{"%d file":{"other":"파일 %d개"}}}`,
	})
	s, err := Find(dir, DefaultGlobs)
	if err != nil {
		t.Fatal(err)
	}
	c := s.Catalogs[0]
	added, padded := c.Add(s)
	if !slices.Equal(added, []string{"Hello, %s", "Long message", "Open", "Open [verb]"}) || !slices.Equal(padded, []string{"%d sandbox running"}) {
		t.Errorf("added %q and %q", added, padded)
	}
	if err := c.WriteJSON(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(c.Path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "lang": "ko",
  "messages": {
    "Hello, %s": "",
    "Long message": "",
    "Open": "",
    "Open [verb]": "",
    "Saved": "저장됨"
  },
  "note": "by hand",
  "plurals": {
    "%d file": {
      "other": "파일 %d개"
    },
    "%d sandbox running": {
      "other": ""
    }
  }
}
`
	if string(got) != want {
		t.Errorf("wrote:\n%s\nwant:\n%s", got, want)
	}
	entries, _ := os.ReadDir(filepath.Dir(c.Path))
	if len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
}
