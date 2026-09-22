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
// A dialog is reached through the Host: App.Dialogs is the platform's
// picker, and Probe.Dialogs a StubFilePicker, so code written against Host
// runs under a window and under a test alike. A dialog needs a running
// App: it is asked for from a handler, a shortcut or posted work, not
// before Run.
package runtime

import "errors"

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

// FilePicker opens the file dialogs. NativeFilePicker is the platform's;
// StubFilePicker answers from fixed paths, for tests.
type FilePicker interface {
	// OpenFile asks for one existing file and returns its path.
	OpenFile(d FileDialog) (string, error)
	// OpenFiles asks for one or more existing files and returns their paths.
	OpenFiles(d FileDialog) ([]string, error)
	// PickFolder asks for one existing directory and returns its path.
	PickFolder(d FileDialog) (string, error)
	// SaveFile asks where to write a file and returns the path chosen. The
	// platform asks before handing back a path that already exists.
	SaveFile(d FileDialog) (string, error)
}

// NativeFilePicker returns the platform's file dialogs, attached to no
// window; NativeFilePickerForWindow attaches them to one.
func NativeFilePicker() FilePicker { return NativeFilePickerForWindow(nil) }

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

// answer records the dialog and returns a copy of Paths, or Err.
func (s *StubFilePicker) answer(d FileDialog) ([]string, error) {
	s.Asked = append(s.Asked, d)
	if s.Err != nil {
		return nil, s.Err
	}
	return append([]string(nil), s.Paths...), nil
}

// OpenFile implements FilePicker.
func (s *StubFilePicker) OpenFile(d FileDialog) (string, error) { return first(s.answer(d)) }

// OpenFiles implements FilePicker.
func (s *StubFilePicker) OpenFiles(d FileDialog) ([]string, error) { return all(s.answer(d)) }

// PickFolder implements FilePicker.
func (s *StubFilePicker) PickFolder(d FileDialog) (string, error) { return first(s.answer(d)) }

// SaveFile implements FilePicker.
func (s *StubFilePicker) SaveFile(d FileDialog) (string, error) { return first(s.answer(d)) }

// dialogKind is which dialog the platform half shows.
type dialogKind uint8

const (
	kindOpen dialogKind = iota
	kindOpenMultiple
	kindFolder
	kindSave
)

// NativeFilePickerForWindow binds dialogs to the caller's native window. The
// getter runs before entering the native main-thread callback.
func NativeFilePickerForWindow(window func() uintptr) FilePicker { return nativePicker{window: window} }

// nativePicker is the platform's dialog. Each platform file supplies
// showDialog, which returns the chosen paths or ErrCanceled.
type nativePicker struct{ window func() uintptr }

func (p nativePicker) show(k dialogKind, d FileDialog) ([]string, error) {
	var owner uintptr
	if p.window != nil {
		owner = p.window()
	}
	return showDialog(k, d, owner)
}

func (p nativePicker) OpenFile(d FileDialog) (string, error) { return first(p.show(kindOpen, d)) }

func (p nativePicker) OpenFiles(d FileDialog) ([]string, error) {
	return all(p.show(kindOpenMultiple, d))
}

func (p nativePicker) PickFolder(d FileDialog) (string, error) { return first(p.show(kindFolder, d)) }

func (p nativePicker) SaveFile(d FileDialog) (string, error) { return first(p.show(kindSave, d)) }

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
	if paths, err = all(paths, err); err != nil {
		return "", err
	}
	return paths[0], nil
}
