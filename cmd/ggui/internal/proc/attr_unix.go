//go:build unix && !linux

package proc

import "syscall"

// sysProcAttr starts the child in a process group of its own.
func sysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
