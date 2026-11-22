package macos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/ironpark/ggui/cmd/ggui/internal/shared"
	"github.com/ironpark/zapp/pkg/plist"
)

// DevBundleID returns the bundle identifier for a macOS .dev.app bundle,
// appending ".dev" so it never collides with release builds. When id is
// empty it derives a reverse-DNS fallback from name.
func DevBundleID(id, name string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		part := sanitizeBundlePart(name)
		if part == "" {
			part = "app"
		}
		id = "com.example." + part
	}
	return id + ".dev"
}

func sanitizeBundlePart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) || r == '-':
			b.WriteRune(unicode.ToLower(r))
		case r == '_' || r == '.' || r == ' ':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// DevBundleSpec describes a macOS <Name>.dev.app bundle managed during ggui dev.
type DevBundleSpec struct {
	Info
	// Dir is the directory in which <Name>.dev.app is created.
	Dir string
	// Icon is the path to the icon source (PNG or SVG), or "" for no icon.
	Icon string
	// IconName is how messages refer to Icon (e.g. app.icon from ggui.toml);
	// Icon when empty.
	IconName string
	// IconInset scales the icon into Apple's 824x824 grid inside 1024x1024.
	IconInset bool
	// PrepareIcon, when set, renders an SVG Icon into a PNG at dst. It is
	// only invoked when the icon is missing or its source bytes/inset changed.
	PrepareIcon func(ctx context.Context, dst string) error
	// IconCacheDir, when set, caches encoded .icns files keyed by source
	// content hash + IconInset so subsequent ggui dev sessions skip icon
	// rendering and encoding.
	IconCacheDir string
}

// DevBundle maintains an incremental macOS <Name>.dev.app bundle across
// rebuilds. Info.plist and AppIcon.icns are only written when missing or
// changed; regular Go source rebuilds swap only Contents/MacOS/<Executable>
// (atomically with a fresh inode) and re-sign.
type DevBundle struct {
	mu      sync.Mutex
	spec    DevBundleSpec
	signing DevSigning

	// signFn overrides signDevPath in tests.
	signFn func(ctx context.Context, ds DevSigning, path string, isBundle bool, identifier string) error

	// Tracked state from the last Stage call so unchanged metadata is skipped
	// and NeedsRefresh can tell whether ggviewer needs restarting.
	staged             bool
	stagedPlist        []byte
	stagedIconKey      string
	stagedEntitlements string
	specDirty          bool
}

// NewDevBundle returns a DevBundle for spec and signing.
func NewDevBundle(spec DevBundleSpec, signing DevSigning) *DevBundle {
	return &DevBundle{
		spec:    spec,
		signing: signing,
		signFn:  signDevPath,
	}
}

// Dir returns the directory holding <Name>.dev.app.
func (b *DevBundle) Dir() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spec.Dir
}

// AppPath returns the full path of <Dir>/<Name>.dev.app.
func (b *DevBundle) AppPath() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.appPathLocked()
}

func (b *DevBundle) appPathLocked() string {
	return filepath.Join(b.spec.Dir, b.spec.Name+".dev.app")
}

// ExecutablePath returns the path to the executable inside <Name>.dev.app.
func (b *DevBundle) ExecutablePath() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return filepath.Join(b.appPathLocked(), "Contents", "MacOS", b.spec.Executable)
}

// UpdateSpec updates the bundle's mutable metadata (Info.plist fields, icon,
// entitlements) after a ggui.toml reload, keeping Dir, Name, Executable and ID
// fixed for the running session.
func (b *DevBundle) UpdateSpec(spec DevBundleSpec, entitlements string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	spec.Dir = b.spec.Dir
	spec.Name = b.spec.Name
	spec.Executable = b.spec.Executable
	spec.ID = b.spec.ID

	if b.staged {
		if b.spec.Icon != spec.Icon || b.spec.IconInset != spec.IconInset || b.signing.Entitlements != entitlements {
			b.specDirty = true
		} else if data, err := marshalInfoPlist(spec.Info, b.stagedIconKey != ""); err == nil && !bytes.Equal(data, b.stagedPlist) {
			b.specDirty = true
		}
	}
	b.spec = spec
	b.signing.Entitlements = entitlements
}

