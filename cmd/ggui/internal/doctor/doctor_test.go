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
)

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
		{write("wide.png", 200, 100), Fail},
		{filepath.Join(dir, "missing.png"), Fail},
	} {
		r := &Report{}
		checkIcon(r, tc.path)
		if got := r.Checks[0].Status; got != tc.want {
			t.Errorf("%s: %s (%s), want %s", filepath.Base(tc.path), got, r.Checks[0].Detail, tc.want)
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
