package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultID(t *testing.T) {
	for in, want := range map[string]string{
		"github.com/alice/hello":   "com.github.alice.hello",
		"example.com/my_app/cmd/x": "com.example.my-app.cmd.x",
		"hello":                    "com.example.hello",
		"gitlab.com/a/b.v2":        "com.gitlab.a.b-v2",
	} {
		if got := DefaultID(in); got != want {
			t.Errorf("DefaultID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenAndPaths(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644)
	os.WriteFile(filepath.Join(root, "ggui.toml"), []byte("[app]\npackage = \"./cmd/app\"\n[build]\noutput = \"dist\"\n"), 0o644)
	sub := filepath.Join(root, "cmd", "app")
	os.MkdirAll(sub, 0o755)

	t.Chdir(sub)
	p, err := Open(".", "", "")
	if err != nil {
		t.Fatal(err)
	}
	real := func(s string) string { r, _ := filepath.EvalSymlinks(s); return r }
	if real(p.Root) != real(root) {
		t.Errorf("Root = %s, want the file's directory %s", p.Root, root)
	}
	// The configured package is relative to the file, a command-line
	// one to the working directory.
	if pkg, _ := p.Package(""); real(pkg) != real(sub) {
		t.Errorf("configured package = %s, want %s", pkg, sub)
	}
	if pkg, _ := p.Package("."); real(pkg) != real(sub) {
		t.Errorf("package . = %s, want %s", pkg, sub)
	}
	if pkg, _ := p.Package("example.com/x/cmd/app"); pkg != "example.com/x/cmd/app" {
		t.Errorf("import path changed to %s", pkg)
	}
	if out, _ := p.OutDir(""); real(filepath.Dir(out)) != real(root) || filepath.Base(out) != "dist" {
		t.Errorf("OutDir = %s", out)
	}

	if _, err := Open(".", "", filepath.Join(root, "missing.toml")); err == nil {
		t.Error("an explicit --config that does not exist was accepted")
	}
}

// A package named on the command line brings its own ggui.toml, so that
// each app in a repository can have one.
func TestOpenFromPackageDir(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644)
	app := filepath.Join(root, "apps", "one")
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "ggui.toml"), []byte("[app]\nname = \"One\"\n"), 0o644)

	t.Chdir(root)
	p, err := Open(".", "./apps/one", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Config.App.Name != "One" {
		t.Errorf("did not read the package's ggui.toml: %+v", p.Config.App)
	}
	// Without a file anywhere, the defaults apply from the working
	// directory, where build/ then goes.
	os.MkdirAll(filepath.Join(root, "apps", "two"), 0o755)
	p, err = Open(".", "./apps/two", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.File != "" || filepath.Base(p.Root) != filepath.Base(root) {
		t.Errorf("File = %q, Root = %q", p.File, p.Root)
	}
}
