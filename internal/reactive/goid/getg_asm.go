//go:build amd64 || arm64 || 386 || arm

package goid

import "unsafe"

// getg returns the running goroutine's g, the runtime's record of it.
func getg() unsafe.Pointer

const haveGetg = true
