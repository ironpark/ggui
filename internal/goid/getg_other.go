//go:build !amd64 && !arm64 && !386 && !arm

package goid

import "unsafe"

func getg() unsafe.Pointer { return nil }

const haveGetg = false
