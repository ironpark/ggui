package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

const (
	// DescribeEnv, when set to a file path by `ggui build` or `ggui release`,
	// instructs Main/Run to write the App's build-time Description JSON to
	// that file and return immediately without opening a window.
	DescribeEnv = "GGUI_INSTALLER_DESCRIBE"

	describeProtocol = 1
)

var (
	embeddedMu      sync.RWMutex
	embeddedPayload []byte
)

// SetEmbeddedPayload registers the archive embedded by `ggui build`.
// During `ggui build --target windows --installer`, ggui injects an init()
// function via -overlay that calls SetEmbeddedPayload with the packaged
// application archive.
func SetEmbeddedPayload(data []byte) {
	embeddedMu.Lock()
	embeddedPayload = data
	embeddedMu.Unlock()
}

// EmbeddedPayload returns the archive registered by SetEmbeddedPayload, or nil
// when running outside a `ggui build` (such as under `ggui dev`).
func EmbeddedPayload() []byte {
	embeddedMu.RLock()
	defer embeddedMu.RUnlock()
	return embeddedPayload
}

// Bool returns a pointer to v, for tri-state boolean fields on WindowsConfig.
func Bool(v bool) *bool { return &v }

// App defines a Go-authored installer and uninstaller for a ggui application.
//
// Pass an *App to Main or Run inside your installer main package (for example
// ./cmd/installer or ./installer). Passing nil runs the default standard
// setup wizard with sensible defaults.
type App struct {
	// Payload overrides the archive injected by `ggui build`. Leave nil in
	// normal usage so `ggui build` injects the packaged app automatically and
	// `ggui dev` runs in interactive preview mode.
	Payload []byte

	// Optional metadata overrides (also used in `ggui dev` preview mode).
	// When `ggui build` embeds a payload, non-empty values here override
	// the corresponding values from ggui.toml.
	Name        string
	ID          string
	Version     string
	Description string
	Publisher   string
	Copyright   string
	Homepage    string

	// License is plain-text license copy (for example via //go:embed LICENSE)
	// shown and accepted during installation on both Windows and macOS.
	License string
	// LicenseFile is an optional path (relative to the project root) to a
	// plain-text license file, used when License is empty.
	LicenseFile string

	// Platform-specific installer configuration.
	Windows WindowsConfig
	MacOS   MacOSConfig

	// UI customization for the ggui setup wizard.
	Title     string         // window title override (default "<Name> Setup")
	Width     int            // logical window width (default 540)
	Height    int            // logical window height (default 390)
	Resizable bool           // whether the setup window is resizable
	Theme     *uitheme.Theme // light theme (nil uses uitheme.Default())
	DarkTheme *uitheme.Theme // dark theme (nil follows Theme if set, else uitheme.Dark())

	// Header optionally replaces the default icon + title banner at the top
	// of the wizard window.
	Header func(ctx *Context) ggui.Widget

	// BuildUI optionally replaces the entire wizard window contents with a
	// fully custom ggui widget tree driven by ctx.
	BuildUI func(ctx *Context) ggui.Widget

	// Steps customizes the install wizard flow. Nil uses the default flow:
	// [LicenseStep, LocationStep, ProgressStep, FinishStep].
	Steps []Step

	// UninstallSteps customizes the uninstall wizard flow. Nil uses the
	// default flow: [UninstallConfirmStep, ProgressStep, FinishStep].
	UninstallSteps []Step

	// Lifecycle hooks run on a worker goroutine during both GUI and --silent
	// installs/uninstalls. Read reactive UI state with ggui.Peek and report
	// progress via ctx.ReportProgress (or UI mutations via ctx.Post).
	BeforeInstall   func(ctx *Context) error
	AfterInstall    func(ctx *Context) error
	BeforeUninstall func(ctx *Context) error
	AfterUninstall  func(ctx *Context) error
}

