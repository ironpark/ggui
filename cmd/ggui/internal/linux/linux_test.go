package linux

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestPackageName(t *testing.T) {
	for in, want := range map[string]string{
		"Hello World":     "hello-world",
		"hello":           "hello",
		"My_App 2":        "my-app-2",
		"  Spaces  ":      "spaces",
		"g++":             "g++",
		"Café Browser":    "caf-browser",
		".hidden":         "hidden",
		"-x-":             "",
		"x":               "",
		"日本語":             "",
		"Notes (Beta)":    "notes-beta",
		"v1.2 tool.":      "v1.2-tool",
		"ACME Corp.-Tool": "acme-corp.-tool",
	} {
		if got := PackageName(in); got != want {
			t.Errorf("PackageName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPackageVersion(t *testing.T) {
	for in, want := range map[string]string{
		"1":                "1",
		"1.2":              "1.2",
		"1.2.3":            "1.2.3",
		"v1.2.3":           "1.2.3",
		"1.2.0-rc.1":       "1.2.0~rc.1",
		"1.2.0-rc.1+git.5": "1.2.0~rc.1+git.5",
		"dev":              "0~dev",
		"":                 "0",
		" 2.0 ":            "2.0",
		"1.0_beta":         "1.0.beta",
		"version":          "0~version",
		"1.0+a/b":          "1.0+a.b",
	} {
		if got := PackageVersion(in); got != want {
			t.Errorf("PackageVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEscapeString(t *testing.T) {
	for in, want := range map[string]string{
		"Hello World": "Hello World",
		`a\b`:         `a\\b`,
		"a\nb":        `a\nb`,
		"a\tb\rc":     `a\tb\rc`,
		" lead":       `\slead`,
		"trail ":      `trail\s`,
		"a;b":         "a;b", // only a list escapes it
		"Ünïcode ✓":   "Ünïcode ✓",
	} {
		if got := escapeString(in); got != want {
			t.Errorf("escapeString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEscapeList(t *testing.T) {
	if got, want := escapeList([]string{"Utility", "Graphics"}), "Utility;Graphics;"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := escapeList([]string{"a;b", `c\d`}), `a\;b;c\\d;`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExecValue(t *testing.T) {
	for _, c := range []struct {
		prog  string
		opens bool
		want  string
	}{
		{"hello", false, "hello"},
		{"hello", true, "hello %U"},
		{"my app", false, `"my app"`},
		{"100%", false, "100%%"},
		// The quoting puts a backslash before $, which the string
		// escaping then doubles.
		{"a$b", false, `"a\\$b"`},
		{`a"b`, false, `"a\\"b"`},
		{`a\b`, false, `"a\\\\b"`},
		{"", false, `""`},
	} {
		if got := execValue(c.prog, c.opens); got != c.want {
			t.Errorf("execValue(%q, %v) = %q, want %q", c.prog, c.opens, got, c.want)
		}
	}
}

func testApp() App {
	return App{
		Name:    "Hello World",
		Summary: "Says hello",
		ID:      "com.example.hello",
		Package: "hello-world",
		Version: "1.2.0",
		Arch:    "amd64",
	}
}

// desktopKeys reads a desktop entry's keys, checking it is one group.
func desktopKeys(t *testing.T, data []byte) map[string]string {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if lines[0] != "[Desktop Entry]" {
		t.Fatalf("first line %q", lines[0])
	}
	keys := map[string]string{}
	for _, l := range lines[1:] {
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			t.Fatalf("line %q is not a key", l)
		}
		if _, dup := keys[k]; dup {
			t.Fatalf("key %s twice", k)
		}
		keys[k] = v
	}
	return keys
}

func TestDesktopEntry(t *testing.T) {
	a := testApp()
	keys := desktopKeys(t, DesktopEntry(a, true))
	want := map[string]string{
		"Type":           "Application",
		"Version":        "1.5",
		"Name":           "Hello World",
		"Comment":        "Says hello",
		"Exec":           "hello-world",
		"Icon":           "com.example.hello",
		"Terminal":       "false",
		"Categories":     "Utility;",
		"StartupWMClass": "com.example.hello",
	}
	for k, v := range want {
		if keys[k] != v {
			t.Errorf("%s=%q, want %q", k, keys[k], v)
		}
	}
	for _, k := range []string{"GenericName", "MimeType"} {
		if _, ok := keys[k]; ok {
			t.Errorf("%s set without a value for it", k)
		}
	}

	a.Summary = ""
	a.GenericName = "Greeter"
	a.Categories = []string{"Graphics", "Viewer"}
	a.MIMETypes = []string{"image/png", "x-scheme-handler/hello"}
	keys = desktopKeys(t, DesktopEntry(a, false))
	for k, v := range map[string]string{
		"GenericName": "Greeter",
		"Exec":        "hello-world %U",
		"Categories":  "Graphics;Viewer;",
		"MimeType":    "image/png;x-scheme-handler/hello;",
	} {
		if keys[k] != v {
			t.Errorf("%s=%q, want %q", k, keys[k], v)
		}
	}
	for _, k := range []string{"Icon", "Comment"} {
		if _, ok := keys[k]; ok {
			t.Errorf("%s set without a value for it", k)
		}
	}
}

func TestMetainfo(t *testing.T) {
	a := testApp()
	a.License = "MIT"
	a.Homepage = "https://example.com/hello?a=1&b=2"
	a.Name = "Hello <World>"
	data := Metainfo(a)
	if !bytes.HasPrefix(data, []byte(`<?xml version="1.0" encoding="UTF-8"?>`)) {
		t.Errorf("no XML declaration:\n%s", data)
	}
	var c struct {
		Type            string `xml:"type,attr"`
		ID              string `xml:"id"`
		MetadataLicense string `xml:"metadata_license"`
		ProjectLicense  string `xml:"project_license"`
		Name            string `xml:"name"`
		Summary         string `xml:"summary"`
		Launchable      struct {
			Type  string `xml:"type,attr"`
			Value string `xml:",chardata"`
		} `xml:"launchable"`
		URL struct {
			Type  string `xml:"type,attr"`
			Value string `xml:",chardata"`
		} `xml:"url"`
		Binary  string `xml:"provides>binary"`
		Release struct {
			Version string `xml:"version,attr"`
		} `xml:"releases>release"`
	}
	if err := xml.Unmarshal(data, &c); err != nil {
		t.Fatalf("%v:\n%s", err, data)
	}
	if c.Type != "desktop-application" || c.ID != a.ID || c.MetadataLicense != "CC0-1.0" || c.ProjectLicense != "MIT" ||
		c.Name != a.Name || c.Summary != "Says hello" ||
		c.Launchable.Type != "desktop-id" || c.Launchable.Value != "com.example.hello.desktop" ||
		c.URL.Type != "homepage" || c.URL.Value != a.Homepage || c.Binary != "hello-world" || c.Release.Version != "1.2.0" {
		t.Errorf("read back %+v from:\n%s", c, data)
	}

	// Without a license, homepage or summary: the license and URL are
	// left out, and the summary is the name.
	a = testApp()
	a.Summary = ""
	data = Metainfo(a)
	for _, s := range []string{"project_license", "<url"} {
		if bytes.Contains(data, []byte(s)) {
			t.Errorf("has %s:\n%s", s, data)
		}
	}
	if !bytes.Contains(data, []byte("<summary>Hello World</summary>")) {
		t.Errorf("summary is not the name:\n%s", data)
	}
}

func writePNG(t *testing.T, path string, size int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.SetNRGBA(x, y, color.NRGBA{200, 100, 50, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assemble makes a tree for testApp with a fake program and an icon of
// iconSize pixels, none for 0.
func assemble(t *testing.T, iconSize int, svg bool) (Tree, string, []string) {
	t.Helper()
	src := t.TempDir()
	tr := Tree{App: testApp(), OutDir: t.TempDir(), Binary: filepath.Join(src, "bin")}
	if err := os.WriteFile(tr.Binary, []byte("\x7fELF program"), 0o600); err != nil {
		t.Fatal(err)
	}
	if iconSize > 0 {
		tr.Icon = filepath.Join(src, "icon.png")
		writePNG(t, tr.Icon, iconSize)
	}
	if svg {
		tr.SVG = filepath.Join(src, "icon.svg")
		if err := os.WriteFile(tr.SVG, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dir, warnings, err := Assemble(tr)
	if err != nil {
		t.Fatal(err)
	}
	return tr, dir, warnings
}

// treeFiles lists the files under dir, slash-separated, with their modes.
func treeFiles(t *testing.T, dir string) map[string]fs.FileMode {
	t.Helper()
	files := map[string]fs.FileMode{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = fi.Mode().Perm()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// wantTree lists the files a tree of testApp has, with an icon and
// optionally its SVG.
func wantTree(svg bool) []string {
	want := []string{
		"usr/bin/hello-world",
		"usr/share/applications/com.example.hello.desktop",
		"usr/share/metainfo/com.example.hello.metainfo.xml",
	}
	for _, s := range []string{"16", "24", "32", "48", "64", "128", "256", "512"} {
		want = append(want, "usr/share/icons/hicolor/"+s+"x"+s+"/apps/com.example.hello.png")
	}
	if svg {
		want = append(want, "usr/share/icons/hicolor/scalable/apps/com.example.hello.svg")
	}
	slices.Sort(want)
	return want
}

func TestAssemble(t *testing.T) {
	_, dir, warnings := assemble(t, 1024, true)
	if filepath.Base(dir) != "hello-world-linux-amd64" {
		t.Errorf("tree is %s", dir)
	}
	if len(warnings) > 0 {
		t.Errorf("warnings: %q", warnings)
	}
	files := treeFiles(t, dir)
	got := slices.Sorted(func(yield func(string) bool) {
		for k := range files {
			if !yield(k) {
				return
			}
		}
	})
	if want := wantTree(true); !slices.Equal(got, want) {
		t.Errorf("tree has\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if runtime.GOOS != "windows" {
		for f, m := range files {
			want := fs.FileMode(0o644)
			if f == "usr/bin/hello-world" {
				want = 0o755
			}
			if m != want {
				t.Errorf("%s has mode %v, want %v", f, m, want)
			}
		}
	}
	for _, size := range IconSizes {
		p := filepath.Join(dir, iconPath(testApp(), size))
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(f)
		f.Close()
		if err != nil || cfg.Width != size || cfg.Height != size {
			t.Errorf("%s: %dx%d, %v", p, cfg.Width, cfg.Height, err)
		}
	}
	keys := desktopKeys(t, mustRead(t, filepath.Join(dir, "usr/share/applications/com.example.hello.desktop")))
	if keys["Icon"] != "com.example.hello" {
		t.Errorf("Icon=%q", keys["Icon"])
	}

	// Assembling again replaces the tree: a file left there goes.
	stray := filepath.Join(dir, "stray")
	if err := os.WriteFile(stray, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tr := Tree{App: testApp(), OutDir: filepath.Dir(dir), Binary: filepath.Join(dir, "usr/bin/hello-world")}
	tmp := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(tmp, mustRead(t, tr.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	tr.Binary = tmp
	if _, _, err := Assemble(tr); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stray); err == nil {
		t.Error("a file of the old tree is left")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAssembleSmallIcon(t *testing.T) {
	_, dir, warnings := assemble(t, 100, false)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "128, 256, 512") {
		t.Errorf("warnings: %q", warnings)
	}
	files := treeFiles(t, dir)
	if _, ok := files["usr/share/icons/hicolor/64x64/apps/com.example.hello.png"]; !ok {
		t.Error("no 64x64 icon")
	}
	if _, ok := files["usr/share/icons/hicolor/128x128/apps/com.example.hello.png"]; ok {
		t.Error("a 128x128 icon scaled up")
	}
}

func TestAssembleNoIcon(t *testing.T) {
	_, dir, _ := assemble(t, 0, false)
	files := treeFiles(t, dir)
	if len(files) != 3 {
		t.Errorf("files: %v", files)
	}
	keys := desktopKeys(t, mustRead(t, filepath.Join(dir, "usr/share/applications/com.example.hello.desktop")))
	if _, ok := keys["Icon"]; ok {
		t.Error("Icon set without icons")
	}
}

func TestAssembleBadIcon(t *testing.T) {
	src := t.TempDir()
	tr := Tree{App: testApp(), OutDir: t.TempDir(), Binary: filepath.Join(src, "bin"), Icon: filepath.Join(src, "icon.png")}
	if err := os.WriteFile(tr.Binary, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, tr.Icon, 32)
	if _, _, err := Assemble(tr); err == nil || !strings.Contains(err.Error(), "at least 64") {
		t.Errorf("a 32-pixel icon: %v", err)
	}
}

// tarEntry is what a test reads back of a tarball's entry.
type tarEntry struct {
	mode       int64
	dir        bool
	uname      string
	uid, gid   int
	size       int64
	firstBytes string
}

func readTar(t *testing.T, path string) map[string]tarEntry {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]tarEntry{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		head := make([]byte, 4)
		n, _ := io.ReadFull(tr, head)
		out[h.Name] = tarEntry{mode: h.Mode, dir: h.Typeflag == tar.TypeDir, uname: h.Uname, uid: h.Uid, gid: h.Gid, size: h.Size, firstBytes: string(head[:n])}
	}
	return out
}

func TestTar(t *testing.T) {
	_, dir, _ := assemble(t, 1024, true)
	out := filepath.Join(t.TempDir(), TarName(testApp()))
	if filepath.Base(out) != "hello-world-1.2.0-linux-amd64.tar.gz" {
		t.Errorf("tarball is %s", filepath.Base(out))
	}
	if err := Tar(dir, out); err != nil {
		t.Fatal(err)
	}
	entries := readTar(t, out)
	var files []string
	for name, e := range entries {
		if strings.HasPrefix(name, "/") || strings.Contains(name, "..") || !strings.HasPrefix(name, "usr/") {
			t.Errorf("entry %q is not relative below usr/", name)
		}
		if e.uname != "root" || e.uid != 0 || e.gid != 0 {
			t.Errorf("%s is owned by %s (%d:%d)", name, e.uname, e.uid, e.gid)
		}
		switch {
		case e.dir:
			if e.mode != 0o755 {
				t.Errorf("directory %s has mode %o", name, e.mode)
			}
		case name == "usr/bin/hello-world":
			files = append(files, name)
			if e.mode != 0o755 || e.firstBytes != "\x7fELF" {
				t.Errorf("program: mode %o, starts %q", e.mode, e.firstBytes)
			}
		default:
			files = append(files, name)
			if e.mode != 0o644 {
				t.Errorf("%s has mode %o", name, e.mode)
			}
		}
	}
	slices.Sort(files)
	if want := wantTree(true); !slices.Equal(files, want) {
		t.Errorf("tarball has\n  %s\nwant\n  %s", strings.Join(files, "\n  "), strings.Join(want, "\n  "))
	}
	for _, d := range []string{"usr/", "usr/bin/", "usr/share/icons/hicolor/scalable/apps/"} {
		if !entries[d].dir {
			t.Errorf("no directory entry %s", d)
		}
	}
}
