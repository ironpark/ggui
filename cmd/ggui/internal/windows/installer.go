package windows

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/installer"
)

const (
	gguiModule       = "github.com/ironpark/ggui"
	installerPackage = gguiModule + "/installer"
)

// SetupBuilder implements installer.Builder for self-contained Windows
// ggui-based installers (Name-Setup.exe).
//
// The installer embeds a ZIP payload via //go:embed and links its own icon,
// version info, and UAC manifest via a temporary .syso file inside a short-lived
// package directory in the project's module, so it cross-builds cleanly on any
// host (macOS, Linux, or Windows) using the project's own ggui dependency.
type SetupBuilder struct{}

var _ installer.Builder = SetupBuilder{}

// Target implements installer.Builder.
func (SetupBuilder) Target() string { return "windows" }

// OutputPath implements installer.Builder.
func (SetupBuilder) OutputPath(spec installer.Spec) string {
	return filepath.Join(spec.OutDir, spec.Name+"-Setup.exe")
}

// Validate implements installer.Builder.
func (SetupBuilder) Validate(spec installer.Spec) error {
	if spec.Name == "" {
		return errors.New("windows installer: app name is required")
	}
	if err := CheckFileName(spec.Name); err != nil {
		return err
	}
	if spec.ID == "" {
		return errors.New("windows installer: app ID is required")
	}
	if _, err := ParseVersion(spec.Version); err != nil {
		return fmt.Errorf("windows installer: %w", err)
	}
	if !slices.Contains(Arches, spec.Arch) {
		return fmt.Errorf("windows installer: unsupported GOARCH %q (use %s)", spec.Arch, strings.Join(Arches, ", "))
	}
	if spec.ProjectDir == "" {
		return errors.New("windows installer: project directory is required")
	}
	if spec.Artifact == "" {
		return errors.New("windows installer: built executable path is required")
	}
	if fi, err := os.Stat(spec.Artifact); err != nil {
		return fmt.Errorf("windows installer: %w", err)
	} else if fi.IsDir() {
		return fmt.Errorf("windows installer: %s is a directory, want an executable file", spec.Artifact)
	}
	for _, extra := range spec.ExtraFiles {
		if fi, err := os.Stat(extra); err != nil {
			return fmt.Errorf("windows installer: extra file %s: %w", extra, err)
		} else if fi.IsDir() {
			return fmt.Errorf("windows installer: extra file %s is a directory", extra)
		}
	}
	if spec.OutDir == "" {
		return errors.New("windows installer: output directory is required")
	}
	switch spec.Windows.Scope {
	case "", installer.ScopeUser, installer.ScopeMachine:
	default:
		return fmt.Errorf("windows installer: scope %q must be %q or %q", spec.Windows.Scope, installer.ScopeUser, installer.ScopeMachine)
	}
	if spec.License != "" {
		if _, err := os.Stat(spec.License); err != nil {
			return fmt.Errorf("installer license %s: %w", spec.License, err)
		}
	}
	return nil
}

// Build implements installer.Builder, compiling Name-Setup.exe in spec.OutDir.
func (b SetupBuilder) Build(ctx context.Context, spec installer.Spec) (installer.Result, error) {
	if err := b.Validate(spec); err != nil {
		return installer.Result{}, err
	}
	gguiMod, err := projectGGUI(ctx, spec.ProjectDir)
	if err != nil {
		return installer.Result{}, err
	}

	scope := spec.Windows.Scope
	if scope == "" {
		scope = installer.ScopeUser
	}
	payloadZip, err := buildPayloadZip(spec, scope)
	if err != nil {
		return installer.Result{}, err
	}

	level := ExecutionLevelAsInvoker
	if scope == installer.ScopeMachine {
		level = ExecutionLevelRequireAdmin
	}
	internalName := spec.Executable
	if internalName == "" {
		internalName = strings.ToLower(strings.ReplaceAll(spec.Name, " ", "-"))
	}
	setupFileName := spec.Name + "-Setup.exe"
	obj, warnings, err := Object(Resources{
		Arch:           spec.Arch,
		Name:           spec.Name,
		Executable:     internalName + "-setup",
		OriginalName:   setupFileName,
		ID:             spec.ID + ".setup",
		Version:        spec.Version,
		Description:    spec.Name + " Setup",
		Company:        spec.Publisher,
		Copyright:      spec.Copyright,
		Icon:           spec.Icon,
		ExecutionLevel: level,
	})
	if err != nil {
		return installer.Result{}, err
	}

	// Go reads .syso files and //go:embed files strictly from the package's
	// directory on disk (ignoring .syso files provided only via -overlay).
	// Create a short-lived dot-prefixed package directory inside the project
	// module so Go resolves github.com/ironpark/ggui/installer from go.mod
	// while ignoring the directory in ./... pattern walks.
	pkgDir, err := os.MkdirTemp(spec.ProjectDir, ".ggui_winsetup_")
	if err != nil {
		return installer.Result{}, fmt.Errorf("windows installer: creating build directory: %w", err)
	}
	defer os.RemoveAll(pkgDir)

	if err := os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte(setupMainSource), 0o644); err != nil {
		return installer.Result{}, err
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "payload.zip"), payloadZip, 0o644); err != nil {
		return installer.Result{}, err
	}
	if err := os.WriteFile(filepath.Join(pkgDir, SysoName(spec.Arch)), obj, 0o644); err != nil {
		return installer.Result{}, err
	}

	out := b.OutputPath(spec)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return installer.Result{}, err
	}
	bo := gotool.BuildOptions{
		Dir:      spec.ProjectDir,
		Package:  "." + string(filepath.Separator) + filepath.Base(pkgDir),
		Output:   out,
		GOOS:     "windows",
		GOARCH:   spec.Arch,
		Trimpath: true,
		LDFlags:  "-s -w -H=windowsgui",
		Mod:      "readonly",
		Env:      []string{"CGO_ENABLED=0"},
	}
	if spec.Logf != nil {
		spec.Logf("%s", bo)
	}
	if err := gotool.Build(ctx, bo); err != nil {
		var be *gotool.BuildError
		if errors.As(err, &be) && ctx.Err() == nil {
			return installer.Result{}, setupBuildFailure(gguiMod, string(be.Output))
		}
		return installer.Result{}, err
	}
	return installer.Result{Path: out, Warnings: warnings}, nil
}

