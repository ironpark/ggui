package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverAndIsMainPackageDir(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)

	// Nothing yet.
	pkg, err := Discover(t.Context(), root, root, "", "")
	if err != nil || pkg != nil {
		t.Fatalf("empty project: pkg=%+v err=%v", pkg, err)
	}

	// Non-main package in ./installer (e.g. library) must be ignored.
	libDir := filepath.Join(root, "installer")
	os.MkdirAll(libDir, 0o755)
	os.WriteFile(filepath.Join(libDir, "lib.go"), []byte("package installer\n"), 0o644)
	if IsMainPackageDir(libDir) {
		t.Fatal("IsMainPackageDir returned true for non-main package")
	}
	if pkg, err := Discover(t.Context(), root, root, "", ""); err != nil || pkg != nil {
		t.Fatalf("library ./installer should be ignored: pkg=%+v err=%v", pkg, err)
	}

	// Main package in ./cmd/installer is auto-discovered.
	cmdInst := filepath.Join(root, "cmd", "installer")
	os.MkdirAll(cmdInst, 0o755)
	os.WriteFile(filepath.Join(cmdInst, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)
	if !IsMainPackageDir(cmdInst) {
		t.Fatal("IsMainPackageDir returned false for cmd/installer")
	}
	pkg, err = Discover(t.Context(), root, root, "", "")
	if err != nil || pkg == nil || pkg.Explicit {
		t.Fatalf("auto-discover cmd/installer: pkg=%+v err=%v", pkg, err)
	}
	if !sameDir(pkg.Dir, cmdInst) {
		t.Errorf("discovered dir = %q, want %q", pkg.Dir, cmdInst)
	}

	// Explicit configured path takes precedence over convention.
	customDir := filepath.Join(root, "packaging", "setup")
	os.MkdirAll(customDir, 0o755)
	os.WriteFile(filepath.Join(customDir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)
	pkg, err = Discover(t.Context(), root, root, "./packaging/setup", "")
	if err != nil || pkg == nil || !pkg.Explicit || !sameDir(pkg.Dir, customDir) {
		t.Fatalf("explicit configured pkg: pkg=%+v err=%v", pkg, err)
	}

	// Skip auto-discovery if mainDir itself is ./cmd/installer.
	if pkg, err := Discover(t.Context(), root, cmdInst, "", ""); err != nil || pkg != nil {
		t.Fatalf("when mainDir == cmd/installer: pkg=%+v err=%v", pkg, err)
	}
}

func TestDescriptionValidationAndWireFormat(t *testing.T) {
	// Golden JSON matching github.com/ironpark/ggui/installer.Description.
	golden := `{
		"v": 1,
		"name": "Acme",
		"license": "MIT",
		"windows": {
			"enabled": true,
			"scope": "machine",
			"install_dir": "%ProgramFiles%\\Acme",
			"license": "MIT",
			"publisher": "Acme Corp",
			"url": "https://acme.example",
			"start_menu_shortcut": true,
			"desktop_shortcut": true,
			"launch_after": false
		},
		"macos": {
			"enabled": true,
			"signing_identity": "Developer ID Installer",
			"install_location": "/Applications/Acme",
			"scripts": "scripts/pkg",
			"license": "MIT"
		}
	}`
	var d Description
	if err := json.Unmarshal([]byte(golden), &d); err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("valid golden description failed validation: %v", err)
	}
	if d.Windows.Scope != ScopeMachine || !d.Windows.DesktopShortcut || d.Windows.LaunchAfter || d.MacOS.InstallLocation != "/Applications/Acme" {
		t.Fatalf("unexpected decoded description: %+v", d)
	}

	badScope := d
	badScope.Windows.Scope = "admin"
	if err := badScope.Validate(); err == nil || !strings.Contains(err.Error(), "Windows.Scope") {
		t.Errorf("bad scope err = %v", err)
	}
	badURL := d
	badURL.Windows.URL = "acme.example"
	if err := badURL.Validate(); err == nil || !strings.Contains(err.Error(), "Windows.URL") {
		t.Errorf("bad url err = %v", err)
	}
	badLoc := d
	badLoc.MacOS.InstallLocation = "Applications"
	if err := badLoc.Validate(); err == nil || !strings.Contains(err.Error(), "MacOS.InstallLocation") {
		t.Errorf("bad macos loc err = %v", err)
	}
	badVer := d
	badVer.V = 99
	if err := badVer.Validate(); err == nil || !strings.Contains(err.Error(), "protocol version") {
		t.Errorf("bad version err = %v", err)
	}
}