// NeedsRefresh reports whether bundle metadata or the icon source changed
// since the last Stage, so a running viewer holding <Name>.dev.app should be
// stopped and re-staged cleanly.
func (b *DevBundle) NeedsRefresh(changedPaths []string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.staged {
		return false
	}
	if b.specDirty {
		return true
	}
	if b.spec.Icon != "" && slices.Contains(changedPaths, b.spec.Icon) {
		key, err := iconFingerprint(b.spec.Icon, b.spec.IconInset)
		return err != nil || key != b.stagedIconKey
	}
	if b.signing.Entitlements != "" && slices.Contains(changedPaths, b.signing.Entitlements) {
		return true
	}
	return false
}

// Stage ensures <Name>.dev.app exists with up-to-date Info.plist, PkgInfo and
// AppIcon.icns, atomically replaces Contents/MacOS/<Executable> with srcBinary
// (creating a fresh inode so Apple Silicon AMFI never serves a stale vnode
// code signature), and re-signs the bundle. It returns the executable path
// inside the bundle to launch directly via proc.Start.
func (b *DevBundle) Stage(ctx context.Context, srcBinary string) (exePath string, warnings []string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	app := b.appPathLocked()
	contents := filepath.Join(app, "Contents")
	plistPath := filepath.Join(contents, "Info.plist")

	// Never clobber an existing file or non-bundle directory.
	if fi, err := os.Lstat(app); err == nil {
		if !fi.IsDir() {
			return "", nil, fmt.Errorf("%s exists and is not an app bundle; move it away or choose another --bundle-dir", app)
		}
		if _, err := os.Stat(plistPath); err != nil {
			return "", nil, fmt.Errorf("%s exists and is not an app bundle; move it away or choose another --bundle-dir", app)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}

	macosDir := filepath.Join(contents, "MacOS")
	resDir := filepath.Join(contents, "Resources")
	for _, d := range []string{macosDir, resDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", nil, err
		}
	}

	// PkgInfo: write only when missing or different.
	pkgInfoPath := filepath.Join(contents, "PkgInfo")
	if cur, err := os.ReadFile(pkgInfoPath); err != nil || !bytes.Equal(cur, []byte("APPL????")) {
		if err := os.WriteFile(pkgInfoPath, []byte("APPL????"), 0o644); err != nil {
			return "", nil, err
		}
	}

	// Icon: build or copy from cache only when missing or source/inset changed.
	hasIcon, iconKey, iconWarns := b.ensureIconLocked(ctx, resDir)
	warnings = append(warnings, iconWarns...)

	// Info.plist: write only when missing or bytes differ.
	plistData, err := marshalInfoPlist(b.spec.Info, hasIcon)
	if err != nil {
		return "", warnings, fmt.Errorf("writing Info.plist: %w", err)
	}
	if cur, err := os.ReadFile(plistPath); err != nil || !bytes.Equal(cur, plistData) {
		if err := os.WriteFile(plistPath, plistData, 0o644); err != nil {
			return "", warnings, err
		}
	}

	// Clean any stray files in Contents/MacOS so codesign only sees the main executable.
	if entries, err := os.ReadDir(macosDir); err == nil {
		for _, e := range entries {
			if e.Name() != b.spec.Executable {
				_ = os.RemoveAll(filepath.Join(macosDir, e.Name()))
			}
		}
	}

	// Atomically place the executable with a fresh inode (temp file + rename).
	exePath = filepath.Join(macosDir, b.spec.Executable)
	if err := stageExecutable(exePath, srcBinary); err != nil {
		return "", warnings, err
	}

	// Re-sign the bundle cleanly so _CodeSignature matches the new binary.
	if err := b.signFn(ctx, b.signing, app, true, b.spec.ID); err != nil {
		return "", warnings, err
	}

	b.staged = true
	b.stagedPlist = plistData
	b.stagedIconKey = iconKey
	b.stagedEntitlements = b.signing.Entitlements
	b.specDirty = false
	return exePath, warnings, nil
}

// SignBinary signs a standalone executable that runs outside <Name>.dev.app
// (the app binary in remote display mode, while ggviewer holds the bundle).
func (b *DevBundle) SignBinary(ctx context.Context, exePath string) error {
	b.mu.Lock()
	ds := b.signing
	id := b.spec.ID
	signFn := b.signFn
	b.mu.Unlock()
	if ds.AdHoc() && ds.Entitlements == "" {
		// Go's linker on darwin/arm64 already ad-hoc signs executables.
		return nil
	}
	return signFn(ctx, ds, exePath, false, id)
}

