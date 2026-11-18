package linux

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// entry is a file or directory of the tree.
type entry struct {
	rel  string // slash-separated, relative to the tree: usr/bin/hello
	src  string // on disk
	dir  bool
	mode fs.FileMode
}

// modeFor is the mode a file of the tree is installed with: the program
// 0755, everything else 0644, whatever the file system the tree was made
// on records, as Windows records none.
func modeFor(rel string) fs.FileMode {
	if path.Dir(rel) == "usr/bin" {
		return 0o755
	}
	return 0o644
}

// walk lists the tree under dir, parents before children, in lexical
// order.
func walk(dir string) ([]entry, error) {
	var out []entry
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			out = append(out, entry{rel: rel, src: p, dir: true, mode: 0o755})
			return nil
		}
		out = append(out, entry{rel: rel, src: p, mode: modeFor(rel)})
		return nil
	})
	return out, err
}

// TarName is the tarball's file name: Package-Version-linux-Arch.tar.gz.
func TarName(a App) string {
	return a.Package + "-" + PackageVersion(a.Version) + "-linux-" + a.Arch + ".tar.gz"
}

// Tar writes the tree under dir to out as a gzipped tarball whose paths
// start at usr/, owned by root, so that it unpacks onto / as the
// packages install, or with --strip-components=1 onto /usr/local or
// ~/.local.
func Tar(dir, out string) (err error) {
	entries, err := walk(dir)
	if err != nil {
		return err
	}
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
	gz, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		fi, err := os.Stat(e.src)
		if err != nil {
			return err
		}
		h := &tar.Header{
			Name:    e.rel,
			Mode:    int64(e.mode),
			ModTime: fi.ModTime().UTC().Truncate(1e9),
			Uname:   "root",
			Gname:   "root",
		}
		if e.dir {
			h.Typeflag = tar.TypeDir
			h.Name += "/"
		} else {
			h.Typeflag = tar.TypeReg
			h.Size = fi.Size()
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if !e.dir {
			if err := copyFile(tw, e.src); err != nil {
				return err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func copyFile(w io.Writer, src string) (err error) {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(w, f)
	return err
}
