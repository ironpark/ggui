package main

import (
	"bytes"
	"debug/pe"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/installer"
	"github.com/ironpark/ggui/cmd/ggui/internal/macos"
	"github.com/ironpark/ggui/cmd/ggui/internal/scaffold"
	"github.com/ironpark/ggui/cmd/ggui/internal/windows"
)

func TestInstallerBuilderContract(t *testing.T) {
	dir := t.TempDir()
	spec := installer.Spec{Name: "Hello", OutDir: dir}
	for _, tc := range []struct {
		b      installer.Builder
		target string
		want   string
	}{
		{macos.PKGBuilder{}, "macos", filepath.Join(dir, "Hello.pkg")},
		{windows.SetupBuilder{}, "windows", filepath.Join(dir, "Hello-Setup.exe")},
	} {
		if tc.b.Target() != tc.target {
			t.Errorf("%T.Target() = %q, want %q", tc.b, tc.b.Target(), tc.target)
		}
		if got := tc.b.OutputPath(spec); got != tc.want {
			t.Errorf("%T.OutputPath() = %q, want %q", tc.b, got, tc.want)
		}
	}
}

func TestBuildInstallerFlagsValidation(t *testing.T) {
	dir := tiny(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"build", dir, "--target", "web", "--installer"}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "--target macos or --target windows") {
		t.Errorf("--installer on web: exit %d, stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"build", dir, "--target", "macos", "--installer", "--pkg=false"}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "disagree") {
		t.Errorf("conflicting --installer and --pkg: exit %d, stderr=%s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"build", dir, "--target", "windows", "--installer=false", "--installer-pkg", "./cmd/installer"}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "disagree") {
		t.Errorf("conflicting --installer=false and --installer-pkg: exit %d, stderr=%s", code, stderr.String())
	}
}

func TestBuildGoCodeInstallersEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles ggui apps and Go-authored installers")
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
	os.WriteFile(filepath.Join(dir, "LICENSE.txt"), []byte("MIT License — Hello World\n"), 0o644)

	tomlPath := filepath.Join(dir, config.FileName)
	data, _ := os.ReadFile(tomlPath)
	data = bytes.Replace(data, []byte(`# icon = "icon.png"`), []byte(`icon = "icon.png"`), 1)
	data = bytes.Replace(data, []byte(`# company = ""`), []byte(`company = "Example Ltd"`), 1)
	os.WriteFile(tomlPath, data, 0o644)

	// 1. Zero-code Windows per-user installer via --installer flag (no Go installer package).
	t.Run("zero-code windows user installer via flag", func(t *testing.T) {
		out := t.TempDir()
		var stdout, stderr bytes.Buffer
		saved := reportOut
		reportOut = &stdout
		defer func() { reportOut = saved }()
		if code := run([]string{"build", dir, "--target", "windows", "--installer", "--out", out}, &stdout, &stderr); code != 0 {
			t.Fatalf("ggui build --target windows --installer exited %d:\n%s", code, stderr.String())
		}
		exePath := filepath.Join(out, "Hello World.exe")
		setupPath := filepath.Join(out, "Hello World-Setup.exe")
		for _, p := range []string{exePath, setupPath} {
			if !strings.Contains(stdout.String(), p) {
				t.Errorf("stdout missing %s:\n%s", p, stdout.String())
			}
		}
		info := readExe(t, setupPath)
		if info.subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_GUI {
			t.Errorf("setup subsystem %d, want GUI", info.subsystem)
		}
		if info.types[3] != 9 || info.types[14] != 1 || info.types[16] != 1 || info.types[24] != 1 {
			t.Errorf("setup resources by type: %v", info.types)
		}
		if !info.has("Hello World Setup") || !info.has("Hello World-Setup.exe") || !info.has("Example Ltd") {
			t.Error("setup version info does not match expected installer metadata")
		}
		if !bytes.Contains(info.rsrc, []byte(`level="asInvoker"`)) {
			t.Error("user-scope installer manifest does not specify asInvoker")
		}
		if left := sysos(t, dir); len(left) != 0 {
			t.Errorf("leftover .syso files: %v", left)
		}
		if leftDirs, _ := filepath.Glob(filepath.Join(dir, ".ggui_winsetup_*")); len(leftDirs) != 0 {
			t.Errorf("leftover temp installer dirs: %v", leftDirs)
		}
	})

	// 2. Add a user-authored Go installer package at ./cmd/installer — zero TOML config!
	//    Defines machine UAC scope, inline license, custom step marker string,
	//    AfterInstall hook, and macOS .pkg settings all in Go code.
	instDir := filepath.Join(dir, "cmd", "installer")
	if err := os.MkdirAll(instDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const customStepMarker = "CUSTOM_GO_INSTALLER_STEP_MARKER_777"
	instMain := `package main

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/installer"
	"github.com/ironpark/ggui/ui"
)

func main() {
	installer.Main(&installer.App{
		License: "MIT License — Defined In Go\n",
		Windows: installer.WindowsConfig{
			Scope:           installer.ScopeMachine,
			DesktopShortcut: installer.Bool(true),
		},
		MacOS: installer.MacOSConfig{
			InstallLocation: "/Applications",
		},
		Steps: []installer.Step{
			installer.LicenseStep(),
			installer.LocationStep(),
			installer.CustomStep("opts", "` + customStepMarker + `", func(ctx *installer.Context) ggui.Widget {
				return ui.Checkbox(ctx.Bool("add_path", true), "Add to PATH")
			}),
			installer.ProgressStep(),
			installer.FinishStep(),
		},
		AfterInstall: func(ctx *installer.Context) error {
			if ctx.GetBool("add_path") {
				return ctx.AddToUserPath(ctx.Dir())
			}
			return nil
		},
	})
}
`
	if err := os.WriteFile(filepath.Join(instDir, "main.go"), []byte(instMain), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("auto-discovered Go installer: windows machine scope, overlay embed, and --installer=false override", func(t *testing.T) {
		out := t.TempDir()
		var stdout, stderr bytes.Buffer
		if code := run([]string{"build", dir, "--target", "windows", "--out", out}, &stdout, &stderr); code != 0 {
			t.Fatalf("ggui build --target windows exited %d:\n%s", code, stderr.String())
		}
		appInfo := readExe(t, filepath.Join(out, "Hello World.exe"))
		if !bytes.Contains(appInfo.rsrc, []byte(`level="asInvoker"`)) {
			t.Error("app executable should remain asInvoker")
		}
		setupPath := filepath.Join(out, "Hello World-Setup.exe")
		setupInfo := readExe(t, setupPath)
		if !bytes.Contains(setupInfo.rsrc, []byte(`level="requireAdministrator"`)) {
			t.Error("machine-scope setup manifest from Go code should request requireAdministrator")
		}
		setupBytes, err := os.ReadFile(setupPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(setupBytes, []byte(customStepMarker)) {
			t.Error("Hello World-Setup.exe does not contain custom Go installer code")
		}
		if !bytes.Contains(setupBytes, []byte("files/Hello World.exe")) {
			t.Error("Hello World-Setup.exe does not contain overlay-embedded files/Hello World.exe")
		}
		if left := sysos(t, instDir); len(left) != 0 {
			t.Errorf("leftover .syso in user installer dir: %v", left)
		}

		// --installer=false skips Name-Setup.exe even when ./cmd/installer exists.
		outSkip := t.TempDir()
		stdout.Reset()
		stderr.Reset()
		if code := run([]string{"build", dir, "--target", "windows", "--installer=false", "--out", outSkip}, &stdout, &stderr); code != 0 {
			t.Fatalf("ggui build --installer=false exited %d:\n%s", code, stderr.String())
		}
		if _, err := os.Stat(filepath.Join(outSkip, "Hello World-Setup.exe")); !os.IsNotExist(err) {
			t.Error("--installer=false still produced Hello World-Setup.exe")
		}
	})

	t.Run("auto-discovered Go installer produces macos pkg", func(t *testing.T) {
		out := t.TempDir()
		var stdout, stderr bytes.Buffer
		if code := run([]string{"build", dir, "--target", "macos", "--out", out}, &stdout, &stderr); code != 0 {
			t.Fatalf("ggui build --target macos exited %d:\n%s", code, stderr.String())
		}
		pkgPath := filepath.Join(out, "Hello World.pkg")
		if fi, err := os.Stat(pkgPath); err != nil || fi.Size() == 0 {
			t.Fatalf("Go installer package did not produce %s: %v", pkgPath, err)
		}
	})

	t.Run("windows zig renderer bundles ggfx_driver.dll into custom Go setup", func(t *testing.T) {
		out := t.TempDir()
		var stdout, stderr bytes.Buffer
		if code := run([]string{"build", dir, "--target", "windows", "--renderer", "zig", "--out", out}, &stdout, &stderr); code != 0 {
			t.Fatalf("ggui build --target windows --renderer zig exited %d:\n%s", code, stderr.String())
		}
		for _, name := range []string{"Hello World.exe", "ggfx_driver.dll", "Hello World-Setup.exe"} {
			if _, err := os.Stat(filepath.Join(out, name)); err != nil {
				t.Errorf("missing %s: %v", name, err)
			}
		}
		setupBytes, err := os.ReadFile(filepath.Join(out, "Hello World-Setup.exe"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(setupBytes, []byte("files/ggfx_driver.dll")) {
			t.Error("Hello World-Setup.exe embedded payload does not contain files/ggfx_driver.dll")
		}
	})

	t.Run("release dry-run includes Go-authored windows setup exe", func(t *testing.T) {
		out := t.TempDir()
		var stdout, stderr bytes.Buffer
		if code := run([]string{"release", dir, "--targets", "windows", "--arches", "amd64", "--publish", "none", "--dry-run", "--out", out}, &stdout, &stderr); code != 0 {
			t.Fatalf("ggui release --dry-run exited %d:\n%s", code, stderr.String())
		}
		relDir := filepath.Join(out, "release", "0.1.0")
		for _, name := range []string{
			"hello-world-0.1.0-windows-amd64.zip",
			"hello-world-0.1.0-windows-amd64-setup.exe",
			"SHA256SUMS",
		} {
			if fi, err := os.Stat(filepath.Join(relDir, name)); err != nil || fi.Size() == 0 {
				t.Errorf("release asset %s missing or empty: %v", name, err)
			}
		}
	})
}
