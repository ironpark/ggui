//go:build windows && (amd64 || arm64)

package a11y

import "syscall"

// Two of the methods UI Automation calls take their arguments as doubles:
// IRawElementProviderFragmentRoot::ElementProviderFromPoint takes two, and
// IRangeValueProvider::SetValue takes one. A Go callback cannot receive
// either. Windows passes a floating-point argument in a floating-point
// register, and the entry point Go builds for a callback saves only the
// integer ones, so the value is simply not there by the time Go code runs.
//
// The fix is three instructions: a thunk that copies the floating-point
// registers into the integer registers the same arguments would have
// occupied, and then jumps to the callback Go built. Windows assigns
// registers by position rather than by type, so those integer slots are
// free -- nothing else was passed in them -- and the bits arrive intact.
// The Go side reads them back with math.Float64frombits.
//
// The thunks are in a11y_windows_amd64.s and a11y_windows_arm64.s. Where
// there is neither, a11y_windows_thunk_other.go stands in with methods
// that decline rather than guess.

// The callbacks the thunks jump to. They are read from assembly, which is
// why they are plain package-level words rather than anything cleverer.
var (
	winFromPointCB uintptr
	winRangeSetCB  uintptr
)

// The addresses of the thunks themselves, defined as data in the assembly
// files because the address wanted is the raw one, not the wrapper Go would
// hand back for a function value.
var (
	winFromPointThunkAddr uintptr
	winRangeSetThunkAddr  uintptr
)

// The thunks. Declared here so that the assembler's definitions have a Go
// declaration to be checked against; nothing calls them from Go.
func winFromPointThunk()
func winRangeSetThunk()

// winFromPointEntry is what goes in the fragment root's method table.
func winFromPointEntry() uintptr {
	winFromPointCB = syscall.NewCallback(winElementFromPoint)
	return winFromPointThunkAddr
}

// winRangeSetValueEntry is what goes in the range value method table.
func winRangeSetValueEntry() uintptr {
	winRangeSetCB = syscall.NewCallback(winRangeSetValue)
	return winRangeSetThunkAddr
}
