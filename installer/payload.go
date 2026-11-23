// Package installer implements a self-contained, ggui-based installer and
// uninstaller written in Go and packaged by `ggui build --installer` and
// `ggui release`.
//
// Create a main package (by convention ./cmd/installer or ./installer) and call
// installer.Main(&installer.App{...}). During `ggui build` and `ggui release`,
// ggui queries the package's build-time specification via GGUI_INSTALLER_DESCRIBE
// (Windows UAC scope, macOS .pkg options, license, shortcuts) and, on Windows,
// cross-compiles your package into Name-Setup.exe with the built application
// embedded automatically via -overlay. Running the same package with
// `ggui dev ./cmd/installer` opens an interactive preview in a safe sandbox so
// you can design and test custom wizard steps with hot reload.
package installer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const (
	// ScopeUser installs into %LOCALAPPDATA%\Programs\<Name> without UAC.
	ScopeUser = "user"
	// ScopeMachine installs into %ProgramFiles%\<Name> for all users (UAC).
	ScopeMachine = "machine"

	metaEntryName  = "meta.json"
	iconEntryName  = "icon.png"
	filesPrefix    = "files/"
	manifestName   = "install-manifest.json"
	uninstallerExe = "Uninstall.exe"
)

// Metadata describes the application bundled inside an installer payload.
type Metadata struct {
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

// NormalizedScope returns ScopeMachine if Scope is "machine", else ScopeUser.
func (m Metadata) NormalizedScope() string {
	if strings.EqualFold(strings.TrimSpace(m.Scope), ScopeMachine) {
		return ScopeMachine
	}
	return ScopeUser
}

// PayloadFile is one application file stored inside the embedded archive.
type PayloadFile struct {
	// RelPath is the forward-slash path relative to the install directory.
	RelPath string
	Data    []byte
}

// Payload is the decoded installer archive.
type Payload struct {
	Meta       Metadata
	Icon       image.Image
	IconPNG    []byte
	Files      []PayloadFile
	TotalBytes int64
}

// ParsePayload decodes the ZIP archive produced by ggui build.
func ParsePayload(zipBytes []byte) (*Payload, error) {
	if len(zipBytes) == 0 {
		return nil, errors.New("installer: empty payload")
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("installer: reading payload archive: %w", err)
	}
	p := &Payload{}
	var hasMeta bool
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := path.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		data, err := readZipFile(f)
		if err != nil {
			return nil, err
		}
		switch {
		case name == metaEntryName:
			if err := json.Unmarshal(data, &p.Meta); err != nil {
				return nil, fmt.Errorf("installer: decoding %s: %w", metaEntryName, err)
			}
			hasMeta = true
		case name == iconEntryName:
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return nil, fmt.Errorf("installer: decoding %s: %w", iconEntryName, err)
			}
			p.Icon = img
			p.IconPNG = data
		case strings.HasPrefix(name, filesPrefix):
			rel := strings.TrimPrefix(name, filesPrefix)
			if !validRelPath(rel) {
				return nil, fmt.Errorf("installer: unsafe payload path %q", f.Name)
			}
			p.Files = append(p.Files, PayloadFile{RelPath: rel, Data: data})
			p.TotalBytes += int64(len(data))
		}
	}
	if !hasMeta {
		return nil, fmt.Errorf("installer: payload missing %s", metaEntryName)
	}
	if strings.TrimSpace(p.Meta.Name) == "" {
		return nil, errors.New("installer: metadata missing application name")
	}
	if strings.TrimSpace(p.Meta.ID) == "" {
		return nil, errors.New("installer: metadata missing application ID")
	}
	if strings.TrimSpace(p.Meta.Executable) == "" {
		return nil, errors.New("installer: metadata missing executable name")
	}
	if len(p.Files) == 0 {
		return nil, errors.New("installer: payload contains no application files")
	}
	p.Meta.Scope = p.Meta.NormalizedScope()
	slices.SortFunc(p.Files, func(a, b PayloadFile) int {
		return strings.Compare(a.RelPath, b.RelPath)
	})
	return p, nil
}

// PackPayload creates an installer ZIP archive from meta, optional PNG icon
// bytes, and a map of relative slash-separated file paths to file contents.
func PackPayload(meta Metadata, iconPNG []byte, files map[string][]byte) ([]byte, error) {
	if strings.TrimSpace(meta.Name) == "" || strings.TrimSpace(meta.ID) == "" || strings.TrimSpace(meta.Executable) == "" {
		return nil, errors.New("installer: meta.Name, meta.ID and meta.Executable are required")
	}
	meta.Scope = meta.NormalizedScope()
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeEntry := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := writeEntry(metaEntryName, metaBytes); err != nil {
		return nil, err
	}
	if len(iconPNG) > 0 {
		if err := writeEntry(iconEntryName, iconPNG); err != nil {
			return nil, err
		}
	}
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		clean := path.Clean(strings.ReplaceAll(k, "\\", "/"))
		if !validRelPath(clean) {
			return nil, fmt.Errorf("installer: invalid payload file path %q", k)
		}
		if err := writeEntry(filesPrefix+clean, files[k]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func validRelPath(rel string) bool {
	if rel == "" || rel == "." || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || rel == ".." {
		return false
	}
	if strings.ContainsRune(rel, 0) {
		return false
	}
	return filepath.IsLocal(filepath.FromSlash(rel))
}