func (b *DevBundle) ensureIconLocked(ctx context.Context, resDir string) (hasIcon bool, iconKey string, warnings []string) {
	dst := filepath.Join(resDir, iconName+".icns")
	if b.spec.Icon == "" {
		_ = os.Remove(dst)
		return false, "", nil
	}
	name := b.spec.IconName
	if name == "" {
		name = b.spec.Icon
	}
	key, err := iconFingerprint(b.spec.Icon, b.spec.IconInset)
	if err != nil {
		_ = os.Remove(dst)
		return false, "", []string{fmt.Sprintf("icon %s: %v; continuing without a bundle icon", name, err)}
	}

	// Fast path 1: already staged in this session with identical input and file present.
	if b.staged && b.stagedIconKey == key {
		if _, err := os.Stat(dst); err == nil {
			return true, key, nil
		}
	}

	// Fast path 2: cached .icns from a previous ggui dev session.
	var cachePath string
	if b.spec.IconCacheDir != "" {
		cachePath = filepath.Join(b.spec.IconCacheDir, key+".icns")
		if cached, err := os.ReadFile(cachePath); err == nil && len(cached) > 0 {
			if cur, err := os.ReadFile(dst); err != nil || !bytes.Equal(cur, cached) {
				if err := os.WriteFile(dst, cached, 0o644); err != nil { //nolint:gosec // G703: the path is the CLI user's own argument or build output
					return false, "", []string{fmt.Sprintf("writing %s: %v; continuing without a bundle icon", dst, err)}
				}
			}
			return true, key, nil
		}
	}

	// Slow path: render SVG if needed, encode PNG -> .icns.
	pngPath := b.spec.Icon
	if b.spec.PrepareIcon != nil {
		tmpDir, err := os.MkdirTemp("", "ggui-dev-icon-")
		if err != nil {
			_ = os.Remove(dst)
			return false, "", []string{fmt.Sprintf("icon %s: %v; continuing without a bundle icon", name, err)}
		}
		defer os.RemoveAll(tmpDir)
		pngPath = filepath.Join(tmpDir, "icon.png")
		if err := b.spec.PrepareIcon(ctx, pngPath); err != nil {
			_ = os.Remove(dst)
			return false, "", []string{fmt.Sprintf("icon %s: %v; continuing without a bundle icon", name, err)}
		}
	}
	skipped, fullBleed, err := writeIcon(dst, pngPath, b.spec.IconInset)
	if err != nil {
		_ = os.Remove(dst)
		return false, "", []string{fmt.Sprintf("icon %s: %v; continuing without a bundle icon", name, err)}
	}
	if len(skipped) > 0 {
		warnings = append(warnings, fmt.Sprintf("icon %s is smaller than 1024×1024, so the icon has no %v pixel images; macOS scales the largest it has", name, skipped))
	}
	if fullBleed && !b.spec.IconInset {
		warnings = append(warnings, fmt.Sprintf("icon %s is drawn to the edges of its canvas, so it looks larger than other apps' in the Dock, which sit in the middle 824×824 of 1024; set macos.icon_inset = true to scale it in, or leave the margin in the artwork", name))
	}
	if cachePath != "" {
		if data, err := os.ReadFile(dst); err == nil {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
			_ = os.WriteFile(cachePath, data, 0o644) //nolint:gosec // G703: the path is the CLI user's own argument or build output
		}
	}
	return true, key, warnings
}

func marshalInfoPlist(in Info, hasIcon bool) ([]byte, error) {
	in.HasIcon = hasIcon
	return plist.MarshalXML(plistValue(in.Plist()))
}

func iconFingerprint(path string, inset bool) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if inset {
		_, _ = h.Write([]byte("inset=1\n"))
	} else {
		_, _ = h.Write([]byte("inset=0\n"))
	}
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))[:24], nil
}

// stageExecutable writes src into a new file in dst's directory and renames it
// over dst so the target always receives a fresh inode.
func stageExecutable(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".ggui-stage-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := shared.Rename(tmpName, dst); err != nil {
		return err
	}
	ok = true
	return nil
}

func readEntitlements(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the entitlements: %w", err)
	}
	return data, nil
}
