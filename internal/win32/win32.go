//go:build windows

// Package win32 is the little of the Windows API that ggui's platform
// services share: today, finding the app's window. Everything is reached
// through lazily loaded system DLLs, so the project stays free of cgo.
package win32

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NewLazySystemDLL resolves the libraries out of the system directory
// rather than out of whatever is beside the executable, and nothing is
// loaded until the first call.
var (
	dllUser32   = windows.NewLazySystemDLL("user32.dll")
	dllKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procGetCurrentThreadID = dllKernel32.NewProc("GetCurrentThreadId")
	procEnumThreadWindows  = dllUser32.NewProc("EnumThreadWindows")
	procGetClassNameW      = dllUser32.NewProc("GetClassNameW")
	procIsWindowVisible    = dllUser32.NewProc("IsWindowVisible")
)

// glfwClass is the window class Ebitengine's GLFW registers, from
// _GLFW_WNDCLASSNAME in its win32 platform header.
const glfwClass = "GLFW30"

// enumProc is the enumeration callback, and found is where it leaves its
// answer. Both are package-level because a callback is never freed and the
// process may only ever have a few thousand: building one per attempt,
// once a second until a window appears, would eventually exhaust them.
// Only the main thread runs this, so the shared word needs no lock.
var (
	enumProc = syscall.NewCallback(enumWindow)
	found    uintptr
)

// AppWindow returns Ebitengine's window, or 0 while there is none. It must
// run on the main thread, which is the one that owns the window.
//
// GLFW gives its real window and its hidden message window the same class
// name, so the class alone is not enough to tell them apart: the message
// window is one pixel square and never shown, so visibility is what
// separates them. Do not fall back to any window of the thread; a file
// dialog is one too.
func AppWindow() uintptr {
	found = 0
	tid, _, _ := procGetCurrentThreadID.Call()
	procEnumThreadWindows.Call(tid, enumProc, 0)
	return found
}

func enumWindow(h, _ uintptr) uintptr {
	var name [32]uint16
	n, _, _ := procGetClassNameW.Call(h, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	if n == 0 || windows.UTF16ToString(name[:n]) != glfwClass {
		return 1
	}
	if vis, _, _ := procIsWindowVisible.Call(h); vis == 0 {
		return 1
	}
	found = h
	return 0
}
