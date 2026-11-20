package macos

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/zapp/pkg/icns"
)

func TestDevBundleID(t *testing.T) {
	tests := []struct {
		id, name, want string
	}{
		{"com.example.hello", "Hello", "com.example.hello.dev"},
		{"", "Hello World", "com.example.hello-world.dev"},
		{"", "My_App.1", "com.example.my-app-1.dev"},
		{"", "!!!", "com.example.app.dev"},
	}
	for _, tt := range tests {
		if got := DevBundleID(tt.id, tt.name); got != tt.want {
			t.Errorf("DevBundleID(%q, %q) = %q, want %q", tt.id, tt.name, got, tt.want)
		}
	}
}

func writeTestPNG(t *testing.T, path string, size int, c color.NRGBA) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.SetNRGBA(x, y, c)
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildNamedProgram(t *testing.T, dir, name, output string) string {
	t.Helper()
	src := filepath.Join(dir, name+".go")
	code := "package main\nimport \"fmt\"\nfunc main() { fmt.Println(" + `"` + output + `"` + ") }\n"
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", bin, src)
	arch := runtime.GOARCH
	if arch != "arm64" {
		arch = "amd64"
	}
	cmd.Env = append(os.Environ(), "GOWORK=off", "GO111MODULE=off", "GOOS=darwin", "GOARCH="+arch, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %s", out)
	}
	return bin
}

