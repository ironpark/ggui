package runtime

import (
	"os/exec"
	"syscall"
)

// explorerSelect is Explorer selecting path. Explorer parses its own
// command line and wants the path quoted after the comma, which Go's
// quoting of a whole argument would not do, so the line is written here.
func explorerSelect(path string) *exec.Cmd {
	cmd := exec.Command("explorer")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer /select,"` + path + `"`}
	return cmd
}
