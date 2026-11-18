//go:build darwin || dragonfly || freebsd || netbsd || openbsd || linux

package ui

import "golang.org/x/sys/unix"

// isTerminal reports whether fd is a terminal: whether it has terminal
// attributes to read.
func isTerminal(fd uintptr) bool {
	_, err := unix.IoctlGetTermios(int(fd), ioctlReadTermios)
	return err == nil
}

// enableVT readies a terminal for escape sequences, which a Unix terminal
// always takes.
func enableVT(uintptr) bool { return true }

// termWidth returns the terminal's width in columns, or 0 when unknown.
func termWidth(fd uintptr) int {
	ws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
	if err != nil {
		return 0
	}
	return int(ws.Col)
}