// TestDevBundleStageIncremental verifies bundle layout, Info.plist, icon
// encoding, incremental rebuild skipping unchanged Info.plist/icon, fresh
// inode replacement, icon cache reuse across sessions, UpdateSpec/NeedsRefresh,
// and codesign verification on macOS.
func TestDevBundleStageIncremental(t *testing.T) {
	if testing.Short() {
		t.Skip("builds macOS programs")
	}
	dir := t.TempDir()
	binV1 := buildNamedProgram(t, dir, "prog1", "hello-v1")
	binV2 := buildNamedProgram(t, dir, "prog2", "hello-v2")

	iconSrc := filepath.Join(dir, "icon.svg")
	if err := os.WriteFile(iconSrc, []byte("<svg viewBox='0 0 100 100'/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(dir, "icon-cache")
	bundleDir := filepath.Join(dir, "bundles")

	prepareCalls := 0
	makeSpec := func(extra map[string]any) DevBundleSpec {
		return DevBundleSpec{
			Name:                 "My App",
			Executable:           "myapp",
			ID:                   DevBundleID("com.example.myapp", "My App"),
			Version:              "1.0.0",
			MinimumSystemVersion: "13.0",
			Extra:                extra,
			Dir:                  bundleDir,
			Icon:                 iconSrc,
			IconName:             "icon.svg",
			IconCacheDir:         cacheDir,
			PrepareIcon: func(_ context.Context, dst string) error {
				prepareCalls++
				writeTestPNG(t, dst, 512, color.NRGBA{R: 40, G: 120, B: 220, A: 255})
				return nil
			},
		}
	}

	b := NewDevBundle(makeSpec(map[string]any{"NSCameraUsageDescription": "Camera test"}), DevSigning{Target: "-", Display: "ad hoc"})
	exe1, _, err := b.Stage(t.Context(), binV1)
	if err != nil {
		t.Fatal(err)
	}
	wantApp := filepath.Join(bundleDir, "My App.dev.app")
	wantExe := filepath.Join(wantApp, "Contents", "MacOS", "myapp")
	if exe1 != wantExe {
		t.Fatalf("staged exe = %q, want %q", exe1, wantExe)
	}
	if prepareCalls != 1 {
		t.Errorf("PrepareIcon called %d times, want 1", prepareCalls)
	}

	plistPath := filepath.Join(wantApp, "Contents", "Info.plist")
	icnsPath := filepath.Join(wantApp, "Contents", "Resources", "AppIcon.icns")
	plistBytes, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<string>com.example.myapp.dev</string>",
		"<string>My App</string>",
		"<string>myapp</string>",
		"<key>CFBundleIconFile</key>\n\t<string>AppIcon</string>",
		"<key>NSHighResolutionCapable</key>\n\t<true/>",
		"<key>NSRequiresAquaSystemAppearance</key>\n\t<false/>",
		"<key>NSCameraUsageDescription</key>\n\t<string>Camera test</string>",
	} {
		if !bytes.Contains(plistBytes, []byte(want)) {
			t.Errorf("Info.plist lacks %q:\n%s", want, plistBytes)
		}
	}
	f, err := os.Open(icnsPath)
	if err != nil {
		t.Fatal(err)
	}
	family, err := icns.Decode(f)
	f.Close()
	if err != nil || len(family.Elements) == 0 {
		t.Fatalf("AppIcon.icns invalid: %v", err)
	}

	// Set Info.plist and AppIcon.icns timestamps into the past so any rewrite is detectable.
	past := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	if err := os.Chtimes(plistPath, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(icnsPath, past, past); err != nil {
		t.Fatal(err)
	}
	exe1Info, err := os.Stat(exe1)
	if err != nil {
		t.Fatal(err)
	}

	// Regular rebuild: swaps only executable (new inode) and re-signs; skips Info.plist & icon.
	exe2, _, err := b.Stage(t.Context(), binV2)
	if err != nil {
		t.Fatal(err)
	}
	if prepareCalls != 1 {
		t.Errorf("PrepareIcon called %d times after unchanged rebuild, want 1", prepareCalls)
	}
	if fi, _ := os.Stat(plistPath); !fi.ModTime().Equal(past) {
		t.Errorf("Info.plist was rewritten on unchanged rebuild (mtime %v != %v)", fi.ModTime(), past)
	}
	if fi, _ := os.Stat(icnsPath); !fi.ModTime().Equal(past) {
		t.Errorf("AppIcon.icns was rewritten on unchanged rebuild (mtime %v != %v)", fi.ModTime(), past)
	}
	exe2Info, err := os.Stat(exe2)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && os.SameFile(exe1Info, exe2Info) {
		t.Error("executable kept the same inode across rebuilds (must replace with fresh inode)")
	}

	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("plutil", "-lint", plistPath).CombinedOutput(); err != nil {
			t.Errorf("plutil -lint: %s", out)
		}
		if out, err := exec.Command("codesign", "--verify", "--strict", "--deep", "-vv", wantApp).CombinedOutput(); err != nil {
			t.Errorf("codesign --verify: %s", out)
		}
		if out, err := exec.Command(exe2).CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "hello-v2" {
			t.Errorf("running staged exe2: out=%q err=%v", out, err)
		}
	}

	// NeedsRefresh is false for unrelated files and unchanged UpdateSpec.
	if b.NeedsRefresh([]string{filepath.Join(dir, "main.go")}) {
		t.Error("NeedsRefresh = true for unrelated Go file")
	}
	b.UpdateSpec(makeSpec(map[string]any{"NSCameraUsageDescription": "Camera test"}), "")
	if b.NeedsRefresh(nil) {
		t.Error("NeedsRefresh = true after identical UpdateSpec")
	}

	// Updating plist metadata triggers NeedsRefresh and rewrites Info.plist on next Stage.
	b.UpdateSpec(makeSpec(map[string]any{"NSCameraUsageDescription": "Updated camera"}), "")
	if !b.NeedsRefresh(nil) {
		t.Error("NeedsRefresh = false after plist change")
	}
	if _, _, err := b.Stage(t.Context(), binV2); err != nil {
		t.Fatal(err)
	}
	if b.NeedsRefresh(nil) {
		t.Error("NeedsRefresh = true right after Stage")
	}
	if data, _ := os.ReadFile(plistPath); !bytes.Contains(data, []byte("Updated camera")) {
		t.Errorf("Info.plist was not updated:\n%s", data)
	}

	// Editing the icon source triggers NeedsRefresh and re-renders the icon.
	if err := os.WriteFile(iconSrc, []byte("<svg viewBox='0 0 200 200'/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !b.NeedsRefresh([]string{iconSrc}) {
		t.Error("NeedsRefresh = false after icon source changed")
	}
	if _, _, err := b.Stage(t.Context(), binV2); err != nil {
		t.Fatal(err)
	}
	if prepareCalls != 2 {
		t.Errorf("PrepareIcon called %d times after icon edit, want 2", prepareCalls)
	}

	// A brand-new DevBundle session with the same IconCacheDir reuses the cached .icns without calling PrepareIcon.
	freshCalls := 0
	specFresh := makeSpec(map[string]any{"NSCameraUsageDescription": "Updated camera"})
	specFresh.PrepareIcon = func(_ context.Context, dst string) error {
		freshCalls++
		return nil
	}
	b2 := NewDevBundle(specFresh, DevSigning{Target: "-", Display: "ad hoc"})
	if _, _, err := b2.Stage(t.Context(), binV1); err != nil {
		t.Fatal(err)
	}
	if freshCalls != 0 {
		t.Errorf("new session called PrepareIcon %d times despite IconCacheDir hit", freshCalls)
	}
}

func TestDevBundleRefusesNonBundle(t *testing.T) {
	dir := t.TempDir()
	clash := filepath.Join(dir, "Clash.dev.app")
	if err := os.MkdirAll(clash, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(clash, "keep.txt"), []byte("user data"), 0o644)

	b := NewDevBundle(DevBundleSpec{
		Name: "Clash", Executable: "clash", ID: "com.example.clash.dev", Version: "0.1.0",
		Dir: dir,
	}, DevSigning{Target: "-"})
	if _, _, err := b.Stage(t.Context(), filepath.Join(dir, "nope")); err == nil || !strings.Contains(err.Error(), "not an app bundle") {
		t.Errorf("Stage on non-bundle dir = %v, want refusal", err)
	}
	if _, err := os.Stat(filepath.Join(clash, "keep.txt")); err != nil {
		t.Error("user data inside non-bundle directory was removed")
	}
}

func TestDevBundleMissingIconWarnsAndContinues(t *testing.T) {
	if testing.Short() {
		t.Skip("builds macOS program")
	}
	dir := t.TempDir()
	bin := buildNamedProgram(t, dir, "prog", "ok")
	b := NewDevBundle(DevBundleSpec{
		Name: "NoIcon", Executable: "noicon", ID: "com.example.noicon.dev", Version: "0.1.0",
		Dir:  filepath.Join(dir, "out"),
		Icon: filepath.Join(dir, "does-not-exist.png"),
	}, DevSigning{Target: "-"})
	exe, warnings, err := b.Stage(t.Context(), bin)
	if err != nil {
		t.Fatalf("Stage failed on missing icon: %v", err)
	}
	if exe == "" || len(warnings) == 0 || !strings.Contains(warnings[0], "continuing without a bundle icon") {
		t.Errorf("exe=%q warnings=%q, want warning and valid exe", exe, warnings)
	}
	plistBytes, err := os.ReadFile(filepath.Join(b.AppPath(), "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(plistBytes, []byte("CFBundleIconFile")) {
		t.Error("CFBundleIconFile set when icon was missing")
	}
}
