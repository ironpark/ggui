//go:build unix

package gotool

import (
	"os"
	"os/exec"
	"syscall"
)

// interrupt asks the go command to stop as Ctrl-C would.
func interrupt(p *os.Process) error { return p.Signal(os.Interrupt) }

// killTree has cmd, a build, start in a process group of its own and
// stop, when its context is done, by killing the group: the go command
// and the compilers and linker it started, which an interrupt to the go
// command alone would leave to finish. The go command dies of an
// interrupt as of a kill, without removing its work directory, so the two
// differ in nothing else. The build cache is written file by file, each
// renamed into place, and stays valid.
func killTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
