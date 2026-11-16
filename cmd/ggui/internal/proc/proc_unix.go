//go:build unix

package proc

import (
	"os"
	"syscall"
)

// group is the child's process group, whose ID is the child's PID.
type group struct{ pgid int }

func newGroup(p *os.Process) (group, error) { return group{pgid: p.Pid}, nil }

// interrupt sends SIGTERM, the request to exit that a Go program, ggui's
// included, ends on by default and that a program wanting to save first
// can catch.
func (g group) interrupt() { syscall.Kill(-g.pgid, syscall.SIGTERM) }

// kill sends SIGKILL to every process left in the group. Once the group is
// empty there is nothing to send to and the call fails harmlessly.
func (g group) kill() { syscall.Kill(-g.pgid, syscall.SIGKILL) }

func (g group) close() {}
