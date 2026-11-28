package svgicon

import (
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
)

// The program is Go that gofmt leaves as it is, a main package that
// imports svgx, drawing at the size asked for.
func TestSource(t *testing.T) {
	src := Source(512)
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name.Name != "main" {
		t.Errorf("package %s", f.Name.Name)
	}
	var imports []string
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		imports = append(imports, p)
	}
	for _, want := range []string{svgxModule, "image/png"} {
		if !slices.Contains(imports, want) {
			t.Errorf("imports %v, without %s", imports, want)
		}
	}
	if !strings.Contains(string(src), "const size = 512\n") || strings.Contains(string(src), "SIZE") {
		t.Errorf("the size is not filled in:\n%s", src)
	}
	formatted, err := format.Source(src)
	if err != nil || string(formatted) != string(src) {
		t.Errorf("not gofmt'd: %v", err)
	}
}

func TestIs(t *testing.T) {
	for path, want := range map[string]bool{
		"icon.svg": true, "art/Icon.SVG": true, "icon.png": false, "icon.svgz": false, "": false,
	} {
		if Is(path) != want {
			t.Errorf("Is(%q) = %v", path, !want)
		}
	}
}

// The program's directory is one the project does not have.
func TestVirtualDir(t *testing.T) {
	dir := t.TempDir()
	if name, err := virtualDir(dir); err != nil || name != "ggui_svgicon" {
		t.Errorf("%q, %v", name, err)
	}
	os.Mkdir(filepath.Join(dir, "ggui_svgicon"), 0o755)
	os.WriteFile(filepath.Join(dir, "ggui_svgicon1"), nil, 0o644)
	if name, err := virtualDir(dir); err != nil || name != "ggui_svgicon2" {
		t.Errorf("%q, %v", name, err)
	}
}

// What the go command says when the program does not build becomes what
// to do about it.
func TestBuildFailure(t *testing.T) {
	pinned := gotool.Module{Path: svgxModule, Version: "v0.1.0"}
	replaced := gotool.Module{Path: svgxModule, Version: "v0.1.1", Replace: &gotool.Module{Path: "../svgx"}}
	for _, tc := range []struct {
		name    string
		svgxMod gotool.Module
		output  string
		want    []string
	}{
		{"no svgx package, readonly", pinned,
			"ggui_svgicon/main.go:13:2: cannot find module providing package github.com/ironpark/svgx: import lookup disabled by -mod=readonly\n",
			[]string{"app.icon art/icon.svg is an SVG", "(v0.1.0) does not provide that package", "go get github.com/ironpark/ggui@latest", "1024×1024 PNG"}},
		{"no svgx package, replaced", replaced,
			"ggui_svgicon/main.go:13:2: no required module provides package github.com/ironpark/svgx; to add it:\n\tgo get github.com/ironpark/svgx\n",
			[]string{"(../svgx) does not provide"}},
		{"no svgx package in the version", pinned,
			"ggui_svgicon/main.go:13:2: module github.com/ironpark/svgx@v0.1.0 found, but does not contain package github.com/ironpark/svgx\n",
			[]string{"does not provide that package"}},
		{"go.sum", pinned,
			"ggui_svgicon/main.go:13:2: missing go.sum entry for module providing package github.com/ironpark/svgx (imported by hello/ggui_svgicon); to add:\n\tgo get hello/ggui_svgicon\n",
			[]string{"go.sum lacks", "go mod download github.com/ironpark/svgx", "missing go.sum entry"}},
		{"another API", pinned,
			"# hello/ggui_svgicon1\nggui_svgicon1\\main.go:41:7: doc.Draw undefined (type *svgx.Document has no field or method Draw)\n",
			[]string{"not one this ggui draws with", "update ggui and the ggui command together", "doc.Draw undefined"}},
		{"anything else", pinned,
			"go: updates to go.mod needed; to update it:\n\tgo mod tidy\n",
			[]string{"building the program that draws it with the project's svgx failed", "go mod tidy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := buildFailure("art/icon.svg", tc.svgxMod, tc.output).Error()
			for _, w := range tc.want {
				if !strings.Contains(msg, w) {
					t.Errorf("%q\nlacks %q", msg, w)
				}
			}
		})
	}
}
