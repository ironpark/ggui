//go:build unix && !ios && !android

package main

// allowForeground does nothing: the app's dialogs come to the front by
// themselves (macOS: see ggui's runtime/remote_darwin.go).
func allowForeground(pid int) {}
