//go:build windows && 386

package a11ybridge

import (
	"math"
	"syscall"
)

// On 32-bit Windows (__stdcall), double arguments are passed on the stack as
// two little-endian 32-bit words (low word first, high word second), and the
// callee must pop the exact byte count of all arguments. Matching the exact
// number of 32-bit stack slots here preserves the stack pointer and lets both
// methods decode their float64 arguments directly without an assembly thunk.

func winFromPointEntry() uintptr {
	return syscall.NewCallback(func(this, xLo, xHi, yLo, yHi, ppRetVal uintptr) uintptr {
		x := math.Float64frombits(uint64(xLo) | uint64(xHi)<<32)
		y := math.Float64frombits(uint64(yLo) | uint64(yHi)<<32)
		return winElementFromPointXY(this, x, y, ppRetVal)
	})
}

func winRangeSetValueEntry() uintptr {
	return syscall.NewCallback(func(this, vLo, vHi uintptr) uintptr {
		v := math.Float64frombits(uint64(vLo) | uint64(vHi)<<32)
		return winRangeSetValueF64(this, v)
	})
}
