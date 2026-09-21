//go:build windows && 386

package a11y

import "unsafe"

// winRaisePropertyChanged tells UI Automation that one property of an
// element now reads differently.
//
// On a thirty-two bit Windows a struct argument is pushed onto the stack
// whole, so a variant occupies four words there rather than one pointer.
// Passing a pointer instead would leave the call twenty-four bytes short of
// what it reads, which is not a wrong answer but a crash.
func winRaisePropertyChanged(el, prop uintptr, old, fresh *winVariant) {
	const words = unsafe.Sizeof(winVariant{}) / unsafe.Sizeof(uintptr(0))
	o := unsafe.Slice((*uintptr)(unsafe.Pointer(old)), words)
	f := unsafe.Slice((*uintptr)(unsafe.Pointer(fresh)), words)
	args := make([]uintptr, 0, 2+2*words)
	args = append(args, el, prop)
	args = append(args, o...)
	args = append(args, f...)
	procUiaRaisePropertyChanged.Call(args...)
}
