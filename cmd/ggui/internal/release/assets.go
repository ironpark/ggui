// Package release collects what ggui release builds into the files a
// release publishes, and holds what its channels share: the asset names,
// the checksums, the git and GitHub details a release is made from.
package release

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// SumsName is the checksum file published beside the assets.
const SumsName = "SHA256SUMS"

// Asset is one file of a release.
type Asset struct {
	Name   string // the file's name, which is the asset's
	Path   string // where it is
	Target string // macos, windows or linux; "" for SHA256SUMS
	// Arch is amd64, arm64, 386, riscv64, or universal for a macOS
	// build of both.
	Arch   string
	Size   int64
	SHA256 string // hex
}

// Slug makes a file-name-safe, lowercase form of an app's name: letters
// and digits, with hyphens between words, so that an asset's URL needs no
// escaping.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		default:
			dash = true
		}
	}
	return b.String()
}

// MacName is the macOS asset's name: slug-version-macos-arch.ext, ext dmg
// or zip.
func MacName(slug, version, arch, ext string) string {
	return slug + "-" + version + "-macos-" + arch + "." + ext
}

// WindowsName is the Windows asset's name, a zip holding the program.
func WindowsName(slug, version, arch string) string {
	return slug + "-" + version + "-windows-" + arch + ".zip"
}

// WindowsSetupName is the Windows installer asset's name.
func WindowsSetupName(slug, version, arch string) string {
	return slug + "-" + version + "-windows-" + arch + "-setup.exe"
}

// Hash fills in a's size and SHA-256 from its file.
func (a *Asset) Hash() error {
	f, err := os.Open(a.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	a.Size, a.SHA256 = n, hex.EncodeToString(h.Sum(nil))
	return nil
}

// WriteSums writes SHA256SUMS in dir for assets, which must be hashed, in
// the format sha256sum -c reads, sorted by name, and returns it as an
// asset itself.
func WriteSums(dir string, assets []Asset) (Asset, error) {
	sorted := slices.Clone(assets)
	slices.SortFunc(sorted, func(a, b Asset) int { return strings.Compare(a.Name, b.Name) })
	var b strings.Builder
	for _, a := range sorted {
		if a.SHA256 == "" {
			return Asset{}, fmt.Errorf("%s is not hashed", a.Name)
		}
		fmt.Fprintf(&b, "%s  %s\n", a.SHA256, a.Name)
	}
	s := Asset{Name: SumsName, Path: filepath.Join(dir, SumsName)}
	if err := os.WriteFile(s.Path, []byte(b.String()), 0o644); err != nil {
		return Asset{}, err
	}
	return s, s.Hash()
}

// ZipDir writes the directory src to out as a zip whose entries start
// with src's own name, as ditto -c -k --keepParent writes an .app: so it
// unpacks to Name.app. Modes are kept, executable bits among them, and a
// symbolic link is stored as a link, as a framework's are, not followed.
func ZipDir(src, out string) error {
	parent := filepath.Dir(src)
	return writeZip(out, func(zw *zip.Writer) error {
		return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(parent, path)
			if err != nil {
				return err
			}
			return addEntry(zw, path, filepath.ToSlash(rel))
		})
	})
}

// ZipFiles writes files to out as a zip with each at its top level, under
// its own name.
func ZipFiles(out string, files ...string) error {
	return writeZip(out, func(zw *zip.Writer) error {
		for _, f := range files {
			if err := addEntry(zw, f, filepath.Base(f)); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeZip(out string, fill func(*zip.Writer) error) (err error) {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(out)
		}
	}()
	bw := bufio.NewWriter(f)
	zw := zip.NewWriter(bw)
	if err := fill(zw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return bw.Flush()
}

// zipTime is the time every entry is stamped with, so that the same
// files make the same zip.
var zipTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// addEntry adds the file at path to zw as name, keeping its Unix mode.
func addEntry(zw *zip.Writer, path, name string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	h, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}
	h.Name = name
	h.Modified = zipTime
	switch {
	case fi.IsDir():
		h.Name += "/"
		h.Method = zip.Store
		_, err := zw.CreateHeader(h)
		return err
	case fi.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		h.Method = zip.Store
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, filepath.ToSlash(target))
		return err
	case !fi.Mode().IsRegular():
		return fmt.Errorf("%s is not a regular file, a directory or a link", path)
	}
	h.Method = zip.Deflate
	w, err := zw.CreateHeader(h)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// Move moves the file src to dir under name, copying when a rename
// cannot, as across file systems, and returns the new path.
func Move(src, dir, name string) (string, error) {
	dst := filepath.Join(dir, name)
	if err := os.Rename(src, dst); err == nil {
		return dst, nil
	}
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	out, err := os.Create(dst)
	if err != nil {
		in.Close()
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		in.Close()
		os.Remove(dst)
		return "", err
	}
	if err := out.Close(); err != nil {
		in.Close()
		os.Remove(dst)
		return "", err
	}
	if err := in.Close(); err != nil {
		os.Remove(dst)
		return "", err
	}
	return dst, os.Remove(src)
}

// HumanSize writes n bytes as people read it: 812 B, 4.2 MB.
func HumanSize(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}

// Summary writes a table of the assets: name, size and the start of the
// SHA-256.
func Summary(w io.Writer, assets []Asset) {
	width := len("asset")
	for _, a := range assets {
		width = max(width, len(a.Name))
	}
	fmt.Fprintf(w, "%-*s  %9s  %s\n", width, "asset", "size", "sha256")
	for _, a := range assets {
		fmt.Fprintf(w, "%-*s  %9s  %s\n", width, a.Name, HumanSize(a.Size), a.SHA256[:min(12, len(a.SHA256))])
	}
}