// WindowsConfig configures the Windows Name-Setup.exe installer.
type WindowsConfig struct {
	// Disabled opts Windows out when building installers (default false = enabled).
	Disabled bool
	// Scope is ScopeUser ("%LOCALAPPDATA%\Programs\<Name>", no UAC, default)
	// or ScopeMachine ("%ProgramFiles%\<Name>", requires administrator UAC).
	Scope string
	// InstallDir overrides the default install directory (may contain Windows
	// environment variables such as %LOCALAPPDATA% or %ProgramFiles%).
	InstallDir string
	// License overrides App.License for Windows.
	License string
	// LicenseFile overrides App.LicenseFile for Windows.
	LicenseFile string
	// Publisher overrides App.Publisher / windows.company in Add/Remove Programs.
	Publisher string
	// URL overrides App.Homepage / linux.homepage in Add/Remove Programs.
	URL string
	// StartMenuShortcut defaults to true when nil. Use Bool(false) to default unchecked.
	StartMenuShortcut *bool
	// DesktopShortcut defaults to false when nil. Use Bool(true) to default checked.
	DesktopShortcut *bool
	// LaunchAfter defaults to true when nil. Use Bool(false) to default unchecked.
	LaunchAfter *bool
	// AddToPath adds the install directory to PATH (user or machine, matching
	// Scope) during installation and removes it automatically on uninstall.
	AddToPath bool
	// Env sets environment variables (matching Scope) during installation and
	// removes them automatically on uninstall. Values may reference
	// %INSTALL_DIR%, %EXE_PATH%, %APP_NAME%, %APP_ID%, and %VERSION%.
	Env map[string]string
	// Registry writes Windows Registry values during installation and removes
	// them automatically on uninstall. String values may reference
	// %INSTALL_DIR%, %EXE_PATH%, %APP_NAME%, %APP_ID%, and %VERSION%.
	Registry []RegistryEntry
}

// MacOSConfig configures the macOS Name.pkg installer generated by
// `ggui build --target macos` and `ggui release`.
type MacOSConfig struct {
	// Disabled opts macOS out when building installers (default false = enabled).
	Disabled bool
	// SigningIdentity names the Developer ID Installer identity; empty uses
	// the keychain's or certificate's default Developer ID Installer identity.
	SigningIdentity string
	// InstallLocation is the absolute destination directory ("/Applications" by default).
	InstallLocation string
	// Scripts is a project-relative (or absolute) directory holding
	// preinstall/postinstall scripts.
	Scripts string
	// License overrides App.License for macOS .pkg.
	License string
	// LicenseFile overrides App.LicenseFile for macOS .pkg.
	LicenseFile string
}

// Description is the versioned JSON payload written to DescribeEnv so
// `ggui build` and `ggui release` read build-time installer configuration
// directly from Go code without duplicating settings in ggui.toml.
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

// WindowsDescription is the serialized build-time Windows installer specification.
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

// MacOSDescription is the serialized build-time macOS .pkg specification.
type MacOSDescription struct {
	Enabled         bool   `json:"enabled"`
	SigningIdentity string `json:"signing_identity,omitempty"`
	InstallLocation string `json:"install_location,omitempty"`
	Scripts         string `json:"scripts,omitempty"`
	License         string `json:"license,omitempty"`
	LicenseFile     string `json:"license_file,omitempty"`
}

