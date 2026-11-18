package linux

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/icns"
)

// IconSizes are the hicolor theme's fixed sizes an icon is installed at,
// those desktops look up for menus, docks, task switchers and software
// centres, which take the nearest for any other.
var IconSizes = []int{16, 24, 32, 48, 64, 128, 256, 512}

// MinIconSize is the smallest source icon accepted, the macOS icon's
// minimum, so that one PNG serves every target.
const MinIconSize = icns.MinSize

// Tree says what to assemble.
type Tree struct {
	App
	Binary string // the built program
	// Icon is a square PNG to scale into the hicolor sizes, or "".
	Icon string
	// IconName is what messages call the icon, as for a PNG drawn from
	// an SVG; Icon when "".
	IconName string
	// SVG is the SVG the icon was drawn from, or "", installed as the
	// scalable icon.
	SVG    string
	OutDir string // the directory the tree is made in
}

// TreeName is the install tree's directory name: Package-linux-Arch.
func TreeName(a App) string { return a.Package + "-linux-" + a.Arch }

// Paths in the tree, relative to it.
func binPath(a App) string     { return filepath.Join("usr", "bin", a.Package) }
func desktopPath(a App) string { return filepath.Join("usr", "share", "applications", a.ID+".desktop") }
func metainfoPath(a App) string {
	return filepath.Join("usr", "share", "metainfo", a.ID+".metainfo.xml")
}
func iconPath(a App, size int) string {
	s := strconv.Itoa(size)
	return filepath.Join("usr", "share", "icons", "hicolor", s+"x"+s, "apps", a.ID+".png")
}
func svgPath(a App) string {
	return filepath.Join("usr", "share", "icons", "hicolor", "scalable", "apps", a.ID+".svg")
}

// Assemble makes OutDir/Package-linux-Arch, replacing one already there:
// the tree a package installs under /, with the program at mode 0755 and
// every other file at 0644. It returns the directory and any warnings
// worth showing.
func Assemble(t Tree) (dir string, warnings []string, err error) {
	if t.Package == "" || t.ID == "" || t.Arch == "" {
		return "", nil, errors.New("linux: the package name, ID and architecture must be set")
	}
	dir = filepath.Join(t.OutDir, TreeName(t.App))
	if err := os.RemoveAll(dir); err != nil {
		return "", nil, err
	}
	write := func(rel string, data []byte, mode fs.FileMode) error {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, data, mode); err != nil {
			return err
		}
		// WriteFile's mode is cut by the umask; the tree's modes are
		// what the archives record, so they are set as given.
		return os.Chmod(p, mode)
	}

	bin, err := os.ReadFile(t.Binary)
	if err != nil {
		return "", nil, err
	}
	if err := write(binPath(t.App), bin, 0o755); err != nil {
		return "", nil, err
	}

	hasIcon := t.Icon != ""
	if hasIcon {
		name := t.IconName
		if name == "" {
			name = t.Icon
		}
		icons, skipped, err := Icons(t.Icon)
		if err != nil {
			return "", nil, fmt.Errorf("icon %s: %w", name, err)
		}
		for _, ic := range icons {
			if err := write(iconPath(t.App, ic.Size), ic.PNG, 0o644); err != nil {
				return "", nil, err
			}
		}
		if len(skipped) > 0 {
			var s []string
			for _, n := range skipped {
				s = append(s, strconv.Itoa(n))
			}
			warnings = append(warnings, fmt.Sprintf("icon %s is smaller than %s pixels, so the hicolor theme has no icon at that size, and desktops scale a smaller one up", name, strings.Join(s, ", ")))
		}
		if t.SVG != "" {
			svg, err := os.ReadFile(t.SVG)
			if err != nil {
				return "", nil, err
			}
			if err := write(svgPath(t.App), svg, 0o644); err != nil {
				return "", nil, err
			}
		}
	}
	if err := write(desktopPath(t.App), DesktopEntry(t.App, hasIcon), 0o644); err != nil {
		return "", nil, err
	}
	if err := write(metainfoPath(t.App), Metainfo(t.App), 0o644); err != nil {
		return "", nil, err
	}
	return dir, warnings, nil
}

// Icon is one size of the icon, as a PNG file.
type Icon struct {
	Size int
	PNG  []byte
}

// Icons scales the square PNG at path to each of IconSizes up to its own
// size, by the same resizer as the macOS and Windows icons. Larger sizes
// are left out rather than scaled up, and returned so the caller can say
// so.
func Icons(path string) (icons []Icon, skipped []int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	src, err := png.Decode(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, nil, err
	}
	return scaleIcons(src)
}

func scaleIcons(src image.Image) (icons []Icon, skipped []int, err error) {
	b := src.Bounds()
	if b.Dx() != b.Dy() {
		return nil, nil, fmt.Errorf("icon is %d×%d; it must be square, ideally 1024×1024", b.Dx(), b.Dy())
	}
	if b.Dx() < MinIconSize {
		return nil, nil, fmt.Errorf("icon is %d×%d; it must be at least %d×%d, ideally 1024×1024", b.Dx(), b.Dy(), MinIconSize, MinIconSize)
	}
	for _, size := range IconSizes {
		if size > b.Dx() {
			skipped = append(skipped, size)
			continue
		}
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, icns.Resize(src, size)); err != nil {
			return nil, nil, err
		}
		icons = append(icons, Icon{Size: size, PNG: buf.Bytes()})
	}
	return icons, skipped, nil
}
