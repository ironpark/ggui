package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"debug/elf"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/linux/linuxtest"
	"github.com/ironpark/ggui/cmd/ggui/internal/scaffold"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

// buildLinux runs ggui build --target linux on dir into a new directory
// and returns it, the exit status and what ggui said.
func buildLinux(t *testing.T, dir string, args ...string) (string, int, string) {
	t.Helper()
	out := t.TempDir()
	var said, stdout, stderr bytes.Buffer
	saved := ui.Stderr
	ui.Stderr = ui.New(&said)
	defer func() { ui.Stderr = saved }()
	code := run(append([]string{"build", dir, "--target", "linux", "--out", out}, args...), &stdout, &stderr)
	return out, code, said.String() + stderr.String()
}

// tarNames lists the regular files of a gzipped tarball.
func tarNames(t *testing.T, path string) []string {
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
	var names []string
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			names = append(names, h.Name)
		}
	}
	slices.Sort(names)
	return names
}

// checkELF checks path is a Linux program for x86-64 that needs no
// shared library at start-up but glibc's, which purego links to open the
// rest at run time.
func checkELF(t *testing.T, path string) {
	t.Helper()
	f, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Machine != elf.EM_X86_64 || f.OSABI != elf.ELFOSABI_NONE && f.OSABI != elf.ELFOSABI_LINUX {
		t.Errorf("%s: machine %v, OS ABI %v", path, f.Machine, f.OSABI)
	}
	libs, err := f.ImportedLibraries()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range libs {
		if !slices.Contains([]string{"libc.so.6", "libdl.so.2", "libpthread.so.0"}, l) {
			t.Errorf("%s links %s at start-up", path, l)
		}
	}
}

func TestBuildLinuxTiny(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles a program for Linux")
	}
	buildEnv(t)
	out, code, said := buildLinux(t, tiny(t))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, said)
	}
	tree := filepath.Join(out, "tiny-linux-amd64")
	checkELF(t, filepath.Join(tree, "usr/bin/tiny"))
	want := []string{
		"usr/bin/tiny",
		"usr/share/applications/com.example.tiny.desktop",
		"usr/share/metainfo/com.example.tiny.metainfo.xml",
	}
	if got := tarNames(t, filepath.Join(out, "tiny-0.1.0-linux-amd64.tar.gz")); !slices.Equal(got, want) {
		t.Errorf("tarball has %v, want %v", got, want)
	}
	if !strings.Contains(said, "app.icon is not set") {
		t.Errorf("no word of the missing icon:\n%s", said)
	}
	// The program is told its ID, which it sets as its window class.
	if !strings.Contains(said, "CGO_ENABLED=0 ") || !strings.Contains(said, "-X github.com/ironpark/ggui.buildID=com.example.tiny") {
		t.Errorf("the go command line lacks CGO_ENABLED=0 or the ID:\n%s", said)
	}
}