const setupMainSource = `// Code generated by ggui build for the Windows installer. DO NOT EDIT.

package main

import (
	_ "embed"

	"github.com/ironpark/ggui/installer"
)

//go:embed payload.zip
var payload []byte

func main() {
	installer.Main(payload)
}
`

// payloadMetadata mirrors github.com/ironpark/ggui/installer.Metadata so
// cmd/ggui stays decoupled from the root ggui module.
type payloadMetadata struct {
	Name              string `json:"name"`
	ID                string `json:"id"`
	Version           string `json:"version"`
	Executable        string `json:"executable"`
	Description       string `json:"description,omitempty"`
	Publisher         string `json:"publisher,omitempty"`
	Copyright         string `json:"copyright,omitempty"`
	Homepage          string `json:"homepage,omitempty"`
	Scope             string `json:"scope,omitempty"`
	InstallDir        string `json:"install_dir,omitempty"`
	License           string `json:"license,omitempty"`
	StartMenuShortcut bool   `json:"start_menu_shortcut"`
	DesktopShortcut   bool   `json:"desktop_shortcut"`
	LaunchAfter       bool   `json:"launch_after"`
}

var payloadZipTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func buildPayloadZip(spec installer.Spec, scope string) ([]byte, error) {
	var licenseText string
	if spec.License != "" {
		raw, err := os.ReadFile(spec.License)
		if err != nil {
			return nil, fmt.Errorf("reading installer license %s: %w", spec.License, err)
		}
		licenseText = string(raw)
	}

	meta := payloadMetadata{
		Name:              spec.Name,
		ID:                spec.ID,
		Version:           spec.Version,
		Executable:        filepath.Base(spec.Artifact),
		Description:       spec.Description,
		Publisher:         spec.Publisher,
		Copyright:         spec.Copyright,
		Homepage:          spec.Homepage,
		Scope:             scope,
		InstallDir:        spec.Windows.InstallLocation,
		License:           licenseText,
		StartMenuShortcut: spec.Windows.StartMenuShortcut,
		DesktopShortcut:   spec.Windows.DesktopShortcut,
		LaunchAfter:       spec.Windows.LaunchAfter,
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	writeBytes := func(name string, data []byte) error {
		h := &zip.FileHeader{
			Name:     name,
			Method:   zip.Deflate,
			Modified: payloadZipTime,
		}
		h.SetMode(0o644)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	writeFile := func(name, path string) error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		h := &zip.FileHeader{
			Name:     name,
			Method:   zip.Deflate,
			Modified: payloadZipTime,
		}
		h.SetMode(0o755)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		return err
	}

	if err := writeBytes("meta.json", metaBytes); err != nil {
		return nil, err
	}
	if spec.Icon != "" {
		iconBytes, err := os.ReadFile(spec.Icon)
		if err != nil {
			return nil, fmt.Errorf("reading icon %s: %w", spec.Icon, err)
		}
		if err := writeBytes("icon.png", iconBytes); err != nil {
			return nil, err
		}
	}

	seen := map[string]bool{}
	addAppFile := func(path string) error {
		base := filepath.Base(path)
		if seen[base] {
			return nil
		}
		seen[base] = true
		return writeFile("files/"+base, path)
	}
	if err := addAppFile(spec.Artifact); err != nil {
		return nil, err
	}
	for _, extra := range spec.ExtraFiles {
		if err := addAppFile(extra); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func projectGGUI(ctx context.Context, dir string) (gotool.Module, error) {
	mods, err := gotool.Modules(ctx, dir, gguiModule)
	if err != nil {
		return gotool.Module{}, err
	}
	if len(mods) != 1 || mods[0].Error != nil {
		return gotool.Module{}, fmt.Errorf("building the ggui Windows installer needs %s in the project's module; run go get %s@latest", gguiModule, gguiModule)
	}
	return mods[0], nil
}

var noInstallerPkg = regexp.MustCompile(`(?m)package ` + regexp.QuoteMeta(installerPackage) + `(?:[:;]|$)`)

func setupBuildFailure(gguiMod gotool.Module, output string) error {
	version := gguiMod.Version
	if r := gguiMod.Replace; r != nil {
		version = r.Path
		if r.Version != "" {
			version += "@" + r.Version
		}
	}
	if version == "" {
		version = gguiMod.Dir
	}
	switch {
	case noInstallerPkg.MatchString(output):
		return fmt.Errorf("the project's %s (%s) does not include %s; update ggui (go get %s@latest) to build the Windows installer", gguiModule, version, installerPackage, gguiModule)
	case strings.Contains(output, "missing go.sum entry"):
		return fmt.Errorf("the project's go.sum lacks entries needed to build %s; run go mod tidy and retry:\n%s", installerPackage, strings.TrimSpace(output))
	}
	return fmt.Errorf("compiling the Windows installer failed:\n%s", strings.TrimSpace(output))
}
