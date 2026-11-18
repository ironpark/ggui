//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package ui

import "golang.org/x/sys/unix"

const ioctlReadTermios = unix.TIOCGETA
