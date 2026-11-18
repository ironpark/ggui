package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/scaffold"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
	"github.com/ironpark/ggui/cmd/ggui/internal/windows"
)

// exeInfo is what a test reads back from a Windows program.
type exeInfo struct {
	subsystem uint16
	types     map[uint32]int // resource type: how many IDs it has
	rsrc      []byte         // the .rsrc section, to search
}

func readExe(t *testing.T, path string) exeInfo {
	t.Helper()
	f, err := pe.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var info exeInfo
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		info.subsystem = oh.Subsystem
	case *pe.OptionalHeader64:
		info.subsystem = oh.Subsystem
	}
	info.types = map[uint32]int{}
	s := f.Section(".rsrc")
	if s == nil {
		return info
	}
	data, err := s.Data()
	if err != nil {
		t.Fatal(err)
	}
	info.rsrc = data
	le := binary.LittleEndian
	// The root directory's entries are the types, each pointing at a
	// directory of IDs; internal/windows walks the whole tree.
	n := int(le.Uint16(data[12:])) + int(le.Uint16(data[14:]))
	for i := range n {
		e := 16 + 8*i
		sub := le.Uint32(data[e+4:]) &^ 0x80000000
		info.types[le.Uint32(data[e:])] = int(le.Uint16(data[sub+12:])) + int(le.Uint16(data[sub+14:]))
	}
	return info
}

// has reports whether the .rsrc section holds s as UTF-16, as a version
// resource's strings are.
func (i exeInfo) has(s string) bool {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u))
	for j, c := range u {
		binary.LittleEndian.PutUint16(b[2*j:], c)
	}
	return bytes.Contains(i.rsrc, b)
}

func writeIcon(t *testing.T, path string, size int) {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.SetNRGBA(0, 0, color.NRGBA{})
	var buf bytes.Buffer
	png.Encode(&buf, img)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sysos lists the .syso files in dir.
func sysos(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "*.syso"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// buildEnv has the go command build what the test wrote, alone: no
// workspace of the checkout's, and amd64 whatever the host.
func buildEnv(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOARCH", "amd64")
	t.Setenv("CGO_ENABLED", "0")
}

// The app ggui init writes must build for Windows with its icon, version
// information and manifest, as a GUI program, and leave its sources as
// they were.
func TestBuildWindowsInitApp(t *testing.T) {
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
		t.Fatal(err)
	}
	writeIcon(t, filepath.Join(dir, "icon.png"), 1024)
	toml := filepath.Join(dir, config.FileName)
	data, _ := os.ReadFile(toml)
	data = bytes.Replace(data, []byte(`# icon = "icon.png"`), []byte(`icon = "icon.png"`), 1)
	data = bytes.Replace(data, []byte(`# company = ""`), []byte(`company = "Example Ltd"`), 1)
	os.WriteFile(toml, data, 0o644)

	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"build", dir, "--target", "windows", "--out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("ggui build exited %d:\n%s", code, stderr.String())
	}
	info := readExe(t, filepath.Join(out, "Hello World.exe"))
	if info.subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_GUI {
		t.Errorf("subsystem %d, want GUI", info.subsystem)
	}
	// RT_ICON's nine sizes, one RT_GROUP_ICON, RT_VERSION and RT_MANIFEST.
	if info.types[3] != 9 || info.types[14] != 1 || info.types[16] != 1 || info.types[24] != 1 {
		t.Errorf("resources by type: %v", info.types)
	}
	if !info.has("Example Ltd") || !info.has("Hello World.exe") || !bytes.Contains(info.rsrc, []byte("PerMonitorV2")) {
		t.Error("the version information or the manifest is not what ggui.toml says")
	}
	if left := sysos(t, dir); len(left) != 0 {
		t.Errorf("the build left %v in the sources", left)
	}
}

// tiny writes a module with an empty main package and a ggui.toml naming
// it, which builds in a moment.
func tiny(t *testing.T) string {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/tiny\n\ngo 1.27\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, config.FileName), []byte("[app]\nname = \"Tiny\"\nid = \"com.example.tiny\"\n"), 0o644)
	return dir
}

// buildWindows runs ggui build --target windows on dir and returns the
// program's path, the exit status and what ggui said: its steps and
// notes, which go to ui.Stderr, then its error.
func buildWindows(t *testing.T, dir string, args ...string) (string, int, string) {
	t.Helper()
	out := t.TempDir()
	var said, stdout, stderr bytes.Buffer
	saved := ui.Stderr
	ui.Stderr = ui.New(&said)
	defer func() { ui.Stderr = saved }()
	code := run(append([]string{"build", dir, "--target", "windows", "--out", out}, args...), &stdout, &stderr)
	return filepath.Join(out, "Tiny.exe"), code, said.String() + stderr.String()
}

func TestBuildWindowsResourceFile(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles programs for Windows")
	}
	buildEnv(t)

	t.Run("console", func(t *testing.T) {
		dir := tiny(t)
		exe, code, stderr := buildWindows(t, dir, "--console")
		if code != 0 {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if info := readExe(t, exe); info.subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_CUI || info.types[16] != 1 {
			t.Errorf("--console: subsystem %d, resources %v", info.subsystem, info.types)
		}
	})

	t.Run("leftover replaced", func(t *testing.T) {
		dir := tiny(t)
		left := filepath.Join(dir, windows.SysoName("amd64"))
		os.WriteFile(left, []byte("what a killed build left"), 0o644)
		exe, code, stderr := buildWindows(t, dir)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if !strings.Contains(stderr, "left behind") {
			t.Errorf("the leftover was not mentioned:\n%s", stderr)
		}
		if info := readExe(t, exe); info.subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_GUI || info.types[24] != 1 {
			t.Errorf("subsystem %d, resources %v", info.subsystem, info.types)
		}
		if s := sysos(t, dir); len(s) != 0 {
			t.Errorf("left %v", s)
		}
	})

	// A package with resources of its own keeps them: the linker takes
	// one resource section, so ggui adds none, and leaves the file.
	t.Run("own resources kept", func(t *testing.T) {
		dir := tiny(t)
		obj, _, err := windows.Object(windows.Resources{Arch: "amd64", Name: "Theirs", Executable: "tiny", ID: "com.example.theirs", Version: "9.9"})
		if err != nil {
			t.Fatal(err)
		}
		theirs := filepath.Join(dir, "rsrc_windows_amd64.syso")
		os.WriteFile(theirs, obj, 0o644)
		exe, code, stderr := buildWindows(t, dir)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if !strings.Contains(stderr, "rsrc_windows_amd64.syso") {
			t.Errorf("the package's own resources were not mentioned:\n%s", stderr)
		}
		info := readExe(t, exe)
		if !info.has("Theirs.exe") || info.has("Tiny.exe") {
			t.Error("the program does not have the package's own resources")
		}
		if s := sysos(t, dir); len(s) != 1 || s[0] != theirs {
			t.Errorf(".syso files after the build: %v, want only theirs", s)
		}
	})

	t.Run("failed build cleans up", func(t *testing.T) {
		dir := tiny(t)
		os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { undefined() }\n"), 0o644)
		if _, code, stderr := buildWindows(t, dir); code == 0 || !strings.Contains(stderr, "undefined") {
			t.Errorf("exit %d: %s", code, stderr)
		}
		if s := sysos(t, dir); len(s) != 0 {
			t.Errorf("a failed build left %v", s)
		}
	})

	t.Run("console for another target", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"build", tiny(t), "--target", "web", "--console"}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "--target windows") {
			t.Errorf("exit %d: %s", code, stderr.String())
		}
	})
}
