package windows

import (
	"debug/pe"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SysoName is the file the resources are written to in the main package
// for a build for arch. The go command links a package's .syso files into
// the program, and the name's _windows_arch suffix is a build constraint,
// so the file is linked into that build only: a build for another
// platform that runs meanwhile, as ggui dev's may, does not see it.
//
// The go command takes a .syso file only from the package's directory on
// disk: an overlay (go build -overlay) adds one to the package, as go list
// reports, but the go command then packs and checks the file by its path
// on disk, which an overlay does not replace, and the build fails. So the
// file is written beside the sources for the build and removed after it.
func SysoName(arch string) string { return "ggui_windows_" + arch + ".syso" }

// HasResources reports whether the object file at path has a resource
// section. The linker takes one per program and fails on a second, so a
// package that links resources of its own, as from go-winres or rsrc,
// keeps them, and ggui adds none. A file debug/pe cannot read is taken to
// have them: the linker would fail on it either way, and saying so is
// better than adding a second section to fail on.
func HasResources(path string) bool {
	f, err := pe.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	for _, s := range f.Sections {
		if s.Name == ".rsrc" || strings.HasPrefix(s.Name, ".rsrc$") {
			return true
		}
	}
	return false
}

// Place writes obj into dir as SysoName(arch), and returns its path and a
// function that removes it. A file of that name is one a build that was
// killed left behind, which leftover reports, and is replaced.
func Place(dir, arch string, obj []byte) (path string, leftover bool, remove func() error, err error) {
	path = filepath.Join(dir, SysoName(arch))
	if _, err := os.Lstat(path); err == nil {
		leftover = true
	}
	remove = func() error {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.WriteFile(path, obj, 0o644); err != nil {
		remove()
		return "", leftover, nil, err
	}
	return path, leftover, remove, nil
}
