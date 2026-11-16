//go:build !windows

package gotool

import "os"

// interrupt asks the go command to stop as Ctrl-C would, which it passes on
// to the compilers it started.
func interrupt(p *os.Process) error { return p.Signal(os.Interrupt) }