// Describe returns the resolved build-time description for a (or defaults when
// a is nil).
func (a *App) Describe() Description {
	if a == nil {
		a = &App{}
	}
	scope := strings.TrimSpace(a.Windows.Scope)
	if scope == "" {
		scope = ScopeUser
	}
	winLic := a.Windows.License
	winLicFile := a.Windows.LicenseFile
	if winLic == "" && winLicFile == "" {
		winLic = a.License
		winLicFile = a.LicenseFile
	}
	macLic := a.MacOS.License
	macLicFile := a.MacOS.LicenseFile
	if macLic == "" && macLicFile == "" {
		macLic = a.License
		macLicFile = a.LicenseFile
	}
	macLoc := strings.TrimSpace(a.MacOS.InstallLocation)
	if macLoc == "" {
		macLoc = "/Applications"
	}
	winPub := a.Windows.Publisher
	if winPub == "" {
		winPub = a.Publisher
	}
	winURL := a.Windows.URL
	if winURL == "" {
		winURL = a.Homepage
	}
	return Description{
		V:           describeProtocol,
		Name:        a.Name,
		ID:          a.ID,
		Version:     a.Version,
		Description: a.Description,
		Publisher:   a.Publisher,
		Copyright:   a.Copyright,
		Homepage:    a.Homepage,
		License:     a.License,
		LicenseFile: a.LicenseFile,
		Windows: WindowsDescription{
			Enabled:           !a.Windows.Disabled,
			Scope:             scope,
			InstallDir:        a.Windows.InstallDir,
			License:           winLic,
			LicenseFile:       winLicFile,
			Publisher:         winPub,
			URL:               winURL,
			StartMenuShortcut: boolOr(a.Windows.StartMenuShortcut, true),
			DesktopShortcut:   boolOr(a.Windows.DesktopShortcut, false),
			LaunchAfter:       boolOr(a.Windows.LaunchAfter, true),
		},
		MacOS: MacOSDescription{
			Enabled:         !a.MacOS.Disabled,
			SigningIdentity: a.MacOS.SigningIdentity,
			InstallLocation: macLoc,
			Scripts:         a.MacOS.Scripts,
			License:         macLic,
			LicenseFile:     macLicFile,
		},
	}
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func writeDescription(path string, app *App) error {
	data, err := json.MarshalIndent(app.Describe(), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (a *App) effectiveWindowsLicense() string {
	if a == nil {
		return ""
	}
	if a.Windows.License != "" {
		return a.Windows.License
	}
	return a.License
}

func (a *App) effectiveWindowsLicenseFile() string {
	if a == nil {
		return ""
	}
	if a.Windows.LicenseFile != "" {
		return a.Windows.LicenseFile
	}
	return a.LicenseFile
}

// resolvePayload returns the decoded payload and whether the installer is
// running in interactive preview mode (no embedded archive present, e.g. under
// `ggui dev`).
func resolvePayload(app *App) (*Payload, bool, error) {
	if app == nil {
		app = &App{}
	}
	raw := app.Payload
	if len(raw) == 0 {
		raw = EmbeddedPayload()
	}
	if len(raw) == 0 {
		return buildPreviewPayload(app), true, nil
	}
	p, err := ParsePayload(raw)
	if err != nil {
		return nil, false, err
	}
	applyAppOverrides(&p.Meta, app)
	return p, false, nil
}

func applyAppOverrides(meta *Metadata, app *App) {
	if app == nil {
		return
	}
	if app.Name != "" {
		meta.Name = app.Name
	}
	if app.ID != "" {
		meta.ID = app.ID
	}
	if app.Version != "" {
		meta.Version = app.Version
	}
	if app.Description != "" {
		meta.Description = app.Description
	}
	if app.Windows.Publisher != "" {
		meta.Publisher = app.Windows.Publisher
	} else if app.Publisher != "" {
		meta.Publisher = app.Publisher
	}
	if app.Copyright != "" {
		meta.Copyright = app.Copyright
	}
	if app.Windows.URL != "" {
		meta.Homepage = app.Windows.URL
	} else if app.Homepage != "" {
		meta.Homepage = app.Homepage
	}
	if strings.TrimSpace(app.Windows.Scope) != "" {
		meta.Scope = Metadata{Scope: app.Windows.Scope}.NormalizedScope()
	}
	if app.Windows.InstallDir != "" {
		meta.InstallDir = app.Windows.InstallDir
	}
	if lic := app.effectiveWindowsLicense(); lic != "" {
		meta.License = lic
	}
	if app.Windows.StartMenuShortcut != nil {
		meta.StartMenuShortcut = *app.Windows.StartMenuShortcut
	}
	if app.Windows.DesktopShortcut != nil {
		meta.DesktopShortcut = *app.Windows.DesktopShortcut
	}
	if app.Windows.LaunchAfter != nil {
		meta.LaunchAfter = *app.Windows.LaunchAfter
	}
}

func buildPreviewPayload(app *App) *Payload {
	desc := app.Describe()
	name := desc.Name
	if name == "" {
		name = "My App"
	}
	id := desc.ID
	if id == "" {
		id = "com.example.app"
	}
	version := desc.Version
	if version == "" {
		version = "1.0.0-dev"
	}
	description := desc.Description
	if description == "" {
		description = "Interactive installer preview (no files will be installed outside a temporary sandbox)."
	}
	lic := desc.Windows.License
	if lic == "" {
		if path := app.effectiveWindowsLicenseFile(); path != "" {
			if raw, err := readPreviewFile(path); err == nil {
				lic = string(raw)
			}
		}
	}
	exe := name + ".exe"
	sampleExe := []byte("ggui installer preview binary")
	return &Payload{
		Meta: Metadata{
			Name:              name,
			ID:                id,
			Version:           version,
			Executable:        exe,
			Description:       description,
			Publisher:         desc.Windows.Publisher,
			Copyright:         desc.Copyright,
			Homepage:          desc.Windows.URL,
			Scope:             desc.Windows.Scope,
			InstallDir:        desc.Windows.InstallDir,
			License:           lic,
			StartMenuShortcut: desc.Windows.StartMenuShortcut,
			DesktopShortcut:   desc.Windows.DesktopShortcut,
			LaunchAfter:       desc.Windows.LaunchAfter,
		},
		Files: []PayloadFile{
			{RelPath: exe, Data: sampleExe},
		},
		TotalBytes: int64(len(sampleExe)),
	}
}

// readPreviewFile resolves path relative to the current working directory or
// walks up to the module root (go.mod) so `ggui dev ./cmd/installer` finds
// project-relative files such as LICENSE.
func readPreviewFile(path string) ([]byte, error) {
	if filepath.IsAbs(path) {
		return os.ReadFile(path)
	}
	if data, err := os.ReadFile(path); err == nil {
		return data, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for {
		candidate := filepath.Join(dir, path)
		if data, err := os.ReadFile(candidate); err == nil {
			return data, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return nil, os.ErrNotExist
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, os.ErrNotExist
		}
		dir = parent
	}
}
