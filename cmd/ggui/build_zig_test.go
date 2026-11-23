package main

import (
	"archive/zip"
	"bytes"
	"debug/macho"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/linux/linuxtest"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
	"github.com/ironpark/ggui/cmd/ggui/internal/ziglib"
)

// zigModule creates a minimal Go module requiring github.com/ironpark/ggfx v0.1.0
// (from the module cache, or the local checkout when available). If initHost is
// true, main prints the active ggfx driver implementation and graphics library
// via host.Init(nil).
func zigModule(t *testing.T, initHost bool) string {
	t.Helper()
	if testing.Short() {
		t.Skip("cross-compiles programs with ggfx v0.1.0")
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("CGO_ENABLED", "0")

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	sumData, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Skipf("cannot read root go.sum: %v", err)
	}

	dir := t.TempDir()
	goMod := "module example.com/zigtiny\n\ngo 1.27\n\nrequire github.com/ironpark/ggfx v0.1.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), sumData, 0o644); err != nil {
		t.Fatal(err)
	}
	mainSrc := "package main\n\nimport _ \"github.com/ironpark/ggfx/host\"\n\nfunc main() {}\n"
	if initHost {
		mainSrc = `package main

import (
	"fmt"
	"os"

	"github.com/ironpark/ggfx/host"
)

func main() {
	if err := host.Init(nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer host.Terminate()
	fmt.Println(host.DriverImplementation(), host.CurrentDriver().Library)
}
`
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	toml := "[app]\nname = \"ZigTiny\"\nid = \"com.example.zigtiny\"\n"
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	// Resolve transitive go.mod requirements (e.g. purego) from the local cache.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Skipf("go mod tidy against ggfx v0.1.0 unavailable in this environment: %v\n%s", err, out)
	}
	return dir
}

func runBuildCapture(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var said, stdout, stderr bytes.Buffer
	saved := ui.Stderr
	ui.Stderr = ui.New(&said)
	defer func() { ui.Stderr = saved }()
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), said.String() + stderr.String()
}

func TestBuildMacOSUniversalZigRenderer(t *testing.T) {
	dir := zigModule(t, true)
	out := t.TempDir()
	code, _, said := runBuildCapture(t, "build", dir, "--target", "macos", "--universal", "--renderer", "zig", "--out", out)
	if code != 0 {
		t.Fatalf("ggui build exited %d:\n%s", code, said)
	}
	if !strings.Contains(said, "ggfxzig,ggfxnoembed") {
		t.Errorf("build command did not pass ggfxzig,ggfxnoembed:\n%s", said)
	}

	app := filepath.Join(out, "ZigTiny.app")
	dylib := filepath.Join(app, "Contents", "Frameworks", ziglib.FileName("darwin"))
	f, err := macho.OpenFat(dylib)
	if err != nil {
		t.Fatalf("Contents/Frameworks/%s is not a universal Mach-O: %v", ziglib.FileName("darwin"), err)
	}
	var cpus []string
	for _, a := range f.Arches {
		cpus = append(cpus, a.Cpu.String())
	}
	f.Close()
	if strings.Join(cpus, " ") != "CpuAmd64 CpuArm64" {
		t.Errorf("dylib slices = %v, want [CpuAmd64 CpuArm64]", cpus)
	}

	if runtime.GOOS != "darwin" {
		return
	}
	if msg, err := exec.Command("codesign", "--verify", "--strict", "--deep", "-vv", app).CombinedOutput(); err != nil {
		t.Fatalf("codesign --verify failed:\n%s", msg)
	}
	exe := filepath.Join(app, "Contents", "MacOS", filepath.Base(dir))
	msg, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatalf("running bundled macOS Zig binary failed: %v\n%s", err, msg)
	}
	if got := strings.TrimSpace(string(msg)); got != "zig Metal" {
		t.Errorf("bundled app reported %q, want %q", got, "zig Metal")
	}
}

