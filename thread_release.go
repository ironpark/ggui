//go:build !ggui_debug

package ggui

// The UI-goroutine check costs nothing unless the ggui_debug build tag is
// set; see thread_debug.go for what it does and why.

func markUIThread() {}

func unmarkUIThread() {}

func checkUIThread(op string) {}
