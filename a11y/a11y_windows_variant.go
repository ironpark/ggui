//go:build windows && !386

package a11y

import "unsafe"

// winRaisePropertyChanged tells UI Automation that one property of an
// element now reads differently.
//
// The two variants are declared as arguments by value. On a sixty-four bit
// Windows anything wider than a word is passed as a pointer to the caller's
// copy, so that is what goes across here; see the thirty-two bit file for
// the other half of this.
func winRaisePropertyChanged(el, prop uintptr, old, fresh *winVariant) {
	procUiaRaisePropertyChanged.Call(el, prop,
		uintptr(unsafe.Pointer(old)), uintptr(unsafe.Pointer(fresh)))
}
