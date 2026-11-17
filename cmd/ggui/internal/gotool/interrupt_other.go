//go:build !unix && !windows

package gotool

import (
	"os"
	"os/exec"
)

func interrupt(p *os.Process) error { return p.Kill() }

func killTree(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Kill() }
}