func TestBuildLinuxFormatFlag(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--target", "linux", "--format", "tar,zip"}, `--format "zip"`},
		{[]string{"--target", "web", "--format", "deb"}, "--target linux"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(append([]string{"build", tiny(t)}, c.args...), &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), c.want) {
			t.Errorf("%v: exit %d: %s", c.args, code, stderr.String())
		}
	}
	for in, want := range map[string][]string{
		"":              nil,
		"tar":           {"tar"},
		"deb, RPM,deb,": {"deb", "rpm"},
	} {
		if got, err := parseFormats(in); err != nil || !slices.Equal(got, want) {
			t.Errorf("parseFormats(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
}

func TestBuildLinuxArch(t *testing.T) {
	if testing.Short() {
		t.Skip("asks the go command")
	}
	buildEnv(t)
	t.Setenv("GOARCH", "mips")
	if _, code, said := buildLinux(t, tiny(t)); code == 0 || !strings.Contains(said, "GOARCH=mips") {
		t.Errorf("exit %d: %s", code, said)
	}
}

// The app ggui init writes, with an SVG icon, must build for Linux into
// an install tree with every icon size and the SVG, and the tarball,
// Debian package and RPM package of it.
func TestBuildLinuxInitApp(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go mod tidy and cross-compiles the app")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "go.mod")); err != nil || !strings.HasPrefix(string(data), "module "+scaffold.GGUI+"\n") {
		t.Skipf("not inside a ggui checkout: %s", root)
	}
	buildEnv(t)
	dir := filepath.Join(t.TempDir(), "hello")
	if _, err := scaffold.Init(t.Context(), scaffold.Options{Dir: dir, Name: "Hello World", Replace: root, Log: io.Discard}); err != nil {
		t.Skipf("a ggui project against %s needs the modules ggui requires, from the module cache or the network: %v", root, err)
	}
	os.WriteFile(filepath.Join(dir, "icon.svg"), []byte(circleSVG), 0o644)
	setTOML(t, dir, `# icon = "icon.png"`, `icon = "icon.svg"`)

	out, code, said := buildLinux(t, dir, "--format", "tar,deb,rpm")
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, said)
	}
	id := readID(t, dir)
	tree := filepath.Join(out, "hello-world-linux-amd64")
	checkELF(t, filepath.Join(tree, "usr/bin/hello-world"))
	want := []string{
		"usr/bin/hello-world",
		"usr/share/applications/" + id + ".desktop",
		"usr/share/icons/hicolor/scalable/apps/" + id + ".svg",
		"usr/share/metainfo/" + id + ".metainfo.xml",
	}
	for _, s := range []string{"16", "24", "32", "48", "64", "128", "256", "512"} {
		want = append(want, "usr/share/icons/hicolor/"+s+"x"+s+"/apps/"+id+".png")
	}
	slices.Sort(want)
	if got := tarNames(t, filepath.Join(out, "hello-world-0.1.0-linux-amd64.tar.gz")); !slices.Equal(got, want) {
		t.Errorf("tarball has\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	deb, err := linuxtest.ReadDeb(filepath.Join(out, "hello-world_0.1.0_amd64.deb"))
	if err != nil {
		t.Fatal(err)
	}
	rpm, err := linuxtest.ReadRPM(filepath.Join(out, "hello-world-0.1.0-1.x86_64.rpm"))
	if err != nil {
		t.Fatal(err)
	}
	for _, files := range []map[string]fs.FileMode{deb.Files, rpm.Files} {
		var got []string
		for f := range files {
			got = append(got, strings.TrimPrefix(f, "/"))
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("a package has\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}
	}
	if deb.Control["Package"] != "hello-world" || deb.Control["Architecture"] != "amd64" || !strings.Contains(deb.Control["Depends"], "libx11-6") {
		t.Errorf("deb control: %v", deb.Control)
	}
	if rpm.Name != "hello-world" || rpm.Arch != "x86_64" || !slices.Contains(rpm.Requires, "libX11") {
		t.Errorf("rpm header: %+v", rpm)
	}
	desktop, err := os.ReadFile(filepath.Join(tree, "usr/share/applications", id+".desktop"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"Name=Hello World", "Exec=hello-world", "Icon=" + id, "StartupWMClass=" + id} {
		if !bytes.Contains(desktop, []byte("\n"+line+"\n")) {
			t.Errorf("the desktop entry has no line %s:\n%s", line, desktop)
		}
	}
}

// setTOML replaces old with new in dir's ggui.toml.
func setTOML(t *testing.T, dir, old, new string) {
	t.Helper()
	p := filepath.Join(dir, "ggui.toml")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(old)) {
		t.Fatalf("ggui.toml has no %q", old)
	}
	if err := os.WriteFile(p, bytes.Replace(data, []byte(old), []byte(new), 1), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readID returns app.id from dir's ggui.toml.
func readID(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "ggui.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "id = "); ok {
			return strings.Trim(v, `"`)
		}
	}
	t.Fatal("ggui.toml has no app.id")
	return ""
}
