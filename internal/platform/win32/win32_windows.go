//go:build windows

// Package win32 holds the Windows system DLL handles, shared procedure
// lookups, and foreign-pointer conversions used across ggui's Windows platform
// packages. Everything resolves through windows.NewLazySystemDLL so libraries
// come only from the Windows system directory and load on first use.
package win32

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Shared Windows system DLL handles.
var (
	User32   = windows.NewLazySystemDLL("user32.dll")
	Kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	Ole32    = windows.NewLazySystemDLL("ole32.dll")
	OleAut32 = windows.NewLazySystemDLL("oleaut32.dll")
	Shell32  = windows.NewLazySystemDLL("shell32.dll")
)

// Shared procedures used across multiple Windows platform packages.
var (
	GlobalAlloc     = Kernel32.NewProc("GlobalAlloc")
	GlobalFree      = Kernel32.NewProc("GlobalFree")
	GlobalLock      = Kernel32.NewProc("GlobalLock")
	GlobalUnlock    = Kernel32.NewProc("GlobalUnlock")
	GetDpiForWindow = User32.NewProc("GetDpiForWindow")
	CoTaskMemFree   = Ole32.NewProc("CoTaskMemFree")
)

// Ptr turns a non-Go address returned by or received from Windows (such as
// a COM out-parameter, GlobalLock address, or window-procedure pointer)
// into an unsafe.Pointer. The memory belongs to the OS or C runtime, never
// to Go's heap; reading the pointer value through &addr makes that clear
// to go vet's unsafeptr analyzer.
func Ptr(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

// Deref interprets a non-Go address returned by or received from Windows
// as a pointer to T.
func Deref[T any](addr uintptr) *T {
	return (*T)(Ptr(addr))
}
