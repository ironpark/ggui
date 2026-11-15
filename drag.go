package ggui

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// The drag session: what input does while something is dragged. A drag
// inside the app starts on a region registered with hitDrag and ends on
// a region whose pointer handler is a dropTarget; DragSource, DropZone
// and Reorderable, in dragdrop.go, are built on it. Files dragged in from
// the desktop are routed further down.
//
// A source does not take the press. The widget it wraps keeps its clicks,
// and only when the pointer moves a few pixels with the button down does the
// press become a drag: the widget gets a PointerUp without a tap, and from
// then on the drag follows the pointer until the button comes up over a
// zone that accepts it, anywhere else, or Escape cancels it.

// dragThreshold is how far, in logical pixels, a press moves before it
// becomes a drag.
const dragThreshold = 4

// dragSource is the side of a DragSourceWidget input talks to.
type dragSource interface {
	dragValue() any
	dragMoved(at, grab Point)
	dragEnded(dropped bool)
}

// dropTarget is the side of a DropZoneWidget input talks to.
type dropTarget interface {
	acceptsDrag(v any) bool
	dragOver(over bool)
	dragMoved(v any, at Point) // the drag is over the target, at a point of its own
	dropped(v any, at Point)
}

// appDrag is the drag in progress, or about to be: from the press until it
// moves far enough, only candidate and from are set.
type appDrag struct {
	candidate *hitRegion // the source under the press
	from      Point      // where the press was
	source    dragSource // set once it is a drag
	value     any
	target    *hitRegion // the zone it is over, if one accepts it
}

// hitDrag registers r as a region a drag can start from, carrying what s
// gives: a press on r that moves dragThreshold with the button down
// becomes a drag, which s hears of as it moves and when it ends. The
// region takes no press itself, so a source registers before what it
// wraps, whose regions stay on top and keep their clicks.
func (c *Canvas) hitDrag(r Rect, s dragSource) {
	c.add(c.region(r, nil, hitRegion{drag: s}))
}

// pressDrag notes the drag source under a press, if there is one.
func (in *inputState) pressDrag(at Point) {
	r := in.topmost(func(r *hitRegion) bool { return r.drag != nil && r.contains(at) })
	in.drag = nil
	if r != nil {
		in.drag = &appDrag{candidate: keep(r), from: at}
	}
}

// updateAppDrag moves the drag in progress: it starts one when a press on a
// source has moved far enough, tracks the zone under the pointer, and on
// release drops onto it. It runs after the pointer buttons were dispatched,
// so a release ends the drag in the frame it happens.
func (in *inputState) updateAppDrag(f frameInput) {
	d := in.drag
	if d == nil {
		return
	}
	released := slices.Contains(f.up, MouseButtonLeft)
	if d.source == nil {
		dx := f.pos.Sub(d.from)
		if released || dx.X*dx.X+dx.Y*dx.Y < dragThreshold*dragThreshold {
			if released {
				in.drag = nil
			}
			return
		}
		src := d.candidate.drag
		d.source, d.value = src, src.dragValue()
		in.releasePress(f.pos, f.mods)
	}
	d.source.dragMoved(d.candidate.at(f.pos), d.candidate.offset(d.from))
	now := in.topmost(func(r *hitRegion) bool {
		t, ok := r.pointer.(dropTarget)
		return ok && r.contains(f.pos) && t.acceptsDrag(d.value)
	})
	if !in.continues(now, d.target) {
		if d.target != nil {
			d.target.pointer.(dropTarget).dragOver(false)
		}
		if now != nil {
			now.pointer.(dropTarget).dragOver(true)
		}
	}
	d.target = keep(now)
	if now != nil && !released {
		now.pointer.(dropTarget).dragMoved(d.value, now.offset(f.pos))
	}
	if !released {
		return
	}
	dropped := false
	if now != nil {
		t := now.pointer.(dropTarget)
		t.dragOver(false)
		t.dropped(d.value, now.offset(f.pos))
		dropped = true
	}
	in.drag = nil
	d.source.dragEnded(dropped)
}

// cancelAppDrag ends a drag in progress without a drop, as Escape does, and
// reports whether there was one.
func (in *inputState) cancelAppDrag() bool {
	d := in.drag
	in.drag = nil
	if d == nil || d.source == nil {
		return false
	}
	if d.target != nil {
		d.target.pointer.(dropTarget).dragOver(false)
	}
	d.source.dragEnded(false)
	return true
}

// Files dropped onto the window from the desktop. The platform reports a
// drop once, at the cursor's position, with a file system holding what was
// dropped; ggui routes it like a pointer event, to the topmost region under
// the cursor that accepts drops, and otherwise to the handlers registered
// with Host.OnDrop.
//
// While files are still being dragged, the zone under the cursor hears of
// it through DragHandler, so that it can highlight. ggfx reports only
// the drop, so ggui follows the drag on the platform's own drag session
// where it can: macOS today. Elsewhere a zone learns of a drag when it
// lands, and its drop handler still runs.

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
	Pos   Point // where the handler painted: window coordinates, unless under a Transform
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

// DragKind is what a DragEvent reports: files over the zone, or gone.
type DragKind uint8

const (
	// DragOver is sent every frame files are dragged over the zone.
	DragOver DragKind = iota
	// DragExit is sent once when they leave it, or are dropped or let go.
	DragExit
)

// DragEvent is files being dragged over the window, before any drop.
type DragEvent struct {
	Kind DragKind
	Pos  Point // where the handler painted: window coordinates, unless under a Transform
}

// DragHandler is a DropHandler that also follows the drag before the drop.
// Each frame files are over the window, the topmost region under the cursor
// whose handler returns true from HandleDrag for a DragOver is the drag's
// target; it receives a DragExit when the cursor moves off it or the drag
// ends. A handler that returns false for DragOver is passed over. A drop
// zone should claim the drag whether or not it shows it, as PointerWidget
// does, so that the zone which highlights is the one the drop reaches.
type DragHandler interface {
	HandleDrag(DragEvent) bool
}

// absPather is what a dropped entry implements where the platform knows
// the file's real path; ggfx names it AbsPather.
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
		return ok && r.contains(ev.Pos) && h.HandleDrop(DropEvent{Pos: r.at(ev.Pos), Files: ev.Files})
	})
	if taken != nil {
		return
	}
	for _, fn := range in.drops {
		fn(ev)
	}
}
