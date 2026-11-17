package macos

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/zapp/pkg/plist"
)

// What cannot work is refused before a build starts, saying what to do.
func TestNewSignerRefuses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		s        Signing
		notarize bool
		want     string
	}{
		{"ad hoc notarized", Signing{Identity: "-"}, true, "Developer ID signature"},
		{"no notary credentials", Signing{Identity: "Developer ID Application", Notary: Notary{PasswordVar: "GGUI_NOTARY_PASSWORD"}}, true, "store-credentials"},
		{"Apple ID without password", Signing{Identity: "Developer ID Application", Notary: Notary{AppleID: "me@example.com", TeamID: "ABCDE12345", PasswordVar: "GGUI_NOTARY_PASSWORD"}}, true, "$GGUI_NOTARY_PASSWORD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewSigner(tc.s, tc.notarize)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
	// Away from a Mac nothing signs: neither an identity nor a certificate.
	if runtime.GOOS != "darwin" {
		for _, s := range []Signing{{Identity: "Developer ID Application"}, {Identity: "-", P12File: "cert.p12"}} {
			_, err := NewSigner(s, false)
			if err == nil || !strings.Contains(err.Error(), "on a Mac") {
				t.Errorf("signing with %s away from a Mac: %v", s.Describe(), err)
			}
		}
	}
}

func TestAdHocSignerLeavesInstallersUnsigned(t *testing.T) {
	sg, err := NewSigner(Signing{Identity: "-"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !sg.AdHoc() {
		t.Error("not ad hoc")
	}
	if signed, err := sg.Sign(t.Context(), "App.pkg"); signed || err != nil {
		t.Errorf("installer: signed = %v, %v", signed, err)
	}
}

func TestSigningDescribe(t *testing.T) {
	for _, tc := range []struct {
		s    Signing
		want string
	}{
		{Signing{Identity: "-"}, "ad hoc"},
		{Signing{Identity: "Developer ID Application"}, `the keychain identity "Developer ID Application"`},
		{Signing{Identity: "-", P12Base64: "AAAA", P12Source: "$GGUI_P12_BASE64"}, "the certificate $GGUI_P12_BASE64"},
	} {
		if got := tc.s.Describe(); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.s, got, tc.want)
		}
		// The password never shows.
		if strings.Contains(tc.s.Describe(), "AAAA") {
			t.Error("description shows the certificate")
		}
	}
	n := Notary{Profile: "ggui", Password: "secret", Timeout: time.Hour}
	if d := n.Describe(); strings.Contains(d, "secret") || !strings.Contains(d, "ggui") {
		t.Errorf("notary: %q", d)
	}
}

// TOML's arrays of tables become plist arrays of dictionaries.
func TestPlistValue(t *testing.T) {
	v := plistValue(map[string]any{
		"CFBundleURLTypes": []map[string]any{{"CFBundleURLSchemes": []any{"hello"}}},
		"Names":            []string{"a", "b"},
	})
	data, err := plist.MarshalXML(v)
	if err != nil {
		t.Fatal(err)
	}
	back, err := plist.ParseDict(data)
	if err != nil {
		t.Fatal(err)
	}
	types, ok := back["CFBundleURLTypes"].([]any)
	if !ok || len(types) != 1 {
		t.Fatalf("CFBundleURLTypes = %#v", back["CFBundleURLTypes"])
	}
	if d, ok := types[0].(map[string]any); !ok || len(d["CFBundleURLSchemes"].([]any)) != 1 {
		t.Errorf("CFBundleURLTypes[0] = %#v", types[0])
	}
}

func TestDMGLayout(t *testing.T) {
	d := DMG{Width: 640, Height: 480, IconSize: 128, LabelSize: 14}
	app, link := d.layout()
	// Side by side across the middle, symmetric about the centre.
	if sum := app[0] + link[0]; app[1] != link[1] || sum < 639 || sum > 641 || app[0] >= link[0] {
		t.Errorf("default layout: app %v, Applications %v", app, link)
	}
	d.AppPosition, d.ApplicationsPosition = []int{100, 120}, []int{400, 120}
	if app, link := d.layout(); app != [2]int{100, 120} || link != [2]int{400, 120} {
		t.Errorf("given layout: app %v, Applications %v", app, link)
	}
}

// The disk image and the installer are made in pure Go on any host; on
// macOS, hdiutil and pkgutil must read them.
func TestMakeDMGAndPKG(t *testing.T) {
	if testing.Short() {
		t.Skip("makes a disk image")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "prog")
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	app, _, err := Assemble(Bundle{Info: info(), Binary: bin, OutDir: filepath.Join(dir, "out")})
	if err != nil {
		t.Fatal(err)
	}

	img := filepath.Join(dir, "out", "Hello.dmg")
	err = MakeDMG(t.Context(), DMG{
		App: app, Out: img, Title: "Hello",
		Width: 640, Height: 480, IconSize: 128, LabelSize: 14, FS: "hfsplus", Format: "udzo",
	})
	if err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(dir, "out", "Hello.pkg")
	err = MakePKG(t.Context(), PKG{App: app, Out: pkg, ID: "com.example.hello", Version: "1.2.0", MinOS: "13.0", InstallLocation: "/Applications"})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "darwin" {
		for _, p := range []string{img, pkg} {
			if fi, err := os.Stat(p); err != nil || fi.Size() == 0 {
				t.Errorf("%s: %v", p, err)
			}
		}
		return
	}
	if out, err := exec.Command("hdiutil", "imageinfo", img).CombinedOutput(); err != nil {
		t.Errorf("hdiutil imageinfo: %s", out)
	}
	out, err := exec.Command("pkgutil", "--payload-files", pkg).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Hello & Goodbye.app/Contents/MacOS/hello") {
		t.Errorf("pkgutil --payload-files: %s", out)
	}
}
