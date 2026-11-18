// Package macos makes what ships a ggui application to Macs: the Name.app
// bundle around a built program, with its Info.plist and icon; its code
// signature; and the disk image and installer package made from it,
// signed, notarized and verified.
//
// The work past assembling the bundle is done by zapp's packages
// (github.com/ironpark/zapp/pkg/...): signing and notarizing through
// Apple's tools, on a Mac only;
// the disk image and the installer in pure Go; and the checks a release
// should pass.
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
	"github.com/ironpark/zapp/pkg/plist"
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
	// IconName is what messages call the icon, as for a PNG drawn from
	// an SVG; Icon when "".
	IconName string
	// IconInset scales the icon into the body of Apple's icon grid, 824
	// of 1024 pixels, for artwork drawn edge to edge.
	IconInset bool
	OutDir    string // the directory Name.app is made in
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
		name := b.IconName
		if name == "" {
			name = b.Icon
		}
		skipped, fullBleed, err := writeIcon(filepath.Join(contents, "Resources", iconName+".icns"), b.Icon, b.IconInset)
		if err != nil {
			return "", nil, fmt.Errorf("icon %s: %w", name, err)
		}
		if len(skipped) > 0 {
			warnings = append(warnings, fmt.Sprintf("icon %s is smaller than 1024×1024, so the icon has no %v pixel images; macOS scales the largest it has", name, skipped))
		}
		if fullBleed && !b.IconInset {
			warnings = append(warnings, fmt.Sprintf("icon %s is drawn to the edges of its canvas, so it looks larger than other apps' in the Dock, which sit in the middle 824×824 of 1024; set macos.icon_inset = true to scale it in, or leave the margin in the artwork", name))
		}
		b.HasIcon = true
	}
	data, err := plist.MarshalXML(plistValue(b.Plist()))
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

// plistValue returns v with every list as []any, the one kind of list
// zapp's plist writer takes: TOML gives an array of tables as
// []map[string]any.
func plistValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(v))
		for k, e := range v {
			m[k] = plistValue(e)
		}
		return m
	case []map[string]any:
		l := make([]any, len(v))
		for i, e := range v {
			l[i] = plistValue(e)
		}
		return l
	case []any:
		l := make([]any, len(v))
		for i, e := range v {
			l[i] = plistValue(e)
		}
		return l
	case []string:
		l := make([]any, len(v))
		for i, e := range v {
			l[i] = e
		}
		return l
	}
	return v
}

// WriteIcon writes the .icns made from the PNG src to dst, and returns
// the pixel sizes it lacks because src is smaller.
func WriteIcon(dst, src string) ([]int, error) {
	skipped, _, err := writeIcon(dst, src, false)
	return skipped, err
}

// writeIcon is WriteIcon, scaling src into Apple's icon grid first when
// inset is set, and also reports whether src is drawn edge to edge.
func writeIcon(dst, src string, inset bool) (skipped []int, fullBleed bool, err error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, false, fmt.Errorf("not a PNG: %w", err)
	}
	if b := img.Bounds(); b.Dx() == b.Dy() {
		fullBleed = icns.FullBleed(img)
		if inset {
			img = icns.Inset(img)
		}
	}
	var buf bytes.Buffer
	skipped, err = icns.Encode(&buf, img)
	if err != nil {
		return nil, false, err
	}
	return skipped, fullBleed, os.WriteFile(dst, buf.Bytes(), 0o644)
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
