//go:build windows

package runtime

import (
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Windows clipboard is user32's. It is open to one caller at a time
// across the whole desktop, so opening it is retried briefly while another
// program holds it.

var (
	dllUser32   = windows.NewLazySystemDLL("user32.dll")
	dllKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procOpenClipboard    = dllUser32.NewProc("OpenClipboard")
	procCloseClipboard   = dllUser32.NewProc("CloseClipboard")
	procEmptyClipboard   = dllUser32.NewProc("EmptyClipboard")
	procGetClipboardData = dllUser32.NewProc("GetClipboardData")
	procSetClipboardData = dllUser32.NewProc("SetClipboardData")
	procGlobalAlloc      = dllKernel32.NewProc("GlobalAlloc")
	procGlobalFree       = dllKernel32.NewProc("GlobalFree")
	procGlobalLock       = dllKernel32.NewProc("GlobalLock")
	procGlobalUnlock     = dllKernel32.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

func nativeClipboard() Clipboard { return winClipboard{} }

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
	if !open() {
		return ""
	}
	defer procCloseClipboard.Call()
	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(h)
	s := windows.UTF16PtrToString((*uint16)(global(p)))
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
	if !open() {
		return
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	size := uintptr(len(u)) * 2
	h, _, _ := procGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return
	}
	copy(unsafe.Slice((*uint16)(global(p)), len(u)), u)
	procGlobalUnlock.Call(h)
	// On success the clipboard owns the memory; otherwise it is still ours.
	if r, _, _ := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		procGlobalFree.Call(h)
	}
}

// global turns what GlobalLock returned into a pointer. The memory is the
// system's, not Go's, so the garbage collector has no reason to track it;
// the double conversion only keeps vet from mistaking it for Go memory.
func global(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }
