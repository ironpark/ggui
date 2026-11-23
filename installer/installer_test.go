package installer

import (
	"bytes"
	"encoding/json"
	"errors"
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
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

type fakeBackend struct {
	*PreviewBackend
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
	pb := NewPreviewBackend()
	t.Cleanup(func() { _ = pb.Close() })
	return &fakeBackend{
		PreviewBackend: pb,
		defaultDir:     filepath.Join(root, "Programs", "Hello"),
		startMenuDir:   filepath.Join(root, "StartMenu"),
		desktopDir:     filepath.Join(root, "Desktop"),
		selfExe:        self,
		shortcuts:      map[string]string{},
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

func TestSilentRunInstallAndUninstallWithHooks(t *testing.T) {
	fb := newFakeBackend(t)
	raw := testPayloadBytes(t, "MIT")
	dir := filepath.Join(t.TempDir(), "MyHello")

	var events []string
	app := &App{
		Payload: raw,
		BeforeInstall: func(ctx *Context) error {
			events = append(events, "before-install:"+ctx.Dir())
			return nil
		},
		AfterInstall: func(ctx *Context) error {
			events = append(events, "after-install")
			if err := ctx.WriteFile("conf/defaults.json", []byte(`{"ok":true}`), 0o644); err != nil {
				return err
			}
			return ctx.AddToUserPath(ctx.Dir())
		},
		BeforeUninstall: func(ctx *Context) error {
			events = append(events, "before-uninstall")
			return ctx.RemoveFromUserPath(ctx.Dir())
		},
		AfterUninstall: func(ctx *Context) error {
			events = append(events, "after-uninstall")
			return nil
		},
	}

	if err := runWithBackend(app, []string{"--silent", "--dir", dir}, fb); err != nil {
		t.Fatalf("silent install failed: %v", err)
	}
	for _, rel := range []string{"Hello.exe", "conf/defaults.json"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("silent install missing %s: %v", rel, err)
		}
	}
	if !slices.Contains(fb.PathDirs(ScopeUser), dir) {
		t.Fatalf("AddToUserPath did not record %s: %v", dir, fb.PathDirs(ScopeUser))
	}

	if err := runWithBackend(app, []string{"--uninstall", "--silent", "--dir", dir}, fb); err != nil {
		t.Fatalf("silent uninstall failed: %v", err)
	}
	for _, rel := range []string{"Hello.exe", "conf/defaults.json"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("silent uninstall left %s behind", rel)
		}
	}
	if len(fb.PathDirs(ScopeUser)) != 0 {
		t.Fatalf("RemoveFromUserPath left entries: %v", fb.PathDirs(ScopeUser))
	}
	wantEvents := []string{"before-install:" + dir, "after-install", "before-uninstall", "after-uninstall"}
	if !slices.Equal(events, wantEvents) {
		t.Errorf("hook events = %v, want %v", events, wantEvents)
	}
}

func TestAfterInstallErrorRollsBack(t *testing.T) {
	fb := newFakeBackend(t)
	raw := testPayloadBytes(t, "")
	dir := filepath.Join(t.TempDir(), "RollbackTest")
	app := &App{
		Payload: raw,
		AfterInstall: func(ctx *Context) error {
			if err := ctx.WriteFile("extra.txt", []byte("temp"), 0o644); err != nil {
				return err
			}
			return errors.New("post-install boom")
		},
	}
	err := runWithBackend(app, []string{"--silent", "--dir", dir}, fb)
	if err == nil || !strings.Contains(err.Error(), "post-install boom") {
		t.Fatalf("expected post-install error, got %v", err)
	}
	for _, name := range []string{"Hello.exe", "extra.txt", manifestName} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("rollback left %s behind", name)
		}
	}
	if len(fb.shortcuts) != 0 || fb.regEntry != nil {
		t.Errorf("rollback left shortcuts=%v reg=%v", fb.shortcuts, fb.regEntry)
	}
}

func TestDescribeProtocol(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "describe.json")
	t.Setenv(DescribeEnv, outPath)

	app := &App{
		Name:      "Acme",
		License:   "Apache 2.0",
		Publisher: "Acme Corp",
		Homepage:  "https://acme.example",
		Windows: WindowsConfig{
			Scope:             ScopeMachine,
			InstallDir:        `%ProgramFiles%\Acme`,
			DesktopShortcut:   Bool(true),
			LaunchAfter:       Bool(false),
			StartMenuShortcut: Bool(true),
		},
		MacOS: MacOSConfig{
			SigningIdentity: "Developer ID Installer: Acme",
			InstallLocation: "/Applications/Acme",
			Scripts:         "scripts/pkg",
		},
	}
	if err := Run(app, nil); err != nil {
		t.Fatalf("Run with DescribeEnv failed: %v", err)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var desc Description
	if err := json.Unmarshal(raw, &desc); err != nil {
		t.Fatal(err)
	}
	if desc.V != describeProtocol || desc.Name != "Acme" || desc.License != "Apache 2.0" {
		t.Fatalf("unexpected top-level description: %+v", desc)
	}
	if !desc.Windows.Enabled || desc.Windows.Scope != ScopeMachine || desc.Windows.Publisher != "Acme Corp" ||
		desc.Windows.URL != "https://acme.example" || !desc.Windows.StartMenuShortcut || !desc.Windows.DesktopShortcut || desc.Windows.LaunchAfter {
		t.Fatalf("unexpected windows description: %+v", desc.Windows)
	}
	if !desc.MacOS.Enabled || desc.MacOS.SigningIdentity != "Developer ID Installer: Acme" ||
		desc.MacOS.InstallLocation != "/Applications/Acme" || desc.MacOS.Scripts != "scripts/pkg" || desc.MacOS.License != "Apache 2.0" {
		t.Fatalf("unexpected macos description: %+v", desc.MacOS)
	}
}

