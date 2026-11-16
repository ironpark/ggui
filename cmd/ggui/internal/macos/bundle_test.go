package macos

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/icns"
)

func info() Info {
	return Info{
		Name: "Hello & Goodbye", Executable: "hello", ID: "com.example.hello",
		Version: "1.2.0", MinimumSystemVersion: "13.0",
		Category: "public.app-category.productivity",
		Extra: map[string]any{
			"NSCameraUsageDescription": "Scans <documents>.",
			"LSUIElement":              true,
			"CFBundleVersion":          "42", // overrides the generated one
			"CFBundleURLTypes":         []map[string]any{{"CFBundleURLSchemes": []any{"hello"}}},
			"Ratio":                    1.5,
			"Count":                    int64(3),
		},
	}
}

func TestPlist(t *testing.T) {
	d := info().Plist()
	for k, want := range map[string]any{
		"CFBundleName":               "Hello & Goodbye",
		"CFBundleExecutable":         "hello",
		"CFBundleIdentifier":         "com.example.hello",
		"CFBundleShortVersionString": "1.2.0",
		"CFBundleVersion":            "42",
		"CFBundlePackageType":        "APPL",
		"LSMinimumSystemVersion":     "13.0",
		"LSApplicationCategoryType":  "public.app-category.productivity",
		"NSHighResolutionCapable":    true,
	} {
		if d[k] != want {
			t.Errorf("%s = %v, want %v", k, d[k], want)
		}
	}
	if _, ok := d["CFBundleIconFile"]; ok {
		t.Error("CFBundleIconFile set without an icon")
	}
}

func TestAssemble(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "prog")
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	icon := filepath.Join(dir, "icon.png")
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.SetNRGBA(0, 0, color.NRGBA{})
	var buf bytes.Buffer
	png.Encode(&buf, img)
	os.WriteFile(icon, buf.Bytes(), 0o644)

	b := Bundle{Info: info(), Binary: bin, Icon: icon, OutDir: filepath.Join(dir, "out")}
	app, warnings, err := Assemble(b)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(app) != "Hello & Goodbye.app" {
		t.Errorf("bundle is %s", app)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "1024") {
		t.Errorf("warnings = %q, want one about the 512-pixel icon", warnings)
	}
	exe, err := os.Stat(filepath.Join(app, "Contents", "MacOS", "hello"))
	if err != nil {
		t.Error(err)
	} else if runtime.GOOS != "windows" && exe.Mode()&0o111 == 0 {
		// Windows has no execute bits to keep.
		t.Errorf("executable has mode %v", exe.Mode())
	}
	f, err := os.Open(filepath.Join(app, "Contents", "Resources", "AppIcon.icns"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := icns.Read(f)
	// Closed before the bundle is built again: Windows removes no file that is open.
	f.Close()
	if err != nil || len(entries) == 0 {
		t.Errorf("icon: %d entries, %v", len(entries), err)
	}
	plistPath := filepath.Join(app, "Contents", "Info.plist")
	data, _ := os.ReadFile(plistPath)
	for _, want := range []string{
		"<key>CFBundleIconFile</key>\n\t<string>AppIcon</string>",
		"<string>Hello &amp; Goodbye</string>",
		"<string>Scans &lt;documents&gt;.</string>",
		"<key>LSUIElement</key>\n\t<true/>",
		"<real>1.5</real>",
		"<integer>3</integer>",
		"<key>CFBundleURLSchemes</key>",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("Info.plist lacks %q:\n%s", want, data)
		}
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("plutil", "-lint", plistPath).CombinedOutput(); err != nil {
			t.Errorf("plutil -lint: %s", out)
		}
	}

	// A second build replaces the bundle; anything else of that name is
	// left alone.
	if _, _, err := Assemble(b); err != nil {
		t.Fatalf("rebuilding: %v", err)
	}
	os.RemoveAll(app)
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "keep"), nil, 0o644)
	if _, _, err := Assemble(b); err == nil {
		t.Error("replaced a directory that is not a bundle")
	}
	if _, err := os.Stat(filepath.Join(app, "keep")); err != nil {
		t.Error("removed a directory that is not a bundle")
	}
}

func TestSignAdHoc(t *testing.T) {
	if !CanSign() {
		t.Skip("codesign needs macOS")
	}
	if testing.Short() {
		t.Skip("builds a program")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	os.WriteFile(src, []byte("package main\nfunc main() {}\n"), 0o644)
	bin := filepath.Join(dir, "prog")
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = append(os.Environ(), "GOWORK=off", "GO111MODULE=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %s", out)
	}
	app, _, err := Assemble(Bundle{Info: info(), Binary: bin, OutDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := Sign(t.Context(), app, "-", ""); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("codesign", "--verify", "--strict", app).CombinedOutput(); err != nil {
		t.Errorf("codesign --verify: %s", out)
	}
}
