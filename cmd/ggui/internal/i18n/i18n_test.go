package i18n

import (
	"fmt"
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
		{"%[0]s", "1!"},
		{"%[abc]s", "1!"},
		{"%[]s", "1!"},
		{"%[1s", "1!"},
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

// wrapped is a project that wraps the i18n package in functions of its
// own, of every form a Func names.
var wrapped = map[string]string{
	"go.mod": "module example.com/app\n",
	"main.go": `package main

import (
	"fmt"

	u "example.com/app/ui"
	"example.com/app/msgs"
)

const greeting = "Hello, %s"

const (
	saveLabel = "Save" + " all"
	alsoSave
)

type model struct{}

func (m *model) tr(key string, args ...any) string { return key }

func main() {
	var m model
	name := "x"
	fmt.Println(
		tr("Delete %s?", name),
		tr(greeting, name),
		tr(fromAnotherFile),
		tr(alsoSave),
		m.tr("Method"),
		tr("Open").Context("verb"),
		trn(2, "%d file", "%d files", 2),
		u.T("Aliased"),
		say(1, "Positioned"),
		count("%d item", "%d items", 3),
		tr(name),
		tr(msgs.Hello),
		untr("Not a wrapper"),
	)
}

func shadowed() {
	greeting := "local"
	_ = tr(greeting)
}

func localConst() {
	const k = "Local constant"
	_ = tr(k)
}
`,
	"consts.go": "package main\n\nconst fromAnotherFile = \"From another file\"\n",
	"wrap.go": `package main

import "github.com/ironpark/ggui/i18n"

var loc *i18n.Localizer

func tr(key string, args ...any) string { return i18n.T(key, args...).String() }

func trn(n int, one, other string, args ...any) string { return loc.N(n, one, other, args...) }

var say = func(n int, key string) string { return i18n.T(key).String() }

func count(one, other string, n int) string { return i18n.N(n, one, other).String() }

func untr(key string) string { return i18n.T(key).String() }
`,
	"ui/ui.go":       "package ui\n\nfunc T(key string) string { return key }\n\nvar _ = T(\"Same package\")\n",
	"other/o.go":     "package other\n\nimport \"example.com/app/ui\"\n\nvar x struct{ T func(string) string }\n\nvar _, _ = ui.T(\"Qualified\"), x.T(\"Not ui's\")\n",
	"dotted/d.go":    "package dotted\n\nimport . \"example.com/app/ui\"\n\nvar _ = T(\"Dot-imported\")\n",
	"msgs/m.go":      "package msgs\n\nconst Hello = \"Hello\"\n",
	"twice/a.go":     "package twice\n\nconst k = \"A\"\n\nfunc tr(string) {}\n\nfunc init() { tr(k) }\n",
	"twice/b_x.go":   "package twice\n\nconst k = \"B\"\n",
	"twice/same.go":  "package twice\n\nconst s = \"Same\"\n\nfunc init() { tr(s) }\n",
	"twice/same2.go": "package twice\n\nconst s = \"Same\"\n",
}

func TestScanFuncs(t *testing.T) {
	funcs, err := ParseFuncs([]string{"tr", "ui.T", "say:1"}, []string{"trn", "count:0,1"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := Scan(write(t, wrapped), funcs...)
	if err != nil {
		t.Fatal(err)
	}
	keys, pkeys := s.Keys()
	want := []string{
		"Aliased", "Delete %s?", "Dot-imported", "From another file", "Hello, %s",
		"Local constant", "Method", "Open [verb]", "Positioned", "Qualified",
		"Same", "Same package", "Save all",
	}
	if !slices.Equal(keys, want) {
		t.Errorf("messages:\n got %q\nwant %q", keys, want)
	}
	if want := []string{"%d file", "%d item"}; !slices.Equal(pkeys, want) {
		t.Errorf("plurals = %q, want %q", pkeys, want)
	}
	if m := s.Plurals["%d item"]; m == nil || m.Other != "%d items" {
		t.Errorf("count's plural = %+v", m)
	}
	// The wrappers' own bodies pass their parameters on, and are not
	// listed; untr is not a wrapper, and a name that is not a constant of
	// the package, or is declared twice with two values, is no message.
	var skipped []string
	for _, sk := range s.Skipped {
		skipped = append(skipped, fmt.Sprintf("%s %s:%d", sk.Call, sk.Pos.Filename, sk.Pos.Line))
	}
	wantSkipped := []string{"tr main.go:35", "tr main.go:36", "tr main.go:43", "tr twice/a.go:7", "i18n.T wrap.go:15"}
	if !slices.Equal(skipped, wantSkipped) {
		t.Errorf("skipped:\n got %q\nwant %q", skipped, wantSkipped)
	}

	// Without the funcs, only the i18n package's calls count, and the
	// wrappers' bodies are listed.
	s, err = Scan(write(t, wrapped))
	if err != nil {
		t.Fatal(err)
	}
	if keys, _ := s.Keys(); len(keys) != 0 || len(s.Skipped) != 4 {
		t.Errorf("without funcs: %q, skipped %+v", keys, s.Skipped)
	}
}

func TestParseFunc(t *testing.T) {
	for _, tc := range []struct {
		spec   string
		plural bool
		want   string // pkg name key other, or error
	}{
		{"tr", false, " tr 0 -1"},
		{"ui.T", false, "ui T 0 -1"},
		{"tr:2", false, " tr 2 -1"},
		{"trn", true, " trn 1 2"},
		{"ui.N:0,1", true, "ui N 0 1"},
		{"(*M).tr", false, "error"},
		{"M.tr.x", false, "error"},
		{"", false, "error"},
		{"tr:", false, "error"},
		{"tr:-1", false, "error"},
		{"trn:1", true, "error"},
		{"trn:1,1", true, "error"},
		{"trn:a,b", true, "error"},
	} {
		f, err := ParseFunc(tc.spec, tc.plural)
		got := fmt.Sprintf("%s %s %d %d", f.Pkg, f.Name, f.Key, f.Other)
		if err != nil {
			got = "error"
		}
		if got != tc.want {
			t.Errorf("ParseFunc(%q, %v) = %q, %v; want %q", tc.spec, tc.plural, got, err, tc.want)
		}
	}
	if _, err := ParseFuncs([]string{"a:x", "ok"}, []string{"b:1"}); err == nil || !strings.Contains(err.Error(), `"a:x"`) || !strings.Contains(err.Error(), `"b:1"`) {
		t.Errorf("ParseFuncs did not report each: %v", err)
	}
	if got := pathName("gopkg.in/yaml.v3") + " " + pathName("example.com/go-ui/v2") + " " + pathName("ui"); got != "yaml ui ui" {
		t.Errorf("pathName = %q", got)
	}
}

func TestShadowedLocalConstAndSeparateDynamicMaps(t *testing.T) {
	dir := write(t, map[string]string{
		"main.go": `package main

import "github.com/ironpark/ggui/i18n"

func dyn() string { return "x" }

var ko = i18n.Catalog{
	Lang:     "ko",
	Messages: map[string]string{"%d item": dyn()},
	Plurals:  map[string]i18n.Forms{"%d item": {Other: ""}},
}

func main() {
	const k = "const-val"
	if true {
		k := dyn()
		_ = i18n.T(k)
	}
	_ = i18n.T("%d item")
	_ = i18n.N(1, "%d item", "%d items", 1)
}
`,
	})
	s, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Messages["const-val"]; ok {
		t.Error("shadowed local const was resolved as literal")
	}
	if len(s.Skipped) != 1 {
		t.Errorf("skipped = %+v, want 1", s.Skipped)
	}
	ps := Check(s.Catalogs[0], s, Options{})
	foundMissingPlural := false
	for _, p := range ps {
		if p.Kind == Missing && p.Key == "%d item" && strings.Contains(p.Detail, "plural") {
			foundMissingPlural = true
		}
	}
	if !foundMissingPlural {
		t.Errorf("empty plural was skipped because Messages had same dynamic key: %+v", ps)
	}
}
