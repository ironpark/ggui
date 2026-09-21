package ggui

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Files dropped onto the window from the desktop. The platform reports a
// drop once, at the cursor's position, with a file system holding what was
// dropped; ggui routes it like a pointer event, to the topmost region under
// the cursor that accepts drops, and otherwise to the handlers registered
// with Host.OnDrop.
//
// There is no drag-over feedback: the platform layer ggui renders with
// reports the drop itself and nothing before it, so a drop zone cannot
// highlight while a file hovers over it.

// DroppedFile is one file or directory that was dropped onto the window.
type DroppedFile struct {
	// Name is the file's base name, as it appears in the file system the
	// platform handed over.
	Name string
	// Path is the absolute path in the real file system, or empty where the
	// platform provides none, which is the case in a browser.
	Path string
	// Dir reports whether a directory was dropped rather than a file.
	Dir bool

	fsys fs.FS
}

// Open reads the dropped file, wherever the platform keeps it. It is the
// one way to read a drop that has no Path.
func (f DroppedFile) Open() (fs.File, error) {
	if f.fsys == nil {
		return nil, &fs.PathError{Op: "open", Path: f.Name, Err: fs.ErrNotExist}
	}
	return f.fsys.Open(f.Name)
}

// DropEvent is a drop of files onto the window.
type DropEvent struct {
	Pos   Point // in window coordinates
	Files []DroppedFile
}

// Paths returns the real file system path of every dropped file that has
// one, in the order they were dropped.
func (ev DropEvent) Paths() []string {
	var paths []string
	for _, f := range ev.Files {
		if f.Path != "" {
			paths = append(paths, f.Path)
		}
	}
	return paths
}

// DropHandler is a PointerHandler that also accepts dropped files. A drop
// goes to the topmost region under the cursor whose handler implements it
// and returns true; a region that returns false lets the drop fall through
// to what is beneath, and then to the host's handlers.
type DropHandler interface {
	HandleDrop(DropEvent) bool
}

// absPather is what a dropped entry implements where the platform knows
// the file's real path; Ebitengine names it AbsPather.
type absPather interface {
	AbsPath() string
}

// droppedFiles reads the root of the file system a drop handed over into
// the files a DropEvent carries.
func droppedFiles(fsys fs.FS) []DroppedFile {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	files := make([]DroppedFile, 0, len(entries))
	for _, e := range entries {
		f := DroppedFile{Name: e.Name(), Dir: e.IsDir(), fsys: fsys}
		if p, ok := e.(absPather); ok {
			f.Path = p.AbsPath()
		}
		files = append(files, f)
	}
	return files
}

// droppedPaths makes the files a drop of real paths would carry, for a
// Probe. Each file reads from its own directory, so the paths need not
// share one.
func droppedPaths(paths []string) []DroppedFile {
	files := make([]DroppedFile, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		info, err := os.Stat(abs)
		files = append(files, DroppedFile{
			Name: filepath.Base(abs),
			Path: abs,
			Dir:  err == nil && info.IsDir(),
			fsys: os.DirFS(filepath.Dir(abs)),
		})
	}
	return files
}

// dispatchDrop routes a frame's drop: to the topmost accepting region under
// the cursor, else to every handler the host registered.
func (in *inputState) dispatchDrop(f frameInput) {
	ev := DropEvent{Pos: f.pos, Files: f.drop}
	taken := in.topmost(func(r *hitRegion) bool {
		h, ok := r.pointer.(DropHandler)
		return ok && r.rect.Contains(ev.Pos) && h.HandleDrop(ev)
	})
	if taken != nil {
		return
	}
	for _, fn := range in.drops {
		fn(ev)
	}
}
