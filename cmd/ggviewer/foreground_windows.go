package main

import "syscall"

var procAllowSetForegroundWindow = syscall.NewLazyDLL("user32.dll").NewProc("AllowSetForegroundWindow")

// allowForeground lets the app's process bring a window of its own to the
// front: Windows lets only the process the user works in do so, which is
// the viewer, and an app's dialog would open behind it. The permission
// holds until the next input, so it is given with each press.
func allowForeground(pid int) {
	_, _, _ = procAllowSetForegroundWindow.Call(uintptr(pid))
}
