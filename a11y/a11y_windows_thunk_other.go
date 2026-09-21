//go:build windows && !amd64 && !arm64

package a11y

import "syscall"

// Without a thunk for the architecture, the two methods that take doubles
// cannot read their arguments at all, so they decline instead of guessing:
// a hit test that answers nothing leaves the client with the root, which is
// what it would fall back to anyway, and a slider that refuses to be set
// can still be moved a step at a time through the range value pattern.
//
// See a11y_windows_thunk.go for what the thunk does where there is one.

func winFromPointEntry() uintptr {
	return syscall.NewCallback(func(_, _, _, ppRetVal uintptr) uintptr {
		return winHandOut(0, 0, ppRetVal)
	})
}

func winRangeSetValueEntry() uintptr {
	return syscall.NewCallback(func(_, _ uintptr) uintptr { return uiaInvalidOperation })
}
