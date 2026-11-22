package scaffold

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
)

// checkout returns the root of the ggui checkout this test runs in.
func checkout(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil || !strings.HasPrefix(string(data), "module "+GGUI+"\n") {
		t.Skipf("not inside a ggui checkout: %s", root)
	}
	return root
}

// The app init writes must compile against the ggui it is next to: the
// template is code, and nothing else would notice it falling behind the
// API.
func TestInitCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go mod tidy and builds the app")
	}
	root := checkout(t)
	dir := filepath.Join(t.TempDir(), "My App")
	r, err := Init(t.Context(), Options{Dir: dir, Replace: root, Log: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if r.Module != "my-app" || r.Name != "My App" || !r.Fetched {
		t.Errorf("result = %+v", r)
	}
	for _, f := range Files {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Error(err)
		}
	}
	if _, err := config.Load(filepath.Join(dir, config.FileName)); err != nil {
		t.Errorf("the ggui.toml written does not load: %v", err)
	}
	for _, args := range [][]string{{"vet", "."}, {"build", "-o", os.DevNull, "."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

func TestInitRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	_, err := Init(t.Context(), Options{Dir: dir, Offline: true})
	if err == nil || !strings.Contains(err.Error(), "main.go") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want a refusal naming main.go and --force", err)
	}
	// Nothing was written, not even the files that did not exist.
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		t.Error("go.mod was written though init refused")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "main.go")); string(data) != "package main\n" {
		t.Error("main.go was changed")
	}

	if _, err := Init(t.Context(), Options{Dir: dir, Offline: true, Force: true, Module: "example.com/x"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.HasPrefix(string(data), "module example.com/x\n") {
		t.Errorf("go.mod = %q", data)
	}
}

func TestModulePath(t *testing.T) {
	for in, want := range map[string]string{
		"hello":       "hello",
		"My App":      "my-app",
		"app.v2":      "app.v2",
		"--weird!!--": "weird",
		"한글":          "",
	} {
		if got := modulePath(in); got != want {
			t.Errorf("modulePath(%q) = %q, want %q", in, got, want)
		}
	}
	if err := checkModulePath("example.com/hello world"); err == nil {
		t.Error("a module path with a space was accepted")
	}
}

func TestQuoteGoModPath(t *testing.T) {
	if got := quoteGoModPath("../ggui"); got != "../ggui" {
		t.Errorf("plain path quoted: %q", got)
	}
	if got := quoteGoModPath("/Users/a b/ggui"); got != `"/Users/a b/ggui"` {
		t.Errorf("spaced path = %q", got)
	}
	if got := quoteGoModPath(`../my"dir`); got != `"../my\"dir"` {
		t.Errorf("quoted char path = %q", got)
	}
}
