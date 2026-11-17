package macos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
