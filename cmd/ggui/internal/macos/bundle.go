// Package macos assembles a macOS application bundle around a built
// program: the Name.app directory, its Info.plist and icon, and the code
// signature.
package macos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/icns"
	"github.com/ironpark/ggui/cmd/ggui/internal/plist"
)

// Info is what goes in the bundle's Info.plist.
type Info struct {
	Name                 string // CFBundleName and the bundle's file name
	Executable           string // the program's file name in Contents/MacOS
	ID                   string
	Version              string
	MinimumSystemVersion string
	Category             string
	HasIcon              bool
	// Extra keys, written over the generated ones, so a project can set
	// anything ggui has no field for and override what it guessed.
	Extra map[string]any
}

// iconName is the icon file's name in Contents/Resources, without the
// extension, as CFBundleIconFile names it.
const iconName = "AppIcon"

// Plist returns the Info.plist dictionary.
func (in Info) Plist() map[string]any {
	d := map[string]any{
		"CFBundleDevelopmentRegion":     "en",
		"CFBundleDisplayName":           in.Name,
		"CFBundleExecutable":            in.Executable,
		"CFBundleIdentifier":            in.ID,
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleName":                  in.Name,
		"CFBundlePackageType":           "APPL",
		"CFBundleShortVersionString":    in.Version,
		"CFBundleVersion":               in.Version,
		// Without it macOS draws the window at 1x and scales it up.
		"NSHighResolutionCapable": true,
		// macOS otherwise infers from the SDK version recorded in the
		// binary whether the app may take the dark look; saying so here
		// does not depend on what the Go linker records.
		"NSRequiresAquaSystemAppearance": false,
	}
	if in.MinimumSystemVersion != "" {
		d["LSMinimumSystemVersion"] = in.MinimumSystemVersion
	}
	if in.Category != "" {
		d["LSApplicationCategoryType"] = in.Category
	}
	if in.HasIcon {
		d["CFBundleIconFile"] = iconName
	}
	maps.Copy(d, in.Extra)
	return d
}

// Bundle says what to assemble.
type Bundle struct {
	Info
	Binary string // the built program, copied into the bundle
	Icon   string // a PNG, or ""
	OutDir string // the directory Name.app is made in
}

// Assemble makes OutDir/Name.app, replacing one already there, and returns
// its path and any warnings worth showing.
func Assemble(b Bundle) (app string, warnings []string, err error) {
	app = filepath.Join(b.OutDir, b.Name+".app")
	if err := removeBundle(app); err != nil {
		return "", nil, err
	}
	contents := filepath.Join(app, "Contents")
	for _, d := range []string{"MacOS", "Resources"} {
		if err := os.MkdirAll(filepath.Join(contents, d), 0o755); err != nil {
			return "", nil, err
		}
	}
	if err := copyFile(filepath.Join(contents, "MacOS", b.Executable), b.Binary, 0o755); err != nil {
		return "", nil, err
	}
	if b.Icon != "" {
		skipped, err := writeIcon(filepath.Join(contents, "Resources", iconName+".icns"), b.Icon)
		if err != nil {
			return "", nil, fmt.Errorf("icon %s: %w", b.Icon, err)
		}
		if len(skipped) > 0 {
			warnings = append(warnings, fmt.Sprintf("icon %s is smaller than 1024×1024, so the icon has no %v pixel images; macOS scales the largest it has", b.Icon, skipped))
		}
		b.HasIcon = true
	}
	data, err := plist.Marshal(b.Plist())
	if err != nil {
		return "", nil, fmt.Errorf("writing Info.plist: %w", err)
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), data, 0o644); err != nil {
		return "", nil, err
	}
	// PkgInfo predates Info.plist, but Finder still reads it.
	if err := os.WriteFile(filepath.Join(contents, "PkgInfo"), []byte("APPL????"), 0o644); err != nil {
		return "", nil, err
	}
	return app, warnings, nil
}

// removeBundle removes a bundle a previous build made, and refuses to
// remove anything else that happens to have the name.
func removeBundle(app string) error {
	if _, err := os.Lstat(app); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if _, err := os.Stat(filepath.Join(app, "Contents", "Info.plist")); err != nil {
		return fmt.Errorf("%s exists and is not an app bundle; move it away or choose another --out", app)
	}
	return os.RemoveAll(app)
}

func writeIcon(dst, src string) ([]int, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("not a PNG: %w", err)
	}
	var buf bytes.Buffer
	skipped, err := icns.Encode(&buf, img)
	if err != nil {
		return nil, err
	}
	return skipped, os.WriteFile(dst, buf.Bytes(), 0o644)
}

func copyFile(dst, src string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// CanSign reports whether this host can sign a bundle: codesign is part of
// macOS and nothing else.
func CanSign() bool { return runtime.GOOS == "darwin" }

// Sign signs the bundle with identity, "-" for ad hoc. A real identity
// also gets the hardened runtime and a secure timestamp, which
// notarization requires; ad hoc signing gets neither, because it is for
// running on this Mac, and the timestamp needs Apple's server.
func Sign(ctx context.Context, app, identity, entitlements string) error {
	args := []string{"--force", "--sign", identity}
	if identity != "-" {
		args = append(args, "--options", "runtime", "--timestamp")
	}
	if entitlements != "" {
		args = append(args, "--entitlements", entitlements)
	}
	args = append(args, app)
	out, err := exec.CommandContext(ctx, "codesign", args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "no identity found") {
			msg += "\nlist the identities this Mac has with: security find-identity -v -p codesigning"
		}
		return fmt.Errorf("codesign: %s (%w)", msg, err)
	}
	return nil
}

// Lipo joins programs built for different architectures into one.
func Lipo(ctx context.Context, out string, inputs ...string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("a universal build joins its halves with lipo, which only macOS has: build on a Mac, or drop --universal and set GOARCH")
	}
	args := append([]string{"-create", "-output", out}, inputs...)
	if msg, err := exec.CommandContext(ctx, "lipo", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("lipo: %s (%w); lipo comes with the Xcode command line tools: xcode-select --install", strings.TrimSpace(string(msg)), err)
	}
	return nil
}
