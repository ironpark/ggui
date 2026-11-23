package ggui

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/ironpark/ggui/gem"
)

// The drag session: what input does while something is dragged. A drag
// inside the app starts on a region registered with HitDrag and ends on
// a region whose pointer handler is a DropTarget; package widget's
// DragSource, DropZone and Reorderable are built on it. Files dragged in from
// the desktop are routed further down, to a DropHandler and a DragHandler:
// a drag inside the app carries a value of any type from a Draggable to a
// DropTarget, one from the desktop carries files, and neither reaches the
// other's handlers.
//
// A source does not take the press. The widget it wraps keeps its clicks,
// and only when the pointer moves a few pixels with the button down does the
// press become a drag: the widget gets a PointerUp without a tap, and from
// then on the drag follows the pointer until the button comes up over a
// zone that accepts it, anywhere else, or Escape cancels it.

// dragThreshold is how far, in logical pixels, a press moves before it
// becomes a drag.
const dragThreshold = 4

// Draggable is what a drag inside the app is picked up from, registered
// with Canvas.HitDrag. Input asks it for the value the drag carries as the
// drag starts, tells it where the pointer is every frame the drag moves,
// and tells it once when the drag ends.
type Draggable interface {
	// DragValue is the value the drag carries, asked for once as it
	// starts. A DropTarget sees it as it is.
	DragValue() any
	// DragMoved is called every frame the drag follows the pointer, the
	// first of them as it starts: at is the pointer, and grab where on the
	// region it took hold, both in the coordinates the region was painted
	// in. A source draws what follows the pointer from them, as a faded
	// copy at at less grab.
	DragMoved(at, grab Point)
	// DragEnded is called once when the drag ends: dropped on a target
	// that took it, or released elsewhere, or cancelled with Escape.
	DragEnded(dropped bool)
}

// DropTarget is a PointerHandler that a drag inside the app can be put
// down on: the topmost region under the pointer whose handler implements
// it and accepts the drag's value is where it would land. It is the in-app
// counterpart of DropHandler, which takes files from the desktop instead.
// A region that registers only to be found under a drag returns false
// from HandlePointer, as DropZone does.
type DropTarget interface {
	// AcceptsDrop reports whether the target takes v. It is asked every
	// frame the drag is over the region; one that refuses is passed over
	// for a target beneath.
	AcceptsDrop(v any) bool
	// DropHover is called with true when the drag comes over the target
	// and with false when it leaves, is dropped or is cancelled.
	DropHover(over bool)
	// DropOver is called every frame the drag moves over the target, with
	// where it would land in the target's own coordinates.
	DropOver(v any, at Point)
	// Drop puts v down on the target at a point of its own.
	Drop(v any, at Point)
}

// appDrag is the drag in progress, or about to be: from the press until it
// moves far enough, only candidate and from are set.
type appDrag struct {
	candidate *hitRegion // the source under the press
	from      Point      // where the press was
	source    Draggable  // set once it is a drag
	value     any
	target    *hitRegion // the zone it is over, if one accepts it
}

// HitDrag registers r as a region a drag inside the app can start from,
// carrying what d gives: a press on r that moves a few pixels with the
// button down becomes a drag, which d hears of as it moves and when it
// ends. The region takes no press itself, so a source registers before
// what it wraps, whose regions stay on top and keep their clicks.
func (c *Canvas) HitDrag(r Rect, d Draggable) {
	c.add(c.region(r, nil, hitRegion{drag: d}))
}

