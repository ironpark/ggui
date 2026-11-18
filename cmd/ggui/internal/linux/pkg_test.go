package linux_test

import (
	"bytes"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/linux"
	"github.com/ironpark/ggui/cmd/ggui/internal/linux/linuxtest"
)

// tree assembles a tree for a, with a fake program and a 256-pixel icon.
func tree(t *testing.T, a linux.App) string {
	t.Helper()
	src := t.TempDir()
	bin := filepath.Join(src, "bin")
	if err := os.WriteFile(bin, []byte("\x7fELF program"), 0o755); err != nil {
		t.Fatal(err)
	}
	icon := filepath.Join(src, "icon.png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 256, 256))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(icon, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, _, err := linux.Assemble(linux.Tree{App: a, Binary: bin, Icon: icon, OutDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func app() linux.App {
	return linux.App{
		Name:       "Hello World",
		Summary:    "Says hello",
		ID:         "com.example.hello",
		Package:    "hello-world",
		Version:    "1.2.0",
		Arch:       "amd64",
		Homepage:   "https://example.com",
		License:    "MIT",
		Maintainer: "Jane Doe <jane@example.com>",
	}
}

// wantFiles are the files the tree of app has, with their modes.
var wantFiles = map[string]fs.FileMode{
	"/usr/bin/hello-world":                                        0o755,
	"/usr/share/applications/com.example.hello.desktop":           0o644,
	"/usr/share/metainfo/com.example.hello.metainfo.xml":          0o644,
	"/usr/share/icons/hicolor/16x16/apps/com.example.hello.png":   0o644,
	"/usr/share/icons/hicolor/24x24/apps/com.example.hello.png":   0o644,
	"/usr/share/icons/hicolor/32x32/apps/com.example.hello.png":   0o644,
	"/usr/share/icons/hicolor/48x48/apps/com.example.hello.png":   0o644,
	"/usr/share/icons/hicolor/64x64/apps/com.example.hello.png":   0o644,
	"/usr/share/icons/hicolor/128x128/apps/com.example.hello.png": 0o644,
	"/usr/share/icons/hicolor/256x256/apps/com.example.hello.png": 0o644,
}

func checkFiles(t *testing.T, got map[string]fs.FileMode) {
	t.Helper()
	for f, m := range wantFiles {
		if g, ok := got[f]; !ok {
			t.Errorf("no %s", f)
		} else if g != m {
			t.Errorf("%s has mode %v, want %v", f, g, m)
		}
	}
	for f := range got {
		if _, ok := wantFiles[f]; !ok {
			t.Errorf("unexpected file %s", f)
		}
	}
}

func TestDeb(t *testing.T) {
	a := app()
	for _, arch := range []struct{ goarch, deb string }{{"amd64", "amd64"}, {"arm64", "arm64"}, {"riscv64", "riscv64"}} {
		t.Run(arch.goarch, func(t *testing.T) {
			a.Arch = arch.goarch
			p := linux.Package{App: a, Tree: tree(t, a), Depends: config.DefaultDependsDeb, OutDir: t.TempDir()}
			name, err := p.FileName("deb")
			if err != nil {
				t.Fatal(err)
			}
			if want := "hello-world_1.2.0_" + arch.deb + ".deb"; name != want {
				t.Errorf("file name %s, want %s", name, want)
			}
			path, err := p.Make("deb")
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(path) != name {
				t.Errorf("made %s, named %s", path, name)
			}
			d, err := linuxtest.ReadDeb(path)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(d.Members, []string{"debian-binary", "control.tar.gz", "data.tar.gz"}) {
				t.Errorf("members %v", d.Members)
			}
			for k, v := range map[string]string{
				"Package":      "hello-world",
				"Version":      "1.2.0",
				"Architecture": arch.deb,
				"Maintainer":   "Jane Doe <jane@example.com>",
				"Homepage":     "https://example.com",
				"Description":  "Says hello",
				"Section":      "misc",
				"Priority":     "optional",
				"Depends":      "libx11-6, libxrandr2, libxi6, libxcursor1, libxinerama1, libxext6, libxrender1, libgl1, libegl1",
			} {
				if d.Control[k] != v {
					t.Errorf("%s: %q, want %q", k, d.Control[k], v)
				}
			}
			checkFiles(t, d.Files)
			for _, s := range []string{"postinst", "postrm"} {
				if !strings.Contains(d.Scripts[s], "command -v update-desktop-database") || !strings.Contains(d.Scripts[s], "gtk-update-icon-cache") {
					t.Errorf("%s: %q", s, d.Scripts[s])
				}
			}
		})
	}
}

func TestRPM(t *testing.T) {
	a := app()
	for _, arch := range []struct{ goarch, rpm string }{{"amd64", "x86_64"}, {"arm64", "aarch64"}, {"riscv64", "riscv64"}} {
		t.Run(arch.goarch, func(t *testing.T) {
			a.Arch = arch.goarch
			p := linux.Package{App: a, Tree: tree(t, a), Depends: config.DefaultDependsRPM, OutDir: t.TempDir()}
			path, err := p.Make("rpm")
			if err != nil {
				t.Fatal(err)
			}
			if want := "hello-world-1.2.0-1." + arch.rpm + ".rpm"; filepath.Base(path) != want {
				t.Errorf("file name %s, want %s", filepath.Base(path), want)
			}
			r, err := linuxtest.ReadRPM(path)
			if err != nil {
				t.Fatal(err)
			}
			if r.Name != "hello-world" || r.Version != "1.2.0" || r.Release != "1" || r.Arch != arch.rpm || r.Summary != "Says hello" || r.License != "MIT" {
				t.Errorf("header: %+v", r)
			}
			for _, d := range config.DefaultDependsRPM {
				if !slices.Contains(r.Requires, d) {
					t.Errorf("requires %v, not %s", r.Requires, d)
				}
			}
			checkFiles(t, r.Files)
			if !strings.Contains(r.PostIn, "gtk-update-icon-cache") || !strings.Contains(r.PostUn, "update-desktop-database") {
				t.Errorf("scripts: %q, %q", r.PostIn, r.PostUn)
			}
		})
	}
}

// A version that is not app.version's numbers, and no maintainer or
// license, still make valid packages.
func TestPackageOddVersion(t *testing.T) {
	a := app()
	a.Version = "dev"
	a.Maintainer, a.License, a.Homepage = "", "", ""
	dir := tree(t, a)
	d, err := linuxtest.ReadDeb(mk(t, linux.Package{App: a, Tree: dir, OutDir: t.TempDir()}, "deb"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Control["Version"] != "0~dev" || d.Control["Maintainer"] == "" {
		t.Errorf("control: %v", d.Control)
	}
	if _, ok := d.Control["Depends"]; ok {
		t.Errorf("Depends without dependencies: %q", d.Control["Depends"])
	}
	r, err := linuxtest.ReadRPM(mk(t, linux.Package{App: a, Tree: dir, OutDir: t.TempDir()}, "rpm"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "0~dev" {
		t.Errorf("rpm version %q", r.Version)
	}

	a.Version = "1.2.0-rc.1"
	r, err = linuxtest.ReadRPM(mk(t, linux.Package{App: a, Tree: dir, OutDir: t.TempDir()}, "rpm"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "1.2.0~rc.1" {
		t.Errorf("rpm version %q", r.Version)
	}
}

// mk makes p in format and returns its path.
func mk(t *testing.T, p linux.Package, format string) string {
	t.Helper()
	path, err := p.Make(format)
	if err != nil {
		t.Fatal(err)
	}
	return path
}
