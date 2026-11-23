// Package windows makes what ships a ggui application to Windows: the
// resources linked into Name.exe, its icon, its version information and
// its application manifest, as a COFF object the go command links in
// from a .syso file in the main package.
//
// The object is written in pure Go, as the macOS icon is, so that a
// Windows program cross-built on macOS or Linux is the same as one built
// on Windows, with no rc.exe, windres or go generate step. What a
// release takes past that, an installer and a signature, is not made
// here: the program is ready for either.
package windows

import (
	"fmt"
	"image/png"
	"os"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/ico"
)

// Resources says what Name.exe carries.
type Resources struct {
	Arch string // GOARCH
	// Name is the name people see, Name.exe's; Executable the main
	// package directory's, its internal name.
	Name, Executable string
	// OriginalName is the version info's OriginalFilename; empty is
	// Name + ".exe".
	OriginalName string
	ID           string // the reverse-DNS application ID, for the manifest
	Version      string // app.version
	Description  string // FileDescription; empty is Name
	Company      string
	Copyright    string
	Icon         string // a PNG, or ""
	// ExecutionLevel is the generated manifest's requestedExecutionLevel
	// ("asInvoker" by default, or "requireAdministrator"). Ignored when
	// Manifest is set.
	ExecutionLevel string
	// Manifest is a manifest file to link instead of the generated one,
	// or "".
	Manifest string
}

// Object returns the COFF object holding r's resources, and warnings
// worth showing.
func Object(r Resources) (obj []byte, warnings []string, err error) {
	v, err := ParseVersion(r.Version)
	if err != nil {
		return nil, nil, fmt.Errorf("app.version: %w", err)
	}
	if err := CheckFileName(r.Name); err != nil {
		return nil, nil, err
	}
	desc := r.Description
	if desc == "" {
		desc = r.Name
	}
	orig := r.OriginalName
	if orig == "" {
		orig = r.Name + ".exe"
	}
	res := []resource{{rtVersion, 1, langENUS, versionInfo(Version{
		Version: v, Text: r.Version, Product: r.Name, Description: desc,
		Company: r.Company, Copyright: r.Copyright,
		Internal: r.Executable, Original: orig,
	})}}

	// The manifest's ID, 1, is the one Windows reads a program's manifest
	// from when it starts it.
	m := manifest(r.ID, v, r.ExecutionLevel)
	if r.Manifest != "" {
		if m, err = ReadManifest(r.Manifest); err != nil {
			return nil, nil, fmt.Errorf("windows.manifest: %w", err)
		}
	}
	res = append(res, resource{rtManifest, 1, langENUS, m})

	if r.Icon != "" {
		imgs, skipped, err := icon(r.Icon)
		if err != nil {
			return nil, nil, fmt.Errorf("icon %s: %w", r.Icon, err)
		}
		if len(skipped) > 0 {
			warnings = append(warnings, fmt.Sprintf("icon %s is smaller than 256×256, so the icon of %s.exe has no %v pixel images; Windows scales up the largest it has", r.Icon, r.Name, skipped))
		}
		// The group is ID 1, the first and only one: Explorer shows a
		// program's first icon group as its icon.
		for i, im := range imgs {
			res = append(res, resource{rtIcon, uint16(1 + i), langENUS, im.Data})
		}
		res = append(res, resource{rtGroupIcon, 1, langENUS, ico.Group(imgs, 1)})
	}
	sect, relocs := rsrc(res)
	obj, err = coff(r.Arch, sect, relocs)
	return obj, warnings, err
}

// icon reads the PNG at path and makes the icon's images.
func icon(path string) ([]ico.Image, []int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, nil, fmt.Errorf("not a PNG: %w", err)
	}
	return ico.Images(img)
}

// CheckFileName reports a name that cannot be a Windows file's, Name.exe.
// macOS takes nearly any name for a bundle, so app.name is checked here
// rather than with the file.
func CheckFileName(name string) error {
	if i := strings.IndexAny(name, `<>:"/\|?*`); i >= 0 {
		return fmt.Errorf("app.name %q has %q, which Windows does not allow in a file name, Name.exe: set a name without it, or build with --target native", name, name[i])
	}
	for _, r := range name {
		if r < 0x20 {
			return fmt.Errorf("app.name %q has a control character, which Windows does not allow in a file name", name)
		}
	}
	if strings.TrimRight(name, ". ") != name {
		return fmt.Errorf("app.name %q ends in a dot or space, which Windows drops from a file name", name)
	}
	base, _, _ := strings.Cut(name, ".")
	switch strings.ToUpper(strings.TrimSpace(base)) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return fmt.Errorf("app.name %q is a device's name on Windows, which no file can have", name)
	}
	return nil
}
