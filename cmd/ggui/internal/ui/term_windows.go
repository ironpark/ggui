package ui

import "golang.org/x/sys/windows"

// isTerminal reports whether fd is a console.
func isTerminal(fd uintptr) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(fd), &mode) == nil
}

// enableVT turns on the console's processing of escape sequences, which
// Windows 10 and later have and older consoles refuse.
func enableVT(fd uintptr) bool {
	h := windows.Handle(fd)
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}

// termWidth returns the console window's width in columns, or 0.
func termWidth(fd uintptr) int {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(fd), &info); err != nil {
		return 0
	}
	return int(info.Window.Right-info.Window.Left) + 1
}
