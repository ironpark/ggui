// Package runtime is the platform beneath a ggui app: the services an
// application asks of the desktop it runs on rather than of its own
// window. Today that is the native file dialog; what belongs here is
// anything whose answer comes from the operating system.
//
// Every dialog is modal and blocks until the user picks or cancels. On
// macOS the panel is a sheet attached to the app's window; on Windows it
// is a modal dialog. Both run on the window's thread, so the window stops
// repainting while one is up, which is how a modal dialog behaves on those
// platforms. Elsewhere the panel is the zenity or kdialog program, found on
// PATH; where neither is, every call returns ErrUnsupported.
//
// A dialog needs a running App: it is asked for from a handler, a shortcut
// or posted work, not before Run. A test replaces it with SetFilePicker.
package runtime

import (
	"errors"
	"sync"
)

// FileFilter is one entry of a dialog's file type menu: a name and the
// extensions it admits, without dots. A filter with no extensions admits
// every file, and a dialog with no filters shows every file.
type FileFilter struct {
	Name       string
	Extensions []string
}

// FileDialog configures a file dialog. Every field is optional.
type FileDialog struct {
	// Title is the dialog's caption, where the platform shows one.
	Title string
	// Directory is where the dialog starts; empty means wherever the
	// platform remembers.
	Directory string
	// FileName is the name SaveFile proposes.
	FileName string
	// Filters restricts what OpenFile, OpenFiles and SaveFile admit.
	Filters []FileFilter
}

// ErrCanceled is returned when the user closes a dialog without choosing.
var ErrCanceled = errors.New("runtime: canceled")

// ErrUnsupported is returned where the platform offers no dialog.
var ErrUnsupported = errors.New("runtime: not supported on this platform")

// OpenFile asks for one existing file and returns its path.
func OpenFile(d FileDialog) (string, error) { return currentPicker().OpenFile(d) }

// OpenFiles asks for one or more existing files and returns their paths.
func OpenFiles(d FileDialog) ([]string, error) { return currentPicker().OpenFiles(d) }

// PickFolder asks for one existing directory and returns its path.
func PickFolder(d FileDialog) (string, error) { return currentPicker().PickFolder(d) }

// SaveFile asks where to write a file and returns the path chosen. The
// platform asks before handing back a path that already exists.
func SaveFile(d FileDialog) (string, error) { return currentPicker().SaveFile(d) }

// FilePicker is what the file dialog functions call. The default is the
// platform's dialog; SetFilePicker replaces it.
type FilePicker interface {
	OpenFile(d FileDialog) (string, error)
	OpenFiles(d FileDialog) ([]string, error)
	PickFolder(d FileDialog) (string, error)
	SaveFile(d FileDialog) (string, error)
}

var (
	pickerMu sync.Mutex
	picker   FilePicker = nativePicker{}
)

// SetFilePicker replaces the dialogs the package opens, for tests and for
// a host that draws its own. A nil p restores the platform's.
func SetFilePicker(p FilePicker) {
	pickerMu.Lock()
	defer pickerMu.Unlock()
	if p == nil {
		p = nativePicker{}
	}
	picker = p
}

func currentPicker() FilePicker {
	pickerMu.Lock()
	defer pickerMu.Unlock()
	return picker
}

// StubFilePicker is a FilePicker that answers every dialog with fixed
// paths, for tests. OpenFile, PickFolder and SaveFile return the first of
// Paths; OpenFiles returns them all. With no Paths every call is a cancel,
// and Err, when set, is returned instead of anything.
type StubFilePicker struct {
	Paths []string
	Err   error
	// Asked records every dialog opened, in order.
	Asked []FileDialog
}

func (s *StubFilePicker) one(d FileDialog) (string, error) {
	s.Asked = append(s.Asked, d)
	if s.Err != nil {
		return "", s.Err
	}
	if len(s.Paths) == 0 {
		return "", ErrCanceled
	}
	return s.Paths[0], nil
}

// OpenFile implements FilePicker.
func (s *StubFilePicker) OpenFile(d FileDialog) (string, error) { return s.one(d) }

// OpenFiles implements FilePicker.
func (s *StubFilePicker) OpenFiles(d FileDialog) ([]string, error) {
	s.Asked = append(s.Asked, d)
	if s.Err != nil {
		return nil, s.Err
	}
	if len(s.Paths) == 0 {
		return nil, ErrCanceled
	}
	return append([]string(nil), s.Paths...), nil
}

// PickFolder implements FilePicker.
func (s *StubFilePicker) PickFolder(d FileDialog) (string, error) { return s.one(d) }

// SaveFile implements FilePicker.
func (s *StubFilePicker) SaveFile(d FileDialog) (string, error) { return s.one(d) }

// dialogKind is which dialog the platform half shows.
type dialogKind uint8

const (
	kindOpen dialogKind = iota
	kindOpenMultiple
	kindFolder
	kindSave
)

// nativePicker is the platform's dialog. Each platform file supplies
// showDialog, which returns the chosen paths or ErrCanceled.
type nativePicker struct{}

func (nativePicker) OpenFile(d FileDialog) (string, error) { return first(showDialog(kindOpen, d)) }

func (nativePicker) OpenFiles(d FileDialog) ([]string, error) {
	return all(showDialog(kindOpenMultiple, d))
}

func (nativePicker) PickFolder(d FileDialog) (string, error) { return first(showDialog(kindFolder, d)) }

func (nativePicker) SaveFile(d FileDialog) (string, error) { return first(showDialog(kindSave, d)) }

// all is what a platform half's answer means to a caller: a nil error with
// nothing chosen is a cancel, which is what a dialog that never ran
// amounts to.
func all(paths []string, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, ErrCanceled
	}
	return paths, nil
}

func first(paths []string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", ErrCanceled
	}
	return paths[0], nil
}
