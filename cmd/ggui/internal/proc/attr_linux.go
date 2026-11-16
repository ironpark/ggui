package proc

import "syscall"

// sysProcAttr starts the child in a process group of its own and has the
// kernel kill it if ggui dies first.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
