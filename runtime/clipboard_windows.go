//go:build windows

package runtime

import (
	goruntime "runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ironpark/ggui/internal/platform/win32"
	"golang.org/x/sys/windows"
)

// The Windows clipboard is user32's. It is open to one caller at a time
// across the whole desktop, so opening it is retried briefly while another
// program holds it.

var (
	procOpenClipboard    = win32.User32.NewProc("OpenClipboard")
	procCloseClipboard   = win32.User32.NewProc("CloseClipboard")
	procEmptyClipboard   = win32.User32.NewProc("EmptyClipboard")
	procGetClipboardData = win32.User32.NewProc("GetClipboardData")
	procSetClipboardData = win32.User32.NewProc("SetClipboardData")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

func nativeClipboard() Clipboard { return winClipboard{} }

var clipboardMu sync.Mutex

// winClipboard is the system clipboard through user32.
type winClipboard struct{}

// open opens the clipboard, waiting up to about 100ms for another program
// to let go of it.
func open() bool {
	for range 20 {
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func (winClipboard) Read() string {
	clipboardMu.Lock()
	defer clipboardMu.Unlock()
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()
	if !open() {
		return ""
	}
	defer procCloseClipboard.Call()
	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	p, _, _ := win32.GlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer win32.GlobalUnlock.Call(h)
	s := windows.UTF16PtrToString(win32.Deref[uint16](p))
	// Text on the Windows clipboard ends its lines with CRLF; text fields
	// work in LF.
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func (winClipboard) Write(s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	u, err := windows.UTF16FromString(s)
	if err != nil {
		// s holds a NUL, which the clipboard's text format cannot.
		u, _ = windows.UTF16FromString(strings.ReplaceAll(s, "\x00", ""))
	}
	clipboardMu.Lock()
	defer clipboardMu.Unlock()
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()
	if !open() {
		return
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	size := uintptr(len(u)) * 2
	h, _, _ := win32.GlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return
	}
	p, _, _ := win32.GlobalLock.Call(h)
	if p == 0 {
		win32.GlobalFree.Call(h)
		return
	}
	copy(unsafe.Slice(win32.Deref[uint16](p), len(u)), u)
	win32.GlobalUnlock.Call(h)
	// On success the clipboard owns the memory; otherwise it is still ours.
	if r, _, _ := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		win32.GlobalFree.Call(h)
	}
}