func TestWizardUIInstallAndUninstallFlow(t *testing.T) {
	fb := newFakeBackend(t)
	raw := testPayloadBytes(t, "Copyright (c) 2026 Example Ltd. All rights reserved.")

	customNote := ggui.State("hello-note")
	app := &App{
		Payload: raw,
		Steps: []Step{
			LicenseStep(),
			LocationStep(),
			CustomStep("options", "Additional Options", func(ctx *Context) ggui.Widget {
				return widget.Column(
					ui.Checkbox(ctx.Bool("add_path", false), "Add CLI to user PATH"),
				).Space(0.75)
			}),
			ProgressStep(),
			FinishStep(),
		},
		AfterInstall: func(ctx *Context) error {
			note := Value(ctx, customNote)
			if err := ctx.WriteFile("note.txt", []byte(note), 0o644); err != nil {
				return err
			}
			if ctx.GetBool("add_path") {
				return ctx.AddToUserPath(ctx.Dir())
			}
			return nil
		},
	}

	ctx, err := NewContext(app, nil, fb)
	if err != nil {
		t.Fatal(err)
	}

	closed := false
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		return ctx.BuildWidget(probe.Post, probe.Dialogs(), func() { closed = true })
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

	// Step 1: License agreement — Next is disabled until checkbox is ticked.
	probe.Frame()
	if _, ok := probe.FindRole(ggui.RoleButton, "Next"); ok {
		t.Fatalf("expected disabled Next button to not be clickable before license acceptance")
	}
	clickRole(probe, ggui.RoleCheckbox, "I accept the terms in the License Agreement")
	probe.Frame()
	clickRole(probe, ggui.RoleButton, "Next")
	probe.Frame()
	if cur, _ := ctx.CurrentStep(); cur.ID != "location" {
		t.Fatalf("step after accepting license = %q, want location", cur.ID)
	}

	// Step 2: Location — browse folder via StubFilePicker, then click Next (to custom options step).
	pickedParent := t.TempDir()
	stub := probe.Dialogs().(*runtime.StubFilePicker)
	stub.Paths = []string{pickedParent}
	clickRole(probe, ggui.RoleButton, "Browse…")
	wantDir := filepath.Join(pickedParent, "Hello")
	waitUntil(t, 2*time.Second, func() bool {
		probe.Frame()
		return ggui.Peek(ctx.InstallDir) == wantDir
	})
	clickRole(probe, ggui.RoleButton, "Next")
	probe.Frame()
	if cur, _ := ctx.CurrentStep(); cur.ID != "options" {
		t.Fatalf("step after location = %q, want options", cur.ID)
	}

	// Step 3: Custom Options step — toggle PATH checkbox and click Install (smart footer label!).
	clickRole(probe, ggui.RoleCheckbox, "Add CLI to user PATH")
	probe.Frame()
	clickRole(probe, ggui.RoleButton, "Install")
	waitUntil(t, 3*time.Second, func() bool {
		probe.Frame()
		cur, _ := ctx.CurrentStep()
		return cur.Kind == StepFinish
	})

	if !slices.Contains(fb.PathDirs(ScopeUser), wantDir) {
		t.Errorf("custom step + AfterInstall did not add %s to PATH: %v", wantDir, fb.PathDirs(ScopeUser))
	}

	// Step 4: Finish launches the app and closes the window.
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
	uctx, err := NewContext(app, []string{"--uninstall"}, fb)
	if err != nil {
		t.Fatal(err)
	}
	var uprobe *ggui.Probe
	uprobe = ggui.ProbeBuilder(func() ggui.Widget {
		return uctx.BuildWidget(uprobe.Post, uprobe.Dialogs(), func() { closed = true })
	}, ggui.Sz(540, 390))
	defer uprobe.Close()
	uprobe.Frame()
	clickRole(uprobe, ggui.RoleButton, "Uninstall")
	waitUntil(t, 3*time.Second, func() bool {
		uprobe.Frame()
		cur, _ := uctx.CurrentStep()
		return cur.Kind == StepFinish
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

func TestCustomBuildUIAndPreviewMode(t *testing.T) {
	// Ensure no global embedded payload leaks from other tests.
	SetEmbeddedPayload(nil)

	app := &App{
		Name:    "PreviewApp",
		Version: "2.0.0",
		BuildUI: func(ctx *Context) ggui.Widget {
			return widget.Column(
				ui.Title("One-Click "+ctx.Meta().Name),
				ui.Button("Install Now", ctx.StartInstall),
				ui.Button("Done", ctx.Finish),
			).Space(1)
		},
	}
	ctx, err := NewContext(app, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pb, ok := ctx.Backend().(*PreviewBackend); ok {
		defer pb.Close()
	}
	if !ctx.IsPreview() {
		t.Fatal("expected IsPreview() == true when no payload is embedded")
	}

	closed := false
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		return ctx.BuildWidget(probe.Post, probe.Dialogs(), func() { closed = true })
	}, ggui.Sz(540, 390))
	defer probe.Close()

	probe.Frame()
	btn, ok := probe.FindRole(ggui.RoleButton, "Install Now")
	if !ok {
		t.Fatal("custom BuildUI Install Now button not found")
	}
	probe.Click(btn.Center())
	waitUntil(t, 3*time.Second, func() bool {
		probe.Frame()
		return ggui.Peek(ctx.Progress) == 1.0
	})

	doneBtn, ok := probe.FindRole(ggui.RoleButton, "Done")
	if !ok {
		t.Fatal("custom BuildUI Done button not found")
	}
	probe.Click(doneBtn.Center())
	probe.Frame()
	if !closed {
		t.Error("Done button did not close window")
	}
}

func TestModifyPathList(t *testing.T) {
	got, changed := modifyPathList(`C:\Windows;C:\Tools\`, `C:\Apps\MyApp`, true)
	if !changed || got != `C:\Windows;C:\Tools\;C:\Apps\MyApp` {
		t.Fatalf("add new: got=%q changed=%v", got, changed)
	}
	// Duplicate detection is case-insensitive and ignores trailing slashes.
	if _, changed := modifyPathList(got, `c:\apps\myapp\`, true); changed {
		t.Error("expected duplicate add to report changed=false")
	}
	got, changed = modifyPathList(got, `c:\apps\myapp`, false)
	if !changed || got != `C:\Windows;C:\Tools\` {
		t.Fatalf("remove: got=%q changed=%v", got, changed)
	}
	if _, changed := modifyPathList(got, `C:\Missing`, false); changed {
		t.Error("expected removing missing path to report changed=false")
	}
}

func TestDeclarativeEnvPathAndRegistryLifecycle(t *testing.T) {
	fb := newFakeBackend(t)
	raw := testPayloadBytes(t, "")
	dir := filepath.Join(t.TempDir(), "Hello")

	// Pre-populate a shared Run key owned by another application; Uninstall must preserve it.
	if err := fb.SetRegistryValue(RegistryHKCU, `Software\Microsoft\Windows\CurrentVersion\Run`, "OtherApp", StringValue(`C:\Other.exe`)); err != nil {
		t.Fatal(err)
	}

	app := &App{
		Payload: raw,
		Windows: WindowsConfig{
			Scope:     ScopeUser,
			AddToPath: true,
			Env: map[string]string{
				"HELLO_HOME":    "%INSTALL_DIR%",
				"HELLO_VERSION": "%VERSION%",
			},
			Registry: []RegistryEntry{
				{Key: `Software\Example\Hello`, Name: "InstallDir", Value: StringValue("%INSTALL_DIR%")},
				{Key: `Software\Example\Hello`, Name: "ExeExpand", Value: ExpandStringValue("%EXE_PATH%")},
				{Key: `Software\Example\Hello`, Name: "AutoUpdate", Value: DWORDValue(1)},
				{Key: `Software\Example\Hello`, Name: "BuildSeq", Value: QWORDValue(42)},
				{Key: `Software\Example\Hello`, Name: "Raw", Value: BinaryValue([]byte{1, 2, 3})},
				{Key: `Software\Example\Hello`, Name: "Tags", Value: MultiStringValue([]string{"%APP_NAME%", "%APP_ID%"})},
				{Root: RegistryHKCU, Key: `Software\Microsoft\Windows\CurrentVersion\Run`, Name: "Hello", Value: StringValue(`"%EXE_PATH%"`)},
			},
		},
	}

	if err := runWithBackend(app, []string{"--silent", "--dir", dir}, fb); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Verify PATH, env vars, and all typed registry values with token expansion.
	if !slices.Contains(fb.PathDirs(ScopeUser), dir) {
		t.Fatalf("PATH missing %s: %v", dir, fb.PathDirs(ScopeUser))
	}
	if v, ok, err := fb.GetEnv(ScopeUser, "HELLO_HOME"); err != nil || !ok || v != dir {
		t.Errorf("HELLO_HOME = (%q, %v, %v), want %q", v, ok, err, dir)
	}
	if v, ok, err := fb.GetEnv(ScopeUser, "HELLO_VERSION"); err != nil || !ok || v != "1.2.0" {
		t.Errorf("HELLO_VERSION = (%q, %v, %v), want 1.2.0", v, ok, err)
	}
	if rv, ok, _ := fb.GetRegistryValue(RegistryHKCU, `Software\Example\Hello`, "InstallDir"); !ok || rv.Type != RegSZ || rv.String != dir {
		t.Errorf("InstallDir reg = (%+v, %v)", rv, ok)
	}
	wantExe := filepath.Join(dir, "Hello.exe")
	if rv, ok, _ := fb.GetRegistryValue(RegistryHKCU, `Software\Example\Hello`, "ExeExpand"); !ok || rv.Type != RegExpandSZ || rv.String != wantExe {
		t.Errorf("ExeExpand reg = (%+v, %v)", rv, ok)
	}
	if rv, ok, _ := fb.GetRegistryValue(RegistryHKCU, `Software\Example\Hello`, "AutoUpdate"); !ok || rv.Type != RegDWORD || rv.DWORD != 1 {
		t.Errorf("AutoUpdate reg = (%+v, %v)", rv, ok)
	}
	if rv, ok, _ := fb.GetRegistryValue(RegistryHKCU, `Software\Example\Hello`, "BuildSeq"); !ok || rv.Type != RegQWORD || rv.QWORD != 42 {
		t.Errorf("BuildSeq reg = (%+v, %v)", rv, ok)
	}
	if rv, ok, _ := fb.GetRegistryValue(RegistryHKCU, `Software\Example\Hello`, "Raw"); !ok || rv.Type != RegBinary || !bytes.Equal(rv.Binary, []byte{1, 2, 3}) {
		t.Errorf("Raw reg = (%+v, %v)", rv, ok)
	}
	if rv, ok, _ := fb.GetRegistryValue(RegistryHKCU, `Software\Example\Hello`, "Tags"); !ok || rv.Type != RegMultiSZ || !slices.Equal(rv.Multi, []string{"Hello", "com.example.hello"}) {
		t.Errorf("Tags reg = (%+v, %v)", rv, ok)
	}

	// Uninstall with NO custom BeforeUninstall hook must clean up all tracked env/PATH/registry
	// entries and delete the newly created key, while preserving OtherApp in the shared Run key.
	if err := runWithBackend(&App{Payload: raw}, []string{"--uninstall", "--silent", "--dir", dir}, fb); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	if len(fb.PathDirs(ScopeUser)) != 0 {
		t.Errorf("PATH still has entries after uninstall: %v", fb.PathDirs(ScopeUser))
	}
	if _, ok, _ := fb.GetEnv(ScopeUser, "HELLO_HOME"); ok {
		t.Error("HELLO_HOME still exists after uninstall")
	}
	if exists, _ := fb.RegistryKeyExists(RegistryHKCU, `Software\Example\Hello`); exists {
		t.Error("newly created Software\\Example\\Hello key was not removed after uninstall")
	}
	if _, ok, _ := fb.GetRegistryValue(RegistryHKCU, `Software\Microsoft\Windows\CurrentVersion\Run`, "Hello"); ok {
		t.Error("Hello Run entry still exists after uninstall")
	}
	if rv, ok, _ := fb.GetRegistryValue(RegistryHKCU, `Software\Microsoft\Windows\CurrentVersion\Run`, "OtherApp"); !ok || rv.String != `C:\Other.exe` {
		t.Errorf("unrelated OtherApp entry in shared Run key was damaged: (%+v, %v)", rv, ok)
	}
}

func TestImperativeEnvAndRegistryRollback(t *testing.T) {
	fb := newFakeBackend(t)
	raw := testPayloadBytes(t, "")
	dir := filepath.Join(t.TempDir(), "RollbackEnvReg")

	// Pre-existing env var and registry value to confirm rollback restores previous values.
	if err := fb.SetEnv(ScopeUser, "EXISTING_VAR", "before"); err != nil {
		t.Fatal(err)
	}
	if err := fb.SetRegistryValue(RegistryHKCU, `Software\Shared`, "ExistingVal", StringValue("keep-me")); err != nil {
		t.Fatal(err)
	}

	app := &App{
		Payload: raw,
		AfterInstall: func(ctx *Context) error {
			if err := ctx.SetEnv("EXISTING_VAR", "during"); err != nil {
				return err
			}
			if err := ctx.SetEnv("BRAND_NEW_VAR", "temp"); err != nil {
				return err
			}
			if err := ctx.AddToPath(ctx.Dir()); err != nil {
				return err
			}
			if err := ctx.SetRegistryString(RegistryHKCU, `Software\Shared`, "ExistingVal", "overwritten"); err != nil {
				return err
			}
			if err := ctx.SetRegistryDWORD(RegistryHKCU, `Software\TempApp`, "Flag", 7); err != nil {
				return err
			}
			if err := ctx.SetRegistryString(RegistryHKCU, `Software\TempApp`, "Second", "two"); err != nil {
				return err
			}
			return errors.New("abort install")
		},
	}

	err := runWithBackend(app, []string{"--silent", "--dir", dir}, fb)
	if err == nil || !strings.Contains(err.Error(), "abort install") {
		t.Fatalf("expected abort error, got %v", err)
	}
	if v, ok, _ := fb.GetEnv(ScopeUser, "EXISTING_VAR"); !ok || v != "before" {
		t.Errorf("EXISTING_VAR after rollback = (%q, %v), want before", v, ok)
	}
	if _, ok, _ := fb.GetEnv(ScopeUser, "BRAND_NEW_VAR"); ok {
		t.Error("BRAND_NEW_VAR was not removed by rollback")
	}
	if len(fb.PathDirs(ScopeUser)) != 0 {
		t.Errorf("PATH was not reverted by rollback: %v", fb.PathDirs(ScopeUser))
	}
	if rv, ok, _ := fb.GetRegistryValue(RegistryHKCU, `Software\Shared`, "ExistingVal"); !ok || rv.String != "keep-me" {
		t.Errorf("ExistingVal after rollback = (%+v, %v), want keep-me", rv, ok)
	}
	if exists, _ := fb.RegistryKeyExists(RegistryHKCU, `Software\TempApp`); exists {
		t.Error("newly created Software\\TempApp key was not removed by rollback")
	}
}

func TestMachineScopeAndValidation(t *testing.T) {
	fb := newFakeBackend(t)
	raw := testPayloadBytes(t, "")
	dir := filepath.Join(t.TempDir(), "MachineHello")

	app := &App{
		Payload: raw,
		Windows: WindowsConfig{
			Scope:     ScopeMachine,
			AddToPath: true,
			Env:       map[string]string{"SYS_HELLO": "%INSTALL_DIR%"},
			Registry: []RegistryEntry{
				{Key: `Software\Acme\Hello`, Name: "Mode", Value: StringValue("machine")},
			},
		},
	}
	if err := runWithBackend(app, []string{"--silent", "--dir", dir}, fb); err != nil {
		t.Fatalf("machine install failed: %v", err)
	}
	if !slices.Contains(fb.PathDirs(ScopeMachine), dir) {
		t.Errorf("machine PATH missing %s: %v", dir, fb.PathDirs(ScopeMachine))
	}
	if v, ok, _ := fb.GetEnv(ScopeMachine, "SYS_HELLO"); !ok || v != dir {
		t.Errorf("machine SYS_HELLO = (%q, %v), want %q", v, ok, dir)
	}
	if rv, ok, _ := fb.GetRegistryValue(RegistryHKLM, `Software\Acme\Hello`, "Mode"); !ok || rv.String != "machine" {
		t.Errorf("HKLM Mode = (%+v, %v), want machine", rv, ok)
	}

	// Invalid names/roots/keys return errors cleanly.
	if err := fb.SetEnv(ScopeUser, "BAD=NAME", "1"); err == nil {
		t.Error("expected error for env name containing '='")
	}
	if err := fb.SetRegistryValue("HKZZ", `Software\X`, "A", StringValue("1")); err == nil {
		t.Error("expected error for invalid registry root")
	}
	if err := fb.SetRegistryValue(RegistryHKCU, "", "A", StringValue("1")); err == nil {
		t.Error("expected error for empty registry key")
	}
}

func TestLegacyUpgradeDetectionAndManifestPreservation(t *testing.T) {
	fb := newFakeBackend(t)
	dir := fb.defaultDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate an existing legacy install (e.g. install.ps1) with Hello.exe on disk
	// and NO Windows Uninstall registry entry yet.
	if err := os.WriteFile(filepath.Join(dir, "Hello.exe"), []byte("legacy-v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := testPayloadBytes(t, "")
	app := &App{Payload: raw}
	ic, err := NewContext(app, nil, fb)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if !ic.IsUpgrade() {
		t.Fatalf("IsUpgrade() = false for existing legacy executable in targetDir, want true")
	}

	// First install adds a custom PATH entry via AfterInstall.
	appWithPath := &App{
		Payload: raw,
		AfterInstall: func(ctx *Context) error {
			return ctx.AddToUserPath(filepath.Join(ctx.Dir(), "bin"))
		},
	}
	if err := runWithBackend(appWithPath, []string{"--silent", "--dir", dir}, fb); err != nil {
		t.Fatalf("initial install: %v", err)
	}

	// Second silent upgrade without AfterInstall must preserve the prior
	// manifest's Path tracking so Uninstall still cleans it up.
	if err := runWithBackend(&App{Payload: raw}, []string{"--silent", "--dir", dir}, fb); err != nil {
		t.Fatalf("silent upgrade: %v", err)
	}
	mfBytes, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}
	var mf InstallManifest
	if err := json.Unmarshal(mfBytes, &mf); err != nil {
		t.Fatalf("decoding manifest: %v", err)
	}
	wantBin := filepath.Join(dir, "bin")
	if !slices.Contains(mf.Path, ManifestPathEntry{Scope: ScopeUser, Dir: wantBin}) {
		t.Errorf("upgraded manifest lost prior Path entry %q: %+v", wantBin, mf.Path)
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
