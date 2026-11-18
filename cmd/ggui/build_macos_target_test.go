package main

import (
	"bytes"
	"debug/macho"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

// A macOS build signs ad hoc and joins a universal program on any host;
// on a Mac, codesign must accept the bundle and the program must run.
func TestBuildMacOSUniversalAdHoc(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles programs for macOS")
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("CGO_ENABLED", "0")
	dir := tiny(t)
	out := t.TempDir()
	var said, stdout, stderr bytes.Buffer
	saved := ui.Stderr
	ui.Stderr = ui.New(&said)
	defer func() { ui.Stderr = saved }()
	if code := run([]string{"build", dir, "--target", "macos", "--universal", "--out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("ggui build exited %d:\n%s%s", code, said.String(), stderr.String())
	}
	if !strings.Contains(said.String(), "Signed Tiny.app ad hoc") {
		t.Errorf("ggui build said:\n%s", said.String())
	}
	app := filepath.Join(out, "Tiny.app")
	exes, _ := filepath.Glob(filepath.Join(app, "Contents", "MacOS", "*"))
	if len(exes) != 1 {
		t.Fatalf("Contents/MacOS holds %v", exes)
	}
	exe := exes[0]
	f, err := macho.OpenFat(exe)
	if err != nil {
		t.Fatalf("not a universal program: %v", err)
	}
	var cpus []string
	for _, a := range f.Arches {
		cpus = append(cpus, a.Cpu.String())
	}
	f.Close()
	if strings.Join(cpus, " ") != "CpuAmd64 CpuArm64" {
		t.Errorf("slices %v", cpus)
	}
	if _, err := os.Stat(filepath.Join(app, "Contents", "_CodeSignature", "CodeResources")); err != nil {
		t.Error(err)
	}
	if runtime.GOOS != "darwin" {
		return
	}
	if msg, err := exec.Command("codesign", "--verify", "--strict", "--deep", "-vv", app).CombinedOutput(); err != nil {
		t.Errorf("codesign --verify: %s", msg)
	}
	if runtime.GOARCH == "arm64" {
		// The kernel kills a program whose signature does not hold.
		if msg, err := exec.Command(exe).CombinedOutput(); err != nil {
			t.Errorf("running %s: %v %s", exe, err, msg)
		}
	}
}
