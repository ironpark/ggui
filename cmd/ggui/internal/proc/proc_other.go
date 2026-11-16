//go:build !unix && !windows

package proc

import (
	"os"
	"syscall"
)

// Elsewhere there is no group to put the child in: it is the whole group,
// and stopping it is killing it.
func sysProcAttr() *syscall.SysProcAttr { return nil }

type group struct{ p *os.Process }

func newGroup(p *os.Process) (group, error) { return group{p}, nil }
func (g group) interrupt()                  { g.p.Kill() }
func (g group) kill()                       { g.p.Kill() }
func (g group) close()                      {}
