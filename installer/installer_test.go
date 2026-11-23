package installer

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/runtime"
)

type fakeBackend struct {
	defaultDir    string
	existingDir   string
	existingVer   string
	startMenuDir  string
	desktopDir    string
	selfExe       string
	shortcuts     map[string]string
	regEntry      *UninstallEntry
	regDeleted    bool
	launched      []string
	notifiedCount int
}

func newFakeBackend(t *testing.T) *fakeBackend {
	t.Helper()
	root := t.TempDir()
	self := filepath.Join(root, "Hello-Setup.exe")
	if err := os.WriteFile(self, []byte("fake-setup-exe"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &fakeBackend{
		defaultDir:   filepath.Join(root, "Programs", "Hello"),
		startMenuDir: filepath.Join(root, "StartMenu"),
		desktopDir:   filepath.Join(root, "Desktop"),
		selfExe:      self,
		shortcuts:    map[string]string{},
	}
}

func (f *fakeBackend) DefaultInstallDir(meta Metadata) string { return f.defaultDir }
func (f *fakeBackend) ExistingInstall(Metadata) (string, string, bool) {
	return f.existingDir, f.existingVer, f.existingDir != ""
}
func (f *fakeBackend) StartMenuDir(string) string { return f.startMenuDir }
func (f *fakeBackend) DesktopDir(string) string   { return f.desktopDir }
func (f *fakeBackend) CreateShortcut(lnkPath, targetPath, _, _, _ string) error {
	f.shortcuts[lnkPath] = targetPath
	if err := os.MkdirAll(filepath.Dir(lnkPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(lnkPath, []byte(targetPath), 0o644)
}
func (f *fakeBackend) RemoveShortcut(lnkPath string) error {
	delete(f.shortcuts, lnkPath)
	_ = os.Remove(lnkPath)
	return nil
}
func (f *fakeBackend) NotifyShell() { f.notifiedCount++ }
func (f *fakeBackend) WriteUninstallEntry(entry UninstallEntry) error {
	cp := entry
	f.regEntry = &cp
	return nil
}
func (f *fakeBackend) DeleteUninstallEntry(Metadata) error {
	f.regEntry = nil
	f.regDeleted = true
	return nil
}
func (f *fakeBackend) Launch(exePath string) error {
	f.launched = append(f.launched, exePath)
	return nil
}
func (f *fakeBackend) SelfExe() (string, error)                        { return f.selfExe, nil }
func (f *fakeBackend) RelaunchForUninstall(string, bool) (bool, error) { return false, nil }
func (f *fakeBackend) ScheduleTempDelete(string)                       {}

func testIconPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img.SetNRGBA(x, y, color.NRGBA{R: 50, G: 120, B: 220, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testPayloadBytes(t *testing.T, license string) []byte {
	t.Helper()
	data, err := PackPayload(Metadata{
		Name:              "Hello",
		ID:                "com.example.hello",
		Version:           "1.2.0",
		Executable:        "Hello.exe",
		Description:       "Friendly test app",
		Publisher:         "Example Ltd",
		Homepage:          "https://example.com",
		Scope:             ScopeUser,
		License:           license,
		StartMenuShortcut: true,
		DesktopShortcut:   false,
		LaunchAfter:       true,
	}, testIconPNG(t), map[string][]byte{
		"Hello.exe":       []byte("hello-exe-bytes"),
		"ggfx_driver.dll": []byte("zig-dll-bytes"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParsePayloadAndArgs(t *testing.T) {
	raw := testPayloadBytes(t, "MIT License")
	p, err := ParsePayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Meta.Name != "Hello" || p.Meta.Version != "1.2.0" || len(p.Files) != 2 || p.Icon == nil {
		t.Fatalf("unexpected payload: %+v, files=%d, icon=%v", p.Meta, len(p.Files), p.Icon != nil)
	}

	// Reject zip-slip paths.
	if _, err := PackPayload(p.Meta, nil, map[string][]byte{"../evil.exe": []byte("x")}); err == nil {
		t.Fatal("PackPayload accepted escaping relative path")
	}

	opts, err := ParseArgs([]string{"/S", `/D=C:\Custom Path\Hello`, "--no-desktop-shortcut", "--no-launch"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Silent || opts.InstallDir != `C:\Custom Path\Hello` || !opts.NoDesktop || !opts.NoLaunch {
		t.Fatalf("unexpected parsed args: %+v", opts)
	}
}

func TestInstallAndUninstallEngine(t *testing.T) {
	fb := newFakeBackend(t)
	raw := testPayloadBytes(t, "")
	p, err := ParsePayload(raw)
	if err != nil {
		t.Fatal(err)
	}

	// Place an unrelated user file in the target folder; Uninstall must keep it.
	if err := os.MkdirAll(fb.defaultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	keepFile := filepath.Join(fb.defaultDir, "user-notes.txt")
	if err := os.WriteFile(keepFile, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stages []float64
	err = Install(t.Context(), p, InstallOptions{
		InstallDir:        fb.defaultDir,
		StartMenuShortcut: true,
		DesktopShortcut:   true,
	}, fb, func(pr Progress) {
		stages = append(stages, pr.Fraction)
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(stages) == 0 || stages[len(stages)-1] != 1.0 {
		t.Errorf("progress stages = %v, want final 1.0", stages)
	}
	for _, name := range []string{"Hello.exe", "ggfx_driver.dll", uninstallerExe, manifestName} {
		if _, err := os.Stat(filepath.Join(fb.defaultDir, name)); err != nil {
			t.Errorf("missing installed file %s: %v", name, err)
		}
	}
	if len(fb.shortcuts) != 2 {
		t.Errorf("created %d shortcuts, want 2: %v", len(fb.shortcuts), fb.shortcuts)
	}
	if fb.regEntry == nil || fb.regEntry.Meta.ID != "com.example.hello" || !strings.Contains(fb.regEntry.QuietUninstallCmd, "--silent") {
		t.Fatalf("unexpected registry entry: %+v", fb.regEntry)
	}

	// Uninstall must remove all installed files & shortcuts while preserving user-notes.txt.
	if err := Uninstall(t.Context(), p.Meta, fb.defaultDir, fb, nil); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	for _, name := range []string{"Hello.exe", "ggfx_driver.dll", uninstallerExe, manifestName} {
		if _, err := os.Stat(filepath.Join(fb.defaultDir, name)); !os.IsNotExist(err) {
			t.Errorf("file %s still exists after uninstall", name)
		}
	}
	if _, err := os.Stat(keepFile); err != nil {
		t.Errorf("unrelated user file was removed: %v", err)
	}
	if len(fb.shortcuts) != 0 || !fb.regDeleted {
		t.Errorf("shortcuts=%v regDeleted=%v, want empty & true", fb.shortcuts, fb.regDeleted)
	}
}

func TestSilentRunInstallAndUninstall(t *testing.T) {
	fb := newFakeBackend(t)
	raw := testPayloadBytes(t, "MIT")
	dir := filepath.Join(t.TempDir(), "MyHello")
	if err := runWithBackend(raw, []string{"--silent", "--dir", dir}, fb); err != nil {
		t.Fatalf("silent install failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Hello.exe")); err != nil {
		t.Fatalf("silent install did not extract Hello.exe: %v", err)
	}
	if err := runWithBackend(raw, []string{"--uninstall", "--silent", "--dir", dir}, fb); err != nil {
		t.Fatalf("silent uninstall failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Hello.exe")); !os.IsNotExist(err) {
		t.Fatalf("silent uninstall left Hello.exe behind")
	}
}

func TestWizardUIInstallAndUninstallFlow(t *testing.T) {
	fb := newFakeBackend(t)
	raw := testPayloadBytes(t, "Copyright (c) 2026 Example Ltd. All rights reserved.")
	p, err := ParsePayload(raw)
	if err != nil {
		t.Fatal(err)
	}

	closed := false
	m := newSetupModel(p, CLIOptions{}, fb.defaultDir, fb)
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		return m.build(probe.Post, probe.Dialogs(), func() { closed = true })
	}, ggui.Sz(540, 390))
	defer probe.Close()

	clickRole := func(pr *ggui.Probe, role ggui.Role, label string) {
		t.Helper()
		f, ok := pr.FindRole(role, label)
		if !ok {
			t.Fatalf("could not find %s %q", role, label)
		}
		pr.Click(f.Center())
	}

	// Step 1: License agreement — Next is disabled (not clickable) until checkbox is ticked.
	probe.Frame()
	if _, ok := probe.FindRole(ggui.RoleButton, "Next"); ok {
		t.Fatalf("expected disabled Next button to not be clickable before license acceptance")
	}
	clickRole(probe, ggui.RoleCheckbox, "I accept the terms in the License Agreement")
	probe.Frame()
	clickRole(probe, ggui.RoleButton, "Next")
	probe.Frame()
	if got := ggui.Peek(m.Step); got != stepConfig {
		t.Fatalf("step after accepting license = %v, want stepConfig", got)
	}

	// Step 2: Config — browse folder via StubFilePicker, then click Install.
	pickedParent := t.TempDir()
	stub := probe.Dialogs().(*runtime.StubFilePicker)
	stub.Paths = []string{pickedParent}
	clickRole(probe, ggui.RoleButton, "Browse…")
	wantDir := filepath.Join(pickedParent, "Hello")
	waitUntil(t, 2*time.Second, func() bool {
		probe.Frame()
		return ggui.Peek(m.InstallDir) == wantDir
	})

	clickRole(probe, ggui.RoleButton, "Install")
	waitUntil(t, 3*time.Second, func() bool {
		probe.Frame()
		return ggui.Peek(m.Step) == stepDone
	})

	// Step 3: Done — Finish launches the app and closes the window.
	clickRole(probe, ggui.RoleButton, "Finish")
	probe.Frame()
	if !closed {
		t.Error("Finish did not close the wizard window")
	}
	wantExe := filepath.Join(wantDir, "Hello.exe")
	if !slices.Contains(fb.launched, wantExe) {
		t.Errorf("launched = %v, want %s", fb.launched, wantExe)
	}

	// Uninstall UI flow.
	fb.existingDir = wantDir
	fb.existingVer = "1.2.0"
	closed = false
	um := newSetupModel(p, CLIOptions{Uninstall: true}, wantDir, fb)
	var uprobe *ggui.Probe
	uprobe = ggui.ProbeBuilder(func() ggui.Widget {
		return um.build(uprobe.Post, uprobe.Dialogs(), func() { closed = true })
	}, ggui.Sz(540, 390))
	defer uprobe.Close()
	uprobe.Frame()
	clickRole(uprobe, ggui.RoleButton, "Uninstall")
	waitUntil(t, 3*time.Second, func() bool {
		uprobe.Frame()
		return ggui.Peek(um.Step) == stepDone
	})
	clickRole(uprobe, ggui.RoleButton, "Finish")
	uprobe.Frame()
	if !closed {
		t.Error("Finish did not close the uninstall window")
	}
	if _, err := os.Stat(wantExe); !os.IsNotExist(err) {
		t.Error("Hello.exe still exists after GUI uninstall")
	}
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}