// pressDrag notes the drag source a press may pick up: the topmost drag
// region under at that pressed, the region that took the press, is inside
// of, as a button in a draggable tab is, or that is inside pressed, as a
// source under a wrapper that takes taps is; or any when nothing took the
// press. A press taken by a region painted over a source by a widget that
// is not part of it, a popup or a layer above, drags nothing.
func (in *inputState) pressDrag(at Point, pressed *hitRegion) {
	p := int32(-1)
	if pressed != nil {
		p = in.indexOf(pressed)
	}
	r := in.topmost(func(r *hitRegion) bool {
		if r.drag == nil || !r.contains(at) {
			return false
		}
		if p < 0 {
			return true
		}
		s := in.indexOf(r)
		return in.inside(p, s) || in.inside(s, p)
	})
	in.drag = nil
	if r != nil {
		in.drag = &appDrag{candidate: keep(r), from: at}
	}
}

// inside reports whether the region at i is the one at outer or is inside
// it.
func (in *inputState) inside(i, outer int32) bool {
	// A region's parent is always before it, so the walk can stop there.
	for ; i >= outer && i >= 0; i = in.regions[i].parent - 1 {
		if i == outer {
			return true
		}
	}
	return false
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
		if in.pressCapturesDrag() {
			// The control under the press drags by itself: a slider's knob,
			// a selection. The source never starts.
			in.drag = nil
			return
		}
		src := d.candidate.drag
		d.source, d.value = src, src.DragValue()
		in.releasePress(f.pos, f.mods)
	}
	d.source.DragMoved(d.candidate.at(f.pos), d.candidate.offset(d.from))
	now := in.hoverDropZone(d, f.pos)
	if now != nil && !released {
		now.pointer.(DropTarget).DropOver(d.value, now.offset(f.pos))
	}
	if !released {
		return
	}
	dropped := false
	if now != nil {
		t := now.pointer.(DropTarget)
		t.DropHover(false)
		t.Drop(d.value, now.offset(f.pos))
		dropped = true
	}
	in.drag = nil
	d.source.DragEnded(dropped)
}

// hoverDropZone moves d over the topmost zone under pos that accepts its
// value, telling the zone it leaves and the one it comes over, and returns
// that zone, or nil.
func (in *inputState) hoverDropZone(d *appDrag, pos Point) *hitRegion {
	now := in.topmost(func(r *hitRegion) bool {
		t, ok := r.pointer.(DropTarget)
		return ok && r.contains(pos) && t.AcceptsDrop(d.value)
	})
	if !in.continues(now, d.target) {
		in.unhoverDrop(d.target)
		if now != nil {
			now.pointer.(DropTarget).DropHover(true)
		}
	} else if now != nil && (d.target == nil || !gem.Same(now.pointer, d.target.pointer)) {
		// The drop target was rebuilt in place under the drag: tell the new
		// instance it is hovered.
		now.pointer.(DropTarget).DropHover(true)
	}
	d.target = keep(now)
	return now
}

// pressCapturesDrag reports whether the handler holding the press keeps
// its drags as a DragCapturer.
func (in *inputState) pressCapturesDrag() bool {
	if in.pressed == nil {
		return false
	}
	if _, ok := in.pressed.pointer.(DragCapturer); !ok {
		return false
	}
	cur := in.findPointer(in.pressed)
	if cur == nil {
		return false
	}
	c, ok := cur.pointer.(DragCapturer)
	return ok && c.CaptureDrag()
}

// cancelAppDrag ends a drag in progress without a drop, as Escape does, and
// reports whether there was one.
func (in *inputState) cancelAppDrag() bool {
	d := in.drag
	in.drag = nil
	if d == nil || d.source == nil {
		return false
	}
	in.unhoverDrop(d.target)
	d.source.DragEnded(false)
	return true
}

// unhoverDrop tells the drop target for target that the drag left it,
// resolving target to the current frame's handler when the widget was rebuilt.
func (in *inputState) unhoverDrop(target *hitRegion) {
	if target == nil {
		return
	}
	if cur := in.findPointer(target); cur != nil {
		if t, ok := cur.pointer.(DropTarget); ok {
			t.DropHover(false)
			return
		}
	}
	if t, ok := target.pointer.(DropTarget); ok {
		t.DropHover(false)
	}
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
