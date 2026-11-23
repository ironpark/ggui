package macos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/installer"
	"github.com/ironpark/zapp/pkg/macpkg"
)

// PKG describes an installer package that installs an app.
type PKG struct {
	App             string // the .app bundle
	Out             string // the .pkg file, replaced if it exists
	ID              string // the package identifier: the app's bundle identifier
	Version         string
	MinOS           string // the oldest macOS the installer runs on
	InstallLocation string // where the app goes, /Applications by default
	Scripts         string // a directory with preinstall and postinstall, or ""
	License         string // a plain-text license to accept, or ""
}

// MakePKG writes the installer package, unsigned. It is pure Go, so it
// works on any host.
func MakePKG(ctx context.Context, p PKG) error {
	if err := os.MkdirAll(filepath.Dir(p.Out), 0o755); err != nil {
		return err
	}
	err := macpkg.BuildApp(ctx, macpkg.AppConfig{
		AppPath:         p.App,
		OutputPath:      p.Out,
		Identifier:      p.ID,
		Version:         p.Version,
		InstallLocation: p.InstallLocation,
		ScriptsDir:      p.Scripts,
		MinOSVersion:    p.MinOS,
		License:         p.License,
	})
	if err != nil {
		return fmt.Errorf("installer package: %w", err)
	}
	return nil
}

// PKGBuilder implements installer.Builder for macOS .pkg installers.
type PKGBuilder struct{}

var _ installer.Builder = PKGBuilder{}

// Target implements installer.Builder.
func (PKGBuilder) Target() string { return "macos" }

// OutputPath implements installer.Builder.
func (PKGBuilder) OutputPath(spec installer.Spec) string {
	return filepath.Join(spec.OutDir, spec.Name+".pkg")
}

// Validate implements installer.Builder.
func (PKGBuilder) Validate(spec installer.Spec) error {
	if spec.Name == "" {
		return errors.New("macos installer: app name is required")
	}
	if spec.ID == "" {
		return errors.New("macos installer: app ID is required")
	}
	if spec.Version == "" {
		return errors.New("macos installer: app version is required")
	}
	if spec.Artifact == "" {
		return errors.New("macos installer: .app bundle path is required")
	}
	if spec.OutDir == "" {
		return errors.New("macos installer: output directory is required")
	}
	if loc := spec.MacOS.InstallLocation; loc != "" && !strings.HasPrefix(loc, "/") {
		return fmt.Errorf("macos installer: install_location %q must be an absolute path, such as /Applications", loc)
	}
	if spec.License != "" {
		if _, err := os.Stat(spec.License); err != nil {
			return fmt.Errorf("installer license %s: %w", spec.License, err)
		}
	}
	return nil
}

// Build implements installer.Builder, creating Name.pkg from spec.Artifact.
func (b PKGBuilder) Build(ctx context.Context, spec installer.Spec) (installer.Result, error) {
	if err := b.Validate(spec); err != nil {
		return installer.Result{}, err
	}
	loc := spec.MacOS.InstallLocation
	if loc == "" {
		loc = "/Applications"
	}
	out := b.OutputPath(spec)
	err := MakePKG(ctx, PKG{
		App:             spec.Artifact,
		Out:             out,
		ID:              spec.ID,
		Version:         spec.Version,
		MinOS:           spec.MacOS.MinOS,
		InstallLocation: loc,
		Scripts:         spec.MacOS.Scripts,
		License:         spec.License,
	})
	if err != nil {
		return installer.Result{}, err
	}
	return installer.Result{Path: out}, nil
}
