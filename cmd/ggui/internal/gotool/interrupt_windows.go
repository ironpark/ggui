package gotool

import (
	"os"
	"os/exec"
	"strconv"
)

// interrupt stops the go command. Windows has no interrupt to send one
// process, so it is killed; the compilers it started finish on their own.
func interrupt(p *os.Process) error { return p.Kill() }

// killTree has cmd, a build, stop, when its context is done, by killing
// the go command and the compilers and linker it started, which taskkill
// finds by their parents. The build cache stays valid: it is written file
// by file, each renamed into place.
func killTree(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		tk := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := tk.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
