// Package proc runs the application under development as a child that can
// be stopped completely: the program and whatever it started.
//
// The child is put in a group of its own, a process group on Unix and a
// job object on Windows. That does two things. The terminal's Ctrl-C goes
// to the foreground group, ggui's, and no longer reaches the app directly,
// so ggui decides how the app stops, the same way whether the user pressed
// Ctrl-C or a rebuild replaced it. And stopping signals the whole group,
// so a helper the app spawned does not outlive it. On Windows the job is
// also set to kill its members when its last handle closes, which happens
// when ggui exits by any means, a crash included; on Linux the child gets
// SIGKILL when ggui dies. macOS has neither, so there a ggui killed with
// SIGKILL leaves the app running, as any parent would.
package proc

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Options says how to start a process.
type Options struct {
	Dir    string
	Env    []string // the whole environment; nil for ggui's own
	Stdout io.Writer
	Stderr io.Writer
}

// Process is a started child and its group.
type Process struct {
	cmd  *exec.Cmd
	grp  group
	done chan struct{}
	err  error // the exit status, once done is closed

	stopOnce sync.Once
	stopErr  error
}

// Start starts name with args in a group of its own. Its standard input
// is empty: the app reads nothing from the terminal, and a process outside
// the terminal's foreground group that tried would be stopped.
func Start(name string, args []string, o Options) (*Process, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = o.Dir
	cmd.Env = o.Env
	cmd.Stdout, cmd.Stderr = o.Stdout, o.Stderr
	cmd.SysProcAttr = sysProcAttr()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &Process{cmd: cmd, done: make(chan struct{})}
	grp, err := newGroup(cmd.Process)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, err
	}
	p.grp = grp
	go func() {
		p.err = cmd.Wait()
		// Whatever the app left running goes with it, now, rather than at
		// the next stop.
		p.grp.kill()
		close(p.done)
	}()
	return p, nil
}

// Pid returns the child's process ID.
func (p *Process) Pid() int { return p.cmd.Process.Pid }

// Done is closed when the child has exited.
func (p *Process) Done() <-chan struct{} { return p.done }

// Err returns how the child exited, once Done is closed: nil for status 0,
// an *exec.ExitError otherwise.
func (p *Process) Err() error { return p.err }

// ErrKilled reports that Stop had to kill the child.
var ErrKilled = errors.New("did not exit in time and was killed")

// Stop asks the child's group to exit, as a terminal or a window manager
// would, waits up to timeout, then kills the group. It returns once the
// child has exited: nil when it stopped when asked, ErrKilled otherwise.
// Calling it again, or after the child exited on its own, returns at once.
func (p *Process) Stop(timeout time.Duration) error {
	p.stopOnce.Do(func() {
		select {
		case <-p.done:
		default:
			p.grp.interrupt()
			select {
			case <-p.done:
			case <-time.After(timeout):
				p.grp.kill()
				<-p.done
				p.stopErr = ErrKilled
			}
		}
		p.grp.close()
	})
	return p.stopErr
}

// Kill kills the child's group without asking first, for a second Ctrl-C
// that will not wait for a graceful stop. A Stop in progress then returns.
func (p *Process) Kill() {
	select {
	case <-p.done:
	default:
		p.grp.kill()
	}
}

// Exited describes how a child that is done exited, for a message.
func (p *Process) Exited() string {
	var ee *exec.ExitError
	switch {
	case p.err == nil:
		return "exited"
	case errors.As(p.err, &ee):
		return ee.ProcessState.String()
	default:
		return p.err.Error()
	}
}

// Environ returns ggui's environment with env added, a later entry for a
// name replacing an earlier one.
func Environ(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := os.Environ()
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}
