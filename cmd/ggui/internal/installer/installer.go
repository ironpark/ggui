// Package installer defines the shared interface, discovery, Go-code
// description protocol, and specification for platform installers built by
// ggui build and ggui release (macOS .pkg and Windows Name-Setup.exe).
package installer

import "context"

// Scope values for Windows installers.
const (
	ScopeUser    = "user"
	ScopeMachine = "machine"
)

// Spec describes an application and the installer to build around it.
// Platform builders read the shared fields together with their own
// platform options (MacOS or Windows).
type Spec struct {
	Name       string // display name (e.g. "Hello World")
	Executable string // binary / main package name (e.g. "hello")
	ID         string // reverse-DNS identifier (e.g. "com.example.hello")
	Version    string // app.version (e.g. "1.2.0")
	Arch       string // target GOARCH, or "universal" on macOS

	ProjectDir   string            // module root directory
	Artifact     string            // primary built artifact (Name.app on macOS, Name.exe on Windows)
	ExtraFiles   []string          // additional top-level files shipped beside Artifact (e.g. ggfx_driver.dll)
	ExtraEntries map[string]string // additional files shipped at relative slash paths inside the install dir (e.g. "bin/tool.exe" -> "/path/to/tool.exe")
	OutDir       string            // directory where the installer file is written

	// Package is the user's Go main package defining the installer with
	// github.com/ironpark/ggui/installer, or nil for the built-in default.
	Package *Package

	Description string // short description
	Publisher   string // publisher / company name
	Copyright   string // copyright notice
	Homepage    string // project URL
	Icon        string // resolved PNG icon path, or ""
	License     string // resolved path to a plain-text license file, or ""
	LicenseText string // inline plain-text license copy (takes precedence over License)

	MacOS   MacOSOptions
	Windows WindowsOptions

	// Logf, when set, receives build command lines or progress notes.
	Logf func(format string, args ...any)
}

// MacOSOptions configures macOS .pkg installer generation.
type MacOSOptions struct {
	MinOS           string // LSMinimumSystemVersion
	InstallLocation string // destination directory, "/Applications" by default
	Scripts         string // resolved directory with preinstall/postinstall scripts, or ""
}

// WindowsOptions configures Windows Name-Setup.exe generation.
type WindowsOptions struct {
	Scope             string // "user" (default) or "machine"
	InstallLocation   string // optional default install directory (may use %LOCALAPPDATA%, %ProgramFiles%)
	StartMenuShortcut bool
	DesktopShortcut   bool
	LaunchAfter       bool
}

// Result describes the built installer artifact and any non-fatal warnings.
type Result struct {
	Path     string
	Warnings []string
}

// Builder builds a platform-specific installer from a Spec.
type Builder interface {
	// Target returns the platform target name ("macos", "windows", ...).
	Target() string
	// OutputPath returns the installer file path Build writes for spec.
	OutputPath(spec Spec) string
	// Validate checks spec before assembling or compiling the installer.
	Validate(spec Spec) error
	// Build creates the installer described by spec.
	Build(ctx context.Context, spec Spec) (Result, error)
}
