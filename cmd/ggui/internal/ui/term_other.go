//go:build !(darwin || dragonfly || freebsd || netbsd || openbsd || linux || windows)

package ui

func isTerminal(uintptr) bool { return false }
func enableVT(uintptr) bool   { return false }
func termWidth(uintptr) int   { return 0 }