func TestBuildWindowsZigRendererAndReleaseZip(t *testing.T) {
	dir := zigModule(t, false)
	t.Setenv("GOARCH", "amd64")

	// 1. Direct ggui build --target windows --renderer zig bundles ggfx_driver.dll beside Name.exe.
	out := t.TempDir()
	code, _, said := runBuildCapture(t, "build", dir, "--target", "windows", "--renderer", "zig", "--out", out)
	if code != 0 {
		t.Fatalf("ggui build windows exited %d:\n%s", code, said)
	}
	if !strings.Contains(said, "ggfxzig,ggfxnoembed") {
		t.Errorf("windows build did not pass ggfxzig,ggfxnoembed:\n%s", said)
	}
	dll := filepath.Join(out, ziglib.FileName("windows"))
	if fi, err := os.Stat(dll); err != nil || fi.Size() == 0 {
		t.Fatalf("expected %s beside exe: %v", dll, err)
	}

	// 2. Unsupported arch (386) warns and falls back to the Go renderer.
	t.Setenv("GOARCH", "386")
	out386 := t.TempDir()
	code, _, said386 := runBuildCapture(t, "build", dir, "--target", "windows", "--renderer", "zig", "--out", out386)
	if code != 0 {
		t.Fatalf("ggui build windows/386 exited %d:\n%s", code, said386)
	}
	if !strings.Contains(said386, "not available for windows/386") {
		t.Errorf("expected fallback warning for windows/386:\n%s", said386)
	}
	if _, err := os.Stat(filepath.Join(out386, ziglib.FileName("windows"))); err == nil {
		t.Error("unexpected ggfx_driver.dll for windows/386")
	}

	// 3. ggui release --dry-run includes ggfx_driver.dll in the Windows zip.
	t.Setenv("GOARCH", "amd64")
	relOut := t.TempDir()
	code, _, relSaid := runBuildCapture(t, "release", dir, "--dry-run", "--targets", "windows", "--arches", "amd64", "--publish", "none", "--renderer", "zig", "--out", relOut)
	if code != 0 {
		t.Fatalf("ggui release exited %d:\n%s", code, relSaid)
	}
	zipPath := filepath.Join(relOut, "release", "0.1.0", "zigtiny-0.1.0-windows-amd64.zip")
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("opening release zip %s: %v", zipPath, err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	want := []string{"ZigTiny.exe", "ggfx_driver.dll"}
	if !slices.Equal(names, want) {
		t.Errorf("windows release zip contains %v, want %v", names, want)
	}
}

func TestBuildLinuxZigRenderer(t *testing.T) {
	dir := zigModule(t, false)
	t.Setenv("GOARCH", "amd64")

	// Also verify auto-detection via manual --tags ggfxzig without --renderer.
	out := t.TempDir()
	code, _, said := runBuildCapture(t, "build", dir, "--target", "linux", "--format", "tar,deb,rpm", "--tags", "ggfxzig", "--out", out)
	if code != 0 {
		t.Fatalf("ggui build linux exited %d:\n%s", code, said)
	}
	if !strings.Contains(said, "ggfxzig,ggfxnoembed") {
		t.Errorf("linux build did not add ggfxnoembed:\n%s", said)
	}
	wantLDFlag := ziglib.LibraryPathLDFlag("../lib/zigtiny/libggfx_driver.so")
	if !strings.Contains(said, wantLDFlag) {
		t.Errorf("linux build lacked LibraryPath ldflag %q:\n%s", wantLDFlag, said)
	}

	wantLib := "usr/lib/zigtiny/libggfx_driver.so"
	gotTar := tarNames(t, filepath.Join(out, "zigtiny-0.1.0-linux-amd64.tar.gz"))
	if !slices.Contains(gotTar, wantLib) {
		t.Errorf("tarball %v missing %s", gotTar, wantLib)
	}

	deb, err := linuxtest.ReadDeb(filepath.Join(out, "zigtiny_0.1.0_amd64.deb"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := deb.Files["/"+wantLib]; !ok {
		t.Errorf("deb missing /%s", wantLib)
	}
	if !strings.Contains(deb.Control["Depends"], config.VulkanDependsDeb) {
		t.Errorf("deb Depends = %q, want %s", deb.Control["Depends"], config.VulkanDependsDeb)
	}

	rpm, err := linuxtest.ReadRPM(filepath.Join(out, "zigtiny-0.1.0-1.x86_64.rpm"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rpm.Files["/"+wantLib]; !ok {
		t.Errorf("rpm missing /%s", wantLib)
	}
	if !slices.Contains(rpm.Requires, config.VulkanDependsRPM) {
		t.Errorf("rpm Requires = %v, want %s", rpm.Requires, config.VulkanDependsRPM)
	}

	// Explicit --renderer go overrides build.renderer = "zig".
	setTOML(t, dir, `id = "com.example.zigtiny"`, "id = \"com.example.zigtiny\"\n\n[build]\nrenderer = \"zig\"")
	outGo := t.TempDir()
	code, _, saidGo := runBuildCapture(t, "build", dir, "--target", "linux", "--renderer", "go", "--out", outGo)
	if code != 0 {
		t.Fatalf("ggui build --renderer go exited %d:\n%s", code, saidGo)
	}
	if strings.Contains(saidGo, "ggfxzig") {
		t.Errorf("--renderer go still passed ggfxzig:\n%s", saidGo)
	}
	if slices.Contains(tarNames(t, filepath.Join(outGo, "zigtiny-0.1.0-linux-amd64.tar.gz")), wantLib) {
		t.Errorf("--renderer go unexpectedly bundled %s", wantLib)
	}
}
