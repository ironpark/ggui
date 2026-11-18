package doctor

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
)

// Away from a Mac ggui does not sign, which matters only when ggui.toml
// asks for a signature.
func TestCheckSigningElsewhere(t *testing.T) {
	m := config.Defaults().MacOS
	r := &Report{}
	checkSigningElsewhere(r, m)
	m.SigningIdentity = "Developer ID Application"
	checkSigningElsewhere(r, m)
	if r.Checks[0].Status != OK || r.Checks[1].Status != Fail {
		t.Errorf("checks = %v", r.Checks)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"1.27.3", "1.27.0", 1},
		{"1.27", "1.27.0", 0},
		{"1.26.9", "1.27.0", -1},
		{"1.27rc1", "1.27", 0},
		{"13.0", "13.0", 0},
		{"12.7.1", "13.0", -1},
	} {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCheckIcon(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, w, h int) string {
		p := filepath.Join(dir, name)
		var buf bytes.Buffer
		png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h)))
		os.WriteFile(p, buf.Bytes(), 0o644)
		return p
	}
	for _, tc := range []struct {
		path string
		want Status
	}{
		{write("big.png", 1024, 1024), OK},
		{write("small.png", 256, 256), Warn},
		{write("tiny.png", 128, 128), Warn},
		{write("wide.png", 200, 100), Fail},
		{filepath.Join(dir, "missing.png"), Fail},
		{filepath.Join(dir, "missing.svg"), Fail},
		// The build draws an SVG; the doctor does not compile anything.
		{func() string {
			p := filepath.Join(dir, "icon.svg")
			os.WriteFile(p, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`), 0o644)
			return p
		}(), OK},
	} {
		r := &Report{}
		checkIcon(r, tc.path)
		if got := r.Checks[0].Status; got != tc.want {
			t.Errorf("%s: %s (%s), want %s", filepath.Base(tc.path), got, r.Checks[0].Detail, tc.want)
		}
	}
}

// What a Windows build takes from ggui.toml that macOS does not check: a
// version in 16-bit parts, a name a file can have, a manifest that parses.
func TestCheckWindowsBuild(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "good.manifest"), []byte(`<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0"/>`), 0o644)
	os.WriteFile(filepath.Join(dir, "bad.manifest"), []byte(`<assembly`), 0o644)
	for _, tc := range []struct {
		name, src string
		want      Status
	}{
		{"defaults", "", OK},
		{"own manifest", "[windows]\nmanifest = \"good.manifest\"\nconsole = true\n", OK},
		{"date version", "[app]\nversion = \"20261118\"\n", Fail},
		{"name with a colon", "[app]\nname = \"Notes: Pro\"\n", Fail},
		{"broken manifest", "[windows]\nmanifest = \"bad.manifest\"\n", Fail},
		{"missing manifest", "[windows]\nmanifest = \"none.manifest\"\n", Fail},
	} {
		cfg, err := config.Parse("ggui.toml", []byte(tc.src))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		r := &Report{}
		checkWindowsBuild(r, &project.Project{Root: dir, File: filepath.Join(dir, "ggui.toml"), Config: cfg})
		if len(r.Checks) != 1 || r.Checks[0].Status != tc.want {
			t.Errorf("%s: %+v, want %s", tc.name, r.Checks, tc.want)
		}
	}
}

func TestReportOutput(t *testing.T) {
	r := &Report{}
	r.ok("go", "go1.27.3")
	r.warn("wasm-opt", "not on PATH", "brew install binaryen")
	var text bytes.Buffer
	r.Print(&text)
	if !strings.Contains(text.String(), "warn  wasm-opt  not on PATH") || !strings.Contains(text.String(), "→ brew install binaryen") {
		t.Errorf("text report:\n%s", text.String())
	}
	if r.Failed() {
		t.Error("a warning counted as a failure")
	}
	r.fail("display", "none", "")
	var js bytes.Buffer
	r.PrintJSON(&js)
	var got struct {
		OK     bool
		Checks []Check
	}
	if err := json.Unmarshal(js.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OK || len(got.Checks) != 3 || got.Checks[2].Status != Fail {
		t.Errorf("JSON report = %+v", got)
	}
}

// Translations are checked only in a project with catalogs, and what is
// wrong with them is a warning: the app still runs.
func TestCheckTranslations(t *testing.T) {
	dir := t.TempDir()
	r := &Report{}
	checkTranslations(r, dir)
	if len(r.Checks) != 0 {
		t.Errorf("a project without catalogs: %+v", r.Checks)
	}
	src := "package main\nimport \"github.com/ironpark/ggui/i18n\"\nvar _ = i18n.T(\"Hello\")\n"
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644)
	os.MkdirAll(filepath.Join(dir, "locales"), 0o755)
	os.WriteFile(filepath.Join(dir, "locales", "ko.json"), []byte(`{"lang": "ko", "messages": {"Hello": "안녕"}}`), 0o644)
	checkTranslations(r, dir)
	os.WriteFile(filepath.Join(dir, "locales", "ja.json"), []byte(`{"lang": "ja"}`), 0o644)
	checkTranslations(r, dir)
	if len(r.Checks) != 2 || r.Checks[0].Status != OK || r.Checks[1].Status != Warn || r.Checks[1].Detail != "ja: 1 error" {
		t.Errorf("checks = %+v", r.Checks)
	}
}
