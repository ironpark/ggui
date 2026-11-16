package gotool

import "os"

// interrupt stops the go command. Windows has no interrupt to send one
// process, so it is killed; the compilers it started finish on their own.
func interrupt(p *os.Process) error { return p.Kill() }
