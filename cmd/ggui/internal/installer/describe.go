package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
)

const (
	// DescribeEnv is the environment variable passed to a user's Go installer
	// binary so it serializes its build-time Description JSON and exits.
	// Mirrors github.com/ironpark/ggui/installer.DescribeEnv.
	DescribeEnv      = "GGUI_INSTALLER_DESCRIBE"
	describeProtocol = 1
)

// Description mirrors github.com/ironpark/ggui/installer.Description so
// cmd/ggui remains decoupled from the root ggui module.
type Description struct {
	V           int                `json:"v"`
	Name        string             `json:"name,omitempty"`
	ID          string             `json:"id,omitempty"`
	Version     string             `json:"version,omitempty"`
	Description string             `json:"description,omitempty"`
	Publisher   string             `json:"publisher,omitempty"`
	Copyright   string             `json:"copyright,omitempty"`
	Homepage    string             `json:"homepage,omitempty"`
	License     string             `json:"license,omitempty"`
	LicenseFile string             `json:"license_file,omitempty"`
	Windows     WindowsDescription `json:"windows"`
	MacOS       MacOSDescription   `json:"macos"`
}

// WindowsDescription mirrors github.com/ironpark/ggui/installer.WindowsDescription.
type WindowsDescription struct {
	Enabled           bool   `json:"enabled"`
	Scope             string `json:"scope"`
	InstallDir        string `json:"install_dir,omitempty"`
	License           string `json:"license,omitempty"`
	LicenseFile       string `json:"license_file,omitempty"`
	Publisher         string `json:"publisher,omitempty"`
	URL               string `json:"url,omitempty"`
	StartMenuShortcut bool   `json:"start_menu_shortcut"`
	DesktopShortcut   bool   `json:"desktop_shortcut"`
	LaunchAfter       bool   `json:"launch_after"`
}

// MacOSDescription mirrors github.com/ironpark/ggui/installer.MacOSDescription.
type MacOSDescription struct {
	Enabled         bool   `json:"enabled"`
	SigningIdentity string `json:"signing_identity,omitempty"`
	InstallLocation string `json:"install_location,omitempty"`
	Scripts         string `json:"scripts,omitempty"`
	License         string `json:"license,omitempty"`
	LicenseFile     string `json:"license_file,omitempty"`
}

// Validate checks fields returned by a user's Go installer package before
// building platform artifacts.
func (d *Description) Validate() error {
	if d.V != describeProtocol {
		return fmt.Errorf("installer package reports protocol version %d, this ggui reads %d: update ggui and cmd/ggui together", d.V, describeProtocol)
	}
	switch d.Windows.Scope {
	case "", ScopeUser, ScopeMachine:
	default:
		return fmt.Errorf("installer Windows.Scope %q must be %q or %q", d.Windows.Scope, ScopeUser, ScopeMachine)
	}
	if d.Windows.URL != "" && !isHTTPURL(d.Windows.URL) {
		return fmt.Errorf("installer Windows.URL %q must be an http or https URL", d.Windows.URL)
	}
	if d.MacOS.InstallLocation != "" && !strings.HasPrefix(d.MacOS.InstallLocation, "/") {
		return fmt.Errorf("installer MacOS.InstallLocation %q must be an absolute path, such as /Applications", d.MacOS.InstallLocation)
	}
	return nil
}

// Describe builds pkg for the host platform and runs it with
// GGUI_INSTALLER_DESCRIBE set so the Go installer code emits its build-time
// specification (Windows UAC scope, shortcuts, license, macOS .pkg options)
// without opening a window.
func Describe(ctx context.Context, projectRoot string, pkg *Package, logf func(string, ...any)) (*Description, error) {
	if pkg == nil {
		return nil, errors.New("installer: nil package")
	}
	tmp, err := os.MkdirTemp("", "ggui-installer-desc-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	host, err := gotool.Env(ctx, projectRoot, nil, "GOHOSTOS", "GOHOSTARCH")
	if err != nil {
		return nil, err
	}
	exe := filepath.Join(tmp, "describe")
	if host["GOHOSTOS"] == "windows" {
		exe += ".exe"
	}
	bo := gotool.BuildOptions{
		Dir:     projectRoot,
		Package: pkg.Arg,
		Output:  exe,
		GOOS:    host["GOHOSTOS"],
		GOARCH:  host["GOHOSTARCH"],
		Mod:     "readonly",
		TmpDir:  tmp,
		Env:     []string{"CGO_ENABLED=0"},
	}
	if logf != nil {
		logf("%s", bo)
	}
	if err := gotool.Build(ctx, bo); err != nil {
		return nil, fmt.Errorf("building installer package %s: %w", pkg.Arg, err)
	}

	reportPath := filepath.Join(tmp, "describe.json")
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, exe)
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), DescribeEnv+"="+reportPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("running installer package %s: %s", pkg.Arg, msg)
		}
		return nil, fmt.Errorf("running installer package %s: %w", pkg.Arg, err)
	}

	raw, err := os.ReadFile(reportPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("installer package %s exited without describing itself: main must call installer.Main or installer.Run (github.com/ironpark/ggui/installer)", pkg.Arg)
	}
	if err != nil {
		return nil, err
	}
	var desc Description
	if err := json.Unmarshal(raw, &desc); err != nil {
		return nil, fmt.Errorf("decoding installer description from %s: %w", pkg.Arg, err)
	}
	if err := desc.Validate(); err != nil {
		return nil, err
	}
	return &desc, nil
}

func isHTTPURL(s string) bool {
	return (strings.HasPrefix(s, "https://") && len(s) > len("https://")) ||
		(strings.HasPrefix(s, "http://") && len(s) > len("http://"))
}
