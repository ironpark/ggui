//go:build windows

package runtime

import (
	goruntime "runtime"
	"strings"
	"unsafe"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/sys/windows"

	"github.com/ironpark/ggui/internal/platform/win32"
)

// The Windows dialogs are the common dialogs of comdlg32 and the folder
// browser of shell32: plain functions rather than COM objects, which keeps
// this to a struct and a few calls. They run on the main thread, which owns
// the window and its message loop; the frame goroutine waits, so the window
// stops repainting until the dialog closes, as it would under any modal
// dialog.

var (
	dllComdlg32 = windows.NewLazySystemDLL("comdlg32.dll")
	dllShell32  = windows.NewLazySystemDLL("shell32.dll")
	dllOle32    = windows.NewLazySystemDLL("ole32.dll")

	procGetOpenFileNameW     = dllComdlg32.NewProc("GetOpenFileNameW")
	procGetSaveFileNameW     = dllComdlg32.NewProc("GetSaveFileNameW")
	procSHBrowseForFolderW   = dllShell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = dllShell32.NewProc("SHGetPathFromIDListW")
	procCoInitializeEx       = dllOle32.NewProc("CoInitializeEx")
	procCoTaskMemFree        = dllOle32.NewProc("CoTaskMemFree")
)

// OPENFILENAMEW flags, from commdlg.h.
const (
	ofnOverwritePrompt  = 0x00000002
	ofnNoChangeDir      = 0x00000008
	ofnAllowMultiSelect = 0x00000200
	ofnPathMustExist    = 0x00000800
	ofnFileMustExist    = 0x00001000
	ofnExplorer         = 0x00080000
)

// BROWSEINFOW flags, from shlobj_core.h.
const (
	bifReturnOnlyFSDirs = 0x00000001
	bifNewDialogStyle   = 0x00000040
)

// coinitApartmentThreaded is the threading model the new-style folder
// browser needs on its thread.
const coinitApartmentThreaded = 0x2

// maxPath is the longest path the folder browser hands back.
const maxPath = 260

// fileBufferLen is how many UTF-16 units the file dialog may fill: enough
// for a large multiple selection.
const fileBufferLen = 64 * 1024

// openFileName is OPENFILENAMEW, laid out as the operating system lays it
// out; the size field is what tells the dialog which version it was given.
type openFileName struct {
	structSize    uint32
	owner         uintptr
	instance      uintptr
	filter        *uint16
	customFilter  *uint16
	maxCustFilter uint32
	filterIndex   uint32
	file          *uint16
	maxFile       uint32
	fileTitle     *uint16
	maxFileTitle  uint32
	initialDir    *uint16
	title         *uint16
	flags         uint32
	fileOffset    uint16
	fileExtension uint16
	defExt        *uint16
	custData      uintptr
	hook          uintptr
	templateName  *uint16
	pvReserved    uintptr
	dwReserved    uint32
	flagsEx       uint32
}

// browseInfo is BROWSEINFOW.
type browseInfo struct {
	owner       uintptr
	root        uintptr
	displayName *uint16
	title       *uint16
	flags       uint32
	callback    uintptr
	param       uintptr
	image       int32
}

// showDialog runs the dialog on the main thread and waits for it.
func showDialog(k dialogKind, d FileDialog) (paths []string, err error) {
	ebiten.RunOnMainThread(func() {
		// Owned by the app's window, the dialog is modal to it and centred
		// on it; without one it stands on its own.
		owner := win32.AppWindow()
		if k == kindFolder {
			paths, err = browseFolder(d, owner)
		} else {
			paths, err = fileDialog(k, d, owner)
		}
	})
	return paths, err
}

// fileDialog runs GetOpenFileNameW or GetSaveFileNameW. The buffer the
// dialog fills is a NUL-terminated string, or for a multiple selection the
// directory followed by each name, each NUL-terminated, then an empty one.
func fileDialog(k dialogKind, d FileDialog, owner uintptr) ([]string, error) {
	buf := make([]uint16, fileBufferLen)
	if d.FileName != "" {
		copy(buf, utf16z(d.FileName))
	}
	ofn := openFileName{
		owner:      owner,
		file:       &buf[0],
		maxFile:    uint32(len(buf)),
		flags:      ofnExplorer | ofnNoChangeDir | ofnPathMustExist,
		title:      utf16ptr(d.Title),
		initialDir: utf16ptr(d.Directory),
	}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))
	filter := filterSpec(d.Filters)
	if filter != nil {
		ofn.filter = &filter[0]
		ofn.filterIndex = 1
	}
	proc := procGetOpenFileNameW
	switch k {
	case kindOpen:
		ofn.flags |= ofnFileMustExist
	case kindOpenMultiple:
		ofn.flags |= ofnFileMustExist | ofnAllowMultiSelect
	case kindSave:
		proc = procGetSaveFileNameW
		ofn.flags |= ofnOverwritePrompt
		if len(d.Filters) > 0 && len(d.Filters[0].Extensions) > 0 {
			ofn.defExt = utf16ptr(d.Filters[0].Extensions[0])
		}
	}
	ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&ofn)))
	goruntime.KeepAlive(&ofn)
	goruntime.KeepAlive(filter)
	if ok == 0 {
		// Zero is both a cancel and a failure; CommDlgExtendedError would
		// tell them apart, and a failure of a dialog that was asked for
		// correctly is not something a caller can act on.
		return nil, ErrCanceled
	}
	parts := splitz(buf)
	if len(parts) == 0 {
		return nil, ErrCanceled
	}
	if k != kindOpenMultiple || len(parts) == 1 {
		return parts[:1], nil
	}
	dir := strings.TrimRight(parts[0], `\`)
	paths := make([]string, 0, len(parts)-1)
	for _, name := range parts[1:] {
		paths = append(paths, dir+`\`+name)
	}
	return paths, nil
}

// browseFolder runs SHBrowseForFolderW, which hands back an item list that
// SHGetPathFromIDListW turns into a path and CoTaskMemFree releases.
func browseFolder(d FileDialog, owner uintptr) ([]string, error) {
	// The new-style browser, the one with a tree that can be typed into,
	// wants COM initialised on its thread. A second initialisation is a
	// harmless S_FALSE, and a mismatch with an earlier one is also harmless
	// here: the browser falls back to the old style.
	procCoInitializeEx.Call(0, coinitApartmentThreaded)
	display := make([]uint16, maxPath)
	bi := browseInfo{
		owner:       owner,
		displayName: &display[0],
		title:       utf16ptr(d.Title),
		flags:       bifReturnOnlyFSDirs | bifNewDialogStyle,
	}
	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	goruntime.KeepAlive(&bi)
	goruntime.KeepAlive(display)
	if pidl == 0 {
		return nil, ErrCanceled
	}
	defer procCoTaskMemFree.Call(pidl)
	path := make([]uint16, maxPath)
	ok, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
	if ok == 0 {
		return nil, ErrCanceled
	}
	return []string{windows.UTF16ToString(path)}, nil
}

// filterSpec builds the dialog's type menu: pairs of display name and
// pattern list, each NUL-terminated, with an empty string at the end.
func filterSpec(filters []FileFilter) []uint16 {
	if len(filters) == 0 {
		return nil
	}
	var spec []uint16
	for _, f := range filters {
		pattern := globs(f, ";")
		if pattern == "*" {
			pattern = "*.*"
		}
		spec = append(spec, utf16z(f.Name+" ("+pattern+")")...)
		spec = append(spec, utf16z(pattern)...)
	}
	return append(spec, 0)
}

// utf16z is s as UTF-16 with a terminator.
func utf16z(s string) []uint16 {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return []uint16{0}
	}
	return u
}

// utf16ptr is s as a terminated wide string, or nil for an empty s, which
// is how the dialog structures say "none".
func utf16ptr(s string) *uint16 {
	if s == "" {
		return nil
	}
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

// splitz splits a buffer of NUL-terminated wide strings ending in an empty
// one, and stops at the first empty string.
func splitz(buf []uint16) []string {
	var out []string
	for len(buf) > 0 {
		end := 0
		for end < len(buf) && buf[end] != 0 {
			end++
		}
		if end == 0 {
			break
		}
		out = append(out, windows.UTF16ToString(buf[:end]))
		if end == len(buf) {
			break
		}
		buf = buf[end+1:]
	}
	return out
}
